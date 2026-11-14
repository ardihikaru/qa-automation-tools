package gaossync

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	caseCodeRe  = regexp.MustCompile(`^TC-([A-Z]+)-[0-9]{3}$`)
	epicCodeRe  = regexp.MustCompile(`^[A-Z]{2,8}$`)
	moduleRe    = regexp.MustCompile(`^[A-D][0-9]{2}$`)
	storyRe     = regexp.MustCompile(`^US-[A-Z]+-[0-9]{2}$`)
	featureRe   = regexp.MustCompile(`^[A-D][0-9]{2}\.[0-9]{2,3}(\.[0-9]{2})?$`)
	levels      = map[string]bool{"API": true, "GQL": true, "WEB": true, "E2E": true}
	priorities  = map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
	polarities  = map[string]bool{"+": true, "-": true}
	maxScenario = 600
	maxSpec     = 240
	maxFeatures = 12
	maxTitle    = 200
)

type catCaseT struct {
	code, epic, story, scenario, polarity, level, priority, status string
	specPaths                                                      []string
}

type catEpic struct{ title, service string }

type catalog struct {
	cases    []catCaseT
	features map[string][]storyFeature // story code -> its features, ordered by feature code
	epics    map[string]catEpic
	results  map[string]storedRun // case code -> its latest stored run (kb_case_results)
}

// storedRun is one kb_case_results row; runAt is unix seconds.
type storedRun struct {
	result, runRef string
	runAt          int64
}

type storyFeature struct{ code, module string }

// readCatalog reads one consistent snapshot of the product's active cases, story links and epics.
func (s *Syncer) readCatalog(ctx context.Context, productID int) (*catalog, error) {
	tx, err := s.db.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	cat := &catalog{features: map[string][]storyFeature{}, epics: map[string]catEpic{}, results: map[string]storedRun{}}

	rows, err := tx.QueryContext(ctx, `SELECT code, epic_code, COALESCE(story_code,''), COALESCE(scenario,''), polarity, level, priority, status, spec_paths
FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL ORDER BY code`, productID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c catCaseT
		var specs []byte
		if err := rows.Scan(&c.code, &c.epic, &c.story, &c.scenario, &c.polarity, &c.level, &c.priority, &c.status, &specs); err != nil {
			rows.Close()
			return nil, err
		}
		if len(specs) > 0 {
			_ = json.Unmarshal(specs, &c.specPaths) // a malformed value means no spec
		}
		cat.cases = append(cat.cases, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	frows, err := tx.QueryContext(ctx, `SELECT sf.story_code, sf.feature_code, f.module_code FROM kb_story_features sf
JOIN kb_features f ON f.product_id = sf.product_id AND f.code = sf.feature_code AND f.removed_at IS NULL
WHERE sf.product_id = ? ORDER BY sf.feature_code`, productID)
	if err != nil {
		return nil, err
	}
	for frows.Next() {
		var story string
		var f storyFeature
		if err := frows.Scan(&story, &f.code, &f.module); err != nil {
			frows.Close()
			return nil, err
		}
		cat.features[story] = append(cat.features[story], f)
	}
	frows.Close()
	if err := frows.Err(); err != nil {
		return nil, err
	}

	erows, err := tx.QueryContext(ctx, `SELECT code, title, service FROM kb_epics WHERE product_id = ? AND removed_at IS NULL`, productID)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var code string
		var e catEpic
		if err := erows.Scan(&code, &e.title, &e.service); err != nil {
			erows.Close()
			return nil, err
		}
		cat.epics[code] = e
	}
	erows.Close()
	if err := erows.Err(); err != nil {
		return nil, err
	}

	// run_at is a UTC DATETIME: the difference from the epoch is independent of the session time zone.
	rrows, err := tx.QueryContext(ctx, `SELECT case_code, result, TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', run_at), run_ref
FROM kb_case_results WHERE product_id = ?`, productID)
	if err != nil {
		return nil, err
	}
	for rrows.Next() {
		var code string
		var r storedRun
		if err := rrows.Scan(&code, &r.result, &r.runAt, &r.runRef); err != nil {
			rrows.Close()
			return nil, err
		}
		cat.results[code] = r
	}
	rrows.Close()
	return cat, rrows.Err()
}

type caseItem struct {
	CaseID      string   `json:"caseId"`
	Epic        string   `json:"epic"`
	Module      string   `json:"module"`
	Features    []string `json:"features"`
	UserStory   string   `json:"userStory"`
	Scenario    string   `json:"scenario"`
	Polarity    string   `json:"polarity"`
	Level       string   `json:"level"`
	Pri         string   `json:"pri"`
	Decision    string   `json:"decision"`
	Reason      string   `json:"reason"`
	Implemented bool     `json:"implemented"`
	Spec        string   `json:"spec"`
}

type epicItem struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Service string `json:"service"`
	Source  string `json:"source"`
	Module  string `json:"module"`
}

type casesBody struct {
	Source string     `json:"source"`
	Epics  []epicItem `json:"epics"`
	Cases  []caseItem `json:"cases"`
}

