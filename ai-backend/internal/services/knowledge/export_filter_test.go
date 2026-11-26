package knowledge

import (
	"strings"
	"testing"
	"time"
)

func TestParseProdSince(t *testing.T) {
	ok := map[string]ProdSince{
		"2026-07":          {From: "2026-07", To: "2026-07"},
		"2026-07..2026-08": {From: "2026-07", To: "2026-08"},
		"2025-12..2026-01": {From: "2025-12", To: "2026-01"},
		"<2026-05":         {To: "2026-04"},
		"<2026-01":         {To: "2025-12"},
		"?":                {Unknown: true},
	}
	for in, want := range ok {
		got, err := ParseProdSince(in)
		if err != nil || got != want {
			t.Errorf("ParseProdSince(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "2026-13", "2026-7", "2026-08..2026-07", "<2026", "Jul 2026", "—", "2026-07..", ">2026-05"} {
		if _, err := ParseProdSince(in); err == nil {
			t.Errorf("ParseProdSince(%q) accepted", in)
		}
	}
}

func TestProdSinceText(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"2026-07":          "Jul 2026",
		"2026-07..2026-08": "Jul–Agu 2026",
		"2025-12..2026-01": "Des 2025–Jan 2026",
		"<2026-05":         "< Mei 2026",
		"?":                "?",
		"bogus":            "",
	}
	for in, want := range cases {
		if got := prodSinceText(in); got != want {
			t.Errorf("prodSinceText(%q) = %q, want %q", in, got, want)
		}
	}
}

func filterFixture() *ChecklistSnapshot {
	rows := []ChecklistRow{
		{ID: "A01.01", Status: "PROD", ProdSince: "<2026-05"},
		{ID: "A01.02", Status: "PROD", ProdSince: "2026-05..2026-06"},
		{ID: "A01.03", Status: "PROD", ProdSince: "2026-07..2026-08"},
		{ID: "A01.04", Status: "PROD", ProdSince: "2026-11"},
		{ID: "A01.05", Status: "PROD", ProdSince: "?"},
		{ID: "A01.06", Status: "PROD"}, // no Prod since column
		{ID: "A01.07", Status: "DEV-DONE"},
	}
	return &ChecklistSnapshot{ProductName: "P", Parts: []ChecklistPart{
		{Code: "A", Name: "PA", Modules: []ChecklistModule{{Code: "A01", Name: "M", Rows: rows}}},
		{Code: "B", Name: "PB", Modules: []ChecklistModule{{Code: "B01", Name: "N", Rows: []ChecklistRow{{ID: "B01.01", Status: "IN-PROGRESS"}}}}},
	}}
}

func ids(s *ChecklistSnapshot) string {
	var out []string
	for _, p := range s.Parts {
		for _, m := range p.Modules {
			for _, r := range m.Rows {
				out = append(out, r.ID)
			}
		}
	}
	return strings.Join(out, ",")
}

func TestFilterChecklist(t *testing.T) {
	src := filterFixture()
	cases := []struct {
		name     string
		f        ChecklistFilter
		want     string
		excluded int
		parts    int
	}{
		{"all", ChecklistFilter{}, "A01.01,A01.02,A01.03,A01.04,A01.05,A01.06,A01.07,B01.01", 0, 2},
		{"prod", ChecklistFilter{Scope: ScopeProd}, "A01.01,A01.02,A01.03,A01.04,A01.05,A01.06", 0, 1},
		{"may-nov", ChecklistFilter{Scope: ScopeProd, From: "2026-05", To: "2026-11"}, "A01.02,A01.03,A01.04", 2, 1},
		{"before-may", ChecklistFilter{Scope: ScopeProd, To: "2026-04"}, "A01.01", 2, 1},
		{"from-aug", ChecklistFilter{Scope: ScopeProd, From: "2026-08"}, "A01.03,A01.04", 2, 1},
		{"edge-jun", ChecklistFilter{Scope: ScopeProd, From: "2026-06", To: "2026-06"}, "A01.02", 2, 1},
		{"apr-may", ChecklistFilter{Scope: ScopeProd, From: "2026-04", To: "2026-05"}, "A01.01,A01.02", 2, 1},
		{"dec", ChecklistFilter{Scope: ScopeProd, From: "2026-12", To: "2026-12"}, "", 2, 0},
	}
	for _, c := range cases {
		got := FilterChecklist(src, c.f)
		if ids(got) != c.want || got.ExcludedUnknown != c.excluded || len(got.Parts) != c.parts {
			t.Errorf("%s: rows %q excluded %d parts %d; want %q %d %d", c.name, ids(got), got.ExcludedUnknown, len(got.Parts), c.want, c.excluded, c.parts)
		}
	}
	if ids(src) != "A01.01,A01.02,A01.03,A01.04,A01.05,A01.06,A01.07,B01.01" {
		t.Error("FilterChecklist changed its input")
	}
}

func TestScopeInRenders(t *testing.T) {
	snap := FilterChecklist(filterFixture(), ChecklistFilter{Scope: ScopeProd, From: "2026-05", To: "2026-11"})
	want := "Hanya fitur PROD yang periode Prod sejak-nya beririsan dengan Mei 2026 s.d. Nov 2026 (2 fitur PROD tanpa tanggal tidak disertakan)"
	if got := scopeText(snap); got != want {
		t.Fatalf("scopeText = %q", got)
	}
	recs := csvRecords(t, render(t, snap, "csv", false))
	if len(recs) != 4 || recs[1][4] != "Mei–Jun 2026" || recs[3][4] != "Nov 2026" {
		t.Errorf("csv = %v", recs)
	}
	if pdf := string(render(t, snap, "pdf", false)); !strings.Contains(pdf, "Hanya fitur PROD") {
		t.Error("PDF lacks the scope line")
	}
	sheets, rows := xlsxDump(t, render(t, snap, "xlsx", false))
	if len(sheets) != 2 || !strings.Contains(strings.Join(rows["Ringkasan"][2], ""), "Hanya fitur PROD") {
		t.Errorf("xlsx summary = %v", rows["Ringkasan"][:3])
	}
	if scopeText(FilterChecklist(filterFixture(), ChecklistFilter{})) != "" {
		t.Error("full export has a scope line")
	}
}

// The sample catalogue mixes the 12-column (A, B) and 13-column (C) layouts.
func TestFixtureProdSince(t *testing.T) {
	cs, _ := fixtureSnapshot(t, false)
	got := map[string]string{}
	for _, r := range csvRecords(t, render(t, cs, "csv", false))[1:] {
		got[r[0]] = r[4]
	}
	want := map[string]string{"C01.01": "< Mei 2026", "C01.02": "Jul–Agu 2026", "C02.01": "?", "C02.02": "", "A01.01": ""}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s Prod sejak = %q, want %q", id, got[id], w)
		}
	}
	if n := ids(FilterChecklist(cs, ChecklistFilter{Scope: ScopeProd, From: "2026-05", To: "2026-11"})); n != "C01.02" {
		t.Errorf("ranged fixture rows = %q", n)
	}
}

