package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/oldj/voa-learning-app/backend/internal/identity"
	"github.com/oldj/voa-learning-app/backend/internal/repository/dbgen"
)

type BuildInfo struct {
	Version string
}

type Dependencies struct {
	Queries     *dbgen.Queries
	MediaClient *http.Client
	Identity    *identity.Service
}

func NewRouter(logger *slog.Logger, build BuildInfo, dependencies ...Dependencies) http.Handler {
	var deps Dependencies
	if len(dependencies) > 0 {
		deps = dependencies[0]
	}
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(accessLog(logger))

	router.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	router.Get("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ready",
			"version": build.Version,
		})
	})
	router.Route("/api/v1", func(r chi.Router) {
		r.Use(newIPRateLimiter(120, 30).middleware)
		r.Get("/bootstrap", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"apiVersion": "v1",
					"anonymous":  true,
					"features": map[string]bool{
						"mediaStreaming": true,
						"offlineMedia":   false,
					},
				},
			})
		})
		if deps.Queries != nil {
			mountPublicRoutes(r, deps)
		}
		if deps.Identity != nil {
			mountIdentityRoutes(r, deps.Identity)
		}
	})
	router.Get("/metrics", metricsHandler)

	return router
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", middleware.GetReqID(r.Context()),
				"duration_ms", time.Since(started).Milliseconds(),
			)
		})
	}
}
