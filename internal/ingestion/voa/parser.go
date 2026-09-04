package voa

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/oldj/english-learning/backend/internal/catalog"
)

var articleIDPattern = regexp.MustCompile(`/a/(?:[^/]+/)?(?:[^/]*-)?(\d+)\.html$`)

type ArticleParser struct{}

func (ArticleParser) Parse(sourceURL string, body io.Reader) (catalog.Content, error) {
	canonical, err := url.Parse(sourceURL)
	if err != nil {
		return catalog.Content{}, fmt.Errorf("parse source URL: %w", err)
	}
	document, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return catalog.Content{}, fmt.Errorf("parse HTML: %w", err)
	}
	if err := validateStructuredArticle(canonical, document); err != nil {
		return catalog.Content{}, err
	}

	content := catalog.Content{
		Source: catalog.SourceRef{
			ExternalID:   externalID(canonical.Path),
			CanonicalURL: canonical.String(),
		},
		Type:        detectType(document),
		Title:       cleanText(document.Find("h1").First().Text()),
		SeriesTitle: cleanText(document.Find(".category, .page-header__category").First().Text()),
		Attribution: "VOA Learning English",
	}
	if content.Title == "" {
		return catalog.Content{}, fmt.Errorf("required title is missing")
	}

	content.PublishedAt = parsePublishedAt(document)
	content.Level = inferLevel(content.SeriesTitle, document)
	content.Body = parseBlocks(document)
	content.Assets = parseAssets(document, canonical)
	content.RightsStatus = classifyRights(document)
	return content, nil
}

func inferLevel(series string, document *goquery.Document) catalog.Level {
	value := strings.ToLower(series + " " + cleanText(document.Find("meta[name='keywords']").AttrOr("content", "")))
	switch {
	case strings.Contains(value, "intermediate"):
		return catalog.LevelIntermediate
	case strings.Contains(value, "advanced"):
		return catalog.LevelAdvanced
	case strings.Contains(value, "beginning"), strings.Contains(value, "let's learn english"):
		return catalog.LevelBeginning
	default:
		return ""
	}
}

func validateStructuredArticle(source *url.URL, document *goquery.Document) error {
	if !strings.HasPrefix(source.Path, "/a/") {
		return nil
	}
	seen, valid := false, false
	document.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, selection *goquery.Selection) bool {
		var data map[string]any
		if json.Unmarshal([]byte(selection.Text()), &data) != nil {
			return true
		}
		main, _ := data["mainEntityOfPage"].(string)
		kind, _ := data["@type"].(string)
		if main == "" {
			return true
		}
		seen = true
		target, err := url.Parse(main)
		sourceID, targetID := externalID(source.Path), ""
		if target != nil {
			targetID = externalID(target.Path)
		}
		valid = err == nil && sourceID != "" && sourceID == targetID && (kind == "NewsArticle" || kind == "Article" || kind == "VideoObject" || kind == "AudioObject")
		return false
	})
	if seen && !valid {
		return fmt.Errorf("article structured data does not match source URL")
	}
	return nil
}

func classifyRights(document *goquery.Document) catalog.RightsStatus {
	text := strings.ToLower(cleanText(document.Find("body").Text()))
	for _, marker := range []string{"associated press", "ap photo", "reuters", "agence france-presse", "afp photo", "based on reports from"} {
		if strings.Contains(text, marker) {
			return catalog.RightsThirdPartyRestricted
		}
	}
	if strings.Contains(text, "voa learning english") {
		return catalog.RightsPublicDomainVerified
	}
	return catalog.RightsReviewRequired
}

func externalID(path string) string {
	matches := articleIDPattern.FindStringSubmatch(path)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func detectType(document *goquery.Document) catalog.ContentType {
	text := strings.ToLower(document.Find("body").Text())
	if strings.Contains(text, "lesson plan") || strings.Contains(text, "start the quiz") {
		return catalog.ContentTypeLesson
	}
	if document.Find("audio").Length() > 0 {
		return catalog.ContentTypePodcast
	}
	return catalog.ContentTypeArticle
}

func parsePublishedAt(document *goquery.Document) time.Time {
	selectors := []string{"time[datetime]", "meta[property='article:published_time']", "meta[name='date']"}
	for _, selector := range selectors {
		selection := document.Find(selector).First()
		value, exists := selection.Attr("datetime")
		if !exists {
			value, exists = selection.Attr("content")
		}
		if !exists {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05Z", "2006-01-02", "January 2, 2006"} {
			if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func parseBlocks(document *goquery.Document) []catalog.Block {
	root := document.Find(".wsw, .article__body, main article").First()
	if root.Length() == 0 {
		return nil
	}
	blocks := make([]catalog.Block, 0)
	root.Find("h2, h3, p, li").Each(func(_ int, selection *goquery.Selection) {
		text := cleanText(selection.Text())
		if text == "" {
			return
		}
		block := catalog.Block{Type: "paragraph", Text: text}
		if goquery.NodeName(selection) == "h2" {
			block.Type, block.Level = "heading", 2
		} else if goquery.NodeName(selection) == "h3" {
			block.Type, block.Level = "heading", 3
		} else if goquery.NodeName(selection) == "li" {
			block.Type = "listItem"
		}
		blocks = append(blocks, block)
	})
	return blocks
}

func parseAssets(document *goquery.Document, base *url.URL) []catalog.Asset {
	assets := make([]catalog.Asset, 0)
	seen := make(map[string]struct{})
	document.Find("a[href], audio[src], video[src], source[src], img[src]").Each(func(_ int, selection *goquery.Selection) {
		raw, ok := selection.Attr("href")
		if !ok {
			raw, ok = selection.Attr("src")
		}
		if !ok {
			return
		}
		resolved, err := base.Parse(strings.TrimSpace(raw))
		if err != nil || (resolved.Scheme != "https" && resolved.Scheme != "http") {
			return
		}
		kind := assetKind(resolved.Path, selection)
		if kind == "" {
			return
		}
		if _, exists := seen[resolved.String()]; exists {
			return
		}
		seen[resolved.String()] = struct{}{}
		asset := catalog.Asset{Kind: kind, SourceURL: resolved.String()}
		asset.QualityLabel = cleanText(selection.Text())
		if size, ok := selection.Attr("data-size"); ok {
			asset.ByteSize, _ = strconv.ParseInt(size, 10, 64)
		}
		assets = append(assets, asset)
	})
	return assets
}

func assetKind(path string, selection *goquery.Selection) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".mp3"), goquery.NodeName(selection) == "audio":
		return "audio"
	case strings.HasSuffix(lower, ".mp4"), goquery.NodeName(selection) == "video", goquery.NodeName(selection) == "source":
		return "video"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"), strings.HasSuffix(lower, ".png"), strings.HasSuffix(lower, ".webp"):
		return "image"
	case strings.HasSuffix(lower, ".pdf"), strings.HasSuffix(lower, ".doc"), strings.HasSuffix(lower, ".docx"):
		return "document"
	default:
		return ""
	}
}

func cleanText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
