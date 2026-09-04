package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
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
type MigrationInput struct {
	Bookmarks []uuid.UUID     `json:"bookmarks"`
	Progress  []ProgressInput `json:"progress"`
}
type LoginResult struct {
	UserID      uuid.UUID `json:"userId"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

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
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LoginResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbgen.New(tx)
	user, err := q.UpsertAppleUser(ctx, apple.Subject)
	if err != nil {
		return LoginResult{}, err
	}
	identityHash := sha256.Sum256([]byte(identityToken))
	if err := q.RecordAppleIdentityTokenUse(ctx, dbgen.RecordAppleIdentityTokenUseParams{TokenHash: identityHash[:], UserID: user.ID}); err != nil {
		return LoginResult{}, fmt.Errorf("identity token replay rejected: %w", err)
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

func (s *Service) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	return dbgen.New(s.pool).DeleteUser(ctx, userID)
}
func (s *Service) Queries() *dbgen.Queries { return dbgen.New(s.pool) }
