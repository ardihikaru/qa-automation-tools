package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Spec: docs/designs/MODULE_CHECKLIST_EXPORT.md.

// TemplateVersion is part of the export cache key. Raise it on any change that alters the output
// of any renderer (layout, columns, colours, wording, time zone); the golden-hash test fails when
// the output changes without a bump, including after a library upgrade.
const TemplateVersion = 3

// maxExportFeatures is the largest catalogue the export accepts.
const maxExportFeatures = 20000

var (
	// ErrNoSucceededSync means the product has no succeeded sync yet (HTTP 409).
	ErrNoSucceededSync = errors.New("no knowledge base sync has succeeded yet")
	// ErrExportTooLarge means the catalogue has more than maxExportFeatures features (HTTP 413).
	ErrExportTooLarge = errors.New("catalogue too large to export")
)

// Date sources of a "status since" value.
const (
	DateSourceSync     = "sync"
	DateSourceBackfill = "backfill"
)

// ChecklistRow is one feature line of the checklist.
type ChecklistRow struct {
	ID          string
	Submodule   string
	Feature     string
	Status      string
	ProdSince   string     // raw "Prod since" cell ("" when absent); rendered with prodSinceText
	StatusSince *time.Time // UTC; nil when unknown or the option is off
	DateSource  string     // DateSourceSync, DateSourceBackfill, or "" when there is no date
}

// ChecklistModule is one module and its feature rows.
type ChecklistModule struct {
	Code, Name string
	Rows       []ChecklistRow
}

// ChecklistPart is one part and its modules.
type ChecklistPart struct {
	Code, Name string
	Modules    []ChecklistModule
}

// ChecklistSnapshot is everything a renderer needs; renderers never read the database or the clock.
type ChecklistSnapshot struct {
	ProductName string
	SyncID      int64
	CommitSHA   string
	SyncedAt    time.Time // the sync's finished_at (UTC)
	Skipped     int       // orphan rows left out
	// HistoryMaxID is COALESCE(MAX(id), 0) of the product's status history, read in the same
	// transaction (0 when history was not requested). The cache key uses it.
	HistoryMaxID int64
	Parts        []ChecklistPart
	// Filter is the scope the rows were selected with (FilterChecklist); the zero value is the
	// full export. ExcludedUnknown counts PROD rows a ranged filter left out for lack of a period.
	Filter          ChecklistFilter
	ExcludedUnknown int
}

// FeatureCount is the number of feature rows in the snapshot.
func (s *ChecklistSnapshot) FeatureCount() int {
	n := 0
	for _, p := range s.Parts {
		for _, m := range p.Modules {
			n += len(m.Rows)
		}
	}
	return n
}

// KeyInfo is what the cache key is built from (one light read).
type KeyInfo struct {
	SyncID       int64
	ProductName  string
	HistoryMaxID int64 // COALESCE(MAX(id), 0); 0 when history was not requested
}

// pickStatusDate returns the first date of: a sync row, a backfill row, none.
func pickStatusDate(syncAt, backfillAt *time.Time) (*time.Time, string) {
	if syncAt != nil {
		return syncAt, DateSourceSync
	}
	if backfillAt != nil {
		return backfillAt, DateSourceBackfill
	}
	return nil, ""
}

var wib = time.FixedZone("WIB", 7*3600)

var idMonths = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// dateID formats a UTC instant as a WIB date, "01 Sep 2026".
func dateID(t time.Time) string {
	t = t.In(wib)
	return fmt.Sprintf("%02d %s %d", t.Day(), idMonths[t.Month()-1], t.Year())
}

// dateISO formats a UTC instant as a WIB date, "2026-09-01".
func dateISO(t time.Time) string { return t.In(wib).Format("2006-01-02") }

// codeName is a code with its name, as read from kb_parts, kb_modules and kb_submodules.
type codeName struct{ code, name string }

type checklistModuleIn struct{ code, part, name string }

type checklistFeatureIn struct{ code, module, submodule, name, status, prodSince string }

// statusDates holds, per feature code and status, the latest history date per source.
type statusDates map[string]map[string][2]*time.Time // code -> status -> [sync, backfill]

func (d statusDates) add(code, status, source string, at time.Time) {
	m := d[code]
	if m == nil {
		m = map[string][2]*time.Time{}
		d[code] = m
	}
	v := m[status]
	i := 0
	if source == DateSourceBackfill {
		i = 1
	}
	if v[i] == nil || at.After(*v[i]) {
		a := at
		v[i] = &a
	}
	m[status] = v
}

// checklistInput is the raw (already filtered and ordered) catalogue.
type checklistInput struct {
	parts      []codeName
	modules    []checklistModuleIn
	submodules map[string]string // code -> name (active only)
	features   []checklistFeatureIn
	dates      statusDates // nil when the option is off
}

