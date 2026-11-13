package api

import (
	"bytes"
	"context"
	stdsql "database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/seed"
)

// Impersonation integration test. It needs a disposable MySQL 8 database:
//
//	IMPERSONATION_IT_DSN='root:pw@tcp(127.0.0.1:33306)/qa_it?parseTime=true&loc=Local&multiStatements=true'
//
// It runs the migrations on that database, so never point it at production.
// Skipped when the variable is unset.

type itEnv struct {
	t      *testing.T
	sql    *stdsql.DB
	srv    *httptest.Server
	prefix string
}

func setupImpersonationIT(t *testing.T) *itEnv {
	t.Helper()
	dsn := os.Getenv("IMPERSONATION_IT_DSN")
	if dsn == "" {
		t.Skip("IMPERSONATION_IT_DSN not set")
	}
	sqlDB, err := stdsql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := db.RunMigrations(sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := NewRouter(&db.DB{SQL: sqlDB}, nil, nil, nil)
	r.SetAuth(&AuthHandlers{DB: sqlDB, CookieName: "qa_session", SessionTTL: 12 * time.Hour},
		auth.Require(auth.Config{DB: sqlDB, CookieName: "qa_session"}), "http://localhost:6173")
	env := &itEnv{t: t, sql: sqlDB, srv: httptest.NewServer(r), prefix: "it" + uuid.NewString()[:8] + "-"}
	t.Cleanup(func() {
		env.srv.Close()
		_, _ = sqlDB.Exec(`DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE username LIKE ?)`, env.prefix+"%")
		_, _ = sqlDB.Exec(`DELETE FROM users WHERE username LIKE ?`, env.prefix+"%")
		sqlDB.Close()
	})
	return env
}

func (e *itEnv) user(name, role string, active bool) string {
	e.t.Helper()
	hash, err := auth.Hash("password-" + name)
	if err != nil {
		e.t.Fatal(err)
	}
	id := uuid.NewString()
	act := 0
	if active {
		act = 1
	}
	if _, err := e.sql.Exec(`INSERT INTO users (id, username, role, is_lead, password_hash, must_change_pw, active)
		VALUES (?, ?, ?, 0, ?, 1, ?)`, id, e.prefix+name, role, hash, act); err != nil {
		e.t.Fatal(err)
	}
	return id
}

type itClient struct {
	e *itEnv
	c *http.Client
}

func (e *itEnv) login(name string) *itClient {
	e.t.Helper()
	jar, _ := cookiejar.New(nil)
	cl := &itClient{e: e, c: &http.Client{Jar: jar}}
	st, _ := cl.do(http.MethodPost, "/api/auth/login", map[string]string{"username": e.prefix + name, "password": "password-" + name})
	if st != 200 {
		e.t.Fatalf("login %s: %d", name, st)
	}
	return cl
}

func (c *itClient) do(method, path string, body any) (int, map[string]any) {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, rd)
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.c.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (c *itClient) token() string {
	for _, ck := range c.c.Jar.Cookies(mustURL(c.e.srv.URL)) {
		if ck.Name == "qa_session" {
			return ck.Value
		}
	}
	return ""
}

// me returns (username, impersonator username or "").
func (c *itClient) me() (int, string, string) {
	st, body := c.do(http.MethodGet, "/api/auth/me", nil)
	if st != 200 {
		return st, "", ""
	}
	u := body["user"].(map[string]any)
	imp := ""
	if i, ok := u["impersonator"].(map[string]any); ok {
		imp = i["username"].(string)
	}
	return st, u["username"].(string), imp
}

func (e *itEnv) auditReasons(targetID string) []string {
	rows, err := e.sql.Query(`SELECT COALESCE(ended_reason, 'NULL') FROM impersonation_audit WHERE target_user_id = ? ORDER BY id`, targetID)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

func TestImpersonationIT_Flow(t *testing.T) {
	e := setupImpersonationIT(t)
	e.user("svc", "Service", true)
	qaID := e.user("qa", "QA", true)
	e.user("off", "QA", false)
	e.user("dev", "Developer", true)

	svc := e.login("svc")
	parentToken := svc.token()

	// /api/users: impersonators only, no password fields.
	if st, _ := e.login("dev").do(http.MethodGet, "/api/users", nil); st != 403 {
		t.Fatalf("developer /api/users: %d", st)
	}
	req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/users", nil)
	resp, err := svc.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || strings.Contains(strings.ToLower(string(raw)), "password") || !strings.Contains(string(raw), e.prefix+"qa") {
		t.Fatalf("/api/users: %d %s", resp.StatusCode, raw)
	}

	// Error cases.
	for _, c := range []struct {
		name string
		st   int
	}{{e.prefix + "nobody", 404}, {e.prefix + "svc", 400}, {e.prefix + "off", 409}} {
		if st, _ := svc.do(http.MethodPost, "/api/auth/impersonate", map[string]string{"username": c.name}); st != c.st {
			t.Errorf("impersonate %s: got %d want %d", c.name, st, c.st)
		}
	}

	// Start: identity switches, cookie unchanged.
	st, body := svc.do(http.MethodPost, "/api/auth/impersonate", map[string]string{"username": strings.ToUpper(e.prefix + "qa ")})
	if st != 200 || body["user"].(map[string]any)["username"] != e.prefix+"qa" {
		t.Fatalf("impersonate: %d %v", st, body)
	}
	if svc.token() != parentToken {
		t.Fatal("cookie must not change on impersonate")
	}
	if _, who, imp := svc.me(); who != e.prefix+"qa" || imp != e.prefix+"svc" {
		t.Fatalf("/me while impersonating: %s by %s", who, imp)
	}

	// Acting as QA: product management refused, password change refused, no nesting.
	if st, _ := svc.do(http.MethodPost, "/api/products", map[string]string{"name": "x"}); st != 403 {
		t.Errorf("QA product create: %d", st)
	}
	if st, _ := svc.do(http.MethodPost, "/api/auth/change-password", map[string]string{"currentPassword": "x", "newPassword": "yyyyyyyyyy"}); st != 403 {
		t.Errorf("change-password while impersonating: %d", st)
	}
	if st, _ := svc.do(http.MethodPost, "/api/auth/impersonate", map[string]string{"username": e.prefix + "dev"}); st != 403 {
		t.Errorf("nested impersonate: %d", st)
	}

	// Stop: back to the real user; audit says stop.
	if st, body := svc.do(http.MethodPost, "/api/auth/impersonate/stop", map[string]string{}); st != 200 || body["user"].(map[string]any)["username"] != e.prefix+"svc" {
		t.Fatalf("stop: %d %v", st, body)
	}
	if st, _ := svc.do(http.MethodPost, "/api/auth/impersonate/stop", map[string]string{}); st != 400 {
		t.Errorf("second stop: %d", st)
	}
	if got := e.auditReasons(qaID); len(got) != 1 || got[0] != "stop" {
		t.Errorf("audit after stop: %v", got)
	}

	// Target deactivated mid-session → falls back to the real user.
	svc.do(http.MethodPost, "/api/auth/impersonate", map[string]string{"username": e.prefix + "qa"})
	_, _ = e.sql.Exec(`UPDATE users SET active = 0 WHERE id = ?`, qaID)
	if _, who, imp := svc.me(); who != e.prefix+"svc" || imp != "" {
		t.Errorf("deactivated target: got %s/%s", who, imp)
	}
	_, _ = e.sql.Exec(`UPDATE users SET active = 1 WHERE id = ?`, qaID)

	// Expired row → real user; a new start over it leaves the old audit NULL.
	_, _ = e.sql.Exec(`UPDATE active_impersonations SET expires_at = ? WHERE parent_token = ?`, time.Now().UTC().Add(-time.Minute), parentToken)
	if _, who, imp := svc.me(); who != e.prefix+"svc" || imp != "" {
		t.Errorf("expired: got %s/%s", who, imp)
	}
	if st, _ := svc.do(http.MethodPost, "/api/auth/impersonate", map[string]string{"username": e.prefix + "qa"}); st != 200 {
		t.Fatalf("restart over expired row: %d", st)
	}
	// Direct second start on a live row supersedes it.
	if _, _, err := auth.StartImpersonation(context.Background(), e.sql, parentToken, time.Now().Add(time.Hour), "x", qaID, "", ""); err != nil {
		t.Fatal(err)
	}
	if got := e.auditReasons(qaID); strings.Join(got, ",") != "stop,NULL,superseded,NULL" {
		t.Errorf("audit reasons: %v", got)
	}

	// Impersonator demoted → falls back (row itself still live).
	_, _ = e.sql.Exec(`UPDATE users SET role = 'QA' WHERE username = ?`, e.prefix+"svc")
	if _, who, imp := svc.me(); who != e.prefix+"svc" || imp != "" {
		t.Errorf("demoted impersonator: got %s/%s", who, imp)
	}
	_, _ = e.sql.Exec(`UPDATE users SET role = 'Service' WHERE username = ?`, e.prefix+"svc")

	// Logout while impersonating kills the real session too.
	if st, _ := svc.do(http.MethodPost, "/api/auth/logout", map[string]string{}); st != 204 {
		t.Fatalf("logout: %d", st)
	}
	if _, err := auth.ValidateSession(context.Background(), e.sql, parentToken); err != auth.ErrSessionInvalid {
		t.Errorf("parent token after logout: %v", err)
	}
	var n int
	_ = e.sql.QueryRow(`SELECT COUNT(*) FROM active_impersonations WHERE parent_token = ?`, parentToken).Scan(&n)
	if n != 0 {
		t.Errorf("active row left after logout")
	}
}

func TestImpersonationIT_ServiceSeed(t *testing.T) {
	e := setupImpersonationIT(t)
	ctx := context.Background()
	long := strings.Repeat("s", 40)

	// Short password refused.
	if _, err := seed.SeedUsers(ctx, e.sql, seed.Options{ServiceUsername: e.prefix + "short", ServicePassword: "tooshort"}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = e.sql.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ?`, e.prefix+"short").Scan(&n)
	if n != 0 {
		t.Error("short password must not seed")
	}

	// Created as Service, no forced password change; trims and lowercases.
	if _, err := seed.SeedUsers(ctx, e.sql, seed.Options{ServiceUsername: " " + strings.ToUpper(e.prefix) + "AI ", ServicePassword: long}); err != nil {
		t.Fatal(err)
	}
	var role string
	var must bool
	if err := e.sql.QueryRow(`SELECT role, must_change_pw FROM users WHERE username = ?`, e.prefix+"ai").Scan(&role, &must); err != nil {
		t.Fatal(err)
	}
	if role != "Service" || must {
		t.Errorf("service row: role=%s must_change_pw=%v", role, must)
	}

	// Existing human account is never promoted.
	e.user("human", "QA", true)
	if _, err := seed.SeedUsers(ctx, e.sql, seed.Options{ServiceUsername: e.prefix + "human", ServicePassword: long}); err != nil {
		t.Fatal(err)
	}
	_ = e.sql.QueryRow(`SELECT role FROM users WHERE username = ?`, e.prefix+"human").Scan(&role)
	if role != "QA" {
		t.Errorf("human promoted to %s", role)
	}

	// Admin-path upsert (same update as the Neo4j path) never demotes Service.
	if _, err := seed.SeedUsers(ctx, e.sql, seed.Options{AdminUsername: e.prefix + "ai", AdminPassword: "x"}); err != nil {
		t.Fatal(err)
	}
	_ = e.sql.QueryRow(`SELECT role FROM users WHERE username = ?`, e.prefix+"ai").Scan(&role)
	if role != "Service" {
		t.Errorf("service demoted to %s", role)
	}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
