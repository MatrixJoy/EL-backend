package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

func TestVocabularyResponseIncludesReviewSchedule(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	row := dbgen.VocabularyEntry{
		ID: uuid.New(), UserID: uuid.New(), Word: "practice", NormalizedWord: "practice",
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		ReviewStage: 2, ReviewCount: 3, LapseCount: 1,
		ReviewDueAt:           pgtype.Timestamptz{Time: now.Add(7 * 24 * time.Hour), Valid: true},
		LastReviewedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		ReviewClientUpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	encoded, err := json.Marshal(vocabularyResponse(row))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Review struct {
			Stage       int32 `json:"stage"`
			ReviewCount int32 `json:"reviewCount"`
			LapseCount  int32 `json:"lapseCount"`
		} `json:"review"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	if response.Review.Stage != 2 || response.Review.ReviewCount != 3 || response.Review.LapseCount != 1 {
		t.Fatalf("review response = %s", encoded)
	}
}
