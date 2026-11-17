package knowledge

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func ptr(t time.Time) *time.Time { return &t }

func TestPickStatusDate(t *testing.T) {
	s := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	b := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		sync, back *time.Time
		wantAt     *time.Time
		wantSrc    string
	}{
		{"sync wins over backfill", &s, &b, &s, "sync"},
		{"sync only", &s, nil, &s, "sync"},
		{"backfill only", nil, &b, &b, "backfill"},
		{"neither", nil, nil, nil, ""},
	}
	for _, c := range cases {
		at, src := pickStatusDate(c.sync, c.back)
		if at != c.wantAt || src != c.wantSrc {
			t.Errorf("%s: got (%v,%q)", c.name, at, src)
		}
	}
}

func TestStatusDatesIgnoreOtherStatuses(t *testing.T) {
	d := statusDates{}
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d.add("A01.01", "UNKNOWN", "sync", old)
	d.add("A01.01", "PROD", "backfill", old.Add(time.Hour))
	parts, _ := buildChecklist(checklistInput{
		parts:    []codeName{{"A", "P"}},
		modules:  []checklistModuleIn{{"A01", "A", "M"}},
		features: []checklistFeatureIn{{"A01.01", "A01", "", "F", "PROD"}},
		dates:    d,
	})
	r := parts[0].Modules[0].Rows[0]
	if r.DateSource != "backfill" || r.StatusSince == nil || !r.StatusSince.Equal(old.Add(time.Hour)) {
		t.Fatalf("row: %+v", r)
	}
}

func TestDateFormatting(t *testing.T) {
	// 2026-08-31 18:00 UTC is 2026-09-01 01:00 in Jakarta.
	at := time.Date(2026, 8, 31, 18, 0, 0, 0, time.UTC)
	if got := dateID(at); got != "01 Sep 2026" {
		t.Errorf("dateID = %q", got)
	}
	if got := dateISO(at); got != "2026-09-01" {
		t.Errorf("dateISO = %q", got)
	}
	if got := dateID(time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)); got != "03 Mei 2026" {
		t.Errorf("Mei: %q", got)
	}
}

func TestBuildChecklistOrphans(t *testing.T) {
	parts, skipped := buildChecklist(checklistInput{
		parts:      []codeName{{"A", "PA"}},
		modules:    []checklistModuleIn{{"A01", "A", "M1"}, {"B01", "B", "orphan module"}},
		submodules: map[string]string{"S1": "Sub one"},
		features: []checklistFeatureIn{
			{"A01.01", "A01", "S1", "f1", "PROD"},
			{"A01.02", "A01", "GONE", "f2", "UNKNOWN"}, // inactive sub-module: kept, empty cell
			{"B01.01", "B01", "", "f3", "PROD"},        // module not active
			{"Z.01", "Z01", "", "f4", "PROD"},          // module unknown
		},
	})
	if skipped != 3 {
		t.Errorf("skipped = %d", skipped)
	}
	rows := parts[0].Modules[0].Rows
	if len(parts) != 1 || len(parts[0].Modules) != 1 || len(rows) != 2 || rows[0].Submodule != "Sub one" || rows[1].Submodule != "" {
		t.Fatalf("parts: %+v", parts)
	}
}

