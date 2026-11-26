package knowledge

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// statusColors are the status text colours, shared by the PDF and the XLSX (hex without '#').
// A status outside the five uses UNKNOWN's colour.
var statusColors = map[string]string{
	"PROD":        "1B7F3B",
	"DEV-DONE":    "1F5FBF",
	"IN-PROGRESS": "C46A00",
	"NOT-STARTED": "6B7280",
	"UNKNOWN":     "9CA3AF",
}

func statusColor(s string) string {
	if c, ok := statusColors[s]; ok {
		return c
	}
	return statusColors["UNKNOWN"]
}

const teal = "007E82"

// sheetName makes a legal, unique worksheet name (at most 31 characters, none of []:*?/\).
func sheetName(base string, used map[string]bool) string {
	base = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) || r < 0x20 {
			return -1
		}
		return r
	}, base)
	base = strings.Trim(strings.TrimSpace(base), "'")
	if base == "" {
		base = "Bagian"
	}
	clip := func(s string, n int) string {
		r := []rune(s)
		if len(r) > n {
			r = r[:n]
		}
		return string(r)
	}
	name := clip(base, 31)
	for i := 2; used[strings.ToLower(name)]; i++ {
		suffix := fmt.Sprintf(" (%d)", i)
		name = clip(base, 31-len([]rune(suffix))) + suffix
	}
	used[strings.ToLower(name)] = true
	return name
}

