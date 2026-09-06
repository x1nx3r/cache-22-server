package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cache-22/cache-22-server/internal/entity"
	"github.com/cache-22/cache-22-server/internal/infra/scanner"
)

type stubRepo struct{}

func (stubRepo) List(_ context.Context) ([]entity.Game, error) { return nil, nil }

func (stubRepo) GetBySerial(_ context.Context, _ string) (entity.Game, error) {
	return entity.Game{}, nil
}

func (stubRepo) Upsert(_ context.Context, _ entity.Game) error { return nil }

func newUploadServer(t *testing.T) (*httptest.Server, *fakeUC, string, string) {
	t.Helper()
	dir := t.TempDir()
	uc := &fakeUC{games: map[string]entity.Game{}}
	authSvc, adminTok, _ := newTestAuth(t)
	h := New(uc, scanner.New(dir, stubRepo{}), authSvc, nilCover(t))
	mux := http.NewServeMux()
	h.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, uc, dir, adminTok
}

func uploadRequest(t *testing.T, url, token, filename string, content []byte, title string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if content != nil {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if title != "" {
		if err := mw.WriteField("title", title); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestUploadSuccess(t *testing.T) {
	srv, uc, dir, adminTok := newUploadServer(t)
	content := []byte("fake-iso-bytes-1234")

	res, err := http.DefaultClient.Do(uploadRequest(t, srv.URL+"/v1/games/upload", adminTok, "My Game.iso", content, "My Game"))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 201 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, want 201: %s", res.StatusCode, b)
	}
	var g entity.Game
	if err := json.NewDecoder(res.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.Serial != "My Game" || g.Title != "My Game" {
		t.Errorf("game = %+v", g)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "My Game.iso"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, content) {
		t.Error("saved bytes differ from upload")
	}
	if _, ok := uc.games["My Game"]; !ok {
		t.Error("game not registered in usecase")
	}
}

func TestUploadAuth(t *testing.T) {
	srv, _, _, adminTok := newUploadServer(t)
	for name, tc := range map[string]struct {
		token string
		want  int
	}{
		"no header":   {"", 401},
		"wrong token": {"nope", 401},
	} {
		res, err := http.DefaultClient.Do(uploadRequest(t, srv.URL+"/v1/games/upload", tc.token, "g.iso", []byte("x"), ""))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Errorf("%s: status = %d, want %d", name, res.StatusCode, tc.want)
		}
	}
	res, err := http.DefaultClient.Do(uploadRequest(t, srv.URL+"/v1/games/upload", adminTok, "g.iso", []byte("x"), ""))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Errorf("admin upload = %d, want 201", res.StatusCode)
	}
}

func TestUploadRejects(t *testing.T) {
	srv, _, dir, adminTok := newUploadServer(t)

	res, err := http.DefaultClient.Do(uploadRequest(t, srv.URL+"/v1/games/upload", adminTok, "game.chd", []byte("x"), ""))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 415 {
		t.Errorf("chd status = %d, want 415", res.StatusCode)
	}

	if err := os.WriteFile(filepath.Join(dir, "Dup.iso"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := http.DefaultClient.Do(uploadRequest(t, srv.URL+"/v1/games/upload", adminTok, "Dup.iso", []byte("new"), ""))
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != 409 {
		t.Errorf("dup status = %d, want 409", res2.StatusCode)
	}
	kept, _ := os.ReadFile(filepath.Join(dir, "Dup.iso"))
	if string(kept) != "old" {
		t.Error("duplicate upload overwrote existing file")
	}

	req, _ := http.NewRequest("POST", srv.URL+"/v1/games/upload", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	res3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res3.Body.Close()
	if res3.StatusCode != 400 {
		t.Errorf("non-multipart status = %d, want 400", res3.StatusCode)
	}
}

func TestSanitizeSerial(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"game.iso", "game", true},
		{"My Game (USA).iso", "My Game (USA)", true},
		{"../evil.iso", "evil", true},
		{"", "", false},
		{".iso", "", false},
		{"a/b.iso", "b", true},
		{"..", "", false},
		{string(make([]byte, 200)) + ".iso", "", false},
	}
	for _, c := range cases {
		got, ok := sanitizeSerial(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("sanitizeSerial(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