// fixtureSnapshot builds a ChecklistSnapshot from the parsed SUT fixture. With dates, every third
// feature has a sync date and every third a backfill date.
func fixtureSnapshot(t *testing.T, dates bool) (*ChecklistSnapshot, *Snapshot) {
	t.Helper()
	f, _ := Lookup(TestCatalogFormatName)
	snap, _, err := f.Parse(loadFixture(t, "testdata/sut-521618a"))
	if err != nil {
		t.Fatal(err)
	}
	in := checklistInput{submodules: map[string]string{}}
	for _, p := range snap.Parts {
		in.parts = append(in.parts, codeName{p.Code, p.Name})
	}
	for _, m := range snap.Modules {
		in.modules = append(in.modules, checklistModuleIn{m.Code, m.PartCode, m.Name})
	}
	for _, s := range snap.Submodules {
		in.submodules[s.Code] = s.Name
	}
	for _, x := range snap.Features {
		in.features = append(in.features, checklistFeatureIn{x.Code, x.ModuleCode, x.SubmoduleCode, x.Name, x.ReleaseStatus})
	}
	if dates {
		in.dates = statusDates{}
		base := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
		for i, x := range snap.Features {
			switch i % 3 {
			case 0:
				in.dates.add(x.Code, x.ReleaseStatus, "sync", base.AddDate(0, 0, i%40))
			case 1:
				in.dates.add(x.Code, x.ReleaseStatus, "backfill", base.AddDate(0, -1, i%40))
			}
		}
	}
	parts, skipped := buildChecklist(in)
	return &ChecklistSnapshot{
		ProductName: "SUT", SyncID: 7, CommitSHA: "521618a0000000000000000000000000000000aa",
		SyncedAt: time.Date(2026, 11, 17, 2, 30, 0, 0, time.UTC), Skipped: skipped, Parts: parts,
	}, snap
}

func render(t *testing.T, s *ChecklistSnapshot, format string, upd bool) []byte {
	t.Helper()
	b, ct, err := RenderChecklist(s, format, upd)
	if err != nil || len(b) == 0 || ct == "" {
		t.Fatalf("render %s: %v (%d bytes, %q)", format, err, len(b), ct)
	}
	return b
}

func csvRecords(t *testing.T, b []byte) [][]string {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("\xEF\xBB\xBF")) {
		t.Fatal("CSV has no BOM")
	}
	recs, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF")))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

// xlsxRows returns, per sheet, the non-header rows of the part sheets (Status cell non-empty).
func xlsxDump(t *testing.T, b []byte) (sheets []string, rows map[string][][]string) {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows = map[string][][]string{}
	for _, sh := range f.GetSheetList() {
		rs, err := f.GetRows(sh)
		if err != nil {
			t.Fatal(err)
		}
		sheets = append(sheets, sh)
		rows[sh] = rs
	}
	return sheets, rows
}

func TestRenderRowCounts(t *testing.T) {
	cs, raw := fixtureSnapshot(t, false)
	want := len(raw.Features)
	if cs.FeatureCount() != want || cs.Skipped != 0 {
		t.Fatalf("snapshot has %d features (skipped %d), want %d", cs.FeatureCount(), cs.Skipped, want)
	}
	recs := csvRecords(t, render(t, cs, "csv", false))
	if len(recs)-1 != want {
		t.Errorf("CSV rows = %d, want %d", len(recs)-1, want)
	}
	if strings.Join(recs[0], ",") != "ID,Sub-modul,Fitur,Status,Cek" {
		t.Errorf("CSV header = %v", recs[0])
	}
	sheets, rows := xlsxDump(t, render(t, cs, "xlsx", false))
	if sheets[0] != "Ringkasan" || len(sheets) != 1+len(cs.Parts) {
		t.Errorf("sheets = %v", sheets)
	}
	n := 0
	for _, sh := range sheets[1:] {
		n += len(rows[sh]) - 1
	}
	if n != want {
		t.Errorf("XLSX rows = %d, want %d", n, want)
	}
	b := render(t, cs, "pdf", false)
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Error("PDF magic")
	}
	if !bytes.HasPrefix(render(t, cs, "xlsx", false), []byte("PK")) {
		t.Error("XLSX magic")
	}
}

