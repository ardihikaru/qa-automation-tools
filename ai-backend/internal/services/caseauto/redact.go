package caseauto

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Redactor is the secret screen of CASE_AUTOMATION.md section 3: everything written to
// job_events, sent to GAOS or put in an LLM prompt goes through Redact.
type Redactor struct {
	literals []string
}

var redactPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`), "${1}***"},
	{regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{16,}`), "***"},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`), "***"},
	{regexp.MustCompile(`gaos_[A-Za-z0-9_-]{8,}`), "***"},
	{regexp.MustCompile(`(?i)(https?://)[^/\s:@]+(?::[^/\s@]*)?@`), "${1}***@"},
}

// NewRedactor returns a screen that also hides the given literal secret values (values shorter
// than 6 characters are ignored: they would shred ordinary text).
func NewRedactor(literals ...string) *Redactor {
	r := &Redactor{}
	for _, l := range literals {
		if l = strings.TrimSpace(l); len(l) >= 6 {
			r.literals = append(r.literals, l)
		}
	}
	return r
}

// Redact replaces secrets in s by "***". A nil Redactor still applies the patterns.
func (r *Redactor) Redact(s string) string {
	if r != nil {
		for _, l := range r.literals {
			s = strings.ReplaceAll(s, l, "***")
		}
	}
	for _, p := range redactPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

// Clip cuts s to at most n bytes without splitting a rune, marking the cut.
func Clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	cut := n - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}
