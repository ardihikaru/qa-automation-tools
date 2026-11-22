package gaossync

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const goodToken = "gaos_0123456789abcdef0123456789abcdef01"

func TestValidateConfig(t *testing.T) {
	ok := []struct{ url, want string }{
		{"https://gaos.mfardiansyah.id/api/v1", "https://gaos.mfardiansyah.id/api/v1"},
		{"https://gaos.mfardiansyah.id/api/v1/", "https://gaos.mfardiansyah.id/api/v1"},
		{"http://127.0.0.1:8080/api/v1", "http://127.0.0.1:8080/api/v1"},
		{"http://[::1]:8080/api/v1", "http://[::1]:8080/api/v1"},
		{"http://10.8.0.9:8142/api/v1", "http://10.8.0.9:8142/api/v1"},
	}
	for _, c := range ok {
		got, err := ValidateConfig(c.url, goodToken)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v", c.url, got, err)
		}
	}
	bad := []string{
		"http://gaos.mfardiansyah.id/api/v1", "http://localhost:8080/api/v1", "http://100.64.1.4/api/v1", "http://8.8.8.8/api/v1",
		"ftp://gaos.mfardiansyah.id", "https://user:pw@gaos.mfardiansyah.id/api/v1", "https://gaos.mfardiansyah.id/api/v1?x=1",
		"https://gaos.mfardiansyah.id/api/v1?", "https://gaos.mfardiansyah.id/api/v1#f", "https:///api", "not a url", "gaos.mfardiansyah.id/api/v1",
	}
	for _, u := range bad {
		if _, err := ValidateConfig(u, goodToken); err == nil {
			t.Errorf("%q must be refused", u)
		}
	}
	for _, tok := range []string{"gaos_short", "xaos_0123456789abcdef0123456789abcdef01", goodToken + " x", goodToken + "\n", "gaos_0123456789abcdef0123456789abcdé01"} {
		_, err := ValidateConfig("https://gaos.mfardiansyah.id/api/v1", tok)
		if err == nil {
			t.Errorf("token %q must be refused", tok)
		} else if strings.Contains(err.Error(), tok) {
			t.Errorf("the error leaks the token: %v", err)
		}
	}
	if b, err := ValidateConfig("", ""); b != "" || err != nil {
		t.Errorf("both empty is not configured, not an error: %q %v", b, err)
	}
	if _, err := ValidateConfig("https://gaos.mfardiansyah.id/api/v1", ""); err == nil {
		t.Error("url without token must be an error")
	}
	if _, err := ValidateConfig("", goodToken); err == nil {
		t.Error("token without url must be an error")
	}
}

func TestValidProductID(t *testing.T) {
	for _, s := range []string{"p-6", "p-12", "p-0"} {
		if !ValidProductID(s) {
			t.Errorf("%q must be valid", s)
		}
	}
	for _, s := range []string{"", "p-", "P-6", "p-6x", "p6", "p-6/../x", " p-6", "p-" + strings.Repeat("9", 40)} {
		if ValidProductID(s) {
			t.Errorf("%q must be invalid", s)
		}
	}
}

func TestClipBytesKeepsRunesWhole(t *testing.T) {
	if got := clipBytes("short", 600); got != "short" {
		t.Fatal(got)
	}
	long := strings.Repeat("é", 400) // 800 bytes
	got := clipBytes(long, 600)
	if len(got) > 600 || !utf8.ValidString(got) || !strings.HasSuffix(got, "...") {
		t.Fatalf("len %d valid %v", len(got), utf8.ValidString(got))
	}
	if got := clipBytes(strings.Repeat("a", 601), 600); len(got) != 600 || !strings.HasSuffix(got, "...") {
		t.Fatalf("ascii: len %d", len(got))
	}
}

