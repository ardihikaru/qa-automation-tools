package caseauto

import (
	"strings"
	"testing"
)

func TestAnswerSourceHelpers(t *testing.T) {
	if SafeDetail([]byte(`[1]`)) != nil || SafeDetail([]byte(`null`)) != nil || SafeDetail([]byte(`{bad`)) != nil {
		t.Error("non-objects must be dropped")
	}
	if SafeDetail([]byte(`{"a":"`+strings.Repeat("x", MaxDetailBytes)+`"}`)) != nil {
		t.Error("oversize must be dropped")
	}
	if got := string(SafeDetail([]byte(` { "kind" : "gaos" } `))); got != `{"kind":"gaos"}` {
		t.Errorf("compact %q", got)
	}
	if got := CleanText("a\x00b\nc  ", 120); got != "abc" {
		t.Errorf("clean %q", got)
	}
	if got := CleanText(strings.Repeat("é", 200), 120); len([]rune(got)) != 120 {
		t.Errorf("rune clip %d", len([]rune(got)))
	}
	for d, want := range map[string]string{
		`{"actor":"budi"}`: "budi",
		`{"actor":{"username":"budi","display":"B"}}`: "B",
		`{"actor":{"username":"budi"}}`:               "budi",
		`{"kind":"gaos"}`:                             "",
		`{"actor":"operator"}`:                        "operator (GAOS)",
	} {
		if got := ActorFromDetail([]byte(d)); got != want {
			t.Errorf("%s: %q", d, got)
		}
	}
	if ActorUsername([]byte(`{"actor":{"username":"budi","display":"B"}}`)) != "budi" {
		t.Error("username")
	}
	if SourceForDoor("dashboard", ApprovalApproved) != SourceJobPage || SourceForDoor("gaos", ApprovalExpired) != SourceSystem || SourceForDoor("gaos", ApprovalRejected) != SourceGAOS {
		t.Error("door mapping")
	}
	if (&Approval{Actor: "ardi", Source: SourceHistory}).Who() != "ardi (QA tools, history)" || (&Approval{AnsweredBy: "dashboard"}).Who() != "dashboard" || (&Approval{Source: SourceSystem}).Who() != "expired" {
		t.Error("who")
	}
}
