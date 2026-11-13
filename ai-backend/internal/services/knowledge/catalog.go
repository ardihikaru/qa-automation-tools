// Package knowledge imports a product's catalog (parts, modules, sub-modules, features, epics,
// user stories, test cases) from its automation repo into qa_automation, and serves it back.
// Spec: docs/designs/PRODUCT_KNOWLEDGE_SYNC.md.
package knowledge

import (
	"sort"
)

// Entity type names, as used in kb_annotations.entity_type, the diff and the API.
const (
	TypePart      = "part"
	TypeModule    = "module"
	TypeSubmodule = "submodule"
	TypeFeature   = "feature"
	TypeEpic      = "epic"
	TypeStory     = "story"
	TypeTestCase  = "test_case"
)

// EntityTypes lists the entity types in dependency (sync) order.
var EntityTypes = []string{TypePart, TypeModule, TypeSubmodule, TypeFeature, TypeEpic, TypeStory, TypeTestCase}

// Part is a top-level product area (one docs/modules/[A-D]-*.md file).
type Part struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	SourceFile string `json:"source_file"`
}

// Module is a "### A01 — Name" heading.
type Module struct {
	Code       string `json:"code"`
	PartCode   string `json:"part_code"`
	Name       string `json:"name"`
	SourceFile string `json:"source_file"`
	SourceLine int    `json:"source_line"`
}

// Submodule is derived from the Sub-module cell of feature rows.
type Submodule struct {
	Code       string `json:"code"`
	ModuleCode string `json:"module_code"`
	Name       string `json:"name"`
}

// Feature is one feature row of a module table.
type Feature struct {
	Code          string `json:"code"`
	ModuleCode    string `json:"module_code"`
	SubmoduleCode string `json:"submodule_code"`
	Name          string `json:"name"`
	RepoEntry     string `json:"repo_entry"`
	ReleaseStatus string `json:"release_status"`
	StatusAsOf    string `json:"status_as_of"`
	Evidence      string `json:"evidence"`
	Notes         string `json:"notes"`
	SourceFile    string `json:"source_file"`
	SourceLine    int    `json:"source_line"`
}

// Epic is one epic file of the test catalog.
type Epic struct {
	Code       string `json:"code"`
	Title      string `json:"title"`
	Service    string `json:"service"`
	Area       string `json:"area"`
	SourceFile string `json:"source_file"`
}

// Story is one user story line.
type Story struct {
	Code           string   `json:"code"`
	EpicCode       string   `json:"epic_code"`
	Text           string   `json:"text"`
	UnmappedReason string   `json:"unmapped_reason"`
	FeatureCodes   []string `json:"feature_codes"`
	SourceFile     string   `json:"source_file"`
	SourceLine     int      `json:"source_line"`
}

// TestCase is one test case row (the scenario is its Given/When/Then cell).
type TestCase struct {
	Code           string   `json:"code"`
	EpicCode       string   `json:"epic_code"`
	StoryCode      string   `json:"story_code"`
	Scenario       string   `json:"scenario"`
	Polarity       string   `json:"polarity"`
	Level          string   `json:"level"`
	Priority       string   `json:"priority"`
	Status         string   `json:"status"`
	Automated      bool     `json:"automated"`
	SpecPaths      []string `json:"spec_paths"`
	ReasonCategory string   `json:"reason_category"`
	ReasonDetail   string   `json:"reason_detail"`
	SourceFile     string   `json:"source_file"`
	SourceLine     int      `json:"source_line"`
}

// Snapshot is one parsed catalog. Every slice is sorted by code and has unique codes.
type Snapshot struct {
	Parts      []Part
	Modules    []Module
	Submodules []Submodule
	Features   []Feature
	Epics      []Epic
	Stories    []Story
	TestCases  []TestCase
}

// Problem is a non-fatal parse finding.
type Problem struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

// Format parses one catalog layout. Version must change whenever the parse output for the
// same input changes, so the scheduler re-syncs an unchanged commit after a parser upgrade.
type Format interface {
	Name() string
	Version() int
	Parse(files map[string][]byte) (*Snapshot, []Problem, error)
}

var formats = map[string]Format{}

func register(f Format) { formats[f.Name()] = f }

// Lookup returns the registered format with that name.
func Lookup(name string) (Format, bool) {
	f, ok := formats[name]
	return f, ok
}

// Names lists the registered format names, sorted.
func Names() []string {
	out := make([]string, 0, len(formats))
	for n := range formats {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Enum values of the catalog (spec "Enums").
var (
	ReleaseStatuses  = []string{"PROD", "DEV-DONE", "IN-PROGRESS", "NOT-STARTED", "UNKNOWN"}
	Polarities       = []string{"+", "-"}
	Levels           = []string{"API", "GQL", "WEB", "E2E"}
	Priorities       = []string{"P0", "P1", "P2", "P3"}
	CaseStatuses     = []string{"planned", "automated", "blocked", "manual-only", "wont"}
	ReasonCategories = []string{"defect", "not-needed", "blocked", "unsafe", "manual-only", "deferred", "other"}
	// NeedsReason lists the statuses that must state a reason.
	NeedsReason = []string{"blocked", "manual-only", "wont"}
)

// Enums returns the enum lists for the API.
func Enums() map[string][]string {
	return map[string][]string{
		"release":         ReleaseStatuses,
		"polarity":        Polarities,
		"level":           Levels,
		"priority":        Priorities,
		"status":          CaseStatuses,
		"reason_category": ReasonCategories,
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