// A PROD row with a known Prod since period has no "tercatat" date under its status in the PDF.
func TestPDFHidesStatusDateWhenProdSinceKnown(t *testing.T) {
	at := time.Date(2026, 11, 14, 3, 0, 0, 0, time.UTC)
	one := &ChecklistSnapshot{ProductName: "P", Parts: []ChecklistPart{{Code: "A", Name: "PA", Modules: []ChecklistModule{{Code: "A01", Name: "M", Rows: []ChecklistRow{
		{ID: "A01.01", Feature: "x", Status: "PROD", ProdSince: "2026-07", StatusSince: &at, DateSource: DateSourceBackfill},
	}}}}}}
	if pdf := string(render(t, one, "pdf", true)); strings.Contains(pdf, "tercatat ~14 Nov 2026") || !strings.Contains(pdf, "Jul 2026") {
		t.Error("known Prod since: the PDF must show Jul 2026 and no tercatat date")
	}
	one.Parts[0].Modules[0].Rows[0].ProdSince = "?"
	if pdf := string(render(t, one, "pdf", true)); !strings.Contains(pdf, "tercatat ~14 Nov 2026") {
		t.Error("unknown Prod since: the PDF must keep the tercatat date")
	}
	for in, want := range map[ChecklistRow]bool{{Status: "PROD", ProdSince: "<2026-05"}: false, {Status: "PROD"}: true, {Status: "DEV-DONE"}: true, {Status: "PROD", ProdSince: "bad"}: true} {
		if showsStatusDate(in) != want {
			t.Errorf("showsStatusDate(%+v) != %v", in, want)
		}
	}
}

