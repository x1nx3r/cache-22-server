package httphandler

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
)

const maxSaveBytes = 16 << 20

func saveSlot(r *http.Request) (string, int, bool) {
	serial := r.PathValue("serial")
	slot, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil || (slot != 1 && slot != 2) {
		return "", 0, false
	}
	if _, ok := sanitizeSerial(serial + ".iso"); !ok {
		return "", 0, false
	}
	return serial, slot, true
}

func (h *Handler) getSave(w http.ResponseWriter, r *http.Request) {
	h.serveSave(w, r, true)
}

func (h *Handler) headSave(w http.ResponseWriter, r *http.Request) {
	h.serveSave(w, r, false)
}

func (h *Handler) serveSave(w http.ResponseWriter, r *http.Request, withBody bool) {
	u, ok := UserFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	serial, slot, valid := saveSlot(r)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad serial or slot"})
		return
	}
	meta, raw, err := h.saves.Get(r.Context(), u.ID, serial, slot)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no save"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-SHA256", meta.SHA256)
	w.Header().Set("Last-Modified", meta.UpdatedAt.UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
	if withBody {
		w.Write(raw)
	}
}

func (h *Handler) putSave(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	serial, slot, valid := saveSlot(r)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad serial or slot"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSaveBytes)
	defer r.Body.Close()
	raw, err := io.ReadAll(r.Body)
	if err != nil || len(raw) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty save"})
		return
	}
	meta, err := h.saves.Put(r.Context(), u.ID, serial, slot, raw)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (h *Handler) deleteSave(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	serial, slot, valid := saveSlot(r)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad serial or slot"})
		return
	}
	if err := h.saves.Delete(r.Context(), u.ID, serial, slot); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
