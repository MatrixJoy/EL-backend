package voa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFetcherSendsConditionalHeadersAndHashesBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `"old"` {
			t.Errorf("If-None-Match = %q", r.Header.Get("If-None-Match"))
		}
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("ETag", `"new"`)
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	defer server.Close()

	target, _ := url.Parse(server.URL)
	fetcher := NewFetcher(server.Client(), "test-agent", target.Host)
	result, err := fetcher.Fetch(context.Background(), FetchRequest{URL: server.URL, ETag: `"old"`})
	if err != nil {
		t.Fatal(err)
	}
	if result.ETag != `"new"` || len(result.SHA256) != 64 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFetcherRejectsUnknownHost(t *testing.T) {
	fetcher := NewFetcher(http.DefaultClient, "test-agent", "learningenglish.voanews.com")
	_, err := fetcher.Fetch(context.Background(), FetchRequest{URL: "https://127.0.0.1/private"})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestFetcherRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	fetcher := NewFetcher(server.Client(), "test-agent", target.Host)
	fetcher.maxHTMLBytes = 4
	if _, err := fetcher.Fetch(context.Background(), FetchRequest{URL: server.URL}); err == nil {
		t.Fatal("expected size limit error")
	}
}
