package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
)

// ErrNotFound means the product, entity or sync does not exist.
var ErrNotFound = errors.New("not found")

// InputError is a 400.
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

// ConflictError is a 409.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

func badInput(f string, a ...any) error { return &InputError{Msg: fmt.Sprintf(f, a...)} }

// Item is one JSON object returned by the API.
type Item = map[string]any

var jsonCols = map[string]bool{"spec_paths": true, "tags": true, "counts": true, "diff": true, "problems": true, "frontmatter": true}

// scanItems scans rows into maps keyed by column name. Text becomes string, JSON columns become
// json.RawMessage, NULL becomes nil.
func scanItems(rs *sql.Rows) ([]Item, error) {
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	out := []Item{}
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		it := Item{}
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				if jsonCols[c] {
					it[c] = json.RawMessage(append([]byte{}, v...))
				} else {
					it[c] = string(v)
				}
			default:
				it[c] = v
			}
		}
		out = append(out, it)
	}
	return out, rs.Err()
}

const commonCols = "code, origin, removed_at, removed_sync_id, changed_sync_id, first_sync_id, source_file, source_line"

func selectCols(et string) string {
	return commonCols + ", " + strings.Join(tables[et].cols, ", ")
}

func (s *Syncer) productExists(ctx context.Context, pid int) error {
	var x int
	err := s.DB.QueryRow(ctx, `SELECT id FROM products WHERE id = ?`, pid).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// decorate adds type, source_url and annotation to items of one entity type.
func (s *Syncer) decorate(ctx context.Context, pid int, et string, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	var repoURL, sha sql.NullString
	err := s.DB.QueryRow(ctx, `SELECT repo_url, commit_sha FROM kb_syncs WHERE product_id = ? AND status = 'succeeded' ORDER BY id DESC LIMIT 1`, pid).Scan(&repoURL, &sha)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	codes := make([]any, 0, len(items))
	for _, it := range items {
		codes = append(codes, it["code"])
	}
	ann := map[string]Item{}
	for start := 0; start < len(codes); start += 500 {
		b := codes[start:min(start+500, len(codes))]
		args := append([]any{pid, et}, b...)
		rs, err := s.DB.Query(ctx, `SELECT entity_code AS code, note, tags, updated_at FROM kb_annotations WHERE product_id = ? AND entity_type = ? AND entity_code IN (`+inList(len(b))+`)`, args...)
		if err != nil {
			return err
		}
		list, err := scanItems(rs)
		if err != nil {
			return err
		}
		for _, a := range list {
			c := a["code"].(string)
			delete(a, "code")
			ann[c] = a
		}
	}
	for _, it := range items {
		it["type"] = et
		code, _ := it["code"].(string)
		if a, ok := ann[code]; ok {
			it["annotation"] = a
		} else {
			it["annotation"] = nil
		}
		it["source_url"] = nil
		file, _ := it["source_file"].(string)
		if it["origin"] == "sync" && it["removed_at"] == nil && file != "" && repoURL.Valid && sha.Valid {
			u := "https://github.com/" + repopool.NormalizeRepo(repoURL.String) + "/blob/" + sha.String + "/" + file
			if l, ok := it["source_line"].(int64); ok && l > 0 {
				u += fmt.Sprintf("#L%d", l)
			}
			it["source_url"] = u
		}
		if it["automated"] != nil {
			it["automated"] = it["automated"] == int64(1)
		}
	}
	return nil
}

func (s *Syncer) list(ctx context.Context, pid int, et, where string, args []any, order string) ([]Item, error) {
	q := fmt.Sprintf("SELECT %s FROM %s t WHERE t.product_id = ?", selectCols(et), tables[et].name)
	if where != "" {
		q += " AND " + where
	}
	if order == "" {
		order = "t.code"
	}
	q += " ORDER BY " + order
	rs, err := s.DB.Query(ctx, q, append([]any{pid}, args...)...)
	if err != nil {
		return nil, err
	}
	items, err := scanItems(rs)
	if err != nil {
		return nil, err
	}
	return items, s.decorate(ctx, pid, et, items)
}

func activeOnly(include bool) string {
	if include {
		return ""
	}
	return "t.removed_at IS NULL"
}

func and(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " AND ")
}

// ---------------------------------------------------------------- summary

