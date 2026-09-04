package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/english-learning/backend/internal/config"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

func main() {
	if len(os.Args) < 2 {
		usage()
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
	q := dbgen.New(pool)
	switch os.Args[1] {
	case "list-review":
		items, err := q.ListContentsByStatus(ctx, dbgen.ListContentsByStatusParams{Status: dbgen.ContentStatusReview, Limit: 100})
		if err != nil {
			fatal(err)
		}
		for _, item := range items {
			fmt.Printf("%s\t%s\t%s\t%s\n", item.ID, item.Status, item.RightsStatus, item.Title)
		}
	case "publish", "hide":
		if len(os.Args) != 3 {
			usage()
		}
		id, err := uuid.Parse(os.Args[2])
		if err != nil {
			fatal(err)
		}
		status := dbgen.ContentStatusPublished
		if os.Args[1] == "hide" {
			status = dbgen.ContentStatusHidden
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			fatal(err)
		}
		qt := q.WithTx(tx)
		item, err := qt.SetContentStatus(ctx, dbgen.SetContentStatusParams{ID: id, Status: status})
		if err != nil {
			_ = tx.Rollback(ctx)
			fatal(err)
		}
		op := "upsert"
		if status == dbgen.ContentStatusHidden {
			op = "delete"
		}
		if _, err := qt.CreatePublishEvent(ctx, dbgen.CreatePublishEventParams{EntityType: "content", EntityID: id, Operation: op, Revision: pgtype.Int4{Int32: item.Revision, Valid: true}}); err != nil {
			_ = tx.Rollback(ctx)
			fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			fatal(err)
		}
		fmt.Printf("%s status=%s revision=%d\n", item.ID, item.Status, item.Revision)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: admin list-review | publish <content-id> | hide <content-id>")
	os.Exit(2)
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
