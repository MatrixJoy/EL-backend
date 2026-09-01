package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProxyMediaForwardsRangeAndStatus(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Range") != "bytes=10-19" {
			t.Fatalf("range=%q", request.Header.Get("Range"))
		}
		return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 10-19/100"}, "Content-Length": {"10"}}, Body: io.NopCloser(bytes.NewBufferString("0123456789")), Request: request}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "/media/id", nil)
	request.Header.Set("Range", "bytes=10-19")
	response := httptest.NewRecorder()
	if err := proxyMedia(response, request, "https://voa-video-ns.akamaized.net/file.mp4", client); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusPartialContent || response.Header().Get("Content-Range") != "bytes 10-19/100" || response.Body.String() != "0123456789" {
		t.Fatalf("unexpected response: %+v body=%q", response.Result(), response.Body.String())
	}
}

func TestProxyMediaRejectsUnknownHost(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/media/id", nil)
	if err := proxyMedia(httptest.NewRecorder(), request, "https://127.0.0.1/private", http.DefaultClient); err == nil {
		t.Fatal("expected host rejection")
	}
}

func TestAllowedMediaHostIncludesOfficialVOAAudioCDN(t *testing.T) {
	if !allowedMediaHost("voa-audio.voanews.eu") {
		t.Fatal("official VOA audio CDN must be allowed")
	}
	if allowedMediaHost("voa-audio.voanews.eu.attacker.example") {
		t.Fatal("lookalike host must be rejected")
	}
}
