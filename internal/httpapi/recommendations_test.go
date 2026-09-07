package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

func TestPersonalizedHomePrioritizesContinueAndLearningNeeds(t *testing.T) {
	continueID, grammarID, generalID, completedID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	contents := []dbgen.ListPublishedContentsRow{
		recommendationTestRow(completedID, "Completed", 100, "intermediate", nil, 4),
		recommendationTestRow(generalID, "General", 70, "intermediate", nil, 3),
		recommendationTestRow(grammarID, "Grammar", 50, "intermediate", []string{"grammar"}, 2),
		recommendationTestRow(continueID, "Continue", 20, "intermediate", nil, 1),
	}
	progress := []dbgen.LearningProgress{
		{ContentID: continueID, PositionSeconds: 42, Completed: false},
		{ContentID: completedID, PositionSeconds: 120, Completed: true},
	}
	grammar := []dbgen.GrammarAttempt{{CorrectCount: 1, QuestionCount: 4}}

	sections := personalizedHomeSections(contents, progress, grammar, 0)
	if len(sections) != 3 || sections[0]["id"] != "continue" || sections[1]["id"] != "for-you" {
		t.Fatalf("sections = %#v", sections)
	}
	continueItems := sections[0]["items"].([]map[string]any)
	if continueItems[0]["id"] != continueID {
		t.Fatalf("continue items = %#v", continueItems)
	}
	recommended := sections[1]["items"].([]map[string]any)
	if recommended[0]["id"] != grammarID || recommended[0]["recommendationReason"] != "Build confidence with more grammar practice" {
		t.Fatalf("recommended = %#v", recommended)
	}
	for _, item := range recommended {
		if item["id"] == completedID || item["id"] == continueID {
			t.Fatalf("completed or continued content leaked into recommendations: %#v", item)
		}
	}
}

func TestPersonalizedHomeColdStartIsNonNull(t *testing.T) {
	contents := []dbgen.ListPublishedContentsRow{recommendationTestRow(uuid.New(), "First", 80, "beginning", nil, 1)}
	sections := personalizedHomeSections(contents, nil, nil, 0)
	encoded, err := json.Marshal(sections)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 2 || sections[0]["id"] != "for-you" || string(encoded) == "null" {
		t.Fatalf("cold-start sections = %s", encoded)
	}
	if empty := personalizedHomeSections(nil, nil, nil, 0); empty == nil || len(empty) != 0 {
		t.Fatalf("empty sections = %#v", empty)
	}
}

func recommendationTestRow(id uuid.UUID, title string, quality int, level string, goals []string, day int) dbgen.ListPublishedContentsRow {
	metadata, _ := json.Marshal(recommendationMetadata{QualityScore: quality, LearningGoals: goals})
	return dbgen.ListPublishedContentsRow{
		ID: id, Type: dbgen.ContentTypeArticle, Title: title, Revision: 1, Metadata: metadata,
		Level:       pgtype.Text{String: level, Valid: true},
		PublishedAt: pgtype.Timestamptz{Time: time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC), Valid: true},
	}
}
