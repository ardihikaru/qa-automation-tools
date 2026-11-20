package navmap

import (
	"regexp"
	"sort"
	"strings"

	"github.com/qa-automation/ai-backend/internal/services/caseresults"
)

// MaxFileBytes is the largest repo file the scan reads; a bigger one is skipped with a problem.
const MaxFileBytes = 256 << 10

// Repo folders the scan reads (KeepRepoFile is the sync's filter for them).
var repoPrefixes = []string{"src/web/", "src/api/rest/", "src/fixtures/", "tests/web/", "tests/api/rest/"}

// KeepRepoFile reports whether the sync must read a repo file for the map.
func KeepRepoFile(p string) bool {
	if !strings.HasSuffix(p, ".ts") {
		return false
	}
	for _, pre := range repoPrefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

var (
	classRe       = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+(\w+)`)
	pagePathRe    = regexp.MustCompile("(?:protected|public|private)?\\s*(?:override\\s+)?readonly\\s+path(?:\\s*:\\s*string)?\\s*=\\s*['\"`]([^'\"`]+)['\"`]")
	memberRe      = regexp.MustCompile(`readonly\s+(\w+)\s*:\s*(\w+)\s*;`)
	fixtureRe     = regexp.MustCompile(`(?m)^\s{2,}(\w+)\s*:\s*async\s*\(`)
	useNewRe      = regexp.MustCompile(`use\(\s*new\s+(\w+)\(`)
	gotoRe        = regexp.MustCompile("goto\\(\\s*['\"`]([^'\"`]+)['\"`]")
	testStartRe   = regexp.MustCompile("\\btest(?:\\.only|\\.skip|\\.fixme)?\\(\\s*(['\"`])")
	destructureRe = regexp.MustCompile(`async\s*\(\s*\{([^}]*)\}`)
	methodStartRe = regexp.MustCompile(`^  ((?:(?:public|private|protected|static|async|readonly|override)\s+)*)(\w+)\s*(?:<[^>()]*>)?\s*\(`)
	jsdocRouteRe  = regexp.MustCompile("`((?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)(?:\\|(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS))*)\\s+(/[^`\\s]*)`")
	thisCallRe    = regexp.MustCompile("this\\.(get|post|put|patch|delete|url)\\(\\s*['\"`]([^'\"`]+)")
	envVerbURLRe  = regexp.MustCompile("\\.(get|post|put|patch|delete|fetch)\\(\\s*`\\$\\{env\\.\\w+\\}(/[^`'\"\\s?]*)")
	envURLRe      = regexp.MustCompile("\\$\\{env\\.\\w+\\}(/[^`'\"\\s?]*)")
)

var notMethods = map[string]bool{"constructor": true, "super": true, "if": true, "for": true, "while": true, "switch": true, "return": true, "catch": true, "function": true}

// Generic names a spec cannot be credited with (they are the transport, not an endpoint).
var genericClientMethods = map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true, "fetch": true, "url": true, "call": true, "request": true}

// usage is what one spec uses: routes of the shared text and of each test block.
type usage struct {
	shared []ref
	blocks []block
}

type block struct {
	codes  []string
	routes []ref
}

func (u *usage) all() []ref {
	out := append([]ref{}, u.shared...)
	for _, b := range u.blocks {
		out = append(out, b.routes...)
	}
	return out
}

// forCase returns the routes of the blocks titled with the case plus the shared ones; when no block
// names the case, every route of the spec (3.3).
func (u *usage) forCase(code string) []ref {
	var out []ref
	named := false
	for _, b := range u.blocks {
		for _, c := range b.codes {
			if c == code {
				named = true
				out = append(out, b.routes...)
			}
		}
	}
	if !named {
		return u.all()
	}
	return append(out, u.shared...)
}

type repoScan struct {
	apps   map[string]bool
	routes []ref             // page objects and client routes (no link)
	specs  map[string]*usage // spec path -> usage
}

type clientMethod struct {
	name string
	defs []ref // the route with each of its methods (route records)
	uses []ref // one per path, method only when there is exactly one (link records)
}

