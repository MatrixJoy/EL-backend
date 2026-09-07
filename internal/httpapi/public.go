package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/platform/objectstore"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

var requestCount atomic.Uint64

func mountPublicRoutes(r chi.Router, deps Dependencies) {
	h := publicHandlers{queries: deps.Queries, mediaClient: deps.MediaClient, mediaStore: deps.MediaStore}
	r.Get("/home", h.home)
	r.Get("/categories", h.categories)
	r.Get("/categories/{slug}/contents", h.categoryContents)
	r.Get("/contents", h.contents)
	r.Get("/contents/{id}", h.content)
	r.Get("/search", h.search)
	r.Get("/dictionary/{word}", h.dictionary)
	r.Get("/sync", h.sync)
	r.Get("/series", h.series)
	r.Get("/series/{id}", h.seriesDetail)
	r.MethodFunc(http.MethodGet, "/media/{id}", h.media)
	r.MethodFunc(http.MethodHead, "/media/{id}", h.media)
}

type publicHandlers struct {
	queries     *dbgen.Queries
	mediaClient *http.Client
	mediaStore  *objectstore.Store
}

func (h publicHandlers) home(w http.ResponseWriter, r *http.Request) {
	requestCount.Add(1)
	limit := int32(20)
	rows, err := h.queries.ListPublishedContents(r.Context(), dbgen.ListPublishedContentsParams{PageLimit: limit})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", r)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID, "type": row.Type, "title": row.Title, "summary": textValue(row.Summary), "level": textValue(row.Level), "publishedAt": nullableTime(row.PublishedAt), "durationSeconds": intValue(row.DurationSeconds), "learningGoals": contentLearningGoals(row.Metadata), "revision": row.Revision})
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"sections": []any{map[string]any{"id": "latest", "title": "Latest", "items": items}}}})
}

func (h publicHandlers) categories(w http.ResponseWriter, r *http.Request) {
	rows, err := h.queries.ListCategories(r.Context())
	if err != nil {
		writeError(w, 500, "INTERNAL_ERROR", r)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID, "slug": row.Slug, "name": row.Name, "kind": row.Kind, "sortOrder": row.SortOrder})
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h publicHandlers) categoryContents(w http.ResponseWriter, r *http.Request) {
	rows, err := h.queries.ListCategoryContents(r.Context(), dbgen.ListCategoryContentsParams{Slug: chi.URLParam(r, "slug"), Limit: 50})
	if err != nil {
		writeError(w, 500, "INTERNAL_ERROR", r)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID, "type": row.Type, "title": row.Title, "summary": textValue(row.Summary), "level": textValue(row.Level), "publishedAt": nullableTime(row.PublishedAt), "durationSeconds": intValue(row.DurationSeconds), "learningGoals": contentLearningGoals(row.Metadata), "revision": row.Revision})
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h publicHandlers) contents(w http.ResponseWriter, r *http.Request) {
	params := dbgen.ListPublishedContentsParams{PageLimit: 20}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			writeError(w, 400, "VALIDATION_ERROR", r)
			return
		}
		parts := strings.Split(string(decoded), "|")
		if len(parts) != 2 {
			writeError(w, 400, "VALIDATION_ERROR", r)
			return
		}
		when, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			writeError(w, 400, "VALIDATION_ERROR", r)
			return
		}
		id, err := uuid.Parse(parts[1])
		if err != nil {
			writeError(w, 400, "VALIDATION_ERROR", r)
			return
		}
		params.BeforePublishedAt = pgtype.Timestamptz{Time: when, Valid: true}
		params.BeforeID = pgtype.UUID{Bytes: id, Valid: true}
	}
	rows, err := h.queries.ListPublishedContents(r.Context(), params)
	if err != nil {
		writeError(w, 500, "INTERNAL_ERROR", r)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	next := ""
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID, "type": row.Type, "title": row.Title, "summary": textValue(row.Summary), "level": textValue(row.Level), "publishedAt": nullableTime(row.PublishedAt), "durationSeconds": intValue(row.DurationSeconds), "learningGoals": contentLearningGoals(row.Metadata), "revision": row.Revision})
	}
	if len(rows) == int(params.PageLimit) {
		last := rows[len(rows)-1]
		if last.PublishedAt.Valid {
			next = base64.RawURLEncoding.EncodeToString([]byte(last.PublishedAt.Time.UTC().Format(time.RFC3339Nano) + "|" + last.ID.String()))
		}
	}
	writeJSON(w, 200, map[string]any{"data": items, "nextCursor": next})
}

