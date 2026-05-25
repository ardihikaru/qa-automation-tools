package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

// Config carries the bits the middleware needs at construction time.
type Config struct {
	DB         *sql.DB
	CookieName string
}

// Require returns middleware that rejects unauthenticated requests.
// /api/* paths get JSON 401; everything else gets redirected to /login.
func Require(c Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(c.CookieName)
			if err != nil || cookie == nil || cookie.Value == "" {
				unauthorized(w, r)
				return
			}

			u, err := ValidateSession(r.Context(), c.DB, cookie.Value)
			if err != nil || u == nil {
				clearCookie(w, c.CookieName)
				unauthorized(w, r)
				return
			}

			ctx := SetUser(r.Context(), u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsAuthAllowList returns true when path can be served without a session.
// Centralised here so the router and middleware agree.
func IsAuthAllowList(method, path string) bool {
	if method == http.MethodOptions {
		return true
	}
	switch path {
	case "/health", "/api/auth/login":
		return true
	}
	return false
}

func unauthorized(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "unauthorized",
		})
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
