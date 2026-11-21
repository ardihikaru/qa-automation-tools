package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/caseresults"
	"github.com/qa-automation/ai-backend/internal/services/navmap"
)

// NAVIGATION_MAP.md: the product's route map with coverage (GET /api/products/{id}/navigation),
// a route's drill-down (…/navigation/routes/{key}), and the store of the routes real executor runs
// visited (nav_observations).

const (
	navNextLimit      = 25
	navObsDetailLimit = 50
	navRelatedLimit   = 100
)

var routeKeyRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// handleNavigation serves /api/products/{id}/navigation[/routes/{key}] (rest is after "navigation").
func (r *Router) handleNavigation(w http.ResponseWriter, req *http.Request, pid int, rest []string) {
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Database not available")
		return
	}
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx := req.Context()
	var one int
	if err := d.QueryRow(ctx, `SELECT 1 FROM products WHERE id = ?`, pid).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "product not found")
			return
		}
		writeOpError(w, "navigation", err)
		return
	}
	switch {
	case len(rest) == 0:
		q := req.URL.Query()
		f := navFilter{Surface: q.Get("surface"), App: q.Get("app"), Status: q.Get("status"), Q: strings.TrimSpace(q.Get("q"))}
		if f.Surface != "" && f.Surface != navmap.SurfaceWeb && f.Surface != navmap.SurfaceAPI {
			writeJSONError(w, http.StatusBadRequest, "surface must be web or api")
			return
		}
		if f.Status != "" && !contains(navmap.Statuses, f.Status) {
			writeJSONError(w, http.StatusBadRequest, "status must be one of "+strings.Join(navmap.Statuses, ", "))
			return
		}
		out, err := navigationMap(ctx, d, pid, f)
		if err != nil {
			writeOpError(w, "navigation", err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case len(rest) == 2 && rest[0] == "routes":
		if !routeKeyRe.MatchString(rest[1]) {
			writeJSONError(w, http.StatusNotFound, "route not found")
			return
		}
		out, err := navigationRoute(ctx, d, pid, rest[1])
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "route not found")
			return
		}
		if err != nil {
			writeOpError(w, "navigation", err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

type navFilter struct{ Surface, App, Status, Q string }

// navData is everything the map and the drill-down are computed from.
type navData struct {
	build   map[string]any
	routes  []navmap.Route
	byKey   map[string]navmap.Route
	links   map[string][]navmap.Link // route key -> links (all types and sources)
	direct  map[string][]navmap.CaseLink
	facts   map[string]navmap.CaseFact
	runAt   map[string]time.Time
	runRef  map[string]string
	obsHits map[string]int
	obsLast map[string]time.Time
}

func loadNavData(ctx context.Context, d *db.DB, pid int) (*navData, error) {
	nd := &navData{byKey: map[string]navmap.Route{}, links: map[string][]navmap.Link{}, direct: map[string][]navmap.CaseLink{},
		facts: map[string]navmap.CaseFact{}, runAt: map[string]time.Time{}, runRef: map[string]string{}, obsHits: map[string]int{}, obsLast: map[string]time.Time{}}

	var (
		syncID          sql.NullInt64
		sha, status     sql.NullString
		errText         string
		nRoutes, nLinks int
		problems        []byte
		builtAt         time.Time
	)
	err := d.QueryRow(ctx, `SELECT sync_id, commit_sha, status, error, routes, links, problems, built_at FROM nav_builds WHERE product_id = ?`, pid).
		Scan(&syncID, &sha, &status, &errText, &nRoutes, &nLinks, &problems, &builtAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, err
	default:
		var probs []string
		_ = json.Unmarshal(problems, &probs)
		if probs == nil {
			probs = []string{}
		}
		b := map[string]any{"status": status.String, "commit": sha.String, "built_at": builtAt.UTC().Format(time.RFC3339), "error": errText,
			"routes": nRoutes, "links": nLinks, "problems": probs, "sync_id": nil}
		if syncID.Valid {
			b["sync_id"] = syncID.Int64
		}
		nd.build = b
	}

	rows, err := d.Query(ctx, `SELECT route_key, surface, app, path, methods, sources, page_objects FROM nav_routes WHERE product_id = ?`, pid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var rt navmap.Route
		var m, s, p []byte
		if err := rows.Scan(&rt.Key, &rt.Surface, &rt.App, &rt.Path, &m, &s, &p); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal(m, &rt.Methods)
		_ = json.Unmarshal(s, &rt.Sources)
		_ = json.Unmarshal(p, &rt.PageObjects)
		rt.Methods, rt.Sources, rt.PageObjects = nonNil(rt.Methods), nonNil(rt.Sources), nonNil(rt.PageObjects)
		nd.routes = append(nd.routes, rt)
		nd.byKey[rt.Key] = rt
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(nd.routes, func(i, j int) bool {
		a, b := nd.routes[i], nd.routes[j]
		if a.Surface != b.Surface {
			return a.Surface < b.Surface
		}
		if a.App != b.App {
			return a.App < b.App
		}
		return a.Path < b.Path
	})

	rows, err = d.Query(ctx, `SELECT route_key, ref_type, ref, source, method FROM nav_links WHERE product_id = ?`, pid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l navmap.Link
		if err := rows.Scan(&l.RouteKey, &l.RefType, &l.Ref, &l.Source, &l.Method); err != nil {
			rows.Close()
			return nil, err
		}
		nd.links[l.RouteKey] = append(nd.links[l.RouteKey], l)
		if l.RefType == navmap.RefCase && navmap.Direct(l.Source) {
			nd.direct[l.RouteKey] = append(nd.direct[l.RouteKey], navmap.CaseLink{Case: l.Ref, Method: l.Method, Source: l.Source})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = d.Query(ctx, `
		SELECT t.code, t.status, t.priority, t.epic_code, t.automated, t.spec_paths, COALESCE(r.result, ''), r.run_at, COALESCE(r.run_ref, '')
		FROM kb_test_cases t
		LEFT JOIN kb_case_results r ON r.product_id = t.product_id AND r.case_code = t.code
		WHERE t.product_id = ? AND t.removed_at IS NULL`, pid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f navmap.CaseFact
		var spec []byte
		var runAt sql.NullTime
		var runRef string
		if err := rows.Scan(&f.Code, &f.Status, &f.Priority, &f.Epic, &f.Automated, &spec, &f.Result, &runAt, &runRef); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal(spec, &f.SpecPaths)
		nd.facts[f.Code] = f
		if runAt.Valid {
			nd.runAt[f.Code] = runAt.Time
			nd.runRef[f.Code] = runRef
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Observations: one direct link per (route, case, method), plus per-route hits and last time.
	rows, err = d.Query(ctx, `SELECT route_key, case_code, method, SUM(hits), MAX(observed_at) FROM nav_observations
		WHERE product_id = ? GROUP BY route_key, case_code, method`, pid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key, code, method string
		var hits int
		var last time.Time
		if err := rows.Scan(&key, &code, &method, &hits, &last); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := nd.byKey[key]; !ok {
			continue
		}
		nd.direct[key] = append(nd.direct[key], navmap.CaseLink{Case: code, Method: method, Source: navmap.SourceObserved})
		nd.obsHits[key] += hits
		if last.After(nd.obsLast[key]) {
			nd.obsLast[key] = last
		}
	}
	rows.Close()
	return nd, rows.Err()
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func timeOrNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// routeJSON is a route with its coverage.
func (nd *navData) routeJSON(rt navmap.Route, st navmap.RouteStats) map[string]any {
	features, specs := 0, 0
	for _, l := range nd.links[rt.Key] {
		switch l.RefType {
		case navmap.RefFeature:
			features++
		case navmap.RefSpec:
			specs++
		}
	}
	var lastResult time.Time
	for _, c := range st.CaseCodes {
		if t := nd.runAt[c]; t.After(lastResult) {
			lastResult = t
		}
	}
	return map[string]any{
		"key": rt.Key, "surface": rt.Surface, "app": rt.App, "path": rt.Path,
		"methods": rt.Methods, "sources": rt.Sources, "page_objects": rt.PageObjects,
		"status": st.Status, "cases": st.Cases, "automated": st.Automated, "planned": st.Planned,
		"pass": st.Pass, "fail": st.Fail, "features": features, "specs": specs,
		"observed_hits": nd.obsHits[rt.Key], "last_observed_at": timeOrNil(nd.obsLast[rt.Key]),
		"last_result_at": timeOrNil(lastResult),
	}
}

func navigationMap(ctx context.Context, d *db.DB, pid int, f navFilter) (map[string]any, error) {
	nd, err := loadNavData(ctx, d, pid)
	if err != nil {
		return nil, err
	}
	cov := navmap.Coverage(nd.routes, nd.direct, nd.facts)

	type appCount struct {
		Surface string `json:"surface"`
		App     string `json:"app"`
		Routes  int    `json:"routes"`
	}
	appIdx := map[string]int{}
	var apps []appCount
	for _, rt := range nd.routes {
		k := rt.Surface + "\x00" + rt.App
		if i, ok := appIdx[k]; ok {
			apps[i].Routes++
			continue
		}
		appIdx[k] = len(apps)
		apps = append(apps, appCount{Surface: rt.Surface, App: rt.App, Routes: 1})
	}

	q := strings.ToLower(f.Q)
	totals := map[string]int{"routes": 0, "observed": 0}
	for _, s := range navmap.Statuses {
		totals[s] = 0
	}
	routes := []map[string]any{}
	var inScope []navmap.Route
	for _, rt := range nd.routes {
		if (f.Surface != "" && rt.Surface != f.Surface) || (f.App != "" && rt.App != f.App) {
			continue
		}
		inScope = append(inScope, rt)
		st := cov[rt.Key]
		if f.Status != "" && st.Status != f.Status {
			continue
		}
		if q != "" && !routeMatchesQ(rt, st, q) {
			continue
		}
		totals["routes"]++
		totals[st.Status]++
		if nd.obsHits[rt.Key] > 0 {
			totals["observed"]++
		}
		routes = append(routes, nd.routeJSON(rt, st))
	}

	// Cases with no direct link to any route (GraphQL, MQ, data-driven chains, …).
	linked := map[string]bool{}
	for _, ls := range nd.direct {
		for _, l := range ls {
			linked[l.Case] = true
		}
	}
	unmapped := 0
	for c := range nd.facts {
		if !linked[c] {
			unmapped++
		}
	}
	totals["unmapped_cases"] = unmapped

	eligible, err := eligibleCaseSet(ctx, d, pid)
	if err != nil {
		return nil, err
	}
	next := navmap.Next(inScope, nd.direct, nd.facts, eligible, navNextLimit)
	if apps == nil {
		apps = []appCount{}
	}
	var build any
	if nd.build != nil {
		build = nd.build
	}
	return map[string]any{"product_id": pid, "build": build, "apps": apps, "totals": totals, "routes": routes, "next": next}, nil
}

func routeMatchesQ(rt navmap.Route, st navmap.RouteStats, q string) bool {
	if strings.Contains(strings.ToLower(rt.Path), q) {
		return true
	}
	for _, p := range rt.PageObjects {
		if strings.Contains(strings.ToLower(p), q) {
			return true
		}
	}
	for _, c := range st.CaseCodes {
		if strings.Contains(strings.ToLower(c), q) {
			return true
		}
	}
	return false
}

// eligibleCaseSet is the product-wide eligible set (EPIC_AUTOMATION.md decision 2).
func eligibleCaseSet(ctx context.Context, d *db.DB, pid int) (map[string]bool, error) {
	excluded, err := jobExclusions(ctx, d, pid)
	if err != nil {
		return nil, err
	}
	rows, err := d.Query(ctx, `SELECT t.code FROM kb_test_cases t WHERE t.product_id = ? AND `+eligibleCaseWhere, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		if !excluded[c] {
			out[c] = true
		}
	}
	return out, rows.Err()
}

type navLastJob struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	CompletedAt any    `json:"completed_at"`
	DurationMs  *int64 `json:"duration_ms"`
}

// lastCaseJobs returns the newest case_run/case_create job of each code.
func lastCaseJobs(ctx context.Context, d *db.DB, pid int, codes []string) (map[string]*navLastJob, error) {
	out := map[string]*navLastJob{}
	if len(codes) == 0 {
		return out, nil
	}
	want := map[string]bool{}
	for _, c := range codes {
		want[c] = true
	}
	rows, err := d.Query(ctx, `
		SELECT COALESCE(JSON_UNQUOTE(JSON_EXTRACT(payload, '$.case_code')), ''), id, type, status, completed_at, duration_ms
		FROM job_runs
		WHERE type IN ('case_run', 'case_create') AND CAST(JSON_EXTRACT(payload, '$.product_id') AS UNSIGNED) = ?
		ORDER BY created_at DESC, id DESC`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var j navLastJob
		var completed sql.NullTime
		var dur sql.NullInt64
		if err := rows.Scan(&code, &j.ID, &j.Type, &j.Status, &completed, &dur); err != nil {
			return nil, err
		}
		if !want[code] || out[code] != nil {
			continue
		}
		if completed.Valid {
			j.CompletedAt = completed.Time.UTC().Format(time.RFC3339)
		}
		if dur.Valid {
			v := dur.Int64
			j.DurationMs = &v
		}
		jj := j
		out[code] = &jj
	}
	return out, rows.Err()
}

// lastPlaywrightRuns returns each case's newest run-history row (RUN_HISTORY.md: pw_run_cases);
// a case without one is absent.
func lastPlaywrightRuns(ctx context.Context, d *db.DB, pid int, codes []string) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	if len(codes) == 0 {
		return out, nil
	}
	args := []any{pid}
	for _, c := range codes {
		args = append(args, c)
	}
	rows, err := d.Query(ctx, `SELECT c.case_code, c.run_id, c.result, c.duration_ms, r.started_at, r.source FROM pw_run_cases c
		JOIN pw_runs r ON r.id = c.run_id
		WHERE c.product_id = ? AND c.case_code IN (?`+strings.Repeat(",?", len(codes)-1)+`)
		ORDER BY c.run_id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code, result, source string
		var runID, dur int64
		var at time.Time
		if err := rows.Scan(&code, &runID, &result, &dur, &at, &source); err != nil {
			return nil, err
		}
		if out[code] != nil {
			continue
		}
		out[code] = map[string]any{"run_id": runID, "result": result, "duration_ms": dur, "started_at": at.UTC().Format(time.RFC3339), "source": source}
	}
	return out, rows.Err()
}

func navigationRoute(ctx context.Context, d *db.DB, pid int, key string) (map[string]any, error) {
	nd, err := loadNavData(ctx, d, pid)
	if err != nil {
		return nil, err
	}
	rt, ok := nd.byKey[key]
	if !ok {
		return nil, sql.ErrNoRows
	}
	cov := navmap.Coverage(nd.routes, nd.direct, nd.facts)
	st := cov[key]
	out := nd.routeJSON(rt, st)

	// Features.
	var fcodes, specs []string
	related := map[string]string{} // case -> feature
	for _, l := range nd.links[key] {
		switch {
		case l.RefType == navmap.RefFeature:
			fcodes = append(fcodes, l.Ref)
		case l.RefType == navmap.RefSpec:
			specs = append(specs, l.Ref)
		}
	}
	sort.Strings(fcodes)
	sort.Strings(specs)
	features := []map[string]any{}
	if len(fcodes) > 0 {
		args := []any{pid}
		for _, c := range fcodes {
			args = append(args, c)
		}
		rows, err := d.Query(ctx, `SELECT code, COALESCE(name, ''), release_status FROM kb_features WHERE product_id = ? AND removed_at IS NULL AND code IN (?`+strings.Repeat(",?", len(fcodes)-1)+`) ORDER BY code`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c, n, rs string
			if err := rows.Scan(&c, &n, &rs); err != nil {
				rows.Close()
				return nil, err
			}
			features = append(features, map[string]any{"code": c, "name": n, "release_status": rs})
		}
		rows.Close()
	}
	// Related cases through a feature: the cases of the features' stories.
	for _, l := range nd.links[key] {
		if l.RefType == navmap.RefCase && l.Source == navmap.SourceFeature {
			related[l.Ref] = ""
		}
	}

	// Cases with direct links, their link sources and last job.
	sources := map[string]map[string]bool{}
	addSrc := func(c, s string) {
		if sources[c] == nil {
			sources[c] = map[string]bool{}
		}
		sources[c][s] = true
	}
	for _, l := range nd.direct[key] {
		addSrc(l.Case, l.Source)
	}
	for _, c := range st.CaseCodes {
		if sources[c] == nil {
			addSrc(c, "wildcard")
		}
	}
	jobs, err := lastCaseJobs(ctx, d, pid, st.CaseCodes)
	if err != nil {
		return nil, err
	}
	scen := map[string]string{}
	if len(st.CaseCodes) > 0 {
		args := []any{pid}
		for _, c := range st.CaseCodes {
			args = append(args, c)
		}
		rows, err := d.Query(ctx, `SELECT code, COALESCE(scenario, '') FROM kb_test_cases WHERE product_id = ? AND code IN (?`+strings.Repeat(",?", len(st.CaseCodes)-1)+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c, s string
			if err := rows.Scan(&c, &s); err != nil {
				rows.Close()
				return nil, err
			}
			scen[c] = s
		}
		rows.Close()
	}
	lastRuns, err := lastPlaywrightRuns(ctx, d, pid, st.CaseCodes)
	if err != nil {
		return nil, err
	}
	cases := []map[string]any{}
	for _, c := range st.CaseCodes {
		f := nd.facts[c]
		var lj any
		if j := jobs[c]; j != nil {
			lj = j
		}
		var runAt any
		if t, ok := nd.runAt[c]; ok {
			runAt = t.UTC().Format(time.RFC3339)
		}
		cases = append(cases, map[string]any{
			"code": c, "scenario": scen[c], "status": f.Status, "automated": f.Automated, "priority": f.Priority, "epic": f.Epic,
			"link_sources": sortedSet(sources[c]), "result": f.Result, "run_at": runAt, "run_ref": nd.runRef[c], "last_job": lj,
			"last_run": lastRuns[c],
		})
	}
	rel := []map[string]any{}
	relCodes := make([]string, 0, len(related))
	for c := range related {
		if sources[c] == nil {
			if _, ok := nd.facts[c]; ok {
				relCodes = append(relCodes, c)
			}
		}
	}
	sort.Strings(relCodes)
	for i, c := range relCodes {
		if i >= navRelatedLimit {
			break
		}
		rel = append(rel, map[string]any{"code": c, "status": nd.facts[c].Status, "automated": nd.facts[c].Automated})
	}

	obs := []map[string]any{}
	rows, err := d.Query(ctx, `SELECT job_id, case_code, method, hits, observed_at FROM nav_observations
		WHERE product_id = ? AND route_key = ? ORDER BY observed_at DESC, job_id DESC LIMIT ?`, pid, key, navObsDetailLimit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var job, code, method string
		var hits int
		var at time.Time
		if err := rows.Scan(&job, &code, &method, &hits, &at); err != nil {
			rows.Close()
			return nil, err
		}
		obs = append(obs, map[string]any{"job_id": job, "case_code": code, "method": method, "hits": hits, "observed_at": at.UTC().Format(time.RFC3339)})
	}
	rows.Close()

	out["features"] = features
	out["cases"] = cases
	out["related_cases"] = rel
	out["related_total"] = len(relCodes)
	out["specs"] = nonNil(specs)
	out["observations"] = obs
	return out, nil
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------- observations from executor runs (NAVIGATION_MAP.md 3.5) ----------

type navEntry struct {
	Title   string `json:"title"`
	File    string `json:"file"`
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Method  string `json:"method"`
	Host    string `json:"host"`
	Path    string `json:"path"`
	Hits    int    `json:"hits"`
}

type navReport struct {
	Version   int        `json:"version"`
	Truncated bool       `json:"truncated"`
	Entries   []navEntry `json:"entries"`
}

// NavCount is what storing a run's observations did.
type NavCount struct {
	Stored    int `json:"stored"`
	Unmatched int `json:"unmatched"`
}

type navObsRow struct {
	key, code, method string
	hits              int
}

// matchNavEntries turns reporter entries into observation rows against the product's routes. Pure.
func matchNavEntries(entries []navEntry, routes []navmap.Route) ([]navObsRow, int) {
	agg := map[[3]string]int{}
	unmatched := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Project, "setup-") || strings.HasPrefix(e.Project, "vendor-e2e-") {
			unmatched++
			continue
		}
		codes := caseresults.CaseCodes(e.Title)
		surface := map[string]string{"web": navmap.SurfaceWeb, "api": navmap.SurfaceAPI}[e.Kind]
		p := navmap.NormPath(e.Path)
		if len(codes) == 0 || surface == "" || p == "" {
			unmatched++
			continue
		}
		app := ""
		file := strings.TrimPrefix(e.File, "./")
		if surface == navmap.SurfaceWeb {
			app = folderOf(file, "tests/web/")
		} else {
			app = folderOf(file, "tests/api/rest/")
		}
		best, bestLit := "", -1
		for _, rt := range routes {
			if rt.Surface != surface || (app != "" && rt.App != app) {
				continue
			}
			ok, lit := navmap.Match(rt.Path, p)
			if !ok {
				continue
			}
			if lit > bestLit || (lit == bestLit && rt.Key < best) {
				best, bestLit = rt.Key, lit
			}
		}
		if best == "" {
			unmatched++
			continue
		}
		method := strings.ToUpper(e.Method)
		if surface == navmap.SurfaceWeb || len(method) > 8 {
			method = ""
		}
		hits := e.Hits
		if hits <= 0 {
			hits = 1
		}
		for _, c := range codes {
			agg[[3]string{best, c, method}] += hits
		}
	}
	out := make([]navObsRow, 0, len(agg))
	for k, h := range agg {
		out = append(out, navObsRow{key: k[0], code: k[1], method: k[2], hits: h})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.key != b.key {
			return a.key < b.key
		}
		if a.code != b.code {
			return a.code < b.code
		}
		return a.method < b.method
	})
	return out, unmatched
}

func folderOf(p, prefix string) string {
	if !strings.HasPrefix(p, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(p, prefix)
	if i := strings.IndexByte(rest, '/'); i > 0 {
		return rest[:i]
	}
	return ""
}

// storeNavObservations stores the routes an executor run visited (result["nav"]). Only matched
// route keys are stored; a nil or malformed nav stores nothing. Errors are logged, never returned.
func storeNavObservations(ctx context.Context, d *db.DB, pid int, jobID, runID string, nav any) NavCount {
	var cnt NavCount
	if d == nil || pid <= 0 || nav == nil {
		return cnt
	}
	raw, err := json.Marshal(nav)
	if err != nil {
		return cnt
	}
	var rep navReport
	if err := json.Unmarshal(raw, &rep); err != nil || rep.Version != 1 {
		return cnt
	}
	var routes []navmap.Route
	rows, err := d.Query(ctx, `SELECT route_key, surface, app, path FROM nav_routes WHERE product_id = ?`, pid)
	if err != nil {
		log.Printf("job %s: navigation observations: %v", jobID, err)
		return cnt
	}
	for rows.Next() {
		var rt navmap.Route
		if rows.Scan(&rt.Key, &rt.Surface, &rt.App, &rt.Path) == nil {
			routes = append(routes, rt)
		}
	}
	rows.Close()
	obs, unmatched := matchNavEntries(rep.Entries, routes)
	cnt.Unmatched = unmatched
	for _, o := range obs {
		if _, err := d.Exec(ctx, `INSERT INTO nav_observations (product_id, route_key, job_id, run_id, case_code, method, hits, observed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE hits = VALUES(hits), observed_at = VALUES(observed_at)`, pid, o.key, jobID, runID, o.code, o.method, o.hits); err != nil {
			log.Printf("job %s: navigation observation: %v", jobID, err)
			continue
		}
		cnt.Stored++
	}
	if _, err := d.Exec(ctx, `DELETE FROM nav_observations WHERE product_id = ? AND observed_at < UTC_TIMESTAMP() - INTERVAL 180 DAY`, pid); err != nil {
		log.Printf("job %s: prune navigation observations: %v", jobID, err)
	}
	return cnt
}
