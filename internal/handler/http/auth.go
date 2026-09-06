package httphandler

import (
	"context"
	"net/http"
	"strings"

	"github.com/cache-22/cache-22-server/internal/entity"
)

type ctxKey struct{}

func tokenFrom(r *http.Request) string {
	if got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); got != "" {
		return got
	}
	if c, err := r.Cookie("cache22_sess"); err == nil {
		return c.Value
	}
	return ""
}

func UserFrom(ctx context.Context) (entity.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(entity.User)
	return u, ok
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok, err := h.auth.Authenticate(r.Context(), tokenFrom(r))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login required"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

func (h *Handler) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		if !u.IsAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin required"})
			return
		}
		next(w, r)
	})
}
