package knowledge

import (
	"reflect"
	"testing"
)

func TestParseEvidence(t *testing.T) {
	got := ParseEvidence("S:PAY-010; GIT:main · SRV:20261126 · GIT:clinic-backend@de5f91408/2026-06-29 · GIT:main@2025-12-03 · DOC:Plan-2024 · RN:20260107 · BRD:D4 · GIT:stg", "acme")
	want := []EvidenceItem{
		{Kind: "S", Raw: "S:PAY-010", Label: "Plan sheet row PAY-010"},
		{Kind: "GIT", Raw: "GIT:main", Label: "Code on main"},
		{Kind: "SRV", Raw: "SRV:20261126", Label: "Release survey", Date: "2026-11-26"},
		{Kind: "GIT", Raw: "GIT:clinic-backend@de5f91408/2026-06-29", Label: "Merged into main: clinic-backend de5f91408", Repo: "clinic-backend", SHA: "de5f91408", Date: "2026-06-29", URL: "https://github.com/acme/clinic-backend/commit/de5f91408"},
		{Kind: "GIT", Raw: "GIT:main@2025-12-03", Label: "Code reached main", Date: "2025-12-03"},
		{Kind: "DOC", Raw: "DOC:Plan-2024", Label: "Document: Plan-2024"},
		{Kind: "RN", Raw: "RN:20260107", Label: "Release note 20260107", Date: "2026-01-07"},
		{Kind: "BRD", Raw: "BRD:D4", Label: "BRD domain D4"},
		{Kind: "GIT", Raw: "GIT:stg", Label: "Code on staging only"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if l := ParseEvidence("GIT:web@abc1234", ""); len(l) != 1 || l[0].URL != "" || l[0].SHA != "abc1234" || l[0].Date != "" {
		t.Errorf("no owner / no date: %+v", l)
	}
	if l := ParseEvidence("owner confirmed in a meeting", ""); len(l) != 1 || l[0].Kind != "TEXT" {
		t.Errorf("free text: %+v", l)
	}
	if l := ParseEvidence("—", "x"); len(l) != 0 {
		t.Errorf("dash: %+v", l)
	}
	for _, u := range []string{"git@github.com:acme/auto.git", "https://github.com/acme/auto"} {
		if m := repoOwnerRe.FindStringSubmatch(u); m == nil || m[1] != "acme" {
			t.Errorf("owner of %s", u)
		}
	}
}
