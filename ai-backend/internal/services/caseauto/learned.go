package caseauto

// Learned policies in the generation prompt (LEARNING_LOOP.md decision 8): the merged
// .claude/rules/learned-*.md files of the automation repo are sent to the model after the repo
// guide, so a policy merged by the learning loop shapes the next generated spec.

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// GuideClip is how much of AGENTS.md reaches the model. A longer guide is cut and a warn event
// says so.
const GuideClip = 12000

// Budgets of the learned-policy block.
const (
	LearnedFileClip = 1500
	LearnedBudget   = 16000
)

// LearnedFile is one learned policy file of the worktree.
type LearnedFile struct {
	Path    string
	Content string
}

var provenanceRe = regexp.MustCompile(`<!--\s*qa-learning:[^>]*-->`)

// LearnedPolicyPart returns the part of a learned policy file the model needs: its title and the
// text between the provenance comment and "## Why" (why and evidence are for humans). A file not
// in that shape is returned whole, without YAML front matter.
func LearnedPolicyPart(content string) string {
	s := strings.TrimPrefix(strings.ReplaceAll(content, "\r\n", "\n"), "\ufeff")
	if strings.HasPrefix(s, "---\n") {
		if end := strings.Index(s[4:], "\n---"); end >= 0 {
			rest := s[4+end+4:]
			if i := strings.IndexByte(rest, '\n'); i >= 0 {
				s = rest[i+1:]
			} else {
				s = ""
			}
		}
	}
	loc := provenanceRe.FindStringIndex(s)
	if loc == nil {
		return strings.TrimSpace(s)
	}
	title := ""
	head := strings.TrimSpace(s[:loc[0]])
	if strings.HasPrefix(head, "# ") {
		title = strings.TrimSpace(strings.SplitN(strings.TrimPrefix(head, "# "), "\n", 2)[0])
	}
	body := "\n" + s[loc[1]:]
	if i := strings.Index(body, "\n## Why"); i >= 0 {
		body = body[:i]
	}
	body = strings.TrimSpace(body)
	if title != "" {
		return strings.TrimSpace(title + "\n" + body)
	}
	return body
}

// learnedBlock reads the worktree's learned policies and returns the prompt block ("" when there
// are none). Files are taken in path order; one that would pass the budget is left out with a
// warn event naming it.
func (m *Machine) learnedBlock(ctx context.Context, r *run) string {
	files, err := m.Repo.ReadLearned(r.ws)
	if err != nil {
		m.ev(ctx, r.job, "warn", StepGenerate, "could not read the learned policies: "+err.Error())
		return ""
	}
	var b strings.Builder
	used, n := 0, 0
	var left []string
	for _, f := range files {
		part := Clip(LearnedPolicyPart(f.Content), LearnedFileClip)
		if part == "" {
			continue
		}
		entry := "// " + f.Path + "\n" + part + "\n\n"
		if used+len(entry) > LearnedBudget {
			left = append(left, f.Path)
			continue
		}
		b.WriteString(entry)
		used += len(entry)
		n++
	}
	if len(left) > 0 {
		m.ev(ctx, r.job, "warn", StepGenerate, fmt.Sprintf("learned policies over the %d-byte budget were left out: %s", LearnedBudget, strings.Join(left, ", ")))
	}
	if n == 0 {
		return ""
	}
	noun := "policies"
	if n == 1 {
		noun = "policy"
	}
	m.ev(ctx, r.job, "info", StepGenerate, fmt.Sprintf("%d learned %s in the prompt", n, noun))
	return "\nLEARNED POLICIES (merged rules of this repository; follow them like the repo guide):\n" + b.String()
}

var excerptRe = regexp.MustCompile(`(?m)^\s*((?:Error|TypeError|ReferenceError|SyntaxError|RangeError)\b.*|.*\bexpect\(.*|.*\bTimeout\b.*\bexceeded\b.*)$`)

// FailureExcerpt is the first error line of a run's output tail (clipped to 300 bytes), "" when
// there is none. It turns "the spec did not pass: TC-X failed" into a lesson a human can read.
func FailureExcerpt(tail string) string {
	m := excerptRe.FindStringSubmatch(tail)
	if m == nil {
		return ""
	}
	return Clip(strings.TrimSpace(m[1]), 300)
}
