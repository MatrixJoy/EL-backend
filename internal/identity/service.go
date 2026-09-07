package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oldj/english-learning/backend/internal/repository/dbgen"
)

type ProgressInput struct {
	ContentID       uuid.UUID `json:"contentId"`
	PositionSeconds int32     `json:"positionSeconds"`
	Completed       bool      `json:"completed"`
	ClientUpdatedAt time.Time `json:"clientUpdatedAt"`
}
type GrammarAttemptInput struct {
	ID            uuid.UUID `json:"id"`
	ContentID     uuid.UUID `json:"contentId"`
	ContentTitle  string    `json:"contentTitle"`
	CorrectCount  int32     `json:"correctCount"`
	QuestionCount int32     `json:"questionCount"`
	CreatedAt     time.Time `json:"createdAt"`
}
type MigrationInput struct {
	Bookmarks       []uuid.UUID           `json:"bookmarks"`
	Progress        []ProgressInput       `json:"progress"`
	Vocabulary      []VocabularyInput     `json:"vocabulary"`
	GrammarAttempts []GrammarAttemptInput `json:"grammarAttempts"`
}
type LoginResult struct {
	UserID      uuid.UUID `json:"userId"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

var ErrInvalidGrammarAttempt = errors.New("invalid grammar attempt")

type Service struct {
	pool       *pgxpool.Pool
	verifier   AppleVerifier
	sessionTTL time.Duration
}

func NewService(pool *pgxpool.Pool, verifier AppleVerifier, sessionTTL time.Duration) *Service {
	return &Service{pool: pool, verifier: verifier, sessionTTL: sessionTTL}
}

func (s *Service) Login(ctx context.Context, identityToken, nonce string, migration MigrationInput) (LoginResult, error) {
	apple, err := s.verifier.Verify(ctx, identityToken, nonce)
	if err != nil {
		return LoginResult{}, err
	}
	identityHash := sha256.Sum256([]byte(identityToken))
	return s.createLogin(ctx, apple.Subject, identityHash[:], migration)
}

func (s *Service) LoginDevelopment(ctx context.Context, deviceID uuid.UUID, migration MigrationInput) (LoginResult, error) {
	if deviceID == uuid.Nil {
		return LoginResult{}, fmt.Errorf("development device ID is required")
	}
	return s.createLogin(ctx, "development:"+deviceID.String(), nil, migration)
}

func (s *Service) createLogin(ctx context.Context, subject string, identityTokenHash []byte, migration MigrationInput) (LoginResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LoginResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	user, err := q.UpsertAppleUser(ctx, subject)
	if err != nil {
		return LoginResult{}, err
	}
	if len(identityTokenHash) > 0 {
		if err := q.RecordAppleIdentityTokenUse(ctx, dbgen.RecordAppleIdentityTokenUseParams{TokenHash: identityTokenHash, UserID: user.ID}); err != nil {
			return LoginResult{}, fmt.Errorf("identity token replay rejected: %w", err)
		}
	}
	for _, contentID := range migration.Bookmarks {
		if err := q.UpsertBookmark(ctx, dbgen.UpsertBookmarkParams{UserID: user.ID, ContentID: contentID}); err != nil {
			return LoginResult{}, err
		}
		_, _ = q.CreateUserEvent(ctx, dbgen.CreateUserEventParams{UserID: user.ID, EntityType: "bookmark", EntityID: contentID, Operation: "upsert"})
	}
	for _, progress := range migration.Progress {
		if _, err := upsertProgress(ctx, q, user.ID, progress); err != nil {
			return LoginResult{}, err
		}
	}
	for _, entry := range migration.Vocabulary {
		if _, err := upsertVocabulary(ctx, q, user.ID, entry); err != nil {
			return LoginResult{}, err
		}
	}
	for _, attempt := range migration.GrammarAttempts {
		if _, err := upsertGrammarAttempt(ctx, q, user.ID, attempt); err != nil {
			return LoginResult{}, err
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return LoginResult{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	expires := time.Now().Add(s.sessionTTL)
	if _, err := q.CreateSession(ctx, dbgen.CreateSessionParams{UserID: user.ID, TokenHash: hash[:], ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}}); err != nil {
		return LoginResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{UserID: user.ID, AccessToken: token, ExpiresAt: expires}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (dbgen.User, error) {
	if token == "" {
		return dbgen.User{}, fmt.Errorf("missing token")
	}
	hash := sha256.Sum256([]byte(token))
	return dbgen.New(s.pool).GetUserBySessionHash(ctx, hash[:])
}

func (s *Service) SetBookmark(ctx context.Context, userID, contentID uuid.UUID, enabled bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	operation := "upsert"
	if enabled {
		err = q.UpsertBookmark(ctx, dbgen.UpsertBookmarkParams{UserID: userID, ContentID: contentID})
	} else {
		operation = "delete"
		err = q.DeleteBookmark(ctx, dbgen.DeleteBookmarkParams{UserID: userID, ContentID: contentID})
	}
	if err != nil {
		return err
	}
	if _, err = q.CreateUserEvent(ctx, dbgen.CreateUserEventParams{UserID: userID, EntityType: "bookmark", EntityID: contentID, Operation: operation}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) SetProgress(ctx context.Context, userID uuid.UUID, input ProgressInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := upsertProgress(ctx, dbgen.New(tx), userID, input); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func upsertProgress(ctx context.Context, q *dbgen.Queries, userID uuid.UUID, input ProgressInput) (dbgen.LearningProgress, error) {
	row, err := q.UpsertProgress(ctx, dbgen.UpsertProgressParams{UserID: userID, ContentID: input.ContentID, PositionSeconds: input.PositionSeconds, Completed: input.Completed, ClientUpdatedAt: pgtype.Timestamptz{Time: input.ClientUpdatedAt, Valid: true}})
	if err != nil {
		return row, err
	}
	_, err = q.CreateUserEvent(ctx, dbgen.CreateUserEventParams{UserID: userID, EntityType: "progress", EntityID: input.ContentID, Operation: "upsert"})
	return row, err
}

func (s *Service) SetGrammarAttempt(ctx context.Context, userID uuid.UUID, input GrammarAttemptInput) (dbgen.GrammarAttempt, error) {
	if err := validateGrammarAttempt(input); err != nil {
		return dbgen.GrammarAttempt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return dbgen.GrammarAttempt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := upsertGrammarAttempt(ctx, dbgen.New(tx), userID, input)
	if err != nil {
		return row, err
	}
	return row, tx.Commit(ctx)
}

func upsertGrammarAttempt(ctx context.Context, q *dbgen.Queries, userID uuid.UUID, input GrammarAttemptInput) (dbgen.GrammarAttempt, error) {
	if err := validateGrammarAttempt(input); err != nil {
		return dbgen.GrammarAttempt{}, err
	}
	row, err := q.UpsertGrammarAttempt(ctx, dbgen.UpsertGrammarAttemptParams{
		UserID: userID, ID: input.ID, ContentID: input.ContentID, ContentTitle: strings.TrimSpace(input.ContentTitle),
		CorrectCount: input.CorrectCount, QuestionCount: input.QuestionCount,
		CreatedAt: pgtype.Timestamptz{Time: input.CreatedAt, Valid: true},
	})
	if err != nil {
		return row, err
	}
	_, err = q.CreateUserEvent(ctx, dbgen.CreateUserEventParams{UserID: userID, EntityType: "grammarAttempt", EntityID: input.ID, Operation: "upsert"})
	return row, err
}

func validateGrammarAttempt(input GrammarAttemptInput) error {
	title := strings.TrimSpace(input.ContentTitle)
	if input.ID == uuid.Nil || input.ContentID == uuid.Nil || input.CreatedAt.IsZero() || title == "" ||
		input.QuestionCount < 1 || input.QuestionCount > 100 ||
		input.CorrectCount < 0 || input.CorrectCount > input.QuestionCount ||
		len(title) > 300 {
		return ErrInvalidGrammarAttempt
	}
	return nil
}

func (s *Service) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	return dbgen.New(s.pool).DeleteUser(ctx, userID)
}
func (s *Service) Queries() *dbgen.Queries { return dbgen.New(s.pool) }
