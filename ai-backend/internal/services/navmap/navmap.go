// Package navmap builds a product's route and page inventory from its test catalog and its
// automation repo (NAVIGATION_MAP.md). It is pure: no database, clock or I/O. Build is
// deterministic: the same input gives the same output, sorted.
package navmap

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Version changes whenever Build's output for the same input changes; a stored inventory with
// another version is rebuilt on the next (even skipped) sync.
const Version = 1

// Surfaces.
const (
	SurfaceWeb = "web"
	SurfaceAPI = "api"
)

// Link reference types.
const (
	RefFeature = "feature"
	RefCase    = "case"
	RefSpec    = "spec"
)

// Link sources. Direct case evidence is SourceScenario and SourceSpec; SourceFeature is context.
const (
	SourceCatalog  = "catalog"  // feature -> route, from the module catalogue
	SourceScenario = "scenario" // case -> route, from the case's scenario text
	SourceSpec     = "spec"     // case -> route, through its spec files
	SourceFeature  = "feature"  // case -> route, through its story's features (context only)
	SourceRepo     = "repo"     // spec -> route; also a route defined by a page object
)

// Feature is the part of a catalog feature the map reads.
type Feature struct{ Code, Name, RepoEntry string }

// Epic is the part of a catalog epic the map reads (Service: the first folder of the epic file).
type Epic struct{ Code, Service string }

// Story links a story to its features.
type Story struct {
	Code         string
	FeatureCodes []string
}

// Case is the part of a test case the map reads.
type Case struct {
	Code, EpicCode, StoryCode, Scenario, Polarity string
	SpecPaths                                     []string
}

// Input is everything Build reads. Files are repo files by path; only the folders of
// NAVIGATION_MAP.md 3.3 are looked at.
type Input struct {
	Features []Feature
	Epics    []Epic
	Stories  []Story
	Cases    []Case
	Files    map[string][]byte
}

// Route is one page or REST route.
type Route struct {
	Key         string   `json:"key"`
	Surface     string   `json:"surface"`
	App         string   `json:"app"`
	Path        string   `json:"path"`
	Methods     []string `json:"methods"`
	Sources     []string `json:"sources"`
	PageObjects []string `json:"page_objects"`
}

// Link ties a feature, a case or a spec to a route.
type Link struct {
	RouteKey string `json:"route_key"`
	RefType  string `json:"ref_type"`
	Ref      string `json:"ref"`
	Source   string `json:"source"`
	Method   string `json:"method"`
}

// Inventory is Build's output.
type Inventory struct {
	Routes   []Route
	Links    []Link
	Problems []string
}

// ref is one raw finding before the mount prefix is applied and routes are merged.
type ref struct {
	surface, app, path, method string
	source, refType, refID     string // refType "" = the route only (a page object, a client route)
	pageObject                 string
}

var methodWords = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true}

// Build extracts the inventory (NAVIGATION_MAP.md section 3).
func Build(in Input) Inventory {
	b := &builder{}
	repo := scanRepo(in.Files, b)
	known := map[string]bool{}
	for a := range repo.apps {
		known[a] = true
	}
	featureRefs := b.featureRefs(in.Features, known)
	for _, r := range featureRefs {
		known[r.app] = true
	}
	b.refs = append(b.refs, featureRefs...)
	b.scenarioRefs(in, known)
	b.repoRefs(in.Cases, repo)
	b.applyMounts()
	return b.finish(in)
}

type builder struct {
	refs     []ref
	problems []string
}

func (b *builder) problem(format string, a ...any) {
	b.problems = append(b.problems, fmt.Sprintf(format, a...))
}

var (
	backtickRe     = regexp.MustCompile("`([^`]+)`")
	frontendTokRe  = regexp.MustCompile(`\b([a-z0-9-]+-frontend)\b`)
	firstTokenRe   = regexp.MustCompile(`^\s*([a-z0-9][a-z0-9-]*)`)
	methodSpanRe   = regexp.MustCompile(`^(?:(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+)?(/\S*)$`)
	scenarioPathRe = regexp.MustCompile("\\b(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\\s+`?(/[^\\s`),;—]*)")
)

