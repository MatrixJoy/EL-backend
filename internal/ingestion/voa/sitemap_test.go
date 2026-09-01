package voa

import (
	"strings"
	"testing"
)

func TestParseSitemapAllowsOnlyVOAArticles(t *testing.T) {
	xml := `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://learningenglish.voanews.com/a/123.html</loc><lastmod>2026-08-31T00:00:00Z</lastmod></url>
<url><loc>https://evil.example/a/456.html</loc></url><url><loc>https://learningenglish.voanews.com/embed/1</loc></url></urlset>`
	entries, err := ParseSitemap(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].URL != "https://learningenglish.voanews.com/a/123.html" {
		t.Fatalf("entries = %+v", entries)
	}
}
