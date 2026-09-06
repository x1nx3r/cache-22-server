package db

import (
	"context"
	"database/sql"

	"github.com/cache-22/cache-22-server/internal/entity"
)

type CoverRepository struct {
	db *sql.DB
}

func NewCoverRepository(db *sql.DB) *CoverRepository {
	return &CoverRepository{db: db}
}

func (r *CoverRepository) GetBySerial(ctx context.Context, serial string) (entity.Cover, error) {
	var c entity.Cover
	err := r.db.QueryRowContext(ctx, `SELECT serial, provider, provider_id, image_path, fetched_at FROM covers WHERE serial = $1`, serial).
		Scan(&c.Serial, &c.Provider, &c.ProviderID, &c.ImagePath, &c.FetchedAt)
	return c, err
}

func (r *CoverRepository) Upsert(ctx context.Context, c entity.Cover) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO covers (serial, provider, provider_id, image_path, fetched_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT(serial) DO UPDATE SET provider=excluded.provider, provider_id=excluded.provider_id, image_path=excluded.image_path, fetched_at=NOW()
`, c.Serial, c.Provider, c.ProviderID, c.ImagePath)
	return err
}

func (r *CoverRepository) MissingSerials(ctx context.Context, serials []string) ([]string, error) {
	if len(serials) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT s.serial FROM unnest($1::text[]) AS s(serial)
LEFT JOIN covers c ON c.serial = s.serial
WHERE c.serial IS NULL
`, serials)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
