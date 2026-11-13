package knowledge

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"
)

// clipRunes cuts s to at most n runes without splitting a UTF-8 sequence.
func clipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug lowercases s, collapses every run outside [a-z0-9] to "-", trims "-" and cuts to max bytes.
func slug(s string, max int) string {
	out := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(out) > max {
		out = strings.TrimRight(out[:max], "-")
	}
	return out
}

// submoduleCode is "<module>~<slug(33)>-<6 hex of sha1(lower name)>" (at most 44 chars, no "/").
func submoduleCode(module, name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	sum := sha1.Sum([]byte(lower))
	return module + "~" + slug(lower, 33) + "-" + hex.EncodeToString(sum[:])[:6]
}

// CodeRe is the shape of any entity code accepted in an API path.
var CodeRe = regexp.MustCompile(`^[A-Za-z0-9.~_-]{1,64}$`)

// LocalCodeRe is the shape of a local draft code.
var LocalCodeRe = regexp.MustCompile(`^L-[A-Z0-9-]{1,30}$`)

// maxText is the cap for TEXT columns (65 535 bytes), with headroom.
const maxText = 60000

// clipBytes cuts s to at most n bytes on a rune boundary.
func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
