package caseresults

import (
	"strings"
	"testing"
	"time"
)

func TestParseStatsAndCaseDurations(t *testing.T) {
	const rep = `{"stats":{"startTime":"2026-11-14T10:00:00.250Z","duration":1234.6,"expected":1,"unexpected":1,"flaky":1,"skipped":0},
	"suites":[{"specs":[
	 {"title":"TC-AUTH-001 a","tests":[{"status":"expected","results":[{"duration":100.4}]},{"status":"unexpected","results":[{"duration":50},{"duration":60}]}]},
	 {"title":"TC-AUTH-002 b","tests":[{"status":"flaky","results":[{"duration":10},{"duration":20.6}]}]}]}]}`
	r, err := Parse(strings.NewReader(rep), time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if r.Stats.DurationMs == nil || *r.Stats.DurationMs != 1235 {
		t.Fatalf("duration %v", r.Stats.DurationMs)
	}
	if r.Stats.Expected != 1 || r.Stats.Unexpected != 1 || r.Stats.Flaky != 1 || r.Stats.Skipped != 0 {
		t.Errorf("stats %+v", r.Stats)
	}
	if got := r.Cases["TC-AUTH-001"]; got != (CaseStat{DurationMs: 210, Tests: 2, Attempts: 3}) {
		t.Errorf("TC-AUTH-001 %+v", got)
	}
	if got := r.Cases["TC-AUTH-002"]; got != (CaseStat{DurationMs: 31, Tests: 1, Attempts: 2}) {
		t.Errorf("TC-AUTH-002 %+v", got)
	}
	if r.Results["TC-AUTH-001"] != Fail || r.Results["TC-AUTH-002"] != Pass || r.Tests != 3 {
		t.Errorf("results unchanged: %v tests=%d", r.Results, r.Tests)
	}
	if r.StartedAt.Nanosecond() != 250_000_000 {
		t.Errorf("milliseconds of the start lost: %v", r.StartedAt)
	}
}

func TestParseStatsFromTestsWithoutDuration(t *testing.T) {
	const rep = `{"stats":{"startTime":"2026-11-14T10:00:00Z"},"suites":[{"specs":[
	 {"title":"TC-AUTH-001 a","tests":[{"status":"expected"},{"status":"skipped"},{"status":"weird"}]},
	 {"title":"no code","tests":[{"status":"unexpected","results":[{"duration":5}]}]}]}]}`
	r, err := Parse(strings.NewReader(rep), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Stats.DurationMs != nil {
		t.Errorf("no stats.duration must give nil, got %d", *r.Stats.DurationMs)
	}
	if r.Stats.Expected != 1 || r.Stats.Skipped != 1 || r.Stats.Unexpected != 1 || r.Stats.Flaky != 0 {
		t.Errorf("counted %+v", r.Stats)
	}
	if got := r.Cases["TC-AUTH-001"]; got != (CaseStat{Tests: 2}) {
		t.Errorf("case %+v", got)
	}
	if len(r.Cases) != 1 {
		t.Errorf("cases %v", r.Cases)
	}
}