// featureRefs: feature -> route links from repo_entry and name (3.2).
func (b *builder) featureRefs(features []Feature, known map[string]bool) []ref {
	var out []ref
	for _, f := range features {
		spans := append(backtickRe.FindAllStringSubmatch(f.Name, -1), backtickRe.FindAllStringSubmatch(f.RepoEntry, -1)...)
		if len(spans) == 0 {
			continue
		}
		if m := frontendTokRe.FindStringSubmatch(f.RepoEntry); m != nil {
			for _, s := range spans {
				t := strings.TrimSpace(s[1])
				if !strings.HasPrefix(t, "/") || strings.HasPrefix(t, "/api/") {
					continue
				}
				if p := NormPath(t); p != "" {
					out = append(out, ref{surface: SurfaceWeb, app: m[1], path: p, source: SourceCatalog, refType: RefFeature, refID: f.Code})
				}
			}
			continue
		}
		m := firstTokenRe.FindStringSubmatch(f.RepoEntry)
		if m == nil {
			continue
		}
		for _, s := range spans {
			sm := methodSpanRe.FindStringSubmatch(strings.TrimSpace(s[1]))
			if sm == nil {
				continue
			}
			if p := NormPath(sm[2]); p != "" {
				out = append(out, ref{surface: SurfaceAPI, app: m[1], path: p, method: sm[1], source: SourceCatalog, refType: RefFeature, refID: f.Code})
			}
		}
	}
	return out
}

// appOfService maps an epic's service folder to a known app (3.2).
func appOfService(svc string, known map[string]bool) string {
	switch {
	case svc == "":
		return ""
	case known[svc]:
		return svc
	}
	// A service folder may name an app without its product prefix ("vendor-backend" for
	// "acme-vendor-backend"); accept the one known app that ends in "-"+svc.
	match := ""
	for app := range known {
		if strings.HasSuffix(app, "-"+svc) {
			if match != "" {
				return svc // ambiguous
			}
			match = app
		}
	}
	if match != "" {
		return match
	}
	return svc
}

// scenarioRefs: the first "METHOD /path" of each case's scenario (3.2).
func (b *builder) scenarioRefs(in Input, known map[string]bool) {
	service := map[string]string{}
	for _, e := range in.Epics {
		service[e.Code] = e.Service
	}
	for _, c := range in.Cases {
		m := scenarioPathRe.FindStringSubmatch(c.Scenario)
		if m == nil {
			continue
		}
		app := appOfService(service[c.EpicCode], known)
		p := NormPath(m[2])
		if app == "" || p == "" {
			continue
		}
		b.refs = append(b.refs, ref{surface: SurfaceAPI, app: app, path: p, method: m[1], source: SourceScenario, refType: RefCase, refID: c.Code})
	}
}

// repoRefs: page objects and client routes, spec -> route and case -> route through specs (3.3).
func (b *builder) repoRefs(cases []Case, repo *repoScan) {
	b.refs = append(b.refs, repo.routes...)
	specs := make([]string, 0, len(repo.specs))
	for s := range repo.specs {
		specs = append(specs, s)
	}
	sort.Strings(specs)
	for _, s := range specs {
		u := repo.specs[s]
		for _, r := range u.all() {
			r.source, r.refType, r.refID = SourceRepo, RefSpec, s
			b.refs = append(b.refs, r)
		}
	}
	for _, c := range cases {
		for _, s := range c.SpecPaths {
			u, ok := repo.specs[s]
			if !ok {
				continue
			}
			for _, r := range u.forCase(c.Code) {
				r.source, r.refType, r.refID = SourceSpec, RefCase, c.Code
				b.refs = append(b.refs, r)
			}
		}
	}
}

// mountOf returns "/api/vN" when p starts with it.
var mountRe = regexp.MustCompile(`^(/api/v[0-9]+)(?:/|$)`)

// applyMounts prepends each api app's mount to its routes that do not start with /api/ (3.4).
func (b *builder) applyMounts() {
	count := map[string]map[string]int{}
	for _, r := range b.refs {
		if r.surface != SurfaceAPI {
			continue
		}
		if m := mountRe.FindStringSubmatch(r.path); m != nil {
			if count[r.app] == nil {
				count[r.app] = map[string]int{}
			}
			count[r.app][m[1]]++
		}
	}
	mount := map[string]string{}
	for app, c := range count {
		best, n := "", 0
		for m, k := range c {
			if k > n || (k == n && m < best) {
				best, n = m, k
			}
		}
		mount[app] = best
	}
	for i, r := range b.refs {
		if r.surface != SurfaceAPI || mount[r.app] == "" || strings.HasPrefix(r.path, "/api/") {
			continue
		}
		p := mount[r.app]
		if r.path != "/" {
			p += r.path
		}
		if len(p) > MaxPathLen {
			continue
		}
		b.refs[i].path = p
	}
}

