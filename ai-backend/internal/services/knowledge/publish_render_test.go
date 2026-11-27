package knowledge

import (
	"math/rand"
	"strings"
	"testing"
)

func sampleTree(t *testing.T) map[string][]byte {
	t.Helper()
	return readLocalTree(t, "testdata/sample-catalog")
}

func mustRender(t *testing.T, files map[string][]byte, req RenderRequest) *Rendered {
	t.Helper()
	out, err := RenderDrafts(files, req)
	if err != nil {
		t.Fatalf("RenderDrafts: %v", err)
	}
	return out
}

func TestRenderDraftsSample(t *testing.T) {
	files := sampleTree(t)
	req := RenderRequest{
		EpicCode: "APPT",
		Stories: []RenderStory{
			{LocalCode: "L-S1", Text: "As a patient I want to cancel | reschedule\nso that I keep my slot", FeatureCodes: []string{"A02.01", "A01.07", "A02.01"}},
			{LocalCode: "L-S2", Text: "As an admin I want a report", FeatureCodes: nil},
		},
		Cases: []RenderCase{
			{LocalCode: "L-C1", StoryRef: "L-S1", Scenario: "Given a booking, when the patient cancels | reschedules, then it is `updated`\r\nand mailed", Polarity: "+", Level: "API", Priority: "P1", Status: "planned"},
			{LocalCode: "L-C2", StoryRef: "US-APPT-02", Scenario: "Given X, when Y, then Z", Polarity: "-", Level: "WEB", Priority: "P2", Status: "manual-only",
				ReasonCategory: "manual-only", ReasonDetail: "needs two accounts | and a phone"},
			{LocalCode: "L-C3", StoryRef: "L-S2", Scenario: "Given a, when b, then c", Polarity: "+", Level: "API", Priority: "P0", Status: "automated", SpecPaths: []string{"tests/api/x/a.spec.ts", "tests/api/x/b.spec.ts"}},
		},
	}
	out := mustRender(t, files, req)
	if out.File != "docs/test-catalog/clinic-backend/appointments.md" {
		t.Fatalf("file %s", out.File)
	}
	want := map[string]string{"L-S1": "US-APPT-06", "L-S2": "US-APPT-07", "L-C1": "TC-APPT-010", "L-C2": "TC-APPT-011", "L-C3": "TC-APPT-012"}
	for _, p := range out.Pairs {
		if want[p.Local] != p.Repo {
			t.Errorf("%s -> %s, want %s", p.Local, p.Repo, want[p.Local])
		}
	}
	text := string(out.Content)
	for _, frag := range []string{
		"- **US-APPT-06** ([A02.01](../../modules/A-core.md#f-a02-01), [A01.07](../../modules/A-core.md#f-a01-07)) — As a patient I want to cancel | reschedule<br>so that I keep my slot",
		"- **US-APPT-07** (unmapped: " + unmappedDraftReason + ") — As an admin I want a report",
		`| ☐ | TC-APPT-010 | US-APPT-06 | Given a booking, when the patient cancels \| reschedules, then it is ` + "`updated`<br>and mailed" + ` | + | API | P1 | planned | | |`,
		"manual-only:needs two accounts \\| and a phone |",
		"| ☑ | TC-APPT-012 | US-APPT-07 |",
		"`tests/api/x/a.spec.ts`, `tests/api/x/b.spec.ts`",
	} {
		if !strings.Contains(text, frag) {
			t.Errorf("output lacks %q", frag)
		}
	}
	// The existing lines are untouched and the new ones sit after the last bullet and the last row.
	if !strings.HasPrefix(text, "# Appointments and visit check-in (clinic-backend)") {
		t.Error("head changed")
	}
	if i, j := strings.Index(text, "US-APPT-05"), strings.Index(text, "US-APPT-06"); i < 0 || j < i {
		t.Error("story bullets not appended after the last one")
	}
	if i, j := strings.Index(text, "TC-APPT-009"), strings.Index(text, "TC-APPT-010"); i < 0 || j < i {
		t.Error("case rows not appended after the last row")
	}
	// Idempotent input: the same drafts on the output file get the next numbers.
	files[out.File] = out.Content
	again := mustRender(t, files, RenderRequest{EpicCode: "APPT", Stories: []RenderStory{{LocalCode: "L-N", Text: "x", FeatureCodes: []string{"A01.07"}}},
		Cases: []RenderCase{{LocalCode: "L-M", StoryRef: "L-N", Scenario: "s", Polarity: "+", Level: "API", Priority: "P1", Status: "planned"}}})
	if again.Pairs[0].Repo != "US-APPT-08" || again.Pairs[1].Repo != "TC-APPT-013" {
		t.Errorf("second render: %+v", again.Pairs)
	}
}

