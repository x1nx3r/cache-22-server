package webhandler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gameusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/game"
	"github.com/x1nx3r/cache-22-server/internal/entity"
	"github.com/x1nx3r/cache-22-server/internal/infra/scanner"
	"github.com/x1nx3r/cache-22-server/internal/testutil"
)

type fakeUC struct {
	games    map[string]entity.Game
	manifest map[string]entity.Manifest
	repo     *fakeRepo
}

func (f *fakeUC) ListGames(_ context.Context) ([]entity.Game, error) {
	if f.repo != nil {
		var out []entity.Game
		for _, g := range f.repo.stored {
			out = append(out, g)
		}
		if out == nil {
			out = []entity.Game{}
		}
		return out, nil
	}
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
	return &entity.HealthCheck{Status: "ok"}, nil
}

func (f *fakeUC) RegisterGame(_ context.Context, _ entity.Game) error {
	return nil
}

var _ gameusecase.UseCase = (*fakeUC)(nil)

type fakeRepo struct{ stored map[string]entity.Game }

func (f *fakeRepo) List(_ context.Context) ([]entity.Game, error) { return nil, nil }

func (f *fakeRepo) GetBySerial(_ context.Context, _ string) (entity.Game, error) {
	return entity.Game{}, sql.ErrNoRows
}

func (f *fakeRepo) Upsert(_ context.Context, g entity.Game) error {
	if f.stored == nil {
		f.stored = map[string]entity.Game{}
	}
	f.stored[g.Serial] = g
	return nil
}

func newMux(t *testing.T, uc gameusecase.UseCase, libDir string) *http.ServeMux {
	t.Helper()
	authSvc, _, _ := testutil.NewAuth(t, "setup-secret")
	h := New(uc, scanner.New(libDir, &fakeRepo{}), authSvc, false)
	mux := http.NewServeMux()
	h.Routes(mux)
	return mux
}

func login(t *testing.T, mux *http.ServeMux, username, password string) []*http.Cookie {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}.Encode()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303", res.Code)
	}
	return res.Result().Cookies()
}

func TestLibraryPage(t *testing.T) {
	mux := newMux(t, &fakeUC{games: map[string]entity.Game{
		"A": {Serial: "A", Title: "Alpha", SizeBytes: 1 << 20},
	}}, t.TempDir())

	anon := httptest.NewRecorder()
	mux.ServeHTTP(anon, httptest.NewRequest("GET", "/", nil))
	if anon.Code != http.StatusSeeOther {
		t.Errorf("anonymous library = %d, want redirect to login", anon.Code)
	}

	cookies := login(t, mux, "player", "password1")
	res := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	mux.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("status = %d", res.Code)
	}
	body := res.Body.String()
	for _, want := range []string{"Alpha", "/games/A", "1.0 MB", "htmx.min.js", "app.css"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if !strings.Contains(body, `hx-post="/scan"`) {
		t.Error("logged-in page must show Rescan button")
	}
}

func TestLibraryEmpty(t *testing.T) {
	mux := newMux(t, &fakeUC{}, t.TempDir())
	cookies := login(t, mux, "player", "password1")
	res := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	mux.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "No games found") {
		t.Error("empty library must show hint")
	}
}

