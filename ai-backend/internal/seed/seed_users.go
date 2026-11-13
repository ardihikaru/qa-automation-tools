// Package seed pulls user identity from taskmgr Neo4j into qa-automation's
// MySQL `users` table. One-shot at startup; safe to re-run (upsert by
// username, never overwrites an existing row's password).
package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/qa-automation/ai-backend/internal/auth"
)

// Options is sourced from env vars in main.go.
type Options struct {
	// Taskmgr Neo4j connection (read-only)
	Neo4jURI      string
	Neo4jUser     string
	Neo4jPassword string
	Neo4jDatabase string

	// Admin fallback when Neo4j is unreachable. Both required.
	AdminUsername string
	AdminPassword string

	// AI service account (role "Service", may impersonate). Both required.
	ServiceUsername string
	ServicePassword string
}

// minServicePasswordLen: the Service account is Admin-equivalent through
// impersonation, so a short password is refused.
const minServicePasswordLen = 32

// SeedUsers tries Neo4j first; falls back to admin-only on any failure so the
// app still boots. Returns the count of users that ended up in the table
// (existing + newly inserted), useful for the startup log line.
func SeedUsers(ctx context.Context, db *sql.DB, opts Options) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("nil db")
	}

	added := 0
	if opts.Neo4jURI != "" {
		n, err := seedFromNeo4j(ctx, db, opts)
		if err != nil {
			log.Printf("⚠️  seed: taskmgr Neo4j unreachable (%v) — falling back to admin only", err)
		} else {
			added += n
		}
	}

	if opts.AdminUsername != "" && opts.AdminPassword != "" {
		n, err := upsertAdmin(ctx, db, opts.AdminUsername, opts.AdminPassword)
		if err != nil {
			return added, fmt.Errorf("admin seed: %w", err)
		}
		added += n
	}

	if opts.ServiceUsername != "" && opts.ServicePassword != "" {
		n, err := upsertServiceUser(ctx, db, opts.ServiceUsername, opts.ServicePassword)
		if err != nil {
			return added, fmt.Errorf("service seed: %w", err)
		}
		added += n
	}

	total, err := countUsers(ctx, db)
	if err != nil {
		return added, err
	}
	return total, nil
}

func seedFromNeo4j(ctx context.Context, db *sql.DB, opts Options) (int, error) {
	drv, err := neo4j.NewDriverWithContext(opts.Neo4jURI,
		neo4j.BasicAuth(opts.Neo4jUser, opts.Neo4jPassword, ""))
	if err != nil {
		return 0, fmt.Errorf("driver: %w", err)
	}
	defer drv.Close(ctx)

	session := drv.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: opts.Neo4jDatabase,
		AccessMode:   neo4j.AccessModeRead,
	})
	defer session.Close(ctx)

	const cypher = `
		MATCH (u:User)
		WHERE u.active = true AND u.username <> 'sampleMentor123'
		RETURN u.id           AS id,
		       u.username     AS username,
		       u.role         AS role,
		       coalesce(u.isLead, false) AS isLead,
		       coalesce(u.whatsAppNumber, '') AS whatsapp,
		       coalesce(u.odooID, '') AS odooID
	`
	res, err := session.Run(ctx, cypher, nil)
	if err != nil {
		return 0, fmt.Errorf("run cypher: %w", err)
	}

	inserted := 0
	for res.Next(ctx) {
		rec := res.Record()
		username := stringValue(rec, "username")
		if username == "" {
			continue
		}
		ok, err := upsertUser(ctx, db, userRow{
			ID:           stringValue(rec, "id"),
			Username:     username,
			Role:         normalizeRole(stringValue(rec, "role")),
			IsLead:       boolValue(rec, "isLead"),
			WhatsApp:     stringValue(rec, "whatsapp"),
			OdooID:       stringValue(rec, "odooID"),
			Password:     username, // initial password = username; force-change flag on
			MustChangePW: true,
		})
		if err != nil {
			return inserted, fmt.Errorf("upsert %s: %w", username, err)
		}
		if ok {
			inserted++
		}
	}
	if err := res.Err(); err != nil {
		return inserted, fmt.Errorf("iterate results: %w", err)
	}
	log.Printf("✅ seed: inserted %d users from taskmgr Neo4j", inserted)
	return inserted, nil
}

func upsertAdmin(ctx context.Context, db *sql.DB, username, password string) (int, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	ok, err := upsertUser(ctx, db, userRow{
		Username:     username,
		Role:         "Admin",
		IsLead:       true,
		Password:     password,
		MustChangePW: true,
	})
	if err != nil {
		return 0, err
	}
	if ok {
		log.Printf("✅ seed: inserted admin user %q", username)
		return 1, nil
	}
	return 0, nil
}

// upsertServiceUser creates the AI service account (role "Service",
// must_change_pw=0). It never overwrites an existing password and never
// promotes an existing non-Service account.
func upsertServiceUser(ctx context.Context, db *sql.DB, username, password string) (int, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if len(password) < minServicePasswordLen {
		log.Printf("⚠️  seed: AI_SERVICE_PASSWORD shorter than %d characters — service user %q not seeded", minServicePasswordLen, username)
		return 0, nil
	}
	var role string
	err := db.QueryRowContext(ctx, `SELECT role FROM users WHERE username = ? LIMIT 1`, username).Scan(&role)
	switch {
	case err == nil && role == "Service":
		return 0, nil
	case err == nil:
		log.Printf("❌ seed: username %q already exists with role %q — refusing to turn it into the Service account; pick another AI_SERVICE_USERNAME", username, role)
		return 0, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("lookup service user: %w", err)
	}
	ok, err := upsertUser(ctx, db, userRow{
		Username:     username,
		Role:         "Service",
		Password:     password,
		MustChangePW: false,
	})
	if err != nil {
		return 0, err
	}
	if ok {
		log.Printf("✅ seed: inserted service user %q", username)
		return 1, nil
	}
	return 0, nil
}

func countUsers(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// stringValue extracts a string-typed key, returning "" if absent or wrong type.
func stringValue(rec *neo4j.Record, key string) string {
	v, ok := rec.Get(key)
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func boolValue(rec *neo4j.Record, key string) bool {
	v, ok := rec.Get(key)
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

// normalizeRole maps taskmgr role labels to the strings we store in MySQL.
// Unknown roles fall back to "Developer" so the user can still log in.
// "Service" is deliberately NOT accepted: only upsertServiceUser writes it,
// so taskmgr can never grant impersonation rights.
func normalizeRole(in string) string {
	switch in {
	case "Admin", "Developer", "DevOps", "QA", "QC", "UI/UX Designer":
		return in
	}
	return "Developer"
}

// hashForSeed avoids importing auth twice from this file.
func hashForSeed(plain string) (string, error) { return auth.Hash(plain) }

// envOrDefault helper kept here so main.go stays tidy.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
