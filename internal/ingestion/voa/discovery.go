package voa

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type DiscoveredLink struct {
	URL      string
	PageType string
	Title    string
}

// DiscoverLinks handles landing, section and listing pages without coupling
// downstream jobs to a particular VOA CSS class.
func DiscoverLinks(sourceURL string, reader io.Reader) ([]DiscoveredLink, error) {
	base, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("parse source URL: %w", err)
	}
	document, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	unique := make(map[string]DiscoveredLink)
	document.Find("a[href]").Each(func(_ int, selection *goquery.Selection) {
		href, _ := selection.Attr("href")
		target, parseErr := base.Parse(strings.TrimSpace(href))
		if parseErr != nil || target.Hostname() != "learningenglish.voanews.com" {
			return
		}
		target.Fragment = ""
		pageType := ""
		switch {
		case strings.HasPrefix(target.Path, "/a/"):
			pageType = "article"
		case strings.HasPrefix(target.Path, "/p/"):
			pageType = "landing"
		case strings.HasPrefix(target.Path, "/z/"):
			pageType = "listing"
		default:
			return
		}
		unique[target.String()] = DiscoveredLink{URL: target.String(), PageType: pageType, Title: cleanText(selection.Text())}
	})
	links := make([]DiscoveredLink, 0, len(unique))
	for _, link := range unique {
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].URL < links[j].URL })
	return links, nil
}
