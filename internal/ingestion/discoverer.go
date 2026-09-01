package ingestion

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/voa-learning-app/backend/internal/ingestion/voa"
	"github.com/oldj/voa-learning-app/backend/internal/repository/dbgen"
)

const maxSitemapBytes int64 = 20 << 20

type TaskEnqueuer interface {
	Enqueue(*asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error)
}

type Discoverer struct {
	queries   *dbgen.Queries
	client    *http.Client
	enqueuer  TaskEnqueuer
	userAgent string
}

func NewDiscoverer(queries *dbgen.Queries, client *http.Client, enqueuer TaskEnqueuer, userAgent string) *Discoverer {
	return &Discoverer{queries: queries, client: client, enqueuer: enqueuer, userAgent: userAgent}
}

func (d *Discoverer) Discover(ctx context.Context, sitemapURL string) (int, error) {
	if !strings.HasPrefix(sitemapURL, "https://learningenglish.voanews.com/") {
		return 0, fmt.Errorf("sitemap URL is not allowed")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("User-Agent", d.userAgent)
	response, err := d.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("fetch sitemap: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("sitemap status %d", response.StatusCode)
	}
	reader := io.Reader(io.LimitReader(response.Body, maxSitemapBytes+1))
	if strings.HasSuffix(sitemapURL, ".gz") || response.Header.Get("Content-Encoding") == "gzip" || response.Header.Get("Content-Type") == "application/x-gzip" {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return 0, fmt.Errorf("open sitemap gzip: %w", err)
		}
		defer func() { _ = gz.Close() }()
		reader = io.LimitReader(gz, maxSitemapBytes+1)
	}
	entries, err := voa.ParseSitemap(reader)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, entry := range entries {
		item, err := d.queries.UpsertSourceItem(ctx, dbgen.UpsertSourceItemParams{Source: "voa_learning_english", CanonicalUrl: entry.URL, PageType: pgtype.Text{String: "article", Valid: true}})
		if err != nil {
			return queued, fmt.Errorf("upsert discovered item: %w", err)
		}
		task, err := NewProcessSourceTask(item.ID)
		if err != nil {
			return queued, err
		}
		if _, err := d.enqueuer.Enqueue(task, asynq.Unique(25*time.Minute), asynq.MaxRetry(5), asynq.Queue("default")); err != nil && !strings.Contains(err.Error(), "conflicts with another task") {
			return queued, fmt.Errorf("enqueue source: %w", err)
		}
		queued++
	}
	return queued, nil
}
