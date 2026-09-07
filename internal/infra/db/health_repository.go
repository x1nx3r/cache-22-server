package db

import (
	"context"
	"database/sql"
	"time"
)

type HealthRepository struct {
	db *sql.DB
}

func NewHealthRepository(db *sql.DB) *HealthRepository {
	return &HealthRepository{db: db}
}

// GetStatus pings Postgres so /v1/health fails when the database is down.
// The ping is bounded so a wedged DB fails the probe fast instead of
// hanging the healthcheck.
func (r *HealthRepository) GetStatus(ctx context.Context, _ string) (string, error) {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := r.db.PingContext(pingCtx); err != nil {
		return "", err
	}
	return "ok", nil
}
