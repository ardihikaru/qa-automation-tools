package knowledge

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// testCatalog parses the layout of example-org/automation-test-sut docs/ as defined by its
// scripts/test-catalog.mjs (ported at commit 521618a). Duplicates resolve "last wins" like the
// script's Map.set, and every duplicate is also a Problem.
type testCatalog struct{}

func init() { register(testCatalog{}) }

// TestCatalogFormatName is the knowledge_format value of the SUT test catalog.
const TestCatalogFormatName = "test-catalog-v1"

func (testCatalog) Name() string { return TestCatalogFormatName }
func (testCatalog) Version() int { return 1 }

const (
	modulesDir = "docs/modules/"
	catalogDir = "docs/test-catalog/"
)

var (
	partFileRe   = regexp.MustCompile(`^docs/modules/([A-D])-[^/]+\.md$`)
	moduleHeadRe = regexp.MustCompile(`^### ([A-D]\d{2}) — (.+)$`)
	featureIDRe  = regexp.MustCompile(`^(?:<a id="[^"]*"></a>)?([A-D]\d{2}\.\d{2,3}(?:\.\d{2})?)$`)
	featureRefRe = regexp.MustCompile(`[A-D]\d{2}\.\d{2,3}(?:\.\d{2})?`)
	storyLineRe  = regexp.MustCompile(`^- \*\*(US-[A-Z]{1,40}-\d{2})\*\*(.*?)\s+—\s+(.*)$`)
	unmappedRe   = regexp.MustCompile(`\(unmapped:\s*([^)]+)\)`)
	caseRowRe    = regexp.MustCompile(`^\|\s*[☐☑]\s*\|`)
	caseIDRe     = regexp.MustCompile(`^TC-[A-Z]{1,40}-\d{3}$`)
	storyIDRe    = regexp.MustCompile(`^US-[A-Z]{1,40}-\d{2}$`)
	backtickRe   = regexp.MustCompile("`([^`]+)`")
	releaseTokRe = regexp.MustCompile("`([A-Z-]+)`")
)

// ErrEmptyCatalog means nothing parsed: wrong format or wrong path. The sync fails instead of
// soft-deleting everything.
var ErrEmptyCatalog = errors.New("no features and no test cases found under docs/: wrong catalog format or layout")

// splitLines splits on \n and strips one trailing \r per line.
func splitLines(b []byte) []string {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// cells mirrors line.split('|').slice(1, -1).map(trim).
func cells(line string) []string {
	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		return nil
	}
	parts = parts[1 : len(parts)-1]
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func firstHeading(lines []string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(l, "# "))
		}
	}
	return ""
}

