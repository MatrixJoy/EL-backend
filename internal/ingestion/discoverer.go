package ingestion

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/ingestion/voa"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
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
	document, err := voa.ParseSitemapDocument(reader)
	if err != nil {
		return 0, err
	}
	for _, child := range document.Children {
		task, taskErr := NewDiscoverSitemapTask(child)
		if taskErr != nil {
			return 0, taskErr
		}
		if _, taskErr = d.enqueuer.Enqueue(task, asynq.Unique(24*time.Hour), asynq.MaxRetry(5), asynq.Queue("discovery")); taskErr != nil && !errors.Is(taskErr, asynq.ErrDuplicateTask) && !errors.Is(taskErr, asynq.ErrTaskIDConflict) {
			return 0, fmt.Errorf("enqueue child sitemap: %w", taskErr)
		}
	}
	queued := 0
	for _, entry := range document.Entries {
		item, err := d.queries.UpsertSourceItem(ctx, dbgen.UpsertSourceItemParams{Source: "voa_learning_english", CanonicalUrl: entry.URL, PageType: pgtype.Text{String: "article", Valid: true}})
		if err != nil {
			return queued, fmt.Errorf("upsert discovered item: %w", err)
		}
		task, err := NewProcessSourceTask(item.ID)
		if err != nil {
			return queued, err
		}
		if item.FetchState != dbgen.SourceFetchStateDiscovered {
			continue
		}
		if _, err := d.enqueuer.Enqueue(task, asynq.Unique(30*24*time.Hour), asynq.MaxRetry(3), asynq.Queue("crawl")); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) && !errors.Is(err, asynq.ErrTaskIDConflict) {
			return queued, fmt.Errorf("enqueue source: %w", err)
		}
		queued++
	}
	return queued, nil
}
