package knowledge

import (
	"bytes"
	"encoding/csv"
)

// renderCSV writes UTF-8 with a BOM (so Excel opens it correctly), one header row, one row per feature.
func renderCSV(snap *ChecklistSnapshot, withLastUpdate bool) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(&buf)
	head := []string{"ID", "Sub-modul", "Fitur", "Status", "Cek"}
	if withLastUpdate {
		head = append(head, "Status sejak", "Sumber tanggal")
	}
	if err := w.Write(head); err != nil {
		return nil, err
	}
	for _, p := range snap.Parts {
		for _, m := range p.Modules {
			for _, r := range m.Rows {
				rec := []string{guardFormula(r.ID), guardFormula(r.Submodule), guardFormula(r.Feature), r.Status, ""}
				if withLastUpdate {
					since := ""
					if r.StatusSince != nil {
						since = dateISO(*r.StatusSince)
					}
					rec = append(rec, since, r.DateSource)
				}
				if err := w.Write(rec); err != nil {
					return nil, err
				}
			}
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
