package repository

import (
	"context"

	"github.com/cache-22/cache-22-server/internal/entity"
)

type GameRepository interface {
	List(ctx context.Context) ([]entity.Game, error)
	GetBySerial(ctx context.Context, serial string) (entity.Game, error)
	Upsert(ctx context.Context, game entity.Game) error
}

type HealthRepository interface {
	GetStatus(ctx context.Context, source string) (string, error)
}