// Summary is the knowledge card's data.
func (s *Syncer) Summary(ctx context.Context, pid int) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	var f sql.NullString
	_ = s.DB.QueryRow(ctx, `SELECT knowledge_format FROM product_settings WHERE product_id = ?`, pid).Scan(&f)
	_, known := Lookup(f.String)
	out := Item{"format": f.String, "formats": Names(), "configured": f.String != "" && known, "enums": Enums(), "repo": nil}

	var rid int
	var rname, rurl, legacy string
	err := s.DB.QueryRow(ctx, `SELECT r.id, r.name, r.repo_url, p.branch FROM product_repos r JOIN products p ON p.id = r.product_id WHERE r.product_id = ? AND r.is_automation = 1`, pid).Scan(&rid, &rname, &rurl, &legacy)
	if err == nil {
		branch := legacy
		_ = s.DB.QueryRow(ctx, `SELECT branch FROM product_repo_branches WHERE repo_id = ? AND is_default = 1`, rid).Scan(&branch)
		if branch == "" {
			branch = "main"
		}
		out["repo"] = Item{"id": rid, "name": rname, "url": rurl, "branch": branch}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	latest, err := s.syncRows(ctx, pid, "", 1, false)
	if err != nil {
		return nil, err
	}
	out["latest_sync"] = first(latest)
	last, err := s.syncRows(ctx, pid, "status = 'succeeded'", 1, false)
	if err != nil {
		return nil, err
	}
	out["last_success"] = first(last)
	c, err := countActive(ctx, s.DB.SQL, pid)
	if err != nil {
		return nil, err
	}
	out["counts"] = c
	pct := 0.0
	if c.TestCases > 0 {
		pct = float64(c.Automated) * 100 / float64(c.TestCases)
	}
	out["coverage"] = Item{"test_cases": c.TestCases, "automated": c.Automated, "pct": pct}
	var ls, lt int
	_ = s.DB.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM kb_stories WHERE product_id = ? AND origin = 'local'), (SELECT COUNT(*) FROM kb_test_cases WHERE product_id = ? AND origin = 'local')`, pid, pid).Scan(&ls, &lt)
	out["local"] = Item{"stories": ls, "test_cases": lt}
	return out, nil
}

func first(list []Item) any {
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

// ---------------------------------------------------------------- syncs

const syncCols = "id, status, trigger_kind, triggered_by, repo_id, repo_name, repo_url, branch, format, parser_version, commit_sha, started_at, finished_at, counts, diff, problems, ai_status, ai_summary, ai_error, error"

func (s *Syncer) syncRows(ctx context.Context, pid int, where string, limit int, full bool) ([]Item, error) {
	q := "SELECT " + syncCols + " FROM kb_syncs WHERE product_id = ?"
	if where != "" {
		q += " AND " + where
	}
	q += fmt.Sprintf(" ORDER BY id DESC LIMIT %d", limit)
	rs, err := s.DB.Query(ctx, q, pid)
	if err != nil {
		return nil, err
	}
	items, err := scanItems(rs)
	if err != nil {
		return nil, err
	}
	if !full {
		for _, it := range items {
			if raw, ok := it["diff"].(json.RawMessage); ok {
				var d Diff
				if json.Unmarshal(raw, &d) == nil {
					it["diff"] = Item{"unchanged": d.Unchanged, "total": d.Total, "truncated": d.Truncated}
				}
			}
			if raw, ok := it["problems"].(json.RawMessage); ok {
				var p Problems
				if json.Unmarshal(raw, &p) == nil {
					it["problems"] = Item{"total": p.Total, "by_message": p.ByMessage}
				}
			}
		}
	}
	return items, nil
}

// Syncs lists the latest syncs (without the code lists and problem items).
func (s *Syncer) Syncs(ctx context.Context, pid, limit int) ([]Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.syncRows(ctx, pid, "", limit, false)
}

// Sync returns one full sync row.
func (s *Syncer) Sync(ctx context.Context, pid int, sid int64) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	list, err := s.syncRows(ctx, pid, fmt.Sprintf("id = %d", sid), 1, true)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return list[0], nil
}

// ---------------------------------------------------------------- tree

type featureStats struct{ stories, cases, automated int64 }

func (s *Syncer) featureStats(ctx context.Context, pid int, featureCode string) (map[string]featureStats, error) {
	q := `SELECT sf.feature_code, COUNT(DISTINCT st.code), COUNT(DISTINCT tc.code), COUNT(DISTINCT CASE WHEN tc.automated = 1 THEN tc.code END)
