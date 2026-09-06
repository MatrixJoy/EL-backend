package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

func (h publicHandlers) dictionary(w http.ResponseWriter, request *http.Request) {
	normalized, err := identity.NormalizeVocabularyWord(chi.URLParam(request, "word"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", request)
		return
	}
	lemmas, err := h.queries.ListDictionaryFormLemmas(request.Context(), normalized)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
		return
	}
	candidates := dictionaryCandidates(normalized, lemmas)
	rows, err := h.queries.LookupDictionaryEntries(request.Context(), candidates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
		return
	}
	entries := make([]map[string]any, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := row.PartOfSpeech + "\x00" + strings.ToLower(row.Definition)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, dictionaryEntryResponse(row))
	}
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"query":          chi.URLParam(request, "word"),
		"normalizedWord": normalized,
		"entries":        entries,
	}})
}

func dictionaryEntryResponse(row dbgen.DictionaryEntry) map[string]any {
	source := "Learning content"
	if row.Source == "wordnet-3.1" {
		source = "Princeton WordNet 3.1"
	}
	return map[string]any{
		"word":         strings.ReplaceAll(row.Word, "_", " "),
		"partOfSpeech": row.PartOfSpeech,
		"definition":   row.Definition,
		"examples":     row.Examples,
		"source":       source,
	}
}

func dictionaryCandidates(word string, exceptionLemmas []string) []string {
	result := make([]string, 0, 10)
	seen := make(map[string]struct{})
	appendWord := func(value string) {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	appendWord(word)
	for _, lemma := range exceptionLemmas {
		appendWord(lemma)
	}
	if strings.HasSuffix(word, "ies") && len(word) > 3 {
		appendWord(strings.TrimSuffix(word, "ies") + "y")
	}
	if strings.HasSuffix(word, "es") && len(word) > 2 {
		appendWord(strings.TrimSuffix(word, "es"))
		appendWord(strings.TrimSuffix(word, "s"))
	}
	if strings.HasSuffix(word, "s") && len(word) > 1 {
		appendWord(strings.TrimSuffix(word, "s"))
	}
	for _, suffix := range []string{"ing", "ed"} {
		if !strings.HasSuffix(word, suffix) || len(word) <= len(suffix)+1 {
			continue
		}
		base := strings.TrimSuffix(word, suffix)
		appendWord(base)
		appendWord(base + "e")
		if len(base) >= 2 && base[len(base)-1] == base[len(base)-2] {
			appendWord(base[:len(base)-1])
		}
	}
	return result
}
