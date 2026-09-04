package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestParseRetellUpload(t *testing.T) {
	attemptID := uuid.New()
	contentID := uuid.New()
	createdAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	request := httptest.NewRequest("PUT", "/api/v1/me/retell-attempts/"+attemptID.String(), nil)
	request.Header.Set("X-Content-ID", contentID.String())
	request.Header.Set("X-Duration-Milliseconds", "42000")
	request.Header.Set("X-Created-At", createdAt.Format(time.RFC3339))
	request = request.WithContext(request.Context())

	parsedAttempt, parsedContent, duration, parsedCreatedAt, ok := parseRetellUpload(withAttemptURLParam(request, attemptID.String()))
	if !ok || parsedAttempt != attemptID || parsedContent != contentID || duration != 42000 || !parsedCreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected parse result: %v %v %d %v %v", parsedAttempt, parsedContent, duration, parsedCreatedAt, ok)
	}
}

func withAttemptURLParam(request *http.Request, value string) *http.Request {
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("attemptId", value)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

func TestParseRetellUploadRejectsExcessiveDuration(t *testing.T) {
	request := httptest.NewRequest("PUT", "/", nil)
	request.Header.Set("X-Content-ID", uuid.NewString())
	request.Header.Set("X-Duration-Milliseconds", "3600001")
	request.Header.Set("X-Created-At", time.Now().UTC().Format(time.RFC3339))
	_, _, _, _, ok := parseRetellUpload(withAttemptURLParam(request, uuid.NewString()))
	if ok {
		t.Fatal("expected invalid duration to be rejected")
	}
}
