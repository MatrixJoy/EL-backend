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

func mountGrammarRoutes(r chi.Router, service *identity.Service) {
	r.Get("/me/grammar-attempts", func(w http.ResponseWriter, request *http.Request) {
		rows, err := service.Queries().ListGrammarAttempts(request.Context(), currentUser(request).ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": grammarAttemptResponses(rows)})
	})

	r.Put("/me/grammar-attempts/{attemptId}", func(w http.ResponseWriter, request *http.Request) {
		attemptID, err := uuid.Parse(chi.URLParam(request, "attemptId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		var body identity.GrammarAttemptInput
		request.Body = http.MaxBytesReader(w, request.Body, 8<<10)
		if json.NewDecoder(request.Body).Decode(&body) != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		body.ID = attemptID
		row, err := service.SetGrammarAttempt(request.Context(), currentUser(request).ID, body)
		if errors.Is(err, identity.ErrInvalidGrammarAttempt) {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": grammarAttemptResponse(row)})
	})
}

func grammarAttemptResponses(rows []dbgen.GrammarAttempt) []map[string]any {
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, grammarAttemptResponse(row))
	}
	return items
}

func grammarAttemptResponse(row dbgen.GrammarAttempt) map[string]any {
	return map[string]any{
		"id": row.ID, "contentId": row.ContentID, "contentTitle": row.ContentTitle,
		"correctCount": row.CorrectCount, "questionCount": row.QuestionCount,
		"createdAt": row.CreatedAt.Time.UTC(), "updatedAt": row.UpdatedAt.Time.UTC(),
	}
}
