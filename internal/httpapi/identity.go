package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

type userContextKey struct{}

func mountIdentityRoutes(r chi.Router, service *identity.Service) {
	r.Post("/auth/apple", func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			IdentityToken string                  `json:"identityToken"`
			Nonce         string                  `json:"nonce"`
			Migration     identity.MigrationInput `json:"migration"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, request.Body, 1<<20)).Decode(&body) != nil {
			writeError(w, 400, "VALIDATION_ERROR", request)
			return
		}
		result, err := service.Login(request.Context(), body.IdentityToken, body.Nonce, body.Migration)
		if err != nil {
			writeError(w, 401, "UNAUTHORIZED", request)
			return
		}
		writeJSON(w, 200, map[string]any{"data": result})
	})
	r.Group(func(private chi.Router) {
		private.Use(authenticationMiddleware(service))
		private.Get("/me", func(w http.ResponseWriter, r *http.Request) {
			user := currentUser(r)
			bookmarks, _ := service.Queries().ListBookmarks(r.Context(), user.ID)
			progress, _ := service.Queries().ListProgress(r.Context(), user.ID)
			bookmarkItems := make([]map[string]any, 0, len(bookmarks))
			for _, row := range bookmarks {
				bookmarkItems = append(bookmarkItems, map[string]any{"contentId": row.ContentID, "updatedAt": row.UpdatedAt.Time.UTC()})
			}
			progressItems := make([]map[string]any, 0, len(progress))
			for _, row := range progress {
				progressItems = append(progressItems, map[string]any{"contentId": row.ContentID, "positionSeconds": row.PositionSeconds, "completed": row.Completed, "clientUpdatedAt": row.ClientUpdatedAt.Time.UTC(), "serverUpdatedAt": row.ServerUpdatedAt.Time.UTC()})
			}
			writeJSON(w, 200, map[string]any{"data": map[string]any{"id": user.ID, "bookmarks": bookmarkItems, "progress": progressItems}})
		})
		private.Put("/me/bookmarks/{contentId}", bookmarkHandler(service, true))
		private.Delete("/me/bookmarks/{contentId}", bookmarkHandler(service, false))
		private.Put("/me/progress/{contentId}", func(w http.ResponseWriter, r *http.Request) {
			contentID, err := uuid.Parse(chi.URLParam(r, "contentId"))
			if err != nil {
				writeError(w, 400, "VALIDATION_ERROR", r)
				return
			}
			var body identity.ProgressInput
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body) != nil {
				writeError(w, 400, "VALIDATION_ERROR", r)
				return
			}
			body.ContentID = contentID
			if body.ClientUpdatedAt.IsZero() || body.PositionSeconds < 0 {
				writeError(w, 400, "VALIDATION_ERROR", r)
				return
			}
			if service.SetProgress(r.Context(), currentUser(r).ID, body) != nil {
				writeError(w, 500, "INTERNAL_ERROR", r)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		private.Get("/me/sync", func(w http.ResponseWriter, r *http.Request) {
			sequence := int64(0)
			if raw := r.URL.Query().Get("cursor"); raw != "" {
				decoded, err := base64.RawURLEncoding.DecodeString(raw)
				if err != nil {
					writeError(w, 400, "VALIDATION_ERROR", r)
					return
				}
				sequence, err = strconv.ParseInt(string(decoded), 10, 64)
				if err != nil {
					writeError(w, 400, "VALIDATION_ERROR", r)
					return
				}
			}
			events, err := service.Queries().ListUserEventsAfter(r.Context(), dbgen.ListUserEventsAfterParams{UserID: currentUser(r).ID, Sequence: sequence, Limit: 100})
			if err != nil {
				writeError(w, 500, "INTERNAL_ERROR", r)
				return
			}
			next := sequence
			if len(events) > 0 {
				next = events[len(events)-1].Sequence
			}
			writeJSON(w, 200, map[string]any{"data": map[string]any{"changes": events, "nextCursor": base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(next, 10))), "hasMore": len(events) == 100}})
		})
		private.Delete("/me", func(w http.ResponseWriter, r *http.Request) {
			if service.DeleteUser(r.Context(), currentUser(r).ID) != nil {
				writeError(w, 500, "INTERNAL_ERROR", r)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}

func authenticationMiddleware(service *identity.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			value := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			user, err := service.Authenticate(r.Context(), value)
			if err != nil {
				writeError(w, 401, "UNAUTHORIZED", r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
		})
	}
}
func currentUser(r *http.Request) dbgen.User { return r.Context().Value(userContextKey{}).(dbgen.User) }
func bookmarkHandler(service *identity.Service, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "contentId"))
		if err != nil {
			writeError(w, 400, "VALIDATION_ERROR", r)
			return
		}
		if service.SetBookmark(r.Context(), currentUser(r).ID, id, enabled) != nil {
			writeError(w, 500, "INTERNAL_ERROR", r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