// buildChecklist applies the orphan rule: a module whose part is not active, and a feature whose
// module is not active, are left out and counted in Skipped. A feature with an unknown
// sub-module keeps an empty sub-module cell. Input order is kept.
func buildChecklist(in checklistInput) (parts []ChecklistPart, skipped int) {
	partIdx := map[string]int{}
	for i, p := range in.parts {
		parts = append(parts, ChecklistPart{Code: p.code, Name: p.name})
		partIdx[p.code] = i
	}
	type ref struct{ p, m int }
	modIdx := map[string]ref{}
	for _, m := range in.modules {
		pi, ok := partIdx[m.part]
		if !ok {
			skipped++
			continue
		}
		parts[pi].Modules = append(parts[pi].Modules, ChecklistModule{Code: m.code, Name: m.name})
		modIdx[m.code] = ref{pi, len(parts[pi].Modules) - 1}
	}
	for _, f := range in.features {
		r, ok := modIdx[f.module]
		if !ok {
			skipped++
			continue
		}
		row := ChecklistRow{ID: f.code, Submodule: in.submodules[f.submodule], Feature: f.name, Status: f.status, ProdSince: f.prodSince}
		if in.dates != nil {
			v := in.dates[f.code][f.status]
			row.StatusSince, row.DateSource = pickStatusDate(v[0], v[1])
		}
		mod := &parts[r.p].Modules[r.m]
		mod.Rows = append(mod.Rows, row)
	}
	return parts, skipped
}

// roTx opens a read-only REPEATABLE READ transaction (DB.WithTx passes nil options).
func (s *Syncer) roTx(ctx context.Context) (*sql.Tx, error) {
	return s.DB.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
}

type latestSync struct {
	id         int64
	sha        sql.NullString
	startedAt  sql.NullTime
	finishedAt sql.NullTime
}

func readLatestSync(ctx context.Context, tx *sql.Tx, pid int) (latestSync, error) {
	var l latestSync
	err := tx.QueryRowContext(ctx, `SELECT id, commit_sha, started_at, finished_at FROM kb_syncs WHERE product_id = ? AND status = 'succeeded' ORDER BY id DESC LIMIT 1`, pid).
		Scan(&l.id, &l.sha, &l.startedAt, &l.finishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNoSucceededSync
	}
	return l, err
}