// scanRepo reads the automation repo files (3.3).
func scanRepo(files map[string][]byte, b *builder) *repoScan {
	rs := &repoScan{apps: map[string]bool{}, specs: map[string]*usage{}}
	paths := make([]string, 0, len(files))
	for p := range files {
		if KeepRepoFile(p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	text := map[string]string{}
	for _, p := range paths {
		if len(files[p]) > MaxFileBytes {
			b.problem("%s: skipped, larger than %d bytes", p, MaxFileBytes)
			continue
		}
		text[p] = string(files[p])
	}

	// Page objects and members, per web app.
	pageClass := map[string]map[string]string{} // app -> class -> path ("" for none)
	classApp := map[string]string{}
	members := map[string]map[string]string{} // app -> member -> class
	for _, p := range paths {
		app, ok := folderApp(p, "src/web/")
		if !ok || text[p] == "" {
			continue
		}
		if pageClass[app] == nil {
			pageClass[app] = map[string]string{}
		}
		cur := ""
		for _, line := range strings.Split(text[p], "\n") {
			if m := classRe.FindStringSubmatch(line); m != nil {
				cur = m[1]
				if _, seen := pageClass[app][cur]; !seen {
					pageClass[app][cur] = ""
				}
				if _, seen := classApp[cur]; !seen {
					classApp[cur] = app
				}
				continue
			}
			if m := pagePathRe.FindStringSubmatch(line); m != nil && cur != "" {
				if np := NormPath(m[1]); np != "" {
					pageClass[app][cur] = np
					rs.routes = append(rs.routes, ref{surface: SurfaceWeb, app: app, path: np, source: SourceRepo, pageObject: cur})
				}
			}
		}
	}
	for _, p := range paths {
		app, ok := folderApp(p, "src/web/")
		if !ok {
			continue
		}
		rs.apps[app] = true
		for _, m := range memberRe.FindAllStringSubmatch(text[p], -1) {
			if pageClass[app][m[2]] == "" {
				continue
			}
			if members[app] == nil {
				members[app] = map[string]string{}
			}
			if old, ok := members[app][m[1]]; !ok || m[2] < old {
				members[app][m[1]] = m[2]
			}
		}
		// Literal navigations in page objects are routes of the app.
		for _, m := range gotoRe.FindAllStringSubmatch(text[p], -1) {
			if np := NormPath(m[1]); np != "" {
				rs.routes = append(rs.routes, ref{surface: SurfaceWeb, app: app, path: np, source: SourceRepo})
			}
		}
	}

	// Fixtures: name -> class.
	fixtures := map[string]string{}
	for _, p := range paths {
		if !strings.HasPrefix(p, "src/fixtures/") {
			continue
		}
		t := text[p]
		for _, loc := range fixtureRe.FindAllStringSubmatchIndex(t, -1) {
			name := t[loc[2]:loc[3]]
			end := loc[1] + 400
			if end > len(t) {
				end = len(t)
			}
			if m := useNewRe.FindStringSubmatch(t[loc[1]:end]); m != nil {
				if _, ok := fixtures[name]; !ok {
					fixtures[name] = m[1]
				}
			}
		}
	}

	// REST clients: service -> method -> routes.
	clients := map[string]map[string][]ref{}
	for _, p := range paths {
		svc, ok := folderApp(p, "src/api/rest/")
		if !ok || text[p] == "" {
			continue
		}
		rs.apps[svc] = true
		for _, cm := range clientMethods(text[p], svc) {
			if clients[svc] == nil {
				clients[svc] = map[string][]ref{}
			}
			clients[svc][cm.name] = append(clients[svc][cm.name], cm.uses...)
			rs.routes = append(rs.routes, cm.defs...)
		}
	}

	// Web specs.
	for _, p := range paths {
		app, ok := folderApp(p, "tests/web/")
		if !ok || !strings.HasSuffix(p, ".spec.ts") || text[p] == "" {
			continue
		}
		rs.apps[app] = true
		pages := pageClass[app]
		memberRes := map[string]*regexp.Regexp{}
		for mname := range members[app] {
			memberRes[mname] = regexp.MustCompile(`\.` + regexp.QuoteMeta(mname) + `\b`)
		}
		uses := func(t string) []ref {
			var out []ref
			used := map[string]bool{}
			for c, path := range pages {
				if path != "" && strings.Contains(t, "new "+c+"(") {
					used[c] = true
				}
			}
			for mname, c := range members[app] {
				if memberRes[mname].MatchString(t) {
					used[c] = true
				}
			}
			for _, d := range destructureRe.FindAllStringSubmatch(t, -1) {
				for _, part := range strings.Split(d[1], ",") {
					name := strings.TrimSpace(strings.SplitN(part, ":", 2)[0])
					if c, ok := fixtures[name]; ok && pages[c] != "" {
						used[c] = true
					}
				}
			}
			for c := range used {
				out = append(out, ref{surface: SurfaceWeb, app: app, path: pages[c], pageObject: c})
			}
			for _, m := range gotoRe.FindAllStringSubmatch(t, -1) {
				if np := NormPath(m[1]); np != "" {
					out = append(out, ref{surface: SurfaceWeb, app: app, path: np})
				}
			}
			return sortRefs(out)
		}
		rs.specs[p] = splitSpec(text[p], uses)
	}

	// API specs.
	for _, p := range paths {
		svc, ok := folderApp(p, "tests/api/rest/")
		if !ok || !strings.HasSuffix(p, ".spec.ts") || text[p] == "" {
			continue
		}
		rs.apps[svc] = true
		methods := clients[svc]
		uses := func(t string) []ref {
			var out []ref
			for name, routes := range methods {
				if genericClientMethods[name] || len(routes) == 0 {
					continue
				}
				if strings.Contains(t, "."+name+"(") {
					out = append(out, routes...)
				}
			}
			seen := map[string]bool{}
			for _, m := range envVerbURLRe.FindAllStringSubmatch(t, -1) {
				if np := NormPath(m[2]); np != "" {
					method := strings.ToUpper(m[1])
					if method == "FETCH" {
						method = ""
					}
					seen[np] = true
					out = append(out, ref{surface: SurfaceAPI, app: svc, path: np, method: method})
				}
			}
			for _, m := range envURLRe.FindAllStringSubmatch(t, -1) {
				if np := NormPath(m[1]); np != "" && !seen[np] {
					out = append(out, ref{surface: SurfaceAPI, app: svc, path: np})
				}
			}
			return sortRefs(out)
		}
		rs.specs[p] = splitSpec(text[p], uses)
	}
	return rs
}

// folderApp returns the folder right under prefix ("src/web/sut-frontend/x.ts" -> "sut-frontend").
func folderApp(p, prefix string) (string, bool) {
	if !strings.HasPrefix(p, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(p, prefix)
	i := strings.IndexByte(rest, '/')
	if i <= 0 {
		return "", false
	}
	return rest[:i], true
}

// splitSpec cuts a spec into the shared prefix and its test blocks (3.3).
func splitSpec(t string, uses func(string) []ref) *usage {
	locs := testStartRe.FindAllStringSubmatchIndex(t, -1)
	u := &usage{}
	if len(locs) == 0 {
		u.shared = uses(t)
		return u
	}
	u.shared = uses(t[:locs[0][0]])
	for i, loc := range locs {
		end := len(t)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		body := t[loc[0]:end]
		quote := t[loc[2]:loc[3]]
		title := ""
		rest := t[loc[3]:end]
		if j := strings.Index(rest, quote); j >= 0 {
			title = rest[:j]
		}
		u.blocks = append(u.blocks, block{codes: caseresults.CaseCodes(title), routes: uses(body)})
	}
	return u
}

// clientMethods splits a client file into methods at two-space indentation and returns the public
// ones with routes: their JSDoc routes, else the paths of this.<verb>(…) calls in their body (3.3).
func clientMethods(t, svc string) []clientMethod {
	lines := strings.Split(t, "\n")
	type mstart struct {
		line    int
		name    string
		private bool
		doc     string
	}
	var starts []mstart
	docEnd, docText := -1, ""
	inDoc := false
	var cur strings.Builder
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if inDoc {
			cur.WriteString(line + "\n")
			if strings.Contains(trim, "*/") {
				inDoc, docEnd, docText = false, i, cur.String()
			}
			continue
		}
		if strings.HasPrefix(trim, "/**") {
			cur.Reset()
			cur.WriteString(line + "\n")
			if strings.Contains(trim[3:], "*/") {
				docEnd, docText = i, cur.String()
			} else {
				inDoc = true
			}
			continue
		}
		m := methodStartRe.FindStringSubmatch(line)
		if m == nil || notMethods[m[2]] {
			continue
		}
		s := mstart{line: i, name: m[2], private: strings.Contains(m[1], "private") || strings.Contains(m[1], "protected")}
		if docEnd == i-1 {
			s.doc = docText
		}
		starts = append(starts, s)
	}
	var out []clientMethod
	for k, s := range starts {
		if s.private {
			continue
		}
		end := len(lines)
		if k+1 < len(starts) {
			end = starts[k+1].line
		}
		var defs, uses []ref
		for _, m := range jsdocRouteRe.FindAllStringSubmatch(s.doc, -1) {
			p := NormPath(m[2])
			if p == "" {
				continue
			}
			methods := strings.Split(m[1], "|")
			for _, verb := range methods {
				defs = append(defs, ref{surface: SurfaceAPI, app: svc, path: p, method: verb, source: SourceRepo})
			}
			one := ""
			if len(methods) == 1 {
				one = methods[0]
			}
			uses = append(uses, ref{surface: SurfaceAPI, app: svc, path: p, method: one})
		}
		if len(defs) == 0 {
			body := strings.Join(lines[s.line:end], "\n")
			for _, m := range thisCallRe.FindAllStringSubmatch(body, -1) {
				raw := m[2]
				if !strings.HasPrefix(raw, "/") {
					raw = "/" + raw
				}
				p := NormPath(raw)
				if p == "" {
					continue
				}
				verb := strings.ToUpper(m[1])
				if verb == "URL" {
					verb = ""
				}
				defs = append(defs, ref{surface: SurfaceAPI, app: svc, path: p, method: verb, source: SourceRepo})
				uses = append(uses, ref{surface: SurfaceAPI, app: svc, path: p, method: verb})
			}
		}
		if len(defs) > 0 {
			out = append(out, clientMethod{name: s.name, defs: defs, uses: uses})
		}
	}
	return out
}

func sortRefs(rs []ref) []ref {
	sort.Slice(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if a.path != b.path {
			return a.path < b.path
		}
		if a.method != b.method {
			return a.method < b.method
		}
		return a.pageObject < b.pageObject
	})
	return rs
}