// resultItem carries the implementation state and, when a run is stored for the case, the run
// record. GAOS wants result and runAt together (runAt in unix SECONDS), runRef only with runAt.
type resultItem struct {
	CaseID      string `json:"caseId"`
	Implemented bool   `json:"implemented"`
	Spec        string `json:"spec"`
	Result      string `json:"result,omitempty"`
	RunAt       int64  `json:"runAt,omitempty"`
	RunRef      string `json:"runRef,omitempty"`
}

var runResults = map[string]bool{"pass": true, "fail": true, "skipped": true}

const maxRunRef = 120

// withRun adds the stored run to the item. A run that GAOS could not take whole (unknown result,
// no timestamp) is left out; a runRef that is not printable ASCII or is too long is dropped alone.
func (it resultItem) withRun(r storedRun, ok bool) resultItem {
	if !ok || !runResults[r.result] || r.runAt <= 0 {
		return it
	}
	it.Result, it.RunAt = r.result, r.runAt
	if ref := r.runRef; ref != "" && len(ref) <= maxRunRef && printableASCII(ref) {
		it.RunRef = ref
	}
	return it
}

func printableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

type resultsBody struct {
	Source string       `json:"source"`
	Items  []resultItem `json:"items"`
}

type plan struct {
	cases   []caseItem
	epics   map[string]epicItem
	skipped int
	noSpec  int
}

// buildPlan applies the skip rules and the field mapping. Epic entries are computed once over
// every sent case, so an epic that spans batches is identical in all of them.
func buildPlan(cat *catalog) *plan {
	p := &plan{epics: map[string]epicItem{}}
	modulesByEpic := map[string]map[string]int{}
	for _, c := range cat.cases {
		m := caseCodeRe.FindStringSubmatch(c.code)
		if (c.status != "planned" && c.status != "automated") || m == nil || m[1] != c.epic || !epicCodeRe.MatchString(c.epic) ||
			!polarities[c.polarity] || !levels[c.level] || !priorities[c.priority] {
			p.skipped++
			continue
		}
		item := caseItem{
			CaseID: c.code, Epic: c.epic, Features: []string{}, Scenario: clipBytes(c.scenario, maxScenario),
			Polarity: c.polarity, Level: c.level, Pri: c.priority, Decision: "planned",
		}
		if storyRe.MatchString(c.story) {
			item.UserStory = c.story
		}
		for _, f := range cat.features[c.story] {
			if featureRe.MatchString(f.code) {
				item.Features = append(item.Features, f.code)
				// The greatest module code among the case's features: this is what GAOS's seed holds
				// for all 57 multi-module cases of HISv3 (GAOS_INBOUND.md, "Module rule").
				if moduleRe.MatchString(f.module) && f.module > item.Module {
					item.Module = f.module
				}
			}
		}
		sort.Strings(item.Features)
		if len(item.Features) > maxFeatures {
			item.Features = item.Features[:maxFeatures]
		}
		if c.status == "automated" {
			if sp := usableSpec(c.specPaths); sp != "" {
				item.Implemented, item.Spec = true, sp
			} else {
				p.noSpec++
			}
		}
		p.cases = append(p.cases, item)
		if item.Module != "" {
			if modulesByEpic[c.epic] == nil {
				modulesByEpic[c.epic] = map[string]int{}
			}
			modulesByEpic[c.epic][item.Module]++
		}
		if _, ok := p.epics[c.epic]; !ok {
			e, found := cat.epics[c.epic]
			title := e.title
			if !found || strings.TrimSpace(title) == "" {
				title = c.epic
			}
			p.epics[c.epic] = epicItem{Code: c.epic, Title: clipRunes(title, maxTitle), Service: e.service, Source: sourceName}
		}
	}
	for code, counts := range modulesByEpic {
		best, n := "", 0
		for m, k := range counts {
			if k > n || (k == n && m < best) {
				best, n = m, k
			}
		}
		e := p.epics[code]
		e.Module = best
		p.epics[code] = e
	}
	return p
}

// epicsFor lists the epic entries used by a batch, ordered by code.
func (p *plan) epicsFor(chunk []caseItem) []epicItem {
	seen := map[string]bool{}
	out := []epicItem{}
	for _, c := range chunk {
		if !seen[c.Epic] {
			seen[c.Epic] = true
			out = append(out, p.epics[c.Epic])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// usableSpec returns the first spec path when it is a short relative path, else "".
func usableSpec(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	sp := strings.TrimSpace(paths[0])
	if sp == "" || len(sp) > maxSpec || !utf8.ValidString(sp) || strings.HasPrefix(sp, "/") || strings.Contains(sp, `\`) || strings.Contains(sp, "://") {
		return ""
	}
	for _, seg := range strings.Split(sp, "/") {
		if seg == ".." {
			return ""
		}
	}
	return sp
}

// clipBytes limits s to max bytes: longer text is cut on a rune boundary and ends in "...".
func clipBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// clipRunes limits s to max runes ("..." included).
func clipRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-3]) + "..."
}

func clipNote(s string) string { return clipRunes(s, noteMax) }

func noProxyTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil // the bearer token must never travel through an environment proxy
	return t
}
