package voa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverLinks(t *testing.T) {
	html := `<a href="/a/1.html">One</a><a href="https://evil.example/a/2.html">Bad</a><a href="/z/1689">Podcast</a>`
	links, err := DiscoverLinks("https://learningenglish.voanews.com/", strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("links = %+v", links)
	}
}

func TestLiveFixtureSetContainsTwentyPages(t *testing.T) {
	files, err := filepath.Glob("../../../fixtures/voa/live/*.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("run scripts/capture_fixtures.sh to enable the live fixture audit")
	}
	if len(files) < 20 {
		t.Fatalf("live fixtures = %d, want at least 20", len(files))
	}
	for _, name := range files {
		info, statErr := os.Stat(name)
		if statErr != nil || info.Size() == 0 {
			t.Fatalf("invalid fixture %s: %v", name, statErr)
		}
	}
}
