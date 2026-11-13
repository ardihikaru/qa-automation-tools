package seed

import "testing"

func TestNormalizeRole(t *testing.T) {
	for in, want := range map[string]string{
		"Admin": "Admin", "Developer": "Developer", "DevOps": "DevOps",
		"QA": "QA", "QC": "QC", "UI/UX Designer": "UI/UX Designer",
		// Service must never come from taskmgr: only upsertServiceUser writes it.
		"Service": "Developer",
		"":        "Developer", "admin": "Developer", "Mentor": "Developer",
	} {
		if got := normalizeRole(in); got != want {
			t.Errorf("normalizeRole(%q) = %q, want %q", in, got, want)
		}
	}
}
