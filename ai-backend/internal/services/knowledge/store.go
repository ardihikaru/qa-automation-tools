package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// table describes one entity table: its name and the type-specific columns, in write order.
type table struct {
	name string
	cols []string
}

var tables = map[string]table{
	TypePart:      {"kb_parts", []string{"name"}},
	TypeModule:    {"kb_modules", []string{"part_code", "name"}},
	TypeSubmodule: {"kb_submodules", []string{"module_code", "name"}},
	TypeFeature:   {"kb_features", []string{"module_code", "submodule_code", "name", "repo_entry", "release_status", "status_as_of", "prod_since", "evidence", "notes"}},
	TypeEpic:      {"kb_epics", []string{"title", "service", "area"}},
	TypeStory:     {"kb_stories", []string{"epic_code", "text", "unmapped_reason"}},
	TypeAssistant: {"kb_assistants", []string{"kind", "name", "description", "path", "frontmatter", "body"}},
	TypeTestCase:  {"kb_test_cases", []string{"epic_code", "story_code", "scenario", "polarity", "level", "priority", "status", "automated", "spec_paths", "reason_category", "reason_detail"}},
}

// Table returns the table name of an entity type ("" when unknown).
func Table(entityType string) string { return tables[entityType].name }

// row is one entity ready to write. hashExtra holds data that is part of the content but not
// a column of the row (a story's feature links).
type row struct {
	code      string
	vals      []any
	hashExtra any
	file      string
	line      int
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r row) hash() string {
	b, _ := json.Marshal([]any{r.vals, r.hashExtra})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func storyRow(s Story) row {
	fc := append([]string{}, s.FeatureCodes...)
	sort.Strings(fc)
	return row{code: s.Code, vals: []any{s.EpicCode, s.Text, nullIfEmpty(s.UnmappedReason)}, hashExtra: fc, file: s.SourceFile, line: s.SourceLine}
}

func testCaseRow(c TestCase) row {
	specs := c.SpecPaths
	if specs == nil {
		specs = []string{}
	}
	sp, _ := json.Marshal(specs)
	auto := 0
	if c.Automated {
		auto = 1
	}
	return row{code: c.Code, vals: []any{c.EpicCode, nullIfEmpty(c.StoryCode), c.Scenario, c.Polarity, c.Level, c.Priority, c.Status, auto, string(sp), c.ReasonCategory, c.ReasonDetail}, file: c.SourceFile, line: c.SourceLine}
}

// rowsOf turns a snapshot into rows per entity type.
func rowsOf(s *Snapshot) map[string][]row {
	out := map[string][]row{}
	for _, x := range s.Parts {
		out[TypePart] = append(out[TypePart], row{code: x.Code, vals: []any{x.Name}, file: x.SourceFile})
	}
	for _, x := range s.Modules {
		out[TypeModule] = append(out[TypeModule], row{code: x.Code, vals: []any{x.PartCode, x.Name}, file: x.SourceFile, line: x.SourceLine})
	}
	for _, x := range s.Submodules {
		out[TypeSubmodule] = append(out[TypeSubmodule], row{code: x.Code, vals: []any{x.ModuleCode, x.Name}})
	}
	for _, x := range s.Features {
		out[TypeFeature] = append(out[TypeFeature], row{code: x.Code, vals: []any{x.ModuleCode, nullIfEmpty(x.SubmoduleCode), x.Name, x.RepoEntry, x.ReleaseStatus, x.StatusAsOf, x.ProdSince, x.Evidence, x.Notes}, file: x.SourceFile, line: x.SourceLine})
	}
	for _, x := range s.Epics {
		out[TypeEpic] = append(out[TypeEpic], row{code: x.Code, vals: []any{x.Title, x.Service, x.Area}, file: x.SourceFile})
	}
	for _, x := range s.Stories {
		out[TypeStory] = append(out[TypeStory], storyRow(x))
	}
	for _, x := range s.TestCases {
		out[TypeTestCase] = append(out[TypeTestCase], testCaseRow(x))
	}
	for _, x := range s.Assistants {
		fm := string(x.Frontmatter)
		if fm == "" {
			fm = "{}"
		}
		out[TypeAssistant] = append(out[TypeAssistant], row{code: x.Code, vals: []any{x.Kind, x.Name, x.Description, clipRunes(x.Path, 300), fm, x.Body}, file: x.Path})
	}
	return out
}

// Diff is the change set of one sync (kb_syncs.diff).
type Diff struct {
	Unchanged bool                           `json:"unchanged"`
	Total     map[string]int                 `json:"total"`
	ByType    map[string]map[string][]string `json:"by_type"`
	Truncated bool                           `json:"truncated"`
}

const diffListCap = 200

func newDiff() *Diff {
	return &Diff{Total: map[string]int{"added": 0, "updated": 0, "removed": 0, "restored": 0}, ByType: map[string]map[string][]string{}}
}

func (d *Diff) add(entityType, kind, code string) {
	d.Total[kind]++
	m := d.ByType[entityType]
	if m == nil {
		m = map[string][]string{}
		d.ByType[entityType] = m
	}
	if len(m[kind]) >= diffListCap {
		d.Truncated = true
		return
	}
	m[kind] = append(m[kind], code)
}

// Empty reports whether nothing changed.
func (d *Diff) Empty() bool {
	return d.Total["added"]+d.Total["updated"]+d.Total["removed"]+d.Total["restored"] == 0
}

type existing struct {
	origin  string
	hash    string
	removed bool
	status  string // release_status; features only
	file    sql.NullString
	line    sql.NullInt64
}

const batchSize = 200

func placeholders(n, width int) string {
	one := "(" + strings.TrimSuffix(strings.Repeat("?,", width), ",") + ")"
	return strings.TrimSuffix(strings.Repeat(one+",", n), ",")
}

// upsert writes rows with INSERT … ON DUPLICATE KEY UPDATE in batches. full=true updates content
// and clears removal; full=false only refreshes the location columns.
func upsert(ctx context.Context, tx *sql.Tx, productID int, syncID int64, t table, rows []row, full bool) error {
	cols := append([]string{"product_id", "code"}, t.cols...)
	cols = append(cols, "origin", "content_hash", "first_sync_id", "changed_sync_id", "source_file", "source_line")
	var upd []string
	if full {
		for _, c := range t.cols {
			upd = append(upd, fmt.Sprintf("%s = VALUES(%s)", c, c))
		}
		upd = append(upd, "content_hash = VALUES(content_hash)", "changed_sync_id = VALUES(changed_sync_id)", "removed_at = NULL", "removed_sync_id = NULL")
	}
	upd = append(upd, "source_file = VALUES(source_file)", "source_line = VALUES(source_line)")
	for start := 0; start < len(rows); start += batchSize {
		batch := rows[start:min(start+batchSize, len(rows))]
		args := make([]any, 0, len(batch)*len(cols))
		for _, r := range batch {
			args = append(args, productID, r.code)
			args = append(args, r.vals...)
			var line any
			if r.line > 0 {
				line = r.line
			}
			args = append(args, "sync", r.hash(), syncID, syncID, nullIfEmpty(clipRunes(r.file, 300)), line)
		}
		q := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s ON DUPLICATE KEY UPDATE %s",
			t.name, strings.Join(cols, ", "), placeholders(len(batch), len(cols)), strings.Join(upd, ", "))
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("upsert %s: %w", t.name, err)
		}
	}
	return nil
}

