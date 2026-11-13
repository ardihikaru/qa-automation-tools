package knowledge

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Kinds of AI-assistant files synced from a product's automation repo (PRODUCT_KNOWLEDGE_SYNC.md,
// "Assistant files").
const (
	KindAgent        = "agent"
	KindSkill        = "skill"
	KindRule         = "rule"
	KindCommand      = "command"
	KindInstructions = "instructions"
)

// AssistantKinds lists the kinds in display order.
var AssistantKinds = []string{KindAgent, KindSkill, KindRule, KindCommand, KindInstructions}

// AssistantsVersion changes whenever the assistant parse output for the same input changes.
const AssistantsVersion = 1

// Assistant is one agent, skill, rule, command or instructions file.
type Assistant struct {
	Code        string          `json:"code"`
	Kind        string          `json:"kind"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Path        string          `json:"path"`
	Frontmatter json.RawMessage `json:"frontmatter"`
	Body        string          `json:"body"`
}

// skipDirs are never scanned for CLAUDE.md / AGENTS.md (vendored or generated trees).
var skipDirs = map[string]bool{"node_modules": true, "vendor": true, ".git": true, "dist": true, "build": true}

// assistantKind returns the kind of a repo path, or "" when the path is not an assistant file.
func assistantKind(p string) string {
	if !strings.HasSuffix(p, ".md") {
		return ""
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if skipDirs[seg] {
			return ""
		}
	}
	switch base := path.Base(p); {
	case strings.HasPrefix(p, ".claude/agents/"):
		return KindAgent
	case strings.HasPrefix(p, ".claude/skills/") && base == "SKILL.md" && strings.Count(p, "/") == 3:
		return KindSkill
	case strings.HasPrefix(p, ".claude/rules/"):
		return KindRule
	case strings.HasPrefix(p, ".claude/commands/"):
		return KindCommand
	case base == "CLAUDE.md" || base == "AGENTS.md":
		return KindInstructions
	}
	return ""
}

// KeepForSync is the ReadTree filter of a knowledge sync: the docs/ catalog plus assistant files.
func KeepForSync(p string) bool {
	return strings.HasPrefix(p, "docs/") || assistantKind(p) != ""
}

// assistantCode is "<kind>~<slug(33) of the path without .md>-<6 hex of sha1(path)>" (at most 54 chars).
func assistantCode(kind, p string) string {
	sum := sha1.Sum([]byte(p))
	return kind + "~" + slug(strings.TrimSuffix(p, ".md"), 33) + "-" + hex.EncodeToString(sum[:])[:6]
}

// splitFrontmatter separates a leading "---" YAML block from the body.
func splitFrontmatter(b []byte) (fm []byte, body []byte) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	norm := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(norm, []byte("---\n")) {
		return nil, norm
	}
	rest := norm[4:]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return nil, norm
	}
	fm = rest[:end]
	body = rest[end+4:]
	if i := bytes.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	} else {
		body = nil
	}
	return fm, body
}

// describe returns the first heading text and the first plain paragraph line of body.
func describe(body string) (heading, para string) {
lines:
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case t == "":
			continue
		case strings.HasPrefix(t, "#"):
			if heading == "" {
				heading = strings.TrimSpace(strings.TrimLeft(t, "#"))
			}
		case para == "" && !strings.HasPrefix(t, "```") && !strings.HasPrefix(t, "|") && !strings.HasPrefix(t, "<"):
			para = t
		}
		if heading != "" && para != "" {
			break lines
		}
	}
	return heading, para
}

// ParseAssistants extracts assistant files from a repo file set.
func ParseAssistants(files map[string][]byte) ([]Assistant, []Problem) {
	var out []Assistant
	var problems []Problem
	paths := make([]string, 0, len(files))
	for p := range files {
		if assistantKind(p) != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		kind := assistantKind(p)
		fmRaw, body := splitFrontmatter(files[p])
		fm := map[string]any{}
		if len(fmRaw) > 0 {
			if err := yaml.Unmarshal(fmRaw, &fm); err != nil {
				problems = append(problems, Problem{File: p, Line: 1, Msg: "frontmatter is not valid YAML: " + clipRunes(err.Error(), 200)})
				fm = map[string]any{}
			}
		}
		a := Assistant{Code: assistantCode(kind, p), Kind: kind, Path: p, Body: string(body)}
		if !utf8.ValidString(a.Body) {
			a.Body = strings.ToValidUTF8(a.Body, "�")
		}
		if v, ok := fm["name"].(string); ok {
			a.Name = strings.TrimSpace(v)
		}
		if v, ok := fm["description"].(string); ok {
			a.Description = strings.TrimSpace(v)
		}
		heading, para := describe(a.Body)
		if a.Name == "" {
			switch kind {
			case KindSkill:
				a.Name = path.Base(path.Dir(p))
			case KindInstructions:
				if d := path.Dir(p); d != "." {
					a.Name = d + "/" + path.Base(p)
				} else {
					a.Name = path.Base(p)
				}
			default:
				a.Name = strings.TrimSuffix(path.Base(p), ".md")
			}
		}
		if a.Description == "" {
			if para != "" {
				a.Description = para
			} else {
				a.Description = heading
			}
		}
		a.Name = clipRunes(a.Name, 300)
		a.Description = clipRunes(a.Description, 1000)
		if len(a.Body) > 1<<20 {
			a.Body = clipBytes(a.Body, 1<<20)
			problems = append(problems, Problem{File: p, Msg: a.Code + " body truncated to 1 MiB"})
		}
		b, err := json.Marshal(jsonSafe(fm))
		if err != nil {
			b = []byte("{}")
		}
		a.Frontmatter = b
		out = append(out, a)
	}
	return out, problems
}

// jsonSafe converts yaml.v3 values (map[string]any with nested map[any]any is not produced by v3,
// but times and other scalars may be) into JSON-encodable values.
func jsonSafe(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, vv := range x {
			m[k] = jsonSafe(vv)
		}
		return m
	case map[any]any:
		m := make(map[string]any, len(x))
		for k, vv := range x {
			if ks, ok := k.(string); ok {
				m[ks] = jsonSafe(vv)
			}
		}
		return m
	case []any:
		for i := range x {
			x[i] = jsonSafe(x[i])
		}
		return x
	case string, bool, int, int64, float64, nil:
		return x
	default:
		if s, ok := v.(interface{ String() string }); ok {
			return s.String()
		}
		return nil
	}
}
