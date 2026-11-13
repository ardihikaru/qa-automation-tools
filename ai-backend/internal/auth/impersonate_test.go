package auth

import (
	"testing"
	"time"
)

func TestCanImpersonate(t *testing.T) {
	for role, want := range map[string]bool{
		"Admin": true, "Service": true,
		"Developer": false, "DevOps": false, "QA": false, "QC": false, "UI/UX Designer": false, "": false,
	} {
		if got := CanImpersonate(&User{Role: role}); got != want {
			t.Errorf("%q: got %v want %v", role, got, want)
		}
	}
	if CanImpersonate(nil) {
		t.Error("nil user must not impersonate")
	}
	// Nested: an Admin target reached by impersonation cannot impersonate again.
	nested := &User{Role: "Admin", Impersonator: &Actor{ID: "x", Role: "Service"}}
	if CanImpersonate(nested) {
		t.Error("impersonated session must not impersonate (no nesting)")
	}
}

func TestRequireRealUser(t *testing.T) {
	if !RequireRealUser(&User{Role: "QA"}) {
		t.Error("real user must pass")
	}
	if RequireRealUser(&User{Role: "QA", Impersonator: &Actor{}}) {
		t.Error("impersonated user must not pass")
	}
	if RequireRealUser(nil) {
		t.Error("nil must not pass")
	}
}

func TestImpersonationExpiry(t *testing.T) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	if got := ImpersonationExpiry(now, now.Add(12*time.Hour)); !got.Equal(now.Add(time.Hour)) {
		t.Errorf("long parent: got %v want now+1h", got)
	}
	short := now.Add(20 * time.Minute)
	if got := ImpersonationExpiry(now, short); !got.Equal(short) {
		t.Errorf("short parent: got %v want parent expiry", got)
	}
	// Non-UTC input is normalised.
	loc := time.FixedZone("WIB", 7*3600)
	if got := ImpersonationExpiry(now.In(loc), now.Add(5*time.Hour)); got.Location() != time.UTC {
		t.Errorf("result must be UTC, got %v", got.Location())
	}
}
