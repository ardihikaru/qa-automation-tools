package knowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Draft write-back, pure part (KNOWLEDGE_TRANSLATION.md, "Write-back design" steps 4 to 6): assign
// repo codes, render drafts into an epic file in English, and prove with the real parser that the
// result adds no Problem and holds exactly the drafts. No git, no DB, no clock.

// unmappedDraftReason is written for a draft story that names no feature.
const unmappedDraftReason = "added in the QA tool, not yet mapped to a feature"

// RenderStory is a draft story to append to its epic file.
type RenderStory struct {
	LocalCode    string
	Text         string
	FeatureCodes []string
}

// RenderCase is a draft test case. StoryRef is a repo story code, or the LocalCode of a RenderStory
// of the same request.
type RenderCase struct {
	LocalCode      string
	StoryRef       string
	Scenario       string
	Polarity       string
	Level          string
	Priority       string
	Status         string
	SpecPaths      []string
	ReasonCategory string
	ReasonDetail   string
}

// RenderRequest is what RenderDrafts publishes into one epic.
type RenderRequest struct {
	EpicCode string
	Stories  []RenderStory
	Cases    []RenderCase
	// Taken lists repo codes used elsewhere (every kb_* row including removed ones, and the codes of
	// non-failed publications) that the allocation must skip.
	Taken []string
}

// CodePair maps a draft code to the repo code it was given. Type is "story" or "test_case".
type CodePair struct {
	Type  string `json:"type"`
	Local string `json:"local"`
	Repo  string `json:"repo"`
}

// Rendered is the result: the epic file's new content and the assigned codes (stories first, in
// draft order, then cases).
type Rendered struct {
	File    string
	Content []byte
	Pairs   []CodePair
}

// GateError is a refusal of the round-trip gate or of the renderer: the files must not be committed.
type GateError struct{ Msg string }

func (e *GateError) Error() string { return e.Msg }

func gatef(f string, a ...any) error { return &GateError{Msg: fmt.Sprintf(f, a...)} }

var (
	epicCodeRe      = regexp.MustCompile(`^[A-Z]{1,40}$`)
	userStoriesHead = regexp.MustCompile(`^##\s+User stories\s*$`)
	caseTableHead   = regexp.MustCompile(`^\|.*\bTC ID\b`)
	tableSepRe      = regexp.MustCompile(`^\|[\s:|-]+\|\s*$`)
)

// nextNumbers returns, for prefix (e.g. "TC-APPT-"), the highest number among codes and the raw
// text that use it.
func maxNumber(prefix string, codes []string, text string) int {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(prefix) + `(\d+)\b`)
	max := 0
	note := func(s string) {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n > max {
				max = n
			}
		}
	}
	for _, c := range codes {
		note(c)
	}
	note(text)
	return max
}

