package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/platform/objectstore"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

type BuildInfo struct {
	Version string
}

type Dependencies struct {
	Readiness       func(context.Context) error
	Support         func(context.Context, SupportRequest) error
	Queries         *dbgen.Queries
	MediaClient     *http.Client
	MediaStore      *objectstore.Store
	UserMediaStore  recordingObjectStore
	Identity        *identity.Service
	Publisher       publicationPublisher
	PublishToken    string
	DevelopmentAuth bool
}

const (
	publicRequestsPerMinute  = 600
	publicRequestBurst       = 120
	accountRequestsPerMinute = 120
	accountRequestBurst      = 30
)

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
	router.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if deps.Readiness != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := deps.Readiness(ctx); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ready",
			"version": build.Version,
		})
	})
	mountInformationPages(router, deps)
	router.Route("/api/v1", func(r chi.Router) {
		r.Group(func(public chi.Router) {
			public.Use(newIPRateLimiter(publicRequestsPerMinute, publicRequestBurst).middleware)
			public.Get("/bootstrap", func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, http.StatusOK, map[string]any{
					"data": map[string]any{
						"apiVersion": "v1",
						"anonymous":  true,
						"features": map[string]bool{
							"mediaStreaming":  true,
							"offlineMedia":    false,
							"developmentAuth": deps.DevelopmentAuth,
						},
					},
				})
			})
			if deps.Queries != nil {
				mountPublicRoutes(public, deps)
			}
		})
		if deps.Identity != nil {
			r.Group(func(account chi.Router) {
				account.Use(newIPRateLimiter(accountRequestsPerMinute, accountRequestBurst).middleware)
				mountIdentityRoutes(account, deps.Identity, deps.UserMediaStore, deps.DevelopmentAuth)
			})
		}
	})
	if deps.Publisher != nil && deps.PublishToken != "" {
		mountPublishingRoutes(router, deps.Publisher, deps.PublishToken)
	}
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
