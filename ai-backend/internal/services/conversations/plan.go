// Package conversations stores every AI exchange made through ai.Service, grouped per job, sync
// or single call, and serves, archives and prunes them (docs/designs/AI_CONVERSATIONS.md).
package conversations

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/qa-automation/ai-backend/internal/services/caseauto"
)

// Message kinds (AI_CONVERSATIONS.md 3.4).
const (
	KindPrompt    = "prompt"
	KindMessage   = "message"
	KindReply     = "reply"
	KindFeedback  = "feedback"
	KindRefusal   = "refusal"
	KindRunResult = "run_result"
	KindRebuilt   = "rebuilt"
	KindError     = "error"
)

// RoleNote is the role of the store's own notes (rebuilt, error).
const RoleNote = "note"

// MaxMessageBytes clips one stored message (after redaction and hashing).
const MaxMessageBytes = 256 << 10

// RebuiltNote is written when a call did not simply extend the previous one.
const RebuiltNote = "The conversation was rebuilt (for example after a backend restart); the messages below were sent again."

// Screen is the storage redaction: the case-automation redactor (literal secrets and its token
// patterns) followed by patterns used only for what is stored, never for what is sent to the LLM.
type Screen struct {
	base *caseauto.Redactor
}

var storePatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), "***"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "***"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "***"},
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`), "***"},
	{regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|api[_-]?key|token)(["']?\s*[:=]\s*["']?)[^\s"',;]{3,}`), "${1}${2}***"},
}

// NewScreen returns the storage screen; literals are secret values (shorter than 6 characters ignored).
func NewScreen(literals ...string) *Screen { return &Screen{base: caseauto.NewRedactor(literals...)} }

// Redact applies the redactor and the storage-only patterns. A nil Screen still applies the patterns.
func (s *Screen) Redact(text string) string {
	var base *caseauto.Redactor
	if s != nil {
		base = s.base
	}
	out := base.Redact(text)
	for _, p := range storePatterns {
		out = p.re.ReplaceAllString(out, p.repl)
	}
	return out
}

// Hash is the identity of a message in a conversation: sha256(role + "\n" + redacted content),
// over the unclipped redacted text, so both sides of a comparison see the same value.
func Hash(role, redacted string) string {
	sum := sha256.Sum256([]byte(role + "\n" + redacted))
	return hex.EncodeToString(sum[:])
}

// Plan compares the previous call (its sent messages and reply, as hashes) with the new call's
// sent messages: p is their longest common prefix (sent[p:] is new) and rebuilt is true when the
// new call did not simply extend the previous one.
func Plan(last, sent []string) (p int, rebuilt bool) {
	for p < len(last) && p < len(sent) && last[p] == sent[p] {
		p++
	}
	return p, p < len(last)
}

// Clip cuts content at MaxMessageBytes on a rune boundary and says how much was cut.
func Clip(content string) string {
	if len(content) <= MaxMessageBytes {
		return content
	}
	cut := content[:MaxMessageBytes]
	for !utf8.ValidString(cut) && len(cut) > 0 {
		cut = cut[:len(cut)-1]
	}
	return cut + fmt.Sprintf("\n… [clipped, %d bytes]", len(content))
}

// DeriveKind is the kind of a message the caller did not label: system → prompt, the first
// message of a conversation → prompt, assistant → reply, else message.
func DeriveKind(role, given string, first bool) string {
	if k := strings.TrimSpace(given); k != "" {
		return k
	}
	switch {
	case role == "system":
		return KindPrompt
	case role == "assistant":
		return KindReply
	case first:
		return KindPrompt
	}
	return KindMessage
}