type routeAcc struct {
	Route
	methods, sources, pages map[string]bool
}

// finish merges refs into routes and links, drops negative probes and adds feature context links.
func (b *builder) finish(in Input) Inventory {
	routes := map[string]*routeAcc{}
	links := map[Link]bool{}
	for _, r := range b.refs {
		if r.path == "" || r.app == "" {
			continue
		}
		k := RouteKey(r.surface, r.app, r.path)
		acc := routes[k]
		if acc == nil {
			acc = &routeAcc{Route: Route{Key: k, Surface: r.surface, App: r.app, Path: r.path}, methods: map[string]bool{}, sources: map[string]bool{}, pages: map[string]bool{}}
			routes[k] = acc
		}
		if r.method != "" && r.surface == SurfaceAPI {
			acc.methods[r.method] = true
		}
		if r.pageObject != "" {
			acc.pages[r.pageObject] = true
		}
		switch r.source {
		case SourceCatalog:
			acc.sources[SourceCatalog] = true
		case SourceScenario:
			acc.sources[SourceScenario] = true
		case SourceRepo, SourceSpec:
			acc.sources[SourceRepo] = true
		}
		if r.refType != "" {
			links[Link{RouteKey: k, RefType: r.refType, Ref: r.refID, Source: r.source, Method: r.method}] = true
		}
	}

	// Negative probes: a route known only from scenarios of negative cases.
	polarity := map[string]string{}
	for _, c := range in.Cases {
		polarity[c.Code] = c.Polarity
	}
	positive := map[string]bool{}
	for l := range links {
		if l.Source == SourceScenario && polarity[l.Ref] != "-" {
			positive[l.RouteKey] = true
		}
	}
	for k, acc := range routes {
		if len(acc.sources) == 1 && acc.sources[SourceScenario] && !positive[k] {
			delete(routes, k)
		}
	}
	for l := range links {
		if routes[l.RouteKey] == nil {
			delete(links, l)
		}
	}

	// Case -> route through the story's features (context).
	featureRoutes := map[string][]string{}
	for l := range links {
		if l.RefType == RefFeature {
			featureRoutes[l.Ref] = append(featureRoutes[l.Ref], l.RouteKey)
		}
	}
	storyFeatures := map[string][]string{}
	for _, s := range in.Stories {
		storyFeatures[s.Code] = s.FeatureCodes
	}
	for _, c := range in.Cases {
		for _, f := range storyFeatures[c.StoryCode] {
			for _, k := range featureRoutes[f] {
				links[Link{RouteKey: k, RefType: RefCase, Ref: c.Code, Source: SourceFeature}] = true
			}
		}
	}

	inv := Inventory{Problems: b.problems}
	for _, acc := range routes {
		acc.Methods, acc.Sources, acc.PageObjects = sortedKeys(acc.methods), sortedKeys(acc.sources), sortedKeys(acc.pages)
		inv.Routes = append(inv.Routes, acc.Route)
	}
	sort.Slice(inv.Routes, func(i, j int) bool {
		a, c := inv.Routes[i], inv.Routes[j]
		if a.Surface != c.Surface {
			return a.Surface < c.Surface
		}
		if a.App != c.App {
			return a.App < c.App
		}
		return a.Path < c.Path
	})
	for l := range links {
		inv.Links = append(inv.Links, l)
	}
	sort.Slice(inv.Links, func(i, j int) bool { return linkLess(inv.Links[i], inv.Links[j]) })
	sort.Strings(inv.Problems)
	return inv
}

func linkLess(a, b Link) bool {
	if a.RouteKey != b.RouteKey {
		return a.RouteKey < b.RouteKey
	}
	if a.RefType != b.RefType {
		return a.RefType < b.RefType
	}
	if a.Ref != b.Ref {
		return a.Ref < b.Ref
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	return a.Method < b.Method
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Direct reports whether a case link source is evidence (3.6).
func Direct(source string) bool { return source == SourceScenario || source == SourceSpec }
