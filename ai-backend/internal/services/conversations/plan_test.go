package conversations

import (
	"strings"
	"testing"
)

func TestPlan(t *testing.T) {
	cases := []struct {
		name    string
		last    []string
		sent    []string
		p       int
		rebuilt bool
	}{
		{"empty conversation", nil, []string{"a"}, 0, false},
		{"a normal round extends the last call", []string{"a", "r1"}, []string{"a", "r1", "u2"}, 2, false},
		{"identical retry after an error", []string{"a", "u2"}, []string{"a", "u2"}, 2, false},
		{"resume rebuilt the conversation", []string{"a", "r1", "u2", "r2"}, []string{"a", "x", "f"}, 1, true},
		{"resume with a moved repo", []string{"a", "r1"}, []string{"b", "x", "f"}, 0, true},
		{"after the resume, rounds match again", []string{"b", "x", "f", "r3"}, []string{"b", "x", "f", "r3", "u4"}, 4, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, rebuilt := Plan(c.last, c.sent)
			if p != c.p || rebuilt != c.rebuilt {
				t.Errorf("got %d %v want %d %v", p, rebuilt, c.p, c.rebuilt)
			}
		})
	}
}

func TestScreen(t *testing.T) {
	s := NewScreen("db-password-123")
	in := strings.Join([]string{
		"the db password is db-password-123",
		"Authorization: Bearer abcdefghijkl",
		"token = ghp_abcdefghijklmnopqrstuvwxyz0123",
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJlLXZhbHVl",
		"aws AKIAABCDEFGHIJKLMNOP",
		"key sk-abcdefghijklmnopqrstuvwxyz",
		"password: hunter22",
		`"api_key": "zzz-secret-value"`,
		"-----BEGIN RSA PRIVATE KEY-----\nMIIabc\n-----END RSA PRIVATE KEY-----",
		"ordinary text with a token budget of 5",
	}, "\n")
	out := s.Redact(in)
	for _, leak := range []string{"db-password-123", "abcdefghijkl", "ghp_abc", "eyJhbGci", "AKIAABCD", "sk-abcdef", "hunter22", "zzz-secret-value", "MIIabc"} {
		if strings.Contains(out, leak) {
			t.Errorf("leaked %q in\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "ordinary text with a token budget of 5") || !strings.Contains(out, "password: ***") {
		t.Errorf("screen over-redacted:\n%s", out)
	}
}

func TestHashUsesRedactedTextSoBothSidesMatch(t *testing.T) {
	s := NewScreen()
	raw := "Your reply was refused: password=abcd1234"
	if Hash("user", s.Redact(raw)) != Hash("user", s.Redact(raw)) || Hash("user", "x") == Hash("assistant", "x") {
		t.Fatal("hash identity")
	}
}

func TestDeriveKindAndClip(t *testing.T) {
	for _, c := range []struct {
		role, given string
		first       bool
		want        string
	}{
		{"system", "", false, KindPrompt}, {"user", "", true, KindPrompt}, {"user", "", false, KindMessage},
		{"assistant", "", false, KindReply}, {"user", "feedback", false, KindFeedback},
	} {
		if got := DeriveKind(c.role, c.given, c.first); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
	big := strings.Repeat("é", MaxMessageBytes)
	out := Clip(big)
	if len(out) > MaxMessageBytes+64 || !strings.Contains(out, "[clipped,") || !strings.HasPrefix(out, "é") {
		t.Errorf("clip: %d bytes", len(out))
	}
	if Clip("short") != "short" {
		t.Error("short text changed")
	}
}
