package knowledge

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// PDF layout (approximate, see the spec): A4 portrait, 9 pt Helvetica, 14 mm margins (182 mm).
const (
	pdfMargin   = 14.0
	pdfUsable   = 182.0
	pdfBottom   = 16.0
	pdfLineH    = 4.2
	pdfRowPad   = 1.6
	pdfNameMax  = 60
	pdfPageH    = 297.0
	pdfTealR    = 0x00
	pdfTealG    = 0x7E
	pdfTealB    = 0x82
	pdfApproxFN = "~ = tanggal perkiraan (dari backfill, sebelum pelacakan perubahan status dimulai)"
)

// cp1252Extra are the characters of 0x80-0x9F in cp1252 that fpdf's core-font translator supports.
const cp1252Extra = "€‚ƒ„…†‡ˆ‰Š‹ŒŽ‘’“”•–—˜™š›œžŸ"

// pdfSafe replaces every character outside cp1252 (and control characters) with '?' or a space,
// so the translator never meets a rune it cannot encode.
func pdfSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7F:
			b.WriteByte('?')
		case r < 0x80, r >= 0xA0 && r <= 0xFF, strings.ContainsRune(cp1252Extra, r):
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

func clipRunesN(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func hexRGB(h string) (int, int, int) {
	v, _ := strconv.ParseUint(h, 16, 32)
	return int(v >> 16 & 0xFF), int(v >> 8 & 0xFF), int(v & 0xFF)
}

type pdfDoc struct {
	p       *fpdf.Fpdf
	tr      func(string) string
	snap    *ChecklistSnapshot
	withUpd bool
	toc     map[string]int // pass 2: page numbers recorded in pass 1
	pages   map[string]int // recorded in this pass
	header  string
	date    string
}

func (d *pdfDoc) t(s string) string { return d.tr(pdfSafe(s)) }

func (d *pdfDoc) text(h string) { r, g, b := hexRGB(h); d.p.SetTextColor(r, g, b) }
func (d *pdfDoc) teal()         { d.p.SetTextColor(pdfTealR, pdfTealG, pdfTealB) }

// renderPDF renders twice: pass 1 records the page of each part and module, pass 2 prints them in
// the table of contents. The contents table has a fixed number of lines, so the layout is the same.
func renderPDF(snap *ChecklistSnapshot, withLastUpdate bool) ([]byte, error) {
	_, pages, err := buildPDF(snap, withLastUpdate, nil)
	if err != nil {
		return nil, err
	}
	b, _, err := buildPDF(snap, withLastUpdate, pages)
	return b, err
}

func buildPDF(snap *ChecklistSnapshot, withUpd bool, toc map[string]int) ([]byte, map[string]int, error) {
	p := fpdf.New("P", "mm", "A4", "")
	d := &pdfDoc{p: p, tr: p.UnicodeTranslatorFromDescriptor(""), snap: snap, withUpd: withUpd, toc: toc, pages: map[string]int{}}
	name := clipRunesN(snap.ProductName, pdfNameMax)
	d.header = d.t(name + " — Checklist Katalog Modul")
	if !snap.SyncedAt.IsZero() {
		d.date = dateID(snap.SyncedAt)
	}

	// Deterministic output: fixed creation date, no compression.
	created := snap.SyncedAt
	if created.IsZero() {
		created = time.Unix(0, 0).UTC()
	}
	p.SetCreationDate(created)
	p.SetModificationDate(created)
	p.SetCompression(false)
	p.SetCatalogSort(true)
	p.SetTitle(d.t(name+" — Checklist Katalog Modul"), false)
	p.SetCreator("qa-automation", false)
	p.SetProducer("qa-automation (go-pdf/fpdf)", false)
	p.SetMargins(pdfMargin, 16, pdfMargin)
	p.SetAutoPageBreak(false, pdfBottom)
	p.AliasNbPages("{nb}")
	p.SetHeaderFunc(func() {
		p.SetFont("Helvetica", "", 8)
		p.SetTextColor(107, 114, 128)
		p.SetXY(pdfMargin, 7)
		p.CellFormat(pdfUsable-40, 4, d.header, "", 0, "L", false, 0, "")
		p.CellFormat(40, 4, d.date, "", 0, "R", false, 0, "")
		p.SetDrawColor(pdfTealR, pdfTealG, pdfTealB)
		p.SetLineWidth(0.3)
		p.Line(pdfMargin, 12, pdfMargin+pdfUsable, 12)
		p.SetY(16)
	})
	p.SetFooterFunc(func() {
		p.SetY(-11)
		p.SetFont("Helvetica", "", 8)
		p.SetTextColor(107, 114, 128)
		p.CellFormat(0, 4, d.t(fmt.Sprintf("Halaman %d dari {nb}", p.PageNo())), "", 0, "C", false, 0, "")
	})

	p.AddPage()
	d.title()
	d.legend()
	d.totals()
	d.contents()
	for _, part := range snap.Parts {
		d.partSection(part)
	}
	var buf bytes.Buffer
	if err := p.Output(&buf); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), d.pages, nil
}