func (h publicHandlers) content(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "VALIDATION_ERROR", r)
		return
	}
	row, err := h.queries.GetPublishedContent(r.Context(), id)
	if err != nil {
		writeError(w, 404, "CONTENT_NOT_FOUND", r)
		return
	}
	assets, _ := h.queries.ListContentAssets(r.Context(), id)
	assetDTO := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		assetDTO = append(assetDTO, map[string]any{"id": asset.ID, "kind": asset.Kind, "quality": textValue(asset.QualityLabel), "playbackUrl": "/api/v1/media/" + asset.ID.String()})
	}
	w.Header().Set("ETag", fmt.Sprintf(`W/"content-%s-%d"`, row.ID, row.Revision))
	if r.Header.Get("If-None-Match") == fmt.Sprintf(`W/"content-%s-%d"`, row.ID, row.Revision) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	bodyBlocks := json.RawMessage(row.BodyBlocks)
	if len(row.BodyBlocks) == 0 || string(row.BodyBlocks) == "null" {
		bodyBlocks = json.RawMessage("[]")
	}
	transcriptBlocks := json.RawMessage(row.TranscriptBlocks)
	if len(row.TranscriptBlocks) == 0 || string(row.TranscriptBlocks) == "null" {
		transcriptBlocks = json.RawMessage("[]")
	}
	metadata := json.RawMessage(row.Metadata)
	if len(row.Metadata) == 0 || string(row.Metadata) == "null" {
		metadata = json.RawMessage("{}")
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": row.ID, "type": row.Type, "title": row.Title, "summary": textValue(row.Summary), "level": textValue(row.Level), "publishedAt": nullableTime(row.PublishedAt), "durationSeconds": intValue(row.DurationSeconds), "bodyBlocks": bodyBlocks, "transcriptBlocks": transcriptBlocks, "featuredWords": contentFeaturedWords(metadata, bodyBlocks), "grammarPoints": contentGrammarPoints(metadata), "metadata": metadata, "assets": assetDTO, "source": map[string]any{"name": row.Attribution, "canonicalUrl": row.CanonicalUrl}, "revision": row.Revision}})
}

type featuredWordDTO struct {
	Word         string `json:"word"`
	PartOfSpeech string `json:"partOfSpeech,omitempty"`
	Definition   string `json:"definition"`
}

type grammarPointDTO struct {
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	Example     string   `json:"example"`
	Prompt      string   `json:"prompt"`
	Answer      string   `json:"answer"`
	Options     []string `json:"options"`
}

func contentGrammarPoints(metadata json.RawMessage) []grammarPointDTO {
	var stored struct {
		GrammarPoints []grammarPointDTO `json:"grammarPoints"`
	}
	if json.Unmarshal(metadata, &stored) != nil || stored.GrammarPoints == nil {
		return []grammarPointDTO{}
	}
	return stored.GrammarPoints
}

func contentLearningGoals(metadata json.RawMessage) []string {
	var stored struct {
		LearningGoals []string          `json:"learningGoals"`
		GrammarPoints []grammarPointDTO `json:"grammarPoints"`
	}
	if json.Unmarshal(metadata, &stored) != nil {
		return []string{}
	}
	goals := make([]string, 0, len(stored.LearningGoals)+1)
	seen := make(map[string]bool, len(stored.LearningGoals)+1)
	for _, raw := range stored.LearningGoals {
		goal := strings.ToLower(strings.TrimSpace(raw))
		if goal == "" || seen[goal] {
			continue
		}
		seen[goal] = true
		goals = append(goals, goal)
	}
	if len(stored.GrammarPoints) > 0 && !seen["grammar"] {
		goals = append(goals, "grammar")
	}
	return goals
}

