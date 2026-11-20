// Package learnloop is the learning loop of LEARNING_LOOP.md: findings observed in real
// automation work (reviewer answers, repair rounds, repeated failures, manual entries) are
// deduplicated, turned into AI-drafted policy proposals, approved by a human and published as
// one new .claude/rules/learned-<slug>.md file per policy in the product's automation repo.
//
// This file holds the pure text functions: normalisation, fingerprints and failure signatures.
// They decide what counts as "the same lesson" (exact fingerprints only, decision 11).
package learnloop

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/qa-automation/ai-backend/internal/services/epicauto"
)

var (
	caseCodeRe = regexp.MustCompile(`(?i)\btc-[a-z0-9]+-[0-9]+\b`)
	urlRe      = regexp.MustCompile(`(?i)\bhttps?://\S+`)
	quotedRe   = regexp.MustCompile("\"[^\"\n]{0,200}\"|'[^'\n]{0,200}'|`[^`\n]{0,200}`")
	hexRe      = regexp.MustCompile(`(?i)\b[0-9a-f]{7,}\b`)
	numberRe   = regexp.MustCompile(`\b[0-9]+(?:\.[0-9]+)?\b`)
	nonWordRe  = regexp.MustCompile(`[^a-z0-9<>]+`)
	spaceRe    = regexp.MustCompile(`\s+`)
)

// normalizeMax bounds the normalised text (and so the fingerprint input).
const normalizeMax = 400

// Normalize reduces a lesson text to its stable shape: lower case, case codes, URLs, quoted
// strings, long hex and numbers replaced by placeholders, punctuation folded to spaces.
// Two texts that differ only in those details normalise to the same string.
func Normalize(s string) string {
	s = strings.ToLower(s)
	s = urlRe.ReplaceAllString(s, " <url> ")
	s = caseCodeRe.ReplaceAllString(s, " <code> ")
	s = quotedRe.ReplaceAllString(s, " <str> ")
	s = hexRe.ReplaceAllString(s, " <n> ")
	s = numberRe.ReplaceAllString(s, " <n> ")
	s = nonWordRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	if len(s) > normalizeMax {
		s = strings.TrimSpace(s[:normalizeMax])
	}
	return s
}

// Fingerprint is the hex sha256 of Normalize(s).
func Fingerprint(s string) string {
	h := sha256.Sum256([]byte(Normalize(s)))
	return hex.EncodeToString(h[:])
}

// The prefixes the case_create machine writes before a failure text (caseauto/machine.go).
const (
	PrefixRefused = "the reply was refused: "
	PrefixNotPass = "the spec did not pass: "
)

// IsHumanOutcome reports whether a job error records a human decision (epicauto.HumanOutcome) or
// an approval wait that ended; the repeated-failure source ignores those.
func IsHumanOutcome(errText string) bool {
	e := strings.TrimSpace(errText)
	return epicauto.HumanOutcome(e) || strings.HasPrefix(e, "stopped while waiting for approval")
}

// verdictTexts are executor verdicts that name no failure (caseauto/verdict.go).
var verdictTexts = []string{"no test titled", "the executor returned no report", "the report could not be read"}

var genericRe = regexp.MustCompile(`^(?:<code> )?(?:failed|passed|was skipped)$`)

// Signature returns the failure text of a message without its generic prefix, and whether it
// says enough to be a signature: a remainder shorter than 12 characters, or one that only says
// "<code> failed", is not one. For "the spec did not pass: TC-X failed\n<excerpt>" the
// excerpt is the signature.
func Signature(msg string) (string, bool) {
	s := strings.TrimSpace(msg)
	for _, p := range []string{PrefixRefused, PrefixNotPass} {
		if strings.HasPrefix(strings.ToLower(s), p) {
			s = strings.TrimSpace(s[len(p):])
			break
		}
	}
	// "TC-X failed\n<excerpt>": the first line is the generic verdict, the rest is the failure.
	if first, rest, ok := strings.Cut(s, "\n"); ok && genericRe.MatchString(Normalize(first)) {
		s = strings.TrimSpace(rest)
	}
	n := Normalize(s)
	if len(n) < 12 || genericRe.MatchString(n) {
		return "", false
	}
	for _, v := range verdictTexts {
		if strings.HasPrefix(strings.ToLower(s), v) {
			return "", false
		}
	}
	return s, true
}
