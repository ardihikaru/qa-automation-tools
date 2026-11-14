package caseauto

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxSpecBytes is the size limit of a generated spec (CASE_AUTOMATION.md section 5, generate).
const MaxSpecBytes = 20 << 10

// CaseCodeRe is the case code shape the executor accepts too.
var CaseCodeRe = regexp.MustCompile(`^TC-[A-Z0-9]+-[0-9]{3,4}$`)

// GeneratedFile is the single file the LLM returns.
type GeneratedFile struct {
	Path    string
	Content string
}

var (
	fileLineRe  = regexp.MustCompile(`(?m)^[ \t]*FILE:[ \t]*(\S+)[ \t]*\r?$`)
	fenceRe     = regexp.MustCompile("(?s)```[A-Za-z]*[ \t]*\r?\n(.*?)\r?\n[ \t]*```")
	fenceMarkRe = regexp.MustCompile("(?m)^[ \t]*```")
	importRe    = regexp.MustCompile(`(?m)^[ \t]*(?:import|export)\s+(?:[^'"\n;]*?\s+from\s*)?['"]([^'"]+)['"]`)
	dynImportRe = regexp.MustCompile(`\b(?:import|require)\s*\(`)
	hostRe      = regexp.MustCompile(`(?i)\b(?:https?|wss?)://([^/\s'"` + "`" + `:?#\\)]+)`)
	testTitleRe = regexp.MustCompile(`\btest\s*\(\s*(?:'((?:\\.|[^'\\])*)'|"((?:\\.|[^"\\])*)"|` + "`" + `((?:\\.|[^` + "`" + `\\])*)` + "`" + `)`)
	skipRe      = regexp.MustCompile(`\.(?:only|skip|fixme|fail|slow)\b`)
)

// forbidden lists source patterns a generated spec may never contain.
var forbidden = []struct {
	re  *regexp.Regexp
	why string
}{
	{regexp.MustCompile(`console\s*\.\s*\w+\s*\([^;]*process\s*\.\s*env\b`), "logs process.env"},
	{regexp.MustCompile(`JSON\s*\.\s*stringify\s*\(\s*process\s*\.\s*env\b`), "dumps process.env"},
	{regexp.MustCompile(`Object\s*\.\s*\w+\s*\(\s*process\s*\.\s*env\b`), "enumerates process.env"},
	{regexp.MustCompile(`\.\.\.\s*process\s*\.\s*env\b`), "spreads process.env"},
	{regexp.MustCompile(`process\s*\.\s*env\s*\[`), "reads process.env by dynamic key"},
	{regexp.MustCompile(`process\s*\.\s*env\s*(?:[;,)]|$)`), "passes process.env around"},
	{regexp.MustCompile(`\bchild_process\b|\bspawn(?:Sync)?\s*\(|\bexec(?:Sync|File(?:Sync)?)\s*\(`), "runs processes"},
	{regexp.MustCompile(`\beval\s*\(|\bnew\s+Function\b`), "evaluates code"},
	{regexp.MustCompile(`process\s*\.\s*(?:exit|kill|chdir|binding|dlopen)\b`), "controls the process"},
	{regexp.MustCompile(`\bimport\.meta\b|__dirname\s*\+\s*['"]\.\.`), "escapes the repo"},
}

// ParseReply extracts the single file from the LLM reply. The reply must carry exactly one
// "FILE: <path>" line and exactly one fenced code block; prose around them is ignored.
func ParseReply(reply string) (GeneratedFile, error) {
	files := fileLineRe.FindAllStringSubmatch(reply, -1)
	if len(files) != 1 {
		return GeneratedFile{}, fmt.Errorf("the reply must contain exactly one FILE: line (found %d)", len(files))
	}
	blocks := fenceRe.FindAllStringSubmatch(reply, -1)
	if len(blocks) != 1 || len(fenceMarkRe.FindAllString(reply, -1)) != 2 {
		return GeneratedFile{}, errors.New("the reply must contain exactly one fenced code block")
	}
	return GeneratedFile{Path: files[0][1], Content: blocks[0][1] + "\n"}, nil
}

// PathRe returns the only path pattern a generated spec may have for code.
func PathRe(code string) *regexp.Regexp {
	return regexp.MustCompile(`^tests/(?:web|api/rest)/[a-z0-9][a-z0-9_-]{0,40}/` + regexp.QuoteMeta(code) + `\.spec\.ts$`)
}

// ValidateOptions are the facts a generated file is checked against.
type ValidateOptions struct {
	Code           string
	AllowedHosts   []string        // hosts a literal URL may name (the product's staging host)
	AllowedImports map[string]bool // bare module specifiers allowed besides @playwright/test
	// Fixtures are the fixture names the repo declares (src/fixtures/*.ts). When non-empty, a
	// test callback may only destructure these plus Playwright's built-ins.
	Fixtures map[string]bool
}