func renderXLSX(snap *ChecklistSnapshot, withLastUpdate bool) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	fixed := snap.SyncedAt
	if fixed.IsZero() {
		fixed = time.Unix(0, 0).UTC()
	}
	if err := f.SetDocProps(&excelize.DocProperties{Creator: "qa-automation", Title: snap.ProductName + " - Checklist Katalog Modul", Created: fixed.Format(time.RFC3339), Modified: fixed.Format(time.RFC3339), LastModifiedBy: "qa-automation"}); err != nil {
		return nil, err
	}

	headStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{teal}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	titleStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14, Color: teal}})
	if err != nil {
		return nil, err
	}
	statusStyle := map[string]int{}
	for _, st := range ReleaseStatuses {
		id, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: statusColor(st)}})
		if err != nil {
			return nil, err
		}
		statusStyle[st] = id
	}
	dateFmt := "yyyy-mm-dd"
	dateStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &dateFmt})
	if err != nil {
		return nil, err
	}

	// Ringkasan: totals per part, then per module.
	const sum = "Ringkasan"
	used := map[string]bool{strings.ToLower(sum): true}
	if err := f.SetSheetName("Sheet1", sum); err != nil {
		return nil, err
	}
	_ = f.SetCellStr(sum, "A1", snap.ProductName+" - Checklist Katalog Modul")
	_ = f.SetCellStyle(sum, "A1", "A1", titleStyle)
	if !snap.SyncedAt.IsZero() {
		_ = f.SetCellStr(sum, "A2", "Sinkronisasi katalog: "+dateID(snap.SyncedAt))
	}
	if s := scopeText(snap); s != "" {
		_ = f.SetCellStr(sum, "A3", s)
	}
	stCols := snap.statusColumns()
	head := append([]string{"Kode", "Nama", "Modul", "Fitur"}, stCols...)
	row := 4
	writeHead := func(r int) error {
		for i, h := range head {
			c, _ := excelize.CoordinatesToCellName(i+1, r)
			_ = f.SetCellStr(sum, c, h)
		}
		a, _ := excelize.CoordinatesToCellName(1, r)
		b, _ := excelize.CoordinatesToCellName(len(head), r)
		return f.SetCellStyle(sum, a, b, headStyle)
	}
	if err := writeHead(row); err != nil {
		return nil, err
	}
	row++
	put := func(r int, code, name string, mods int, rows []ChecklistRow) {
		_ = f.SetCellStr(sum, fmt.Sprintf("A%d", r), code)
		_ = f.SetCellStr(sum, fmt.Sprintf("B%d", r), name)
		_ = f.SetCellInt(sum, fmt.Sprintf("C%d", r), mods)
		_ = f.SetCellInt(sum, fmt.Sprintf("D%d", r), len(rows))
		for i, n := range statusCountsFor(rows, stCols) {
			c, _ := excelize.CoordinatesToCellName(5+i, r)
			_ = f.SetCellInt(sum, c, n)
		}
	}
	var all []ChecklistRow
	partRows := func(p ChecklistPart) []ChecklistRow {
		var rs []ChecklistRow
		for _, m := range p.Modules {
			rs = append(rs, m.Rows...)
		}
		return rs
	}
	mods := 0
	for _, p := range snap.Parts {
		rs := partRows(p)
		put(row, guardFormula(p.Code), guardFormula(p.Name), len(p.Modules), rs)
		all = append(all, rs...)
		mods += len(p.Modules)
		row++
	}
	put(row, "", "Total", mods, all)
	row += 2
	if err := writeHead(row); err != nil {
		return nil, err
	}
	row++
	for _, p := range snap.Parts {
		for _, m := range p.Modules {
			put(row, guardFormula(m.Code), guardFormula(m.Name), 1, m.Rows)
			row++
		}
	}
	_ = f.SetColWidth(sum, "A", "A", 12)
	_ = f.SetColWidth(sum, "B", "B", 44)
	_ = f.SetColWidth(sum, "C", "I", 13)

	// One sheet per part: columns ID | Sub-modul | Fitur | Status | Prod sejak | Cek [| Status tercatat | Sumber tanggal].
	cols := []string{"ID", "Sub-modul", "Fitur", "Status", "Prod sejak"}
	devCol, cekCol := 0, 0 // 1-based column numbers, 0 = not shown
	if snap.Filter.ShowDevPeriod {
		cols = append(cols, "Periode pelaksanaan")
		devCol = len(cols)
	}
	if !snap.Filter.HideCheck {
		cols = append(cols, "Cek")
		cekCol = len(cols)
	}
	dc := len(cols) + 1 // first last-update column
	if withLastUpdate {
		cols = append(cols, "Status tercatat", "Sumber tanggal")
	}
	for _, p := range snap.Parts {
		sh := sheetName(p.Code+" "+p.Name, used)
		if _, err := f.NewSheet(sh); err != nil {
			return nil, err
		}
		for i, h := range cols {
			c, _ := excelize.CoordinatesToCellName(i+1, 1)
			_ = f.SetCellStr(sh, c, h)
		}
		last, _ := excelize.CoordinatesToCellName(len(cols), 1)
		_ = f.SetCellStyle(sh, "A1", last, headStyle)
		r := 2
		for _, m := range p.Modules {
			for _, x := range m.Rows {
				cell := func(col int) string { c, _ := excelize.CoordinatesToCellName(col, r); return c }
				// SetCellStr always writes a string cell, so a leading "=" is never a formula.
				_ = f.SetCellStr(sh, cell(1), x.ID)
				_ = f.SetCellStr(sh, cell(2), x.Submodule)
				_ = f.SetCellStr(sh, cell(3), x.Feature)
				_ = f.SetCellStr(sh, cell(4), x.Status)
				_ = f.SetCellStyle(sh, cell(4), cell(4), statusStyle[ReleaseStatuses[statusIndex(x.Status)]])
				if ps := rowProdSince(x); ps != "" {
					_ = f.SetCellStr(sh, cell(5), ps)
				}
				if dp := prodSinceText(x.DevPeriod); devCol > 0 && dp != "" {
					_ = f.SetCellStr(sh, cell(devCol), dp)
				}
				if withLastUpdate && x.StatusSince != nil {
					d := x.StatusSince.In(wib)
					_ = f.SetCellValue(sh, cell(dc), time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC))
					_ = f.SetCellStyle(sh, cell(dc), cell(dc), dateStyle)
					_ = f.SetCellStr(sh, cell(dc+1), x.DateSource)
				}
				r++
			}
		}
		_ = f.SetColWidth(sh, "A", "A", 11)
		_ = f.SetColWidth(sh, "B", "B", 30)
		_ = f.SetColWidth(sh, "C", "C", 70)
		_ = f.SetColWidth(sh, "D", "D", 14)
		_ = f.SetColWidth(sh, "E", "E", 16)
		if devCol > 0 {
			n, _ := excelize.ColumnNumberToName(devCol)
			_ = f.SetColWidth(sh, n, n, 20)
		}
		if cekCol > 0 {
			n, _ := excelize.ColumnNumberToName(cekCol)
			_ = f.SetColWidth(sh, n, n, 8)
		}
		if withLastUpdate {
			a, _ := excelize.ColumnNumberToName(dc)
			b, _ := excelize.ColumnNumberToName(dc + 1)
			_ = f.SetColWidth(sh, a, b, 16)
		}
		if err := f.SetPanes(sh, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft", Selection: []excelize.Selection{{SQRef: "A2", ActiveCell: "A2", Pane: "bottomLeft"}}}); err != nil {
			return nil, err
		}
		end := r - 1
		if end < 1 {
			end = 1
		}
		endCell, _ := excelize.CoordinatesToCellName(len(cols), end)
		if err := f.AutoFilter(sh, "A1:"+endCell, nil); err != nil {
			return nil, err
		}
	}
	f.SetActiveSheet(0)
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
