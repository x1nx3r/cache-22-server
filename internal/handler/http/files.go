package httphandler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request) {
	serial := r.PathValue("serial")
	g, err := h.games.GetGame(r.Context(), serial)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "game not found"})
			return
		}
		h.internalErr(w, err)
		return
	}
	if !isISO(g.FilePath) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "compressed image needs extract first, raw .iso only"})
		return
	}
	f, err := os.Open(g.FilePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "backing file missing"})
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		h.internalErr(w, err)
		return
	}
	etag := `"` + g.RedumpHash + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(g.FilePath), st.ModTime(), f)
}

func (h *Handler) serveManifest(w http.ResponseWriter, r *http.Request) {
	serial := r.PathValue("serial")
	m, err := h.games.GetManifest(r.Context(), serial)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "game not found"})
			return
		}
		h.internalErr(w, err)
		return
	}
	if !m.Supported {
		m.UnsupportedHint = "compressed image needs extract first, raw .iso only"
	}
	writeJSON(w, http.StatusOK, m)
}

func isISO(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".iso")
}

func (h *Handler) serveCover(w http.ResponseWriter, r *http.Request) {
	c, err := h.covers.Get(r.Context(), r.PathValue("serial"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no cover art"})
		return
	}
	f, err := os.Open(c.ImagePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no cover art"})
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		h.internalErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, "cover.jpg", st.ModTime(), f)
}
