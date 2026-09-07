package identity

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

var (
	ErrInvalidVocabulary       = errors.New("invalid vocabulary entry")
	ErrInvalidVocabularyReview = errors.New("invalid vocabulary review")
	ErrVocabularyNotFound      = errors.New("vocabulary entry not found")
	vocabularyWordPattern      = regexp.MustCompile(`^[\p{L}]+(?:['’-][\p{L}]+)*$`)
)

type VocabularyReviewInput struct {
	Stage           int32      `json:"stage"`
	ReviewCount     int32      `json:"reviewCount"`
	LapseCount      int32      `json:"lapseCount"`
	DueAt           time.Time  `json:"dueAt"`
	LastReviewedAt  *time.Time `json:"lastReviewedAt,omitempty"`
	ClientUpdatedAt time.Time  `json:"updatedAt"`
}

type VocabularyInput struct {
	ID           uuid.UUID              `json:"id"`
	Word         string                 `json:"word"`
	Definition   string                 `json:"definition,omitempty"`
	Context      string                 `json:"context,omitempty"`
	ContentID    *uuid.UUID             `json:"contentId,omitempty"`
	ContentTitle string                 `json:"contentTitle,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
	Review       *VocabularyReviewInput `json:"review,omitempty"`
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

func (s *Service) SetVocabularyReview(ctx context.Context, userID, entryID uuid.UUID, input VocabularyReviewInput) (dbgen.VocabularyEntry, error) {
	if err := validateVocabularyReview(input); err != nil {
		return dbgen.VocabularyEntry{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return dbgen.VocabularyEntry{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	row, err := updateVocabularyReview(ctx, q, userID, entryID, input)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrVocabularyNotFound
	}
	if err != nil {
		return row, err
	}
	if _, err = q.CreateUserEvent(ctx, dbgen.CreateUserEventParams{UserID: userID, EntityType: "vocabulary", EntityID: entryID, Operation: "upsert"}); err != nil {
		return row, err
	}
	return row, tx.Commit(ctx)
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
	if input.Review != nil {
		if err := validateVocabularyReview(*input.Review); err != nil {
			return row, err
		}
		row, err = updateVocabularyReview(ctx, q, userID, row.ID, *input.Review)
		if err != nil {
			return row, err
		}
	}
	if _, err = q.CreateUserEvent(ctx, dbgen.CreateUserEventParams{UserID: userID, EntityType: "vocabulary", EntityID: row.ID, Operation: "upsert"}); err != nil {
		return row, err
	}
	return row, nil
}

func updateVocabularyReview(ctx context.Context, q *dbgen.Queries, userID, entryID uuid.UUID, input VocabularyReviewInput) (dbgen.VocabularyEntry, error) {
	lastReviewedAt := pgtype.Timestamptz{}
	if input.LastReviewedAt != nil && !input.LastReviewedAt.IsZero() {
		lastReviewedAt = pgtype.Timestamptz{Time: input.LastReviewedAt.UTC(), Valid: true}
	}
	return q.UpdateVocabularyReview(ctx, dbgen.UpdateVocabularyReviewParams{
		Stage:           input.Stage,
		ReviewCount:     input.ReviewCount,
		LapseCount:      input.LapseCount,
		DueAt:           pgtype.Timestamptz{Time: input.DueAt.UTC(), Valid: true},
		LastReviewedAt:  lastReviewedAt,
		ClientUpdatedAt: pgtype.Timestamptz{Time: input.ClientUpdatedAt.UTC(), Valid: true},
		UserID:          userID,
		ID:              entryID,
	})
}

func validateVocabularyReview(input VocabularyReviewInput) error {
	if input.Stage < 0 || input.Stage > 6 || input.ReviewCount < 0 || input.LapseCount < 0 ||
		input.LapseCount > input.ReviewCount || input.DueAt.IsZero() || input.ClientUpdatedAt.IsZero() ||
		(input.ReviewCount > 0 && (input.LastReviewedAt == nil || input.LastReviewedAt.IsZero())) {
		return ErrInvalidVocabularyReview
	}
	return nil
}
