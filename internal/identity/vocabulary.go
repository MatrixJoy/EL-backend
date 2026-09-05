package identity

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

var (
	ErrInvalidVocabulary  = errors.New("invalid vocabulary entry")
	vocabularyWordPattern = regexp.MustCompile(`^[\p{L}]+(?:['’-][\p{L}]+)*$`)
)

type VocabularyInput struct {
	ID           uuid.UUID  `json:"id"`
	Word         string     `json:"word"`
	Definition   string     `json:"definition,omitempty"`
	Context      string     `json:"context,omitempty"`
	ContentID    *uuid.UUID `json:"contentId,omitempty"`
	ContentTitle string     `json:"contentTitle,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
}

func NormalizeVocabularyWord(word string) (string, error) {
	trimmed := strings.TrimSpace(word)
	if !utf8.ValidString(trimmed) || utf8.RuneCountInString(trimmed) > 100 || !vocabularyWordPattern.MatchString(trimmed) {
		return "", ErrInvalidVocabulary
	}
	return strings.ToLower(trimmed), nil
}

func (s *Service) SetVocabulary(ctx context.Context, userID uuid.UUID, input VocabularyInput) (dbgen.VocabularyEntry, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return dbgen.VocabularyEntry{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := upsertVocabulary(ctx, dbgen.New(tx), userID, input)
	if err != nil {
		return dbgen.VocabularyEntry{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return dbgen.VocabularyEntry{}, err
	}
	return row, nil
}

func (s *Service) DeleteVocabulary(ctx context.Context, userID, entryID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	if err := q.DeleteVocabularyEntry(ctx, dbgen.DeleteVocabularyEntryParams{UserID: userID, ID: entryID}); err != nil {
		return err
	}
	if _, err := q.CreateUserEvent(ctx, dbgen.CreateUserEventParams{UserID: userID, EntityType: "vocabulary", EntityID: entryID, Operation: "delete"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func upsertVocabulary(ctx context.Context, q *dbgen.Queries, userID uuid.UUID, input VocabularyInput) (dbgen.VocabularyEntry, error) {
	normalized, err := NormalizeVocabularyWord(input.Word)
	if err != nil || input.ID == uuid.Nil || utf8.RuneCountInString(input.Definition) > 2_000 || utf8.RuneCountInString(input.Context) > 1_000 || utf8.RuneCountInString(input.ContentTitle) > 300 {
		return dbgen.VocabularyEntry{}, ErrInvalidVocabulary
	}
	createdAt := input.CreatedAt.UTC()
	if input.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	contentID := pgtype.UUID{}
	if input.ContentID != nil && *input.ContentID != uuid.Nil {
		contentID = pgtype.UUID{Bytes: *input.ContentID, Valid: true}
	}
	row, err := q.UpsertVocabularyEntry(ctx, dbgen.UpsertVocabularyEntryParams{
		ID:              input.ID,
		UserID:          userID,
		Word:            strings.TrimSpace(input.Word),
		NormalizedWord:  normalized,
		Definition:      strings.TrimSpace(input.Definition),
		SentenceContext: strings.TrimSpace(input.Context),
		ContentID:       contentID,
		ContentTitle:    strings.TrimSpace(input.ContentTitle),
		CreatedAt:       pgtype.Timestamptz{Time: createdAt, Valid: true},
	})
	if err != nil {
		return row, err
	}
	if _, err = q.CreateUserEvent(ctx, dbgen.CreateUserEventParams{UserID: userID, EntityType: "vocabulary", EntityID: row.ID, Operation: "upsert"}); err != nil {
		return row, err
	}
	return row, nil
}
