package httphandler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	coverusecase "github.com/cache-22/cache-22-server/internal/app/usecase/cover"
	"github.com/cache-22/cache-22-server/internal/entity"
	"github.com/cache-22/cache-22-server/internal/infra/scanner"
)

type fakeCoverStore struct {
	mu     sync.Mutex
	stored map[string]entity.Cover
}

func (f *fakeCoverStore) GetBySerial(_ context.Context, serial string) (entity.Cover, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.stored[serial]
	if !ok {
		return entity.Cover{}, sql.ErrNoRows
	}
	return c, nil
}

func (f *fakeCoverStore) Upsert(_ context.Context, c entity.Cover) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stored == nil {
		f.stored = map[string]entity.Cover{}
	}
	f.stored[c.Serial] = c
	return nil
}

func (f *fakeCoverStore) MissingSerials(_ context.Context, serials []string) ([]string, error) {
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

func TestServeCover(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "G.jpg")
	if err := os.WriteFile(img, []byte("fake-jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	covers := &fakeCoverStore{stored: map[string]entity.Cover{
		"G": {Serial: "G", Provider: "igdb", ProviderID: 1, ImagePath: img},
	}}
	authSvc, _, playerTok := newTestAuth(t)
	h := New(&fakeUC{}, scanner.New(t.TempDir(), fakeRepo{}), authSvc,
		coverusecase.New(nilGames{}, covers, nil, dir))
	mux := http.NewServeMux()
	h.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	anon, err := http.Get(srv.URL + "/v1/games/G/cover")
	if err != nil {
		t.Fatal(err)
	}
	anon.Body.Close()
	if anon.StatusCode != 401 {
		t.Errorf("anonymous cover = %d, want 401", anon.StatusCode)
	}

	res := authed(t, "GET", srv.URL+"/v1/games/G/cover", playerTok)
	defer res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("cover = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}

	missing := authed(t, "GET", srv.URL+"/v1/games/NOPE/cover", playerTok)
	missing.Body.Close()
	if missing.StatusCode != 404 {
		t.Errorf("missing cover = %d, want 404", missing.StatusCode)
	}
}

func TestAdminBackfillCovers(t *testing.T) {
	authSvc, adminTok, playerTok := newTestAuth(t)
	h := New(&fakeUC{}, scanner.New(t.TempDir(), fakeRepo{}), authSvc, nilCover(t))
	mux := http.NewServeMux()
	h.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	anon, err := http.Post(srv.URL+"/v1/admin/covers/backfill", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	anon.Body.Close()
	if anon.StatusCode != 401 {
		t.Errorf("anonymous backfill = %d, want 401", anon.StatusCode)
	}

	player := authed(t, "POST", srv.URL+"/v1/admin/covers/backfill", playerTok)
	player.Body.Close()
	if player.StatusCode != 403 {
		t.Errorf("player backfill = %d, want 403", player.StatusCode)
	}

	admin := authed(t, "POST", srv.URL+"/v1/admin/covers/backfill", adminTok)
	defer admin.Body.Close()
	if admin.StatusCode != 500 {
		t.Errorf("disabled backfill = %d, want 500", admin.StatusCode)
	}
}
