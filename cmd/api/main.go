package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/english-learning/backend/internal/config"
	"github.com/oldj/english-learning/backend/internal/httpapi"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/platform/objectstore"
	"github.com/oldj/english-learning/backend/internal/publishing"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		logger.Error("ping database", "error", err)
		os.Exit(1)
	}
	mediaClient := httpapi.NewMediaClient(2 * time.Minute)
	mediaStore, err := objectstore.New(cfg.ObjectEndpoint, cfg.ObjectAccessKey, cfg.ObjectSecretKey, cfg.PublishedBucket, cfg.ObjectUseTLS)
	if err != nil {
		logger.Error("create media store", "error", err)
		os.Exit(1)
	}
	if err = ensureObjectStore(context.Background(), mediaStore); err != nil {
		logger.Error("prepare media store", "error", err)
		os.Exit(1)
	}
	userMediaStore, err := objectstore.New(cfg.ObjectEndpoint, cfg.ObjectAccessKey, cfg.ObjectSecretKey, cfg.UserMediaBucket, cfg.ObjectUseTLS)
	if err != nil {
		logger.Error("create user media store", "error", err)
		os.Exit(1)
	}
	if err = ensureObjectStore(context.Background(), userMediaStore); err != nil {
		logger.Error("prepare user media store", "error", err)
		os.Exit(1)
	}
	identityService := identity.NewService(pool, identity.NewAppleJWTVerifier(cfg.AppleClientID, nil), cfg.SessionTTL)
	publicationService := publishing.NewService(pool, mediaStore, cfg.Environment)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(logger, httpapi.BuildInfo{Version: "dev"}, httpapi.Dependencies{Queries: dbgen.New(pool), MediaClient: mediaClient, MediaStore: mediaStore, UserMediaStore: userMediaStore, Identity: identityService, Publisher: publicationService, PublishToken: cfg.CMSPublishToken, DevelopmentAuth: cfg.Environment != "production"}),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Environment)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("api shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("api stopped")
}

func ensureObjectStore(ctx context.Context, store *objectstore.Store) error {
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		if err = store.EnsureBucket(ctx); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return err
}
