package identity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeVocabularyWord(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		" Chickadee ":   "chickadee",
		"Mother-in-law": "mother-in-law",
		"don’t":         "don’t",
		"O'Brien":       "o'brien",
	}
	for input, want := range tests {
		got, err := NormalizeVocabularyWord(input)
		if err != nil || got != want {
			t.Errorf("NormalizeVocabularyWord(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestValidateVocabularyReview(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	valid := VocabularyReviewInput{Stage: 2, ReviewCount: 3, LapseCount: 1, DueAt: now.Add(7 * 24 * time.Hour), LastReviewedAt: &now, ClientUpdatedAt: now}
	if err := validateVocabularyReview(valid); err != nil {
		t.Fatalf("valid review rejected: %v", err)
	}
	for name, input := range map[string]VocabularyReviewInput{
		"stage":        {Stage: 7, DueAt: now, ClientUpdatedAt: now},
		"counts":       {Stage: 1, ReviewCount: 1, LapseCount: 2, DueAt: now, LastReviewedAt: &now, ClientUpdatedAt: now},
		"missing date": {Stage: 1, ReviewCount: 1, LastReviewedAt: &now, ClientUpdatedAt: now},
		"missing last": {Stage: 1, ReviewCount: 1, DueAt: now, ClientUpdatedAt: now},
	} {
		if err := validateVocabularyReview(input); !errors.Is(err, ErrInvalidVocabularyReview) {
			t.Errorf("%s error = %v", name, err)
		}
	}
}

func TestNormalizeVocabularyWordRejectsPhrasesAndOversizedValues(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"", "two words", "word!", strings.Repeat("a", 101)} {
		if _, err := NormalizeVocabularyWord(input); !errors.Is(err, ErrInvalidVocabulary) {
			t.Errorf("NormalizeVocabularyWord(%q) error = %v", input, err)
		}
	}
}
