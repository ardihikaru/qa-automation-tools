package caseresults

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const report = `{"stats":{"startTime":"2026-11-14T10:00:00.000Z"},"suites":[
 {"title":"a.spec.ts","specs":[
   {"title":"TC-AUTH-001 login works","tests":[{"projectName":"api","status":"expected"}]},
   {"title":"TC-AUTH-002 bad password","tests":[{"projectName":"api","status":"unexpected"}]},
   {"title":"no code here","tests":[{"status":"expected"}]}],
  "suites":[{"title":"nested","specs":[
   {"title":"TC-AUTH-003 and TC-AUTH-004 share a test","tests":[{"status":"flaky"}]},
   {"title":"TC-AUTH-005 skipped","tests":[{"status":"skipped"}]},
   {"title":"TC-AUTH-001 login works on web","tests":[{"status":"unexpected"}]}]}]}]}`

func TestParse(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	rep, err := Parse(strings.NewReader(report), now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"TC-AUTH-001": Fail, "TC-AUTH-002": Fail, "TC-AUTH-003": Pass, "TC-AUTH-004": Pass, "TC-AUTH-005": Skipped}
	if len(rep.Results) != len(want) {
		t.Fatalf("got %v", rep.Results)
	}
	for c, w := range want {
		if rep.Results[c] != w {
			t.Errorf("%s: got %q want %q", c, rep.Results[c], w)
		}
	}
	if rep.Tests != 6 || !rep.StartedAt.Equal(time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("tests=%d start=%v", rep.Tests, rep.StartedAt)
	}
}

func TestParseFutureStartIsCapped(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rep, err := Parse(strings.NewReader(`{"stats":{"startTime":"2030-01-01T00:00:00Z"},"suites":[{}]}`), now)
	if err != nil || !rep.StartedAt.Equal(now) {
		t.Fatalf("%v %v", rep, err)
	}
}

func TestParseRejects(t *testing.T) {
	if _, err := Parse(strings.NewReader(`{"suites":[]}`), time.Now()); !errors.Is(err, ErrNotReport) {
		t.Fatalf("empty suites: %v", err)
	}
	if _, err := Parse(strings.NewReader(`nope`), time.Now()); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestMerge(t *testing.T) {
	for _, c := range [][3]string{{"", Pass, Pass}, {Pass, Fail, Fail}, {Fail, Pass, Fail}, {Skipped, Pass, Pass}, {Pass, Skipped, Pass}, {Skipped, Skipped, Skipped}} {
		if got := merge(c[0], c[1]); got != c[2] {
			t.Errorf("merge(%q,%q)=%q want %q", c[0], c[1], got, c[2])
		}
	}
}
