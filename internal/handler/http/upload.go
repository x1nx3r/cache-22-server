package httphandler

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxUploadBytes = 9 << 30

func sanitizeSerial(name string) (string, bool) {
	base := filepath.Base(strings.TrimSpace(name))
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	if base == "" || base == "." || base == ".." || base[0] == '.' || len(base) > 128 {
		return "", false
	}
	if strings.ContainsAny(base, `/\`) || strings.Contains(base, "..") {
		return "", false
	}
	return base, true
}

func (h *Handler) uploadGame(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "want multipart upload with a file part"})
		return
	}

	var (
		tmp       *os.File
		tmpPath   string
		filename  string
		titleOver string
	)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad multipart body"})
			return
		}
		name := part.FormName()
		if part.FileName() == "" {
			buf, _ := io.ReadAll(io.LimitReader(part, 512))
			if name == "title" {
				titleOver = strings.TrimSpace(string(buf))
			}
			continue
		}
		if tmp != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "one file per upload"})
			return
		}
		filename = part.FileName()
		if !strings.HasSuffix(strings.ToLower(filename), ".iso") {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "raw .iso only"})
			return
		}
		if err := os.MkdirAll(h.scanner.LibraryDir(), 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		tmp, err = os.CreateTemp(h.scanner.LibraryDir(), ".upload-*.part")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		tmpPath = tmp.Name()
		if _, err := io.Copy(tmp, part); err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "upload too large or interrupted"})
			return
		}
		tmp.Close()
	}
	if tmpPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no file part in upload"})
		return
	}

	serial, ok := sanitizeSerial(filename)
	if !ok {
		os.Remove(tmpPath)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad filename"})
		return
	}
	final := filepath.Join(h.scanner.LibraryDir(), serial+".iso")
	if _, err := os.Stat(final); err == nil {
		os.Remove(tmpPath)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "game already exists"})
		return
	}
	if err := os.Rename(tmpPath, final); err != nil {
		os.Remove(tmpPath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	g, err := h.scanner.Inspect(final)
	if err != nil {
		os.Remove(final)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if titleOver != "" {
		g.Title = titleOver
	}
	if err := h.games.RegisterGame(r.Context(), g); err != nil {
		os.Remove(final)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	go func() {
		_, _ = h.covers.Backfill(context.Background(), 25)
	}()
	writeJSON(w, http.StatusCreated, g)
}