func readProductName(ctx context.Context, tx *sql.Tx, pid int) (string, error) {
	var name string
	err := tx.QueryRowContext(ctx, `SELECT name FROM products WHERE id = ?`, pid).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

// ProductExists returns ErrNotFound when the product does not exist (ErrUnavailable without a DB).
func (s *Syncer) ProductExists(ctx context.Context, pid int) error {
	if s == nil || s.DB == nil {
		return ErrUnavailable
	}
	return s.productExists(ctx, pid)
}

// ChecklistKeyInfo reads the latest succeeded sync id, the product name and (with withHistory)
// the history watermark in one small read-only transaction.
func (s *Syncer) ChecklistKeyInfo(ctx context.Context, pid int, withHistory bool) (KeyInfo, error) {
	var k KeyInfo
	tx, err := s.roTx(ctx)
	if err != nil {
		return k, err
	}
	defer tx.Rollback()
	if k.ProductName, err = readProductName(ctx, tx, pid); err != nil {
		return k, err
	}
	l, err := readLatestSync(ctx, tx, pid)
	if err != nil {
		return k, err
	}
	k.SyncID = l.id
	if withHistory {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM kb_feature_status_history WHERE product_id = ?`, pid).Scan(&k.HistoryMaxID); err != nil {
			return k, err
		}
	}
	return k, nil
}

// ChecklistSnapshot reads the catalogue of the latest succeeded sync inside one read-only
// REPEATABLE READ transaction, so the sync id and the rows are consistent with each other.
func (s *Syncer) ChecklistSnapshot(ctx context.Context, pid int, withHistory bool) (*ChecklistSnapshot, error) {
	tx, err := s.roTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	name, err := readProductName(ctx, tx, pid)
	if err != nil {
		return nil, err
	}
	l, err := readLatestSync(ctx, tx, pid)
	if err != nil {
		return nil, err
	}
	in := checklistInput{submodules: map[string]string{}}

	const active = ` WHERE product_id = ? AND origin = 'sync' AND removed_at IS NULL`
	rs, err := tx.QueryContext(ctx, `SELECT code, name FROM kb_parts`+active+` ORDER BY code`, pid)
	if err != nil {
		return nil, err
	}
	for rs.Next() {
		var c codeName
		if err := rs.Scan(&c.code, &c.name); err != nil {
			rs.Close()
			return nil, err
		}
		in.parts = append(in.parts, c)
	}
	if err := closeRows(rs); err != nil {
		return nil, err
	}

	if rs, err = tx.QueryContext(ctx, `SELECT code, part_code, name FROM kb_modules`+active+` ORDER BY code`, pid); err != nil {
		return nil, err
	}
	for rs.Next() {
		var m checklistModuleIn
		if err := rs.Scan(&m.code, &m.part, &m.name); err != nil {
			rs.Close()
			return nil, err
		}
		in.modules = append(in.modules, m)
	}
	if err := closeRows(rs); err != nil {
		return nil, err
	}

	if rs, err = tx.QueryContext(ctx, `SELECT code, module_code, name FROM kb_submodules`+active+` ORDER BY code`, pid); err != nil {
		return nil, err
	}
	for rs.Next() {
		var code, module, nm string
		if err := rs.Scan(&code, &module, &nm); err != nil {
			rs.Close()
			return nil, err
		}
		in.submodules[code] = nm
	}
	if err := closeRows(rs); err != nil {
		return nil, err
	}

	// LIMIT keeps an oversized catalogue from being loaded; one extra row proves it is too large.
	if rs, err = tx.QueryContext(ctx, `SELECT code, module_code, submodule_code, name, release_status, prod_since FROM kb_features`+active+` ORDER BY module_code, code LIMIT ?`, pid, maxExportFeatures+1); err != nil {
		return nil, err
	}
	for rs.Next() {
		var f checklistFeatureIn
		var sub, nm sql.NullString
		if err := rs.Scan(&f.code, &f.module, &sub, &nm, &f.status, &f.prodSince); err != nil {
			rs.Close()
			return nil, err
		}
		f.submodule, f.name = sub.String, nm.String
		in.features = append(in.features, f)
	}
	if err := closeRows(rs); err != nil {
		return nil, err
	}
	if len(in.features) > maxExportFeatures {
		return nil, ErrExportTooLarge
	}

	var histMax int64
	if withHistory {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM kb_feature_status_history WHERE product_id = ?`, pid).Scan(&histMax); err != nil {
			return nil, err
		}
		in.dates = statusDates{}
		rs, err = tx.QueryContext(ctx, `SELECT feature_code, release_status, source, MAX(changed_at) FROM kb_feature_status_history WHERE product_id = ? GROUP BY feature_code, release_status, source`, pid)
		if err != nil {
			return nil, err
		}
		for rs.Next() {
			var code, status, source string
			var at time.Time
			if err := rs.Scan(&code, &status, &source, &at); err != nil {
				rs.Close()
				return nil, err
			}
			in.dates.add(code, status, source, at)
		}
		if err := closeRows(rs); err != nil {
			return nil, err
		}
	}

	snap := &ChecklistSnapshot{ProductName: name, SyncID: l.id, CommitSHA: l.sha.String, HistoryMaxID: histMax}
	switch {
	case l.finishedAt.Valid:
		snap.SyncedAt = l.finishedAt.Time.UTC()
	case l.startedAt.Valid:
		snap.SyncedAt = l.startedAt.Time.UTC()
	}
	snap.Parts, snap.Skipped = buildChecklist(in)
	if snap.Skipped > 0 {
		log.Printf("knowledge export: skipped %d orphan rows, product=%d", snap.Skipped, pid)
	}
	return snap, nil
}

func closeRows(rs *sql.Rows) error {
	err := rs.Err()
	rs.Close()
	return err
}

// RenderChecklist renders the snapshot as "pdf", "xlsx" or "csv" and returns the bytes and the
// content type. The output is deterministic: it depends only on the snapshot.
func RenderChecklist(snap *ChecklistSnapshot, format string, withLastUpdate bool) ([]byte, string, error) {
	switch format {
	case "pdf":
		b, err := renderPDF(snap, withLastUpdate)
		return b, "application/pdf", err
	case "xlsx":
		b, err := renderXLSX(snap, withLastUpdate)
		return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", err
	case "csv":
		b, err := renderCSV(snap, withLastUpdate)
		return b, "text/csv; charset=utf-8", err
	}
	return nil, "", fmt.Errorf("unknown export format %q", format)
}

// statusCounts counts rows per release status in ReleaseStatuses order (other values under UNKNOWN).
func statusCounts(rows []ChecklistRow) []int {
	out := make([]int, len(ReleaseStatuses))
	for _, r := range rows {
		out[statusIndex(r.Status)]++
	}
	return out
}

func statusIndex(s string) int {
	for i, x := range ReleaseStatuses {
		if x == s {
			return i
		}
	}
	return len(ReleaseStatuses) - 1 // UNKNOWN
}

// guardFormula neutralises a text cell that a spreadsheet would read as a formula.
func guardFormula(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