// RenderDrafts appends the drafts of one epic to its file in files (the worktree's docs tree: path
// to bytes, as the sync reads it), after allocating the next free codes. It then parses the result
// with the real parser and refuses unless the new files add no Problem, change no existing entity
// and hold exactly the new stories and cases, field by field.
func RenderDrafts(files map[string][]byte, req RenderRequest) (*Rendered, error) {
	if len(req.Stories) == 0 && len(req.Cases) == 0 {
		return nil, gatef("nothing to publish")
	}
	if !epicCodeRe.MatchString(req.EpicCode) {
		return nil, gatef("epic code %q cannot be used in repo codes (TC-<E>-NNN needs capital letters only)", req.EpicCode)
	}
	cat := testCatalog{}
	before, probsBefore, err := cat.Parse(files)
	if err != nil {
		return nil, gatef("the catalog does not parse: %v", err)
	}
	var epic *Epic
	for i := range before.Epics {
		if before.Epics[i].Code == req.EpicCode {
			epic = &before.Epics[i]
		}
	}
	if epic == nil {
		return nil, gatef("epic %s has no file in the catalog", req.EpicCode)
	}
	file := epic.SourceFile
	raw := files[file]
	text := string(raw)

	// ---- allocation (spec step 4)
	var known []string
	for _, s := range before.Stories {
		known = append(known, s.Code)
	}
	for _, c := range before.TestCases {
		known = append(known, c.Code)
	}
	known = append(known, req.Taken...)
	nextStory := maxNumber("US-"+req.EpicCode+"-", known, text) + 1
	nextCase := maxNumber("TC-"+req.EpicCode+"-", known, text) + 1
	if nextStory+len(req.Stories)-1 > 99 {
		return nil, gatef("epic %s has no free story numbers (US-%s-NN stops at 99)", req.EpicCode, req.EpicCode)
	}
	if nextCase+len(req.Cases)-1 > 999 {
		return nil, gatef("epic %s has no free test case numbers (TC-%s-NNN stops at 999)", req.EpicCode, req.EpicCode)
	}

	features := map[string]Feature{}
	for _, f := range before.Features {
		features[f.Code] = f
	}
	repoStories := map[string]bool{}
	for _, s := range before.Stories {
		repoStories[s.Code] = true
	}

	var pairs []CodePair
	newStory := map[string]string{} // local -> repo
	var storyLines, caseLines []string
	wantStories := map[string]Story{}
	wantCases := map[string]TestCase{}

	for _, s := range req.Stories {
		code := fmt.Sprintf("US-%s-%02d", req.EpicCode, nextStory)
		nextStory++
		newStory[s.LocalCode] = code
		pairs = append(pairs, CodePair{Type: TypeStory, Local: s.LocalCode, Repo: code})
		enc, err := EncodeStoryText(s.Text)
		if err != nil {
			return nil, gatef("story %s, field text: %v", s.LocalCode, err)
		}
		if enc == "" {
			return nil, gatef("story %s, field text: empty", s.LocalCode)
		}
		var mid string
		ws := Story{Code: code, EpicCode: req.EpicCode, Text: canonDraftText(s.Text), FeatureCodes: []string{}, SourceFile: file}
		if len(s.FeatureCodes) == 0 {
			mid = "(unmapped: " + unmappedDraftReason + ")"
			ws.UnmappedReason = unmappedDraftReason
		} else {
			var links []string
			seen := map[string]bool{}
			for _, fc := range s.FeatureCodes {
				if seen[fc] {
					continue
				}
				seen[fc] = true
				f, ok := features[fc]
				if !ok {
					return nil, gatef("story %s names feature %s, which is not in the catalog", s.LocalCode, fc)
				}
				rel, err := filepath.Rel(filepath.Dir(file), f.SourceFile)
				if err != nil {
					return nil, gatef("story %s: no relative link to %s: %v", s.LocalCode, f.SourceFile, err)
				}
				anchor := "f-" + strings.ToLower(strings.ReplaceAll(fc, ".", "-"))
				links = append(links, fmt.Sprintf("[%s](%s#%s)", fc, filepath.ToSlash(rel), anchor))
				ws.FeatureCodes = append(ws.FeatureCodes, fc)
			}
			mid = "(" + strings.Join(links, ", ") + ")"
		}
		storyLines = append(storyLines, fmt.Sprintf("- **%s** %s — %s", code, mid, enc))
		wantStories[code] = ws
	}

	for _, c := range req.Cases {
		code := fmt.Sprintf("TC-%s-%03d", req.EpicCode, nextCase)
		nextCase++
		pairs = append(pairs, CodePair{Type: TypeTestCase, Local: c.LocalCode, Repo: code})
		story := c.StoryRef
		if repo, ok := newStory[story]; ok {
			story = repo
		} else if !repoStories[story] {
			return nil, gatef("test case %s names story %q, which is neither in the catalog nor published with it", c.LocalCode, c.StoryRef)
		}
		cell := func(field, v string) (string, error) {
			s, err := EncodeCell(v)
			if err != nil {
				return "", fmt.Errorf("test case %s, field %s: %w", c.LocalCode, field, err)
			}
			return s, nil
		}
		scenario, err := cell("scenario", c.Scenario)
		if err != nil {
			return nil, &GateError{Msg: err.Error()}
		}
		if scenario == "" {
			return nil, gatef("test case %s, field scenario: empty", c.LocalCode)
		}
		var specs []string
		for _, p := range c.SpecPaths {
			if strings.ContainsAny(p, "`") || strings.TrimSpace(p) == "" {
				return nil, gatef("test case %s, field spec_paths: %q cannot be written as a code span", c.LocalCode, p)
			}
			specs = append(specs, "`"+strings.TrimSpace(p)+"`")
		}
		spec, err := cell("spec_paths", strings.Join(specs, ", "))
		if err != nil {
			return nil, &GateError{Msg: err.Error()}
		}
		reasonRaw := ""
		switch {
		case c.ReasonCategory != "":
			reasonRaw = c.ReasonCategory + ":" + c.ReasonDetail
		case strings.TrimSpace(c.ReasonDetail) != "":
			return nil, gatef("test case %s: a reason detail needs a reason category", c.LocalCode)
		}
		reason, err := cell("reason", reasonRaw)
		if err != nil {
			return nil, &GateError{Msg: err.Error()}
		}
		box := "☐"
		if c.Status == "automated" {
			box = "☑"
		}
		caseLines = append(caseLines, catalogRow(box, code, story, scenario, c.Polarity, c.Level, c.Priority, c.Status, spec, reason))
		wt := TestCase{Code: code, EpicCode: req.EpicCode, StoryCode: story, Scenario: canonDraftText(c.Scenario), Polarity: c.Polarity, Level: c.Level,
			Priority: c.Priority, Status: c.Status, Automated: c.Status == "automated", SpecPaths: []string{}, SourceFile: file}
		for _, p := range c.SpecPaths {
			wt.SpecPaths = append(wt.SpecPaths, strings.TrimSpace(p))
		}
		if c.ReasonCategory != "" {
			wt.ReasonCategory = clipRunes(c.ReasonCategory, 30)
			wt.ReasonDetail = clipRunes(canonDraftText(c.ReasonDetail), 1000)
		}
		wantCases[code] = wt
	}

	// ---- render (spec step 5)
	content, err := insertLines(text, storyLines, caseLines)
	if err != nil {
		return nil, err
	}
	out := &Rendered{File: file, Content: []byte(content), Pairs: pairs}

	// ---- round-trip gate (spec step 6)
	after := map[string][]byte{}
	for k, v := range files {
		after[k] = v
	}
	after[file] = out.Content
	snapAfter, probsAfter, err := cat.Parse(after)
	if err != nil {
		return nil, gatef("round trip: the new catalog does not parse: %v", err)
	}
	if err := gateCompare(before, snapAfter, probsBefore, probsAfter, wantStories, wantCases); err != nil {
		return nil, err
	}
	return out, nil
}