func sortedKeys(files map[string][]byte, keep func(string) bool) []string {
	var out []string
	for k := range files {
		if keep(k) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func (testCatalog) Parse(files map[string][]byte) (*Snapshot, []Problem, error) {
	var problems []Problem
	prob := func(file string, line int, format string, a ...any) {
		problems = append(problems, Problem{File: file, Line: line, Msg: fmt.Sprintf(format, a...)})
	}

	// ------------------------------------------------------------ module catalogue
	parts := map[string]Part{}
	modules := map[string]Module{}
	features := map[string]Feature{}
	submods := map[string]Submodule{} // key: code

	partFiles := sortedKeys(files, func(k string) bool { return partFileRe.MatchString(k) })
	for _, file := range partFiles {
		lines := splitLines(files[file])
		pc := partFileRe.FindStringSubmatch(file)[1]
		name := firstHeading(lines)
		if name == "" {
			name = strings.TrimSuffix(path.Base(file), ".md")
		}
		parts[pc] = Part{Code: pc, Name: name, SourceFile: file}

		var cur *Module
		for i, line := range lines {
			if h := moduleHeadRe.FindStringSubmatch(line); h != nil {
				m := Module{Code: h[1], PartCode: h[1][:1], Name: strings.TrimSpace(h[2]), SourceFile: file, SourceLine: i + 1}
				if _, dup := modules[m.Code]; dup {
					prob(file, i+1, "duplicate module %s", m.Code)
				}
				modules[m.Code] = m
				cur = &m
				continue
			}
			if !strings.HasPrefix(line, "|") {
				continue
			}
			c := cells(line)
			if len(c) == 0 {
				continue
			}
			idm := featureIDRe.FindStringSubmatch(c[0])
			if idm == nil {
				continue
			}
			fid := idm[1]
			if len(c) != 12 {
				prob(file, i+1, "feature row needs 12 columns, found %d", len(c))
				continue
			}
			if _, dup := features[fid]; dup {
				prob(file, i+1, "duplicate feature %s", fid)
			}
			modCode := fid[:3]
			if cur == nil || !strings.HasPrefix(fid, cur.Code+".") {
				prob(file, i+1, "%s is not under its module heading", fid)
			}
			status := ""
			if m := releaseTokRe.FindStringSubmatch(c[4]); m != nil {
				status = m[1]
			}
			if !contains(ReleaseStatuses, status) {
				prob(file, i+1, "%s unknown release status %q", fid, c[4])
				status = "UNKNOWN"
			}
			asOf := c[5]
			if asOf == "—" {
				asOf = ""
			}
			f := Feature{
				Code: fid, ModuleCode: modCode, Name: c[2], RepoEntry: c[3], ReleaseStatus: status,
				StatusAsOf: asOf, Evidence: c[6], Notes: c[7], SourceFile: file, SourceLine: i + 1,
			}
			if sub := strings.TrimSpace(c[1]); sub != "" {
				code := submoduleCode(modCode, sub)
				if _, ok := submods[code]; !ok {
					submods[code] = Submodule{Code: code, ModuleCode: modCode, Name: sub}
				}
				f.SubmoduleCode = code
			}
			features[fid] = f
		}
	}

	// ------------------------------------------------------------ test catalog
	epicFiles := sortedKeys(files, func(k string) bool {
		if !strings.HasPrefix(k, catalogDir) || !strings.HasSuffix(k, ".md") {
			return false
		}
		return k != catalogDir+"README.md" && !strings.HasPrefix(path.Base(k), "_")
	})
	epics := map[string]Epic{}
	epicOfFile := map[string]string{}
	stories := map[string]Story{}
	cases := map[string]TestCase{}

	for _, file := range epicFiles {
		rel := strings.TrimPrefix(file, catalogDir)
		lines := splitLines(files[file])
		segs := strings.Split(rel, "/")
		if len(segs) == 1 {
			prob(file, 0, "epic files belong in a service folder (sut-backend/, vendor-backend/, data-integration/, …)")
		}
		e := Epic{Title: firstHeading(lines), SourceFile: file}
		if e.Title == "" {
			e.Title = path.Base(file)
		}
		if len(segs) > 1 {
			e.Service = segs[0]
		}
		if len(segs) > 2 {
			e.Area = strings.Join(segs[1:len(segs)-1], "/")
		}

		// Stories first (the script's per-file order), so a case can only name a story seen so far.
		for i, line := range lines {
			sm := storyLineRe.FindStringSubmatch(line)
			if sm == nil {
				continue
			}
			id, mid, text := sm[1], sm[2], sm[3]
			if old, dup := stories[id]; dup {
				prob(file, i+1, "duplicate story %s (also in %s)", id, old.SourceFile)
			}
			s := Story{Code: id, EpicCode: strings.Split(id, "-")[1], Text: strings.TrimSpace(text), SourceFile: file, SourceLine: i + 1, FeatureCodes: []string{}}
			if um := unmappedRe.FindStringSubmatch(mid); um != nil {
				s.UnmappedReason = strings.TrimSpace(um[1])
			} else {
				seen := map[string]bool{}
				for _, f := range featureRefRe.FindAllString(mid, -1) {
					if !seen[f] {
						seen[f] = true
						s.FeatureCodes = append(s.FeatureCodes, f)
					}
				}
				if len(s.FeatureCodes) == 0 {
					prob(file, i+1, `%s names no module feature; add "([A01.07](…))" or "(unmapped: <why>)"`, id)
				}
			}
			if e.Code == "" {
				e.Code = s.EpicCode
			}
			stories[id] = s
		}
		if e.Code == "" {
			e.Code = strings.ToUpper(slug(strings.TrimSuffix(rel, ".md"), 64))
		}
		if old, dup := epics[e.Code]; dup {
			prob(file, 0, "duplicate epic code %s (also in %s)", e.Code, old.SourceFile)
		} else {
			epics[e.Code] = e
		}
		epicOfFile[file] = e.Code

		for i, line := range lines {
			if !caseRowRe.MatchString(line) {
				continue
			}
			c := cells(line)
			if len(c) != 10 {
				prob(file, i+1, "expected 10 columns, found %d", len(c))
				continue
			}
			box, id, story, scenario, polarity, level, priority, status, spec, reason := c[0], c[1], c[2], c[3], c[4], c[5], c[6], c[7], c[8], c[9]
			if !caseIDRe.MatchString(id) {
				prob(file, i+1, "bad test case id %q", id)
				continue
			}
			if !contains(CaseStatuses, status) {
				prob(file, i+1, "%s unknown status %q", id, status)
				continue
			}
			if old, dup := cases[id]; dup {
				prob(file, i+1, "duplicate id %s (also in %s:%d)", id, old.SourceFile, old.SourceLine)
			}
			tc := TestCase{
				Code: id, EpicCode: e.Code, Scenario: scenario, Polarity: clipRunes(polarity, 4), Level: clipRunes(level, 8),
				Priority: clipRunes(priority, 4), Status: status, Automated: status == "automated",
				SpecPaths: []string{}, SourceFile: file, SourceLine: i + 1,
			}
			switch {
			case !storyIDRe.MatchString(story):
				prob(file, i+1, "%s has bad story id %q", id, story)
			case !hasStory(stories, story):
				prob(file, i+1, "%s story %s is not listed under \"User stories\"", id, story)
			default:
				tc.StoryCode = story
			}
			if !contains(Polarities, polarity) {
				prob(file, i+1, "%s polarity must be + or -", id)
			}
			if !contains(Levels, level) {
				prob(file, i+1, "%s unknown level %q", id, level)
			}
			if !contains(Priorities, priority) {
				prob(file, i+1, "%s unknown priority %q", id, priority)
			}
			if (box == "☑") != tc.Automated {
				prob(file, i+1, "%s checkbox must be ☑ exactly when status is automated", id)
			}
			for _, m := range backtickRe.FindAllStringSubmatch(spec, -1) {
				tc.SpecPaths = append(tc.SpecPaths, m[1])
			}
			if tc.Automated && len(tc.SpecPaths) == 0 {
				prob(file, i+1, "%s is automated but has no spec link", id)
			}
			if reason != "" {
				cat, detail, _ := strings.Cut(reason, ":")
				tc.ReasonCategory = clipRunes(cat, 30)
				tc.ReasonDetail = clipRunes(strings.TrimSpace(detail), 1000)
				if !contains(ReasonCategories, cat) {
					prob(file, i+1, `%s reason must be "<category>:<detail>", category one of %s`, id, strings.Join(ReasonCategories, "|"))
				}
			} else if contains(NeedsReason, status) {
				prob(file, i+1, `%s reason must be "<category>:<detail>", category one of %s`, id, strings.Join(ReasonCategories, "|"))
			}
			if tc.Automated && reason != "" {
				prob(file, i+1, "%s is automated, reason must be empty", id)
			}
			cases[id] = tc
		}
	}

	// Story → feature links: unknown features are dropped with a Problem.
	for id, s := range stories {
		kept := s.FeatureCodes[:0:0]
		for _, f := range s.FeatureCodes {
			if _, ok := features[f]; ok {
				kept = append(kept, f)
			} else {
				prob(s.SourceFile, s.SourceLine, "%s names unknown feature %s", id, f)
			}
		}
		s.FeatureCodes = kept
		stories[id] = s
	}

	if len(features) == 0 && len(cases) == 0 {
		return nil, problems, ErrEmptyCatalog
	}
	fit := func(code, field string, v *string, n int) {
		if utf8.RuneCountInString(*v) > n {
			*v = clipRunes(*v, n)
			prob("", 0, "%s %s truncated to %d", code, field, n)
		}
	}
	fitText := func(code, field string, v *string) {
		if len(*v) > maxText {
			*v = clipBytes(*v, maxText)
			prob("", 0, "%s %s truncated to %d bytes", code, field, maxText)
		}
	}
	for k, x := range parts {
		fit(k, "name", &x.Name, 300)
		parts[k] = x
	}
	for k, x := range modules {
		fit(k, "name", &x.Name, 300)
		modules[k] = x
	}
	for k, x := range submods {
		fit(k, "name", &x.Name, 300)
		submods[k] = x
	}
	for k, x := range features {
		fitText(k, "name", &x.Name)
		fitText(k, "notes", &x.Notes)
		fit(k, "repo_entry", &x.RepoEntry, 300)
		fit(k, "evidence", &x.Evidence, 500)
		fit(k, "status_as_of", &x.StatusAsOf, 20)
		features[k] = x
	}
	for k, x := range epics {
		fit(k, "title", &x.Title, 500)
		fit(k, "service", &x.Service, 100)
		fit(k, "area", &x.Area, 200)
		epics[k] = x
	}
	for k, x := range stories {
		fitText(k, "text", &x.Text)
		fit(k, "unmapped_reason", &x.UnmappedReason, 500)
		stories[k] = x
	}
	for k, x := range cases {
		fitText(k, "scenario", &x.Scenario)
		cases[k] = x
	}

	snap := &Snapshot{}
	for _, k := range keys(parts) {
		snap.Parts = append(snap.Parts, parts[k])
	}
	for _, k := range keys(modules) {
		snap.Modules = append(snap.Modules, modules[k])
	}
	for _, k := range keys(submods) {
		snap.Submodules = append(snap.Submodules, submods[k])
	}
	for _, k := range keys(features) {
		snap.Features = append(snap.Features, features[k])
	}
	for _, k := range keys(epics) {
		snap.Epics = append(snap.Epics, epics[k])
	}
	for _, k := range keys(stories) {
		snap.Stories = append(snap.Stories, stories[k])
	}
	for _, k := range keys(cases) {
		snap.TestCases = append(snap.TestCases, cases[k])
	}
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		return problems[i].Line < problems[j].Line
	})
	return snap, problems, nil
}

func hasStory(m map[string]Story, id string) bool { _, ok := m[id]; return ok }

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
