package db

import (
	"context"
	"database/sql"

	"github.com/x1nx3r/cache-22-server/internal/entity"
)

type GameRepository struct {
	db *sql.DB
}

func NewGameRepository(db *sql.DB) *GameRepository {
	return &GameRepository{db: db}
}

func (r *GameRepository) List(ctx context.Context) ([]entity.Game, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT serial, redump_hash, title, size_bytes, file_path, created_at, updated_at FROM games ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entity.Game
	for rows.Next() {
		var g entity.Game
		if err := rows.Scan(&g.Serial, &g.RedumpHash, &g.Title, &g.SizeBytes, &g.FilePath, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if out == nil {
		out = []entity.Game{}
	}
	return out, rows.Err()
}

func (r *GameRepository) GetBySerial(ctx context.Context, serial string) (entity.Game, error) {
	var g entity.Game
	err := r.db.QueryRowContext(ctx, `SELECT serial, redump_hash, title, size_bytes, file_path, created_at, updated_at FROM games WHERE serial = $1`, serial).
		Scan(&g.Serial, &g.RedumpHash, &g.Title, &g.SizeBytes, &g.FilePath, &g.CreatedAt, &g.UpdatedAt)
	return g, err
}

func (r *GameRepository) Upsert(ctx context.Context, g entity.Game) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO games (serial, redump_hash, title, size_bytes, file_path, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT(serial) DO UPDATE SET redump_hash=excluded.redump_hash, title=excluded.title, size_bytes=excluded.size_bytes, file_path=excluded.file_path, updated_at=NOW()
`, g.Serial, g.RedumpHash, g.Title, g.SizeBytes, g.FilePath)
	return err
}
