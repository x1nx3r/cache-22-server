package gameusecase

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/cache-22/cache-22-server/internal/entity"
)

type fakeGames struct {
	games map[string]entity.Game
	list  []entity.Game
	err   error
}

func newFakeGames(games ...entity.Game) *fakeGames {
	f := &fakeGames{games: map[string]entity.Game{}}
	for _, g := range games {
		f.games[g.Serial] = g
		f.list = append(f.list, g)
	}
	return f
}

func (f *fakeGames) List(_ context.Context) ([]entity.Game, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

func (f *fakeGames) GetBySerial(_ context.Context, serial string) (entity.Game, error) {
	if f.err != nil {
		return entity.Game{}, f.err
	}
	g, ok := f.games[serial]
	if !ok {
		return entity.Game{}, sql.ErrNoRows
	}
	return g, nil
}

func (f *fakeGames) Upsert(_ context.Context, g entity.Game) error {
	return f.err
}

type fakeHealth struct {
	status string
	err    error
}

func (f *fakeHealth) GetStatus(_ context.Context, _ string) (string, error) {
	return f.status, f.err
}

func TestListGames(t *testing.T) {
	uc := New(newFakeGames(
		entity.Game{Serial: "A", Title: "A"},
		entity.Game{Serial: "B", Title: "B"},
	), &fakeHealth{status: "ok"})

	games, err := uc.ListGames(context.Background())
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d games, want 2", len(games))
	}
}

func TestListGamesError(t *testing.T) {
	uc := New(&fakeGames{err: errors.New("db down")}, &fakeHealth{})
	if _, err := uc.ListGames(context.Background()); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestGetGame(t *testing.T) {
	uc := New(newFakeGames(entity.Game{Serial: "SLUS-1", Title: "Game"}), &fakeHealth{})

	g, err := uc.GetGame(context.Background(), "SLUS-1")
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if g.Serial != "SLUS-1" {
		t.Errorf("Serial = %q", g.Serial)
	}

	if _, err := uc.GetGame(context.Background(), "NOPE"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("missing game err = %v, want ErrNoRows", err)
	}
}

func TestRegisterGame(t *testing.T) {
	f := newFakeGames()
	uc := New(f, &fakeHealth{})
	if err := uc.RegisterGame(context.Background(), entity.Game{Serial: "N", Title: "N"}); err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}

	ucBad := New(&fakeGames{err: errors.New("db down")}, &fakeHealth{})
	if err := ucBad.RegisterGame(context.Background(), entity.Game{Serial: "N"}); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestHealthCheck(t *testing.T) {
	uc := New(newFakeGames(), &fakeHealth{status: "ok"})
	res, err := uc.HealthCheck(context.Background(), "http")
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if res.Status != "ok" || res.Message == "" {
		t.Errorf("unexpected result: %+v", res)
	}

	ucBad := New(newFakeGames(), &fakeHealth{err: errors.New("down")})
	if _, err := ucBad.HealthCheck(context.Background(), "http"); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestGetManifest(t *testing.T) {
	uc := New(newFakeGames(
		entity.Game{Serial: "SMALL", Title: "S", SizeBytes: 100, FilePath: "/x/small.iso", RedumpHash: "h1"},
		entity.Game{Serial: "MID", Title: "M", SizeBytes: 100 << 20, FilePath: "/x/mid.iso"},
		entity.Game{Serial: "BIG", Title: "B", SizeBytes: 4 << 30, FilePath: "/x/big.CHD"},
	), &fakeHealth{})

	small, err := uc.GetManifest(context.Background(), "SMALL")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if small.Version != 2 {
		t.Errorf("Version = %d, want 2", small.Version)
	}
	if small.BlockSize != entity.BlockSize {
		t.Errorf("BlockSize = %d", small.BlockSize)
	}
	if !small.Supported {
		t.Error("small.iso must be supported")
	}
	if small.FileURL != "/v1/files/SMALL" {
		t.Errorf("FileURL = %q", small.FileURL)
	}
	if len(small.BootRanges) != 1 || small.BootRanges[0] != [2]int64{0, 100} {
		t.Errorf("small BootRanges = %v, want full size clamp", small.BootRanges)
	}

	mid, _ := uc.GetManifest(context.Background(), "MID")
	if len(mid.BootRanges) != 1 || mid.BootRanges[0] != [2]int64{0, 64 << 20} {
		t.Errorf("mid BootRanges = %v, want 64MB", mid.BootRanges)
	}

	big, _ := uc.GetManifest(context.Background(), "BIG")
	if len(big.BootRanges) != 1 || big.BootRanges[0][1] != 64<<20 {
		t.Errorf("big BootRanges = %v", big.BootRanges)
	}
	if big.Supported {
		t.Error("big.CHD must not be supported (case-insensitive raw check)")
	}

	if _, err := uc.GetManifest(context.Background(), "NOPE"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("missing err = %v, want ErrNoRows", err)
	}
}

func TestIsRawISO(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/a/game.iso", true},
		{"/a/GAME.ISO", true},
		{"/a/game.chd", false},
		{"/a/game.cso", false},
		{"/a/game.zso", false},
		{"/a/game.iso.bak", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isRawISO(c.path); got != c.want {
			t.Errorf("isRawISO(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
