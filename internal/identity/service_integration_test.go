package identity

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeVerifier struct{ subject string }

func (f fakeVerifier) Verify(_ context.Context, _, nonce string) (AppleIdentity, error) {
	return AppleIdentity{Subject: f.subject + nonce}, nil
}

func TestServiceLoginMigrationProgressAndDelete(t *testing.T) {
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
	var contentID uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM contents WHERE status='published' LIMIT 1").Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	service := NewService(pool, fakeVerifier{subject: "integration-"}, time.Hour)
	now := time.Now().UTC()
	result, err := service.Login(ctx, "token", uuid.NewString(), MigrationInput{Bookmarks: []uuid.UUID{contentID}, Progress: []ProgressInput{{ContentID: contentID, PositionSeconds: 42, ClientUpdatedAt: now}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(ctx, "token", uuid.NewString(), MigrationInput{}); err == nil {
		t.Fatal("expected identity token replay rejection")
	}
	user, err := service.Authenticate(ctx, result.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != result.UserID {
		t.Fatal("session user mismatch")
	}
	if err := service.SetProgress(ctx, user.ID, ProgressInput{ContentID: contentID, PositionSeconds: 1, ClientUpdatedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	progress, err := service.Queries().ListProgress(ctx, user.ID)
	if err != nil || len(progress) != 1 || progress[0].PositionSeconds != 42 {
		t.Fatalf("progress=%+v err=%v", progress, err)
	}
	if err := service.DeleteUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, result.AccessToken); err == nil {
		t.Fatal("deleted session still authenticates")
	}
}