FROM kb_story_features sf
JOIN kb_stories st ON st.product_id = sf.product_id AND st.code = sf.story_code AND st.removed_at IS NULL
LEFT JOIN kb_test_cases tc ON tc.product_id = st.product_id AND tc.story_code = st.code AND tc.removed_at IS NULL
WHERE sf.product_id = ?`
	args := []any{pid}
	if featureCode != "" {
		q += " AND sf.feature_code = ?"
		args = append(args, featureCode)
	}
	rs, err := s.DB.Query(ctx, q+" GROUP BY sf.feature_code", args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[string]featureStats{}
	for rs.Next() {
		var c string
		var st featureStats
		if err := rs.Scan(&c, &st.stories, &st.cases, &st.automated); err != nil {
			return nil, err
		}
		out[c] = st
	}
	return out, rs.Err()
}

func addStats(it Item, st featureStats) {
	it["stories"], it["test_cases"], it["automated"] = st.stories, st.cases, st.automated
	pct := 0.0
	if st.cases > 0 {
		pct = float64(st.automated) * 100 / float64(st.cases)
	}
	it["coverage_pct"] = pct
}

// Tree returns parts → modules → submodules → features with coverage counts.
func (s *Syncer) Tree(ctx context.Context, pid int, includeRemoved bool) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	w := activeOnly(includeRemoved)
	parts, err := s.list(ctx, pid, TypePart, w, nil, "")
	if err != nil {
		return nil, err
	}
	mods, err := s.list(ctx, pid, TypeModule, w, nil, "")
	if err != nil {
		return nil, err
	}
	subs, err := s.list(ctx, pid, TypeSubmodule, w, nil, "t.name")
	if err != nil {
		return nil, err
	}
	feats, err := s.list(ctx, pid, TypeFeature, w, nil, "")
	if err != nil {
		return nil, err
	}
	stats, err := s.featureStats(ctx, pid, "")
	if err != nil {
		return nil, err
	}
	slim := func(it Item, keys ...string) Item {
		o := Item{}
		for _, k := range keys {
			o[k] = it[k]
		}
		o["has_annotation"] = it["annotation"] != nil
		return o
	}
	featBySub := map[string][]Item{}
	featNoSub := map[string][]Item{}
	for _, f := range feats {
		x := slim(f, "code", "name", "release_status", "status_as_of", "prod_since", "origin", "removed_at")
		addStats(x, stats[f["code"].(string)])
		if sc, ok := f["submodule_code"].(string); ok && sc != "" {
			featBySub[sc] = append(featBySub[sc], x)
		} else {
			mc, _ := f["module_code"].(string)
			featNoSub[mc] = append(featNoSub[mc], x)
		}
	}
	subByMod := map[string][]Item{}
	for _, sm := range subs {
		x := slim(sm, "code", "name", "origin", "removed_at")
		x["features"] = orEmpty(featBySub[sm["code"].(string)])
		mc, _ := sm["module_code"].(string)
		subByMod[mc] = append(subByMod[mc], x)
	}
	modByPart := map[string][]Item{}
	for _, m := range mods {
		x := slim(m, "code", "name", "origin", "removed_at")
		code := m["code"].(string)
		x["submodules"] = orEmpty(subByMod[code])
		x["features_without_submodule"] = orEmpty(featNoSub[code])
		pc, _ := m["part_code"].(string)
		modByPart[pc] = append(modByPart[pc], x)
	}
	outParts := []Item{}
	for _, p := range parts {
		x := slim(p, "code", "name", "origin", "removed_at")
		x["modules"] = orEmpty(modByPart[p["code"].(string)])
		outParts = append(outParts, x)
	}
	return Item{"parts": outParts}, nil
}

func orEmpty(l []Item) []Item {
	if l == nil {
		return []Item{}
	}
	return l
}

// ---------------------------------------------------------------- lists

// Filter holds list query parameters.
type Filter struct {
	Epic, Story, Feature, Status, Level, Priority, Origin, Tag, Q string
	// Part, Module, Submodule narrow stories and test cases through their linked features.
	Part, Module, Submodule string
	// Release filters features; Coverage (comma-separated states) filters features and stories.
	Release, Coverage string
	// Sort orders features: "tested" (default, items with test cases first) or "code".
	Sort           string
	Automated      string // "", "1", "0"
	IncludeRemoved bool
	Page, PageSize int
}

func likeArg(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

func (f *Filter) norm() {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Page > 100000 {
		f.Page = 100000
	}
	if f.PageSize < 1 {
		f.PageSize = 50
	}
	if f.PageSize > 200 {
		f.PageSize = 200
	}
}

func (s *Syncer) page(ctx context.Context, pid int, et, where string, args []any, f Filter) (Item, error) {
	var total int
	if err := s.DB.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s t WHERE t.product_id = ?", tables[et].name)+prefixAnd(where), append([]any{pid}, args...)...).Scan(&total); err != nil {
		return nil, err
	}
	items, err := s.list(ctx, pid, et, where, args, fmt.Sprintf("t.code LIMIT %d OFFSET %d", f.PageSize, (f.Page-1)*f.PageSize))
	if err != nil {
		return nil, err
	}
	return Item{"items": items, "total": total, "page": f.Page, "page_size": f.PageSize}, nil
}

func prefixAnd(w string) string {
	if w == "" {
		return ""
	}
	return " AND " + w
}

func commonFilter(et string, f Filter) (string, []any, error) {
	var parts []string
	var args []any
	parts = append(parts, activeOnly(f.IncludeRemoved))
	if f.Origin != "" {
		if f.Origin != "sync" && f.Origin != "local" {
			return "", nil, badInput("origin must be sync or local")
		}
		parts = append(parts, "t.origin = ?")
		args = append(args, f.Origin)
	}
	if f.Epic != "" {
		parts = append(parts, "t.epic_code = ?")
		args = append(args, f.Epic)
	}
	if f.Tag != "" {
		parts = append(parts, "EXISTS (SELECT 1 FROM kb_annotations a WHERE a.product_id = t.product_id AND a.entity_type = ? AND a.entity_code = t.code AND JSON_CONTAINS(a.tags, JSON_QUOTE(?)))")
		args = append(args, et, f.Tag)
	}
	return and(parts...), args, nil
}

// Stories lists stories.
func (s *Syncer) Stories(ctx context.Context, pid int, f Filter) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	f.norm()
	where, args, err := commonFilter(TypeStory, f)
	if err != nil {
		return nil, err
	}
	if f.Feature != "" {
		where = and(where, "t.code IN (SELECT story_code FROM kb_story_features WHERE product_id = t.product_id AND feature_code = ?)")
		args = append(args, f.Feature)
	}
	if f.Q != "" {
		where = and(where, "(t.code LIKE ? OR t.text LIKE ?)")
		args = append(args, likeArg(f.Q), likeArg(f.Q))
	}
	if w, a := featureScope("t.code", f); w != "" {
		where = and(where, w)
		args = append(args, a...)
	}
	items, err := s.list(ctx, pid, TypeStory, where, args, "")
	if err != nil {
		return nil, err
	}
	cov, err := s.CoverageBy(ctx, pid, "story", Scope{Epic: f.Epic})
	if err != nil {
		return nil, err
	}
	attachCoverage(items, cov)
	if items, err = filterCoverage(items, f.Coverage); err != nil {
		return nil, err
	}
	return pageItems(items, f), nil
}

// TestCases lists test cases.
func (s *Syncer) TestCases(ctx context.Context, pid int, f Filter) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	f.norm()
	where, args, err := commonFilter(TypeTestCase, f)
	if err != nil {
		return nil, err
	}
	enum := func(col, v string, allowed []string) error {
		if v == "" {
			return nil
		}
		if !contains(allowed, v) {
			return badInput("%s must be one of %s", col, strings.Join(allowed, ", "))
		}
		where = and(where, "t."+col+" = ?")
		args = append(args, v)
		return nil
	}
	if err := enum("status", f.Status, CaseStatuses); err != nil {
		return nil, err
	}
	if err := enum("level", f.Level, Levels); err != nil {
		return nil, err
	}
	if err := enum("priority", f.Priority, Priorities); err != nil {
		return nil, err
	}
	switch f.Automated {
	case "":
	case "1", "true":
		where = and(where, "t.automated = 1")
	case "0", "false":
		where = and(where, "t.automated = 0")
	default:
		return nil, badInput("automated must be 1 or 0")
	}
	if f.Story != "" {
		where = and(where, "t.story_code = ?")
		args = append(args, f.Story)
	}
	if f.Feature != "" {
		where = and(where, "t.story_code IN (SELECT story_code FROM kb_story_features WHERE product_id = t.product_id AND feature_code = ?)")
		args = append(args, f.Feature)
	}
	if f.Q != "" {
		where = and(where, "(t.code LIKE ? OR t.scenario LIKE ?)")
		args = append(args, likeArg(f.Q), likeArg(f.Q))
	}
	if w, a := featureScope("t.story_code", f); w != "" {
		where = and(where, w)
		args = append(args, a...)
	}
	return s.page(ctx, pid, TypeTestCase, where, args, f)
}

// Epics lists epics with case counts per status.
func (s *Syncer) Epics(ctx context.Context, pid int, includeRemoved bool) ([]Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	items, err := s.list(ctx, pid, TypeEpic, activeOnly(includeRemoved), nil, "")
	if err != nil {
		return nil, err
	}
	rs, err := s.DB.Query(ctx, `SELECT epic_code, status, COUNT(*) FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL GROUP BY epic_code, status`, pid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	counts := map[string]map[string]int{}
	for rs.Next() {
		var e, st string
		var n int
		if err := rs.Scan(&e, &st, &n); err != nil {
			return nil, err
		}
		if counts[e] == nil {
			counts[e] = map[string]int{}
		}
		counts[e][st] = n
	}
	for _, it := range items {
		c := counts[it["code"].(string)]
		if c == nil {
			c = map[string]int{}
		}
		total := 0
		for _, n := range c {
			total += n
		}
		it["case_counts"] = c
		it["test_cases"] = total
	}
	return items, rs.Err()
}

// ---------------------------------------------------------------- detail

func (s *Syncer) one(ctx context.Context, pid int, et, code string) (Item, error) {
	items, err := s.list(ctx, pid, et, "t.code = ?", []any{code}, "")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items[0], nil
}

func (s *Syncer) refs(ctx context.Context, pid int, et, where string, args ...any) ([]Item, error) {
	t := tables[et].name
	label := "name"
	switch et {
	case TypeEpic:
		label = "title"
	case TypeStory:
		label = "text"
	case TypeTestCase:
		label = "scenario"
	}
	rs, err := s.DB.Query(ctx, fmt.Sprintf("SELECT t.code, t.%s AS name, t.origin, t.removed_at FROM %s t WHERE t.product_id = ? AND %s ORDER BY t.code", label, t, where), append([]any{pid}, args...)...)
	if err != nil {
		return nil, err
	}
	return scanItems(rs)
}

// Entity returns one entity with its relations.
func (s *Syncer) Entity(ctx context.Context, pid int, et, code string) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	if _, ok := tables[et]; !ok || !CodeRe.MatchString(code) {
		return nil, ErrNotFound
	}
	it, err := s.one(ctx, pid, et, code)
	if err != nil {
		return nil, err
	}
	var e error
	set := func(key string, v []Item, err error) {
		if err != nil && e == nil {
			e = err
		}
		it[key] = orEmpty(v)
	}
	switch et {
	case TypePart:
		v, err := s.refs(ctx, pid, TypeModule, "t.part_code = ?", code)
		set("modules", v, err)
	case TypeModule:
		v, err := s.refs(ctx, pid, TypePart, "t.code = ?", it["part_code"])
		set("part", v, err)
		v, err = s.refs(ctx, pid, TypeSubmodule, "t.module_code = ?", code)
		set("submodules", v, err)
		v, err = s.list(ctx, pid, TypeFeature, "t.module_code = ? AND t.removed_at IS NULL", []any{code}, "")
		set("features", v, err)
	case TypeSubmodule:
		v, err := s.refs(ctx, pid, TypeModule, "t.code = ?", it["module_code"])
		set("module", v, err)
		v, err = s.list(ctx, pid, TypeFeature, "t.submodule_code = ? AND t.removed_at IS NULL", []any{code}, "")
		set("features", v, err)
	case TypeFeature:
		v, err := s.refs(ctx, pid, TypeModule, "t.code = ?", it["module_code"])
		set("module", v, err)
		v, err = s.refs(ctx, pid, TypeSubmodule, "t.code = ?", it["submodule_code"])
		set("submodule", v, err)
		stories, err := s.list(ctx, pid, TypeStory, "t.code IN (SELECT story_code FROM kb_story_features WHERE product_id = t.product_id AND feature_code = ?)", []any{code}, "")
		if err == nil && len(stories) > 0 {
			// One query for the cases of every story (was one per story).
			var tcs []Item
			tcs, err = s.list(ctx, pid, TypeTestCase, "t.removed_at IS NULL AND t.story_code IN (SELECT story_code FROM kb_story_features WHERE product_id = t.product_id AND feature_code = ?)", []any{code}, "")
			byStory := map[string][]Item{}
			for _, tc := range tcs {
				sc, _ := tc["story_code"].(string)
				byStory[sc] = append(byStory[sc], tc)
			}
			for _, st := range stories {
				st["test_cases"] = orEmpty(byStory[st["code"].(string)])
			}
		}
		set("stories", stories, err)
		stats, err := s.featureStats(ctx, pid, code)
		if err == nil {
			x := Item{}
			addStats(x, stats[code])
			it["stats"] = x
		} else if e == nil {
			e = err
		}
	case TypeEpic:
		v, err := s.list(ctx, pid, TypeStory, "t.epic_code = ? AND t.removed_at IS NULL", []any{code}, "")
		set("stories", v, err)
		v, err = s.refs(ctx, pid, TypeTestCase, "t.epic_code = ? AND t.removed_at IS NULL", code)
		set("test_cases", v, err)
	case TypeStory:
		v, err := s.refs(ctx, pid, TypeEpic, "t.code = ?", it["epic_code"])
		set("epic", v, err)
		v, err = s.refs(ctx, pid, TypeFeature, "t.code IN (SELECT feature_code FROM kb_story_features WHERE product_id = t.product_id AND story_code = ?)", code)
		set("features", v, err)
		v, err = s.list(ctx, pid, TypeTestCase, "t.story_code = ?", []any{code}, "")
		set("test_cases", v, err)
	case TypeTestCase:
		v, err := s.refs(ctx, pid, TypeEpic, "t.code = ?", it["epic_code"])
		set("epic", v, err)
		v, err = s.refs(ctx, pid, TypeStory, "t.code = ?", it["story_code"])
		set("story", v, err)
		v, err = s.refs(ctx, pid, TypeFeature, "t.code IN (SELECT feature_code FROM kb_story_features WHERE product_id = t.product_id AND story_code = ?)", it["story_code"])
		set("features", v, err)
	}
	if e != nil {
		return nil, e
	}
	if err := s.entityCoverage(ctx, pid, et, code, it); err != nil {
		return nil, err
	}
	if et == TypeStory {
		codes := []string{}
		for _, f := range it["features"].([]Item) {
			codes = append(codes, f["code"].(string))
		}
		it["feature_codes"] = codes
	}
	return it, nil
}

// ---------------------------------------------------------------- annotations

// AnnotationBody is the PUT body.
type AnnotationBody struct {
	Note *string  `json:"note"`
	Tags []string `json:"tags"`
}

// PutAnnotation upserts the note and tags of an entity (removed entities count as existing).
func (s *Syncer) PutAnnotation(ctx context.Context, pid int, et, code, userID string, b AnnotationBody) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	if _, ok := tables[et]; !ok {
		return nil, badInput("unknown entity type")
	}
	if !CodeRe.MatchString(code) {
		return nil, ErrNotFound
	}
	note := ""
	if b.Note != nil {
		note = *b.Note
	}
	if utf8.RuneCountInString(note) > 10000 {
		return nil, badInput("note must be at most 10000 characters")
	}
	if len(b.Tags) > 20 {
		return nil, badInput("at most 20 tags")
	}
	tags := []string{}
	seen := map[string]bool{}
	for _, t := range b.Tags {
		t = strings.TrimSpace(t)
		if n := utf8.RuneCountInString(t); n < 1 || n > 40 {
			return nil, badInput("each tag must be 1-40 characters")
		}
		if !seen[t] {
			seen[t] = true
			tags = append(tags, t)
		}
	}
	var x int
	err := s.DB.QueryRow(ctx, fmt.Sprintf("SELECT 1 FROM %s WHERE product_id = ? AND code = ?", tables[et].name), pid, code).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	tj, _ := json.Marshal(tags)
	if _, err := s.DB.Exec(ctx, `INSERT INTO kb_annotations (product_id, entity_type, entity_code, note, tags, updated_by) VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE note = VALUES(note), tags = VALUES(tags), updated_by = VALUES(updated_by), updated_at = CURRENT_TIMESTAMP`,
		pid, et, code, nullIfEmpty(note), string(tj), nullIfEmpty(userID)); err != nil {
		return nil, err
	}
	return Item{"note": nullIfEmpty(note), "tags": tags, "updated_at": time.Now().UTC()}, nil
}

// DeleteAnnotation removes the note and tags of an entity.
func (s *Syncer) DeleteAnnotation(ctx context.Context, pid int, et, code string) error {
	if err := s.productExists(ctx, pid); err != nil {
		return err
	}
	if _, ok := tables[et]; !ok {
		return badInput("unknown entity type")
	}
	_, err := s.DB.Exec(ctx, `DELETE FROM kb_annotations WHERE product_id = ? AND entity_type = ? AND entity_code = ?`, pid, et, code)
	return err
}

// ---------------------------------------------------------------- local drafts

// StoryBody is a local draft story.
type StoryBody struct {
	Code         string   `json:"code"`
	EpicCode     string   `json:"epic_code"`
	Text         string   `json:"text"`
	FeatureCodes []string `json:"feature_codes"`
}

// TestCaseBody is a local draft test case.
type TestCaseBody struct {
	Code           string   `json:"code"`
	EpicCode       string   `json:"epic_code"`
	StoryCode      string   `json:"story_code"`
	Scenario       string   `json:"scenario"`
	Polarity       string   `json:"polarity"`
	Level          string   `json:"level"`
	Priority       string   `json:"priority"`
	Status         string   `json:"status"`
	SpecPaths      []string `json:"spec_paths"`
	ReasonCategory string   `json:"reason_category"`
	ReasonDetail   string   `json:"reason_detail"`
}

func (s *Syncer) activeExists(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, pid int, et, code string) bool {
	var x int
	return q.QueryRowContext(ctx, fmt.Sprintf("SELECT 1 FROM %s WHERE product_id = ? AND code = ? AND removed_at IS NULL", tables[et].name), pid, code).Scan(&x) == nil
}

func (s *Syncer) originOf(ctx context.Context, tx *sql.Tx, pid int, et, code string) (string, error) {
	var o string
	err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT origin FROM %s WHERE product_id = ? AND code = ? FOR UPDATE", tables[et].name), pid, code).Scan(&o)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return o, err
}

func (s *Syncer) managedByRepo(ctx context.Context, pid int) error {
	name := "the automation repo"
	_ = s.DB.QueryRow(ctx, `SELECT name FROM product_repos WHERE product_id = ? AND is_automation = 1`, pid).Scan(&name)
	return &ConflictError{Msg: "managed by repo " + name + "; edit it there, then Sync"}
}

func isDup(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func (s *Syncer) validateStory(ctx context.Context, tx *sql.Tx, pid int, b *StoryBody) error {
	b.Text = strings.TrimSpace(b.Text)
	if b.Text == "" || len(b.Text) > maxText {
		return badInput("text is required (at most %d bytes)", maxText)
	}
	if !s.activeExists(ctx, tx, pid, TypeEpic, b.EpicCode) {
		return badInput("epic_code %q is not an active epic of this product", b.EpicCode)
	}
	seen := map[string]bool{}
	var fc []string
	for _, f := range b.FeatureCodes {
		if seen[f] {
			continue
		}
		seen[f] = true
		if !s.activeExists(ctx, tx, pid, TypeFeature, f) {
			return badInput("feature %q is not an active feature of this product", f)
		}
		fc = append(fc, f)
	}
	b.FeatureCodes = fc
	return nil
}

func (s *Syncer) writeStory(ctx context.Context, tx *sql.Tx, pid int, b StoryBody, userID string, insert bool) error {
	r := storyRow(Story{Code: b.Code, EpicCode: b.EpicCode, Text: b.Text, FeatureCodes: b.FeatureCodes})
	if insert {
		if _, err := tx.ExecContext(ctx, `INSERT INTO kb_stories (product_id, code, epic_code, text, unmapped_reason, origin, content_hash, created_by) VALUES (?, ?, ?, ?, NULL, 'local', ?, ?)`,
			pid, b.Code, b.EpicCode, b.Text, r.hash(), nullIfEmpty(userID)); err != nil {
			if isDup(err) {
				return &ConflictError{Msg: "a story with this code already exists"}
			}
			return err
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE kb_stories SET epic_code = ?, text = ?, content_hash = ? WHERE product_id = ? AND code = ? AND origin = 'local'`,
		b.EpicCode, b.Text, r.hash(), pid, b.Code); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb_story_features WHERE product_id = ? AND story_code = ? AND origin = 'local'`, pid, b.Code); err != nil {
		return err
	}
	for _, f := range b.FeatureCodes {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO kb_story_features (product_id, story_code, feature_code, origin) VALUES (?, ?, ?, 'local')`, pid, b.Code, f); err != nil {
			return err
		}
	}
	return nil
}