func TestRenderLastUpdate(t *testing.T) {
	cs, _ := fixtureSnapshot(t, true)
	recs := csvRecords(t, render(t, cs, "csv", true))
	if got := recs[0]; len(got) != 7 || got[5] != "Status sejak" || got[6] != "Sumber tanggal" {
		t.Fatalf("header: %v", got)
	}
	seen := map[string]int{}
	for _, r := range recs[1:] {
		seen[r[6]]++
		if (r[6] == "") != (r[5] == "") {
			t.Fatalf("date and source disagree: %v", r)
		}
		if r[5] != "" && (len(r[5]) != 10 || r[5][4] != '-') {
			t.Fatalf("date format: %q", r[5])
		}
	}
	if seen["sync"] == 0 || seen["backfill"] == 0 || seen[""] == 0 {
		t.Errorf("sources: %v", seen)
	}
	// 03:00 UTC is 10:00 in Jakarta: same calendar day. A late-evening UTC instant rolls over.
	one := &ChecklistSnapshot{ProductName: "P", SyncedAt: time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC), Parts: []ChecklistPart{{Code: "A", Name: "PA", Modules: []ChecklistModule{{Code: "A01", Name: "M", Rows: []ChecklistRow{
		{ID: "A01.01", Feature: "x", Status: "PROD", StatusSince: ptr(time.Date(2026, 8, 31, 18, 0, 0, 0, time.UTC)), DateSource: "sync"},
		{ID: "A01.02", Feature: "y", Status: "PROD", StatusSince: ptr(time.Date(2026, 8, 31, 18, 0, 0, 0, time.UTC)), DateSource: "backfill"},
		{ID: "A01.03", Feature: "z", Status: "UNKNOWN"},
	}}}}}}
	r := csvRecords(t, render(t, one, "csv", true))
	if r[1][5] != "2026-09-01" || r[1][6] != "sync" || r[2][6] != "backfill" || r[3][5] != "" || r[3][6] != "" {
		t.Errorf("csv dates: %v", r)
	}
	pdf := string(render(t, one, "pdf", true))
	for _, s := range []string{"sejak 01 Sep 2026", "sejak ~01 Sep 2026", "sejak —"} {
		if !strings.Contains(pdf, strings.ReplaceAll(s, "—", "\x97")) {
			t.Errorf("PDF lacks %q", s)
		}
	}
	if !strings.Contains(pdf, "tanggal perkiraan") {
		t.Error("PDF lacks the ~ footnote")
	}
	if strings.Contains(string(render(t, one, "pdf", false)), "sejak") || strings.Contains(string(render(t, one, "pdf", false)), "tanggal perkiraan") {
		t.Error("PDF without the option must not mention dates")
	}
	f, err := excelize.OpenReader(bytes.NewReader(render(t, one, "xlsx", true)))
	if err != nil {
		t.Fatal(err)
	}
	sh := f.GetSheetList()[1]
	if v, _ := f.GetCellValue(sh, "F2"); v != "2026-09-01" {
		t.Errorf("xlsx date cell = %q", v)
	}
	if v, _ := f.GetCellValue(sh, "G3"); v != "backfill" {
		t.Errorf("xlsx source cell = %q", v)
	}
	if v, _ := f.GetCellValue(sh, "F4"); v != "" {
		t.Errorf("xlsx empty date = %q", v)
	}
	if tp, _ := f.GetCellType(sh, "F2"); tp == excelize.CellTypeSharedString || tp == excelize.CellTypeInlineString {
		t.Errorf("date cell is text (%v), want a real date", tp)
	}
}

func TestFormulaInjection(t *testing.T) {
	bad := []string{"=1+1", "+cmd", "-2", "@SUM(A1)", "\tx", "\rx"}
	var rows []ChecklistRow
	for i, s := range bad {
		rows = append(rows, ChecklistRow{ID: fmt.Sprintf("A01.%02d", i), Submodule: s, Feature: s, Status: "PROD"})
	}
	cs := &ChecklistSnapshot{ProductName: "P", Parts: []ChecklistPart{{Code: "A", Name: "PA", Modules: []ChecklistModule{{Code: "A01", Name: "M", Rows: rows}}}}}
	for _, r := range csvRecords(t, render(t, cs, "csv", false))[1:] {
		for _, c := range r[1:3] {
			if !strings.HasPrefix(c, "'") {
				t.Errorf("CSV cell %q not neutralised", c)
			}
		}
	}
	f, err := excelize.OpenReader(bytes.NewReader(render(t, cs, "xlsx", false)))
	if err != nil {
		t.Fatal(err)
	}
	sh := f.GetSheetList()[1]
	for i := range bad {
		for _, col := range []string{"B", "C"} {
			c := fmt.Sprintf("%s%d", col, i+2)
			tp, _ := f.GetCellType(sh, c)
			fm, _ := f.GetCellFormula(sh, c)
			if fm != "" || (tp != excelize.CellTypeSharedString && tp != excelize.CellTypeInlineString) {
				t.Errorf("%s: type %v formula %q", c, tp, fm)
			}
		}
	}
}

