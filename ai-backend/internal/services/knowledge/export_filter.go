package knowledge

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Spec: docs/designs/MODULE_CHECKLIST_EXPORT.md, "Scope and Prod since".

// Export scopes.
const (
	ScopeAll  = "all"
	ScopeProd = "prod"
)

// ProdSince is a parsed "Prod since" cell. Months are "YYYY-MM"; From == "" means open start
// ("<YYYY-MM": before To's next month), Unknown means "?" (in production, date unknown).
type ProdSince struct {
	From, To string
	Unknown  bool
}

var (
	monthRe     = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
	errProdFrom = errors.New(`"Prod since" must be YYYY-MM, YYYY-MM..YYYY-MM, <YYYY-MM or ?`)
)

// ValidMonth reports whether s is a "YYYY-MM" month.
func ValidMonth(s string) bool { return monthRe.MatchString(s) }

// prevMonth returns the month before m ("2026-05" -> "2026-04").
func prevMonth(m string) string {
	var y, mo int
	fmt.Sscanf(m, "%d-%d", &y, &mo)
	mo--
	if mo == 0 {
		y, mo = y-1, 12
	}
	return fmt.Sprintf("%04d-%02d", y, mo)
}

// ParseProdSince parses a non-empty "Prod since" cell.
func ParseProdSince(s string) (ProdSince, error) {
	switch {
	case s == "?":
		return ProdSince{Unknown: true}, nil
	case strings.HasPrefix(s, "<"):
		if m := s[1:]; ValidMonth(m) {
			return ProdSince{To: prevMonth(m)}, nil
		}
	case strings.Contains(s, ".."):
		a, b, _ := strings.Cut(s, "..")
		if ValidMonth(a) && ValidMonth(b) {
			if a > b {
				return ProdSince{}, fmt.Errorf(`"Prod since" range %q is reversed`, s)
			}
			return ProdSince{From: a, To: b}, nil
		}
	case ValidMonth(s):
		return ProdSince{From: s, To: s}, nil
	}
	return ProdSince{}, errProdFrom
}

// ChecklistFilter selects the rows of an export. From/To are "YYYY-MM" or "" (open); a range is
// only meaningful with ScopeProd.
type ChecklistFilter struct {
	Scope    string // ScopeAll ("" means the same) or ScopeProd
	From, To string
	// UndatedOnly keeps only the PROD rows without a known Prod since period (?, empty or
	// unparsable). ScopeProd only, never together with a range.
	UndatedOnly bool
	// HideCheck leaves out the Cek (manual tick box) column in every format. Any scope.
	HideCheck bool
}

// Ranged reports whether the filter has a Prod since range.
func (f ChecklistFilter) Ranged() bool { return f.From != "" || f.To != "" }

// variant is the filter's part of the cache key; "" for the full export (keeps old keys valid).
func (f ChecklistFilter) variant() string {
	v := f.scopeVariant()
	if f.HideCheck {
		v += "/nocheck"
	}
	return v
}

func (f ChecklistFilter) scopeVariant() string {
	if f.Scope != ScopeProd {
		return ""
	}
	v := "/prod"
	if f.UndatedOnly {
		return v + "-undated"
	}
	if f.Ranged() {
		v += "-" + strings.ReplaceAll(orDash(f.From), "-", "") + "-" + strings.ReplaceAll(orDash(f.To), "-", "")
	}
	return v
}

func orDash(s string) string {
	if s == "" {
		return "x"
	}
	return s
}

// overlaps reports whether a known period [p.From, p.To] overlaps [f.From, f.To] (open ends allowed).
func (f ChecklistFilter) overlaps(p ProdSince) bool {
	return (p.From == "" || f.To == "" || p.From <= f.To) && (f.From == "" || p.To >= f.From)
}

// FilterChecklist returns a copy of snap holding only the rows the filter selects. With ScopeProd
// only PROD rows stay; with a range only PROD rows whose period overlaps it stay, and PROD rows with
// an unknown or missing period are counted in ExcludedUnknown. Empty modules and parts are dropped.
func FilterChecklist(snap *ChecklistSnapshot, f ChecklistFilter) *ChecklistSnapshot {
	out := *snap
	out.Filter = f
	if f.Scope != ScopeProd {
		out.Filter.Scope = ScopeAll
		return &out
	}
	out.Parts = nil
	out.ExcludedUnknown = 0
	for _, p := range snap.Parts {
		np := ChecklistPart{Code: p.Code, Name: p.Name}
		for _, m := range p.Modules {
			nm := ChecklistModule{Code: m.Code, Name: m.Name}
			for _, r := range m.Rows {
				if r.Status != "PROD" {
					continue
				}
				if f.UndatedOnly {
					if ps, err := ParseProdSince(r.ProdSince); r.ProdSince != "" && err == nil && !ps.Unknown {
						continue
					}
				} else if f.Ranged() {
					ps, err := ParseProdSince(r.ProdSince)
					// A period is a hard filter: a PROD row without a known period cannot be shown
					// to fall inside it, so it is left out (and counted). Without a period every
					// PROD row is listed, undated ones as "?".
					if r.ProdSince == "" || err != nil || ps.Unknown {
						out.ExcludedUnknown++
						continue
					}
					if !f.overlaps(ps) {
						continue
					}
				}
				nm.Rows = append(nm.Rows, r)
			}
			if len(nm.Rows) > 0 {
				np.Modules = append(np.Modules, nm)
			}
		}
		if len(np.Modules) > 0 {
			out.Parts = append(out.Parts, np)
		}
	}
	return &out
}

// monthID formats "2026-07" as "Jul 2026".
func monthID(m string) string {
	var y, mo int
	fmt.Sscanf(m, "%d-%d", &y, &mo)
	return fmt.Sprintf("%s %d", idMonths[mo-1], y)
}

// prodSinceText renders a raw "Prod since" cell for people: "Jul 2026", "Jul–Agu 2026",
// "Des 2025–Jan 2026", "< Mei 2026", "?" (unknown) or "" (none or unparsable).
func prodSinceText(raw string) string {
	if raw == "" {
		return ""
	}
	p, err := ParseProdSince(raw)
	switch {
	case err != nil:
		return ""
	case p.Unknown:
		return "?"
	case p.From == "":
		return "< " + monthID(raw[1:])
	case p.From == p.To:
		return monthID(p.From)
	case p.From[:4] == p.To[:4]:
		return strings.Fields(monthID(p.From))[0] + "–" + monthID(p.To)
	}
	return monthID(p.From) + "–" + monthID(p.To)
}

// scopeText describes the filter for the export header ("" for the full export).
func scopeText(snap *ChecklistSnapshot) string {
	f := snap.Filter
	if f.Scope != ScopeProd {
		return ""
	}
	if f.UndatedOnly {
		return "Hanya fitur PROD yang tanggal rilisnya (Prod sejak) belum diketahui"
	}
	if !f.Ranged() {
		return "Hanya fitur PROD"
	}
	var r string
	switch {
	case f.From != "" && f.To != "":
		r = monthID(f.From) + " s.d. " + monthID(f.To)
	case f.From != "":
		r = monthID(f.From) + " dan sesudahnya"
	default:
		r = monthID(f.To) + " dan sebelumnya"
	}
	s := "Hanya fitur PROD yang periode Prod sejak-nya beririsan dengan " + r
	if snap.ExcludedUnknown > 0 {
		s += fmt.Sprintf(" (%d fitur PROD tanpa tanggal tidak disertakan)", snap.ExcludedUnknown)
	}
	return s
}