func validateCase(b *TestCaseBody) error {
	b.Scenario = strings.TrimSpace(b.Scenario)
	if b.Scenario == "" || len(b.Scenario) > maxText {
		return badInput("scenario is required (at most %d bytes)", maxText)
	}
	checks := []struct {
		name, v string
		list    []string
	}{{"polarity", b.Polarity, Polarities}, {"level", b.Level, Levels}, {"priority", b.Priority, Priorities}, {"status", b.Status, CaseStatuses}}
	for _, c := range checks {
		if !contains(c.list, c.v) {
			return badInput("%s must be one of %s", c.name, strings.Join(c.list, ", "))
		}
	}
	if b.SpecPaths == nil {
		b.SpecPaths = []string{}
	}
	if len(b.SpecPaths) > 20 {
		return badInput("at most 20 spec paths")
	}
	for _, p := range b.SpecPaths {
		if p == "" || len(p) > 300 {
			return badInput("spec paths must be 1-300 characters")
		}
	}
	if b.Status == "automated" && len(b.SpecPaths) == 0 {
		return badInput("an automated case needs at least one spec path")
	}
	if b.ReasonCategory != "" && !contains(ReasonCategories, b.ReasonCategory) {
		return badInput("reason_category must be one of %s", strings.Join(ReasonCategories, ", "))
	}
	if contains(NeedsReason, b.Status) && (b.ReasonCategory == "" || strings.TrimSpace(b.ReasonDetail) == "") {
		return badInput("status %s needs reason_category and reason_detail", b.Status)
	}
	if utf8.RuneCountInString(b.ReasonDetail) > 1000 {
		return badInput("reason_detail must be at most 1000 characters")
	}
	return nil
}