func TestRenderTakenCodesAreSkipped(t *testing.T) {
	files := sampleTree(t)
	out := mustRender(t, files, RenderRequest{EpicCode: "APPT", Taken: []string{"TC-APPT-015", "US-APPT-09", "TC-OTHER-099"},
		Stories: []RenderStory{{LocalCode: "L-S", Text: "t", FeatureCodes: []string{"A01.07"}}},
		Cases:   []RenderCase{{LocalCode: "L-C", StoryRef: "L-S", Scenario: "s", Polarity: "+", Level: "API", Priority: "P1", Status: "planned"}}})
	if out.Pairs[0].Repo != "US-APPT-10" || out.Pairs[1].Repo != "TC-APPT-016" {
		t.Errorf("pairs %+v", out.Pairs)
	}
}

func TestRenderRefusals(t *testing.T) {
	files := sampleTree(t)
	ok := RenderCase{LocalCode: "L-C", StoryRef: "US-APPT-01", Scenario: "s", Polarity: "+", Level: "API", Priority: "P1", Status: "planned"}
	mod := func(f func(*RenderCase)) RenderRequest {
		c := ok
		f(&c)
		return RenderRequest{EpicCode: "APPT", Cases: []RenderCase{c}}
	}
	for name, req := range map[string]RenderRequest{
		"control char":        mod(func(c *RenderCase) { c.Scenario = "a\x01b" }),
		"literal br":          mod(func(c *RenderCase) { c.Scenario = "a<br>b" }),
		"unknown story":       mod(func(c *RenderCase) { c.StoryRef = "US-APPT-77" }),
		"draft story absent":  mod(func(c *RenderCase) { c.StoryRef = "L-NOPE" }),
		"bad epic":            {EpicCode: "APPT2", Cases: []RenderCase{ok}},
		"missing epic":        {EpicCode: "NOPE", Cases: []RenderCase{ok}},
		"nothing":             {EpicCode: "APPT"},
		"unknown feature":     {EpicCode: "APPT", Stories: []RenderStory{{LocalCode: "L-S", Text: "t", FeatureCodes: []string{"Z99.99"}}}},
		"story control char":  {EpicCode: "APPT", Stories: []RenderStory{{LocalCode: "L-S", Text: "a\x00b", FeatureCodes: []string{"A01.07"}}}},
		"detail w/o category": mod(func(c *RenderCase) { c.ReasonDetail = "why" }),
		"backtick in spec":    mod(func(c *RenderCase) { c.SpecPaths = []string{"a`b"} }),
		// Round-trip gate: the parser reports "automated, reason must be empty".
		"gate: automated with reason": mod(func(c *RenderCase) {
			c.Status, c.SpecPaths, c.ReasonCategory, c.ReasonDetail = "automated", []string{"tests/api/a.spec.ts"}, "manual-only", "x"
		}),
		"gate: bad level": mod(func(c *RenderCase) { c.Level = "NOPE" }),
	} {
		_, err := RenderDrafts(files, req)
		if err == nil {
			t.Errorf("%s: accepted", name)
		} else if !IsGateError(err) {
			t.Errorf("%s: not a GateError: %v", name, err)
		}
	}
	// The control-character refusal names the field.
	_, err := RenderDrafts(files, mod(func(c *RenderCase) { c.Scenario = "a\x01b" }))
	if err == nil || !strings.Contains(err.Error(), "field scenario") {
		t.Errorf("error does not name the field: %v", err)
	}
}