var legacyFeaturedWordPattern = regexp.MustCompile(`(?i)^([[:alpha:]][[:alpha:]'’ -]{0,80}?)\s+[–—-]\s*(phr(?:asal)?\s+v|n|v|adj|adv|prep|pron)\.\s+(.+)$`)

func contentFeaturedWords(metadata, bodyBlocks json.RawMessage) []featuredWordDTO {
	var stored struct {
		FeaturedWords []struct {
			Word         string `json:"word"`
			PartOfSpeech string `json:"part_of_speech"`
			Definition   string `json:"definition"`
		} `json:"featuredWords"`
	}
	if json.Unmarshal(metadata, &stored) == nil && len(stored.FeaturedWords) > 0 {
		words := make([]featuredWordDTO, 0, len(stored.FeaturedWords))
		for _, word := range stored.FeaturedWords {
			words = append(words, featuredWordDTO{Word: word.Word, PartOfSpeech: word.PartOfSpeech, Definition: word.Definition})
		}
		return words
	}

	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(bodyBlocks, &blocks) != nil {
		return []featuredWordDTO{}
	}
	words := make([]featuredWordDTO, 0)
	inGlossary := false
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if len(text) >= 10 && strings.Trim(text, "_") == "" {
			if inGlossary && len(words) > 0 {
				break
			}
			inGlossary = true
			continue
		}
		if !inGlossary {
			continue
		}
		match := legacyFeaturedWordPattern.FindStringSubmatch(text)
		if len(match) != 4 {
			if len(words) > 0 {
				break
			}
			continue
		}
		words = append(words, featuredWordDTO{Word: strings.TrimSpace(match[1]), PartOfSpeech: normalizeFeaturedPartOfSpeech(match[2]), Definition: strings.TrimSpace(match[3])})
	}
	return words
}

func normalizeFeaturedPartOfSpeech(value string) string {
	switch strings.ToLower(strings.Join(strings.Fields(value), " ")) {
	case "n":
		return "noun"
	case "v":
		return "verb"
	case "adj":
		return "adjective"
	case "adv":
		return "adverb"
	case "prep":
		return "preposition"
	case "pron":
		return "pronoun"
	case "phr v", "phrasal v":
		return "phrasal verb"
	default:
		return value
	}
}

func (h publicHandlers) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 || len(query) > 100 {
		writeError(w, 400, "VALIDATION_ERROR", r)
		return
	}
	rows, err := h.queries.SearchPublishedContents(r.Context(), dbgen.SearchPublishedContentsParams{SearchQuery: query, PageLimit: 50})
	if err != nil {
		writeError(w, 500, "INTERNAL_ERROR", r)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID, "type": row.Type, "title": row.Title, "summary": textValue(row.Summary), "level": textValue(row.Level), "publishedAt": nullableTime(row.PublishedAt), "durationSeconds": intValue(row.DurationSeconds), "learningGoals": contentLearningGoals(row.Metadata), "revision": row.Revision})
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h publicHandlers) sync(w http.ResponseWriter, r *http.Request) {
	sequence := int64(0)
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			writeError(w, 400, "VALIDATION_ERROR", r)
			return
		}
		sequence, err = strconv.ParseInt(string(decoded), 10, 64)
		if err != nil {
			writeError(w, 400, "VALIDATION_ERROR", r)
			return
		}
	}
	rows, err := h.queries.ListPublishEventsAfter(r.Context(), dbgen.ListPublishEventsAfterParams{Sequence: sequence, Limit: 100})
	if err != nil {
		writeError(w, 500, "INTERNAL_ERROR", r)
		return
	}
	next := sequence
	changes := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		next = row.Sequence
		changes = append(changes, map[string]any{"sequence": row.Sequence, "operation": row.Operation, "entity": row.EntityType, "id": row.EntityID, "revision": intValue(row.Revision)})
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"changes": changes, "nextCursor": base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(next, 10))), "hasMore": len(rows) == 100}})
}

