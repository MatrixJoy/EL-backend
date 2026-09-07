package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/platform/objectstore"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

const maxRetellRecordingBytes int64 = 25 << 20

type retellReviewInput struct {
	Transcript      string    `json:"transcript"`
	MatchedKeywords []string  `json:"matchedKeywords"`
	KeywordCount    int32     `json:"keywordCount"`
	WordCount       int32     `json:"wordCount"`
	CompletionScore int32     `json:"completionScore"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type recordingObjectStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Open(context.Context, string) (objectstore.ReadSeekCloser, objectstore.ObjectInfo, error)
	Delete(context.Context, string) error
}

func mountRetellRoutes(r chi.Router, service *identity.Service, store recordingObjectStore) {
	r.Get("/me/retell-attempts", func(w http.ResponseWriter, request *http.Request) {
		contentID := pgtype.UUID{}
		if raw := strings.TrimSpace(request.URL.Query().Get("contentId")); raw != "" {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
				return
			}
			contentID = pgtype.UUID{Bytes: parsed, Valid: true}
		}
		rows, err := service.Queries().ListRetellAttempts(request.Context(), dbgen.ListRetellAttemptsParams{
			UserID: currentUser(request).ID, ContentID: contentID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			items = append(items, retellAttemptDTO(row))
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items})
	})

	r.Put("/me/retell-attempts/{attemptId}", func(w http.ResponseWriter, request *http.Request) {
		attemptID, contentID, duration, createdAt, ok := parseRetellUpload(request)
		if !ok {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		if request.ContentLength <= 0 || request.ContentLength > maxRetellRecordingBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "RECORDING_TOO_LARGE", request)
			return
		}
		contentType := strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0])
		if contentType != "audio/mp4" && contentType != "audio/x-m4a" && contentType != "audio/m4a" {
			writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", request)
			return
		}
		user := currentUser(request)
		objectKey := fmt.Sprintf("retell/%s/%s.m4a", user.ID, attemptID)
		body := http.MaxBytesReader(w, request.Body, maxRetellRecordingBytes)
		if err := store.Put(request.Context(), objectKey, body, request.ContentLength, contentType); err != nil {
			writeError(w, http.StatusBadGateway, "MEDIA_STORE_UNAVAILABLE", request)
			return
		}
		row, err := service.Queries().UpsertRetellAttempt(request.Context(), dbgen.UpsertRetellAttemptParams{
			UserID: user.ID, ID: attemptID, ContentID: contentID, ObjectKey: objectKey,
			DurationMilliseconds: duration, ByteSize: request.ContentLength, ContentType: contentType,
			CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
		})
		if err != nil {
			_ = store.Delete(request.Context(), objectKey)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": retellAttemptDTO(row)})
	})

	r.Put("/me/retell-attempts/{attemptId}/review", func(w http.ResponseWriter, request *http.Request) {
		attemptID, err := uuid.Parse(chi.URLParam(request, "attemptId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		var body retellReviewInput
		if json.NewDecoder(http.MaxBytesReader(w, request.Body, 32<<10)).Decode(&body) != nil || !body.normalizeAndValidate(time.Now()) {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		row, err := service.Queries().UpdateRetellReview(request.Context(), dbgen.UpdateRetellReviewParams{
			ClientUpdatedAt: pgtype.Timestamptz{Time: body.UpdatedAt.UTC(), Valid: true},
			Transcript:      pgtype.Text{String: body.Transcript, Valid: true},
			MatchedKeywords: body.MatchedKeywords,
			KeywordCount:    body.KeywordCount,
			WordCount:       body.WordCount,
			CompletionScore: body.CompletionScore,
			UserID:          currentUser(request).ID,
			ID:              attemptID,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			writeError(w, http.StatusNotFound, "RECORDING_NOT_FOUND", request)
			return
		case err != nil:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": retellAttemptDTO(row)})
	})

	serveAudio := func(w http.ResponseWriter, request *http.Request) {
		attemptID, err := uuid.Parse(chi.URLParam(request, "attemptId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		row, err := service.Queries().GetRetellAttempt(request.Context(), dbgen.GetRetellAttemptParams{UserID: currentUser(request).ID, ID: attemptID})
		if err != nil {
			writeError(w, http.StatusNotFound, "RECORDING_NOT_FOUND", request)
			return
		}
		object, info, err := store.Open(request.Context(), row.ObjectKey)
		if err != nil {
			writeError(w, http.StatusBadGateway, "MEDIA_STORE_UNAVAILABLE", request)
			return
		}
		defer object.Close()
		contentType := row.ContentType
		if contentType == "" {
			contentType = info.ContentType
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, request, attemptID.String()+".m4a", info.LastModified, object)
	}
	r.MethodFunc(http.MethodGet, "/me/retell-attempts/{attemptId}/audio", serveAudio)
	r.MethodFunc(http.MethodHead, "/me/retell-attempts/{attemptId}/audio", serveAudio)

	r.Delete("/me/retell-attempts/{attemptId}", func(w http.ResponseWriter, request *http.Request) {
		attemptID, err := uuid.Parse(chi.URLParam(request, "attemptId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		params := dbgen.GetRetellAttemptParams{UserID: currentUser(request).ID, ID: attemptID}
		row, err := service.Queries().GetRetellAttempt(request.Context(), params)
		if err != nil {
			writeError(w, http.StatusNotFound, "RECORDING_NOT_FOUND", request)
			return
		}
		if err = store.Delete(request.Context(), row.ObjectKey); err != nil {
			writeError(w, http.StatusBadGateway, "MEDIA_STORE_UNAVAILABLE", request)
			return
		}
		if _, err = service.Queries().DeleteRetellAttempt(request.Context(), dbgen.DeleteRetellAttemptParams{UserID: params.UserID, ID: params.ID}); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func parseRetellUpload(request *http.Request) (uuid.UUID, uuid.UUID, int32, time.Time, bool) {
	attemptID, err := uuid.Parse(chi.URLParam(request, "attemptId"))
	if err != nil {
		return uuid.Nil, uuid.Nil, 0, time.Time{}, false
	}
	contentID, err := uuid.Parse(request.Header.Get("X-Content-ID"))
	if err != nil {
		return uuid.Nil, uuid.Nil, 0, time.Time{}, false
	}
	duration64, err := strconv.ParseInt(request.Header.Get("X-Duration-Milliseconds"), 10, 32)
	if err != nil || duration64 < 0 || duration64 > 60*60*1000 {
		return uuid.Nil, uuid.Nil, 0, time.Time{}, false
	}
	createdAt, err := time.Parse(time.RFC3339, request.Header.Get("X-Created-At"))
	if err != nil || createdAt.After(time.Now().Add(5*time.Minute)) {
		return uuid.Nil, uuid.Nil, 0, time.Time{}, false
	}
	return attemptID, contentID, int32(duration64), createdAt, true
}

func retellAttemptDTO(row dbgen.RetellAttempt) map[string]any {
	var review any
	if row.Transcript.Valid {
		matchedKeywords := row.MatchedKeywords
		if matchedKeywords == nil {
			matchedKeywords = []string{}
		}
		review = map[string]any{
			"transcript":      row.Transcript.String,
			"matchedKeywords": matchedKeywords,
			"keywordCount":    row.KeywordCount,
			"wordCount":       row.WordCount,
			"completionScore": row.CompletionScore,
			"updatedAt":       row.ReviewClientUpdatedAt.Time.UTC(),
		}
	}
	return map[string]any{
		"id": row.ID, "contentId": row.ContentID,
		"durationMilliseconds": row.DurationMilliseconds, "byteSize": row.ByteSize,
		"createdAt": row.CreatedAt.Time.UTC(), "serverUpdatedAt": row.ServerUpdatedAt.Time.UTC(),
		"audioPath": "/api/v1/me/retell-attempts/" + row.ID.String() + "/audio",
		"review":    review,
	}
}

func (input *retellReviewInput) normalizeAndValidate(now time.Time) bool {
	input.Transcript = strings.TrimSpace(input.Transcript)
	if input.Transcript == "" || !utf8.ValidString(input.Transcript) || utf8.RuneCountInString(input.Transcript) > 20_000 ||
		input.KeywordCount < 0 || input.KeywordCount > 100 || input.WordCount < 1 || input.WordCount > 5_000 ||
		input.CompletionScore < 0 || input.CompletionScore > 100 || input.UpdatedAt.IsZero() || input.UpdatedAt.After(now.Add(5*time.Minute)) ||
		len(input.MatchedKeywords) > int(input.KeywordCount) {
		return false
	}
	seen := make(map[string]struct{}, len(input.MatchedKeywords))
	for index, keyword := range input.MatchedKeywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword == "" || utf8.RuneCountInString(keyword) > 100 {
			return false
		}
		if _, exists := seen[keyword]; exists {
			return false
		}
		seen[keyword] = struct{}{}
		input.MatchedKeywords[index] = keyword
	}
	return true
}
