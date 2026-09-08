package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAppleVerifierChecksSignatureClaimsAndNonce(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	exponent := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test-key", "n": base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(exponent)}}})
	}))
	defer server.Close()
	claims := jwt.MapClaims{"iss": "https://appleid.apple.com", "aud": "com.example.app", "sub": "apple-user", "nonce": "nonce-hash", "exp": time.Now().Add(time.Hour).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"
	raw, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewAppleJWTVerifier("com.example.app", server.Client())
	verifier.jwksURL = server.URL
	identity, err := verifier.Verify(context.Background(), raw, "nonce-hash")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Subject != "apple-user" {
		t.Fatalf("subject=%q", identity.Subject)
	}
	if _, err := verifier.Verify(context.Background(), raw, "wrong"); err == nil {
		t.Fatal("expected nonce rejection")
	}
}

func TestAppleVerifierAcceptsEllipticCurveSigningKey(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := privateKey.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "EC", "kid": "ec-test-key", "crv": "P-256",
			"x": base64.RawURLEncoding.EncodeToString(publicKey[1:33]),
			"y": base64.RawURLEncoding.EncodeToString(publicKey[33:]),
		}}})
	}))
	defer server.Close()
	claims := jwt.MapClaims{"iss": "https://appleid.apple.com", "aud": "com.example.app", "sub": "apple-ec-user", "nonce": "ec-nonce", "exp": time.Now().Add(time.Hour).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "ec-test-key"
	raw, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewAppleJWTVerifier("com.example.app", server.Client())
	verifier.jwksURL = server.URL
	identity, err := verifier.Verify(context.Background(), raw, "ec-nonce")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Subject != "apple-ec-user" {
		t.Fatalf("subject=%q", identity.Subject)
	}
}
