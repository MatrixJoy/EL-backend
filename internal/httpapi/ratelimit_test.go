package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPRateLimiterRejectsBurst(t *testing.T) {
	limiter := newIPRateLimiter(1, 1)
	handler := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for index, expected := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "203.0.113.1:1234"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != expected {
			t.Fatalf("request %d status=%d want=%d", index, response.Code, expected)
		}
	}
}