var (
	destructureRe   = regexp.MustCompile(`async\s*\(\s*\{([^}]*)\}`)
	fixtureDeclRe   = regexp.MustCompile(`(?m)^ {2}(\w+)\??\s*:`)
	builtinFixtures = map[string]bool{"page": true, "context": true, "browser": true, "browserName": true,
		"request": true, "baseURL": true, "playwright": true, "testInfo": true}
)

// FixtureNames extracts the fixture names declared in the source of a repo's fixture files:
// two-space-indented `name:` members of the fixtures interfaces.
func FixtureNames(srcs ...string) map[string]bool {
	out := map[string]bool{}
	for _, src := range srcs {
		for _, m := range fixtureDeclRe.FindAllStringSubmatch(src, -1) {
			out[m[1]] = true
		}
	}
	return out
}

// checkFixtures rejects a test callback that destructures a fixture the repo does not declare
// (the generator guessed one from the naming pattern).
func checkFixtures(c string, known map[string]bool) error {
	if len(known) == 0 {
		return nil
	}
	for _, m := range destructureRe.FindAllStringSubmatch(c, -1) {
		for _, part := range strings.Split(m[1], ",") {
			name := strings.TrimSpace(part)
			if name == "" || strings.HasPrefix(name, "...") {
				continue
			}
			if i := strings.IndexAny(name, ":="); i >= 0 {
				name = strings.TrimSpace(name[:i])
			}
			if !builtinFixtures[name] && !known[name] {
				return fmt.Errorf("the fixture %q does not exist in this repo; use one of the AVAILABLE FIXTURES", name)
			}
		}
	}
	return nil
}

// ImportSpecs lists the module specifiers imported by TypeScript source.
func ImportSpecs(src string) []string {
	var out []string
	for _, m := range importRe.FindAllStringSubmatch(src, -1) {
		out = append(out, m[1])
	}
	return out
}

// Validate is the strict check of CASE_AUTOMATION.md section 5 (generate): one file, the path
// pattern, at most 20 KiB, every test title starting with the case code, no process.env dump,
// no process or code execution, only vetted imports, and no URL to a host outside the allow-list.
func Validate(f GeneratedFile, o ValidateOptions) error {
	if !CaseCodeRe.MatchString(o.Code) {
		return errors.New("invalid case code")
	}
	if !PathRe(o.Code).MatchString(f.Path) || path.Clean(f.Path) != f.Path {
		return fmt.Errorf("the path must be tests/<web|api/rest>/<module>/%s.spec.ts", o.Code)
	}
	c := f.Content
	if strings.TrimSpace(c) == "" {
		return errors.New("the file is empty")
	}
	if len(c) > MaxSpecBytes {
		return fmt.Errorf("the file is larger than %d KiB", MaxSpecBytes>>10)
	}
	if !utf8.ValidString(c) || strings.ContainsRune(c, 0) {
		return errors.New("the file is not valid UTF-8 text")
	}
	if !strings.Contains(c, o.Code) {
		return errors.New("the file does not mention the case code")
	}
	if skipRe.MatchString(c) {
		return errors.New("test.only / skip / fixme / fail / slow are not allowed")
	}
	titles := testTitleRe.FindAllStringSubmatch(c, -1)
	if len(titles) == 0 {
		return errors.New("the file has no test(...) call with a literal title")
	}
	for _, t := range titles {
		title := t[1] + t[2] + t[3]
		if !strings.HasPrefix(title, o.Code) {
			return fmt.Errorf("every test title must start with %s (got %q)", o.Code, Clip(title, 60))
		}
	}
	for _, fb := range forbidden {
		if fb.re.MatchString(c) {
			return fmt.Errorf("the file %s", fb.why)
		}
	}
	if dynImportRe.MatchString(c) {
		return errors.New("dynamic import and require are not allowed")
	}
	dir := path.Dir(f.Path)
	for _, spec := range ImportSpecs(c) {
		switch {
		case spec == "@playwright/test", spec == "@config/targets", o.AllowedImports[spec]:
		case strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../"):
			if strings.HasPrefix(path.Join(dir, spec), "..") {
				return fmt.Errorf("import %q leaves the repo", spec)
			}
		default:
			return fmt.Errorf("import %q is not allowed", spec)
		}
	}
	if err := checkFixtures(c, o.Fixtures); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, h := range o.AllowedHosts {
		allowed[strings.ToLower(h)] = true
	}
	for _, m := range hostRe.FindAllStringSubmatch(c, -1) {
		if !allowed[strings.ToLower(m[1])] {
			return fmt.Errorf("the file names the host %q, which is not the product's staging host", Clip(m[1], 60))
		}
	}
	return nil
}