// A ranged PROD export never lists undated PROD rows, whatever the client asks; an unranged one does.
func TestRangedExportExcludesUndated(t *testing.T) {
	ranged := FilterChecklist(filterFixture(), ChecklistFilter{Scope: ScopeProd, From: "2026-05", To: "2026-11"})
	for _, p := range ranged.Parts {
		for _, m := range p.Modules {
			for _, r := range m.Rows {
				if r.ProdSince == "" || r.ProdSince == "?" || strings.HasPrefix(r.ProdSince, "<") {
					t.Errorf("%s (%q) is in a May-Nov export", r.ID, r.ProdSince)
				}
			}
		}
	}
	if ranged.ExcludedUnknown != 2 {
		t.Errorf("excluded = %d", ranged.ExcludedUnknown)
	}
	all := FilterChecklist(filterFixture(), ChecklistFilter{Scope: ScopeProd})
	if ids(all) != "A01.01,A01.02,A01.03,A01.04,A01.05,A01.06" || all.ExcludedUnknown != 0 {
		t.Errorf("unranged PROD export = %q", ids(all))
	}
}

// A PROD export has no status legend and no status columns in the summaries (Fitur says it).
func TestProdOnlyLegendAndSummary(t *testing.T) {
	prod := FilterChecklist(filterFixture(), ChecklistFilter{Scope: ScopeProd})
	pdf := string(render(t, prod, "pdf", false))
	if strings.Contains(pdf, "Legenda Status") || strings.Contains(pdf, "DEV-DONE") || strings.Contains(pdf, "NOT-STARTED") || !strings.Contains(pdf, "Ringkasan Total") {
		t.Error("PROD PDF still shows the legend or non-PROD status columns")
	}
	_, rows := xlsxDump(t, render(t, prod, "xlsx", false))
	if h := strings.Join(rows["Ringkasan"][3], ","); h != "Kode,Nama,Modul,Fitur" {
		t.Errorf("PROD xlsx summary head = %q", h)
	}
	all := FilterChecklist(filterFixture(), ChecklistFilter{})
	pdf = string(render(t, all, "pdf", false))
	if !strings.Contains(pdf, "Legenda Status") || !strings.Contains(pdf, "NOT-STARTED") {
		t.Error("full PDF lost the legend")
	}
	_, rows = xlsxDump(t, render(t, all, "xlsx", false))
	if h := strings.Join(rows["Ringkasan"][3], ","); h != "Kode,Nama,Modul,Fitur,PROD,DEV-DONE,IN-PROGRESS,NOT-STARTED,UNKNOWN" {
		t.Errorf("full xlsx summary head = %q", h)
	}
}

// check_column=0 leaves the Cek column out of every format; the last-update columns move left.
func TestHideCheckColumn(t *testing.T) {
	at := time.Date(2026, 11, 14, 3, 0, 0, 0, time.UTC)
	snap := FilterChecklist(filterFixture(), ChecklistFilter{HideCheck: true})
	snap.Parts[0].Modules[0].Rows[0].StatusSince, snap.Parts[0].Modules[0].Rows[0].DateSource = &at, DateSourceSync
	recs := csvRecords(t, render(t, snap, "csv", true))
	if h := strings.Join(recs[0], ","); h != "ID,Sub-modul,Fitur,Status,Prod sejak,Status tercatat,Sumber tanggal" || len(recs[1]) != 7 || recs[1][5] != "2026-11-14" {
		t.Errorf("csv = %v / %v", recs[0], recs[1])
	}
	sheets, rows := xlsxDump(t, render(t, snap, "xlsx", true))
	if h := strings.Join(rows[sheets[1]][0], ","); h != "ID,Sub-modul,Fitur,Status,Prod sejak,Status tercatat,Sumber tanggal" {
		t.Errorf("xlsx head = %q", h)
	}
	if v := rows[sheets[1]][1][5]; v == "" {
		t.Errorf("xlsx date moved wrongly: %v", rows[sheets[1]][1])
	}
	if pdf := string(render(t, snap, "pdf", false)); strings.Contains(pdf, "Cek") {
		t.Error("PDF still mentions Cek")
	}
	if pdf := string(render(t, FilterChecklist(filterFixture(), ChecklistFilter{}), "pdf", false)); !strings.Contains(pdf, "Cek") {
		t.Error("default PDF lost Cek")
	}
	if (ChecklistFilter{HideCheck: true}).variant() != "/nocheck" || (ChecklistFilter{Scope: ScopeProd, HideCheck: true}).variant() != "/prod/nocheck" {
		t.Error("nocheck variant")
	}
}