func TestUsableSpec(t *testing.T) {
	cases := map[string]string{"tests/a.spec.ts": "tests/a.spec.ts", "/abs/a.ts": "", "../a.ts": "", "a/../b.ts": "", `a\b.ts`: "", "https://x/a.ts": "", "": "", strings.Repeat("a", 241): ""}
	for in, want := range cases {
		if got := usableSpec([]string{in}); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	if usableSpec(nil) != "" {
		t.Error("no spec")
	}
	if got := usableSpec([]string{"first.ts", "second.ts"}); got != "first.ts" {
		t.Errorf("first of spec_paths: %q", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	for in, want := range map[string]string{"": "1s", "x": "1s", "0": "1s", "-3": "1s", "2": "2s", "10": "10s", "99": "10s", "Wed, 21 Oct 2026 07:28:00 GMT": "1s"} {
		if got := parseRetryAfter(in).String(); got != want {
			t.Errorf("%q: got %s want %s", in, got, want)
		}
	}
}

func catCase(code, epic, story, status string) catCaseT {
	return catCaseT{code: code, epic: epic, story: story, scenario: "Given G When W Then T", polarity: "+", level: "API", priority: "P1", status: status}
}

func TestBuildPlanSkipRulesAndMapping(t *testing.T) {
	cat := &catalog{
		features: map[string][]storyFeature{"US-AUTH-01": {{"A01.07", "A01"}, {"A01.02", "A01"}, {"Z99.01", "Z99"}, {"B02.001", "B02"}}},
		epics:    map[string]catEpic{"AUTH": {"Authentication", "sut-auth"}},
	}
	auto := catCase("TC-AUTH-002", "AUTH", "US-AUTH-01", "automated")
	auto.specPaths = []string{"tests/auth/login.spec.ts"}
	autoNoSpec := catCase("TC-AUTH-005", "AUTH", "", "automated")
	badPol := catCase("TC-AUTH-008", "AUTH", "", "planned")
	badPol.polarity = "x"
	badLvl := catCase("TC-AUTH-009", "AUTH", "", "planned")
	badLvl.level = "UI"
	badPri := catCase("TC-AUTH-010", "AUTH", "", "planned")
	badPri.priority = "P9"
	cat.cases = []catCaseT{
		catCase("TC-AUTH-001", "AUTH", "US-AUTH-01", "planned"),
		auto,
		catCase("TC-AUTH-003", "AUTH", "", "blocked"),
		catCase("TC-AUTH-004", "AUTH", "", "manual-only"),
		autoNoSpec,
		catCase("TC-AUTH-006", "AUTH", "", "wont"),
		catCase("L-001", "AUTH", "", "planned"),                  // local draft
		catCase("TC-LOGIN-001", "AUTH", "", "planned"),           // middle segment != epic
		catCase("TC-AUTH-1234", "AUTH", "", "planned"),           // bad number
		catCase("TC-AUTH-007", "AUTHENTICATIONX", "", "planned"), // epic code too long
		badPol, badLvl, badPri,
		catCase("TC-NEW-001", "NEW", "US-NEW-9", "planned"), // no kb_epics row, bad story code
	}
	p := buildPlan(cat)
	if len(p.cases) != 4 || p.skipped != 10 {
		t.Fatalf("sent %d skipped %d", len(p.cases), p.skipped)
	}
	c1 := p.cases[0]
	if c1.CaseID != "TC-AUTH-001" || c1.Epic != "AUTH" || c1.Module != "B02" || c1.UserStory != "US-AUTH-01" ||
		strings.Join(c1.Features, ",") != "A01.02,A01.07,B02.001" || c1.Decision != "planned" || c1.Reason != "" ||
		c1.Implemented || c1.Spec != "" || c1.Polarity != "+" || c1.Level != "API" || c1.Pri != "P1" {
		t.Fatalf("mapping: %+v", c1)
	}
	if c2 := p.cases[1]; !c2.Implemented || c2.Spec != "tests/auth/login.spec.ts" {
		t.Fatalf("automated: %+v", c2)
	}
	if c3 := p.cases[2]; c3.CaseID != "TC-AUTH-005" || c3.Implemented || c3.Spec != "" || c3.Features == nil || c3.UserStory != "" || c3.Module != "" {
		t.Fatalf("automated without spec: %+v", c3)
	}
	if p.noSpec != 1 {
		t.Fatalf("noSpec %d", p.noSpec)
	}
	if c4 := p.cases[3]; c4.UserStory != "" {
		t.Fatalf("a story code outside the pattern is dropped: %+v", c4)
	}
	if e := p.epics["AUTH"]; e.Title != "Authentication" || e.Service != "sut-auth" || e.Source != "qa-automation-tools" || e.Module != "B02" {
		t.Fatalf("epic: %+v", e)
	}
	if e := p.epics["NEW"]; e.Title != "NEW" || e.Service != "" || e.Module != "" {
		t.Fatalf("missing epic row falls back to the code: %+v", e)
	}
}

func TestBuildPlanFeatureCapAndEpicModuleMode(t *testing.T) {
	var feats []storyFeature
	for i := 1; i <= 15; i++ {
		feats = append(feats, storyFeature{code: "A01." + two(i), module: "A01"})
	}
	cat := &catalog{
		features: map[string][]storyFeature{"US-X-01": feats, "US-X-02": {{"B02.01", "B02"}}, "US-X-03": {{"B02.02", "B02"}}},
		epics:    map[string]catEpic{},
	}
	cat.cases = []catCaseT{
		catCase("TC-XX-001", "XX", "US-X-01", "planned"),
		catCase("TC-XX-002", "XX", "US-X-02", "planned"),
		catCase("TC-XX-003", "XX", "US-X-03", "planned"),
	}
	p := buildPlan(cat)
	if len(p.cases[0].Features) != 12 {
		t.Fatalf("features capped at 12, got %d", len(p.cases[0].Features))
	}
	if p.epics["XX"].Module != "B02" {
		t.Fatalf("most frequent module wins: %q", p.epics["XX"].Module)
	}
	// tie: the smallest code
	cat.cases = cat.cases[:2]
	if p := buildPlan(cat); p.epics["XX"].Module != "A01" {
		t.Fatalf("tie goes to the smallest code: %q", p.epics["XX"].Module)
	}
}

func two(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

func TestScenarioClipInPlan(t *testing.T) {
	c := catCase("TC-AUTH-001", "AUTH", "", "planned")
	c.scenario = strings.Repeat("あ", 300) // 900 bytes
	p := buildPlan(&catalog{cases: []catCaseT{c}, features: map[string][]storyFeature{}, epics: map[string]catEpic{}})
	if got := p.cases[0].Scenario; len(got) > 600 || !utf8.ValidString(got) || !strings.HasSuffix(got, "...") {
		t.Fatalf("len %d", len(got))
	}
}
