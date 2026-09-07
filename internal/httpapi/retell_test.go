package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
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

func TestRetellReviewNormalizesKeywords(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 8, 5, 0, 0, 0, time.UTC)
	input := retellReviewInput{
		Transcript: "  I retold the climate story.  ", MatchedKeywords: []string{" Climate ", "STORY"},
		KeywordCount: 4, WordCount: 6, CompletionScore: 72, UpdatedAt: now,
	}
	if !input.normalizeAndValidate(now) {
		t.Fatal("valid review rejected")
	}
	if input.Transcript != "I retold the climate story." || input.MatchedKeywords[0] != "climate" || input.MatchedKeywords[1] != "story" {
		t.Fatalf("normalized review = %#v", input)
	}
}

func TestRetellReviewRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 8, 5, 0, 0, 0, time.UTC)
	for name, input := range map[string]retellReviewInput{
		"empty transcript": {KeywordCount: 1, WordCount: 1, UpdatedAt: now},
		"duplicate words":  {Transcript: "hello", MatchedKeywords: []string{"story", "STORY"}, KeywordCount: 2, WordCount: 1, UpdatedAt: now},
		"bad score":        {Transcript: "hello", KeywordCount: 1, WordCount: 1, CompletionScore: 101, UpdatedAt: now},
		"future timestamp": {Transcript: "hello", KeywordCount: 1, WordCount: 1, UpdatedAt: now.Add(6 * time.Minute)},
	} {
		if input.normalizeAndValidate(now) {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestRetellAttemptDTOIncludesOptionalReview(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 8, 5, 0, 0, 0, time.UTC)
	row := dbgen.RetellAttempt{
		ID: uuid.New(), ContentID: uuid.New(), DurationMilliseconds: 42_000, ByteSize: 1_024,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}, ServerUpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		Transcript: pgtype.Text{String: "A short retelling.", Valid: true}, MatchedKeywords: []string{"short"},
		KeywordCount: 2, WordCount: 3, CompletionScore: 61,
		ReviewClientUpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	encoded, err := json.Marshal(retellAttemptDTO(row))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Review *struct {
			Transcript      string `json:"transcript"`
			CompletionScore int32  `json:"completionScore"`
		} `json:"review"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil || response.Review == nil || response.Review.Transcript != "A short retelling." || response.Review.CompletionScore != 61 {
		t.Fatalf("response = %s, error = %v", encoded, err)
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
