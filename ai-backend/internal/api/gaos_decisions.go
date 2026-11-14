package api

// GET /api/products/{id}/gaos/decisions: the planning decisions GAOS pushed (gaos_decisions),
// compared with what the QA tools derive for the same case. Read-only, cookie auth.

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"

	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

type gaosDecisionRow struct {
	Code     string            `json:"code"`
	Decision string            `json:"decision"`
	Reason   string            `json:"reason"`
	CatRev   int64             `json:"cat_rev"`
	Updated  string            `json:"updated_at"`
	Theirs   gaossync.Derived  `json:"theirs"`
	Ours     *gaossync.Derived `json:"ours"`
	Differs  []string          `json:"differs"`
}

func diffDerived(t gaossync.Derived, o *gaossync.Derived) []string {
	d := []string{}
	if o == nil {
		return d
	}
	if t.Epic != o.Epic {
		d = append(d, "epic")
	}
	if t.Module != o.Module {
		d = append(d, "module")
	}
	tf, of := append([]string{}, t.Features...), append([]string{}, o.Features...)
	sort.Strings(tf)
	sort.Strings(of)
	if !reflect.DeepEqual(tf, of) {
		d = append(d, "features")
	}
	if t.UserStory != o.UserStory {
		d = append(d, "user_story")
	}
	if t.Polarity != o.Polarity {
		d = append(d, "polarity")
	}
	if t.Level != o.Level {
		d = append(d, "level")
	}
	if t.Pri != o.Pri {
		d = append(d, "pri")
	}
	return d
}

func (r *Router) handleGAOSDecisions(w http.ResponseWriter, req *http.Request, pid int) {
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d := r.getDB()
	if d == nil || r.gaos == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "not available")
		return
	}
	ctx := req.Context()
	q := req.URL.Query()
	onlyDiff := q.Get("diff") == "1"
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 25
	}
	ours, err := r.gaos.Derive(ctx, pid)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not derive the catalog")
		return
	}
	rows, err := d.Query(ctx, `SELECT case_code, decision, reason, cat_rev, DATE_FORMAT(updated_at, '%Y-%m-%dT%H:%i:%s'), epic, module, features, user_story, polarity, level, pri
		FROM gaos_decisions WHERE product_id = ? ORDER BY case_code`, pid)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	all, differing, items := 0, 0, []gaosDecisionRow{}
	for rows.Next() {
		var x gaosDecisionRow
		var feats []byte
		t := &x.Theirs
		if err := rows.Scan(&x.Code, &x.Decision, &x.Reason, &x.CatRev, &x.Updated, &t.Epic, &t.Module, &feats, &t.UserStory, &t.Polarity, &t.Level, &t.Pri); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "query failed")
			return
		}
		t.Features = decodeStringList(feats)
		if o, ok := ours[x.Code]; ok {
			x.Ours = &o
		}
		x.Differs = diffDerived(x.Theirs, x.Ours)
		all++
		if len(x.Differs) > 0 {
			differing++
		}
		if onlyDiff && len(x.Differs) == 0 {
			continue
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "query failed")
		return
	}
	total := len(items)
	lo := (page - 1) * size
	if lo > total {
		lo = total
	}
	hi := lo + size
	if hi > total {
		hi = total
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items[lo:hi], "total": total, "page": page, "page_size": size, "decisions": all, "differing": differing})
}

func decodeStringList(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	return out
}
