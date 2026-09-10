package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestAppleOAuthExchangeAndRevoke(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	client, err := NewAppleOAuthClient("cn.wozdou.ela", "test-team", "test-key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.ParseForm() != nil || r.Form.Get("client_id") != "cn.wozdou.ela" {
			t.Error("invalid request")
		}
		token, err := jwt.Parse(r.Form.Get("client_secret"), func(token *jwt.Token) (any, error) { return &key.PublicKey, nil },
			jwt.WithValidMethods([]string{"ES256"}), jwt.WithIssuer("test-team"), jwt.WithAudience("https://appleid.apple.com"), jwt.WithSubject("cn.wozdou.ela"), jwt.WithExpirationRequired())
		if err != nil || token.Header["kid"] != "test-key" {
			t.Error("invalid client secret")
		}
		switch r.URL.Path {
		case "/token":
			if r.Form.Get("code") != "one-use" || r.Form.Get("grant_type") != "authorization_code" || r.Form.Has("redirect_uri") {
				t.Error("invalid code exchange")
			}
			_ = json.NewEncoder(w).Encode(AppleTokens{IdentityToken: "identity", RefreshToken: "refresh"})
		case "/revoke":
			if r.Form.Get("token") != "refresh" || r.Form.Get("token_type_hint") != "refresh_token" {
				t.Error("invalid revocation")
			}
		default:
			t.Error("unexpected endpoint")
		}
	}))
	defer server.Close()
	client.baseURL = server.URL
	tokens, err := client.Exchange(context.Background(), "one-use")
	if err != nil || tokens.RefreshToken != "refresh" {
		t.Fatalf("exchange: %v", err)
	}
	if err := client.Revoke(context.Background(), tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestAppleOAuthDoesNotLeakErrorBodiesOrFollowRedirects(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	client, _ := NewAppleOAuthClient("app", "team", "key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	for _, status := range []int{400, 429, 500, 302} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "https://example.invalid/never-follow")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"private-credential"}`))
		}))
		client.baseURL = server.URL
		_, err := client.Exchange(context.Background(), "code")
		server.Close()
		if err == nil || strings.Contains(err.Error(), "private-credential") {
			t.Fatalf("status=%d error=%v", status, err)
		}
	}
}

func TestAppleTokenEncryptionBindsRecordAndRejectsTampering(t *testing.T) {
	cipher, err := newTokenCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.seal("secret-refresh", "record-a")
	if err != nil || strings.Contains(string(sealed), "secret-refresh") {
		t.Fatal("plaintext leaked")
	}
	if value, err := cipher.open(sealed, "record-a"); err != nil || value != "secret-refresh" {
		t.Fatal("round trip failed")
	}
	if _, err := cipher.open(sealed, "record-b"); err == nil {
		t.Fatal("record swap accepted")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := cipher.open(sealed, "record-a"); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, err := newTokenCipher([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}
