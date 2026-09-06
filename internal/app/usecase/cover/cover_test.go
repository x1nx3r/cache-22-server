package coverusecase

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/x1nx3r/cache-22-server/internal/entity"
	"github.com/x1nx3r/cache-22-server/internal/infra/igdb"
)

type fakeGames struct {
	games []entity.Game
}

func (f *fakeGames) List(_ context.Context) ([]entity.Game, error) {
	return f.games, nil
}

type fakeCovers struct {
	mu     sync.Mutex
	stored map[string]entity.Cover
}

func (f *fakeCovers) GetBySerial(_ context.Context, serial string) (entity.Cover, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.stored[serial]
	if !ok {
		return entity.Cover{}, sql.ErrNoRows
	}
	return c, nil
}

func (f *fakeCovers) Upsert(_ context.Context, c entity.Cover) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stored == nil {
		f.stored = map[string]entity.Cover{}
	}
	c.FetchedAt = time.Now()
	f.stored[c.Serial] = c
	return nil
}

func (f *fakeCovers) MissingSerials(_ context.Context, serials []string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, s := range serials {
		if _, ok := f.stored[s]; !ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func fakeIGDBServer(t *testing.T) *igdb.Client {
	t.Helper()
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	})
	mux.HandleFunc("/platforms", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":7,"name":"PlayStation 2","abbreviation":"PS2"}]`))
	})
	mux.HandleFunc("/games", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":42,"name":"Some Game","cover":{"url":"` + base + `/t_thumb/x.jpg"}}]`))
	})
	mux.HandleFunc("/t_cover_big/x.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake-jpeg"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base = srv.URL
	return igdb.NewWithBases("id", "secret", srv.URL, srv.URL)
}

func TestBackfill(t *testing.T) {
	dir := t.TempDir()
	games := &fakeGames{games: []entity.Game{
		{Serial: "G1", Title: "Some Game (USA)"},
		{Serial: "G2", Title: "No Art Game (USA)"},
	}}
	covers := &fakeCovers{}
	svc := New(games, covers, fakeIGDBServer(t), dir)

	n, err := svc.Backfill(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("fetched = %d, want 2", n)
	}
	c, err := svc.Get(context.Background(), "G1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Provider != "igdb" || c.ProviderID != 42 {
		t.Errorf("cover = %+v", c)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "G1.jpg"))
	if err != nil || string(raw) != "fake-jpeg" {
		t.Errorf("image = %q, %v", raw, err)
	}

	n2, err := svc.Backfill(context.Background(), 0)
	if err != nil || n2 != 0 {
		t.Errorf("second backfill = %d, %v (nothing missing)", n2, err)
	}

	n3, err := svc.Backfill(context.Background(), 1)
	if err != nil || n3 != 0 {
		t.Errorf("limited backfill = %d, %v", n3, err)
	}
}

func TestBackfillDisabled(t *testing.T) {
	svc := New(&fakeGames{}, &fakeCovers{}, igdb.New("", ""), t.TempDir())
	if _, err := svc.Backfill(context.Background(), 0); err == nil {
		t.Error("want disabled error")
	}
	var nilSvc = New(&fakeGames{}, &fakeCovers{}, nil, t.TempDir())
	if _, err := nilSvc.Backfill(context.Background(), 0); err == nil {
		t.Error("want disabled error for nil client")
	}
}