func (s *Syncer) writeCase(ctx context.Context, tx *sql.Tx, pid int, b TestCaseBody, userID string, insert bool) error {
	if !s.activeExists(ctx, tx, pid, TypeEpic, b.EpicCode) {
		return badInput("epic_code %q is not an active epic of this product", b.EpicCode)
	}
	if b.StoryCode != "" && !s.activeExists(ctx, tx, pid, TypeStory, b.StoryCode) {
		return badInput("story_code %q is not an active story of this product", b.StoryCode)
	}
	r := testCaseRow(TestCase{Code: b.Code, EpicCode: b.EpicCode, StoryCode: b.StoryCode, Scenario: b.Scenario, Polarity: b.Polarity, Level: b.Level,
		Priority: b.Priority, Status: b.Status, Automated: b.Status == "automated", SpecPaths: b.SpecPaths, ReasonCategory: b.ReasonCategory, ReasonDetail: strings.TrimSpace(b.ReasonDetail)})
	if insert {
		args := append([]any{pid, b.Code}, r.vals...)
		args = append(args, r.hash(), nullIfEmpty(userID))
		_, err := tx.ExecContext(ctx, `INSERT INTO kb_test_cases (product_id, code, epic_code, story_code, scenario, polarity, level, priority, status, automated, spec_paths, reason_category, reason_detail, origin, content_hash, created_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'local', ?, ?)`, args...)
		if isDup(err) {
			return &ConflictError{Msg: "a test case with this code already exists"}
		}
		return err
	}
	args := append([]any{}, r.vals...)
	args = append(args, r.hash(), pid, b.Code)
	_, err := tx.ExecContext(ctx, `UPDATE kb_test_cases SET epic_code = ?, story_code = ?, scenario = ?, polarity = ?, level = ?, priority = ?, status = ?, automated = ?, spec_paths = ?, reason_category = ?, reason_detail = ?, content_hash = ?
WHERE product_id = ? AND code = ? AND origin = 'local'`, args...)
	return err
}

