package gaossync

// QA_DEFECT_TRIAGE.md: the defect push. The state lives in qa_defect_state (written by the Playwright
// ingest); this file sends what changed to POST /products/{id}/qa/defects, inside the same push pass
// (and so inside the same outbox marker, sweeper and slot) as the cases and the results.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defectBatchMax   = 50
	defectBodyMax    = 240 << 10 // GAOS reads 256 KiB; the margin covers the envelope
	maxTargetRepos   = 8
	maxOperations    = 5
	maxSpecFiles     = 24
	maxSpecFileBytes = 256 << 10
	unsupportedWait  = 30 * time.Minute // an older GAOS without the route (404/405)
	capWait          = time.Hour        // GAOS's per-day cap on new defect keys
	maxExcerpt       = 4096
)

var (
	repoSlugRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	operationRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-/]{0,80}$`)
	gqlOpRe     = regexp.MustCompile(`\b(?:query|mutation|subscription)\s+([A-Za-z_][A-Za-z0-9_]*)\s*[({]`)
	restCallRe  = regexp.MustCompile("\\.(?:get|post|put|patch|delete|head)\\s*(?:<[^>]*>)?\\(\\s*(['\"`])([^'\"`]+)['\"`]")
	importRe    = regexp.MustCompile(`(?:from\s+|require\(\s*|import\(\s*)['"]([^'"]+)['"]`)
	lineCmtRe   = regexp.MustCompile(`(?m)^\s*//.*$`)
	tmplRe      = regexp.MustCompile(`\$\{[^}]*\}`)
	badOpCharRe = regexp.MustCompile(`[^A-Za-z0-9_.\-/]`)
	repoURLRe   = regexp.MustCompile(`[:/]([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)
)

// SpecReader reads one file of the product's automation repository (a repo-relative, slash path).
// Set by the server; nil means no operation names are extracted.
type SpecReader func(ctx context.Context, productID int, rel string) ([]byte, error)

// SetSpecReader sets the reader used to find the operations a spec calls.
func (s *Syncer) SetSpecReader(fn SpecReader) { s.specReader = fn }

type defectItem struct {
	CaseID         string   `json:"caseId"`
	DefectID       string   `json:"defectId,omitempty"`
	Kind           string   `json:"kind"`
	Level          string   `json:"level"`
	AutomationType string   `json:"automationType"`
	Backend        string   `json:"backend"`
	TargetRepos    []string `json:"targetRepos"`
	Operations     []string `json:"operations"`
	Spec           string   `json:"spec"`
	Title          string   `json:"title"`
	Excerpt        string   `json:"excerpt"`
	RunRef         string   `json:"runRef,omitempty"`
	RunAt          int64    `json:"runAt"`
}

type defectsBody struct {
	Source string       `json:"source"`
	Items  []defectItem `json:"items"`
}

type defectsResponse struct {
	Applied   *int         `json:"applied"`
	Unchanged *int         `json:"unchanged"`
	Rejected  *[]rejection `json:"rejected"`
}

func (r *defectsResponse) check(batch int) error {
	if r.Applied == nil || r.Unchanged == nil {
		return errors.New("missing counters")
	}
	if *r.Applied < 0 || *r.Unchanged < 0 {
		return errors.New("negative counter")
	}
	return checkRejected(r.Rejected, batch)
}

// ---- derivation ----

// AutomationType is "be" for a GQL or API case and "e2e" for a WEB or E2E case.
func AutomationType(level string) string {
	switch level {
	case "API", "GQL":
		return "be"
	case "WEB", "E2E":
		return "e2e"
	}
	return ""
}

var specRoots = []struct{ prefix, backend string }{
	{"tests/api/graphql/", "graphql"},
	{"tests/api/rest/", "rest"},
	{"tests/web/", ""},
}

// BackendKind is "graphql" or "rest" from the spec path, else "".
func BackendKind(spec string) string {
	for _, r := range specRoots {
		if strings.HasPrefix(spec, r.prefix) {
			return r.backend
		}
	}
	return ""
}

// ServiceDir is the service or app directory right under the spec's tests root, else "".
func ServiceDir(spec string) string {
	for _, r := range specRoots {
		if rest, ok := strings.CutPrefix(spec, r.prefix); ok {
			if i := strings.Index(rest, "/"); i > 0 {
				return rest[:i]
			}
		}
	}
	return ""
}

// repoSlug is owner/repo from a repository URL (https or ssh form); "" when it has none.
func repoSlug(url string) string {
	m := repoURLRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return ""
	}
	s := m[1] + "/" + m[2]
	if !repoSlugRe.MatchString(s) {
		return ""
	}
	return s
}

// targetsFor resolves the repositories a case exercises: the repo named like the spec's service
// directory; for an e2e case also the repos the product's mapping gives that directory.
func targetsFor(spec, autoType string, repos map[string]string, mapping map[string][]string) []string {
	dir := ServiceDir(spec)
	out := []string{}
	if dir == "" {
		return out
	}
	add := func(name string) {
		slug := repos[strings.ToLower(name)]
		if slug == "" || len(out) >= maxTargetRepos {
			return
		}
		for _, o := range out {
			if o == slug {
				return
			}
		}
		out = append(out, slug)
	}
	add(dir)
	if autoType == "e2e" {
		for _, n := range mapping[dir] {
			add(n)
		}
	}
	return out
}

func (s *Syncer) loadTargets(ctx context.Context, productID int) (repos map[string]string, mapping map[string][]string, err error) {
	repos, mapping = map[string]string{}, map[string][]string{}
	rows, err := s.db.Query(ctx, `SELECT name, repo_url FROM product_repos WHERE product_id = ? ORDER BY id`, productID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var n, u string
		if err := rows.Scan(&n, &u); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if slug := repoSlug(u); slug != "" {
			repos[strings.ToLower(n)] = slug
		}
	}
	rows.Close()
	m, err := s.db.Query(ctx, `SELECT service_dir, repo_name FROM product_service_repos WHERE product_id = ? ORDER BY service_dir, repo_name`, productID)
	if err != nil {
		return nil, nil, err
	}
	defer m.Close()
	for m.Next() {
		var d, n string
		if err := m.Scan(&d, &n); err != nil {
			return nil, nil, err
		}
		mapping[d] = append(mapping[d], n)
	}
	return repos, mapping, m.Err()
}

// operationsOf lists up to five GraphQL operation names and REST route names the code calls,
// in order of appearance. Names that GAOS would refuse are dropped.
func operationsOf(sources ...string) []string {
	all := allOperations(sources...)
	if len(all) > maxOperations {
		all = all[:maxOperations]
	}
	return all
}

// allOperations is every distinct valid name in the sources, in order of appearance (at most 400).
func allOperations(sources ...string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(n string) {
		if len(out) >= 400 || seen[n] || !operationRe.MatchString(n) {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	for _, src := range sources {
		type hit struct {
			pos  int
			name string
		}
		var hits []hit
		for _, m := range gqlOpRe.FindAllStringSubmatchIndex(src, -1) {
			hits = append(hits, hit{m[0], src[m[2]:m[3]]})
		}
		for _, m := range restCallRe.FindAllStringSubmatchIndex(src, -1) {
			if n := routeName(src[m[4]:m[5]]); n != "" {
				hits = append(hits, hit{m[0], n})
			}
		}
		for i := 1; i < len(hits); i++ { // few hits: insertion sort by position
			for j := i; j > 0 && hits[j].pos < hits[j-1].pos; j-- {
				hits[j], hits[j-1] = hits[j-1], hits[j]
			}
		}
		for _, h := range hits {
			add(h.name)
		}
	}
	return out
}

// routeName turns a request URL or path into a route name: no scheme, host, query or leading slash,
// template holes replaced by _id. A string that is not a path ("x-token" of headers.get) gives "".
func routeName(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			u = rest[j:]
		} else {
			return ""
		}
	}
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	u = tmplRe.ReplaceAllString(u, "_id")
	if !strings.Contains(u, "/") {
		return ""
	}
	u = strings.Trim(u, "/")
	u = badOpCharRe.ReplaceAllString(u, "_")
	if len(u) > 81 {
		u = u[:81]
	}
	if u == "" || !operationRe.MatchString(u) {
		return ""
	}
	return u
}

// operationsFor reads the spec and the files it imports from the same repo (at most maxSpecFiles).
func (s *Syncer) operationsFor(ctx context.Context, productID int, spec string) []string {
	if s.specReader == nil {
		return []string{}
	}
	aliases := s.tsAliases(ctx, productID)
	type srcFile struct{ path, text string }
	var files []srcFile
	seen := map[string]bool{}
	queue := []string{spec}
	for len(queue) > 0 && len(seen) < maxSpecFiles {
		f := queue[0]
		queue = queue[1:]
		if seen[f] {
			continue
		}
		seen[f] = true
		b, err := s.specReader(ctx, productID, f)
		if err != nil || len(b) > maxSpecFileBytes {
			continue
		}
		src := string(b)
		files = append(files, srcFile{f, src})
		for _, m := range importRe.FindAllStringSubmatch(src, -1) {
			p := resolveImport(path.Dir(f), m[1], aliases)
			if p == "" || p == "." || strings.HasPrefix(p, "..") {
				continue
			}
			for _, cand := range []string{p, p + ".ts", p + ".js", p + "/index.ts"} {
				if _, err := s.specReader(ctx, productID, cand); err == nil {
					queue = append(queue, cand)
					break
				}
			}
		}
	}
	// The files of the spec's own service come first: a spec also imports shared fixtures and
	// utilities, whose calls are not the defect's operations (found live: with the shared files
	// first, the five-name cap was spent before the service's operations file was reached).
	svc := serviceSegment(spec)
	sort.SliceStable(files, func(i, j int) bool {
		return svc != "" && strings.Contains(files[i].path, "/"+svc+"/") && !strings.Contains(files[j].path, "/"+svc+"/")
	})
	sources := make([]string, 0, len(files))
	for _, f := range files {
		sources = append(sources, f.text)
	}
	all := allOperations(sources...)
	if len(sources) > 0 { // the spec is the first file read
		all = rankByHint(all, sources[0])
	}
	if len(all) > maxOperations {
		all = all[:maxOperations]
	}
	return all
}

var camelWordRe = regexp.MustCompile(`[A-Z]?[a-z]{3,}`)

// rankByHint puts the operations whose words appear in the spec first ("editMedicationDispense"
// for a spec that calls editDispense), keeping the order of appearance among equals. Found live:
// the first five names of a service's helper files were unrelated to the defect's spec.
func rankByHint(ops []string, spec string) []string {
	words := map[string]bool{}
	for _, w := range camelWordRe.FindAllString(spec, -1) {
		words[strings.ToLower(w)] = true
	}
	score := func(op string) int {
		n := 0
		for _, w := range camelWordRe.FindAllString(op, -1) {
			if words[strings.ToLower(w)] {
				n++
			}
		}
		return n
	}
	out := append([]string(nil), ops...)
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	return out
}

// serviceSegment is the directory after graphql/ or rest/ in a spec path ("his-backend" for
// tests/api/graphql/his-backend/pharmacy/x.spec.ts), or "".
func serviceSegment(spec string) string {
	segs := strings.Split(spec, "/")
	for i, g := range segs {
		if (g == "graphql" || g == "rest") && i+2 < len(segs) {
			return segs[i+1]
		}
	}
	return ""
}

// ---- the push ----

type pendingDefect struct {
	code, kind, defectID, title, excerpt, runRef, hash, level string
	runAt                                                     int64
	specs                                                     []byte
}

func (s *Syncer) pendingDefects(ctx context.Context, productID int) ([]pendingDefect, error) {
	rows, err := s.db.Query(ctx, `SELECT d.case_code, d.kind, d.defect_id, d.title, COALESCE(d.excerpt,''), d.run_ref,
  TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', d.run_at), d.hash, c.level, c.spec_paths
FROM qa_defect_state d JOIN kb_test_cases c ON c.product_id = d.product_id AND c.code = d.case_code AND c.removed_at IS NULL
WHERE d.product_id = ? AND d.hash <> COALESCE(d.pushed_hash, '') AND d.hash <> COALESCE(d.rejected_hash, '')
ORDER BY d.case_code LIMIT 500`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingDefect
	for rows.Next() {
		var p pendingDefect
		var specs sql.RawBytes
		if err := rows.Scan(&p.code, &p.kind, &p.defectID, &p.title, &p.excerpt, &p.runRef, &p.runAt, &p.hash, &p.level, &specs); err != nil {
			return nil, err
		}
		p.specs = append([]byte(nil), specs...)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Syncer) markDefect(ctx context.Context, productID int, p pendingDefect, pushed bool, reason string) {
	var err error
	if pushed {
		_, err = s.db.Exec(ctx, `UPDATE qa_defect_state SET pushed_hash = ?, pushed_at = UTC_TIMESTAMP() WHERE product_id = ? AND case_code = ? AND hash = ?`, p.hash, productID, p.code, p.hash)
	} else {
		_, err = s.db.Exec(ctx, `UPDATE qa_defect_state SET rejected_hash = ?, rejected_reason = ? WHERE product_id = ? AND case_code = ? AND hash = ?`, p.hash, clipRunes(reason, 80), productID, p.code, p.hash)
	}
	if err != nil {
		log.Printf("gaossync: product %d: record defect outcome: %v", productID, err)
	}
}

func (s *Syncer) setDefectBackoff(ctx context.Context, productID int, d time.Duration, note string) {
	if _, err := s.db.Exec(ctx, `INSERT INTO qa_defect_push (product_id, backoff_until, note) VALUES (?, DATE_ADD(UTC_TIMESTAMP(), INTERVAL ? SECOND), ?)
ON DUPLICATE KEY UPDATE backoff_until = VALUES(backoff_until), note = VALUES(note)`, productID, int(d.Seconds()), clipRunes(note, 120)); err != nil {
		log.Printf("gaossync: product %d: defect backoff: %v", productID, err)
	}
}

func (s *Syncer) defectBackoffActive(ctx context.Context, productID int) bool {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM qa_defect_push WHERE product_id = ? AND backoff_until > UTC_TIMESTAMP()`, productID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// buildDefectItem derives the item of one pending state. ok is false (with a reason) for a state
// GAOS could not take: the case has no usable spec or a level GAOS does not know.
func (s *Syncer) buildDefectItem(ctx context.Context, productID int, p pendingDefect, repos map[string]string, mapping map[string][]string) (defectItem, string, bool) {
	var specs []string
	_ = json.Unmarshal(p.specs, &specs)
	spec := usableSpec(specs)
	autoType := AutomationType(p.level)
	switch {
	case spec == "":
		return defectItem{}, "no-usable-spec", false
	case autoType == "":
		return defectItem{}, "bad-level", false
	case p.runAt <= 0:
		return defectItem{}, "no-run-time", false
	}
	title := clipRunes(strings.TrimSpace(p.title), maxTitle)
	if title == "" {
		title = p.code
	}
	it := defectItem{
		CaseID: p.code, DefectID: p.defectID, Kind: p.kind, Level: p.level, AutomationType: autoType,
		Backend: BackendKind(spec), TargetRepos: targetsFor(spec, autoType, repos, mapping),
		Operations: s.operationsFor(ctx, productID, spec), Spec: spec, Title: title,
		Excerpt: clipBytes(p.excerpt, maxExcerpt), RunAt: p.runAt,
	}
	if ref := p.runRef; ref != "" && len(ref) <= maxRunRef && printableASCII(ref) {
		it.RunRef = ref
	}
	return it, "", true
}

// pushDefects sends the pending defect states. It returns a Failure for a failed request (the pass
// is then failed and retried by the outbox), stopped when the flag or the id changed. An older GAOS
// (404 or 405) is not a failure: the states stay pending and the push waits unsupportedWait.
func (s *Syncer) pushDefects(ctx context.Context, productID int, pid string, res *Result) (f *Failure, stopped bool) {
	if s.defectBackoffActive(ctx, productID) {
		return nil, false
	}
	pending, err := s.pendingDefects(ctx, productID)
	if err != nil {
		log.Printf("gaossync: product %d: read pending defects: %v", productID, err)
		return nil, false // the defect half must never fail the cases and results already sent
	}
	if len(pending) == 0 {
		return nil, false
	}
	repos, mapping, err := s.loadTargets(ctx, productID)
	if err != nil {
		log.Printf("gaossync: product %d: read defect targets: %v", productID, err)
		repos, mapping = map[string]string{}, map[string][]string{}
	}
	var items []defectItem
	byCase := map[string]pendingDefect{}
	for _, p := range pending {
		it, why, ok := s.buildDefectItem(ctx, productID, p, repos, mapping)
		if !ok {
			log.Printf("gaossync: product %d: defect of %s is not sent: %s", productID, p.code, why)
			s.markDefect(ctx, productID, p, false, why)
			continue
		}
		items = append(items, it)
		byCase[p.code] = p
	}
	path := "/products/" + pid + "/qa/defects"
	size := defectBatchMax
	for i := 0; i < len(items); {
		if !s.stillCurrent(ctx, productID, pid) {
			return nil, true
		}
		batch := fitDefects(items[i:], size)
		step := fmt.Sprintf("defects from %d/%d", i+1, len(items))
		var out defectsResponse
		if f := s.cl.post(ctx, path, step, defectsBody{Source: "push", Items: batch}, &out); f != nil {
			switch f.Status {
			case http.StatusNotFound, http.StatusMethodNotAllowed:
				s.setDefectBackoff(ctx, productID, unsupportedWait, fmt.Sprintf("GAOS answered HTTP %d: no defect route", f.Status))
				log.Printf("gaossync: product %d: GAOS has no defect route yet (HTTP %d); trying again in %s", productID, f.Status, unsupportedWait)
				return nil, false
			case http.StatusRequestEntityTooLarge:
				if len(batch) == 1 {
					s.markDefect(ctx, productID, byCase[batch[0].CaseID], false, "too-large")
					i++
					continue
				}
				size = len(batch) / 2
				continue
			}
			return f, false
		}
		if err := out.check(len(batch)); err != nil {
			return &Failure{Note: step + ": GAOS answered 200 with an unreadable or incomplete body"}, false
		}
		rej := map[string]string{}
		for _, x := range *out.Rejected {
			rej[x.CaseID] = x.Reason
		}
		capped := false
		for _, it := range batch {
			p := byCase[it.CaseID]
			if reason, bad := rej[it.CaseID]; bad {
				if reason == "defect-limit" { // the daily cap is not a verdict on the item: keep it pending
					capped = true
					continue
				}
				log.Printf("gaossync: product %d: GAOS rejected the defect of %s: %s", productID, it.CaseID, clipRunes(reason, 80))
				s.markDefect(ctx, productID, p, false, reason)
				res.Rejected++
				continue
			}
			s.markDefect(ctx, productID, p, true, "")
		}
		res.Batches++
		if capped {
			s.setDefectBackoff(ctx, productID, capWait, "GAOS reached its daily limit of new defects")
			return nil, false
		}
		i += len(batch)
	}
	return nil, false
}

// fitDefects takes the longest prefix of items that is at most n items and at most defectBodyMax bytes
// (at least one item).
func fitDefects(items []defectItem, n int) []defectItem {
	if n < 1 {
		n = 1
	}
	total := 0
	for k := 0; k < len(items) && k < n; k++ {
		b, _ := json.Marshal(items[k])
		total += len(b) + 1
		if total > defectBodyMax && k > 0 {
			return items[:k]
		}
	}
	return items[:min(n, len(items))]
}

// tsAlias is one tsconfig "paths" entry, "@api/*" -> "src/api/*", as a prefix pair.
type tsAlias struct{ from, to string }

// tsAliases reads compilerOptions.paths of the automation repo's tsconfig.json. The spec reaches
// its helpers through these aliases ("@api/..."), so the relative imports alone find nothing.
// Any failure means no aliases; a wildcard-free entry ("@fixtures") maps the exact name.
func (s *Syncer) tsAliases(ctx context.Context, productID int) []tsAlias {
	b, err := s.specReader(ctx, productID, "tsconfig.json")
	if err != nil || len(b) > maxSpecFileBytes {
		return nil
	}
	var cfg struct {
		CompilerOptions struct {
			Paths map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if json.Unmarshal([]byte(lineCmtRe.ReplaceAllString(string(b), "")), &cfg) != nil {
		return nil
	}
	var out []tsAlias
	for k, v := range cfg.CompilerOptions.Paths {
		if len(v) == 0 {
			continue
		}
		out = append(out, tsAlias{strings.TrimSuffix(k, "*"), strings.TrimSuffix(path.Clean(strings.TrimSuffix(v[0], "*")), "/")})
		if strings.HasSuffix(k, "*") {
			out[len(out)-1].to += "/"
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i].from) > len(out[j].from) })
	return out
}

// resolveImport turns an import specifier into a repo-relative path, or "" for a package.
func resolveImport(dir, spec string, aliases []tsAlias) string {
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return path.Join(dir, spec)
	}
	for _, a := range aliases {
		if strings.HasSuffix(a.from, "/") && strings.HasPrefix(spec, a.from) {
			return path.Join(a.to, strings.TrimPrefix(spec, a.from))
		}
		if a.from == spec {
			return a.to
		}
	}
	return ""
}
