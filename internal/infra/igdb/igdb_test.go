package igdb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeIGDB(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	})
	mux.HandleFunc("/platforms", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":7,"name":"PlayStation 2","abbreviation":"PS2"}]`))
	})
	mux.HandleFunc("/games", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":123,"name":"Ace Combat Zero: The Belkan War","cover":{"url":"//images.igdb.com/igdb/image/upload/t_thumb/abc.jpg"}}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewWithBases("id", "secret", srv.URL, srv.URL)
}

func TestTokenCached(t *testing.T) {
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBases("id", "secret", srv.URL, srv.URL)
	c.mu.Lock()
	t1, err := c.tokenLocked()
	c.mu.Unlock()
	if err != nil || t1 != "tok" {
		t.Fatalf("token = %q, %v", t1, err)
	}
	c.mu.Lock()
	t2, _ := c.tokenLocked()
	c.mu.Unlock()
	if t2 != "tok" || calls != 1 {
		t.Errorf("token not cached: %q calls=%d", t2, calls)
	}
}

func TestPlatformAndSearch(t *testing.T) {
	c := fakeIGDB(t)
	id, err := c.PlatformID("PlayStation 2")
	if err != nil || id != 7 {
		t.Fatalf("platform = %d, %v", id, err)
	}
	games, err := c.Search("Ace Combat Zero The Belkan War", id)
	if err != nil || len(games) != 1 {
		t.Fatalf("search = %+v, %v", games, err)
	}
	if games[0].CoverURL != "https://images.igdb.com/igdb/image/upload/t_cover_big/abc.jpg" {
		t.Errorf("cover = %q", games[0].CoverURL)
	}
}

func TestCleanTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Ace Combat Zero - The Belkan War (USA)": "Ace Combat Zero - The Belkan War",
		"Game (Europe) (En,Fr)":                  "Game",
		"NoSuffix":                               "NoSuffix",
		"Half (Life":                             "Half (Life",
	} {
		if got := CleanTitle(in); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCoverBig(t *testing.T) {
	if got := CoverBig("//images.igdb.com/igdb/image/upload/t_thumb/x.jpg"); got != "https://images.igdb.com/igdb/image/upload/t_cover_big/x.jpg" {
		t.Errorf("got %q", got)
	}
}

func TestDisabled(t *testing.T) {
	c := New("", "")
	if c.Enabled() {
		t.Error("empty creds must disable")
	}
	var nilClient *Client
	if nilClient.Enabled() {
		t.Error("nil client must disable")
	}
}

func TestDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("jpeg-bytes"))
	}))
	t.Cleanup(srv.Close)
	c := fakeIGDB(t)
	dest := t.TempDir() + "/c.jpg"
	if err := c.Download(srv.URL+"/c.jpg", dest); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dest, ".jpg") {
		t.Error("dest name")
	}
}
