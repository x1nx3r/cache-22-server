package db

import (
	"context"
	"database/sql"
	"strings"

	"github.com/cache-22/cache-22-server/internal/entity"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func scanUser(row interface {
	Scan(...any) error
}) (entity.User, error) {
	var u entity.User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
	return u, err
}

func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (r *UserRepository) Create(ctx context.Context, username, hash string, admin bool) (entity.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	var u entity.User
	err := r.db.QueryRowContext(ctx, `
INSERT INTO users (username, password_hash, is_admin)
VALUES ($1, $2, $3)
RETURNING id, username, password_hash, is_admin, created_at
`, username, hash, admin).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
	return u, err
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (entity.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, is_admin, created_at FROM users WHERE username = $1
`, strings.ToLower(strings.TrimSpace(username))))
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (entity.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, is_admin, created_at FROM users WHERE id = $1
`, id))
}

func (r *UserRepository) List(ctx context.Context) ([]entity.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, password_hash, is_admin, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entity.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if out == nil {
		out = []entity.User{}
	}
	return out, rows.Err()
}

func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

func (r *UserRepository) SetPassword(ctx context.Context, id int64, hash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, id)
	return err
}

func (r *UserRepository) SetAdmin(ctx context.Context, id int64, admin bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET is_admin = $1 WHERE id = $2`, admin, id)
	return err
}
