package httphandler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func saveReq(t *testing.T, method, url, token string, body []byte) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
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

func TestSaveRoundTrip(t *testing.T) {
	srv, _, dir, adminTok := newUploadServer(t)
	data := bytes.Repeat([]byte{1, 2, 3, 4}, 2048)
	sum := sha256.Sum256(data)
	wantSHA := hex.EncodeToString(sum[:])

	put := saveReq(t, "PUT", srv.URL+"/v1/saves/SLUS-1/1", adminTok, data)
	pb, _ := io.ReadAll(put.Body)
	put.Body.Close()
	if put.StatusCode != 200 {
		t.Fatalf("put = %d: %s", put.StatusCode, pb)
	}

	get := saveReq(t, "GET", srv.URL+"/v1/saves/SLUS-1/1", adminTok, nil)
	defer get.Body.Close()
	if get.StatusCode != 200 {
		t.Fatalf("get = %d", get.StatusCode)
	}
	if get.Header.Get("X-SHA256") != wantSHA {
		t.Errorf("sha header = %q, want %q", get.Header.Get("X-SHA256"), wantSHA)
	}
	got, err := io.ReadAll(get.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("save bytes differ")
	}
	if _, err := os.Stat(filepath.Join(dir, "saves", "1", "SLUS-1", "slot1.ps2")); err != nil {
		t.Errorf("save file missing on disk: %v", err)
	}
}

func TestSaveRejects(t *testing.T) {
	srv, _, _, adminTok := newUploadServer(t)

	missing := saveReq(t, "GET", srv.URL+"/v1/saves/NOPE/1", adminTok, nil)
	missing.Body.Close()
	if missing.StatusCode != 404 {
		t.Errorf("missing = %d, want 404", missing.StatusCode)
	}

	badSlot := saveReq(t, "PUT", srv.URL+"/v1/saves/SLUS-1/3", adminTok, []byte("x"))
	badSlot.Body.Close()
	if badSlot.StatusCode != 400 {
		t.Errorf("bad slot = %d, want 400", badSlot.StatusCode)
	}

	badSerial := saveReq(t, "PUT", srv.URL+"/v1/saves/.hidden/1", adminTok, []byte("x"))
	badSerial.Body.Close()
	if badSerial.StatusCode != 400 {
		t.Errorf("bad serial = %d, want 400", badSerial.StatusCode)
	}

	empty := saveReq(t, "PUT", srv.URL+"/v1/saves/SLUS-1/1", adminTok, nil)
	empty.Body.Close()
	if empty.StatusCode != 400 {
		t.Errorf("empty = %d, want 400", empty.StatusCode)
	}

	noauth := saveReq(t, "GET", srv.URL+"/v1/saves/SLUS-1/1", "", nil)
	noauth.Body.Close()
	if noauth.StatusCode != 401 {
		t.Errorf("noauth = %d, want 401", noauth.StatusCode)
	}
}