// SaveDraft creates (code from body) or updates (code from path) a local story or test case.
func (s *Syncer) SaveDraft(ctx context.Context, pid int, et, pathCode, userID string, raw any) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	insert := pathCode == ""
	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		switch b := raw.(type) {
		case *StoryBody:
			if insert {
				if !LocalCodeRe.MatchString(b.Code) {
					return badInput(`code must match ^L-[A-Z0-9-]{1,30}$`)
				}
			} else {
				b.Code = pathCode
				if err := s.checkLocal(ctx, tx, pid, et, pathCode); err != nil {
					return err
				}
			}
			if err := s.validateStory(ctx, tx, pid, b); err != nil {
				return err
			}
			pathCode = b.Code
			return s.writeStory(ctx, tx, pid, *b, userID, insert)
		case *TestCaseBody:
			if insert {
				if !LocalCodeRe.MatchString(b.Code) {
					return badInput(`code must match ^L-[A-Z0-9-]{1,30}$`)
				}
			} else {
				b.Code = pathCode
				if err := s.checkLocal(ctx, tx, pid, et, pathCode); err != nil {
					return err
				}
			}
			if err := validateCase(b); err != nil {
				return err
			}
			pathCode = b.Code
			return s.writeCase(ctx, tx, pid, *b, userID, insert)
		}
		return badInput("unsupported entity")
	})
	if err != nil {
		var c *ConflictError
		if errors.As(err, &c) && c.Msg == "" {
			return nil, s.managedByRepo(ctx, pid)
		}
		return nil, err
	}
	return s.Entity(ctx, pid, et, pathCode)
}

