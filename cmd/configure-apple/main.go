// configure-apple reads a .p8 from stdin so private key material never appears
// in command arguments, process listings or normal command output.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	envPath := flag.String("env", "", "existing environment file")
	backupPath := flag.String("backup", "", "new private backup path, outside deployment directory")
	clientID := flag.String("client-id", "", "native App ID")
	teamID := flag.String("team-id", "", "Apple team ID")
	keyID := flag.String("key-id", "", "Sign in with Apple Key ID")
	flag.Parse()
	if *envPath == "" || *backupPath == "" {
		fail(errors.New("environment and backup paths are required"))
	}
	key, err := io.ReadAll(io.LimitReader(os.Stdin, 8193))
	if err != nil {
		fail(errors.New("cannot read key input"))
	}
	if info, err := os.Lstat(*envPath); err != nil || !info.Mode().IsRegular() {
		fail(errors.New("environment must be an existing regular file"))
	}
	previous, err := os.ReadFile(*envPath)
	if err != nil {
		fail(errors.New("cannot read environment"))
	}
	updated, err := configure(previous, key, *clientID, *teamID, *keyID)
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(*backupPath), 0700); err != nil {
		fail(err)
	}
	backup, err := os.OpenFile(*backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fail(errors.New("cannot create new private environment backup"))
	}
	_, writeErr := backup.Write(previous)
	syncErr := backup.Sync()
	closeErr := backup.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		fail(errors.New("cannot complete environment backup"))
	}
	temporary, err := os.CreateTemp(filepath.Dir(*envPath), ".env.apple-*")
	if err != nil {
		fail(err)
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0600); err != nil {
		fail(err)
	}
	if _, err := temporary.Write(updated); err != nil {
		fail(err)
	}
	if err := temporary.Sync(); err != nil {
		fail(err)
	}
	if err := temporary.Close(); err != nil {
		fail(err)
	}
	if err := os.Rename(temporary.Name(), *envPath); err != nil {
		fail(err)
	}
	fmt.Println("Apple configuration saved; existing encryption key preserved when present; private backup created.")
}

func configure(previous, privateKey []byte, clientID, teamID, keyID string) ([]byte, error) {
	identifier := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
	if !identifier.MatchString(clientID) || !regexp.MustCompile(`^[A-Z0-9]{10}$`).MatchString(teamID) || !regexp.MustCompile(`^[A-Z0-9]{10}$`).MatchString(keyID) {
		return nil, errors.New("invalid Apple client, team or key identifier")
	}
	block, rest := pem.Decode(privateKey)
	if len(privateKey) > 8192 || block == nil || block.Type != "PRIVATE KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("expected one PKCS8 private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid private key")
	}
	ec, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, errors.New("Apple requires a P-256 private key")
	}
	values := map[string]string{
		"LEARNING_APPLE_CLIENT_ID": clientID, "LEARNING_APPLE_TEAM_ID": teamID, "LEARNING_APPLE_KEY_ID": keyID,
		"LEARNING_APPLE_PRIVATE_KEY_BASE64":   base64.StdEncoding.EncodeToString(privateKey),
		"LEARNING_APPLE_TOKEN_ENCRYPTION_KEY": "",
	}
	var kept []string
	for _, line := range strings.Split(strings.TrimSuffix(string(previous), "\n"), "\n") {
		name, value, found := strings.Cut(line, "=")
		name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "export "))
		if _, target := values[name]; target && found {
			if name == "LEARNING_APPLE_TOKEN_ENCRYPTION_KEY" && strings.TrimSpace(value) != "" {
				if values[name] != "" {
					return nil, errors.New("duplicate encryption key; resolve manually")
				}
				value = strings.Trim(strings.TrimSpace(value), "\"'")
				decoded, err := base64.StdEncoding.DecodeString(value)
				if err != nil || len(decoded) != 32 {
					return nil, errors.New("existing encryption key is invalid; refusing replacement")
				}
				values[name] = value
			}
			continue
		}
		kept = append(kept, line)
	}
	if values["LEARNING_APPLE_TOKEN_ENCRYPTION_KEY"] == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, errors.New("cannot generate encryption key")
		}
		values["LEARNING_APPLE_TOKEN_ENCRYPTION_KEY"] = base64.StdEncoding.EncodeToString(key)
	}
	for _, name := range []string{"LEARNING_APPLE_CLIENT_ID", "LEARNING_APPLE_TEAM_ID", "LEARNING_APPLE_KEY_ID", "LEARNING_APPLE_PRIVATE_KEY_BASE64", "LEARNING_APPLE_TOKEN_ENCRYPTION_KEY"} {
		kept = append(kept, name+"="+values[name])
	}
	return []byte(strings.Join(kept, "\n") + "\n"), nil
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
