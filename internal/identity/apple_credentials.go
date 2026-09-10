package identity

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type sealedAppleToken struct {
	id         uuid.UUID
	ciphertext []byte
}

func (s *Service) ConfigureAppleOAuth(clientID string, oauth AppleOAuth, key []byte) error {
	cipher, err := newTokenCipher(key)
	if err != nil {
		return err
	}
	if oauth == nil || clientID == "" {
		return ErrAppleUnavailable
	}
	s.appleClientID, s.oauth, s.tokenCipher = clientID, oauth, cipher
	return nil
}

func (s *Service) AppleSignInEnabled() bool { return s.oauth != nil && s.tokenCipher != nil }
func (s *Service) AppleClientID() string    { return s.appleClientID }

func (s *Service) LoginWithAuthorizationCode(ctx context.Context, identityToken, nonce, code string, migration MigrationInput) (LoginResult, error) {
	if !s.AppleSignInEnabled() {
		return LoginResult{}, ErrAppleUnavailable
	}
	if identityToken == "" || nonce == "" || code == "" || len(code) > 8192 || len(identityToken) > 16384 || len(nonce) > 256 {
		return LoginResult{}, ErrAppleAuthorization
	}
	apple, err := s.verifier.Verify(ctx, identityToken, nonce)
	if err != nil {
		return LoginResult{}, ErrAppleAuthorization
	}
	tokens, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return LoginResult{}, err
	}
	exchanged, err := s.verifier.Verify(ctx, tokens.IdentityToken, nonce)
	if err != nil || exchanged.Subject != apple.Subject {
		return LoginResult{}, ErrAppleAuthorization
	}
	credential := sealedAppleToken{id: uuid.New()}
	credential.ciphertext, err = s.tokenCipher.seal(tokens.RefreshToken, credential.id.String())
	if err != nil {
		return LoginResult{}, err
	}
	hash := sha256.Sum256([]byte(identityToken))
	result, err := s.createLogin(ctx, apple.Subject, hash[:], migration, credential)
	if err != nil {
		return LoginResult{}, err
	}
	result.AppleUserID = apple.Subject
	return result, nil
}

// ProcessAppleRevocations claims one detached credential at a time. Its lock
// prevents multiple API replicas from processing the same row concurrently.
// Account deletion itself is never blocked by an Apple outage.
func (s *Service) ProcessAppleRevocations(ctx context.Context) error {
	if !s.AppleSignInEnabled() {
		return nil
	}
	for i := 0; i < 10; i++ {
		found, err := s.processAppleRevocation(ctx)
		if err != nil || !found {
			return err
		}
	}
	return nil
}

func (s *Service) processAppleRevocation(ctx context.Context) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var row sealedAppleToken
	var attempts int
	err = tx.QueryRow(ctx, `SELECT id, encrypted_refresh_token, attempts FROM apple_credentials
		WHERE user_id IS NULL AND next_attempt_at <= now()
		ORDER BY next_attempt_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&row.id, &row.ciphertext, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	token, err := s.tokenCipher.open(row.ciphertext, row.id.String())
	if err == nil {
		err = s.oauth.Revoke(ctx, token)
	}
	if err == nil {
		_, err = tx.Exec(ctx, "DELETE FROM apple_credentials WHERE id=$1", row.id)
	} else {
		// Retain credentials on failures, including invalid configuration. Never
		// discard a revocation request merely because a retry count was reached.
		delay := time.Minute * time.Duration(1<<min(attempts, 10))
		_, err = tx.Exec(ctx, `UPDATE apple_credentials SET attempts=attempts+1, next_attempt_at=$2 WHERE id=$1`, row.id, time.Now().Add(delay))
	}
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}
