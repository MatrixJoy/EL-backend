package identity

import (
	"errors"
	"strings"
	"testing"
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

func TestNormalizeVocabularyWordRejectsPhrasesAndOversizedValues(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"", "two words", "word!", strings.Repeat("a", 101)} {
		if _, err := NormalizeVocabularyWord(input); !errors.Is(err, ErrInvalidVocabulary) {
			t.Errorf("NormalizeVocabularyWord(%q) error = %v", input, err)
		}
	}
}
