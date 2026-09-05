package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

func mountVocabularyRoutes(r chi.Router, service *identity.Service) {
	r.Get("/me/vocabulary", func(w http.ResponseWriter, request *http.Request) {
		rows, err := service.Queries().ListVocabularyEntries(request.Context(), currentUser(request).ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": vocabularyResponses(rows)})
	})

	r.Put("/me/vocabulary/{entryId}", func(w http.ResponseWriter, request *http.Request) {
		entryID, err := uuid.Parse(chi.URLParam(request, "entryId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		var body identity.VocabularyInput
		if json.NewDecoder(http.MaxBytesReader(w, request.Body, 8<<10)).Decode(&body) != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		body.ID = entryID
		row, err := service.SetVocabulary(request.Context(), currentUser(request).ID, body)
		if errors.Is(err, identity.ErrInvalidVocabulary) {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": vocabularyResponse(row)})
	})

	r.Delete("/me/vocabulary/{entryId}", func(w http.ResponseWriter, request *http.Request) {
		entryID, err := uuid.Parse(chi.URLParam(request, "entryId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		if err := service.DeleteVocabulary(request.Context(), currentUser(request).ID, entryID); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func vocabularyResponses(rows []dbgen.VocabularyEntry) []map[string]any {
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, vocabularyResponse(row))
	}
	return items
}

func vocabularyResponse(row dbgen.VocabularyEntry) map[string]any {
	var definition, sentenceContext, contentTitle any
	var contentID any
	if row.Definition.Valid {
		definition = row.Definition.String
	}
	if row.SentenceContext.Valid {
		sentenceContext = row.SentenceContext.String
	}
	if row.ContentID.Valid {
		contentID = uuid.UUID(row.ContentID.Bytes)
	}
	if row.ContentTitle.Valid {
		contentTitle = row.ContentTitle.String
	}
	return map[string]any{
		"id":           row.ID,
		"word":         row.Word,
		"definition":   definition,
		"context":      sentenceContext,
		"contentId":    contentID,
		"contentTitle": contentTitle,
		"createdAt":    row.CreatedAt.Time.UTC(),
		"updatedAt":    row.UpdatedAt.Time.UTC(),
	}
}
