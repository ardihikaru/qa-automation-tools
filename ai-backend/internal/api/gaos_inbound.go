package api

// GAOS_INBOUND.md: the two endpoints GAOS calls on the QA tools.
//   GET  /api/v1/gaos/results  - GAOS pulls automation facts (implemented, spec).
//   POST /api/v1/gaos/catalog  - GAOS pushes its planning decisions; stored in gaos_decisions.
// They sit outside cookie auth (see ServeHTTP) and are guarded by one static bearer token.
// No handler here may use the session user: there is none.

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

const (
	gaosInboundMaxBody  = 1 << 20
	gaosInboundMaxItems = 200
	gaosMaxSpec         = 240
	gaosMaxReason       = 300
	gaosMaxRejected     = 200
)

// gaosCursorLagSeconds hides rows younger than this from results, so a knowledge sync that is
// still inside its transaction cannot be skipped by a cursor that moved past its (later committed) rows.
var gaosCursorLagSeconds = 60

var (
	gaosCaseIDRe   = regexp.MustCompile(`^TC-[A-Z]+-[0-9]{3}$`)
	gaosDecisions  = map[string]bool{"planned": true, "blocked": true, "manual-only": true, "wont": true}
	gaosErrBadBody = errors.New("bad body")
)

// IsGAOSInboundPath reports whether path is one of the two bearer-authenticated routes.
// auth.IsAuthAllowList mirrors this (GAOS_INBOUND.md decision 1).
func IsGAOSInboundPath(path string) bool {
	return path == "/api/v1/gaos/results" || path == "/api/v1/gaos/catalog"
}

// SetGAOSInboundToken enables the inbound routes. Empty = disabled (503, fail-closed).
func (r *Router) SetGAOSInboundToken(token string) { r.gaosInboundToken = token }

