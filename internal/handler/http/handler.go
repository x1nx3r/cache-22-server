package httphandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	authusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/auth"
	coverusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/cover"
	gameusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/game"
	saveusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/save"
	"github.com/x1nx3r/cache-22-server/internal/infra/scanner"
)

type Handler struct {
	games   gameusecase.UseCase
	scanner *scanner.Scanner
	auth    *authusecase.Auth
	covers  *coverusecase.Cover
	saves   *saveusecase.Save

	uploadMu    sync.Mutex
	uploadLocks map[string]*sync.Mutex
}

func New(games gameusecase.UseCase, sc *scanner.Scanner, auth *authusecase.Auth, covers *coverusecase.Cover, saves *saveusecase.Save) *Handler {
	return &Handler{games: games, scanner: sc, auth: auth, covers: covers, saves: saves, uploadLocks: map[string]*sync.Mutex{}}
}

// uploadLock serializes meta read-modify-write cycles for one upload session
// so parallel chunk PUTs cannot lose each other's range records.
func (h *Handler) uploadLock(id string) func() {
	h.uploadMu.Lock()
	l, ok := h.uploadLocks[id]
	if !ok {
		l = &sync.Mutex{}
		h.uploadLocks[id] = l
	}
	h.uploadMu.Unlock()
	l.Lock()
	return l.Unlock
}

func (h *Handler) dropUploadLock(id string) {
	h.uploadMu.Lock()
	delete(h.uploadLocks, id)
	h.uploadMu.Unlock()
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/health", h.health)
	mux.HandleFunc("POST /v1/auth/setup", h.authSetup)
	mux.HandleFunc("POST /v1/auth/login", h.authLogin)
	mux.HandleFunc("POST /v1/auth/logout", h.requireAuth(h.authLogout))
	mux.HandleFunc("GET /v1/auth/me", h.requireAuth(h.authMe))

	mux.HandleFunc("GET /v1/games", h.requireAuth(h.listGames))
	mux.HandleFunc("GET /v1/games/{serial}", h.requireAuth(h.getGame))
	mux.HandleFunc("GET /v1/games/{serial}/manifest.json", h.requireAuth(h.serveManifest))
	mux.HandleFunc("GET /v1/games/{serial}/cover", h.requireAuth(h.serveCover))
	mux.HandleFunc("GET /v1/files/{serial}", h.requireAuth(h.serveFile))

	mux.HandleFunc("POST /v1/games/upload", h.requireAdmin(h.uploadGame))
	mux.HandleFunc("POST /v1/games/upload/init", h.requireAdmin(h.uploadInit))
	mux.HandleFunc("PUT /v1/games/upload/chunk", h.requireAdmin(h.uploadChunk))
	mux.HandleFunc("POST /v1/games/upload/complete", h.requireAdmin(h.uploadComplete))
	mux.HandleFunc("DELETE /v1/games/upload", h.requireAdmin(h.uploadAbort))
	mux.HandleFunc("GET /v1/saves/{serial}/{slot}", h.requireAuth(h.getSave))
	mux.HandleFunc("HEAD /v1/saves/{serial}/{slot}", h.requireAuth(h.headSave))
	mux.HandleFunc("PUT /v1/saves/{serial}/{slot}", h.requireAuth(h.putSave))
	mux.HandleFunc("DELETE /v1/saves/{serial}/{slot}", h.requireAuth(h.deleteSave))
	mux.HandleFunc("POST /v1/scan", h.requireAdmin(h.scan))

	mux.HandleFunc("GET /v1/admin/users", h.requireAdmin(h.adminListUsers))
	mux.HandleFunc("POST /v1/admin/users", h.requireAdmin(h.adminCreateUser))
	mux.HandleFunc("DELETE /v1/admin/users/{id}", h.requireAdmin(h.adminDeleteUser))
	mux.HandleFunc("POST /v1/admin/users/{id}/password", h.requireAdmin(h.adminResetPassword))
	mux.HandleFunc("POST /v1/admin/users/{id}/admin", h.requireAdmin(h.adminSetAdmin))
	mux.HandleFunc("POST /v1/admin/covers/backfill", h.requireAdmin(h.adminBackfillCovers))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	res, err := h.games.HealthCheck(r.Context(), "http")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) listGames(w http.ResponseWriter, r *http.Request) {
	games, err := h.games.ListGames(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": games})
}

func (h *Handler) getGame(w http.ResponseWriter, r *http.Request) {
	serial := r.PathValue("serial")
	g, err := h.games.GetGame(r.Context(), serial)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "game not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	n, err := h.scanner.Run(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	go func() {
		_, _ = h.covers.Backfill(context.Background(), 25)
	}()
	writeJSON(w, http.StatusOK, map[string]any{"scanned": n})
}
