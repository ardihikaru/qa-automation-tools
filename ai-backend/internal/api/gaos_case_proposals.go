package api

// GAOS_CASE_PROPOSALS.md: GAOS's test planner sends the test cases a merged bundle of pull requests
// needs.
//
//	POST /api/v1/gaos/case-proposals
//
// Same guard as the other GAOS routes (serveGAOSInbound: bearer GAOS_INBOUND_TOKEN, fail-closed).
// A "create" item becomes a LOCAL DRAFT test case through the knowledge service's own draft path
// (Syncer.SaveDraft); a "link" item ties an existing case to the bundle's pull request URLs in
// kb_case_pr_links. Nothing is written to the automation repository: a QA person promotes a draft.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/qa-automation/ai-backend/internal/services/knowledge"
)

const (
	gaosCaseProposalsPath = "/api/v1/gaos/case-proposals"
	gaosProposalsMaxBody  = 256 << 10
	gaosMaxProposals      = 20
	gaosMaxProposalPRs    = 20
	gaosMaxProposalTitle  = 200
	gaosMaxProposalText   = 2000
)

var (
	gaosPRURLRe     = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}/pull/[0-9]{1,9}$`)
	gaosBundleKeyRe = regexp.MustCompile(`^(taskmgr:[A-Za-z0-9][A-Za-z0-9_-]{0,63}|pr:[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}#[0-9]{1,9})$`)
	gaosTaskCodeRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	gaosLinkCodeRe  = regexp.MustCompile(`^[A-Z][A-Z0-9-]{1,63}$`)
)

type gaosProposalCase struct {
	Action   string `json:"action"`
	Epic     string `json:"epic"`
	Title    string `json:"title"`
	Scenario string `json:"scenario"`
	Polarity string `json:"polarity"`
	Level    string `json:"level"`
	Priority string `json:"priority"`
	CaseCode string `json:"caseCode"`
	Note     string `json:"note"`
}

type gaosProposalsBody struct {
	Product   string             `json:"product"`
	BundleKey string             `json:"bundleKey"`
	TaskCode  string             `json:"taskCode"`
	PRURLs    []string           `json:"prUrls"`
	Cases     []gaosProposalCase `json:"cases"`
}

