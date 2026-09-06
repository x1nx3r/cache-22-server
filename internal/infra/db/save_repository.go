package db

import (
	"context"
	"database/sql"

	"github.com/x1nx3r/cache-22-server/internal/entity"
)

type SaveRepository struct {
	db *sql.DB
}

func NewSaveRepository(db *sql.DB) *SaveRepository {
	return &SaveRepository{db: db}
}

func (r *SaveRepository) Get(ctx context.Context, userID int64, serial string, slot int) (entity.Save, error) {
	var s entity.Save
	err := r.db.QueryRowContext(ctx, `
SELECT user_id, serial, slot, sha256, size_bytes, public, updated_at
FROM user_saves WHERE user_id = $1 AND serial = $2 AND slot = $3`,
		userID, serial, slot).
		Scan(&s.UserID, &s.Serial, &s.Slot, &s.SHA256, &s.Size, &s.Public, &s.UpdatedAt)
	return s, err
}

func (r *SaveRepository) Upsert(ctx context.Context, s entity.Save) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO user_saves (user_id, serial, slot, sha256, size_bytes, public, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
ON CONFLICT (user_id, serial, slot) DO UPDATE
SET sha256 = excluded.sha256, size_bytes = excluded.size_bytes, updated_at = NOW()
`, s.UserID, s.Serial, s.Slot, s.SHA256, s.Size, s.Public)
	return err
}

func (r *SaveRepository) Delete(ctx context.Context, userID int64, serial string, slot int) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM user_saves WHERE user_id = $1 AND serial = $2 AND slot = $3`,
		userID, serial, slot)
	return err
}
