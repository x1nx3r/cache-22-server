package httphandler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Chunked uploads split the ISO into small PUTs so reverse proxies with
// per-request body caps (e.g. Cloudflare's 100MB) stop rejecting them.
// Flow: init -> PUT chunks (any order) -> complete. Suggested chunk 48MB,
// hard per-chunk cap 64MB.
const (
	suggestedChunkBytes = 48 << 20
	maxChunkBytes       = 64 << 20
	staleUploadAfter    = 24 * time.Hour
)

type uploadMeta struct {
	ID       string    `json:"id"`
	Serial   string    `json:"serial"`
	Filename string    `json:"filename"`
	Size     int64     `json:"size"`
	Title    string    `json:"title"`
	Ranges   [][2]int64 `json:"ranges"`
	Created  int64     `json:"created_unix"`
}

func uploadsDir(libraryDir string) string {
	return filepath.Join(libraryDir, ".uploads")
}

func newUploadID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func validUploadID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func metaPath(libraryDir, id string) string {
	return filepath.Join(uploadsDir(libraryDir), id, "meta.json")
}

func dataPath(libraryDir, id string) string {
	return filepath.Join(uploadsDir(libraryDir), id, "data.part")
}

func readMeta(libraryDir, id string) (uploadMeta, error) {
	var m uploadMeta
	raw, err := os.ReadFile(metaPath(libraryDir, id))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(raw, &m)
}

func writeMeta(libraryDir string, m uploadMeta) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath(libraryDir, m.ID), raw, 0o644)
}

// mergeRange adds [start, end) and returns whether [0, size) is fully covered.
func mergeRange(m *uploadMeta, start, end int64) bool {
	m.Ranges = append(m.Ranges, [2]int64{start, end})
	sort.Slice(m.Ranges, func(i, j int) bool { return m.Ranges[i][0] < m.Ranges[j][0] })
	merged := m.Ranges[:0]
	cur := m.Ranges[0]
	for _, r := range m.Ranges[1:] {
		if r[0] <= cur[1] {
			if r[1] > cur[1] {
				cur[1] = r[1]
			}
			continue
		}
		merged = append(merged, cur)
		cur = r
	}
	merged = append(merged, cur)
	m.Ranges = merged
	return len(merged) == 1 && merged[0][0] == 0 && merged[0][1] == m.Size
}

func sweepStaleUploads(libraryDir string) {
	entries, err := os.ReadDir(uploadsDir(libraryDir))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleUploadAfter).Unix()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := readMeta(libraryDir, e.Name())
		if err != nil || m.Created < cutoff {
			os.RemoveAll(filepath.Join(uploadsDir(libraryDir), e.Name()))
		}
	}
}

func (h *Handler) uploadInit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		Title    string `json:"title"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if !strings.HasSuffix(strings.ToLower(in.Filename), ".iso") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "raw .iso only"})
		return
	}
	if in.Size < 1 || in.Size > maxUploadBytes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad size"})
		return
	}
	serial, ok := sanitizeSerial(in.Filename)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad filename"})
		return
	}
	libraryDir := h.scanner.LibraryDir()
	if _, err := os.Stat(filepath.Join(libraryDir, serial+".iso")); err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "game already exists"})
		return
	}
	sweepStaleUploads(libraryDir)
	if entries, err := os.ReadDir(uploadsDir(libraryDir)); err == nil {
		for _, e := range entries {
			if m, err := readMeta(libraryDir, e.Name()); err == nil && m.Serial == serial {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "upload already in progress for this game"})
				return
			}
		}
	}
	id, err := newUploadID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := os.MkdirAll(filepath.Join(uploadsDir(libraryDir), id), 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	f, err := os.OpenFile(dataPath(libraryDir, id), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := f.Truncate(in.Size); err != nil {
		f.Close()
		os.RemoveAll(filepath.Join(uploadsDir(libraryDir), id))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	f.Close()
	m := uploadMeta{ID: id, Serial: serial, Filename: in.Filename, Size: in.Size,
		Title: strings.TrimSpace(in.Title), Created: time.Now().Unix()}
	if err := writeMeta(libraryDir, m); err != nil {
		os.RemoveAll(filepath.Join(uploadsDir(libraryDir), id))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"uploadId": id, "chunkSize": suggestedChunkBytes})
}

func (h *Handler) uploadChunk(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if !validUploadID(id) || offset < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id or offset"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxChunkBytes)
	defer r.Body.Close()
	libraryDir := h.scanner.LibraryDir()
	// Read the network body BEFORE taking the per-upload lock so parallel
	// chunks overlap in flight; only the meta read-modify-write serializes.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "interrupted chunk"})
		return
	}
	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty chunk"})
		return
	}
	unlock := h.uploadLock(id)
	defer unlock()
	m, err := readMeta(libraryDir, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown upload"})
		return
	}
	if offset+int64(len(body)) > m.Size {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "chunk past end of file"})
		return
	}
	f, err := os.OpenFile(dataPath(libraryDir, id), os.O_WRONLY, 0o644)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer f.Close()
	written := int64(0)
	for len(body) > 0 {
		n, werr := f.WriteAt(body, offset+written)
		if werr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": werr.Error()})
			return
		}
		if n == 0 {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "short write"})
			return
		}
		body = body[n:]
		written += int64(n)
	}
	complete := mergeRange(&m, offset, offset+written)
	if err := writeMeta(libraryDir, m); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": offset + written, "complete": complete})
}

func (h *Handler) uploadComplete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if !validUploadID(in.ID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	libraryDir := h.scanner.LibraryDir()
	unlock := h.uploadLock(in.ID)
	m, err := readMeta(libraryDir, in.ID)
	if err != nil {
		unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown upload"})
		return
	}
	covered := len(m.Ranges) == 1 && m.Ranges[0][0] == 0 && m.Ranges[0][1] == m.Size
	if !covered {
		unlock()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "incomplete upload"})
		return
	}
	g, code, msg := h.finalizeUpload(r.Context(), dataPath(libraryDir, in.ID), m.Filename, m.Title)
	os.RemoveAll(filepath.Join(uploadsDir(libraryDir), in.ID))
	unlock()
	h.dropUploadLock(in.ID)
	if code != http.StatusCreated {
		writeJSON(w, code, map[string]string{"error": msg})
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handler) uploadAbort(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validUploadID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	os.RemoveAll(filepath.Join(uploadsDir(h.scanner.LibraryDir()), id))
	h.dropUploadLock(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

var errGameExists = fmt.Errorf("game already exists")
