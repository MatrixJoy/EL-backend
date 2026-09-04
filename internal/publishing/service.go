package publishing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/english-learning/backend/internal/platform/objectstore"
)

const maxAudioBytes = int64(250 << 20)

var (
	ErrInvalidDocument = errors.New("invalid publication document")
	ErrAudioIntegrity  = errors.New("audio integrity check failed")
	hexSHA256          = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Paragraph struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

type Sentence struct {
	Text       string  `json:"text"`
	StartMS    *int    `json:"start_ms"`
	EndMS      *int    `json:"end_ms"`
	Confidence float64 `json:"confidence"`
	Spoken     bool    `json:"spoken"`
}

type Timeline struct {
	Version             string     `json:"version"`
	Coverage            float64    `json:"coverage"`
	SpokenSentenceCount int        `json:"spoken_sentence_count"`
	DurationMS          int        `json:"duration_ms"`
	Sentences           []Sentence `json:"sentences"`
}

type Audio struct {
	SHA256   string `json:"sha256"`
	ByteSize int64  `json:"byte_size"`
	MIMEType string `json:"mime_type"`
}

type Document struct {
	SchemaVersion   int         `json:"schema_version"`
	SourceContentID string      `json:"source_content_id"`
	Source          string      `json:"source"`
	SourceURL       string      `json:"source_url"`
	Attribution     string      `json:"attribution"`
	ManifestETag    string      `json:"manifest_etag"`
	Title           string      `json:"title"`
	Description     string      `json:"description"`
	Series          string      `json:"series"`
	Level           string      `json:"level"`
	Topics          []string    `json:"topics"`
	LearningGoals   []string    `json:"learning_goals"`
	QualityScore    int         `json:"quality_score"`
	WordCount       int         `json:"word_count"`
	PublishedAt     string      `json:"published_at"`
	Paragraphs      []Paragraph `json:"paragraphs"`
	Timeline        Timeline    `json:"timeline"`
	Audio           Audio       `json:"audio"`
}

type Result struct {
	ContentID   uuid.UUID `json:"contentId"`
	Revision    int32     `json:"revision"`
	Environment string    `json:"environment"`
	Duplicate   bool      `json:"duplicate"`
}

type Service struct {
	db          *pgxpool.Pool
	objects     *objectstore.Store
	environment string
}

func NewService(db *pgxpool.Pool, objects *objectstore.Store, environment string) *Service {
	return &Service{db: db, objects: objects, environment: environment}
}

func (s *Service) Publish(ctx context.Context, idempotencyKey string, document Document, audio io.Reader) (Result, error) {
	if err := validate(idempotencyKey, document); err != nil {
		return Result{}, err
	}
	if existing, ok, err := s.existing(ctx, idempotencyKey); err != nil {
		return Result{}, err
	} else if ok {
		_, _ = io.Copy(io.Discard, audio)
		existing.Duplicate = true
		return existing, nil
	}

	objectKey := fmt.Sprintf("published/audio/%s/%s/%s.mp3", safeSegment(document.Source), safeSegment(document.SourceContentID), document.Audio.SHA256)
	digest := sha256.New()
	if err := s.objects.Put(ctx, objectKey, io.TeeReader(audio, digest), document.Audio.ByteSize, document.Audio.MIMEType); err != nil {
		return Result{}, err
	}
	if !sameDigest(digest, document.Audio.SHA256) {
		return Result{}, ErrAudioIntegrity
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	documentJSON, err := json.Marshal(document)
	if err != nil {
		return Result{}, err
	}
	var reserved string
	err = tx.QueryRow(ctx, `INSERT INTO cms_publications(idempotency_key,source_content_id,source,manifest_etag,document) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING idempotency_key`, idempotencyKey, document.SourceContentID, document.Source, document.ManifestETag, documentJSON).Scan(&reserved)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		existing, ok, lookupErr := s.existing(ctx, idempotencyKey)
		if lookupErr != nil {
			return Result{}, lookupErr
		}
		if !ok {
			return Result{}, fmt.Errorf("publication reservation lost")
		}
		existing.Duplicate = true
		return existing, nil
	}
	if err != nil {
		return Result{}, err
	}

	sourceID, err := upsertSource(ctx, tx, document)
	if err != nil {
		return Result{}, err
	}
	body, transcript, metadata, err := encodeContent(document)
	if err != nil {
		return Result{}, err
	}
	publishedAt := parseTime(document.PublishedAt)
	var contentID uuid.UUID
	var revision int32
	attribution := sourceAttribution(document)
	err = tx.QueryRow(ctx, `INSERT INTO contents(source_item_id,slug,type,title,summary,level,published_at,duration_seconds,body_blocks,transcript_blocks,metadata,rights_status,attribution,status,source_updated_at) VALUES($1,$2,'article',$3,$4,$5,$6,$7,$8,$9,$10,'review_required',$11,'published',$6) ON CONFLICT(source_item_id) DO UPDATE SET title=excluded.title,summary=excluded.summary,level=excluded.level,published_at=excluded.published_at,duration_seconds=excluded.duration_seconds,body_blocks=excluded.body_blocks,transcript_blocks=excluded.transcript_blocks,metadata=excluded.metadata,attribution=excluded.attribution,status='published',revision=contents.revision+1,source_updated_at=excluded.source_updated_at,updated_at=now() RETURNING id,revision`, sourceID, contentSlug(document), document.Title, document.Description, nullableString(document.Level), publishedAt, (document.Timeline.DurationMS+999)/1000, body, transcript, metadata, attribution).Scan(&contentID, &revision)
	if err != nil {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM content_categories WHERE content_id=$1`, contentID); err != nil {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM content_series WHERE content_id=$1`, contentID); err != nil {
		return Result{}, err
	}
	if err = linkTaxonomy(ctx, tx, contentID, document); err != nil {
		return Result{}, err
	}
	var assetID uuid.UUID
	sourceURL := fmt.Sprintf("cms://%s/%s/audio", safeSegment(document.Source), safeSegment(document.SourceContentID))
	err = tx.QueryRow(ctx, `INSERT INTO assets(kind,source_url,source_host,mime_type,byte_size,duration_seconds,quality_label,checksum,delivery_policy,rights_status,attribution,object_key,availability) VALUES('audio',$1,'cms',$2,$3,$4,'original',$5,'managed_cache','review_required',$6,$7,'available') ON CONFLICT(source_url) DO UPDATE SET mime_type=excluded.mime_type,byte_size=excluded.byte_size,duration_seconds=excluded.duration_seconds,checksum=excluded.checksum,delivery_policy='managed_cache',attribution=excluded.attribution,object_key=excluded.object_key,availability='available',updated_at=now() RETURNING id`, sourceURL, document.Audio.MIMEType, document.Audio.ByteSize, (document.Timeline.DurationMS+999)/1000, document.Audio.SHA256, attribution, objectKey).Scan(&assetID)
	if err != nil {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM content_assets USING assets WHERE content_assets.content_id=$1 AND content_assets.asset_id=assets.id AND assets.kind='audio' AND assets.id<>$2`, contentID, assetID); err != nil {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO content_assets(content_id,asset_id,role,position) VALUES($1,$2,'audio',0) ON CONFLICT(content_id,asset_id,role) DO UPDATE SET position=0`, contentID, assetID); err != nil {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO publish_events(entity_type,entity_id,operation,revision) VALUES('content',$1,'upsert',$2)`, contentID, revision); err != nil {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE cms_publications SET content_id=$2,content_revision=$3 WHERE idempotency_key=$1`, idempotencyKey, contentID, revision); err != nil {
		return Result{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{ContentID: contentID, Revision: revision, Environment: s.environment}, nil
}

func (s *Service) existing(ctx context.Context, key string) (Result, bool, error) {
	var result Result
	err := s.db.QueryRow(ctx, `SELECT content_id,content_revision FROM cms_publications WHERE idempotency_key=$1 AND content_id IS NOT NULL`, key).Scan(&result.ContentID, &result.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, err
	}
	result.Environment = s.environment
	return result, true, nil
}

func validate(key string, d Document) error {
	if len(key) < 32 || len(key) > 128 || d.SchemaVersion != 1 || strings.TrimSpace(d.SourceContentID) == "" || strings.TrimSpace(d.Source) == "" || strings.TrimSpace(d.SourceURL) == "" || strings.TrimSpace(d.ManifestETag) == "" || strings.TrimSpace(d.Title) == "" || len(d.Paragraphs) == 0 || len(d.Timeline.Sentences) == 0 {
		return ErrInvalidDocument
	}
	if d.Audio.ByteSize <= 0 || d.Audio.ByteSize > maxAudioBytes || d.Audio.MIMEType != "audio/mpeg" || !hexSHA256.MatchString(d.Audio.SHA256) {
		return ErrInvalidDocument
	}
	return nil
}

func upsertSource(ctx context.Context, tx pgx.Tx, d Document) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO source_items(source,canonical_url,external_id,page_type,fetch_state) VALUES($1,$2,$3,'audio_article','fetched') ON CONFLICT(source,canonical_url) DO UPDATE SET external_id=excluded.external_id,page_type=excluded.page_type,fetch_state='fetched',last_seen_at=now() RETURNING id`, d.Source, d.SourceURL, d.SourceContentID).Scan(&id)
	return id, err
}

func encodeContent(d Document) ([]byte, []byte, []byte, error) {
	body := make([]map[string]any, 0, len(d.Paragraphs))
	for _, paragraph := range d.Paragraphs {
		body = append(body, map[string]any{"type": "paragraph", "text": paragraph.Text, "index": paragraph.Index})
	}
	metadata := map[string]any{"sourceContentId": d.SourceContentID, "manifestEtag": d.ManifestETag, "series": d.Series, "topics": d.Topics, "learningGoals": d.LearningGoals, "qualityScore": d.QualityScore, "wordCount": d.WordCount, "timelineVersion": d.Timeline.Version, "timelineCoverage": d.Timeline.Coverage}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, nil, nil, err
	}
	transcriptJSON, err := json.Marshal(d.Timeline.Sentences)
	if err != nil {
		return nil, nil, nil, err
	}
	metadataJSON, err := json.Marshal(metadata)
	return bodyJSON, transcriptJSON, metadataJSON, err
}

func linkTaxonomy(ctx context.Context, tx pgx.Tx, contentID uuid.UUID, d Document) error {
	if d.Level != "" && d.Level != "unclassified" {
		if err := linkCategory(ctx, tx, contentID, d.Level, title(d.Level), "level", true); err != nil {
			return err
		}
	}
	for _, topic := range d.Topics {
		if err := linkCategory(ctx, tx, contentID, topic, title(topic), "topic", false); err != nil {
			return err
		}
	}
	if strings.TrimSpace(d.Series) != "" {
		var seriesID uuid.UUID
		err := tx.QueryRow(ctx, `INSERT INTO series(slug,title,level) VALUES($1,$2,$3) ON CONFLICT(slug) DO UPDATE SET title=excluded.title,level=excluded.level,updated_at=now() RETURNING id`, "series-"+slugify(d.Series), d.Series, nullableString(d.Level)).Scan(&seriesID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO content_series(content_id,series_id,position) VALUES($1,$2,0) ON CONFLICT(content_id) DO UPDATE SET series_id=excluded.series_id`, contentID, seriesID)
		return err
	}
	return nil
}

