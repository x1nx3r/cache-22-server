package httphandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	authusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/auth"
)

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json body"})
		return false
	}
	return true
}

func (h *Handler) authSetup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SetupToken string `json:"setup_token"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u, err := h.auth.SetupFirstAdmin(r.Context(), in.SetupToken, in.Username, in.Password)
	switch {
	case errors.Is(err, authusecase.ErrSetupDone):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, authusecase.ErrBadSetupToken):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, authusecase.ErrBadUsername), errors.Is(err, authusecase.ErrWeakPassword):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusCreated, u)
	}
}

func (h *Handler) authLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	token, u, err := h.auth.Login(r.Context(), in.Username, in.Password)
	if errors.Is(err, authusecase.ErrBadCredentials) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (h *Handler) authLogout(w http.ResponseWriter, r *http.Request) {
	_ = h.auth.Logout(r.Context(), tokenFrom(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) authMe(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	writeJSON(w, http.StatusOK, u)
}

func (h *Handler) adminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.auth.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handler) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u, err := h.auth.CreateUser(r.Context(), in.Username, in.Password, in.IsAdmin)
	switch {
	case errors.Is(err, authusecase.ErrBadUsername), errors.Is(err, authusecase.ErrWeakPassword):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusCreated, u)
	}
}

func userIDFrom(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func (h *Handler) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := userIDFrom(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	me, _ := UserFrom(r.Context())
	if me.ID == id {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete yourself"})
		return
	}
	if err := h.auth.DeleteUser(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) adminResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := userIDFrom(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := h.auth.ResetPassword(r.Context(), id, in.Password); err != nil {
		if errors.Is(err, authusecase.ErrWeakPassword) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) adminSetAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := userIDFrom(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var in struct {
		IsAdmin bool `json:"is_admin"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	me, _ := UserFrom(r.Context())
	if me.ID == id && !in.IsAdmin {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot demote yourself"})
		return
	}
	if err := h.auth.SetAdmin(r.Context(), id, in.IsAdmin); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) adminBackfillCovers(w http.ResponseWriter, r *http.Request) {
	n, err := h.covers.Backfill(r.Context(), 25)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fetched": n})
}
