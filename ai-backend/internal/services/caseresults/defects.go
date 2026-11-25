package caseresults

// QA_DEFECT_TRIAGE.md: the defect view of a Playwright report. A test is read for three things:
// a pinned failure that failed as expected (xfail), a pinned failure that now passes (xpass) and an
// unexpected failure (fail). The pass/fail/skipped results of Parse do not change.

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Defect kinds, the words GAOS accepts.
const (
	DefectXFail = "xfail"
	DefectXPass = "xpass"
	DefectFail  = "fail"
)

const (
	// MaxDefectTitle is the longest title (runes) and MaxExcerpt the longest excerpt (bytes) GAOS takes.
	MaxDefectTitle = 200
	MaxExcerpt     = 4096
)

var (
	defectIDRe = regexp.MustCompile(`DEF-[0-9]{1,6}\b`)
	ansiRe     = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
)

// Defect is what one case's tests say about defects. DefectID is "" for kind fail.
type Defect struct {
	Kind     string
	DefectID string
	Title    string // from the annotation (id stripped); the test title for a fail
	Excerpt  string // the failure output, or the annotation's reason for an xfail (not yet redacted)
}

func rank(k string) int {
	switch k {
	case DefectFail:
		return 3
	case DefectXPass:
		return 2
	case DefectXFail:
		return 1
	}
	return 0
}

// addDefect keeps the most serious observation per case: fail, then xpass, then xfail.
func (r *Report) addDefect(code string, d Defect) {
	if old, ok := r.Defects[code]; ok && rank(old.Kind) >= rank(d.Kind) {
		return
	}
	r.Defects[code] = d
}

func lastError(rs []pwResult) string {
	for i := len(rs) - 1; i >= 0; i-- {
		if e := rs[i].Error; e != nil && strings.TrimSpace(e.Message) != "" {
			return e.Message
		}
		for _, e := range rs[i].Errors {
			if strings.TrimSpace(e.Message) != "" {
				return e.Message
			}
		}
	}
	return ""
}

// classifyDefect reads one test. Only a test with a defect annotation (type "defect", an id
// DEF-n in its description) can be an xfail or an xpass; any unexpected failure is a fail.
func classifyDefect(specTitle, status, expected string, anns []pwAnnotation, errText string) (Defect, bool) {
	var desc, id string
	for _, a := range anns {
		if a.Type != "defect" {
			continue
		}
		if m := defectIDRe.FindString(a.Description); m != "" {
			desc, id = a.Description, m
			break
		}
	}
	title := defectTitle(desc, id)
	errText = clipBytesUTF8(strings.TrimSpace(ansiRe.ReplaceAllString(errText, "")), MaxExcerpt)
	switch {
	case status == "expected" && expected == "failed" && id != "":
		if title == "" {
			title = "pinned defect " + id
		}
		return Defect{Kind: DefectXFail, DefectID: id, Title: title, Excerpt: strings.TrimSpace(desc)}, true
	case status == "unexpected" && expected == "failed" && id != "":
		if title == "" {
			title = "pinned defect " + id
		}
		return Defect{Kind: DefectXPass, DefectID: id, Title: title, Excerpt: errText}, true
	case status == "unexpected" && expected != "failed":
		t := strings.TrimSpace(specTitle)
		if title != "" {
			t = title
		}
		return Defect{Kind: DefectFail, Title: clipRunesN(t, MaxDefectTitle), Excerpt: errText}, true
	}
	return Defect{}, false
}

// defectTitle is the description without its id and the separators that follow it.
func defectTitle(desc, id string) string {
	if id == "" {
		return ""
	}
	t := strings.Replace(desc, id, "", 1)
	t = strings.TrimSpace(strings.Trim(strings.TrimSpace(t), ":-–—|[]() \t"))
	return clipRunesN(t, MaxDefectTitle)
}

func clipRunesN(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func clipBytesUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