func inList(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

// apply writes the snapshot of one sync inside tx and returns the diff. Unchanged rows are not
// written; rows with only a new location get their location columns refreshed without a diff entry.
//
// sha is the commit the sync read (kb_syncs.commit_sha is written only at the end of the
// transaction). firstSync is true when the product has no earlier succeeded sync; features added
// then get source='backfill' history rows because the first observation is not a change date.
func apply(ctx context.Context, tx *sql.Tx, productID int, syncID int64, sha string, firstSync bool, snap *Snapshot, prob func(Problem)) (*Diff, error) {
	diff := newDiff()
	byType := rowsOf(snap)
	for _, et := range EntityTypes {
		t := tables[et]
		have := map[string]existing{}
		statusCol := "'' AS release_status"
		if et == TypeFeature {
			statusCol = "release_status"
		}
		rs, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT code, origin, content_hash, removed_at IS NOT NULL, %s, source_file, source_line FROM %s WHERE product_id = ?", statusCol, t.name), productID)
		if err != nil {
			return nil, err
		}
		for rs.Next() {
			var code string
			var e existing
			if err := rs.Scan(&code, &e.origin, &e.hash, &e.removed, &e.status, &e.file, &e.line); err != nil {
				rs.Close()
				return nil, err
			}
			have[code] = e
		}
		rs.Close()
		if err := rs.Err(); err != nil {
			return nil, err
		}

		seen := map[string]bool{}
		var write, relocate []row
		var hist []statusEvent
		for _, r := range byType[et] {
			seen[r.code] = true
			e, ok := have[r.code]
			switch {
			case !ok:
				write = append(write, r)
				diff.add(et, "added", r.code)
				if et == TypeFeature {
					src := "sync"
					if firstSync {
						src = "backfill"
					}
					hist = append(hist, statusEvent{code: r.code, status: featureStatus(r), source: src})
				}
			case e.origin == "local":
				prob(Problem{File: r.file, Line: r.line, Msg: fmt.Sprintf("%s %s exists as a local draft; repo entry ignored", et, r.code)})
			case e.removed:
				write = append(write, r)
				diff.add(et, "restored", r.code)
				if et == TypeFeature && featureStatus(r) != e.status {
					hist = append(hist, statusEvent{code: r.code, status: featureStatus(r), prev: e.status, hasPrev: true, source: "sync"})
				}
			case e.hash != r.hash():
				write = append(write, r)
				diff.add(et, "updated", r.code)
				if et == TypeFeature && featureStatus(r) != e.status {
					hist = append(hist, statusEvent{code: r.code, status: featureStatus(r), prev: e.status, hasPrev: true, source: "sync"})
				}
			case e.file.String != clipRunes(r.file, 300) || int(e.line.Int64) != r.line:
				relocate = append(relocate, r)
			}
		}
		if err := upsert(ctx, tx, productID, syncID, t, write, true); err != nil {
			return nil, err
		}
		if err := upsert(ctx, tx, productID, syncID, t, relocate, false); err != nil {
			return nil, err
		}
		if err := insertStatusHistory(ctx, tx, productID, syncID, sha, hist); err != nil {
			return nil, err
		}
		var gone []string
		for code, e := range have {
			if e.origin == "sync" && !e.removed && !seen[code] {
				gone = append(gone, code)
			}
		}
		sort.Strings(gone)
		for _, c := range gone {
			diff.add(et, "removed", c)
		}
		for start := 0; start < len(gone); start += batchSize {
			b := gone[start:min(start+batchSize, len(gone))]
			args := []any{syncID, productID}
			for _, c := range b {
				args = append(args, c)
			}
			q := fmt.Sprintf("UPDATE %s SET removed_at = UTC_TIMESTAMP(), removed_sync_id = ? WHERE product_id = ? AND origin = 'sync' AND removed_at IS NULL AND code IN (%s)", t.name, inList(len(b)))
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				return nil, fmt.Errorf("remove %s: %w", t.name, err)
			}
		}
	}
	if err := applyLinks(ctx, tx, productID, snap); err != nil {
		return nil, err
	}
	return diff, nil
}

