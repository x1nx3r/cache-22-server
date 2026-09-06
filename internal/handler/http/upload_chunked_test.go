package httphandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/x1nx3r/cache-22-server/internal/entity"
)

func doJSON(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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

func TestChunkedUploadRoundTrip(t *testing.T) {
	srv, uc, dir, adminTok := newUploadServer(t)
	content := bytes.Repeat([]byte("abcdefgh"), 3000)

	res := doJSON(t, "POST", srv.URL+"/v1/games/upload/init", adminTok,
		map[string]any{"filename": "Chunk Game.iso", "size": len(content), "title": "Chunk Game"})
	defer res.Body.Close()
	if res.StatusCode != 201 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("init = %d: %s", res.StatusCode, b)
	}
	var initOut struct {
		UploadID  string `json:"uploadId"`
		ChunkSize int64  `json:"chunkSize"`
	}
	if err := json.NewDecoder(res.Body).Decode(&initOut); err != nil {
		t.Fatal(err)
	}
	if len(initOut.UploadID) != 32 || initOut.ChunkSize <= 0 {
		t.Fatalf("init = %+v", initOut)
	}

	put := func(offset int, b []byte) *http.Response {
		t.Helper()
		req, err := http.NewRequest("PUT",
			srv.URL+"/v1/games/upload/chunk?id="+initOut.UploadID+"&offset="+itoa(offset),
			bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+adminTok)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	mid := len(content) / 2
	// second half first: out-of-order must work
	r2 := put(mid, content[mid:])
	b2, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Fatalf("chunk2 = %d: %s", r2.StatusCode, b2)
	}
	// complete too early must fail
	early := doJSON(t, "POST", srv.URL+"/v1/games/upload/complete", adminTok,
		map[string]string{"id": initOut.UploadID})
	eb, _ := io.ReadAll(early.Body)
	early.Body.Close()
	if early.StatusCode != 400 {
		t.Errorf("early complete = %d, want 400: %s", early.StatusCode, eb)
	}
	r1 := put(0, content[:mid])
	b1, _ := io.ReadAll(r1.Body)
	r1.Body.Close()
	if r1.StatusCode != 200 {
		t.Fatalf("chunk1 = %d: %s", r1.StatusCode, b1)
	}

	done := doJSON(t, "POST", srv.URL+"/v1/games/upload/complete", adminTok,
		map[string]string{"id": initOut.UploadID})
	defer done.Body.Close()
	if done.StatusCode != 201 {
		b, _ := io.ReadAll(done.Body)
		t.Fatalf("complete = %d: %s", done.StatusCode, b)
	}
	var g entity.Game
	if err := json.NewDecoder(done.Body).Decode(&g); err != nil {
		t.Fatal(err)
	}
	if g.Serial != "Chunk Game" {
		t.Errorf("serial = %q", g.Serial)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "Chunk Game.iso"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, content) {
		t.Error("reassembled bytes differ")
	}
	if _, ok := uc.games["Chunk Game"]; !ok {
		t.Error("game not registered")
	}
}

func TestChunkedUploadRejects(t *testing.T) {
	srv, _, _, adminTok := newUploadServer(t)

	bad := doJSON(t, "POST", srv.URL+"/v1/games/upload/init", adminTok,
		map[string]any{"filename": "x.chd", "size": 10})
	bb, _ := io.ReadAll(bad.Body)
	bad.Body.Close()
	if bad.StatusCode != 415 {
		t.Errorf("chd init = %d, want 415: %s", bad.StatusCode, bb)
	}

	huge := doJSON(t, "POST", srv.URL+"/v1/games/upload/init", adminTok,
		map[string]any{"filename": "x.iso", "size": int64(10 << 30)})
	huge.Body.Close()
	if huge.StatusCode != 400 {
		t.Errorf("oversize init = %d, want 400", huge.StatusCode)
	}

	req, _ := http.NewRequest("PUT", srv.URL+"/v1/games/upload/chunk?id=nope&offset=0",
		bytes.NewReader([]byte("x")))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Errorf("bad id chunk = %d, want 400", res.StatusCode)
	}

	noauth := doJSON(t, "POST", srv.URL+"/v1/games/upload/init", "",
		map[string]any{"filename": "x.iso", "size": 10})
	noauth.Body.Close()
	if noauth.StatusCode != 401 {
		t.Errorf("una authed init = %d, want 401", noauth.StatusCode)
	}
}

func TestChunkedUploadAbort(t *testing.T) {
	srv, _, dir, adminTok := newUploadServer(t)
	res := doJSON(t, "POST", srv.URL+"/v1/games/upload/init", adminTok,
		map[string]any{"filename": "Abort.iso", "size": 100})
	var out struct {
		UploadID string `json:"uploadId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	del := doJSON(t, "DELETE", srv.URL+"/v1/games/upload?id="+out.UploadID, adminTok, nil)
	del.Body.Close()
	if del.StatusCode != 200 {
		t.Errorf("abort = %d, want 200", del.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, ".uploads", out.UploadID)); !os.IsNotExist(err) {
		t.Error("upload dir must be gone after abort")
	}
}

func TestChunkedUploadParallel(t *testing.T) {
	srv, _, dir, adminTok := newUploadServer(t)
	content := bytes.Repeat([]byte("0123456789abcdef"), 5000)

	res := doJSON(t, "POST", srv.URL+"/v1/games/upload/init", adminTok,
		map[string]any{"filename": "Parallel.iso", "size": len(content)})
	var initOut struct {
		UploadID string `json:"uploadId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&initOut); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	const workers = 8
	const step = 4096
	type job struct{ offset int }
	jobs := make(chan job, 64)
	for off := 0; off < len(content); off += step {
		jobs <- job{off}
	}
	close(jobs)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				end := j.offset + step
				if end > len(content) {
					end = len(content)
				}
				req, _ := http.NewRequest("PUT",
					srv.URL+"/v1/games/upload/chunk?id="+initOut.UploadID+"&offset="+itoa(j.offset),
					bytes.NewReader(content[j.offset:end]))
				req.Header.Set("Authorization", "Bearer "+adminTok)
				r, err := http.DefaultClient.Do(req)
				if err != nil {
					errs <- err
					return
				}
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
				if r.StatusCode != 200 {
					errs <- fmt.Errorf("chunk %d = %d", j.offset, r.StatusCode)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	done := doJSON(t, "POST", srv.URL+"/v1/games/upload/complete", adminTok,
		map[string]string{"id": initOut.UploadID})
	defer done.Body.Close()
	if done.StatusCode != 201 {
		b, _ := io.ReadAll(done.Body)
		t.Fatalf("parallel complete = %d: %s", done.StatusCode, b)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "Parallel.iso"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, content) {
		t.Error("parallel reassembled bytes differ")
	}
}

func TestMergeRange(t *testing.T) {
	var m uploadMeta
	m.Size = 100
	if mergeRange(&m, 50, 100) {
		t.Error("half coverage must not complete")
	}
	if !mergeRange(&m, 0, 50) {
		t.Error("full coverage must complete")
	}
	var m2 uploadMeta
	m2.Size = 100
	mergeRange(&m2, 30, 40)
	mergeRange(&m2, 0, 30)
	if mergeRange(&m2, 40, 100) != true {
		t.Error("adjacent ranges must merge to complete")
	}
	var m3 uploadMeta
	m3.Size = 100
	mergeRange(&m3, 20, 80)
	mergeRange(&m3, 0, 30)
	if !mergeRange(&m3, 70, 100) {
		t.Error("overlapping ranges must merge to complete")
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