// catalogRow writes one 10-cell case row; an empty cell is written as "| |", like the existing rows.
func catalogRow(cells ...string) string {
	var b strings.Builder
	b.WriteString("|")
	for _, c := range cells {
		if c == "" {
			b.WriteString(" |")
		} else {
			b.WriteString(" " + c + " |")
		}
	}
	return b.String()
}

func canonDraftText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

type insertion struct {
	after int // index of the line to insert after
	lines []string
}

// insertLines puts the story bullets after the last story bullet (or the "## User stories" heading)
// and the case rows after the last case row (or the table header), keeping the file's line ends.
func insertLines(text string, stories, cases []string) (string, error) {
	crlf := strings.Contains(text, "\r\n")
	lines := strings.Split(text, "\n")
	bare := func(i int) string { return strings.TrimSuffix(lines[i], "\r") }
	var ins []insertion
	if len(stories) > 0 {
		at := -1
		for i := range lines {
			if storyLineRe.MatchString(bare(i)) {
				at = i
			}
		}
		if at >= 0 {
			ins = append(ins, insertion{at, stories})
		} else {
			for i := range lines {
				if userStoriesHead.MatchString(bare(i)) {
					at = i
					break
				}
			}
			if at < 0 {
				return "", gatef("the epic file has no \"## User stories\" heading to add stories to")
			}
			ins = append(ins, insertion{at, append([]string{""}, stories...)})
		}
	}
	if len(cases) > 0 {
		at := -1
		for i := range lines {
			if caseRowRe.MatchString(bare(i)) {
				at = i
			}
		}
		if at < 0 {
			for i := 0; i+1 < len(lines); i++ {
				if caseTableHead.MatchString(bare(i)) && tableSepRe.MatchString(bare(i+1)) {
					at = i + 1
					break
				}
			}
		}
		if at < 0 {
			return "", gatef("the epic file has no test case table to add cases to")
		}
		ins = append(ins, insertion{at, cases})
	}
	sort.SliceStable(ins, func(i, j int) bool { return ins[i].after > ins[j].after })
	for _, in := range ins {
		add := make([]string, len(in.lines))
		for i, l := range in.lines {
			if crlf {
				l += "\r"
			}
			add[i] = l
		}
		tail := append([]string{}, lines[in.after+1:]...)
		lines = append(append(lines[:in.after+1], add...), tail...)
	}
	return strings.Join(lines, "\n"), nil
}

