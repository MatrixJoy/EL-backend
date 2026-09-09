package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestReadinessChecksDependenciesAndDoesNotLeakErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(logger, BuildInfo{}, Dependencies{Readiness: func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("readiness must have a deadline")
		}
		return errors.New("secret database address")
	}})
	for path, expected := range map[string]int{"/health/ready": 503, "/health/live": 200} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != expected || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestFeedbackPersistsBeforeAcknowledgement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var stored SupportRequest
	router := NewRouter(logger, BuildInfo{}, Dependencies{Support: func(_ context.Context, request SupportRequest) error { stored = request; return nil }})
	form := url.Values{"kind": {"content"}, "email": {"tester@example.invalid"}, "message": {"Please review this article attribution."}}
	r := httptest.NewRequest(http.MethodPost, "/support", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 201 || stored.Kind != "content" || !strings.Contains(w.Body.String(), stored.ID.String()) {
		t.Fatalf("response=%d %s, stored=%+v", w.Code, w.Body.String(), stored)
	}
	if strings.Contains(w.Body.String(), stored.Email) || strings.Contains(w.Body.String(), stored.Message) {
		t.Fatal("confirmation must not repeat private feedback")
	}
}

func TestFeedbackRejectsInvalidOrUnavailableSubmissions(t *testing.T) {
	for _, test := range []struct {
		kind, email, message string
		status               int
	}{
		{"problem", "", "short", 400},
		{"invalid", "", "A valid length message", 400},
		{"privacy", "bad-address", "A valid length message", 400},
		{"problem", "", "A valid length message", 503},
	} {
		router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), BuildInfo{})
		form := url.Values{"kind": {test.kind}, "email": {test.email}, "message": {test.message}}
		r := httptest.NewRequest("POST", "/support", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("status=%d expected=%d", w.Code, test.status)
		}
	}
}

func TestInformationPagesArePublicAndDoNotCachePersonalData(t *testing.T) {
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), BuildInfo{})
	for _, path := range []string{"/privacy", "/support"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("%s: %d %v", path, w.Code, w.Header())
		}
	}
}
