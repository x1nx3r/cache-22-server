package gameusecase

import (
	"context"

	"github.com/x1nx3r/cache-22-server/internal/app/repository"
	"github.com/x1nx3r/cache-22-server/internal/entity"
)

type UseCase interface {
	ListGames(ctx context.Context) ([]entity.Game, error)
	GetGame(ctx context.Context, serial string) (entity.Game, error)
	GetManifest(ctx context.Context, serial string) (entity.Manifest, error)
	RegisterGame(ctx context.Context, game entity.Game) error
	HealthCheck(ctx context.Context, source string) (*entity.HealthCheck, error)
}

type usecase struct {
	games  repository.GameRepository
	health repository.HealthRepository
}

func New(games repository.GameRepository, health repository.HealthRepository) UseCase {
	return &usecase{games: games, health: health}
}

func (uc *usecase) ListGames(ctx context.Context) ([]entity.Game, error) {
	return uc.games.List(ctx)
}

func (uc *usecase) GetGame(ctx context.Context, serial string) (entity.Game, error) {
	return uc.games.GetBySerial(ctx, serial)
}

func (uc *usecase) RegisterGame(ctx context.Context, game entity.Game) error {
	return uc.games.Upsert(ctx, game)
}

func (uc *usecase) GetManifest(ctx context.Context, serial string) (entity.Manifest, error) {
	g, err := uc.games.GetBySerial(ctx, serial)
	if err != nil {
		return entity.Manifest{}, err
	}
	boot := int64(entity.BootBytes)
	if g.SizeBytes < boot {
		boot = g.SizeBytes
	}
	var bootRanges [][2]int64
	if boot > 0 {
		bootRanges = [][2]int64{{0, boot}}
	}
	return entity.Manifest{
		Version:     entity.ManifestVersion,
		Serial:      g.Serial,
		RedumpHash:  g.RedumpHash,
		Title:       g.Title,
		SizeBytes:   g.SizeBytes,
		BlockSize:   entity.BlockSize,
		BootRanges:  bootRanges,
		FileURL:     "/v1/files/" + g.Serial,
		ManifestURL: "/v1/games/" + g.Serial + "/manifest.json",
		Supported:   isRawISO(g.FilePath),
	}, nil
}

func (uc *usecase) HealthCheck(ctx context.Context, source string) (*entity.HealthCheck, error) {
	status, err := uc.health.GetStatus(ctx, source)
	if err != nil {
		return nil, err
	}
	return &entity.HealthCheck{Status: status, Message: "service is healthy"}, nil
}
