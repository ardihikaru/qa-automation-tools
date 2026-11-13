package db

import (
	"strings"
	"testing"
)

func TestParseSkipIf(t *testing.T) {
	cases := []struct {
		name, in, query string
		ok              bool
	}{
		{"plain", "SELECT 1", "", false},
		{"alter", "ALTER TABLE t ADD COLUMN c INT", "", false},
		{"guard", "-- skip-if: SELECT 1\nALTER TABLE t", "SELECT 1", true},
		{"leading blank lines and spaces", "\n\n  \t-- skip-if:   SELECT 2  \nALTER", "SELECT 2", true},
		{"bom", "\ufeff-- skip-if: SELECT 3\nX", "SELECT 3", true},
		{"bom then blank", "\ufeff\n-- skip-if: SELECT 3\nX", "SELECT 3", true},
		{"crlf", "-- skip-if: SELECT 4\r\nX", "SELECT 4", true},
		{"upper case is not a directive", "-- SKIP-IF: SELECT 1\nX", "", false},
		{"no space after dashes", "--skip-if: SELECT 1\nX", "", false},
		{"two spaces after dashes", "--  skip-if: SELECT 1\nX", "", false},
		{"empty query", "-- skip-if:\nX", "", true},
		{"empty query spaces", "-- skip-if:   \nX", "", true},
		{"only the first line counts", "-- other comment\n-- skip-if: SELECT 1\nX", "", false},
		{"no newline", "-- skip-if: SELECT 5", "SELECT 5", true},
	}
	for _, c := range cases {
		q, ok := parseSkipIf(c.in)
		if q != c.query || ok != c.ok {
			t.Errorf("%s: got (%q,%v) want (%q,%v)", c.name, q, ok, c.query, c.ok)
		}
	}
}

// TestGuardedMigrationsAreKnown pins which entries carry a skip-if guard: only the automation-role
// ALTER and the two PRODUCT_KNOWLEDGE_SYNC entries (knowledge_format ALTER, one-time seed), in that order.
func TestGuardedMigrationsAreKnown(t *testing.T) {
	want := []string{
		"ADD COLUMN is_automation",
		"ADD COLUMN knowledge_format",
		"INSERT INTO product_settings (product_id, knowledge_format)",
		"ALTER TABLE kb_annotations MODIFY entity_type",
		"ALTER TABLE skills ADD COLUMN product_id",
	}
	var got []string
	for _, m := range migrations {
		if _, ok := parseSkipIf(m); ok {
			got = append(got, m)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("want %d guarded entries, got %d", len(want), len(got))
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("guarded entry %d does not contain %q", i, w)
		}
	}
}

func TestSummarizeSkipsDirective(t *testing.T) {
	got := summarize("-- skip-if: SELECT 1\nALTER TABLE product_repos ADD COLUMN x INT")
	if got != "ALTER TABLE product_repos ADD COLUMN x INT" {
		t.Fatalf("got %q", got)
	}
	if got := summarize("\n  \n  CREATE TABLE a (x INT)\nmore"); got != "CREATE TABLE a (x INT)" {
		t.Fatalf("plain: %q", got)
	}
	if got := summarize("-- not a directive\nX"); got != "-- not a directive" {
		t.Fatalf("other comment: %q", got)
	}
	if got := summarize("\ufeff-- skip-if: SELECT 1\nSELECT 2"); got != "SELECT 2" {
		t.Fatalf("bom: %q", got)
	}
}