// normalized turns a snapshot into code-keyed JSON without source lines, so entities compare by
// content, not by where they moved to.
func normalized(snap *Snapshot) map[string]string {
	out := map[string]string{}
	put := func(kind string, v any, code string) {
		b, _ := json.Marshal(v)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		delete(m, "source_line")
		b, _ = json.Marshal(m)
		out[kind+"\x00"+code] = string(b)
	}
	for _, x := range snap.Parts {
		put("part", x, x.Code)
	}
	for _, x := range snap.Modules {
		put("module", x, x.Code)
	}
	for _, x := range snap.Submodules {
		put("submodule", x, x.Code)
	}
	for _, x := range snap.Features {
		put("feature", x, x.Code)
	}
	for _, x := range snap.Epics {
		put("epic", x, x.Code)
	}
	for _, x := range snap.Stories {
		put("story", x, x.Code)
	}
	for _, x := range snap.TestCases {
		put("test_case", x, x.Code)
	}
	return out
}

func gateCompare(before, after *Snapshot, pb, pa []Problem, wantStories map[string]Story, wantCases map[string]TestCase) error {
	count := func(ps []Problem) map[string]int {
		m := map[string]int{}
		for _, p := range ps {
			m[p.File+"\x00"+p.Msg]++
		}
		return m
	}
	cb := count(pb)
	ca := count(pa)
	keys := make([]string, 0, len(ca))
	for k := range ca {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if ca[k] > cb[k] {
			i := strings.Index(k, "\x00")
			return gatef("round trip: the new text adds a catalog problem (%s: %s)", k[:i], k[i+1:])
		}
	}
	nb, na := normalized(before), normalized(after)
	for k, v := range nb {
		if na[k] != v {
			return gatef("round trip: existing %s changed", strings.Replace(k, "\x00", " ", 1))
		}
	}
	want := map[string]string{}
	for code, s := range wantStories {
		want["story\x00"+code] = jsonNoLine(s)
	}
	for code, c := range wantCases {
		want["test_case\x00"+code] = jsonNoLine(c)
	}
	added := 0
	for k, v := range na {
		if _, existed := nb[k]; existed {
			continue
		}
		added++
		w, ok := want[k]
		if !ok {
			return gatef("round trip: unexpected new %s", strings.Replace(k, "\x00", " ", 1))
		}
		if w != v {
			return gatef("round trip: %s does not read back as written (wrote %s, parsed %s)", strings.Replace(k, "\x00", " ", 1), w, v)
		}
	}
	if added != len(want) {
		return gatef("round trip: %d of %d new entries were read back", added, len(want))
	}
	return nil
}

func jsonNoLine(v any) string {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	delete(m, "source_line")
	b, _ = json.Marshal(m)
	return string(b)
}

// IsGateError reports whether err is a renderer or gate refusal (the API answers 422).
func IsGateError(err error) bool {
	var g *GateError
	return errors.As(err, &g)
}
