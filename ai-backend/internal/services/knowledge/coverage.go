package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Coverage counts the distinct active test cases under an entity, by status.
type Coverage struct {
	TestCases  int     `json:"test_cases"`
	Automated  int     `json:"automated"`
	Planned    int     `json:"planned"`
	Blocked    int     `json:"blocked"`
	ManualOnly int     `json:"manual_only"`
	Wont       int     `json:"wont"`
	Pct        float64 `json:"pct"`
	// State is untested (no cases), none (no case automated), partial or full.
	State string `json:"state"`
}

func (c *Coverage) add(status string, n int) {
	c.TestCases += n
	switch status {
	case "automated":
		c.Automated += n
	case "planned":
		c.Planned += n
	case "blocked":
		c.Blocked += n
	case "manual-only":
		c.ManualOnly += n
	case "wont":
		c.Wont += n
	}
}

func (c *Coverage) finish() {
	switch {
	case c.TestCases == 0:
		c.State = "untested"
	case c.Automated == 0:
		c.State = "none"
	case c.Automated < c.TestCases:
		c.State = "partial"
	default:
		c.State = "full"
	}
	if c.TestCases > 0 {
		c.Pct = float64(c.Automated) * 100 / float64(c.TestCases)
	}
}

// CoverageStates are the values of Coverage.State (and of the coverage list filter).
var CoverageStates = []string{"untested", "none", "partial", "full"}

// CoverageBys are the groupings accepted by CoverageBy.
var CoverageBys = []string{"part", "module", "submodule", "feature", "epic", "story"}

// Scope narrows a coverage query (empty fields are ignored).
type Scope struct{ Part, Module, Submodule, Epic string }

