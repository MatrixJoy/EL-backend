package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestConfigurePreservesUnrelatedSettingsAndEncryptionKey(t *testing.T) {
	key := testKey(t)
	old := "# production\nPASSWORD=unchanged=exactly\nLEARNING_APPLE_CLIENT_ID=old.app\n"
	first, err := configure([]byte(old), key, "cn.wozdou.ela", "34L323UH86", "9Q234VQTL6")
	if err != nil {
		t.Fatal(err)
	}
	second, err := configure(first, key, "cn.wozdou.ela", "34L323UH86", "9Q234VQTL6")
	if err != nil || string(first) != string(second) {
		t.Fatal("repeat configuration rotated a credential")
	}
	if !strings.Contains(string(first), "PASSWORD=unchanged=exactly\n") || strings.Contains(string(first), "BEGIN PRIVATE KEY") {
		t.Fatal("unrelated setting changed or multiline key emitted")
	}
	if strings.Count(string(first), "LEARNING_APPLE_CLIENT_ID=") != 1 {
		t.Fatal("duplicate setting")
	}
	for _, line := range strings.Split(string(first), "\n") {
		if value, ok := strings.CutPrefix(line, "LEARNING_APPLE_TOKEN_ENCRYPTION_KEY="); ok {
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err != nil || len(decoded) != 32 {
				t.Fatal("invalid encryption key")
			}
		}
	}
}

func TestConfigureRejectsInvalidInputsWithoutReplacingKey(t *testing.T) {
	key := testKey(t)
	for _, previous := range []string{"LEARNING_APPLE_TOKEN_ENCRYPTION_KEY=bad\n", "LEARNING_APPLE_TOKEN_ENCRYPTION_KEY='bad'\n"} {
		if _, err := configure([]byte(previous), key, "cn.wozdou.ela", "34L323UH86", "9Q234VQTL6"); err == nil {
			t.Fatal("invalid existing key was overwritten")
		}
	}
	if _, err := configure(nil, []byte("not a private key"), "cn.wozdou.ela", "34L323UH86", "9Q234VQTL6"); err == nil {
		t.Fatal("invalid private key accepted")
	}
	if _, err := configure(nil, key, "app\nINJECT=1", "34L323UH86", "9Q234VQTL6"); err == nil {
		t.Fatal("environment injection accepted")
	}
}
