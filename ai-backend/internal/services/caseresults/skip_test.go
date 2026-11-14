package caseresults

import (
	"strings"
	"testing"
	"time"
)

// Real-shaped Playwright JSON: a project-dependency setup (no case code in the title) plus the case.
const (
	setupSkipped = `{"title":"preflight + seed","tests":[{"projectName":"vendor-e2e-setup","status":"skipped","annotations":[{"type":"skip","description":"the staging DB credential (~/.config/x.json) is missing, so the vendor clusters cannot be seeded."}],"results":[{"status":"skipped"}]}]}`
	caseSkipped  = `{"title":"TC-VOCBOOK-007 books","tests":[{"projectName":"api-rest","status":"skipped","annotations":[{"type":"skip","description":"the staging DB credential (~/.config/x.json) is missing, so the fixtures of this cluster cannot be read."},{"type":"skip","description":"the staging DB credential (~/.config/x.json) is missing, so the fixtures of this cluster cannot be read."}],"results":[{"status":"skipped"}]}]}`
	casePassed   = `{"title":"TC-VOCBOOK-007 books","tests":[{"projectName":"api-rest","status":"expected","annotations":[],"results":[{"status":"passed"}]}]}`
	caseFailed   = `{"title":"TC-VOCBOOK-007 books","tests":[{"projectName":"api-rest","status":"unexpected","results":[{"status":"failed"}]}]}`
)

func wrap(specs ...string) string {
	return `{"stats":{"startTime":"2026-11-14T10:00:00.000Z"},"suites":[{"title":"x.spec.ts","specs":[` + strings.Join(specs, ",") + `]}]}`
}

func TestParseSkipReasons(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name, json, want, reason string
		hint                     bool
	}{
		{"pass", wrap(setupSkipped, casePassed), Pass, "", false},
		{"fail", wrap(setupSkipped, caseFailed), Fail, "", false},
		{"all skipped with annotations", wrap(setupSkipped, caseSkipped), Skipped, "the staging DB credential (~/.config/x.json) is missing, so the fixtures of this cluster cannot be read.", true},
		{"skipped without annotation", wrap(`{"title":"TC-VOCBOOK-007 x","tests":[{"status":"skipped"}]}`), Skipped, "", false},
		{"not found", wrap(setupSkipped), "", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			rep, err := Parse(strings.NewReader(c.json), now)
			if err != nil {
				t.Fatal(err)
			}
			if got := rep.Results["TC-VOCBOOK-007"]; got != c.want {
				t.Errorf("result %q want %q", got, c.want)
			}
			if len(rep.Results) > 1 {
				t.Errorf("setup/teardown must not count as a case: %v", rep.Results)
			}
			r := rep.SkipReason("TC-VOCBOOK-007", 400)
			if r != c.reason || HarnessHint(r) != c.hint {
				t.Errorf("reason %q hint %v", r, HarnessHint(r))
			}
		})
	}
	rep, _ := Parse(strings.NewReader(wrap(caseSkipped)), now)
	if got := rep.SkipReason("TC-VOCBOOK-007", 20); len(got) > 24 {
		t.Errorf("not clipped: %q", got)
	}
}
