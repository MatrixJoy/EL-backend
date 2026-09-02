package voa

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

type SitemapEntry struct {
	URL          string
	LastModified time.Time
}

type sitemapURLSet struct {
	URLs []struct {
		Location     string `xml:"loc"`
		LastModified string `xml:"lastmod"`
	} `xml:"url"`
}

type sitemapIndex struct {
	Sitemaps []struct {
		Location string `xml:"loc"`
	} `xml:"sitemap"`
}

type SitemapDocument struct {
	Entries  []SitemapEntry
	Children []string
}

func ParseSitemapDocument(reader io.Reader) (SitemapDocument, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 20<<20))
	if err != nil {
		return SitemapDocument{}, fmt.Errorf("read sitemap: %w", err)
	}
	var index sitemapIndex
	if err := xml.Unmarshal(data, &index); err == nil && len(index.Sitemaps) > 0 {
		result := SitemapDocument{}
		for _, item := range index.Sitemaps {
			target, parseErr := url.Parse(strings.TrimSpace(item.Location))
			if parseErr == nil && target.Scheme == "https" && target.Hostname() == "learningenglish.voanews.com" {
				result.Children = append(result.Children, target.String())
			}
		}
		return result, nil
	}
	entries, err := ParseSitemap(strings.NewReader(string(data)))
	return SitemapDocument{Entries: entries}, err
}

func ParseSitemap(reader io.Reader) ([]SitemapEntry, error) {
	var payload sitemapURLSet
	decoder := xml.NewDecoder(io.LimitReader(reader, 20<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode sitemap: %w", err)
	}
	entries := make([]SitemapEntry, 0, len(payload.URLs))
	for _, item := range payload.URLs {
		target, err := url.Parse(strings.TrimSpace(item.Location))
		if err != nil || target.Scheme != "https" || target.Hostname() != "learningenglish.voanews.com" {
			continue
		}
		if !strings.HasPrefix(target.Path, "/a/") {
			continue
		}
		entry := SitemapEntry{URL: target.String()}
		entry.LastModified, _ = time.Parse(time.RFC3339Nano, item.LastModified)
		entries = append(entries, entry)
	}
	return entries, nil
}
