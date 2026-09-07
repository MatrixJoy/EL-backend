package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oldj/english-learning/backend/internal/identity"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

const recommendationCandidateLimit = 100

type recommendationMetadata struct {
	QualityScore  int      `json:"qualityScore"`
	LearningGoals []string `json:"learningGoals"`
}

type rankedRecommendation struct {
	row    dbgen.ListPublishedContentsRow
	score  int
	reason string
}

func mountRecommendationRoutes(r chi.Router, service *identity.Service) {
	r.Get("/me/home", func(w http.ResponseWriter, request *http.Request) {
		userID := currentUser(request).ID
		queries := service.Queries()
		contents, err := queries.ListPublishedContents(request.Context(), dbgen.ListPublishedContentsParams{PageLimit: recommendationCandidateLimit})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		progress, err := queries.ListProgress(request.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		grammar, err := queries.ListGrammarAttempts(request.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		vocabulary, err := queries.ListVocabularyEntries(request.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", request)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"sections": personalizedHomeSections(contents, progress, grammar, len(vocabulary)),
		}})
	})
}

func personalizedHomeSections(contents []dbgen.ListPublishedContentsRow, progress []dbgen.LearningProgress, grammar []dbgen.GrammarAttempt, vocabularyCount int) []map[string]any {
	byID := make(map[uuid.UUID]dbgen.ListPublishedContentsRow, len(contents))
	for _, row := range contents {
		byID[row.ID] = row
	}

	completed := make(map[uuid.UUID]bool, len(progress))
	continued := make(map[uuid.UUID]bool)
	continueItems := make([]map[string]any, 0, 5)
	levelActivity := map[string]int{}
	for _, item := range progress {
		completed[item.ContentID] = item.Completed
		row, ok := byID[item.ContentID]
		if !ok {
			continue
		}
		if row.Level.Valid {
			levelActivity[row.Level.String]++
		}
		if item.Completed || item.PositionSeconds <= 0 || len(continueItems) == 5 {
			continue
		}
		continueItems = append(continueItems, recommendationSummary(row, "Continue where you left off"))
		continued[row.ID] = true
	}

	preferredLevel := ""
	for level, count := range levelActivity {
		if count > levelActivity[preferredLevel] || count == levelActivity[preferredLevel] && level < preferredLevel {
			preferredLevel = level
		}
	}
	grammarCorrect, grammarQuestions := int32(0), int32(0)
	for _, attempt := range grammar {
		grammarCorrect += attempt.CorrectCount
		grammarQuestions += attempt.QuestionCount
	}
	needsGrammarPractice := grammarQuestions > 0 && float64(grammarCorrect)/float64(grammarQuestions) < 0.8

	ranked := make([]rankedRecommendation, 0, len(contents))
	for index, row := range contents {
		if completed[row.ID] || continued[row.ID] {
			continue
		}
		metadata := parseRecommendationMetadata(row.Metadata)
		score := metadata.QualityScore + max(0, 20-index)
		reason := "A high-quality lesson selected for you"
		if preferredLevel != "" && row.Level.Valid && row.Level.String == preferredLevel {
			score += 18
			reason = "Matches the level you are learning"
		}
		if vocabularyCount > 0 && containsString(metadata.LearningGoals, "vocabulary") {
			score += min(15, vocabularyCount*2)
			reason = "Practice with vocabulary-rich listening"
		}
		if needsGrammarPractice && containsString(metadata.LearningGoals, "grammar") {
			score += 30
			reason = "Build confidence with more grammar practice"
		}
		ranked = append(ranked, rankedRecommendation{row: row, score: score, reason: reason})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].row.PublishedAt.Valid != ranked[j].row.PublishedAt.Valid {
			return ranked[i].row.PublishedAt.Valid
		}
		if ranked[i].row.PublishedAt.Valid && !ranked[i].row.PublishedAt.Time.Equal(ranked[j].row.PublishedAt.Time) {
			return ranked[i].row.PublishedAt.Time.After(ranked[j].row.PublishedAt.Time)
		}
		return ranked[i].row.ID.String() < ranked[j].row.ID.String()
	})
	recommendedItems := make([]map[string]any, 0, min(10, len(ranked)))
	for _, item := range ranked {
		if len(recommendedItems) == 10 {
			break
		}
		recommendedItems = append(recommendedItems, recommendationSummary(item.row, item.reason))
	}

	latestItems := make([]map[string]any, 0, min(8, len(contents)))
	for _, row := range contents {
		if len(latestItems) == 8 {
			break
		}
		latestItems = append(latestItems, recommendationSummary(row, "Recently added"))
	}

	sections := make([]map[string]any, 0, 3)
	if len(continueItems) > 0 {
		sections = append(sections, map[string]any{"id": "continue", "title": "Continue Learning", "items": continueItems})
	}
	if len(recommendedItems) > 0 {
		sections = append(sections, map[string]any{"id": "for-you", "title": "For You", "items": recommendedItems})
	}
	if len(latestItems) > 0 {
		sections = append(sections, map[string]any{"id": "latest", "title": "Recently Added", "items": latestItems})
	}
	return sections
}

func recommendationSummary(row dbgen.ListPublishedContentsRow, reason string) map[string]any {
	return map[string]any{
		"id": row.ID, "type": row.Type, "title": row.Title,
		"summary": textValue(row.Summary), "level": textValue(row.Level),
		"publishedAt": nullableTime(row.PublishedAt), "durationSeconds": intValue(row.DurationSeconds),
		"learningGoals": contentLearningGoals(row.Metadata), "revision": row.Revision, "recommendationReason": reason,
	}
}

func parseRecommendationMetadata(raw []byte) recommendationMetadata {
	var metadata recommendationMetadata
	_ = json.Unmarshal(raw, &metadata)
	metadata.QualityScore = min(100, max(0, metadata.QualityScore))
	return metadata
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(value, expected) {
			return true
		}
	}
	return false
}