func (d *pdfDoc) title() {
	p := d.p
	d.teal()
	p.SetFont("Helvetica", "B", 20)
	p.CellFormat(pdfUsable, 10, d.t(clipRunesN(d.snap.ProductName, pdfNameMax)), "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "B", 14)
	p.CellFormat(pdfUsable, 8, "Checklist Katalog Modul", "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "", 9)
	p.SetTextColor(107, 114, 128)
	sub := fmt.Sprintf("%d fitur dalam %d modul", d.snap.FeatureCount(), d.moduleCount())
	if d.date != "" {
		sub += ", katalog disinkronkan " + d.date
	}
	if sha := d.snap.CommitSHA; len(sha) >= 7 {
		sub += " (commit " + sha[:7] + ")"
	}
	p.CellFormat(pdfUsable, 6, d.t(sub), "", 1, "L", false, 0, "")
	if s := scopeText(d.snap); s != "" {
		p.SetFont("Helvetica", "B", 9)
		d.teal()
		p.MultiCell(pdfUsable, 5, d.t(s), "", "L", false)
	}
	p.Ln(4)
}

func (d *pdfDoc) moduleCount() int {
	n := 0
	for _, p := range d.snap.Parts {
		n += len(p.Modules)
	}
	return n
}

func (d *pdfDoc) heading(s string) {
	p := d.p
	d.teal()
	p.SetFont("Helvetica", "B", 12)
	p.CellFormat(pdfUsable, 7, d.t(s), "", 1, "L", false, 0, "")
	p.SetTextColor(0, 0, 0)
	p.SetFont("Helvetica", "", 9)
}

func (d *pdfDoc) legend() {
	p := d.p
	d.heading("Legenda Status")
	for _, st := range ReleaseStatuses {
		d.text(statusColors[st])
		p.SetFont("Helvetica", "B", 9)
		p.CellFormat(34, pdfLineH, st, "", 0, "L", false, 0, "")
	}
	p.Ln(pdfLineH + 1)
	p.SetTextColor(0, 0, 0)
	p.SetFont("Helvetica", "", 8)
	p.CellFormat(pdfUsable, pdfLineH, d.t("Kolom Cek dikosongkan untuk dicentang manual saat pengecekan."), "", 1, "L", false, 0, "")
	if d.withUpd {
		p.CellFormat(pdfUsable, pdfLineH, d.t(pdfApproxFN), "", 1, "L", false, 0, "")
	}
	p.Ln(3)
	p.SetFont("Helvetica", "", 9)
}

// ensure starts a new page when h mm do not fit above the bottom margin; it returns true if it did.
func (d *pdfDoc) ensure(h float64) bool {
	if d.p.GetY()+h > pdfPageH-pdfBottom {
		d.p.AddPage()
		return true
	}
	return false
}

func (d *pdfDoc) tableHead(labels []string, widths []float64) {
	p := d.p
	p.SetFillColor(pdfTealR, pdfTealG, pdfTealB)
	p.SetTextColor(255, 255, 255)
	p.SetDrawColor(pdfTealR, pdfTealG, pdfTealB)
	for i, l := range labels {
		al := "L"
		p.SetFont("Helvetica", "B", 9)
		if i > 0 && len(labels) > 3 && widths[i] < 25 {
			al = "C"
			p.SetFont("Helvetica", "B", 7) // narrow numeric columns: statuses must fit
		}
		p.CellFormat(widths[i], 6, d.t(l), "1", 0, al, true, 0, "")
	}
	p.Ln(-1)
	p.SetTextColor(0, 0, 0)
	p.SetFont("Helvetica", "", 9)
	p.SetDrawColor(180, 186, 192)
}

// summaryTable prints one row per item (label, module count or -1, feature rows) plus an optional total.
type sumRow struct {
	label string
	mods  int // -1: no module column value
	rows  []ChecklistRow
}

func (d *pdfDoc) summaryTable(first string, withMods bool, rows []sumRow, total bool) {
	p := d.p
	labels := []string{first}
	widths := []float64{60}
	if withMods {
		labels = append(labels, "Modul")
		widths = append(widths, 14)
	} else {
		widths[0] = 74
	}
	labels = append(labels, "Fitur")
	widths = append(widths, 16)
	rest := (pdfUsable - sumF(widths)) / float64(len(ReleaseStatuses))
	for _, st := range ReleaseStatuses {
		labels = append(labels, st)
		widths = append(widths, rest)
	}
	head := func() { d.tableHead(labels, widths) }
	d.ensure(6 + pdfLineH + 2)
	head()
	var all []ChecklistRow
	mods := 0
	line := func(r sumRow, bold bool) {
		if d.ensure(pdfLineH + 1) {
			head()
		}
		st := ""
		if bold {
			st = "B"
		}
		p.SetFont("Helvetica", st, 8)
		i := 0
		p.CellFormat(widths[i], pdfLineH+1, d.fit(r.label, widths[i]-2), "1", 0, "L", false, 0, "")
		i++
		if withMods {
			p.CellFormat(widths[i], pdfLineH+1, strconv.Itoa(r.mods), "1", 0, "C", false, 0, "")
			i++
		}
		p.CellFormat(widths[i], pdfLineH+1, strconv.Itoa(len(r.rows)), "1", 0, "C", false, 0, "")
		i++
		for k, n := range statusCounts(r.rows) {
			if n > 0 {
				d.text(statusColors[ReleaseStatuses[k]])
			}
			p.CellFormat(widths[i+k], pdfLineH+1, strconv.Itoa(n), "1", 0, "C", false, 0, "")
			p.SetTextColor(0, 0, 0)
		}
		p.Ln(-1)
	}
	for _, r := range rows {
		line(r, false)
		all = append(all, r.rows...)
		mods += max(r.mods, 0)
	}
	if total {
		line(sumRow{label: "Total", mods: mods, rows: all}, true)
	}
	p.SetFont("Helvetica", "", 9)
	p.Ln(4)
}

func sumF(v []float64) float64 {
	t := 0.0
	for _, x := range v {
		t += x
	}
	return t
}

func partRows(p ChecklistPart) []ChecklistRow {
	var rs []ChecklistRow
	for _, m := range p.Modules {
		rs = append(rs, m.Rows...)
	}
	return rs
}

func (d *pdfDoc) totals() {
	d.heading("Ringkasan Total")
	var rows []sumRow
	for _, p := range d.snap.Parts {
		rows = append(rows, sumRow{label: p.Code + " — " + p.Name, mods: len(p.Modules), rows: partRows(p)})
	}
	d.summaryTable("Bagian", true, rows, true)
}

func (d *pdfDoc) contents() {
	p := d.p
	d.ensure(40)
	d.heading("Daftar Isi")
	line := func(label string, key string, bold bool) {
		d.ensure(pdfLineH + 0.6)
		st := ""
		if bold {
			st = "B"
		}
		p.SetFont("Helvetica", st, 9)
		page := ""
		if n, ok := d.toc[key]; ok {
			page = strconv.Itoa(n)
		}
		indent := 0.0
		if !bold {
			indent = 6
		}
		p.SetX(pdfMargin + indent)
		p.CellFormat(pdfUsable-indent-16, pdfLineH+0.6, d.t(clipRunesN(label, 80)), "", 0, "L", false, 0, "")
		p.CellFormat(16, pdfLineH+0.6, page, "", 1, "R", false, 0, "")
	}
	for _, part := range d.snap.Parts {
		line(part.Code+" — "+part.Name, "p:"+part.Code, true)
		for _, m := range part.Modules {
			line(m.Code+" — "+m.Name, "m:"+m.Code, false)
		}
	}
	p.SetFont("Helvetica", "", 9)
}

func (d *pdfDoc) partSection(part ChecklistPart) {
	p := d.p
	p.AddPage()
	d.pages["p:"+part.Code] = p.PageNo()
	d.heading("Bagian " + part.Code + " — " + part.Name)
	var rows []sumRow
	for _, m := range part.Modules {
		rows = append(rows, sumRow{label: m.Code + " — " + m.Name, mods: -1, rows: m.Rows})
	}
	d.summaryTable("Modul", false, rows, false)
	for _, m := range part.Modules {
		d.moduleTable(m)
	}
}

var modWidths = []float64{18, 32, 60, 28, 30, 14} // Prod sejak fits "Des 2025–Jan 2026" at 8 pt

func (d *pdfDoc) moduleTable(m ChecklistModule) {
	p := d.p
	labels := []string{"ID", "Sub-modul", "Fitur", "Status", "Prod sejak", "Cek"}
	// Heading, the table header and at least one row stay together.
	d.ensure(7 + 6 + pdfLineH*2 + pdfRowPad)
	d.pages["m:"+m.Code] = p.PageNo()
	d.heading(m.Code + " — " + m.Name)
	d.tableHead(labels, modWidths)
	for _, r := range m.Rows {
		p.SetFont("Helvetica", "", 9)
		sub := d.split(r.Submodule, modWidths[1]-2)
		feat := d.split(r.Feature, modWidths[2]-2)
		lines := max(len(sub), len(feat), 1)
		h := float64(lines)*pdfLineH + pdfRowPad
		if d.withUpd {
			h = max(h, 2*pdfLineH+pdfRowPad)
		}
		if d.ensure(h) {
			d.tableHead(labels, modWidths)
		}
		x, y := pdfMargin, p.GetY()
		xs := x
		for _, w := range modWidths {
			p.Rect(xs, y, w, h, "D")
			xs += w
		}
		p.SetFont("Helvetica", "", 9)
		p.SetTextColor(0, 0, 0)
		p.SetXY(x, y+pdfRowPad/2)
		p.CellFormat(modWidths[0], pdfLineH, d.t(r.ID), "", 0, "L", false, 0, "")
		for i, l := range sub {
			p.SetXY(x+modWidths[0]+1, y+pdfRowPad/2+float64(i)*pdfLineH)
			p.CellFormat(modWidths[1]-2, pdfLineH, l, "", 0, "L", false, 0, "")
		}
		for i, l := range feat {
			p.SetXY(x+modWidths[0]+modWidths[1]+1, y+pdfRowPad/2+float64(i)*pdfLineH)
			p.CellFormat(modWidths[2]-2, pdfLineH, l, "", 0, "L", false, 0, "")
		}
		sx := x + modWidths[0] + modWidths[1] + modWidths[2]
		d.text(statusColor(r.Status))
		p.SetFont("Helvetica", "B", 9)
		p.SetXY(sx+1, y+pdfRowPad/2)
		p.CellFormat(modWidths[3]-2, pdfLineH, d.t(r.Status), "", 0, "L", false, 0, "")
		if d.withUpd {
			p.SetFont("Helvetica", "", 7)
			p.SetTextColor(107, 114, 128)
			p.SetXY(sx+1, y+pdfRowPad/2+pdfLineH)
			p.CellFormat(modWidths[3]-2, pdfLineH, d.sinceText(r), "", 0, "L", false, 0, "")
		}
		p.SetTextColor(0, 0, 0)
		p.SetFont("Helvetica", "", 8)
		p.SetXY(sx+modWidths[3]+1, y+pdfRowPad/2)
		p.CellFormat(modWidths[4]-2, pdfLineH, d.fit(prodSinceText(r.ProdSince), modWidths[4]-2), "", 0, "L", false, 0, "")
		// Empty check square, centred in the Cek column.
		cx := sx + modWidths[3] + modWidths[4] + (modWidths[5]-3.6)/2
		p.SetDrawColor(60, 60, 60)
		p.Rect(cx, y+(h-3.6)/2, 3.6, 3.6, "D")
		p.SetDrawColor(180, 186, 192)
		p.SetY(y + h)
	}
	p.Ln(5)
}

// fit cuts text to w mm in the current font, ending with an ellipsis when it was cut.
func (d *pdfDoc) fit(s string, w float64) string {
	r := []rune(pdfSafe(s))
	if d.p.GetStringWidth(d.tr(string(r))) <= w {
		return d.tr(string(r))
	}
	for len(r) > 1 {
		r = r[:len(r)-1]
		if d.p.GetStringWidth(d.tr(string(r)+"...")) <= w {
			break
		}
	}
	return d.tr(string(r) + "...")
}

// split wraps text to w mm in the current font. The text is already encoded to cp1252 bytes, which
// fpdf's SplitText cannot index (it works on runes), so this wraps by words with GetStringWidth and
// breaks a word longer than the line byte by byte.
func (d *pdfDoc) split(s string, w float64) []string {
	s = d.t(s)
	var lines []string
	cur := ""
	fits := func(x string) bool { return d.p.GetStringWidth(x) <= w }
	for _, word := range strings.Fields(s) {
		for !fits(word) {
			// longest prefix that fits (at least one byte)
			n := 1
			for n < len(word) && fits(word[:n+1]) {
				n++
			}
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			lines = append(lines, word[:n])
			word = word[n:]
		}
		switch {
		case cur == "":
			cur = word
		case fits(cur + " " + word):
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" || len(lines) == 0 {
		lines = append(lines, cur)
	}
	return lines
}

func (d *pdfDoc) sinceText(r ChecklistRow) string {
	switch {
	case r.StatusSince == nil:
		return d.t("sejak —")
	case r.DateSource == DateSourceBackfill:
		return d.t("sejak ~" + dateID(*r.StatusSince))
	}
	return d.t("sejak " + dateID(*r.StatusSince))
}