func linkCategory(ctx context.Context, tx pgx.Tx, contentID uuid.UUID, slug, name, kind string, primary bool) error {
	slug = slugify(slug)
	if slug == "" {
		return nil
	}
	var categoryID uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO categories(slug,name,kind) VALUES($1,$2,$3) ON CONFLICT(slug) DO UPDATE SET name=excluded.name RETURNING id`, slug, name, kind).Scan(&categoryID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO content_categories(content_id,category_id,is_primary) VALUES($1,$2,$3) ON CONFLICT(content_id,category_id) DO UPDATE SET is_primary=excluded.is_primary`, contentID, categoryID, primary)
	return err
}

func contentSlug(d Document) string { return slugify(d.Source + "-" + d.SourceContentID) }

func sourceAttribution(d Document) string {
	if value := strings.TrimSpace(d.Attribution); value != "" {
		return value
	}
	if d.Source == "voa-learning-english" {
		return "VOA Learning English"
	}
	return d.Source
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	dash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func safeSegment(value string) string {
	if result := slugify(value); result != "" {
		return result
	}
	return "content"
}

func title(value string) string {
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, "_", " ")
	words := strings.Fields(value)
	for index, word := range words {
		runes := []rune(word)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			words[index] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func parseTime(value string) any {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return parsed
}

func sameDigest(digest hash.Hash, expected string) bool {
	return hex.EncodeToString(digest.Sum(nil)) == expected
}
