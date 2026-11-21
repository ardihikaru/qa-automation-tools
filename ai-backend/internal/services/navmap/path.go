package navmap

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
)

// MaxPathLen is the longest normalised path kept (nav_routes.path is VARCHAR(300)).
const MaxPathLen = 300

var (
	schemeHostRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]*`)
	// A bare "host:port/..." or "host.tld/..." prefix (Playwright's step subtitle drops the scheme).
	bareHostRe  = regexp.MustCompile(`^(?:localhost|[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+|[A-Za-z0-9.-]+:[0-9]+)(?:/|$)`)
	angleRe     = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_]*)>`)
	braceRe     = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	templateRe  = regexp.MustCompile(`\$\{[^}]*\}`)
	uuidRe      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	longHexRe   = regexp.MustCompile(`^[0-9a-fA-F]{16,}$`)
	digitsRe    = regexp.MustCompile(`^[0-9]+$`)
	paramSegRe  = regexp.MustCompile(`^:[A-Za-z_][A-Za-z0-9_]*$`)
	badCharsRe  = regexp.MustCompile("[\\s\\x00-\\x1f\\x7f`'\"]")
	doubleSlash = regexp.MustCompile(`/{2,}`)
)

// NormPath normalises a route or URL to a path pattern (NAVIGATION_MAP.md 3.1). It returns ""
// when the text is not a usable path.
func NormPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	// 1. scheme://host, a bare host prefix, the query and the fragment.
	if loc := schemeHostRe.FindStringIndex(p); loc != nil {
		p = p[loc[1]:]
	} else if !strings.HasPrefix(p, "/") && bareHostRe.MatchString(p) {
		if i := strings.IndexByte(p, '/'); i >= 0 {
			p = p[i:]
		} else {
			p = "/"
		}
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	// 2. placeholders.
	p = templateRe.ReplaceAllString(p, ":param")
	p = angleRe.ReplaceAllString(p, ":$1")
	p = braceRe.ReplaceAllString(p, ":$1")
	// 3. slashes.
	p = doubleSlash.ReplaceAllString(p, "/")
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	if !strings.HasPrefix(p, "/") || strings.Contains(p, "..") || badCharsRe.MatchString(p) {
		return ""
	}
	// 4. id-like segments.
	segs := strings.Split(p[1:], "/")
	for i, s := range segs {
		if s == "" {
			continue
		}
		if digitsRe.MatchString(s) || uuidRe.MatchString(s) || longHexRe.MatchString(s) {
			segs[i] = ":id"
		}
	}
	p = "/" + strings.Join(segs, "/")
	// 5. a path that is only a parameter or a wildcard says nothing.
	if len(segs) == 1 && (segs[0] == "*" || paramSegRe.MatchString(segs[0])) {
		return ""
	}
	if len(p) > MaxPathLen {
		return ""
	}
	return p
}

// Segments splits a normalised path ("/" has none).
func Segments(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// IsParam reports whether a pattern segment is a ":name" parameter.
func IsParam(seg string) bool { return paramSegRe.MatchString(seg) }

// Match reports whether a concrete (normalised) path matches a route pattern: same number of
// segments, or a final "*" that matches one or more remaining segments; each pattern segment equals
// the path segment or is a ":name" parameter. The second result is the number of literal segments
// matched (more is a better match).
func Match(pattern, path string) (bool, int) {
	ps, xs := Segments(pattern), Segments(path)
	literal := 0
	for i, s := range ps {
		if s == "*" && i == len(ps)-1 {
			return len(xs) > i, literal
		}
		if i >= len(xs) {
			return false, 0
		}
		switch {
		case IsParam(s):
		case s == xs[i]:
			literal++
		default:
			return false, 0
		}
	}
	return len(ps) == len(xs), literal
}

// RouteKey is the stable identity of a route: hex(sha1(surface NUL app NUL path)).
func RouteKey(surface, app, path string) string {
	sum := sha1.Sum([]byte(surface + "\x00" + app + "\x00" + path))
	return hex.EncodeToString(sum[:])
}

// ParentPath returns the path without its last segment ("" for a one-segment path).
func ParentPath(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i <= 0 {
		return ""
	}
	return p[:i]
}