// prod_since=unknown keeps only the undated PROD rows.
func TestUndatedOnly(t *testing.T) {
	f := ChecklistFilter{Scope: ScopeProd, UndatedOnly: true}
	got := FilterChecklist(filterFixture(), f)
	if ids(got) != "A01.05,A01.06" || got.ExcludedUnknown != 0 || len(got.Parts) != 1 {
		t.Errorf("rows %q excluded %d parts %d", ids(got), got.ExcludedUnknown, len(got.Parts))
	}
	if s := scopeText(got); s != "Hanya fitur PROD yang tanggal rilisnya (Prod sejak) belum diketahui" {
		t.Errorf("scopeText = %q", s)
	}
	if f.variant() != "/prod-undated" || (ChecklistFilter{Scope: ScopeProd, UndatedOnly: true, HideCheck: true}).variant() != "/prod-undated/nocheck" {
		t.Error("variant")
	}
	recs := csvRecords(t, render(t, got, "csv", false))
	if len(recs) != 3 || recs[1][4] != "?" || recs[2][4] != "" {
		t.Errorf("csv = %v", recs)
	}
}

// The fixture's B part uses the 14-column layout: Prod since and Periode pelaksanaan are parsed.
func TestFixtureDevPeriod(t *testing.T) {
	cs, _ := fixtureSnapshot(t, false)
	got := FilterChecklist(cs, ChecklistFilter{ShowDevPeriod: true, HideCheck: true})
	recs := csvRecords(t, render(t, got, "csv", false))
	if h := strings.Join(recs[0], ","); h != "ID,Sub-modul,Fitur,Status,Prod sejak,Periode pelaksanaan" {
		t.Fatalf("head = %q", h)
	}
	by := map[string][]string{}
	for _, r := range recs[1:] {
		by[r[0]] = r
	}
	for id, want := range map[string][2]string{
		"B01.001": {"Okt 2025", "Jul–Des 2024"}, "B01.002": {"Des 2025", "Jan–Apr 2025"}, "B01.003": {"?", "Sep 2024"},
		"B02.02": {"< Mei 2026", "Feb–Mar 2025"}, "B02.01": {"Okt 2025", ""}, "A01.01": {"", ""}, "C01.02": {"Jul–Agu 2026", ""},
	} {
		if r := by[id]; r == nil || r[4] != want[0] || r[5] != want[1] {
			t.Errorf("%s = %v, want %v", id, r, want)
		}
	}
	var b01 []ChecklistRow
	for _, p := range got.Parts {
		for _, m := range p.Modules {
			if m.Code == "B01" {
				b01 = m.Rows
			}
		}
	}
	if r := devPeriodRollup(b01); r != "2024-07..2025-04" {
		t.Errorf("B01 rollup = %q", r)
	}
	if devPeriodRollup(nil) != "" || devPeriodRollup([]ChecklistRow{{DevPeriod: "2024-09"}}) != "2024-09" {
		t.Error("rollup edges")
	}
	pdf := string(render(t, got, "pdf", false))
	if !strings.Contains(pdf, "Periode pelaksanaan modul: Jul 2024") || strings.Contains(pdf, "Cek") {
		t.Error("PDF lacks the module roll-up or still has Cek")
	}
	sheets, rows := xlsxDump(t, render(t, FilterChecklist(cs, ChecklistFilter{ShowDevPeriod: true}), "xlsx", true))
	for _, sh := range sheets {
		if strings.HasPrefix(sh, "B ") {
			if h := strings.Join(rows[sh][0], ","); h != "ID,Sub-modul,Fitur,Status,Prod sejak,Periode pelaksanaan,Cek,Status tercatat,Sumber tanggal" {
				t.Errorf("xlsx head = %q", h)
			}
		}
	}
	if (ChecklistFilter{ShowDevPeriod: true}).variant() != "/devperiod" {
		t.Error("variant")
	}
	// Without the option the output has no Periode pelaksanaan anywhere.
	if s := string(render(t, FilterChecklist(cs, ChecklistFilter{}), "csv", false)); strings.Contains(s, "Periode pelaksanaan") {
		t.Error("default CSV shows the development period")
	}
}
