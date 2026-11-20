// Package caseresults turns a Playwright JSON report into per-case results and stores the latest
// result of every test case (docs/designs/CASE_RESULTS.md).
package caseresults

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"strings"
	"time"
)

// Result values, the same words GAOS accepts.
const (
	Pass    = "pass"
	Fail    = "fail"
	Skipped = "skipped"
)

var caseIDRe = regexp.MustCompile(`TC-[A-Z]+-[0-9]{3}`)

// Report is what a Playwright JSON report says, reduced to what the QA tools need.
type Report struct {
	StartedAt time.Time
	Results   map[string]string // case code -> pass|fail|skipped
	Tests     int               // tests seen (with or without a case id)
	// SkipReasons: code -> skip annotation descriptions of its skipped tests (test.skip(cond, reason),
	// testInfo.skip, test.fixme), in report order, without duplicates.
	SkipReasons map[string][]string
	// Stats is the run as a whole (RUN_HISTORY.md): the report's stats, else counted from the tests.
	Stats Stats
	// Cases: code -> how long its tests took, for the run history. Results above is unchanged by it.
	Cases map[string]CaseStat
}

// Stats is a run's duration and counts. DurationMs is nil when the report has no usable
// stats.duration. The counts are Playwright's: expected = passed, unexpected = failed,
// flaky = passed on retry.
type Stats struct {
	DurationMs *int64
	Expected   int
	Unexpected int
	Flaky      int
	Skipped    int
}

// CaseStat is one case code's share of a run: DurationMs is the sum of results[].duration of
// every test (and retry) titled with the code, Tests the number of tests, Attempts the number of
// results (runs of those tests, retries included).
type CaseStat struct {
	DurationMs int64
	Tests      int
	Attempts   int
}

// pwSuite mirrors the part of Playwright's JSON reporter output we read.
type pwSuite struct {
	Specs []struct {
		Title string `json:"title"`
		Tests []struct {
			Status  string `json:"status"` // expected | unexpected | flaky | skipped
			Results []struct {
				Duration float64 `json:"duration"`
			} `json:"results"`
			Annotations []struct {
				Type        string `json:"type"`
				Description string `json:"description"`
			} `json:"annotations"`
		} `json:"tests"`
	} `json:"specs"`
	Suites []pwSuite `json:"suites"`
}

type pwReport struct {
	Suites []pwSuite `json:"suites"`
	Stats  struct {
		StartTime  string   `json:"startTime"`
		Duration   *float64 `json:"duration"`
		Expected   *int     `json:"expected"`
		Unexpected *int     `json:"unexpected"`
		Flaky      *int     `json:"flaky"`
		Skipped    *int     `json:"skipped"`
	} `json:"stats"`
}

// ErrNotReport means the input is JSON but has no Playwright suites.
var ErrNotReport = errors.New("not a Playwright JSON report (no suites)")

// Parse reads a Playwright JSON report. A spec whose title contains one or more TC-XXX-NNN codes
// gives each code its result. Several tests (projects, browsers) for one code are merged:
// any failure wins, then any pass, else skipped. "flaky" (passed on retry) counts as a pass.
// now is used when the report has no usable start time, and caps a start time in the future.
func Parse(r io.Reader, now time.Time) (*Report, error) {
	var pr pwReport
	if err := json.NewDecoder(r).Decode(&pr); err != nil {
		return nil, err
	}
	if len(pr.Suites) == 0 {
		return nil, ErrNotReport
	}
	rep := &Report{StartedAt: now, Results: map[string]string{}, SkipReasons: map[string][]string{}, Cases: map[string]CaseStat{}}
	var counted Stats // from the tests, used when the report has no stats counts
	if t, err := time.Parse(time.RFC3339, pr.Stats.StartTime); err == nil && !t.After(now) {
		rep.StartedAt = t
	}
	var walk func(s pwSuite)
	walk = func(s pwSuite) {
		for _, sp := range s.Specs {
			codes := caseIDRe.FindAllString(sp.Title, -1)
			for _, t := range sp.Tests {
				rep.Tests++
				res := map[string]string{"expected": Pass, "flaky": Pass, "unexpected": Fail, "skipped": Skipped}[t.Status]
				if res == "" {
					continue // unknown status: ignore the test
				}
				switch t.Status {
				case "expected":
					counted.Expected++
				case "unexpected":
					counted.Unexpected++
				case "flaky":
					counted.Flaky++
				case "skipped":
					counted.Skipped++
				}
				var ms float64
				for _, r := range t.Results {
					if r.Duration > 0 {
						ms += r.Duration
					}
				}
				for _, c := range codes {
					cs := rep.Cases[c]
					cs.Tests++
					cs.Attempts += len(t.Results)
					cs.DurationMs += int64(math.Round(ms))
					rep.Cases[c] = cs
					rep.Results[c] = merge(rep.Results[c], res)
					if res != Skipped {
						continue
					}
					for _, a := range t.Annotations {
						if (a.Type == "skip" || a.Type == "fixme") && a.Description != "" && !contains(rep.SkipReasons[c], a.Description) {
							rep.SkipReasons[c] = append(rep.SkipReasons[c], a.Description)
						}
					}
				}
			}
		}
		for _, c := range s.Suites {
			walk(c)
		}
	}
	for _, s := range pr.Suites {
		walk(s)
	}
	st := pr.Stats
	if st.Expected != nil || st.Unexpected != nil || st.Flaky != nil || st.Skipped != nil {
		rep.Stats = Stats{Expected: deref(st.Expected), Unexpected: deref(st.Unexpected), Flaky: deref(st.Flaky), Skipped: deref(st.Skipped)}
	} else {
		rep.Stats = counted
	}
	if st.Duration != nil && *st.Duration >= 0 && !math.IsInf(*st.Duration, 0) && !math.IsNaN(*st.Duration) {
		d := int64(math.Round(*st.Duration))
		rep.Stats.DurationMs = &d
	}
	return rep, nil
}

func deref(p *int) int {
	if p == nil || *p < 0 {
		return 0
	}
	return *p
}

func merge(old, next string) string {
	switch {
	case old == "":
		return next
	case old == Fail || next == Fail:
		return Fail
	case old == Pass || next == Pass:
		return Pass
	}
	return Skipped
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// SkipReason is the reason(s) the tests of code were skipped, joined with "; " and clipped to max bytes.
// Empty when the report gave none. Callers redact it before storing it.
func (r *Report) SkipReason(code string, max int) string {
	s := strings.Join(r.SkipReasons[code], "; ")
	if len(s) > max {
		s = strings.ToValidUTF8(s[:max], "") + "..."
	}
	return s
}

var harnessRe = regexp.MustCompile(`(?i)staging db|credential|harness|seed`)

// HarnessHint is true when a skip reason points at the e2e harness prerequisites (staging DB credential, seed).
func HarnessHint(reason string) bool { return harnessRe.MatchString(reason) }
