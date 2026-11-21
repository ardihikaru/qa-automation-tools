package caseauto

// HISTORY_ANSWER.md: where an approval's answer came from. The vocabulary is closed.

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"unicode"
)

// Answer sources (case_approvals.answer_source).
const (
	SourceGAOS     = "gaos"             // the GAOS webhook, the poll, or a 409 carrying someone else's answer
	SourceHistory  = "qa-tool:history"  // the /history shortcut
	SourceJobPage  = "qa-tool:job-page" // the job page's approval card
	SourceLearning = "qa-tool:learning" // the learning page's proposal buttons
	SourceSystem   = "system"           // an expiry, wherever it was decided
)

// MaxDetailBytes caps GAOS's stored source object.
const MaxDetailBytes = 2048

// AnswerMeta is everything recorded with an answer.
type AnswerMeta struct {
	By     string          // the door: dashboard | gaos | system (answered_by, kept for compatibility)
	Text   string          // the comment
	Source string          // one of the Source* constants
	Actor  string          // who answered ("ardi", "ai-service as ardi", GAOS's actor)
	Detail json.RawMessage // GAOS's source object, verbatim; dropped unless a small JSON object
}

// SourcedAnswerer is implemented by an Approvals store that records the answer's source.
type SourcedAnswerer interface {
	AnswerSourced(ctx context.Context, externalID, status string, m AnswerMeta) (bool, error)
}

// SourceForDoor derives the source of an answer given only its door: an expiry is always system.
func SourceForDoor(by, status string) string {
	if status == ApprovalExpired {
		return SourceSystem
	}
	switch by {
	case "dashboard":
		return SourceJobPage
	case "gaos":
		return SourceGAOS
	case "system":
		return SourceSystem
	}
	return ""
}

func (m AnswerMeta) normalised(status string) AnswerMeta {
	if status == ApprovalExpired {
		m.Source = SourceSystem
	}
	if m.Source == "" {
		m.Source = SourceForDoor(m.By, status)
	}
	m.Actor = CleanText(m.Actor, 120)
	m.Detail = SafeDetail(m.Detail)
	return m
}

// SafeDetail returns d compacted when it is a JSON object of at most MaxDetailBytes, else nil.
func SafeDetail(d json.RawMessage) json.RawMessage {
	d = bytes.TrimSpace(d)
	if len(d) == 0 || d[0] != '{' || !json.Valid(d) {
		return nil
	}
	var buf bytes.Buffer
	if json.Compact(&buf, d) != nil || buf.Len() > MaxDetailBytes {
		return nil
	}
	return buf.Bytes()
}

// CleanText removes control characters, trims, and clips to max runes.
func CleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max]))
	}
	return s
}

type detailActor struct {
	Actor json.RawMessage `json:"actor"`
}

// actorOf returns the actor string, or the actor object's username and display.
func actorOf(d json.RawMessage) (str, username, display string) {
	var src detailActor
	if len(d) == 0 || json.Unmarshal(d, &src) != nil || len(src.Actor) == 0 {
		return "", "", ""
	}
	if json.Unmarshal(src.Actor, &str) == nil {
		return str, str, ""
	}
	var o struct {
		Username string `json:"username"`
		Display  string `json:"display"`
	}
	_ = json.Unmarshal(src.Actor, &o)
	return "", o.Username, o.Display
}

// ActorFromDetail reads who answered from GAOS's source object: actor as a string, else
// actor.display, else actor.username; GAOS's back-office "operator" reads "operator (GAOS)".
// "" when there is none.
func ActorFromDetail(d json.RawMessage) string {
	str, user, display := actorOf(d)
	switch {
	case str == "operator" || user == "operator":
		// GAOS v0.43.0 reports its back-office actor as "operator".
		return "operator (GAOS)"
	case str != "":
		return CleanText(str, 120)
	case display != "":
		return CleanText(display, 120)
	}
	return CleanText(user, 120)
}

// ActorUsername reads actor.username (or the actor string) from GAOS's source object.
func ActorUsername(d json.RawMessage) string {
	_, user, _ := actorOf(d)
	return user
}

// SourceLabel is the human label of a source ("" for an unknown one).
func SourceLabel(source string) string {
	switch source {
	case SourceGAOS:
		return "GAOS"
	case SourceHistory:
		return "QA tools, history"
	case SourceJobPage:
		return "QA tools, job page"
	case SourceLearning:
		return "QA tools, learning"
	case SourceSystem:
		return "expired"
	}
	return ""
}

// Who is "<actor> (<source label>)", else the actor, else the label, else the old door value.
func (a *Approval) Who() string {
	if a == nil {
		return ""
	}
	label := SourceLabel(a.Source)
	switch {
	case a.Actor != "" && label != "":
		return a.Actor + " (" + label + ")"
	case a.Actor != "":
		return a.Actor
	case label != "":
		return label
	}
	return a.AnsweredBy
}