func TestPDFNonCp1252(t *testing.T) {
	if got := pdfSafe("Café — “ok” 日本 😀\x01"); got != "Café — “ok” ?? ??" {
		t.Errorf("pdfSafe = %q", got)
	}
	cs := &ChecklistSnapshot{ProductName: "Prod 日本", Parts: []ChecklistPart{{Code: "A", Name: "PA", Modules: []ChecklistModule{{Code: "A01", Name: "M", Rows: []ChecklistRow{{ID: "A01.01", Feature: "Nama 日本語 ok", Status: "PROD"}}}}}}}
	b := string(render(t, cs, "pdf", false))
	if !strings.Contains(b, "Nama ??? ok") || strings.Contains(b, "日") {
		t.Error("non-cp1252 characters must become '?' in the PDF")
	}
}

func TestRenderDeterministic(t *testing.T) {
	cs, _ := fixtureSnapshot(t, true)
	for _, f := range []string{"pdf", "csv"} {
		if !bytes.Equal(render(t, cs, f, true), render(t, cs, f, true)) {
			t.Errorf("%s output differs between two renders", f)
		}
	}
	_, a := xlsxDump(t, render(t, cs, "xlsx", true))
	_, b := xlsxDump(t, render(t, cs, "xlsx", true))
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Error("xlsx cells differ between two renders")
	}
}

func TestSheetNames(t *testing.T) {
	used := map[string]bool{"ringkasan": true}
	a := sheetName("A [Core]: a/b\\c?*"+strings.Repeat("x", 40), used)
	if len([]rune(a)) > 31 || strings.ContainsAny(a, `[]:*?/\`) {
		t.Errorf("name %q", a)
	}
	if b := sheetName("Ringkasan", used); b == "Ringkasan" || len(b) > 31 {
		t.Errorf("duplicate name %q", b)
	}
}

func hashHex(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// TestGoldenHashes pins the output for the fixture snapshot. When it fails, the output changed:
// raise TemplateVersion and update the hashes (they are printed). A library upgrade that changes
// the output fails here too, so it cannot change cached files silently.
func TestGoldenHashes(t *testing.T) {
	cs, _ := fixtureSnapshot(t, true)
	var dump bytes.Buffer
	sheets, rows := xlsxDump(t, render(t, cs, "xlsx", true))
	for _, sh := range sheets {
		fmt.Fprintf(&dump, "## %s\n", sh)
		for _, r := range rows[sh] {
			io.WriteString(&dump, strings.Join(r, "\x1f")+"\n")
		}
	}
	got := map[string]string{
		"csv":  hashHex(render(t, cs, "csv", true)),
		"xlsx": hashHex(dump.Bytes()),
		"pdf":  hashHex(render(t, cs, "pdf", true)),
	}
	want := map[string]string{
		"csv":  "9df6f91d814b8c34717bbe25a0174b79338668f5c7a90ac55aadf3036d25eec4",
		"xlsx": "e63e8d1a65ac0b2a1805013b69ab18f145ebc6de79910809ec002742831ec34c",
		"pdf":  "04cdb6c739edbc7892de4b2520bb0e63e555702847e06a1287f984e74322b29a",
	}
	for k, v := range got {
		if want[k] != v {
			t.Errorf("golden %s (TemplateVersion %d): got %s, want %s. Raise TemplateVersion and update the hash.", k, TemplateVersion, v, want[k])
		}
	}
}
