package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/navmap"
)

// NAVIGATION_MAP.md section 5: the route inventory is built from the parsed catalog and the repo
// files the sync read, after the catalog transaction, in a transaction of its own. A failure keeps
// the previous inventory and never changes the sync's status.

const (
	navInsertBatch = 300
	navMaxProblems = 50
	navTimeout     = time.Minute
)

// navInput converts a parsed snapshot and the read files to the map's input.
func navInput(snap *Snapshot, files map[string][]byte) navmap.Input {
	in := navmap.Input{Files: files}
	for _, f := range snap.Features {
		in.Features = append(in.Features, navmap.Feature{Code: f.Code, Name: f.Name, RepoEntry: f.RepoEntry})
	}
	for _, e := range snap.Epics {
		in.Epics = append(in.Epics, navmap.Epic{Code: e.Code, Service: e.Service})
	}
	for _, s := range snap.Stories {
		in.Stories = append(in.Stories, navmap.Story{Code: s.Code, FeatureCodes: s.FeatureCodes})
	}
	for _, c := range snap.TestCases {
		in.Cases = append(in.Cases, navmap.Case{Code: c.Code, EpicCode: c.EpicCode, StoryCode: c.StoryCode, Scenario: c.Scenario, Polarity: c.Polarity, SpecPaths: c.SpecPaths})
	}
	return in
}

// buildNavSafe runs navmap.Build, turning a panic into an error.
func buildNavSafe(in navmap.Input) (inv navmap.Inventory, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("build panicked: %v", r)
		}
	}()
	return navmap.Build(in), nil
}

// navStale reports whether the product's stored inventory is missing or from another navmap version.
func (s *Syncer) navStale(ctx context.Context, productID int) bool {
	var v int
	err := s.DB.QueryRow(ctx, `SELECT nav_version FROM nav_builds WHERE product_id = ?`, productID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		// The table may be missing before the migration: do not try to build.
		return false
	}
	return v != navmap.Version
}

// buildNav builds and stores the inventory. It never returns an error: a failure is logged and
// recorded in nav_builds, and the previous inventory stays.
func (s *Syncer) buildNav(syncID int64, productID int, sha string, snap *Snapshot, files map[string][]byte) {
	ctx, cancel := context.WithTimeout(context.Background(), navTimeout)
	defer cancel()
	inv, err := buildNavSafe(navInput(snap, files))
	if err == nil {
		err = s.DB.WithTx(ctx, func(tx *sql.Tx) error { return storeNav(ctx, tx, productID, syncID, sha, inv) })
	}
	if err == nil {
		log.Printf("knowledge: sync %d (product %d): navigation map: %d routes, %d links", syncID, productID, len(inv.Routes), len(inv.Links))
		return
	}
	log.Printf("knowledge: sync %d (product %d): navigation map: %v", syncID, productID, err)
	if _, werr := s.DB.Exec(ctx, `INSERT INTO nav_builds (product_id, sync_id, commit_sha, nav_version, status, error, built_at)
VALUES (?, ?, ?, ?, 'failed', ?, UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE sync_id = VALUES(sync_id), commit_sha = VALUES(commit_sha), nav_version = VALUES(nav_version),
  status = 'failed', error = VALUES(error), built_at = VALUES(built_at)`,
		productID, syncID, nullIfEmpty(sha), navmap.Version, clipRunes(err.Error(), 500)); werr != nil {
		log.Printf("knowledge: sync %d (product %d): navigation map: record failure: %v", syncID, productID, werr)
	}
}

// storeNav replaces the product's inventory and records the build.
func storeNav(ctx context.Context, tx *sql.Tx, productID int, syncID int64, sha string, inv navmap.Inventory) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM nav_links WHERE product_id = ?`, productID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM nav_routes WHERE product_id = ?`, productID); err != nil {
		return err
	}
	for i := 0; i < len(inv.Routes); i += navInsertBatch {
		end := min(i+navInsertBatch, len(inv.Routes))
		var q strings.Builder
		q.WriteString(`INSERT INTO nav_routes (product_id, route_key, surface, app, path, methods, sources, page_objects) VALUES `)
		args := make([]any, 0, (end-i)*8)
		for j, r := range inv.Routes[i:end] {
			if j > 0 {
				q.WriteString(",")
			}
			q.WriteString("(?, ?, ?, ?, ?, ?, ?, ?)")
			args = append(args, productID, r.Key, r.Surface, clipRunes(r.App, 100), r.Path, jsonList(r.Methods), jsonList(r.Sources), jsonList(r.PageObjects))
		}
		if _, err := tx.ExecContext(ctx, q.String(), args...); err != nil {
			return err
		}
	}
	for i := 0; i < len(inv.Links); i += navInsertBatch {
		end := min(i+navInsertBatch, len(inv.Links))
		var q strings.Builder
		q.WriteString(`INSERT IGNORE INTO nav_links (product_id, route_key, ref_type, ref, source, method) VALUES `)
		args := make([]any, 0, (end-i)*6)
		for j, l := range inv.Links[i:end] {
			if j > 0 {
				q.WriteString(",")
			}
			q.WriteString("(?, ?, ?, ?, ?, ?)")
			args = append(args, productID, l.RouteKey, l.RefType, clipRunes(l.Ref, 300), l.Source, l.Method)
		}
		if _, err := tx.ExecContext(ctx, q.String(), args...); err != nil {
			return err
		}
	}
	probs := inv.Problems
	if len(probs) > navMaxProblems {
		probs = probs[:navMaxProblems]
	}
	pj := jsonList(probs)
	_, err := tx.ExecContext(ctx, `INSERT INTO nav_builds (product_id, sync_id, commit_sha, nav_version, status, error, routes, links, problems, built_at)
VALUES (?, ?, ?, ?, 'ok', '', ?, ?, ?, UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE sync_id = VALUES(sync_id), commit_sha = VALUES(commit_sha), nav_version = VALUES(nav_version),
  status = 'ok', error = '', routes = VALUES(routes), links = VALUES(links), problems = VALUES(problems), built_at = VALUES(built_at)`,
		productID, syncID, nullIfEmpty(sha), navmap.Version, len(inv.Routes), len(inv.Links), pj)
	return err
}

func jsonList(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}
