package voa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultMaxHTMLBytes int64 = 5 << 20

type FetchRequest struct {
	URL          string
	ETag         string
	LastModified string
}

type FetchResult struct {
	StatusCode   int
	Body         []byte
	ETag         string
	LastModified string
	ContentType  string
	SHA256       string
	FinalURL     string
}

type Fetcher struct {
	client          *http.Client
	userAgent       string
	allowedHosts    map[string]struct{}
	maxHTMLBytes    int64
	minimumInterval time.Duration
	rateMu          sync.Mutex
	nextRequest     time.Time
}

func (f *Fetcher) SetMinimumInterval(interval time.Duration) { f.minimumInterval = interval }

func NewFetcher(client *http.Client, userAgent string, allowedHosts ...string) *Fetcher {
	hosts := make(map[string]struct{}, len(allowedHosts))
	for _, host := range allowedHosts {
		hosts[strings.ToLower(host)] = struct{}{}
	}
	fetcher := &Fetcher{
		client:       client,
		userAgent:    userAgent,
		allowedHosts: hosts,
		maxHTMLBytes: defaultMaxHTMLBytes,
	}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return fetcher.validateURL(request.URL)
	}
	return fetcher
}

func (f *Fetcher) Fetch(ctx context.Context, input FetchRequest) (FetchResult, error) {
	if err := f.waitForRateLimit(ctx); err != nil {
		return FetchResult{}, err
	}
	target, err := url.Parse(input.URL)
	if err != nil {
		return FetchResult{}, fmt.Errorf("parse URL: %w", err)
	}
	if err := f.validateURL(target); err != nil {
		return FetchResult{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("User-Agent", f.userAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	if input.ETag != "" {
		request.Header.Set("If-None-Match", input.ETag)
	}
	if input.LastModified != "" {
		request.Header.Set("If-Modified-Since", input.LastModified)
	}

	response, err := f.client.Do(request)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch %s: %w", target.Hostname(), err)
	}
	defer func() { _ = response.Body.Close() }()

	result := FetchResult{
		StatusCode:   response.StatusCode,
		ETag:         response.Header.Get("ETag"),
		LastModified: response.Header.Get("Last-Modified"),
		ContentType:  response.Header.Get("Content-Type"),
		FinalURL:     response.Request.URL.String(),
	}
	if response.StatusCode == http.StatusNotModified {
		return result, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("unexpected upstream status %d", response.StatusCode)
	}
	if !strings.Contains(strings.ToLower(result.ContentType), "text/html") {
		return result, fmt.Errorf("unexpected content type %q", result.ContentType)
	}
	if response.ContentLength > f.maxHTMLBytes {
		return result, fmt.Errorf("response exceeds %d bytes", f.maxHTMLBytes)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, f.maxHTMLBytes+1))
	if err != nil {
		return result, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > f.maxHTMLBytes {
		return result, fmt.Errorf("response exceeds %d bytes", f.maxHTMLBytes)
	}
	result.Body = body
	hash := sha256.Sum256(body)
	result.SHA256 = hex.EncodeToString(hash[:])
	return result, nil
}

func (f *Fetcher) waitForRateLimit(ctx context.Context) error {
	if f.minimumInterval <= 0 {
		return nil
	}
	f.rateMu.Lock()
	wait := time.Until(f.nextRequest)
	if wait < 0 {
		wait = 0
	}
	f.nextRequest = time.Now().Add(wait + f.minimumInterval)
	f.rateMu.Unlock()
	if wait == 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (f *Fetcher) validateURL(target *url.URL) error {
	if target.Scheme != "https" && target.Scheme != "http" {
		return fmt.Errorf("URL scheme %q is not allowed", target.Scheme)
	}
	if target.User != nil {
		return fmt.Errorf("URL userinfo is not allowed")
	}
	if _, ok := f.allowedHosts[strings.ToLower(target.Host)]; ok {
		return nil
	}
	if target.Port() != "" && target.Port() != "443" && target.Port() != "80" {
		return fmt.Errorf("URL port %q is not allowed", target.Port())
	}
	if _, ok := f.allowedHosts[strings.ToLower(target.Hostname())]; !ok {
		return fmt.Errorf("URL host %q is not allowed", target.Hostname())
	}
	return nil
}
