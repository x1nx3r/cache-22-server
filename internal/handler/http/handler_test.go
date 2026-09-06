package httphandler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	coverusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/cover"
	gameusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/game"
	saveusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/save"
	"github.com/x1nx3r/cache-22-server/internal/entity"
	"github.com/x1nx3r/cache-22-server/internal/infra/scanner"
)

type fakeUC struct {
	games    map[string]entity.Game
	manifest map[string]entity.Manifest
	healthOK bool
}

func (f *fakeUC) ListGames(_ context.Context) ([]entity.Game, error) {
	var out []entity.Game
	for _, g := range f.games {
		out = append(out, g)
	}
	if out == nil {
		out = []entity.Game{}
	}
	return out, nil
}

func (f *fakeUC) GetGame(_ context.Context, serial string) (entity.Game, error) {
	g, ok := f.games[serial]
	if !ok {
		return entity.Game{}, sql.ErrNoRows
	}
	return g, nil
}

func (f *fakeUC) GetManifest(_ context.Context, serial string) (entity.Manifest, error) {
	m, ok := f.manifest[serial]
	if !ok {
		return entity.Manifest{}, sql.ErrNoRows
	}
	return m, nil
}

func (f *fakeUC) HealthCheck(_ context.Context, _ string) (*entity.HealthCheck, error) {
	if !f.healthOK {
		return nil, sql.ErrNoRows
	}
	return &entity.HealthCheck{Status: "ok", Message: "service is healthy"}, nil
}

func (f *fakeUC) RegisterGame(_ context.Context, g entity.Game) error {
	if f.games == nil {
		f.games = map[string]entity.Game{}
	}
	f.games[g.Serial] = g
	return nil
}

var _ gameusecase.UseCase = (*fakeUC)(nil)

type fakeRepo struct{}

func (fakeRepo) List(_ context.Context) ([]entity.Game, error) { return nil, nil }

func (fakeRepo) GetBySerial(_ context.Context, _ string) (entity.Game, error) {
	return entity.Game{}, sql.ErrNoRows
}

func (fakeRepo) Upsert(_ context.Context, _ entity.Game) error { return nil }

func newTestServer(t *testing.T, uc gameusecase.UseCase) (*httptest.Server, string, string) {
	t.Helper()
	authSvc, adminTok, playerTok := newTestAuth(t)
	sc := scanner.New(t.TempDir(), fakeRepo{})
	h := New(uc, sc, authSvc, nilCover(t), nilSaves(t))
	mux := http.NewServeMux()
	h.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, adminTok, playerTok
}

func nilCover(t *testing.T) *coverusecase.Cover {
	t.Helper()
	return coverusecase.New(nilGames{}, nilCovers{}, nil, t.TempDir())
}

type stubSaveStore struct {
	rows map[string]entity.Save
}

func saveKey(userID int64, serial string, slot int) string {
	return string(rune(userID)) + "|" + serial + "|" + string(rune(slot))
}

func (s *stubSaveStore) Get(_ context.Context, userID int64, serial string, slot int) (entity.Save, error) {
	m, ok := s.rows[saveKey(userID, serial, slot)]
	if !ok {
		return entity.Save{}, sql.ErrNoRows
	}
	return m, nil
}

func (s *stubSaveStore) Upsert(_ context.Context, m entity.Save) error {
	s.rows[saveKey(m.UserID, m.Serial, m.Slot)] = m
	return nil
}

func nilSaves(t *testing.T) *saveusecase.Save {
	t.Helper()
	return saveusecase.New(&stubSaveStore{rows: map[string]entity.Save{}}, t.TempDir())
}

func nilSavesWithDir(t *testing.T, dir string) *saveusecase.Save {
	t.Helper()
	return saveusecase.New(&stubSaveStore{rows: map[string]entity.Save{}}, filepath.Join(dir, "saves"))
}

type nilGames struct{}

func (nilGames) List(_ context.Context) ([]entity.Game, error) { return nil, nil }

type nilCovers struct{}

func (nilCovers) GetBySerial(_ context.Context, _ string) (entity.Cover, error) {
	return entity.Cover{}, sql.ErrNoRows
}

func (nilCovers) Upsert(_ context.Context, _ entity.Cover) error { return nil }

func (nilCovers) MissingSerials(_ context.Context, _ []string) ([]string, error) {
	return nil, nil
}