func TestDetailPage(t *testing.T) {
	uc := &fakeUC{
		games: map[string]entity.Game{
			"A": {Serial: "A", Title: "Alpha", SizeBytes: 2 << 20, RedumpHash: "h"},
		},
		manifest: map[string]entity.Manifest{
			"A": {Serial: "A", BootRanges: [][2]int64{{0, 1 << 20}}, Supported: true, ManifestURL: "/m", FileURL: "/f"},
		},
	}
	mux := newMux(t, uc, t.TempDir())
	cookies := login(t, mux, "player", "password1")
	withCookies := func(r *http.Request) *http.Request {
		for _, c := range cookies {
			r.AddCookie(c)
		}
		return r
	}

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, withCookies(httptest.NewRequest("GET", "/games/A", nil)))
	if res.Code != 200 {
		t.Fatalf("status = %d", res.Code)
	}
	for _, want := range []string{"Alpha", "2.0 MB", "Raw ISO"} {
		if !strings.Contains(res.Body.String(), want) {
			t.Errorf("detail missing %q", want)
		}
	}

	res2 := httptest.NewRecorder()
	mux.ServeHTTP(res2, withCookies(httptest.NewRequest("GET", "/games/NOPE", nil)))
	if res2.Code != 404 {
		t.Errorf("missing game status = %d, want 404", res2.Code)
	}

	res3 := httptest.NewRecorder()
	mux.ServeHTTP(res3, httptest.NewRequest("GET", "/games/A", nil))
	if res3.Code != http.StatusSeeOther {
		t.Errorf("anonymous detail = %d, want redirect", res3.Code)
	}
}

func TestScanPartial(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "GAME.iso"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	repo := &fakeRepo{}
	authSvc, _, _ := testutil.NewAuth(t, "setup-secret")
	h := New(&fakeUC{repo: repo}, scanner.New(dir, repo), authSvc, false)
	mux := http.NewServeMux()
	h.Routes(mux)

	jar := login(t, mux, "admin", "password1")

	req := httptest.NewRequest("POST", "/scan", nil)
	for _, c := range jar {
		req.AddCookie(c)
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("status = %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "GAME") {
		t.Errorf("scan partial must list scanned game, got: %s", res.Body.String())
	}
	if res.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("content-type = %q", res.Header().Get("Content-Type"))
	}
}
func TestLoginFlow(t *testing.T) {
	mux := newMux(t, &fakeUC{}, t.TempDir())

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest("GET", "/login", nil))
	if res.Code != 200 || strings.Contains(res.Body.String(), "First run setup") {
		t.Fatalf("login page status = %d", res.Code)
	}

	bad := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"username": {"player"}, "password": {"nope"}}.Encode()))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resBad := httptest.NewRecorder()
	mux.ServeHTTP(resBad, bad)
	if resBad.Code != 401 || !strings.Contains(resBad.Body.String(), "Wrong username or password") {
		t.Errorf("bad login status = %d", resBad.Code)
	}

	cookies := login(t, mux, "player", "password1")

	authed := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		authed.AddCookie(c)
	}
	resLib := httptest.NewRecorder()
	mux.ServeHTTP(resLib, authed)
	if !strings.Contains(resLib.Body.String(), `hx-post="/scan"`) {
		t.Error("logged-in library must show Rescan button")
	}
	if strings.Contains(resLib.Body.String(), "/admin/users") {
		t.Error("player must not see Users nav")
	}

	adminCookies := login(t, mux, "admin", "password1")
	resAdmin := httptest.NewRequest("GET", "/", nil)
	for _, c := range adminCookies {
		resAdmin.AddCookie(c)
	}
	resAdminRec := httptest.NewRecorder()
	mux.ServeHTTP(resAdminRec, resAdmin)
	if !strings.Contains(resAdminRec.Body.String(), "/admin/users") {
		t.Error("admin must see Users nav")
	}
	if !strings.Contains(resAdminRec.Body.String(), "Add game") {
		t.Error("admin must see upload tile")
	}

	reqOut := httptest.NewRequest("POST", "/logout", nil)
	for _, c := range cookies {
		reqOut.AddCookie(c)
	}
	resOut := httptest.NewRecorder()
	mux.ServeHTTP(resOut, reqOut)
	if resOut.Code != http.StatusSeeOther {
		t.Errorf("logout status = %d, want 303", resOut.Code)
	}
	reuse := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		reuse.AddCookie(c)
	}
	resReuse := httptest.NewRecorder()
	mux.ServeHTTP(resReuse, reuse)
	if resReuse.Code == 200 {
		t.Error("logged-out session must not stay valid")
	}
}

