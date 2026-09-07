package identity

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateGrammarAttempt(t *testing.T) {
	valid := GrammarAttemptInput{
		ID: uuid.New(), ContentID: uuid.New(), ContentTitle: "A useful lesson",
		CorrectCount: 2, QuestionCount: 3, CreatedAt: time.Now(),
	}
	if err := validateGrammarAttempt(valid); err != nil {
		t.Fatalf("valid attempt rejected: %v", err)
	}

	cases := []GrammarAttemptInput{
		{ContentID: valid.ContentID, CorrectCount: 2, QuestionCount: 3, CreatedAt: valid.CreatedAt},
		{ID: valid.ID, CorrectCount: 2, QuestionCount: 3, CreatedAt: valid.CreatedAt},
		{ID: valid.ID, ContentID: valid.ContentID, CorrectCount: 4, QuestionCount: 3, CreatedAt: valid.CreatedAt},
		{ID: valid.ID, ContentID: valid.ContentID, CorrectCount: 0, QuestionCount: 0, CreatedAt: valid.CreatedAt},
		{ID: valid.ID, ContentID: valid.ContentID, CorrectCount: 0, QuestionCount: 1},
		{ID: valid.ID, ContentID: valid.ContentID, ContentTitle: "  ", CorrectCount: 0, QuestionCount: 1, CreatedAt: valid.CreatedAt},
	}
	for index, input := range cases {
		if err := validateGrammarAttempt(input); !errors.Is(err, ErrInvalidGrammarAttempt) {
			t.Errorf("case %d accepted: %v", index, err)
		}
	}
}
