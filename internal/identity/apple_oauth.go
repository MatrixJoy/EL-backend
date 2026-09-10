package identity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrAppleUnavailable = errors.New("Apple authentication is not configured")
var ErrAppleAuthorization = errors.New("Apple authorization was rejected")

type AppleTokens struct {
	IdentityToken string `json:"id_token"`
	RefreshToken  string `json:"refresh_token"`
}

type AppleOAuth interface {
	Exchange(context.Context, string) (AppleTokens, error)
	Revoke(context.Context, string) error
}

type AppleOAuthClient struct {
	clientID, teamID, keyID string
	key                     *ecdsa.PrivateKey
	client                  *http.Client
	baseURL                 string
}

func NewAppleOAuthClient(clientID, teamID, keyID string, privateKey []byte) (*AppleOAuthClient, error) {
	key, err := jwt.ParseECPrivateKeyFromPEM(privateKey)
	if err != nil || clientID == "" || teamID == "" || keyID == "" || key.Curve.Params().BitSize != 256 {
		return nil, errors.New("invalid Apple OAuth configuration")
	}
	return &AppleOAuthClient{clientID: clientID, teamID: teamID, keyID: keyID, key: key,
		client:  &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		baseURL: "https://appleid.apple.com/auth"}, nil
}

func (a *AppleOAuthClient) Exchange(ctx context.Context, code string) (AppleTokens, error) {
	if code == "" || len(code) > 8192 {
		return AppleTokens{}, ErrAppleAuthorization
	}
	var result AppleTokens
	err := a.post(ctx, "/token", url.Values{"code": {code}, "grant_type": {"authorization_code"}}, &result)
	if err != nil {
		return AppleTokens{}, err
	}
	if result.IdentityToken == "" || result.RefreshToken == "" {
		return AppleTokens{}, ErrAppleAuthorization
	}
	return result, nil
}

func (a *AppleOAuthClient) Revoke(ctx context.Context, token string) error {
	return a.post(ctx, "/revoke", url.Values{"token": {token}, "token_type_hint": {"refresh_token"}}, nil)
}

func (a *AppleOAuthClient) post(ctx context.Context, path string, form url.Values, result any) error {
	now := time.Now()
	secret := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.RegisteredClaims{
		Issuer: a.teamID, Subject: a.clientID, Audience: jwt.ClaimStrings{"https://appleid.apple.com"},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
	})
	secret.Header["kid"] = a.keyID
	signed, err := secret.SignedString(a.key)
	if err != nil {
		return ErrAppleUnavailable
	}
	form.Set("client_id", a.clientID)
	form.Set("client_secret", signed)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrAppleUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.client.Do(request)
	if err != nil {
		return ErrAppleUnavailable
	}
	defer response.Body.Close()
	// Never propagate Apple's response body: it may contain credentials.
	if response.StatusCode >= 500 || response.StatusCode == 429 {
		return ErrAppleUnavailable
	}
	if response.StatusCode != http.StatusOK {
		return ErrAppleAuthorization
	}
	if result != nil && json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(result) != nil {
		return ErrAppleAuthorization
	}
	return nil
}

type tokenCipher struct{ aead cipher.AEAD }

func newTokenCipher(key []byte) (*tokenCipher, error) {
	if len(key) != 32 {
		return nil, errors.New("Apple token encryption key must contain 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &tokenCipher{aead}, nil
}

func (c *tokenCipher) seal(token, recordID string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, []byte(token), []byte(recordID)), nil
}

func (c *tokenCipher) open(data []byte, recordID string) (string, error) {
	if len(data) < c.aead.NonceSize() {
		return "", errors.New("invalid encrypted Apple token")
	}
	value, err := c.aead.Open(nil, data[:c.aead.NonceSize()], data[c.aead.NonceSize():], []byte(recordID))
	return string(value), err
}