func TestSetupFlow(t *testing.T) {
	authSvc := testutil.NewAuthFresh(t, "setup-secret")
	h := New(&fakeUC{}, scanner.New(t.TempDir(), &fakeRepo{}), authSvc, false)
	mux := http.NewServeMux()
	h.Routes(mux)

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest("GET", "/login", nil))
	if !strings.Contains(res.Body.String(), "First run setup") {
		t.Fatal("fresh server must show setup form")
	}

	bad := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{
		"setup_token": {"wrong"}, "username": {"boss"}, "password": {"password1"},
	}.Encode()))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resBad := httptest.NewRecorder()
	mux.ServeHTTP(resBad, bad)
	if resBad.Code != 401 {
		t.Errorf("bad setup token = %d, want 401", resBad.Code)
	}

	good := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{
		"setup_token": {"setup-secret"}, "username": {"boss"}, "password": {"password1"},
	}.Encode()))
	good.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resGood := httptest.NewRecorder()
	mux.ServeHTTP(resGood, good)
	if resGood.Code != http.StatusSeeOther {
		t.Fatalf("setup = %d, want 303", resGood.Code)
	}
	var sess *http.Cookie
	for _, c := range resGood.Result().Cookies() {
		if c.Name == sessionCookie {
			sess = c
		}
	}
	if sess == nil {
		t.Fatal("setup must set session cookie")
	}
	users := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/users", nil)
	req.AddCookie(sess)
	mux.ServeHTTP(users, req)
	if users.Code != 200 || !strings.Contains(users.Body.String(), "boss") {
		t.Errorf("admin page = %d", users.Code)
	}
}

func TestAdminUsersPage(t *testing.T) {
	mux := newMux(t, &fakeUC{}, t.TempDir())

	anon := httptest.NewRecorder()
	mux.ServeHTTP(anon, httptest.NewRequest("GET", "/admin/users", nil))
	if anon.Code != http.StatusSeeOther {
		t.Errorf("anonymous users page = %d, want redirect", anon.Code)
	}

	playerCookies := login(t, mux, "player", "password1")
	reqP := httptest.NewRequest("GET", "/admin/users", nil)
	for _, c := range playerCookies {
		reqP.AddCookie(c)
	}
	resP := httptest.NewRecorder()
	mux.ServeHTTP(resP, reqP)
	if resP.Code != http.StatusForbidden {
		t.Errorf("player users page = %d, want 403", resP.Code)
	}

	adminCookies := login(t, mux, "admin", "password1")
	reqA := httptest.NewRequest("GET", "/admin/users", nil)
	for _, c := range adminCookies {
		reqA.AddCookie(c)
	}
	resA := httptest.NewRecorder()
	mux.ServeHTTP(resA, reqA)
	if resA.Code != 200 || !strings.Contains(resA.Body.String(), "player") {
		t.Errorf("admin users page = %d", resA.Code)
	}

	form := url.Values{"username": {"temp"}, "password": {"password1"}}.Encode()
	reqC := httptest.NewRequest("POST", "/admin/users", strings.NewReader(form))
	reqC.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range adminCookies {
		reqC.AddCookie(c)
	}
	resC := httptest.NewRecorder()
	mux.ServeHTTP(resC, reqC)
	if resC.Code != http.StatusSeeOther {
		t.Errorf("create user = %d, want 303", resC.Code)
	}
}

func TestWebScanGated(t *testing.T) {
	mux := newMux(t, &fakeUC{}, t.TempDir())

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest("POST", "/scan", nil))
	if res.Code != 401 {
		t.Errorf("anonymous scan status = %d, want 401", res.Code)
	}
	if res.Header().Get("HX-Redirect") != "/login" {
		t.Errorf("HX-Redirect = %q, want /login", res.Header().Get("HX-Redirect"))
	}
}