// checkLocal returns ErrNotFound or an empty ConflictError (replaced by managedByRepo) for synced rows.
func (s *Syncer) checkLocal(ctx context.Context, tx *sql.Tx, pid int, et, code string) error {
	if !CodeRe.MatchString(code) {
		return ErrNotFound
	}
	o, err := s.originOf(ctx, tx, pid, et, code)
	if err != nil {
		return err
	}
	if o != "local" {
		return &ConflictError{}
	}
	return nil
}

// DeleteDraft hard-deletes a local story or test case, with its annotation and local links.
func (s *Syncer) DeleteDraft(ctx context.Context, pid int, et, code string) error {
	if err := s.productExists(ctx, pid); err != nil {
		return err
	}
	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.checkLocal(ctx, tx, pid, et, code); err != nil {
			return err
		}
		if et == TypeStory {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb_test_cases WHERE product_id = ? AND story_code = ?`, pid, code).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return &ConflictError{Msg: "local test cases still reference this story"}
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM kb_story_features WHERE product_id = ? AND story_code = ? AND origin = 'local'`, pid, code); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE product_id = ? AND code = ? AND origin = 'local'", tables[et].name), pid, code); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM kb_annotations WHERE product_id = ? AND entity_type = ? AND entity_code = ?`, pid, et, code)
		return err
	})
	var c *ConflictError
	if errors.As(err, &c) && c.Msg == "" {
		return s.managedByRepo(ctx, pid)
	}
	return err
}

