package db

import (
	"context"
)

type HealthRepository struct{}

func NewHealthRepository() *HealthRepository {
	return &HealthRepository{}
}

func (r *HealthRepository) GetStatus(_ context.Context, _ string) (string, error) {
	return "ok", nil
}