func authed(t *testing.T, method, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func writeSizedFile(t *testing.T, path string, size int, fill byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = fill
	}
	if _, err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
}

func TestHealth(t *testing.T) {
	srv, _, _ := newTestServer(t, &fakeUC{healthOK: true})
	res, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	var body entity.HealthCheck
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q", body.Status)
	}
}

func TestListAndGetGame(t *testing.T) {
	uc := &fakeUC{games: map[string]entity.Game{
		"A": {Serial: "A", Title: "A"},
	}}
	srv, _, playerTok := newTestServer(t, uc)

	anon, err := http.Get(srv.URL + "/v1/games")
	if err != nil {
		t.Fatal(err)
	}
	anon.Body.Close()
	if anon.StatusCode != 401 {
		t.Errorf("anonymous games = %d, want 401", anon.StatusCode)
	}

	res := authed(t, "GET", srv.URL+"/v1/games", playerTok)
	defer res.Body.Close()
	var list struct {
		Games []entity.Game `json:"games"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Games) != 1 {
		t.Errorf("games = %d, want 1", len(list.Games))
	}

	res2 := authed(t, "GET", srv.URL+"/v1/games/A", playerTok)
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		t.Errorf("get status = %d, want 200", res2.StatusCode)
	}

	res3 := authed(t, "GET", srv.URL+"/v1/games/NOPE", playerTok)
	defer res3.Body.Close()
	if res3.StatusCode != 404 {
		t.Errorf("missing status = %d, want 404", res3.StatusCode)
	}
}

func TestServeFile(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "game.iso")
	writeSizedFile(t, iso, 3<<20, 0xAB)
	chd := filepath.Join(dir, "game.chd")
	writeSizedFile(t, chd, 16, 0)

	uc := &fakeUC{games: map[string]entity.Game{
		"ISO": {Serial: "ISO", RedumpHash: "h", SizeBytes: 3 << 20, FilePath: iso},
		"CHD": {Serial: "CHD", SizeBytes: 16, FilePath: chd},
	}}
	srv, _, playerTok := newTestServer(t, uc)

	res := authed(t, "GET", srv.URL+"/v1/files/ISO", playerTok)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if res.Header.Get("Accept-Ranges") != "bytes" || res.Header.Get("ETag") == "" {
		t.Errorf("missing range headers: %v", res.Header)
	}
	if res.ContentLength != 3<<20 {
		t.Errorf("length = %d, want %d", res.ContentLength, 3<<20)
	}

	req, _ := http.NewRequest("GET", srv.URL+"/v1/files/ISO", nil)
	req.Header.Set("Range", "bytes=0-1048575")
	req.Header.Set("Authorization", "Bearer "+playerTok)
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != 206 {
		t.Errorf("range status = %d, want 206", res2.StatusCode)
	}
	if res2.Header.Get("Content-Range") != "bytes 0-1048575/3145728" {
		t.Errorf("Content-Range = %q", res2.Header.Get("Content-Range"))
	}

	for path, want := range map[string]int{
		"/v1/files/CHD":  415,
		"/v1/files/NOPE": 404,
	} {
		r := authed(t, "GET", srv.URL+path, playerTok)
		r.Body.Close()
		if r.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, r.StatusCode, want)
		}
	}
}

func TestAPIScanGated(t *testing.T) {
	srv, adminTok, playerTok := newTestServer(t, &fakeUC{})

	res := authed(t, "POST", srv.URL+"/v1/scan", "")
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Errorf("anonymous scan = %d, want 401", res.StatusCode)
	}

	resP := authed(t, "POST", srv.URL+"/v1/scan", playerTok)
	resP.Body.Close()
	if resP.StatusCode != 403 {
		t.Errorf("player scan = %d, want 403", resP.StatusCode)
	}

	res2 := authed(t, "POST", srv.URL+"/v1/scan", adminTok)
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		t.Errorf("admin scan = %d, want 200", res2.StatusCode)
	}
}

func TestServeManifest(t *testing.T) {
	uc := &fakeUC{manifest: map[string]entity.Manifest{
		"A": {Serial: "A", BootRanges: [][2]int64{{0, 1 << 20}}, Supported: true},
		"C": {Serial: "C", Supported: false},
	}}
	srv, _, playerTok := newTestServer(t, uc)

	res := authed(t, "GET", srv.URL+"/v1/games/A/manifest.json", playerTok)
	defer res.Body.Close()
	var m entity.Manifest
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Serial != "A" || len(m.BootRanges) != 1 {
		t.Errorf("manifest = %+v", m)
	}

	res2 := authed(t, "GET", srv.URL+"/v1/games/C/manifest.json", playerTok)
	defer res2.Body.Close()
	var m2 entity.Manifest
	if err := json.NewDecoder(res2.Body).Decode(&m2); err != nil {
		t.Fatal(err)
	}
	if m2.UnsupportedHint == "" {
		t.Error("unsupported manifest must carry a hint")
	}

	res3 := authed(t, "GET", srv.URL+"/v1/games/NOPE/manifest.json", playerTok)
	defer res3.Body.Close()
	if res3.StatusCode != 404 {
		t.Errorf("missing status = %d, want 404", res3.StatusCode)
	}
}

func TestAuthEndpoints(t *testing.T) {
	srv, adminTok, playerTok := newTestServer(t, &fakeUC{healthOK: true})
	post := func(url, token string, body any) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req, err := http.NewRequest("POST", url, &buf)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := post(srv.URL+"/v1/auth/setup", "", map[string]string{"username": "x", "password": "password1"})
	res.Body.Close()
	if res.StatusCode != 409 {
		t.Errorf("setup after seed = %d, want 409", res.StatusCode)
	}

	bad := post(srv.URL+"/v1/auth/login", "", map[string]string{"username": "admin", "password": "wrong"})
	bad.Body.Close()
	if bad.StatusCode != 401 {
		t.Errorf("bad login = %d, want 401", bad.StatusCode)
	}

	me := authed(t, "GET", srv.URL+"/v1/auth/me", playerTok)
	defer me.Body.Close()
	var meBody map[string]any
	if err := json.NewDecoder(me.Body).Decode(&meBody); err != nil {
		t.Fatal(err)
	}
	if meBody["username"] != "player" || meBody["is_admin"] != false {
		t.Errorf("me = %v", meBody)
	}

	logout := authed(t, "POST", srv.URL+"/v1/auth/logout", playerTok)
	logout.Body.Close()
	if logout.StatusCode != 200 {
		t.Errorf("logout = %d, want 200", logout.StatusCode)
	}
	dead := authed(t, "GET", srv.URL+"/v1/auth/me", playerTok)
	dead.Body.Close()
	if dead.StatusCode != 401 {
		t.Errorf("logged-out me = %d, want 401", dead.StatusCode)
	}

	fresh := post(srv.URL+"/v1/auth/login", "", map[string]string{"username": "player", "password": "password1"})
	defer fresh.Body.Close()
	var freshBody map[string]any
	if err := json.NewDecoder(fresh.Body).Decode(&freshBody); err != nil {
		t.Fatal(err)
	}
	playerTok2, _ := freshBody["token"].(string)
	users := authed(t, "GET", srv.URL+"/v1/admin/users", playerTok2)
	users.Body.Close()
	if users.StatusCode != 403 {
		t.Errorf("player admin list = %d, want 403", users.StatusCode)
	}

	created := post(srv.URL+"/v1/admin/users", adminTok, map[string]any{"username": "new", "password": "password1"})
	defer created.Body.Close()
	if created.StatusCode != 201 {
		t.Errorf("create = %d, want 201", created.StatusCode)
	}
	var createdBody map[string]any
	if err := json.NewDecoder(created.Body).Decode(&createdBody); err != nil {
		t.Fatal(err)
	}
	newID := int(createdBody["id"].(float64))

	pw := post(srv.URL+"/v1/admin/users/999/password", adminTok, map[string]string{"password": "password22"})
	pw.Body.Close()
	if pw.StatusCode != 200 {
		t.Errorf("reset missing user = %d (no row check, ok)", pw.StatusCode)
	}

	delSelf := authed(t, "DELETE", srv.URL+"/v1/admin/users/1", adminTok)
	delSelf.Body.Close()
	if delSelf.StatusCode != 400 {
		t.Errorf("delete self = %d, want 400", delSelf.StatusCode)
	}

	del := authed(t, "DELETE", srv.URL+fmt.Sprintf("/v1/admin/users/%d", newID), adminTok)
	del.Body.Close()
	if del.StatusCode != 200 {
		t.Errorf("delete = %d, want 200", del.StatusCode)
	}
}
