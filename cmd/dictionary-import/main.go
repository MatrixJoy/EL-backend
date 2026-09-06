package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/english-learning/backend/internal/dictionary"
)

func main() {
	archivePath := flag.String("archive", "", "path to the official wn3.1.dict.tar.gz archive")
	databaseURL := flag.String("database", firstNonEmpty(os.Getenv("LEARNING_DATABASE_URL"), os.Getenv("VOA_DATABASE_URL")), "PostgreSQL connection URL")
	flag.Parse()
	if *archivePath == "" || *databaseURL == "" {
		log.Fatal("-archive and -database are required")
	}
	archive, err := os.Open(*archivePath)
	if err != nil {
		log.Fatal(err)
	}
	defer archive.Close()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	result, err := dictionary.ImportWordNet31(ctx, pool, archive)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("imported WordNet 3.1: entries=%d forms=%d\n", result.Entries, result.Forms)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
