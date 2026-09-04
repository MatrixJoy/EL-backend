package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/oldj/english-learning/backend/internal/publishing"
)

const maxPublicationBytes = int64(260 << 20)

type publicationPublisher interface {
	Publish(ctx context.Context, idempotencyKey string, document publishing.Document, audio io.Reader) (publishing.Result, error)
}

type publicationHandlers struct {
	publisher publicationPublisher
	token     string
}

func mountPublishingRoutes(r chi.Router, publisher publicationPublisher, token string) {
	h := publicationHandlers{publisher: publisher, token: token}
	r.Route("/internal/v1", func(r chi.Router) {
		r.Use(h.bearerAuth)
		r.Post("/publications", h.publish)
	})
}

func (h publicationHandlers) bearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		providedHash := sha256.Sum256([]byte(provided))
		expectedHash := sha256.Sum256([]byte(h.token))
		if provided == "" || subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"code": "UNAUTHORIZED", "message": "Unauthorized"}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h publicationHandlers) publish(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPublicationBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "INVALID_MULTIPART", "message": "Invalid multipart request"}})
		return
	}
	documentPart, err := reader.NextPart()
	if err != nil || documentPart.FormName() != "document" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "DOCUMENT_REQUIRED", "message": "The document part must be first"}})
		return
	}
	documentBody, err := io.ReadAll(io.LimitReader(documentPart, 5<<20))
	_ = documentPart.Close()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "DOCUMENT_INVALID", "message": "Could not read document"}})
		return
	}
	var document publishing.Document
	if err = json.Unmarshal(documentBody, &document); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "DOCUMENT_INVALID", "message": "Invalid publication document"}})
		return
	}
	audioPart, err := reader.NextPart()
	if err != nil || audioPart.FormName() != "audio" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "AUDIO_REQUIRED", "message": "Audio part is required"}})
		return
	}
	defer audioPart.Close()
	result, err := h.publisher.Publish(r.Context(), r.Header.Get("Idempotency-Key"), document, audioPart)
	if errors.Is(err, publishing.ErrInvalidDocument) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"code": "DOCUMENT_INVALID", "message": err.Error()}})
		return
	}
	if errors.Is(err, publishing.ErrAudioIntegrity) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{"code": "AUDIO_INTEGRITY_FAILED", "message": err.Error()}})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"code": "PUBLISH_FAILED", "message": "Publication failed"}})
		return
	}
	status := http.StatusCreated
	if result.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"data": result})
}
