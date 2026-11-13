package knowledge

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestSUTGolden parses a verbatim copy of automation-test-sut docs/ at 521618a. The expected
// numbers are the output of `node scripts/test-catalog.mjs --check` at that commit ("Catalog OK").
func TestSUTGolden(t *testing.T) {
	f, _ := Lookup(TestCatalogFormatName)
	snap, problems, err := f.Parse(loadFixture(t, "testdata/sut-521618a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("want 0 problems (Catalog OK), got %d: %+v", len(problems), problems[:min(5, len(problems))])
	}
	automated, unmapped, withStories := 0, 0, map[string]bool{}
	for _, c := range snap.TestCases {
		if c.Automated {
			automated++
		}
	}
	for _, s := range snap.Stories {
		if s.UnmappedReason != "" {
			unmapped++
		}
		for _, fc := range s.FeatureCodes {
			withStories[fc] = true
		}
	}
	checks := []struct {
		name      string
		got, want int
	}{
		{"parts", len(snap.Parts), 4},
		{"modules", len(snap.Modules), 39},
		{"features", len(snap.Features), 444},
		{"epics", len(snap.Epics), 46},
		{"stories", len(snap.Stories), 246},
		{"unmapped", unmapped, 9},
		{"test cases", len(snap.TestCases), 1659},
		{"automated", automated, 456},
		{"features with stories", len(withStories), 70},
		{"submodules", len(snap.Submodules), goldenSubmodules},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
	for _, s := range snap.Submodules {
		if len(s.Code) > 44 || strings.Contains(s.Code, "/") || !CodeRe.MatchString(s.Code) {
			t.Errorf("bad submodule code %q", s.Code)
		}
	}
	for _, e := range snap.Epics {
		if !CodeRe.MatchString(e.Code) {
			t.Errorf("bad epic code %q", e.Code)
		}
	}
}

// goldenSubmodules is the distinct (module, lower(name)) count recorded when the fixture was copied.
const goldenSubmodules = 200

func parse(t *testing.T, files map[string]string) (*Snapshot, []Problem, error) {
	t.Helper()
	m := map[string][]byte{}
	for k, v := range files {
		m[k] = []byte(v)
	}
	return testCatalog{}.Parse(m)
}

const partA = "# Part A — Core\r\n\n### A01 — Registrasi\n\n| ID | Sub | Feature | Repo | Status | As of | Ev | Notes | E | S | T | A |\n| --- |\n" +
	"| <a id=\"f-a01-01\"></a>A01.01 | Master Pasien | Search | his | 🟢 `PROD` | 2026-11-12 | GIT:main | | x | 1 | 2 | 1 |\r\n" +
	"| A01.02 | master pasien | Edit | his | `WEIRD` | — | | n | | | | |\n" +
	"| A02.01 | | Wrong heading | his | `UNKNOWN` | — | | | | | | |\n" +
	"| A01.03 | too | few |\n"

const epicOut = "# Outpatient\n\n## User stories\n" +
	"- **US-OUT-01** ([A01.01](../m.md#f-a01-01), A01.02, A01.01) — As a clerk I search\n" +
	"- **US-OUT-02** (unmapped: ops only) — As ops\n" +
	"- **US-OUT-03** — no features\n" +
	"- **US-OUT-04** (A09.99) — unknown feature\n" +
	"## Test cases\n" +
	"| ☑ | TC-OUT-001 | US-OUT-01 | Given x, when y, then z | + | API | P0 | automated | `tests/a.spec.ts` | |\n" +
	"| ☐ | TC-OUT-002 | US-OUT-09 | G | - | GQL | P1 | blocked | | defect:DEF-12 |\n" +
	"| ☐ | TC-OUT-003 | US-OUT-02 | G | ? | XXX | P9 | wont | | |\n" +
	"| ☐ | TC-OUT-004 | US-OUT-01 | G | + | API | P0 | bogus | | |\n" +
	"| ☐ | TC-OUT-005 | US-OUT-01 | G | + | API |\n" +
	"| ☑ | TC-OUT-001 | US-OUT-01 | dup wins | + | API | P0 | planned | | |\n"

func TestSUTRules(t *testing.T) {
	snap, problems, err := parse(t, map[string]string{
		"docs/modules/A-core.md":                     partA,
		"docs/modules/nested/A-x.md":                 "### A05 — ignored\n",
		"docs/test-catalog/sut-backend/out.md":       epicOut,
		"docs/test-catalog/sut-backend/_template.md": "- **US-TPL-01** (unmapped: x) — tpl\n",
		"docs/test-catalog/README.md":                "- **US-RD-01** (unmapped: x) — readme\n",
		"docs/test-catalog/loose.md":                 "# Loose\n",
		"docs/test-catalog/svc/area/deep.md":         "# Deep\n- **US-DEEP-01** (A01.01) — deep\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs := []string{}
	for _, p := range problems {
		msgs = append(msgs, p.Msg)
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{
		`A01.02 unknown release status`,
		`A02.01 is not under its module heading`,
		`feature row needs 12 columns, found 3`,
		`US-OUT-03 names no module feature`,
		`US-OUT-04 names unknown feature A09.99`,
		`TC-OUT-002 story US-OUT-09 is not listed`,
		`TC-OUT-003 polarity must be + or -`,
		`TC-OUT-003 unknown level "XXX"`,
		`TC-OUT-003 unknown priority "P9"`,
		`TC-OUT-003 reason must be`,
		`TC-OUT-004 unknown status "bogus"`,
		`expected 10 columns, found 6`,
		`duplicate id TC-OUT-001`,
		`TC-OUT-001 checkbox must be ☑ exactly when status is automated`,
		`epic files belong in a service folder`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing problem %q in:\n%s", want, all)
		}
	}
	if len(snap.Parts) != 1 || snap.Parts[0].Name != "Part A — Core" {
		t.Errorf("parts = %+v", snap.Parts)
	}
	if len(snap.Modules) != 1 {
		t.Errorf("nested module file must be ignored: %+v", snap.Modules)
	}
	if len(snap.Features) != 3 {
		t.Fatalf("features = %+v", snap.Features)
	}
	f := map[string]Feature{}
	for _, x := range snap.Features {
		f[x.Code] = x
	}
	if f["A01.02"].ReleaseStatus != "UNKNOWN" || f["A01.02"].StatusAsOf != "" || f["A01.01"].StatusAsOf != "2026-11-12" {
		t.Errorf("release fields: %+v %+v", f["A01.01"], f["A01.02"])
	}
	if f["A02.01"].ModuleCode != "A02" || f["A02.01"].SubmoduleCode != "" {
		t.Errorf("A02.01 = %+v", f["A02.01"])
	}
	if f["A01.01"].SubmoduleCode != f["A01.02"].SubmoduleCode || len(snap.Submodules) != 1 || snap.Submodules[0].Name != "Master Pasien" {
		t.Errorf("submodules = %+v", snap.Submodules)
	}
	if f["A01.01"].Notes != "" || f["A01.02"].Notes != "n" {
		t.Errorf("notes")
	}
	st := map[string]Story{}
	for _, s := range snap.Stories {
		st[s.Code] = s
	}
	if _, ok := st["US-TPL-01"]; ok {
		t.Error("_template must be skipped")
	}
	if _, ok := st["US-RD-01"]; ok {
		t.Error("catalog README must be skipped")
	}
	if got := st["US-OUT-01"].FeatureCodes; len(got) != 2 || got[0] != "A01.01" || got[1] != "A01.02" {
		t.Errorf("US-OUT-01 features = %v", got)
	}
	if st["US-OUT-02"].UnmappedReason != "ops only" || len(st["US-OUT-04"].FeatureCodes) != 0 {
		t.Errorf("stories = %+v", st)
	}
	ep := map[string]Epic{}
	for _, e := range snap.Epics {
		ep[e.Code] = e
	}
	if ep["OUT"].Service != "sut-backend" || ep["DEEP"].Area != "area" || ep["DEEP"].Service != "svc" || ep["LOOSE"].Title != "Loose" {
		t.Errorf("epics = %+v", snap.Epics)
	}
	tc := map[string]TestCase{}
	for _, c := range snap.TestCases {
		tc[c.Code] = c
	}
	if _, ok := tc["TC-OUT-004"]; ok {
		t.Error("unknown status rows are skipped")
	}
	if tc["TC-OUT-001"].Scenario != "dup wins" || tc["TC-OUT-001"].Automated {
		t.Errorf("last wins: %+v", tc["TC-OUT-001"])
	}
	if c := tc["TC-OUT-002"]; c.StoryCode != "" || c.ReasonCategory != "defect" || c.ReasonDetail != "DEF-12" || c.EpicCode != "OUT" {
		t.Errorf("TC-OUT-002 = %+v", c)
	}
	if len(snap.TestCases) != 3 {
		t.Errorf("cases = %d", len(snap.TestCases))
	}
}

func TestSUTEmpty(t *testing.T) {
	if _, _, err := parse(t, map[string]string{"docs/README.md": "# x"}); err != ErrEmptyCatalog {
		t.Fatalf("err = %v", err)
	}
}

func TestSubmoduleCode(t *testing.T) {
	long := strings.Repeat("Sangat Panjang ", 10)
	c := submoduleCode("A01", long)
	if len(c) > 44 || !CodeRe.MatchString(c) {
		t.Errorf("code %q (%d)", c, len(c))
	}
	if submoduleCode("A01", "Master  Pasien") == submoduleCode("A01", "Master Pasien") {
		t.Error("different names must hash differently")
	}
	if clipRunes("héllo", 2) != "hé" {
		t.Error("clipRunes")
	}
}