func TestRenderNumberOverflow(t *testing.T) {
	files := sampleTree(t)
	_, err := RenderDrafts(files, RenderRequest{EpicCode: "APPT", Taken: []string{"US-APPT-99"},
		Stories: []RenderStory{{LocalCode: "L-S", Text: "t", FeatureCodes: []string{"A01.07"}}}})
	if err == nil || !strings.Contains(err.Error(), "no free story numbers") {
		t.Errorf("story overflow: %v", err)
	}
}

func TestRenderCRLFAndEmptyEpic(t *testing.T) {
	files := map[string][]byte{
		"docs/modules/A-core.md":     []byte("# Core\n### A01 — Reg\n| <a id=\"f-a01-01\"></a>A01.01 | Pasien | Search | app | `PROD` | 2026-01-01 | | | | | | |\n"),
		"docs/test-catalog/svc/e.md": []byte("# Epic E\r\n\r\n## User stories\r\n\r\n## Test cases\r\n\r\n| Done | TC ID | Story | Scenario | +/- | Level | Pri | Status | Spec | Reason |\r\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\r\n"),
	}
	req := RenderRequest{EpicCode: "E", Stories: []RenderStory{{LocalCode: "L-S", Text: "story", FeatureCodes: []string{"A01.01"}}},
		Cases: []RenderCase{{LocalCode: "L-C", StoryRef: "L-S", Scenario: "scen", Polarity: "+", Level: "API", Priority: "P1", Status: "planned"}}}
	// A file whose first story fixes the epic code, with CRLF line ends.
	files["docs/test-catalog/svc/e.md"] = []byte("# Epic E\r\n\r\n## User stories\r\n\r\n- **US-E-01** ([A01.01](../../modules/A-core.md#f-a01-01)) — first\r\n\r\n## Test cases\r\n\r\n| Done | TC ID | Story | Scenario | +/- | Level | Pri | Status | Spec | Reason |\r\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\r\n")
	out, err := RenderDrafts(files, req)
	if err != nil {
		t.Fatalf("CRLF epic: %v", err)
	}
	text := string(out.Content)
	if strings.Count(text, "\r\n") != strings.Count(text, "\n") {
		t.Errorf("a bare LF was introduced: %q", text)
	}
	if !strings.Contains(text, "TC-E-001") || !strings.Contains(text, "US-E-02") {
		t.Errorf("codes: %q", text)
	}
	// The case table has no row yet: rows go after the separator.
	if i, j := strings.Index(text, "| --- |"), strings.Index(text, "TC-E-001"); j < i {
		t.Error("row before the table separator")
	}
}

func TestRenderRoundTripProperty(t *testing.T) {
	files := sampleTree(t)
	alphabet := []string{"a", "b", " ", "|", `\`, "\n", "\r\n", "`", "é", "—", "(", ")", "[", "]", "<", ">", "-", "☐", ":", "  "}
	rng := rand.New(rand.NewSource(42))
	gen := func() string {
		var b strings.Builder
		b.WriteString("x")
		for n := rng.Intn(14); n > 0; n-- {
			b.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		b.WriteString("y")
		return b.String()
	}
	for i := 0; i < 400; i++ {
		s, c, d := gen(), gen(), gen()
		req := RenderRequest{EpicCode: "APPT",
			Stories: []RenderStory{{LocalCode: "L-S", Text: s, FeatureCodes: []string{"A01.07"}}},
			Cases:   []RenderCase{{LocalCode: "L-C", StoryRef: "L-S", Scenario: c, Polarity: "+", Level: "API", Priority: "P1", Status: "manual-only", ReasonCategory: "manual-only", ReasonDetail: d}}}
		_, err := RenderDrafts(files, req)
		if err != nil && !strings.Contains(err.Error(), "cannot be written") {
			t.Fatalf("story %q scenario %q detail %q: %v", s, c, d, err)
		}
	}
}
