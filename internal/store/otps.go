package store

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	OTPVerifyEmail   = "verify_email"
	OTPResetPassword = "reset_password"
)

// maxOTPAttempts caps brute-force guessing of a 6-digit code: past this
// many wrong tries the code is dead regardless of expiry, and the caller
// has to request a new one.
const maxOTPAttempts = 5

// ErrOTPInvalid covers "no such code", "expired", "already used", and
// "too many wrong attempts" - deliberately one error so none of those
// states leak to the caller beyond "request a new code".
var ErrOTPInvalid = errors.New("invalid or expired code")

// CreateOTP invalidates any previous unused code for (userID, purpose) and
// inserts a fresh one, so only the most recently sent code ever works -
// requesting a new code (e.g. "resend") can't leave two valid ones around.
func (s *Store) CreateOTP(ctx context.Context, userID uuid.UUID, purpose, codeHash string, expiresAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE otps SET used_at = now() WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL`,
		userID, purpose,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO otps (user_id, purpose, code_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, purpose, codeHash, expiresAt,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// VerifyOTP checks codeHash against the active (unused, unexpired) code for
// (userID, purpose) and consumes it on success. A wrong code counts as an
// attempt; past maxOTPAttempts the code is invalidated outright.
func (s *Store) VerifyOTP(ctx context.Context, userID uuid.UUID, purpose, codeHash string) error {
	var id uuid.UUID
	var storedHash string
	var attempts int
	err := s.pool.QueryRow(ctx,
		`SELECT id, code_hash, attempts FROM otps
		 WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
		 ORDER BY created_at DESC LIMIT 1`,
		userID, purpose,
	).Scan(&id, &storedHash, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOTPInvalid
	}
	if err != nil {
		return err
	}
	if attempts >= maxOTPAttempts {
		return ErrOTPInvalid
	}

	// Constant-time: both operands are always a 64-char hex SHA-256 sum, so
	// this never leaks the wrong length by timing either.
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(codeHash)) == 1 {
		_, err := s.pool.Exec(ctx, `UPDATE otps SET used_at = now() WHERE id = $1`, id)
		return err
	}

	if _, err := s.pool.Exec(ctx, `UPDATE otps SET attempts = attempts + 1 WHERE id = $1`, id); err != nil {
		return err
	}
	return ErrOTPInvalid
}
