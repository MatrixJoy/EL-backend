package httpapi

import (
	"slices"
	"testing"
)

func TestDictionaryCandidates(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"birds":     {"bird"},
		"stories":   {"story"},
		"running":   {"run"},
		"discarded": {"discard", "discarde"},
	}
	for word, expected := range tests {
		got := dictionaryCandidates(word, nil)
		for _, candidate := range expected {
			if !slices.Contains(got, candidate) {
				t.Errorf("dictionaryCandidates(%q) = %v; missing %q", word, got, candidate)
			}
		}
	}
}

func TestDictionaryCandidatesPreferExceptionLemma(t *testing.T) {
	t.Parallel()
	got := dictionaryCandidates("children", []string{"child"})
	if len(got) < 2 || got[0] != "children" || got[1] != "child" {
		t.Fatalf("dictionaryCandidates() = %v", got)
	}
}