// SetFormat validates a knowledge_format value ("" turns sync off).
func ValidFormat(name string) bool {
	if name == "" {
		return true
	}
	_, ok := Lookup(name)
	return ok
}

// entityCoverage adds the entity's own coverage and the coverage of its listed children.
func (s *Syncer) entityCoverage(ctx context.Context, pid int, et, code string, it Item) error {
	c, err := s.coverageOf(ctx, pid, et, code)
	if err != nil {
		return err
	}
	if c != nil {
		it["coverage"] = c
	}
	child := func(key, by string, sc Scope) error {
		list, ok := it[key].([]Item)
		if !ok || len(list) == 0 {
			return nil
		}
		m, err := s.CoverageBy(ctx, pid, by, sc)
		if err != nil {
			return err
		}
		attachCoverage(list, m)
		return nil
	}
	switch et {
	case TypePart:
		return child("modules", "module", Scope{Part: code})
	case TypeModule:
		if err := child("submodules", "submodule", Scope{Module: code}); err != nil {
			return err
		}
		return child("features", "feature", Scope{Module: code})
	case TypeSubmodule:
		return child("features", "feature", Scope{Submodule: code})
	case TypeFeature:
		return child("stories", "story", Scope{Feature: code})
	case TypeEpic:
		return child("stories", "story", Scope{Epic: code})
	case TypeTestCase:
		return child("features", "feature", Scope{})
	case TypeStory:
		return child("features", "feature", Scope{})
	}
	return nil
}

// ---------------------------------------------------------------- assistant files

// AssistantFilter narrows assistant lists.
type AssistantFilter struct {
	ProductID      int // 0 = every product (cross-product list only)
	Kind, Q        string
	IncludeRemoved bool
	Page, PageSize int
}

func (f *AssistantFilter) where(alias string) (string, []any, error) {
	var parts []string
	var args []any
	if !f.IncludeRemoved {
		parts = append(parts, alias+".removed_at IS NULL")
	}
	if f.ProductID > 0 {
		parts = append(parts, alias+".product_id = ?")
		args = append(args, f.ProductID)
	}
	if f.Kind != "" {
		var ks []string
		for _, k := range strings.Split(f.Kind, ",") {
			if !contains(AssistantKinds, k) {
				return "", nil, badInput("kind must be one of %s", strings.Join(AssistantKinds, ", "))
			}
			ks = append(ks, k)
			args = append(args, k)
		}
		parts = append(parts, alias+".kind IN ("+inList(len(ks))+")")
	}
	if f.Q != "" {
		parts = append(parts, "("+alias+".name LIKE ? OR "+alias+".description LIKE ? OR "+alias+".path LIKE ?)")
		args = append(args, likeArg(f.Q), likeArg(f.Q), likeArg(f.Q))
	}
	if len(parts) == 0 {
		return "1=1", args, nil
	}
	return strings.Join(parts, " AND "), args, nil
}

// Assistants lists the assistant files of every product (or one), without their bodies, with
// counts per kind for the same filter minus the kind.
func (s *Syncer) Assistants(ctx context.Context, f AssistantFilter) (Item, error) {
	if f.Page < 1 || f.Page > 100000 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 200 {
		f.PageSize = 50
	}
	where, args, err := f.where("a")
	if err != nil {
		return nil, err
	}
	var total int
	if err := s.DB.QueryRow(ctx, "SELECT COUNT(*) FROM kb_assistants a WHERE "+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	q := `SELECT a.code, a.kind, a.name, a.description, a.path, a.frontmatter, a.origin, a.removed_at, a.removed_sync_id, a.changed_sync_id,
  a.product_id, p.name AS product_name,
  (SELECT CONCAT(k.repo_url, ' ', k.commit_sha) FROM kb_syncs k WHERE k.product_id = a.product_id AND k.status = 'succeeded' ORDER BY k.id DESC LIMIT 1) AS repo_at
FROM kb_assistants a JOIN products p ON p.id = a.product_id WHERE ` + where + fmt.Sprintf(" ORDER BY FIELD(a.kind, 'instructions','agent','skill','rule','command'), a.name, a.path LIMIT %d OFFSET %d", f.PageSize, (f.Page-1)*f.PageSize)
	rs, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	items, err := scanItems(rs)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		it["type"] = TypeAssistant
		it["source_url"] = nil
		if ra, ok := it["repo_at"].(string); ok && it["removed_at"] == nil {
			if url, sha, ok := strings.Cut(ra, " "); ok {
				it["source_url"] = "https://github.com/" + repopool.NormalizeRepo(url) + "/blob/" + sha + "/" + it["path"].(string)
			}
		}
		delete(it, "repo_at")
	}
	kf := f
	kf.Kind = ""
	kw, kargs, _ := kf.where("a")
	krs, err := s.DB.Query(ctx, "SELECT a.kind, COUNT(*) FROM kb_assistants a WHERE "+kw+" GROUP BY a.kind", kargs...)
	if err != nil {
		return nil, err
	}
	defer krs.Close()
	kinds := map[string]int{}
	for _, k := range AssistantKinds {
		kinds[k] = 0
	}
	for krs.Next() {
		var k string
		var n int
		if err := krs.Scan(&k, &n); err != nil {
			return nil, err
		}
		kinds[k] = n
	}
	return Item{"items": items, "total": total, "page": f.Page, "page_size": f.PageSize, "kinds": kinds}, krs.Err()
}
