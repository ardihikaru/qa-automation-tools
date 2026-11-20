package navmap

import (
	"fmt"
	"sort"
	"strings"
)

// Route statuses (NAVIGATION_MAP.md section 6).
const (
	StatusFailing   = "failing"
	StatusPassing   = "passing"
	StatusAutomated = "automated"
	StatusPlanned   = "planned"
	StatusUntested  = "untested"
)

// Statuses lists the valid statuses.
var Statuses = []string{StatusFailing, StatusPassing, StatusAutomated, StatusPlanned, StatusUntested}

// SourceObserved marks a case link that comes from a real run (nav_observations).
const SourceObserved = "observed"

// CaseFact is what the coverage needs to know about an active case.
type CaseFact struct {
	Code, Status, Priority, Epic string
	Result                       string // latest kb_case_results result: pass | fail | skipped | ""
	Automated                    bool
	SpecPaths                    []string
}

// CaseLink is one direct link of a case to a route: scenario, spec or observed.
type CaseLink struct {
	Case, Method, Source string
}

// RouteStats is a route's coverage.
type RouteStats struct {
	Status    string   `json:"status"`
	Cases     int      `json:"cases"`
	Automated int      `json:"automated"`
	Planned   int      `json:"planned"`
	Pass      int      `json:"pass"`
	Fail      int      `json:"fail"`
	CaseCodes []string `json:"-"`
}

// CaseSets returns each route's active case codes (sorted): its direct links; a wildcard route
// "X/*" also has the cases of every route of the same surface and app under "X/", and so has a
// route X (not "/") with no case of its own.
func CaseSets(routes []Route, links map[string][]CaseLink, facts map[string]CaseFact) map[string][]string {
	own := map[string]map[string]bool{}
	for _, r := range routes {
		set := map[string]bool{}
		for _, l := range links[r.Key] {
			if _, ok := facts[l.Case]; ok {
				set[l.Case] = true
			}
		}
		own[r.Key] = set
	}
	out := map[string][]string{}
	for _, r := range routes {
		set := map[string]bool{}
		for c := range own[r.Key] {
			set[c] = true
		}
		// A wildcard route, and a route with no case of its own (a catalog prefix such as
		// /api/v1/kiosk-sut-v2), stand for the routes under them.
		// The app root "/" never stands for the whole app.
		if strings.HasSuffix(r.Path, "/*") || (len(set) == 0 && r.Path != "/") {
			prefix := strings.TrimSuffix(r.Path, "*")
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			for _, o := range routes {
				if o.Key != r.Key && o.Surface == r.Surface && o.App == r.App && strings.HasPrefix(o.Path, prefix) {
					for c := range own[o.Key] {
						set[c] = true
					}
				}
			}
		}
		out[r.Key] = sortedKeys(set)
	}
	return out
}

// Coverage computes each route's stats and status.
func Coverage(routes []Route, links map[string][]CaseLink, facts map[string]CaseFact) map[string]RouteStats {
	sets := CaseSets(routes, links, facts)
	out := map[string]RouteStats{}
	for _, r := range routes {
		st := RouteStats{CaseCodes: sets[r.Key]}
		for _, c := range st.CaseCodes {
			f := facts[c]
			st.Cases++
			if f.Status == "planned" {
				st.Planned++
			}
			if f.Automated {
				st.Automated++
				switch f.Result {
				case "pass":
					st.Pass++
				case "fail":
					st.Fail++
				}
			}
		}
		st.Status = statusOf(st)
		out[r.Key] = st
	}
	return out
}

func statusOf(st RouteStats) string {
	switch {
	case st.Fail > 0:
		return StatusFailing
	case st.Pass > 0:
		return StatusPassing
	case st.Automated > 0:
		return StatusAutomated
	case st.Cases > 0:
		return StatusPlanned
	}
	return StatusUntested
}

// NextItem is one suggested case to automate.
type NextItem struct {
	CaseCode    string `json:"case_code"`
	Priority    string `json:"priority"`
	Epic        string `json:"epic"`
	RouteKey    string `json:"route_key"`
	Path        string `json:"path"`
	App         string `json:"app"`
	Surface     string `json:"surface"`
	Method      string `json:"method"`
	Tier        int    `json:"tier"`
	Reason      string `json:"reason"`
	ExampleCase string `json:"example_case"`
	ExampleSpec string `json:"example_spec"`
}

var priorityRank = map[string]int{"P0": 0, "P1": 1, "P2": 2, "P3": 3}

func prio(p string) int {
	if r, ok := priorityRank[p]; ok {
		return r
	}
	return 4
}

// Next suggests eligible cases on routes that already have a passing automated case (section 6):
// tier 1 when a passing case's direct link has the same method (any method on a web route), tier 2
// otherwise. One entry per case (best tier, then the smallest path); ordered by tier, priority, code.
func Next(routes []Route, links map[string][]CaseLink, facts map[string]CaseFact, eligible map[string]bool, limit int) []NextItem {
	best := map[string]NextItem{}
	for _, r := range routes {
		// Passing automated cases on this route, by link method.
		passing := map[string][]string{} // method -> case codes
		var any []string
		for _, l := range links[r.Key] {
			f, ok := facts[l.Case]
			if !ok || !f.Automated || f.Result != "pass" {
				continue
			}
			passing[l.Method] = append(passing[l.Method], l.Case)
			any = append(any, l.Case)
		}
		if len(any) == 0 {
			continue
		}
		for _, l := range links[r.Key] {
			if l.Source != SourceScenario || !eligible[l.Case] {
				continue
			}
			f, ok := facts[l.Case]
			if !ok {
				continue
			}
			tier, ex := 2, any
			if r.Surface == SurfaceWeb {
				tier = 1
			} else if same := passing[l.Method]; l.Method != "" && len(same) > 0 {
				tier, ex = 1, same
			}
			example := minString(ex)
			spec := ""
			if sp := facts[example].SpecPaths; len(sp) > 0 {
				spec = sp[0]
			}
			item := NextItem{
				CaseCode: l.Case, Priority: f.Priority, Epic: f.Epic, RouteKey: r.Key, Path: r.Path, App: r.App, Surface: r.Surface,
				Method: l.Method, Tier: tier, ExampleCase: example, ExampleSpec: spec,
			}
			label := strings.TrimSpace(l.Method + " " + r.Path)
			if tier == 1 {
				item.Reason = fmt.Sprintf("%s has a passing %scase (%s)", label, methodWord(l.Method, r.Surface), example)
			} else {
				item.Reason = fmt.Sprintf("%s has a passing case with another method (%s)", r.Path, example)
			}
			old, seen := best[l.Case]
			if !seen || item.Tier < old.Tier || (item.Tier == old.Tier && item.Path < old.Path) {
				best[l.Case] = item
			}
		}
	}
	out := make([]NextItem, 0, len(best))
	for _, it := range best {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Tier != b.Tier {
			return a.Tier < b.Tier
		}
		if prio(a.Priority) != prio(b.Priority) {
			return prio(a.Priority) < prio(b.Priority)
		}
		return a.CaseCode < b.CaseCode
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func methodWord(m, surface string) string {
	if surface == SurfaceWeb || m == "" {
		return ""
	}
	return m + " "
}

func minString(xs []string) string {
	m := ""
	for _, x := range xs {
		if m == "" || x < m {
			m = x
		}
	}
	return m
}
