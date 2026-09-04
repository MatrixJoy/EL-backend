package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/platform/objectstore"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

const maxRetellRecordingBytes int64 = 25 << 20

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
	return map[string]any{
		"id": row.ID, "contentId": row.ContentID,
		"durationMilliseconds": row.DurationMilliseconds, "byteSize": row.ByteSize,
		"createdAt": row.CreatedAt.Time.UTC(), "serverUpdatedAt": row.ServerUpdatedAt.Time.UTC(),
		"audioPath": "/api/v1/me/retell-attempts/" + row.ID.String() + "/audio",
	}
}
