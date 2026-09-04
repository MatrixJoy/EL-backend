package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/oldj/voa-learning-app/backend/internal/publishing"
)

type fakePublicationPublisher struct {
	called bool
}

func (f *fakePublicationPublisher) Publish(_ context.Context, key string, document publishing.Document, audio io.Reader) (publishing.Result, error) {
	f.called = true
	if key != "12345678901234567890123456789012" || document.Title != "Test" {
		return publishing.Result{}, publishing.ErrInvalidDocument
	}
	_, _ = io.Copy(io.Discard, audio)
	return publishing.Result{ContentID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"), Revision: 1, Environment: "test"}, nil
}

func TestPublicationEndpointRequiresBearerToken(t *testing.T) {
	publisher := &fakePublicationPublisher{}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), BuildInfo{}, Dependencies{Publisher: publisher, PublishToken: "12345678901234567890123456789012"})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/publications", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || publisher.called {
		t.Fatalf("status=%d called=%v", response.Code, publisher.called)
	}
}

func TestPublicationEndpointAcceptsMultipart(t *testing.T) {
	publisher := &fakePublicationPublisher{}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), BuildInfo{}, Dependencies{Publisher: publisher, PublishToken: "12345678901234567890123456789012"})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	documentPart, _ := writer.CreateFormField("document")
	_ = json.NewEncoder(documentPart).Encode(publishing.Document{Title: "Test"})
	audioPart, _ := writer.CreateFormFile("audio", "audio.mp3")
	_, _ = audioPart.Write([]byte("audio"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/publications", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer 12345678901234567890123456789012")
	request.Header.Set("Idempotency-Key", "12345678901234567890123456789012")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !publisher.called {
		t.Fatalf("status=%d body=%s called=%v", response.Code, response.Body.String(), publisher.called)
	}
}
