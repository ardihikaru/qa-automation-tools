package knowledge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAssistants(t *testing.T) {
	files := map[string][]byte{
		".claude/agents/e2e-test-engineer.md": []byte("---\nname: e2e-test-engineer\ndescription: Writes E2E specs.\ntools: Read, Grep\n---\n\nYou write E2E tests.\n"),
		".claude/agents/bad.md":               []byte("---\nname: [unclosed\n---\nBody\n"),
		".claude/skills/deploy/SKILL.md":      []byte("---\ndescription: Deploy the app\n---\n# Deploy\n"),
		".claude/skills/deploy/reference.md":  []byte("not a skill"),
		".claude/rules/go.md":                 []byte("---\npaths:\n  - \"**/*.go\"\n---\n# Go rules\n\nUse -mod=mod.\n"),
		".claude/commands/release-check.md":   []byte("Check the release.\r\n"),
		"CLAUDE.md":                           []byte("# Project\n\nRead AGENTS.md first.\n"),
		"AGENTS.md":                           []byte("# Agents guide\n"),
		"tests/api/CLAUDE.md":                 []byte("Local rules\n"),
		"node_modules/pkg/CLAUDE.md":          []byte("vendored"),
		"docs/modules/A-core.md":              []byte("# A"),
	}
	got, problems := ParseAssistants(files)
	by := map[string]Assistant{}
	for _, a := range got {
		by[a.Path] = a
		if len(a.Code) > 64 || !CodeRe.MatchString(a.Code) {
			t.Errorf("bad code %q", a.Code)
		}
	}
	if len(got) != 8 {
		t.Fatalf("want 8 assistant files, got %d: %v", len(got), by)
	}
	if a := by[".claude/agents/e2e-test-engineer.md"]; a.Kind != KindAgent || a.Name != "e2e-test-engineer" || a.Description != "Writes E2E specs." || !strings.Contains(string(a.Frontmatter), `"tools":"Read, Grep"`) {
		t.Errorf("agent: %+v", a)
	}
	if a := by[".claude/agents/bad.md"]; a.Name != "bad" || len(problems) != 1 || !strings.Contains(problems[0].Msg, "not valid YAML") {
		t.Errorf("bad yaml: %+v %v", a, problems)
	}
	if a := by[".claude/skills/deploy/SKILL.md"]; a.Kind != KindSkill || a.Name != "deploy" || a.Description != "Deploy the app" {
		t.Errorf("skill: %+v", a)
	}
	var fm map[string]any
	_ = json.Unmarshal(by[".claude/rules/go.md"].Frontmatter, &fm)
	if a := by[".claude/rules/go.md"]; a.Kind != KindRule || a.Description != "Use -mod=mod." || fm["paths"] == nil {
		t.Errorf("rule: %+v", a)
	}
	if a := by[".claude/commands/release-check.md"]; a.Kind != KindCommand || a.Body != "Check the release.\n" {
		t.Errorf("command: %+v", a)
	}
	if a := by["tests/api/CLAUDE.md"]; a.Kind != KindInstructions || a.Name != "tests/api/CLAUDE.md" {
		t.Errorf("nested instructions: %+v", a)
	}
	if a := by["CLAUDE.md"]; a.Name != "CLAUDE.md" || a.Description != "Read AGENTS.md first." {
		t.Errorf("root instructions: %+v", a)
	}
	if !KeepForSync("docs/x.md") || !KeepForSync(".claude/agents/a.md") || KeepForSync("src/a.ts") || KeepForSync("node_modules/x/AGENTS.md") {
		t.Error("KeepForSync")
	}
}
