package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/voa-learning-app/backend/internal/catalog"
	"github.com/oldj/voa-learning-app/backend/internal/ingestion/voa"
	"github.com/oldj/voa-learning-app/backend/internal/repository/dbgen"
)

const parserVersion = "voa-article-v1"

type HTMLStore interface {
	PutHTML(context.Context, string, []byte) error
}

type Pipeline struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
	store   HTMLStore
	fetcher *voa.Fetcher
	parser  voa.ArticleParser
}

func NewPipeline(pool *pgxpool.Pool, store HTMLStore, fetcher *voa.Fetcher) *Pipeline {
	return &Pipeline{pool: pool, queries: dbgen.New(pool), store: store, fetcher: fetcher}
}

func (p *Pipeline) Process(ctx context.Context, sourceID uuid.UUID) error {
	item, err := p.queries.GetSourceItem(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("get source item: %w", err)
	}
	result, err := p.fetcher.Fetch(ctx, voa.FetchRequest{URL: item.CanonicalUrl})
	if err != nil {
		_ = p.queries.SetSourceFetchState(ctx, dbgen.SetSourceFetchStateParams{ID: sourceID, FetchState: dbgen.SourceFetchStateFailed, NextFetchAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}})
		return err
	}
	requestedURL, _ := url.Parse(item.CanonicalUrl)
	finalURL, _ := url.Parse(result.FinalURL)
	if requestedURL != nil && finalURL != nil && strings.HasPrefix(requestedURL.Path, "/a/") && !strings.HasPrefix(finalURL.Path, "/a/") {
		_ = p.queries.SetSourceFetchState(ctx, dbgen.SetSourceFetchStateParams{ID: sourceID, FetchState: dbgen.SourceFetchStateFailed, NextFetchAt: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true}})
		return fmt.Errorf("article redirected to incompatible page type: %s", finalURL.Path)
	}
	key := fmt.Sprintf("voa/%s/%s.html", sourceID, result.SHA256)
	if err := p.store.PutHTML(ctx, key, result.Body); err != nil {
		return err
	}
	snapshot, err := p.queries.CreateSourceSnapshot(ctx, dbgen.CreateSourceSnapshotParams{
		SourceItemID: sourceID, HttpStatus: int32(result.StatusCode), Etag: text(result.ETag), LastModified: text(result.LastModified), ContentHash: result.SHA256, ObjectKey: key, ParseState: "pending",
	})
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}
	content, err := p.parser.Parse(item.CanonicalUrl, bytes.NewReader(result.Body))
	if err != nil {
		_ = p.queries.MarkSnapshotParsed(ctx, dbgen.MarkSnapshotParsedParams{ID: snapshot.ID, ParserVersion: text(parserVersion), ParseState: "failed", ErrorCode: text("PARSE_FAILED")})
		return err
	}
	if err := p.normalize(ctx, sourceID, content); err != nil {
		return err
	}
	if err := p.queries.MarkSnapshotParsed(ctx, dbgen.MarkSnapshotParsedParams{ID: snapshot.ID, ParserVersion: text(parserVersion), ParseState: "parsed"}); err != nil {
		return err
	}
	return p.queries.SetSourceFetchState(ctx, dbgen.SetSourceFetchStateParams{ID: sourceID, FetchState: dbgen.SourceFetchStateFetched, NextFetchAt: pgtype.Timestamptz{Time: time.Now().Add(7 * 24 * time.Hour), Valid: true}})
}

func (p *Pipeline) normalize(ctx context.Context, sourceID uuid.UUID, parsed catalog.Content) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := p.queries.WithTx(tx)
	body, _ := json.Marshal(parsed.Body)
	published := pgtype.Timestamptz{Time: parsed.PublishedAt, Valid: !parsed.PublishedAt.IsZero()}
	content, err := q.UpsertContent(ctx, dbgen.UpsertContentParams{SourceItemID: sourceID, Slug: "voa-" + parsed.Source.ExternalID, Type: dbgen.ContentType(parsed.Type), Title: parsed.Title, Level: text(string(parsed.Level)), PublishedAt: published, BodyBlocks: body, Column8: dbgen.RightsStatus(parsed.RightsStatus), Attribution: parsed.Attribution})
	if err != nil {
		return fmt.Errorf("upsert content: %w", err)
	}
	if parsed.SeriesTitle != "" {
		series, err := q.UpsertSeries(ctx, dbgen.UpsertSeriesParams{Slug: slugify(parsed.SeriesTitle), Title: parsed.SeriesTitle})
		if err != nil {
			return fmt.Errorf("upsert series: %w", err)
		}
		if err := q.LinkContentSeries(ctx, dbgen.LinkContentSeriesParams{ContentID: content.ID, SeriesID: series.ID}); err != nil {
			return fmt.Errorf("link series: %w", err)
		}
	}
	if parsed.Level != "" {
		if err := q.LinkContentCategoryBySlug(ctx, dbgen.LinkContentCategoryBySlugParams{ContentID: content.ID, Slug: string(parsed.Level)}); err != nil {
			return fmt.Errorf("link category: %w", err)
		}
	}
	for index, parsedAsset := range parsed.Assets {
		target, err := url.Parse(parsedAsset.SourceURL)
		if err != nil {
			continue
		}
		asset, err := q.UpsertAsset(ctx, dbgen.UpsertAssetParams{Kind: parsedAsset.Kind, SourceUrl: parsedAsset.SourceURL, SourceHost: target.Hostname(), MimeType: text(parsedAsset.MIMEType), ByteSize: pgtype.Int8{Int64: parsedAsset.ByteSize, Valid: parsedAsset.ByteSize > 0}, QualityLabel: text(parsedAsset.QualityLabel), Column7: dbgen.RightsStatus(parsed.RightsStatus), Attribution: parsed.Attribution})
		if err != nil {
			return fmt.Errorf("upsert asset: %w", err)
		}
		if err := q.LinkContentAsset(ctx, dbgen.LinkContentAssetParams{ContentID: content.ID, AssetID: asset.ID, Role: parsedAsset.Kind, Position: int32(index)}); err != nil {
			return err
		}
	}
	if content.Status == dbgen.ContentStatusPublished {
		if _, err := q.CreatePublishEvent(ctx, dbgen.CreatePublishEventParams{EntityType: "content", EntityID: content.ID, Operation: "upsert", Revision: pgtype.Int4{Int32: content.Revision, Valid: true}}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func text(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	value = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(value), "-"), "-")
	if value == "" {
		return "series-unknown"
	}
	return value
}
