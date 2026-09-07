package webhandler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"

	authusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/auth"
	gameusecase "github.com/x1nx3r/cache-22-server/internal/app/usecase/game"
	"github.com/x1nx3r/cache-22-server/internal/entity"
	"github.com/x1nx3r/cache-22-server/internal/infra/scanner"
	"github.com/x1nx3r/cache-22-server/internal/web"
)

const sessionCookie = "cache22_sess"

type Handler struct {
	games   gameusecase.UseCase
	scanner *scanner.Scanner
	auth    *authusecase.Auth
	secure  bool
}

func New(games gameusecase.UseCase, sc *scanner.Scanner, auth *authusecase.Auth, secure bool) *Handler {
	return &Handler{games: games, scanner: sc, auth: auth, secure: secure}
}

// internal logs the real error and shows a generic message, so handler
// failures don't leak paths and SQL details into HTML error pages.
func internal(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.library)
	mux.HandleFunc("GET /games/{serial}", h.detail)
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("POST /scan", h.scan)
	mux.HandleFunc("GET /admin/users", h.usersPage)
	mux.HandleFunc("POST /admin/users", h.createUser)
	mux.HandleFunc("POST /admin/users/{id}/delete", h.deleteUser)
	mux.HandleFunc("POST /admin/users/{id}/password", h.resetPassword)
}

func (h *Handler) currentUser(r *http.Request) (entity.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return entity.User{}, false
	}
	u, ok, err := h.auth.Authenticate(r.Context(), c.Value)
	if err != nil || !ok {
		return entity.User{}, false
	}
	return u, true
}

func (h *Handler) requireLogin(w http.ResponseWriter, r *http.Request) (entity.User, bool) {
	u, ok := h.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return entity.User{}, false
	}
	return u, true
}

func (h *Handler) requireAdminPage(w http.ResponseWriter, r *http.Request) (entity.User, bool) {
	u, ok := h.requireLogin(w, r)
	if !ok {
		return u, false
	}
	if !u.IsAdmin {
		http.Error(w, "admin required", http.StatusForbidden)
		return u, false
	}
	return u, true
}

func (h *Handler) library(w http.ResponseWriter, r *http.Request) {
	u, ok := h.requireLogin(w, r)
	if !ok {
		return
	}
	games, err := h.games.ListGames(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = web.Library(games, true, u.IsAdmin).Render(r.Context(), w)
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	u, ok := h.requireLogin(w, r)
	if !ok {
		return
	}
	serial := r.PathValue("serial")
	g, err := h.games.GetGame(r.Context(), serial)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		internal(w, err)
		return
	}
	m, err := h.games.GetManifest(r.Context(), serial)
	if err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = web.Detail(g, m, true, u.IsAdmin).Render(r.Context(), w)
}

func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.currentUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	needs, err := h.auth.NeedsSetup(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = web.LoginForm("", needs).Render(r.Context(), w)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	fail := func(msg string, setup bool) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = web.LoginForm(msg, setup).Render(r.Context(), w)
	}
	if err := r.ParseForm(); err != nil {
		fail("Bad form.", false)
		return
	}
	needs, err := h.auth.NeedsSetup(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	var token string
	if needs {
		_, err := h.auth.SetupFirstAdmin(r.Context(), r.FormValue("setup_token"), r.FormValue("username"), r.FormValue("password"))
		if err != nil {
			fail(friendlyAuthErr(err), true)
			return
		}
		var u entity.User
		token, u, err = h.auth.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
		if err != nil {
			fail(friendlyAuthErr(err), true)
			return
		}
		_ = u
	} else {
		var err error
		token, _, err = h.auth.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
		if err != nil {
			fail(friendlyAuthErr(err), false)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func friendlyAuthErr(err error) string {
	switch {
	case errors.Is(err, authusecase.ErrBadCredentials):
		return "Wrong username or password."
	case errors.Is(err, authusecase.ErrBadSetupToken):
		return "Wrong setup token."
	case errors.Is(err, authusecase.ErrWeakPassword):
		return "Password must be 8 or more characters."
	case errors.Is(err, authusecase.ErrBadUsername):
		return "Username must be 1-64 characters."
	default:
		log.Printf("auth error: %v", err)
		return "Something went wrong."
	}
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = h.auth.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.currentUser(r); !ok {
		w.Header().Set("HX-Redirect", "/login")
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	if _, err := h.scanner.Run(r.Context()); err != nil {
		internal(w, err)
		return
	}
	games, err := h.games.ListGames(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = web.GameTable(games, true).Render(r.Context(), w)
}

func (h *Handler) usersPage(w http.ResponseWriter, r *http.Request) {
	me, ok := h.requireAdminPage(w, r)
	if !ok {
		return
	}
	users, err := h.auth.ListUsers(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = web.Users(me, users, "").Render(r.Context(), w)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	me, ok := h.requireAdminPage(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	_, err := h.auth.CreateUser(r.Context(), r.FormValue("username"), r.FormValue("password"), r.FormValue("is_admin") == "on")
	if err != nil {
		users, _ := h.auth.ListUsers(r.Context())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = web.Users(me, users, friendlyAuthErr(err)).Render(r.Context(), w)
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	me, ok := h.requireAdminPage(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || me.ID == id {
		http.Error(w, "cannot delete this user", http.StatusBadRequest)
		return
	}
	if err := h.auth.DeleteUser(r.Context(), id); err != nil {
		internal(w, err)
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	me, ok := h.requireAdminPage(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if err := h.auth.ResetPassword(r.Context(), id, r.FormValue("password")); err != nil {
		users, _ := h.auth.ListUsers(r.Context())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = web.Users(me, users, friendlyAuthErr(err)).Render(r.Context(), w)
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}
