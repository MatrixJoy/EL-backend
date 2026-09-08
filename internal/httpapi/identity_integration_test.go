package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

type HTTPFakeAppleVerifier struct{}

func (HTTPFakeAppleVerifier) Verify(_ context.Context, _, nonce string) (identity.AppleIdentity, error) {
	return identity.AppleIdentity{Subject: "http-integration-" + nonce}, nil
}

func TestIdentityHTTPFlow(t *testing.T) {
	databaseURL := os.Getenv("VOA_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VOA_INTEGRATION_DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()
	sourceID := uuid.New()
	contentID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO source_items (id, source, canonical_url)
		VALUES ($1, 'integration-test', $2)`, sourceID, "https://example.invalid/http/"+contentID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contents (id, source_item_id, slug, type, title, status, published_at, released_at)
		VALUES ($1, $2, $3, 'article', 'HTTP integration fixture', 'published', now(), now())`, contentID, sourceID, "http-integration-"+contentID.String()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM contents WHERE id=$1", contentID)
		_, _ = pool.Exec(ctx, "DELETE FROM source_items WHERE id=$1", sourceID)
	}()
	service := identity.NewService(pool, HTTPFakeAppleVerifier{}, time.Hour)
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), BuildInfo{Version: "test"}, Dependencies{Queries: dbgen.New(pool), Identity: service})
	nonce := uuid.NewString()
	loginBody, _ := json.Marshal(map[string]any{"identityToken": "test", "nonce": nonce, "migration": map[string]any{"bookmarks": []uuid.UUID{contentID}}})
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/apple", bytes.NewReader(loginBody)))
	if login.Code != 200 {
		t.Fatalf("login=%d %s", login.Code, login.Body.String())
	}
	var envelope struct {
		Data identity.LoginResult `json:"data"`
	}
	if json.Unmarshal(login.Body.Bytes(), &envelope) != nil || envelope.Data.AccessToken == "" {
		t.Fatal("missing access token")
	}
	progressBody := bytes.NewBufferString(`{"positionSeconds":15,"completed":false,"clientUpdatedAt":"2026-09-01T00:00:00Z"}`)
	progressRequest := httptest.NewRequest(http.MethodPut, "/api/v1/me/progress/"+contentID.String(), progressBody)
	progressRequest.Header.Set("Authorization", "Bearer "+envelope.Data.AccessToken)
	progress := httptest.NewRecorder()
	router.ServeHTTP(progress, progressRequest)
	if progress.Code != http.StatusNoContent {
		t.Fatalf("progress=%d %s", progress.Code, progress.Body.String())
	}
	attemptID := uuid.New()
	grammarBody := bytes.NewBufferString(`{"contentId":"` + contentID.String() + `","contentTitle":"Grammar lesson","correctCount":2,"questionCount":3,"createdAt":"2026-09-01T00:00:00Z"}`)
	grammarRequest := httptest.NewRequest(http.MethodPut, "/api/v1/me/grammar-attempts/"+attemptID.String(), grammarBody)
	grammarRequest.Header.Set("Authorization", "Bearer "+envelope.Data.AccessToken)
	grammar := httptest.NewRecorder()
	router.ServeHTTP(grammar, grammarRequest)
	if grammar.Code != http.StatusOK {
		t.Fatalf("grammar=%d %s", grammar.Code, grammar.Body.String())
	}
	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meRequest.Header.Set("Authorization", "Bearer "+envelope.Data.AccessToken)
	me := httptest.NewRecorder()
	router.ServeHTTP(me, meRequest)
	if me.Code != 200 {
		t.Fatalf("me=%d", me.Code)
	}
	if !bytes.Contains(me.Body.Bytes(), []byte(attemptID.String())) {
		t.Fatalf("me response missing grammar attempt: %s", me.Body.String())
	}
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/me", nil)
	deleteRequest.Header.Set("Authorization", "Bearer "+envelope.Data.AccessToken)
	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, deleteRequest)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete=%d", deleted.Code)
	}
}
