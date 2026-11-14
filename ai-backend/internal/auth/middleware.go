package auth

import (
	"database/sql"
	"encoding/json"
	"log"
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

			serveAs(w, r, u, next)
		})
	}
}

// serveAs runs next with u on the context. While impersonating, every
// non-GET/HEAD request gets one audit log line with the final status.
func serveAs(w http.ResponseWriter, r *http.Request, u *User, next http.Handler) {
	r = r.WithContext(SetUser(r.Context(), u))
	if u.Impersonator == nil || r.Method == http.MethodGet || r.Method == http.MethodHead {
		next.ServeHTTP(w, r)
		return
	}
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	next.ServeHTTP(sw, r)
	logImpersonatedWrite(u, r.Method, r.URL.Path, sw.status)
}

// logImpersonatedWrite is a variable so tests can capture the line.
var logImpersonatedWrite = func(u *User, method, path string, status int) {
	log.Printf("impersonated-write actor=%s target=%s method=%s path=%s status=%d",
		u.Impersonator.Username, u.Username, method, path, status)
}

// statusWriter records the response status; 200 when WriteHeader is never called.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
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
	case "/api/v1/gaos/results", "/api/v1/gaos/catalog", "/api/v1/gaos/inbox-answer", "/api/v1/results/playwright":
		// Bearer-token routes (GAOS_INBOUND.md); the handler enforces the token, fail-closed.
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