// statusEvent is one kb_feature_status_history row to write.
type statusEvent struct {
	code    string
	status  string
	prev    string
	hasPrev bool
	source  string // 'sync' or 'backfill'
}

// featureStatus is the release status of a feature row (vals order: module_code, submodule_code,
// name, repo_entry, release_status, ...; see rowsOf).
func featureStatus(r row) string {
	s, _ := r.vals[4].(string)
	return s
}

// insertStatusHistory writes the history rows of one sync inside the sync's transaction. The
// unique key (product_id, feature_code, sync_id, source) plus ON DUPLICATE KEY UPDATE id = id
// makes a re-executed write a no-op (INSERT IGNORE would also hide truncation and FK errors).
func insertStatusHistory(ctx context.Context, tx *sql.Tx, productID int, syncID int64, sha string, evs []statusEvent) error {
	for start := 0; start < len(evs); start += batchSize {
		batch := evs[start:min(start+batchSize, len(evs))]
		var sb strings.Builder
		args := make([]any, 0, len(batch)*6)
		for i, e := range batch {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString("(?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(), ?)")
			var prev any
			if e.hasPrev {
				prev = e.prev
			}
			args = append(args, productID, e.code, e.status, prev, syncID, nullIfEmpty(sha), e.source)
		}
		q := "INSERT INTO kb_feature_status_history (product_id, feature_code, release_status, prev_status, sync_id, commit_sha, changed_at, source) VALUES " +
			sb.String() + " ON DUPLICATE KEY UPDATE id = id"
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("insert status history: %w", err)
		}
	}
	return nil
}

type link struct{ story, feature string }

// applyLinks applies only the difference between the snapshot's story→feature links and the
// stored origin='sync' links.
func applyLinks(ctx context.Context, tx *sql.Tx, productID int, snap *Snapshot) error {
	want := map[link]bool{}
	for _, s := range snap.Stories {
		for _, f := range s.FeatureCodes {
			want[link{s.Code, f}] = true
		}
	}
	have := map[link]bool{}
	rs, err := tx.QueryContext(ctx, `SELECT story_code, feature_code FROM kb_story_features WHERE product_id = ? AND origin = 'sync'`, productID)
	if err != nil {
		return err
	}
	for rs.Next() {
		var l link
		if err := rs.Scan(&l.story, &l.feature); err != nil {
			rs.Close()
			return err
		}
		have[l] = true
	}
	rs.Close()
	var del, add []link
	for l := range have {
		if !want[l] {
			del = append(del, l)
		}
	}
	for l := range want {
		if !have[l] {
			add = append(add, l)
		}
	}
	for start := 0; start < len(del); start += batchSize {
		b := del[start:min(start+batchSize, len(del))]
		args := []any{productID}
		for _, l := range b {
			args = append(args, l.story, l.feature)
		}
		q := `DELETE FROM kb_story_features WHERE product_id = ? AND origin = 'sync' AND (story_code, feature_code) IN (` + placeholders(len(b), 2) + `)`
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("delete links: %w", err)
		}
	}
	for start := 0; start < len(add); start += batchSize {
		b := add[start:min(start+batchSize, len(add))]
		args := []any{}
		for _, l := range b {
			args = append(args, productID, l.story, l.feature, "sync")
		}
		q := `INSERT IGNORE INTO kb_story_features (product_id, story_code, feature_code, origin) VALUES ` + placeholders(len(b), 4)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("insert links: %w", err)
		}
	}
	return nil
}