func (h publicHandlers) series(w http.ResponseWriter, r *http.Request) {
	rows, err := h.queries.ListSeries(r.Context(), 100)
	if err != nil {
		writeError(w, 500, "INTERNAL_ERROR", r)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID, "slug": row.Slug, "title": row.Title, "description": textValue(row.Description), "level": textValue(row.Level)})
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h publicHandlers) seriesDetail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "VALIDATION_ERROR", r)
		return
	}
	series, err := h.queries.GetSeries(r.Context(), id)
	if err != nil {
		writeError(w, 404, "CONTENT_NOT_FOUND", r)
		return
	}
	items, err := h.queries.ListSeriesContents(r.Context(), id)
	if err != nil {
		writeError(w, 500, "INTERNAL_ERROR", r)
		return
	}
	contentItems := make([]map[string]any, 0, len(items))
	for _, row := range items {
		contentItems = append(contentItems, map[string]any{"id": row.ID, "type": row.Type, "title": row.Title, "summary": textValue(row.Summary), "level": textValue(row.Level), "publishedAt": nullableTime(row.PublishedAt), "durationSeconds": intValue(row.DurationSeconds), "learningGoals": contentLearningGoals(row.Metadata), "revision": row.Revision, "position": row.Position})
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"series": map[string]any{"id": series.ID, "slug": series.Slug, "title": series.Title, "description": textValue(series.Description), "level": textValue(series.Level)}, "items": contentItems}})
}

func (h publicHandlers) media(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "VALIDATION_ERROR", r)
		return
	}
	asset, err := h.queries.GetDeliverableAsset(r.Context(), id)
	if err != nil {
		writeError(w, 404, "MEDIA_UNAVAILABLE", r)
		return
	}
	if asset.DeliveryPolicy == dbgen.DeliveryPolicyManagedCache && asset.ObjectKey.Valid && h.mediaStore != nil {
		object, info, openErr := h.mediaStore.Open(r.Context(), asset.ObjectKey.String)
		if openErr != nil {
			writeError(w, 502, "MEDIA_UNAVAILABLE", r)
			return
		}
		defer object.Close()
		contentType := "audio/mpeg"
		if asset.MimeType.Valid {
			contentType = asset.MimeType.String
		} else if info.ContentType != "" {
			contentType = info.ContentType
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeContent(w, r, asset.ID.String()+".mp3", info.LastModified, object)
		return
	}
	if err := proxyMedia(w, r, asset.SourceUrl, h.mediaClient); err != nil {
		writeError(w, 502, "UPSTREAM_TEMPORARILY_UNAVAILABLE", r)
	}
}

func proxyMedia(w http.ResponseWriter, r *http.Request, sourceURL string, client *http.Client) error {
	target, err := url.Parse(sourceURL)
	if err != nil || target.Scheme != "https" || !allowedMediaHost(target.Hostname()) {
		return fmt.Errorf("media source is not allowed")
	}
	request, _ := http.NewRequestWithContext(r.Context(), r.Method, target.String(), nil)
	if value := r.Header.Get("Range"); value != "" {
		request.Header.Set("Range", value)
	}
	if client == nil {
		client = NewMediaClient(2 * time.Minute)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, io.LimitReader(response.Body, 2<<30))
	}
	return nil
}

func allowedMediaHost(host string) bool {
	host = strings.ToLower(host)
	return host == "learningenglish.voanews.com" || host == "gdb.voanews.com" || host == "voa-audio.voanews.eu" || strings.HasSuffix(host, ".akamaized.net")
}

func NewMediaClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 || request.URL.Scheme != "https" || !allowedMediaHost(request.URL.Hostname()) {
			return fmt.Errorf("media redirect is not allowed")
		}
		return nil
	}}
}
func nullableTime(value pgtype.Timestamptz) any {
	if !value.Valid {
		return nil
	}
	return value.Time.UTC().Format(time.RFC3339)
}
func textValue(value pgtype.Text) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
func intValue(value pgtype.Int4) any {
	if !value.Valid {
		return nil
	}
	return value.Int32
}
func writeError(w http.ResponseWriter, status int, code string, r *http.Request) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": http.StatusText(status), "requestId": r.Header.Get("X-Request-Id")}})
}
func metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "voa_api_requests_total %d\n", requestCount.Load())
}
