package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"
)

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, token string, userID int64, ttl time.Duration) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)
`, hashToken(token), userID, time.Now().Add(ttl))
	return err
}

func (r *SessionRepository) UserID(ctx context.Context, token string) (int64, bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT user_id FROM sessions WHERE token_hash = $1 AND expires_at > NOW()
`, hashToken(token)).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (r *SessionRepository) Revoke(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

func (r *SessionRepository) RevokeUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func (r *SessionRepository) Sweep(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= NOW()`)
	return err
}