// CoverageBy returns the coverage of every active entity of one type, keyed by code. Entities
// without test cases are present with State "untested". Feature-based groupings count each test
// case once per group even when it reaches the group through several features.
func (s *Syncer) CoverageBy(ctx context.Context, pid int, by string, sc Scope) (map[string]*Coverage, error) {
	out := map[string]*Coverage{}
	var keysQ, countQ string
	var args []any
	switch by {
	case "part", "module", "submodule", "feature":
		col := map[string]string{"part": "LEFT(f.module_code, 1)", "module": "f.module_code", "submodule": "f.submodule_code", "feature": "f.code"}[by]
		where := "f.product_id = ? AND f.removed_at IS NULL"
		args = []any{pid}
		if sc.Part != "" {
			where += " AND LEFT(f.module_code, 1) = ?"
			args = append(args, sc.Part)
		}
		if sc.Module != "" {
			where += " AND f.module_code = ?"
			args = append(args, sc.Module)
		}
		if sc.Submodule != "" {
			where += " AND f.submodule_code = ?"
			args = append(args, sc.Submodule)
		}
		if by == "submodule" {
			where += " AND f.submodule_code IS NOT NULL"
		}
		keysQ = fmt.Sprintf("SELECT DISTINCT %s FROM kb_features f WHERE %s", col, where)
		countQ = fmt.Sprintf(`SELECT %s, tc.status, COUNT(DISTINCT tc.code) FROM kb_features f
JOIN kb_story_features sf ON sf.product_id = f.product_id AND sf.feature_code = f.code
JOIN kb_stories st ON st.product_id = sf.product_id AND st.code = sf.story_code AND st.removed_at IS NULL
JOIN kb_test_cases tc ON tc.product_id = st.product_id AND tc.story_code = st.code AND tc.removed_at IS NULL
WHERE %s GROUP BY 1, 2`, col, where)
	case "epic", "story":
		col := map[string]string{"epic": "epic_code", "story": "story_code"}[by]
		table := map[string]string{"epic": "kb_epics", "story": "kb_stories"}[by]
		kw, cw := "product_id = ? AND removed_at IS NULL", "product_id = ? AND removed_at IS NULL AND "+col+" IS NOT NULL"
		args = []any{pid}
		if sc.Epic != "" {
			if by == "epic" {
				kw += " AND code = ?"
			} else {
				kw += " AND epic_code = ?"
			}
			cw += " AND epic_code = ?"
			args = append(args, sc.Epic)
		}
		keysQ = fmt.Sprintf("SELECT code FROM %s WHERE %s", table, kw)
		countQ = fmt.Sprintf("SELECT %s, status, COUNT(*) FROM kb_test_cases WHERE %s GROUP BY 1, 2", col, cw)
	default:
		return nil, badInput("by must be one of %s", strings.Join(CoverageBys, ", "))
	}
	rs, err := s.DB.Query(ctx, keysQ, args...)
	if err != nil {
		return nil, err
	}
	for rs.Next() {
		var k *string
		if err := rs.Scan(&k); err != nil {
			rs.Close()
			return nil, err
		}
		if k != nil {
			out[*k] = &Coverage{}
		}
	}
	rs.Close()
	rs, err = s.DB.Query(ctx, countQ, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	for rs.Next() {
		var k *string
		var status string
		var n int
		if err := rs.Scan(&k, &status, &n); err != nil {
			return nil, err
		}
		if k == nil {
			continue
		}
		c := out[*k]
		if c == nil {
			c = &Coverage{}
			out[*k] = c
		}
		c.add(status, n)
	}
	for _, c := range out {
		c.finish()
	}
	return out, rs.Err()
}

// one entity's coverage ("" when the type has no coverage).
func (s *Syncer) coverageOf(ctx context.Context, pid int, et, code string) (*Coverage, error) {
	var by string
	var sc Scope
	switch et {
	case TypePart:
		by, sc = "part", Scope{Part: code}
	case TypeModule:
		by, sc = "module", Scope{Module: code}
	case TypeSubmodule:
		by, sc = "submodule", Scope{Submodule: code}
	case TypeFeature:
		by = "feature"
	case TypeEpic:
		by, sc = "epic", Scope{Epic: code}
	case TypeStory:
		by = "story"
	default:
		return nil, nil
	}
	m, err := s.CoverageBy(ctx, pid, by, sc)
	if err != nil {
		return nil, err
	}
	if c, ok := m[code]; ok {
		return c, nil
	}
	c := &Coverage{}
	c.finish()
	return c, nil
}

// attachCoverage sets item["coverage"] from m (untested when missing).
func attachCoverage(items []Item, m map[string]*Coverage) {
	for _, it := range items {
		code, _ := it["code"].(string)
		if c, ok := m[code]; ok {
			it["coverage"] = c
		} else {
			c := &Coverage{}
			c.finish()
			it["coverage"] = c
		}
	}
}

// filterCoverage keeps items whose coverage state is in states (comma-separated).
func filterCoverage(items []Item, states string) ([]Item, error) {
	if states == "" {
		return items, nil
	}
	want := map[string]bool{}
	for _, st := range strings.Split(states, ",") {
		if !contains(CoverageStates, st) {
			return nil, badInput("coverage must be one of %s", strings.Join(CoverageStates, ", "))
		}
		want[st] = true
	}
	out := []Item{}
	for _, it := range items {
		if c, ok := it["coverage"].(*Coverage); ok && want[c.State] {
			out = append(out, it)
		}
	}
	return out, nil
}

func pageItems(items []Item, f Filter) Item {
	total := len(items)
	from := min((f.Page-1)*f.PageSize, total)
	to := min(from+f.PageSize, total)
	return Item{"items": items[from:to], "total": total, "page": f.Page, "page_size": f.PageSize}
}

// featureScope is the SQL that limits story codes (column expr) to stories linked to features
// matching the part/module/submodule filters.
func featureScope(expr string, f Filter) (string, []any) {
	var conds []string
	var args []any
	if f.Part != "" {
		conds = append(conds, "LEFT(fx.module_code, 1) = ?")
		args = append(args, f.Part)
	}
	if f.Module != "" {
		conds = append(conds, "fx.module_code = ?")
		args = append(args, f.Module)
	}
	if f.Submodule != "" {
		conds = append(conds, "fx.submodule_code = ?")
		args = append(args, f.Submodule)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return expr + ` IN (SELECT sfx.story_code FROM kb_story_features sfx JOIN kb_features fx ON fx.product_id = sfx.product_id AND fx.code = sfx.feature_code AND fx.removed_at IS NULL
WHERE sfx.product_id = t.product_id AND ` + strings.Join(conds, " AND ") + `)`, args
}

// Features lists features with their coverage.
func (s *Syncer) Features(ctx context.Context, pid int, f Filter) (Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	f.norm()
	where, args, err := commonFilter(TypeFeature, Filter{Origin: f.Origin, Tag: f.Tag, IncludeRemoved: f.IncludeRemoved})
	if err != nil {
		return nil, err
	}
	if f.Part != "" {
		where = and(where, "LEFT(t.module_code, 1) = ?")
		args = append(args, f.Part)
	}
	if f.Module != "" {
		where = and(where, "t.module_code = ?")
		args = append(args, f.Module)
	}
	if f.Submodule != "" {
		where = and(where, "t.submodule_code = ?")
		args = append(args, f.Submodule)
	}
	if f.Release != "" {
		if !contains(ReleaseStatuses, f.Release) {
			return nil, badInput("release must be one of %s", strings.Join(ReleaseStatuses, ", "))
		}
		where = and(where, "t.release_status = ?")
		args = append(args, f.Release)
	}
	if f.Q != "" {
		where = and(where, "(t.code LIKE ? OR t.name LIKE ?)")
		args = append(args, likeArg(f.Q), likeArg(f.Q))
	}
	items, err := s.list(ctx, pid, TypeFeature, where, args, "")
	if err != nil {
		return nil, err
	}
	cov, err := s.CoverageBy(ctx, pid, "feature", Scope{Part: f.Part, Module: f.Module, Submodule: f.Submodule})
	if err != nil {
		return nil, err
	}
	attachCoverage(items, cov)
	if items, err = filterCoverage(items, f.Coverage); err != nil {
		return nil, err
	}
	return pageItems(items, f), nil
}

// Modules lists modules (with part and sub-module counts) and their coverage.
func (s *Syncer) Modules(ctx context.Context, pid int, includeRemoved bool) ([]Item, error) {
	if err := s.productExists(ctx, pid); err != nil {
		return nil, err
	}
	items, err := s.list(ctx, pid, TypeModule, activeOnly(includeRemoved), nil, "")
	if err != nil {
		return nil, err
	}
	cov, err := s.CoverageBy(ctx, pid, "module", Scope{})
	if err != nil {
		return nil, err
	}
	attachCoverage(items, cov)
	counts := map[string][2]int{}
	rs, err := s.DB.Query(ctx, `SELECT module_code, COUNT(*), COUNT(DISTINCT submodule_code) FROM kb_features WHERE product_id = ? AND removed_at IS NULL GROUP BY module_code`, pid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	for rs.Next() {
		var m string
		var nf, ns int
		if err := rs.Scan(&m, &nf, &ns); err != nil {
			return nil, err
		}
		counts[m] = [2]int{nf, ns}
	}
	for _, it := range items {
		c := counts[it["code"].(string)]
		it["features"], it["submodules"] = c[0], c[1]
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i]["code"].(string) < items[j]["code"].(string) })
	return items, rs.Err()
}
