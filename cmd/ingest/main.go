package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/voa-learning-app/backend/internal/config"
	"github.com/oldj/voa-learning-app/backend/internal/ingestion"
	"github.com/oldj/voa-learning-app/backend/internal/repository/dbgen"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/ingest <VOA article URL>")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal(err)
	}
	defer pool.Close()
	item, err := dbgen.New(pool).UpsertSourceItem(ctx, dbgen.UpsertSourceItemParams{Source: "voa_learning_english", CanonicalUrl: os.Args[1], PageType: pgtype.Text{String: "article", Valid: true}})
	if err != nil {
		fatal(err)
	}
	task, err := ingestion.NewProcessSourceTask(item.ID)
	if err != nil {
		fatal(err)
	}
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.RedisAddr})
	defer func() { _ = client.Close() }()
	info, err := client.Enqueue(task, asynq.MaxRetry(5))
	if err != nil {
		fatal(err)
	}
	fmt.Printf("enqueued task=%s source=%s\n", info.ID, item.ID)
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
