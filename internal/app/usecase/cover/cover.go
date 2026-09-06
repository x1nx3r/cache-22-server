package coverusecase

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/x1nx3r/cache-22-server/internal/entity"
	"github.com/x1nx3r/cache-22-server/internal/infra/igdb"
)

type Games interface {
	List(ctx context.Context) ([]entity.Game, error)
}

type Covers interface {
	GetBySerial(ctx context.Context, serial string) (entity.Cover, error)
	Upsert(ctx context.Context, c entity.Cover) error
	MissingSerials(ctx context.Context, serials []string) ([]string, error)
}

type Cover struct {
	games    Games
	covers   Covers
	igdb     *igdb.Client
	coverDir string
}

func New(games Games, covers Covers, client *igdb.Client, coverDir string) *Cover {
	return &Cover{games: games, covers: covers, igdb: client, coverDir: coverDir}
}

func (c *Cover) Enabled() bool {
	return c.igdb.Enabled()
}

func (c *Cover) Get(ctx context.Context, serial string) (entity.Cover, error) {
	return c.covers.GetBySerial(ctx, serial)
}

func (c *Cover) imagePath(serial string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, serial)
	return filepath.Join(c.coverDir, safe+".jpg")
}

func (c *Cover) Backfill(ctx context.Context, limit int) (int, error) {
	if c.igdb == nil || !c.Enabled() {
		return 0, fmt.Errorf("cover art disabled, set IGDB_CLIENT_ID and IGDB_CLIENT_SECRET")
	}
	games, err := c.games.List(ctx)
	if err != nil {
		return 0, err
	}
	serials := make([]string, 0, len(games))
	titles := map[string]string{}
	for _, g := range games {
		serials = append(serials, g.Serial)
		titles[g.Serial] = g.Title
	}
	missing, err := c.covers.MissingSerials(ctx, serials)
	if err != nil {
		return 0, err
	}
	platformID, err := c.igdb.PlatformID("PlayStation 2")
	if err != nil {
		return 0, err
	}
	done := 0
	for _, serial := range missing {
		if limit > 0 && done >= limit {
			break
		}
		if ctx.Err() != nil {
			return done, ctx.Err()
		}
		if err := c.fetchOne(ctx, serial, titles[serial], platformID); err != nil {
			continue
		}
		done++
		time.Sleep(300 * time.Millisecond)
	}
	return done, nil
}

func (c *Cover) fetchOne(ctx context.Context, serial, title string, platformID int64) error {
	results, err := c.igdb.Search(igdb.CleanTitle(title), platformID)
	if err != nil || len(results) == 0 {
		return fmt.Errorf("no match: %w", err)
	}
	var picked *igdb.Game
	for i := range results {
		if results[i].CoverURL != "" {
			picked = &results[i]
			break
		}
	}
	if picked == nil {
		return fmt.Errorf("no cover art")
	}
	dest := c.imagePath(serial)
	if err := c.igdb.Download(picked.CoverURL, dest); err != nil {
		return err
	}
	return c.covers.Upsert(ctx, entity.Cover{Serial: serial, Provider: "igdb", ProviderID: picked.ID, ImagePath: dest})
}
