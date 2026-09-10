package identity

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testAppleOAuth struct {
	fail    bool
	revoked []string
}

func (f *testAppleOAuth) Exchange(context.Context, string) (AppleTokens, error) {
	return AppleTokens{IdentityToken: "exchanged", RefreshToken: "private-refresh"}, nil
}
func (f *testAppleOAuth) Revoke(_ context.Context, token string) error {
	if f.fail {
		return errors.New("offline")
	}
	f.revoked = append(f.revoked, token)
	return nil
}

func TestAppleCredentialsEncryptedAndDeletionRevocationRetries(t *testing.T) {
	databaseURL := os.Getenv("VOA_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VOA_INTEGRATION_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	oauth := &testAppleOAuth{fail: true}
	s := NewService(pool, fakeVerifier{subject: "oauth-"}, time.Hour)
	if _, err := s.LoginWithAuthorizationCode(ctx, "id", "nonce", "code", MigrationInput{}); !errors.Is(err, ErrAppleUnavailable) {
		t.Fatal("unconfigured login allowed")
	}
	if err := s.ConfigureAppleOAuth("cn.wozdou.ela", oauth, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	result, err := s.LoginWithAuthorizationCode(ctx, uuid.NewString(), uuid.NewString(), "code", MigrationInput{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.DeleteUser(ctx, result.UserID)
	var id uuid.UUID
	var encrypted []byte
	if err := pool.QueryRow(ctx, "SELECT id, encrypted_refresh_token FROM apple_credentials WHERE user_id=$1", result.UserID).Scan(&id, &encrypted); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DELETE FROM apple_credentials WHERE id=$1", id)
	if strings.Contains(string(encrypted), "private-refresh") {
		t.Fatal("plaintext token persisted")
	}
	if err := s.DeleteUser(ctx, result.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, result.AccessToken); err == nil {
		t.Fatal("deleted user session remained")
	}
	if err := s.ProcessAppleRevocations(ctx); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var detached bool
	if err := pool.QueryRow(ctx, "SELECT attempts, user_id IS NULL FROM apple_credentials WHERE id=$1", id).Scan(&attempts, &detached); err != nil || attempts != 1 || !detached {
		t.Fatalf("queue attempts=%d detached=%v err=%v", attempts, detached, err)
	}
	oauth.fail = false
	_, _ = pool.Exec(ctx, "UPDATE apple_credentials SET next_attempt_at=now() WHERE id=$1", id)
	if err := s.ProcessAppleRevocations(ctx); err != nil {
		t.Fatal(err)
	}
	var remaining int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM apple_credentials WHERE id=$1", id).Scan(&remaining)
	if remaining != 0 || len(oauth.revoked) != 1 || oauth.revoked[0] != "private-refresh" {
		t.Fatal("revocation did not remove credential")
	}
}