// Counts are the active synced rows of a product (kb_syncs.counts and the summary).
type Counts struct {
	Parts               int `json:"parts"`
	Modules             int `json:"modules"`
	Submodules          int `json:"submodules"`
	Features            int `json:"features"`
	FeaturesWithStories int `json:"features_with_stories"`
	Epics               int `json:"epics"`
	Stories             int `json:"stories"`
	StoriesUnmapped     int `json:"stories_unmapped"`
	TestCases           int `json:"test_cases"`
	Automated           int `json:"automated"`
	Agents              int `json:"agents"`
	Skills              int `json:"skills"`
	Rules               int `json:"rules"`
	Commands            int `json:"commands"`
	Instructions        int `json:"instructions"`
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func countActive(ctx context.Context, q querier, productID int) (Counts, error) {
	var c Counts
	act := " WHERE product_id = ? AND origin = 'sync' AND removed_at IS NULL"
	err := q.QueryRowContext(ctx, `SELECT
  (SELECT COUNT(*) FROM kb_parts`+act+`),
  (SELECT COUNT(*) FROM kb_modules`+act+`),
  (SELECT COUNT(*) FROM kb_submodules`+act+`),
  (SELECT COUNT(*) FROM kb_features`+act+`),
  (SELECT COUNT(DISTINCT sf.feature_code) FROM kb_story_features sf
     JOIN kb_stories s ON s.product_id = sf.product_id AND s.code = sf.story_code AND s.removed_at IS NULL AND s.origin = 'sync'
     JOIN kb_features f ON f.product_id = sf.product_id AND f.code = sf.feature_code AND f.removed_at IS NULL
     WHERE sf.product_id = ? AND sf.origin = 'sync'),
  (SELECT COUNT(*) FROM kb_epics`+act+`),
  (SELECT COUNT(*) FROM kb_stories`+act+`),
  (SELECT COUNT(*) FROM kb_stories`+act+` AND unmapped_reason IS NOT NULL),
  (SELECT COUNT(*) FROM kb_test_cases`+act+`),
  (SELECT COUNT(*) FROM kb_test_cases`+act+` AND automated = 1),
  (SELECT COUNT(*) FROM kb_assistants`+act+` AND kind = 'agent'),
  (SELECT COUNT(*) FROM kb_assistants`+act+` AND kind = 'skill'),
  (SELECT COUNT(*) FROM kb_assistants`+act+` AND kind = 'rule'),
  (SELECT COUNT(*) FROM kb_assistants`+act+` AND kind = 'command'),
  (SELECT COUNT(*) FROM kb_assistants`+act+` AND kind = 'instructions')`,
		productID, productID, productID, productID, productID, productID, productID, productID, productID, productID,
		productID, productID, productID, productID, productID,
	).Scan(&c.Parts, &c.Modules, &c.Submodules, &c.Features, &c.FeaturesWithStories, &c.Epics, &c.Stories, &c.StoriesUnmapped, &c.TestCases, &c.Automated,
		&c.Agents, &c.Skills, &c.Rules, &c.Commands, &c.Instructions)
	return c, err
}

// Problems is the stored shape of kb_syncs.problems.
type Problems struct {
	Total     int            `json:"total"`
	ByMessage map[string]int `json:"by_message"`
	Items     []Problem      `json:"items"`
}

const problemItemsCap = 500

var templRe = regexp.MustCompile(`\b(?:US|TC)-[A-Z]+-\d+\b|\b[A-D]\d{2}(?:\.\d+)*\b|"[^"]*"|\d+`)

func summarizeProblems(list []Problem) Problems {
	p := Problems{Total: len(list), ByMessage: map[string]int{}, Items: []Problem{}}
	for i, x := range list {
		msg := x.Msg
		if j := strings.Index(msg, " (also in"); j >= 0 {
			msg = msg[:j]
		}
		p.ByMessage[templRe.ReplaceAllString(msg, "#")]++
		if i < problemItemsCap {
			p.Items = append(p.Items, x)
		}
	}
	return p
}
