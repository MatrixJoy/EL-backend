package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/voa-learning-app/backend/internal/config"
	"github.com/oldj/voa-learning-app/backend/internal/ingestion"
	"github.com/oldj/voa-learning-app/backend/internal/ingestion/voa"
	"github.com/oldj/voa-learning-app/backend/internal/platform/objectstore"
	"github.com/oldj/voa-learning-app/backend/internal/repository/dbgen"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		logger.Error("ping database", "error", err)
		os.Exit(1)
	}
	store, err := objectstore.New(cfg.ObjectEndpoint, cfg.ObjectAccessKey, cfg.ObjectSecretKey, cfg.ObjectBucket, cfg.ObjectUseTLS)
	if err != nil {
		logger.Error("object store", "error", err)
		os.Exit(1)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		logger.Error("ensure bucket", "error", err)
		os.Exit(1)
	}
	fetcher := voa.NewFetcher(&http.Client{Timeout: cfg.FetchTimeout}, cfg.UserAgent, "learningenglish.voanews.com")
	pipeline := ingestion.NewPipeline(pool, store, fetcher)
	redisOptions := asynq.RedisClientOpt{Addr: cfg.RedisAddr}
	queueClient := asynq.NewClient(redisOptions)
	defer func() { _ = queueClient.Close() }()
	discoverer := ingestion.NewDiscoverer(dbgen.New(pool), &http.Client{Timeout: cfg.FetchTimeout}, queueClient, cfg.UserAgent)
	server := asynq.NewServer(redisOptions, asynq.Config{Concurrency: 4})
	mux := asynq.NewServeMux()
	mux.HandleFunc(ingestion.TaskProcessSource, ingestion.ProcessSourceHandler(pipeline))
	mux.HandleFunc(ingestion.TaskDiscoverSitemap, ingestion.DiscoverSitemapHandler(discoverer))
	scheduler := asynq.NewScheduler(redisOptions, nil)
	discoveryTask, err := ingestion.NewDiscoverSitemapTask("https://learningenglish.voanews.com/sitemap_428_latest.xml.gz")
	if err != nil {
		logger.Error("build discovery task", "error", err)
		os.Exit(1)
	}
	if _, err := scheduler.Register("@every 30m", discoveryTask, asynq.Unique(25*time.Minute)); err != nil {
		logger.Error("schedule discovery", "error", err)
		os.Exit(1)
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			logger.Error("scheduler failed", "error", err)
			stop()
		}
	}()
	if _, err := queueClient.Enqueue(discoveryTask, asynq.Unique(25*time.Minute)); err != nil && !strings.Contains(err.Error(), "conflicts with another task") {
		logger.Error("enqueue initial discovery", "error", err)
	}
	go func() {
		logger.Info("worker ready", "env", cfg.Environment)
		if err := server.Run(mux); err != nil {
			logger.Error("worker failed", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	scheduler.Shutdown()
	server.Shutdown()
	logger.Info("worker stopped")
}
