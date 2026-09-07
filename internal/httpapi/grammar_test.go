package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

func TestGrammarAttemptResponsesAreStableAndNeverNull(t *testing.T) {
	if encoded, err := json.Marshal(grammarAttemptResponses(nil)); err != nil || string(encoded) != "[]" {
		t.Fatalf("empty response = %s, %v", encoded, err)
	}
	now := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	row := dbgen.GrammarAttempt{
		UserID: uuid.New(), ID: uuid.New(), ContentID: uuid.New(), ContentTitle: "Grammar lesson",
		CorrectCount: 2, QuestionCount: 3,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	encoded, err := json.Marshal(grammarAttemptResponse(row))
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if json.Unmarshal(encoded, &response) != nil || response["contentTitle"] != "Grammar lesson" || response["correctCount"] != float64(2) {
		t.Fatalf("response = %s", encoded)
	}
}