func (r *Router) serveGAOSInbound(w http.ResponseWriter, req *http.Request) {
	if r.gaosInboundToken == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "GAOS inbound API is not configured")
		return
	}
	if !bearerMatches(req.Header.Get("Authorization"), r.gaosInboundToken) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="gaos"`)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	switch req.URL.Path {
	case "/api/v1/gaos/results":
		if req.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		r.gaosResults(w, req)
	default:
		if req.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		r.gaosCatalog(w, req)
	}
}

// bearerMatches compares in constant time (hashing first hides the length).
func bearerMatches(header, token string) bool {
	const p = "Bearer "
	if len(header) <= len(p) || !strings.EqualFold(header[:len(p)], p) {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(header[len(p):])))
	want := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// gaosProductLocalID maps a GAOS product id to the local product id; only flagged products match.
func (r *Router) gaosProductLocalID(ctx context.Context, gaosID string) (int, bool, error) {
	if !gaossync.ValidProductID(gaosID) {
		return 0, false, nil
	}
	var id int
	err := r.getDB().QueryRow(ctx, `SELECT product_id FROM product_settings WHERE gaos_integrated = 1 AND gaos_product_id = ? ORDER BY product_id LIMIT 1`, gaosID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// ---- results ----

type gaosResultItem struct {
	CaseID      string `json:"caseId"`
	Implemented bool   `json:"implemented"`
	Spec        string `json:"spec,omitempty"`
}

type gaosResultsPage struct {
	Product string           `json:"product"`
	Items   []gaosResultItem `json:"items"`
	Next    string           `json:"next"`
	More    bool             `json:"more"`
}

// parseGAOSCursor parses "<unix>:<id>"; empty is the start.
func parseGAOSCursor(s string) (ts, id int64, ok bool) {
	if s == "" {
		return 0, 0, true
	}
	a, b, found := strings.Cut(s, ":")
	if !found {
		return 0, 0, false
	}
	var e1, e2 error
	ts, e1 = strconv.ParseInt(a, 10, 64)
	id, e2 = strconv.ParseInt(b, 10, 64)
	return ts, id, e1 == nil && e2 == nil && ts >= 0 && id >= 0
}

// gaosUsableSpec returns the first spec path when GAOS would accept it, else "".
func gaosUsableSpec(raw []byte) string {
	var paths []string
	if len(raw) == 0 || json.Unmarshal(raw, &paths) != nil || len(paths) == 0 {
		return ""
	}
	sp := strings.TrimSpace(paths[0])
	if sp == "" || len(sp) > gaosMaxSpec || !utf8.ValidString(sp) || strings.HasPrefix(sp, "/") || strings.Contains(sp, `\`) || strings.Contains(sp, "://") {
		return ""
	}
	for _, c := range sp {
		if c < 0x20 || c > 0x7e {
			return ""
		}
	}
	for _, seg := range strings.Split(sp, "/") {
		if seg == ".." {
			return ""
		}
	}
	return sp
}

func (r *Router) gaosResults(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	product := q.Get("product")
	ctx := req.Context()
	pid, found, err := r.gaosProductLocalID(ctx, product)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "unknown product")
		return
	}
	limit := gaosInboundMaxItems
	if s := q.Get("limit"); s != "" {
		n, e := strconv.Atoi(s)
		if e != nil || n < 1 {
			writeJSONError(w, http.StatusBadRequest, "bad limit")
			return
		}
		if n < limit {
			limit = n
		}
	}
	after := q.Get("after")
	ts, id, ok := parseGAOSCursor(after)
	if !ok || len(after) > 128 {
		writeJSONError(w, http.StatusBadRequest, "bad cursor")
		return
	}
	rows, err := r.getDB().Query(ctx, `SELECT id, UNIX_TIMESTAMP(updated_at), code, automated, spec_paths FROM kb_test_cases
		WHERE product_id = ? AND origin = 'sync' AND removed_at IS NULL
		  AND updated_at <= NOW() - INTERVAL ? SECOND
		  AND (updated_at > FROM_UNIXTIME(?) OR (updated_at = FROM_UNIXTIME(?) AND id > ?))
		ORDER BY updated_at, id LIMIT ?`, pid, gaosCursorLagSeconds, ts, ts, id, limit+1)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	page := gaosResultsPage{Product: product, Items: []gaosResultItem{}, Next: strconv.FormatInt(ts, 10) + ":" + strconv.FormatInt(id, 10)}
	n := 0
	for rows.Next() {
		var rid, rts int64
		var code string
		var automated bool
		var specs []byte
		if err := rows.Scan(&rid, &rts, &code, &automated, &specs); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "query failed")
			return
		}
		n++
		if n > limit {
			page.More = true
			break
		}
		page.Next = strconv.FormatInt(rts, 10) + ":" + strconv.FormatInt(rid, 10)
		if !gaosCaseIDRe.MatchString(code) {
			continue
		}
		it := gaosResultItem{CaseID: code}
		if sp := gaosUsableSpec(specs); automated && sp != "" {
			it.Implemented, it.Spec = true, sp
		}
		page.Items = append(page.Items, it)
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// ---- catalog ----

type gaosCatalogCase struct {
	CaseID    string   `json:"caseId"`
	Epic      string   `json:"epic"`
	Module    string   `json:"module"`
	Features  []string `json:"features"`
	UserStory string   `json:"userStory"`
	Polarity  string   `json:"polarity"`
	Level     string   `json:"level"`
	Pri       string   `json:"pri"`
	Decision  string   `json:"decision"`
	Reason    string   `json:"reason"`
	CatRev    int64    `json:"catRev"`
	// Scenario, status and the automation fields are informational and ignored (not decoded).
}

type gaosCatalogBody struct {
	Product struct {
		ID string `json:"id"`
	} `json:"product"`
	Cases []gaosCatalogCase `json:"cases"`
}

type gaosRejected struct {
	CaseID string `json:"caseId"`
	Reason string `json:"reason"`
}

func validGAOSCase(c *gaosCatalogCase) string {
	switch {
	case !gaosCaseIDRe.MatchString(c.CaseID):
		return "bad-case-id"
	case !gaosDecisions[c.Decision]:
		return "bad-decision"
	case len(c.Reason) > gaosMaxReason || !utf8.ValidString(c.Reason):
		return "bad-reason"
	case len(c.Features) > 12:
		return "bad-features"
	case len(c.Epic) > 16 || len(c.Module) > 16 || len(c.UserStory) > 32 || len(c.Polarity) > 4 || len(c.Level) > 8 || len(c.Pri) > 4:
		return "bad-field"
	}
	for _, f := range c.Features {
		if len(f) > 32 {
			return "bad-features"
		}
	}
	return ""
}

func (r *Router) gaosCatalog(w http.ResponseWriter, req *http.Request) {
	var body gaosCatalogBody
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, gaosInboundMaxBody)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.Cases) > gaosInboundMaxItems {
		writeJSONError(w, http.StatusBadRequest, "too many cases (max 200)")
		return
	}
	ctx := req.Context()
	pid, found, err := r.gaosProductLocalID(ctx, body.Product.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "unknown product")
		return
	}
	rejected := []gaosRejected{}
	var valid []gaosCatalogCase
	seen := map[string]bool{}
	for i := range body.Cases {
		c := &body.Cases[i]
		if seen[c.CaseID] {
			rejected = append(rejected, gaosRejected{c.CaseID, "duplicate-case"})
			continue
		}
		seen[c.CaseID] = true
		if reason := validGAOSCase(c); reason != "" {
			rejected = append(rejected, gaosRejected{c.CaseID, reason})
			continue
		}
		valid = append(valid, *c)
	}
	accepted := 0
	if len(valid) > 0 {
		known, err := r.gaosKnownCases(ctx, pid, valid)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		err = r.getDB().WithTx(ctx, func(tx *sql.Tx) error {
			accepted = 0
			for _, c := range valid {
				if !known[c.CaseID] {
					continue
				}
				feats, _ := json.Marshal(append([]string{}, c.Features...))
				if _, err := tx.ExecContext(ctx, `INSERT INTO gaos_decisions
					(product_id, case_code, epic, module, features, user_story, polarity, level, pri, decision, reason, cat_rev)
					VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
					ON DUPLICATE KEY UPDATE
					 epic=IF(VALUES(cat_rev) >= cat_rev, VALUES(epic), epic),
					 module=IF(VALUES(cat_rev) >= cat_rev, VALUES(module), module),
					 features=IF(VALUES(cat_rev) >= cat_rev, VALUES(features), features),
					 user_story=IF(VALUES(cat_rev) >= cat_rev, VALUES(user_story), user_story),
					 polarity=IF(VALUES(cat_rev) >= cat_rev, VALUES(polarity), polarity),
					 level=IF(VALUES(cat_rev) >= cat_rev, VALUES(level), level),
					 pri=IF(VALUES(cat_rev) >= cat_rev, VALUES(pri), pri),
					 decision=IF(VALUES(cat_rev) >= cat_rev, VALUES(decision), decision),
					 reason=IF(VALUES(cat_rev) >= cat_rev, VALUES(reason), reason),
					 cat_rev=GREATEST(cat_rev, VALUES(cat_rev))`,
					pid, c.CaseID, c.Epic, c.Module, string(feats), c.UserStory, c.Polarity, c.Level, c.Pri, c.Decision, c.Reason, c.CatRev); err != nil {
					return err
				}
				accepted++
			}
			return nil
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "store failed")
			return
		}
		for _, c := range valid {
			if !known[c.CaseID] {
				rejected = append(rejected, gaosRejected{c.CaseID, "unknown-case"})
			}
		}
	}
	if len(rejected) > gaosMaxRejected {
		rejected = rejected[:gaosMaxRejected]
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": accepted, "rejected": rejected})
}

// gaosKnownCases returns the codes (of cs) that exist as active active cases (any origin) of the product.
func (r *Router) gaosKnownCases(ctx context.Context, pid int, cs []gaosCatalogCase) (map[string]bool, error) {
	args := make([]any, 0, len(cs)+1)
	args = append(args, pid)
	for _, c := range cs {
		args = append(args, c.CaseID)
	}
	rows, err := r.getDB().Query(ctx, `SELECT code FROM kb_test_cases WHERE product_id = ? AND removed_at IS NULL AND code IN (`+strings.TrimSuffix(strings.Repeat("?,", len(cs)), ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		known[code] = true
	}
	return known, rows.Err()
}

// GAOSInboundEnabled reports whether the inbound token is set.
func (r *Router) GAOSInboundEnabled() bool { return r.gaosInboundToken != "" }
