package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AppleIdentity struct{ Subject string }
type AppleVerifier interface {
	Verify(context.Context, string, string) (AppleIdentity, error)
}

type AppleJWTVerifier struct {
	audience   string
	client     *http.Client
	mu         sync.Mutex
	keys       map[string]any
	keysExpire time.Time
	jwksURL    string
}

func NewAppleJWTVerifier(audience string, client *http.Client) *AppleJWTVerifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &AppleJWTVerifier{audience: audience, client: client, jwksURL: "https://appleid.apple.com/auth/keys"}
}

func (v *AppleJWTVerifier) Verify(ctx context.Context, rawToken, expectedNonce string) (AppleIdentity, error) {
	if v.audience == "" {
		return AppleIdentity{}, errors.New("apple client ID is not configured")
	}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256", "ES256"}), jwt.WithIssuer("https://appleid.apple.com"), jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithLeeway(30*time.Second))
	token, err := parser.Parse(rawToken, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing key ID")
		}
		return v.key(ctx, kid)
	})
	if err != nil || !token.Valid {
		return AppleIdentity{}, fmt.Errorf("verify Apple identity token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return AppleIdentity{}, errors.New("invalid claims")
	}
	subject, _ := claims["sub"].(string)
	nonce, _ := claims["nonce"].(string)
	if subject == "" || expectedNonce == "" || nonce != expectedNonce {
		return AppleIdentity{}, errors.New("identity token subject or nonce is invalid")
	}
	return AppleIdentity{Subject: subject}, nil
}

func (v *AppleJWTVerifier) key(ctx context.Context, kid string) (any, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().Before(v.keysExpire) {
		if key := v.keys[kid]; key != nil {
			return key, nil
		}
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	response, err := v.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apple JWKS status %d", response.StatusCode)
	}
	var set struct {
		Keys []struct{ Kty, Kid, N, E, Crv, X, Y string } `json:"keys"`
	}
	if err := json.NewDecoder(response.Body).Decode(&set); err != nil {
		return nil, err
	}
	v.keys = make(map[string]any)
	for _, item := range set.Keys {
		switch item.Kty {
		case "RSA":
			nBytes, nErr := base64.RawURLEncoding.DecodeString(item.N)
			eBytes, eErr := base64.RawURLEncoding.DecodeString(item.E)
			if nErr != nil || eErr != nil {
				continue
			}
			e := 0
			for _, value := range eBytes {
				e = e<<8 + int(value)
			}
			v.keys[item.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
		case "EC":
			if item.Crv != "P-256" {
				continue
			}
			x, xErr := base64.RawURLEncoding.DecodeString(item.X)
			y, yErr := base64.RawURLEncoding.DecodeString(item.Y)
			if xErr != nil || yErr != nil {
				continue
			}
			curve := elliptic.P256()
			coordinateSize := (curve.Params().BitSize + 7) / 8
			if len(x) > coordinateSize || len(y) > coordinateSize {
				continue
			}
			encoded := make([]byte, 1+2*coordinateSize)
			encoded[0] = 4
			copy(encoded[1+coordinateSize-len(x):1+coordinateSize], x)
			copy(encoded[1+2*coordinateSize-len(y):], y)
			key, parseErr := ecdsa.ParseUncompressedPublicKey(curve, encoded)
			if parseErr != nil {
				continue
			}
			v.keys[item.Kid] = key
		}
	}
	v.keysExpire = time.Now().Add(time.Hour)
	key := v.keys[kid]
	if key == nil {
		return nil, errors.New("apple signing key not found")
	}
	return key, nil
}