type gaosProposalResult struct {
	Index    int    `json:"index"`
	Outcome  string `json:"outcome"` // created | updated | linked | refused
	CaseCode string `json:"caseCode,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// gaosDraftCode is the code of a proposed case: positional, so a retried plan overwrites its
// drafts instead of adding a second set (GAOS design §7.7).
func gaosDraftCode(product, bundleKey string, index int) string {
	h := sha256.Sum256([]byte(product + "|" + bundleKey))
	return "L-G" + strings.ToUpper(hex.EncodeToString(h[:])[:8]) + "-" + strconv.Itoa(index)
}

// validateProposals checks the envelope; a bad one is a 400 and nothing is written.
func validateProposals(b *gaosProposalsBody) string {
	switch {
	case !gaosBundleKeyRe.MatchString(b.BundleKey):
		return "bundleKey must be taskmgr:<code> or pr:<owner>/<repo>#<n>"
	case b.TaskCode != "" && !gaosTaskCodeRe.MatchString(b.TaskCode):
		return "taskCode must match ^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$"
	case len(b.PRURLs) == 0 || len(b.PRURLs) > gaosMaxProposalPRs:
		return "prUrls must hold 1 to 20 pull request URLs"
	case len(b.Cases) == 0 || len(b.Cases) > gaosMaxProposals:
		return "cases must hold 1 to 20 items"
	}
	for _, u := range b.PRURLs {
		if !gaosPRURLRe.MatchString(u) {
			return "each prUrl must be https://github.com/<owner>/<repo>/pull/<n>"
		}
	}
	return ""
}

func clipRunesTo(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// gaosCaseProposals serves POST /api/v1/gaos/case-proposals.
func (r *Router) gaosCaseProposals(w http.ResponseWriter, req *http.Request) {
	var b gaosProposalsBody
	if !decodeStrictJSON(w, req, gaosProposalsMaxBody, &b) {
		return
	}
	if why := validateProposals(&b); why != "" {
		writeJSONError(w, http.StatusBadRequest, why)
		return
	}
	if r.kb == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "the knowledge service is not configured")
		return
	}
	ctx := req.Context()
	pid, found, err := r.gaosProductLocalID(ctx, b.Product)
	if err != nil {
		log.Printf("gaos case proposals: product lookup: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "unknown product")
		return
	}
	results := make([]gaosProposalResult, 0, len(b.Cases))
	for i, c := range b.Cases {
		var res gaosProposalResult
		var err error
		switch c.Action {
		case "create":
			res, err = r.gaosProposeCreate(ctx, pid, &b, i, c)
		case "link":
			res, err = r.gaosProposeLink(ctx, pid, &b, i, c)
		default:
			res = gaosProposalResult{Index: i, Outcome: "refused", Reason: "action must be create or link"}
		}
		if err != nil {
			log.Printf("gaos case proposals: item %d: %v", i, err)
			writeJSONError(w, http.StatusInternalServerError, "write failed")
			return
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// gaosProposeCreate upserts the local draft of one proposed case. An input the knowledge service
// refuses (an inactive epic, a bad enum) refuses this item only.
func (r *Router) gaosProposeCreate(ctx context.Context, pid int, b *gaosProposalsBody, i int, c gaosProposalCase) (gaosProposalResult, error) {
	refuse := func(why string) (gaosProposalResult, error) {
		return gaosProposalResult{Index: i, Outcome: "refused", Reason: why}, nil
	}
	title, scenario := strings.TrimSpace(c.Title), strings.TrimSpace(c.Scenario)
	switch {
	case title == "" || utf8.RuneCountInString(title) > gaosMaxProposalTitle:
		return refuse("title is required (at most 200 characters)")
	case utf8.RuneCountInString(scenario) > gaosMaxProposalText:
		return refuse("scenario is at most 2000 characters")
	case c.Epic == "":
		return refuse("epic is required")
	}
	code := gaosDraftCode(b.Product, b.BundleKey, i)
	text := title
	if scenario != "" {
		text += "\n\n" + scenario
	}
	detail := "Proposed by GAOS for " + b.BundleKey
	if b.TaskCode != "" {
		detail += " (task " + b.TaskCode + ")"
	}
	detail += ": " + strings.Join(b.PRURLs, " ")
	body := func() *knowledge.TestCaseBody {
		return &knowledge.TestCaseBody{Code: code, EpicCode: c.Epic, Scenario: text, Polarity: c.Polarity, Level: c.Level,
			Priority: c.Priority, Status: "planned", ReasonDetail: clipRunesTo(detail, 1000)}
	}
	outcome := "created"
	_, err := r.kb.SaveDraft(ctx, pid, knowledge.TypeTestCase, "", "", body())
	var conflict *knowledge.ConflictError
	if errors.As(err, &conflict) {
		outcome = "updated"
		_, err = r.kb.SaveDraft(ctx, pid, knowledge.TypeTestCase, code, "", body())
	}
	var in *knowledge.InputError
	switch {
	case errors.As(err, &in):
		return refuse(in.Msg)
	case errors.As(err, &conflict):
		return refuse("the code is held by a case that is not a local draft")
	case errors.Is(err, knowledge.ErrNotFound), errors.Is(err, knowledge.ErrProductNotFound):
		return refuse("not found")
	case err != nil:
		return gaosProposalResult{}, err
	}
	return gaosProposalResult{Index: i, Outcome: outcome, CaseCode: code}, nil
}

// gaosProposeLink ties an existing, live case to every pull request URL of the bundle.
func (r *Router) gaosProposeLink(ctx context.Context, pid int, b *gaosProposalsBody, i int, c gaosProposalCase) (gaosProposalResult, error) {
	refuse := func(why string) (gaosProposalResult, error) {
		return gaosProposalResult{Index: i, Outcome: "refused", Reason: why}, nil
	}
	note := strings.TrimSpace(c.Note)
	switch {
	case !gaosLinkCodeRe.MatchString(c.CaseCode):
		return refuse("caseCode is not a case code")
	case utf8.RuneCountInString(note) > gaosMaxProposalText:
		return refuse("note is at most 2000 characters")
	}
	d := r.getDB()
	cs, err := r.loadCase(ctx, d, pid, c.CaseCode)
	if err != nil {
		return gaosProposalResult{}, err
	}
	if cs == nil {
		return refuse("unknown case")
	}
	for _, u := range b.PRURLs {
		if _, err := d.Exec(ctx, `INSERT IGNORE INTO kb_case_pr_links (product_id, case_code, pr_url, bundle_key, note) VALUES (?, ?, ?, ?, ?)`,
			pid, c.CaseCode, u, b.BundleKey, note); err != nil {
			return gaosProposalResult{}, err
		}
	}
	return gaosProposalResult{Index: i, Outcome: "linked", CaseCode: c.CaseCode}, nil
}

// casePRLinks are the pull requests GAOS linked to one case, newest first; nil on any error.
func (r *Router) casePRLinks(ctx context.Context, pid int, code string) []map[string]any {
	d := r.getDB()
	if d == nil {
		return nil
	}
	rows, err := d.Query(ctx, `SELECT pr_url, bundle_key, note, created_at FROM kb_case_pr_links WHERE product_id = ? AND case_code = ? ORDER BY created_at DESC, pr_url LIMIT 100`, pid, code)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var u, k, n string
		var at any
		if rows.Scan(&u, &k, &n, &at) == nil {
			out = append(out, map[string]any{"pr_url": u, "bundle_key": k, "note": n, "created_at": at})
		}
	}
	return out
}
