package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
)

// SetGAOS wires the GAOS pusher (GAOS_INTEGRATION.md). A nil value keeps the integration off.
func (r *Router) SetGAOS(g *gaossync.Syncer) { r.gaos = g }

// SetRepoPool enables clone/checkout/verify. A nil pool keeps CRUD only.
func (r *Router) SetRepoPool(p *repopool.Pool) { r.repoPool = p }

const productCols = `id, name, repo_url, staging_url, root_dir, branch, playwright_accessible,
	last_verified_at, verify_report, created_at, updated_at`

// repoCols selects from product_repos aliased as r.
const repoCols = `r.id, r.product_id, r.name, r.repo_url, r.root_dir, r.is_main, r.is_automation, r.is_central,
	r.playwright_accessible, r.last_verified_at, r.verify_report, r.sync_report, r.created_at, r.updated_at`

type rowScanner interface{ Scan(dest ...any) error }

// queryer is satisfied by *sql.DB and *sql.Tx, so loaders work inside and outside transactions.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func scanProduct(s rowScanner) (models.Product, error) {
	var p models.Product
	var verified sql.NullTime
	var report sql.NullString
	var accessible bool
	if err := s.Scan(&p.ID, &p.Name, &p.RepoURL, &p.StagingURL, &p.RootDir, &p.Branch, &accessible,
		&verified, &report, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return p, err
	}
	p.PlaywrightAccessible = accessible
	if verified.Valid {
		t := verified.Time
		p.LastVerifiedAt = &t
	}
	if report.Valid && report.String != "" {
		p.VerifyReport = json.RawMessage(report.String)
	}
	p.Repos = []models.ProductRepo{}
	return p, nil
}

func scanRepo(s rowScanner) (models.ProductRepo, error) {
	var rp models.ProductRepo
	var verified sql.NullTime
	var verifyRep, syncRep sql.NullString
	if err := s.Scan(&rp.ID, &rp.ProductID, &rp.Name, &rp.RepoURL, &rp.RootDir, &rp.IsMain, &rp.IsAutomation, &rp.IsCentral,
		&rp.PlaywrightAccessible, &verified, &verifyRep, &syncRep, &rp.CreatedAt, &rp.UpdatedAt); err != nil {
		return rp, err
	}
	if verified.Valid {
		t := verified.Time
		rp.LastVerifiedAt = &t
	}
	if verifyRep.Valid && verifyRep.String != "" {
		rp.VerifyReport = json.RawMessage(verifyRep.String)
	}
	if syncRep.Valid && syncRep.String != "" {
		rp.SyncReport = json.RawMessage(syncRep.String)
	}
	rp.Branches = []string{}
	return rp, nil
}

// sortRepos orders repos: main, automation (if not main), central (if neither), then by name.
func sortRepos(repos []models.ProductRepo) {
	rank := func(r models.ProductRepo) int {
		switch {
		case r.IsMain:
			return 0
		case r.IsAutomation:
			return 1
		case r.IsCentral:
			return 2
		}
		return 3
	}
	sort.SliceStable(repos, func(i, j int) bool {
		if ri, rj := rank(repos[i]), rank(repos[j]); ri != rj {
			return ri < rj
		}
		return repos[i].Name < repos[j].Name
	})
}

// orderBranches puts the default branch first, then the rest alphabetically.
func orderBranches(branches []string, def string) []string {
	out := append([]string{}, branches...)
	sort.Slice(out, func(i, j int) bool {
		if (out[i] == def) != (out[j] == def) {
			return out[i] == def
		}
		return out[i] < out[j]
	})
	return out
}

// loadRepos reads repos (with branches) matching a WHERE clause on product_repos aliased r.
func loadRepos(ctx context.Context, q queryer, where string, args ...any) ([]models.ProductRepo, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+repoCols+` FROM product_repos r WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	repos := []models.ProductRepo{}
	for rows.Next() {
		rp, err := scanRepo(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		repos = append(repos, rp)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return repos, nil
	}
	brows, err := q.QueryContext(ctx, `SELECT b.repo_id, b.branch, b.is_default FROM product_repo_branches b
		JOIN product_repos r ON r.id = b.repo_id WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer brows.Close()
	idx := map[int]int{}
	for i, rp := range repos {
		idx[rp.ID] = i
	}
	for brows.Next() {
		var rid int
		var name string
		var def bool
		if err := brows.Scan(&rid, &name, &def); err != nil {
			return nil, err
		}
		if i, ok := idx[rid]; ok {
			repos[i].Branches = append(repos[i].Branches, name)
			if def {
				repos[i].DefaultBranch = name
			}
		}
	}
	if err := brows.Err(); err != nil {
		return nil, err
	}
	for i := range repos {
		repos[i].Branches = orderBranches(repos[i].Branches, repos[i].DefaultBranch)
	}
	return repos, nil
}

// loadProducts returns one product (id > 0) or all products (id == 0) with flag and repos.
func loadProducts(ctx context.Context, q queryer, id int) ([]models.Product, error) {
	query, args := `SELECT `+productCols+` FROM products ORDER BY name`, []any{}
	if id > 0 {
		query, args = `SELECT `+productCols+` FROM products WHERE id = ?`, []any{id}
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	products := []models.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		products = append(products, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return products, nil
	}
	flags := map[int]bool{}
	formats := map[int]string{}
	gaos := map[int]models.GAOSSettings{}
	frows, err := q.QueryContext(ctx, `SELECT product_id, allow_non_automation_work, COALESCE(knowledge_format, ''),
	gaos_integrated, gaos_product_id, gaos_last_sync_at, gaos_last_status, gaos_last_note FROM product_settings`)
	if err != nil {
		return nil, err
	}
	for frows.Next() {
		var pid int
		var on bool
		var kf string
		var g models.GAOSSettings
		var at sql.NullTime
		if err := frows.Scan(&pid, &on, &kf, &g.Integrated, &g.ProductID, &at, &g.LastStatus, &g.LastNote); err != nil {
			frows.Close()
			return nil, err
		}
		if at.Valid {
			t := at.Time
			g.LastSyncAt = &t
		}
		flags[pid] = on
		formats[pid] = kf
		gaos[pid] = g
	}
	frows.Close()
	where, rargs := `1=1`, []any{}
	if id > 0 {
		where, rargs = `r.product_id = ?`, []any{id}
	}
	repos, err := loadRepos(ctx, q, where, rargs...)
	if err != nil {
		return nil, err
	}
	byProduct := map[int][]models.ProductRepo{}
	for _, rp := range repos {
		byProduct[rp.ProductID] = append(byProduct[rp.ProductID], rp)
	}
	for i := range products {
		products[i].AllowNonAutomationWork = flags[products[i].ID]
		products[i].KnowledgeFormat = formats[products[i].ID]
		products[i].GAOS = gaos[products[i].ID]
		rs := byProduct[products[i].ID]
		if rs == nil {
			rs = []models.ProductRepo{}
		}
		sortRepos(rs)
		products[i].Repos = rs
	}
	return products, nil
}

// productRole classifies the caller for product actions.
func productRole(req *http.Request) (canManage, canVerify bool) {
	u, ok := auth.GetUser(req.Context())
	if !ok {
		// Fail closed: no authenticated user means no product access, whatever the wiring.
		return false, false
	}
	switch u.Role {
	case "Admin", "DevOps", "Developer":
		return true, true
	case "QA", "QC":
		return false, true
	}
	return false, false
}

// canToggleWork reports whether the caller may change allow_non_automation_work (Admin, DevOps).
func canToggleWork(req *http.Request) bool {
	u, ok := auth.GetUser(req.Context())
	if !ok {
		return false
	}
	return u.Role == "Admin" || u.Role == "DevOps"
}

// validateProduct normalises and validates the product-level fields (name, staging URL).
func validateProduct(p *models.Product) string {
	p.Name = strings.TrimSpace(p.Name)
	p.StagingURL = strings.TrimSpace(p.StagingURL)
	if p.Name == "" || len(p.Name) > 100 {
		return "name is required (max 100 characters)"
	}
	if u, err := url.Parse(p.StagingURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(p.StagingURL) > 500 {
		return "staging_url must be an http(s) URL without credentials"
	}
	return ""
}

// footprint is every path a repo claims inside the pool: its clone and its worktree area.
func footprint(rootDir string) []string {
	return []string{rootDir, rootDir + repopool.BranchesSuffix}
}

func pathsNested(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// rootDirOverlap reports whether rel's footprint (the clone and its ".branches" area) equals,
// contains or sits inside the footprint of any other repo's root_dir (nested checkouts would
// write into each other's working tree).
func rootDirOverlap(rel string, others []string) bool {
	for _, o := range others {
		for _, a := range footprint(rel) {
			for _, b := range footprint(o) {
				if pathsNested(a, b) {
					return true
				}
			}
		}
	}
	return false
}

// firstLine returns a short, single-line version of an operation error for API clients.
func firstLine(err error) string {
	m := err.Error()
	if i := strings.IndexByte(m, '\n'); i >= 0 {
		m = m[:i]
	}
	if len(m) > 200 {
		m = m[:200] + "…"
	}
	return m
}

// apiError is an error with an HTTP status, returned from handlers and transactions.
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func badRequest(msg string) *apiError { return &apiError{http.StatusBadRequest, msg} }
func conflict(msg string) *apiError   { return &apiError{http.StatusConflict, msg} }
func notFound(msg string) *apiError   { return &apiError{http.StatusNotFound, msg} }

var (
	errNoMain       = conflict("product has no main repo")
	errNoAutomation = conflict("product has no automation repo")
	errProductGone  = notFound("product not found")
	errRepoNotFound = notFound("repo not found")
)

// duplicateKeyMessages maps a violated unique key to its 409 message.
var duplicateKeyMessages = map[string]string{
	"uq_products_name":               "a product with this name already exists",
	"uq_products_root_dir":           "this root_dir is used by another product",
	"uq_product_repos_root_dir":      "root_dir is already used by another repo",
	"uq_product_repos_name":          "a repo with this name already exists in the product",
	"uq_product_repos_url":           "this repository is already listed for the product",
	"uq_product_repos_main":          "the product already has a main repo",
	"uq_product_repos_central":       "the product already has a central repo",
	"uq_product_repos_automation":    "the product already has an automation repo",
	"uq_product_repo_branches":       "duplicate or colliding branch name",
	"uq_product_repo_branch_slug":    "duplicate or colliding branch name",
	"uq_product_repo_branch_default": "only one default branch is allowed",
}

// conflictMessage maps a MySQL duplicate-key error (1062) to its 409 message by key name.
// Unknown keys and other errors report false.
func conflictMessage(err error) (string, bool) {
	var me *mysql.MySQLError
	if !errors.As(err, &me) || me.Number != 1062 {
		return "", false
	}
	i := strings.LastIndex(me.Message, "for key '")
	if i < 0 {
		return "", false
	}
	key := me.Message[i+len("for key '"):]
	if j := strings.IndexByte(key, '\''); j >= 0 {
		key = key[:j]
	}
	if j := strings.LastIndexByte(key, '.'); j >= 0 {
		key = key[j+1:]
	}
	msg, ok := duplicateKeyMessages[key]
	return msg, ok && msg != ""
}

// writeDBConflict answers a duplicate-key error with 409 and reports whether it did.
func writeDBConflict(w http.ResponseWriter, err error) bool {
	if msg, ok := conflictMessage(err); ok {
		writeJSONError(w, http.StatusConflict, msg)
		return true
	}
	return false
}

// poolError maps a repopool failure to an API error: busy 409, input 400, anything else 502.
func poolError(err error) error {
	var inErr *repopool.InputError
	switch {
	case errors.Is(err, repopool.ErrBusy):
		return conflict(repopool.ErrBusy.Error())
	case errors.As(err, &inErr):
		return badRequest(inErr.Msg)
	}
	return &apiError{http.StatusBadGateway, firstLine(err)}
}

// writeOpError answers any handler error: apiError as is, duplicate keys 409, the rest 500 with
// a generic text (the detail is logged, never sent).
func writeOpError(w http.ResponseWriter, what string, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeJSONError(w, ae.status, ae.msg)
		return
	}
	if writeDBConflict(w, err) {
		return
	}
	log.Printf("products %s: %v", what, err)
	writeJSONError(w, http.StatusInternalServerError, "internal error")
}

// lockProduct is the first statement of every mutating transaction: it serialises all
// mutations of one product (see PRODUCT_REPOS.md, Concurrency).
func lockProduct(ctx context.Context, tx *sql.Tx, id int) error {
	var x int
	err := tx.QueryRowContext(ctx, `SELECT id FROM products WHERE id = ? FOR UPDATE`, id).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return errProductGone
	}
	return err
}

// productTx runs fn in a transaction that starts by locking the product row.
func productTx(ctx context.Context, d *db.DB, id int, fn func(tx *sql.Tx) error) error {
	return d.WithTx(ctx, func(tx *sql.Tx) error {
		if err := lockProduct(ctx, tx, id); err != nil {
			return err
		}
		return fn(tx)
	})
}

// handleProducts serves /api/products, /api/products/{id}[/verify|clone|checkout] and
// /api/products/{id}/repos[/{rid}[/branches|verify|clone|checkout]].
func (r *Router) handleProducts(w http.ResponseWriter, req *http.Request) {
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Database not available")
		return
	}
	canManage, canVerify := productRole(req)
	rest := strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/products"), "/")

	// Collection.
	if rest == "" {
		switch req.Method {
		case http.MethodGet:
			list, err := loadProducts(req.Context(), d.SQL, 0)
			if err != nil {
				writeOpError(w, "list", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"products": list, "pool_enabled": r.repoPool != nil, "gaos_configured": r.gaos.Configured(), "gaos_web_url": r.gaos.WebURL()})
		case http.MethodPost:
			if !canManage {
				writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
				return
			}
			r.createProduct(w, req, d)
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	parts := strings.Split(rest, "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil || id <= 0 || len(parts) > 6 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	switch {
	case len(parts) == 1:
		switch req.Method {
		case http.MethodGet:
			r.respondProduct(w, req, id, http.StatusOK)
		case http.MethodPut:
			if !canManage {
				writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
				return
			}
			r.updateProduct(w, req, d, id)
		case http.MethodDelete:
			if !canManage {
				writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
				return
			}
			// Deletes the registry rows only (repos, branches and settings cascade); checkouts stay on disk.
			if _, err := d.Exec(req.Context(), `DELETE FROM products WHERE id=?`, id); err != nil {
				writeOpError(w, "delete", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	case len(parts) == 2 && (parts[1] == "verify" || parts[1] == "clone" || parts[1] == "checkout"):
		if req.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		r.legacyRepoAction(w, req, d, id, parts[1], canManage, canVerify)

	case parts[1] == "repos":
		r.handleProductRepos(w, req, d, id, parts[2:], canManage, canVerify)

	case parts[1] == "knowledge":
		r.handleKnowledge(w, req, id, parts[2:], canVerify)

	case parts[1] == "navigation":
		r.handleNavigation(w, req, id, parts[2:])

	case len(parts) == 3 && parts[1] == "gaos" && parts[2] == "sync":
		r.handleGAOSSync(w, req, id)

	case len(parts) == 2 && parts[1] == "service-repos":
		r.handleServiceRepos(w, req, id, canManage)

	case len(parts) == 2 && parts[1] == "case-results":
		r.handleCaseResults(w, req, id, canVerify)

	case len(parts) == 3 && parts[1] == "gaos" && parts[2] == "status":
		r.handleGAOSStatus(w, req, id)

	case len(parts) == 3 && parts[1] == "gaos" && parts[2] == "decisions":
		r.handleGAOSDecisions(w, req, id)

	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

type productCreateBody struct {
	Name       string      `json:"name"`
	StagingURL string      `json:"staging_url"`
	Repos      *[]repoBody `json:"repos"`
	// Legacy flat form: creates one main repo named "main".
	RepoURL string `json:"repo_url"`
	RootDir string `json:"root_dir"`
	Branch  string `json:"branch"`
}

// decodeStrictJSON decodes a size-capped request body into v and rejects unknown
// fields and trailing data, so a misspelt key (e.g. "default_branches") fails with
// 400 instead of being silently ignored.
func decodeStrictJSON(w http.ResponseWriter, req *http.Request, limit int64, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+firstLine(err))
		return false
	}
	if dec.More() {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: unexpected trailing data")
		return false
	}
	return true
}

func (r *Router) createProduct(w http.ResponseWriter, req *http.Request, d *db.DB) {
	var body productCreateBody
	if !decodeStrictJSON(w, req, 1<<16, &body) {
		return
	}
	p := models.Product{Name: body.Name, StagingURL: body.StagingURL}
	if msg := validateProduct(&p); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	legacy := body.RepoURL != "" || body.RootDir != "" || body.Branch != ""
	var repos []repoBody
	switch {
	case body.Repos != nil && legacy:
		writeJSONError(w, http.StatusBadRequest, "send either repos or the flat repo_url/root_dir/branch fields, not both")
		return
	case body.Repos != nil:
		repos = *body.Repos
	default:
		in := repoBody{Name: "main", RepoURL: body.RepoURL, RootDir: body.RootDir, IsMain: true, IsAutomation: true}
		if strings.TrimSpace(body.Branch) != "" {
			in.Branches = []string{body.Branch}
			in.DefaultBranch = body.Branch
		}
		repos = []repoBody{in}
	}
	repos, msg := validateRepoSet(repos)
	if msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	// Advisory overlap check against every other repo (the unique keys are the authority).
	others, err := otherRootDirs(req.Context(), d.SQL, 0)
	if err != nil {
		writeOpError(w, "create", err)
		return
	}
	for _, in := range repos {
		if rootDirOverlap(in.RootDir, others) {
			writeJSONError(w, http.StatusConflict, "root_dir overlaps another repo's root_dir")
			return
		}
	}

	var newID int
	err = d.WithTx(req.Context(), func(tx *sql.Tx) error {
		// The legacy columns start from the automation row (the mirror overwrites them below).
		var auto repoBody
		for _, in := range repos {
			if in.IsAutomation {
				auto = in
			}
		}
		res, err := tx.ExecContext(req.Context(),
			`INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES (?, ?, ?, ?, '')`,
			p.Name, auto.RepoURL, p.StagingURL, auto.RootDir)
		if err != nil {
			return err
		}
		pid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, in := range repos {
			if _, err := insertRepo(req.Context(), tx, int(pid), in); err != nil {
				return err
			}
		}
		newID = int(pid)
		return mirrorAutomationToLegacy(tx, newID)
	})
	if err != nil {
		writeOpError(w, "create", err)
		return
	}
	r.respondProduct(w, req, newID, http.StatusCreated)
}

type productUpdateBody struct {
	Name       *string `json:"name"`
	StagingURL *string `json:"staging_url"`
	Allow      *bool   `json:"allow_non_automation_work"`
	// KnowledgeFormat: "" turns the knowledge sync off; otherwise a registered format (Admin/DevOps only).
	KnowledgeFormat *string `json:"knowledge_format"`
	// GaosIntegrated and GaosProductID: the GAOS integration (GAOS_INTEGRATION.md; Admin/DevOps only).
	GaosIntegrated *bool   `json:"gaos_integrated"`
	GaosProductID  *string `json:"gaos_product_id"`
	// Legacy flat repo fields are accepted for old clients and ignored (see updateProduct).
	LegacyRepoURL string `json:"repo_url"`
	LegacyRootDir string `json:"root_dir"`
	LegacyBranch  string `json:"branch"`
}

// updateProduct applies name, staging_url and the flag. Legacy repo fields in the body are ignored.
func (r *Router) updateProduct(w http.ResponseWriter, req *http.Request, d *db.DB, id int) {
	var body productUpdateBody
	if !decodeStrictJSON(w, req, 1<<16, &body) {
		return
	}
	toggle := canToggleWork(req)
	ctx := req.Context()
	var flagChanged bool
	var newFlag bool
	var gaosTrigger bool
	err := productTx(ctx, d, id, func(tx *sql.Tx) error {
		flagChanged, gaosTrigger = false, false
		var cur models.Product
		if err := tx.QueryRowContext(ctx, `SELECT name, staging_url FROM products WHERE id = ?`, id).Scan(&cur.Name, &cur.StagingURL); err != nil {
			return err
		}
		oldStaging := cur.StagingURL
		if body.Name != nil {
			cur.Name = *body.Name
		}
		if body.StagingURL != nil {
			cur.StagingURL = *body.StagingURL
		}
		if msg := validateProduct(&cur); msg != "" {
			return badRequest(msg)
		}
		curFlag := false
		err := tx.QueryRowContext(ctx, `SELECT allow_non_automation_work FROM product_settings WHERE product_id = ?`, id).Scan(&curFlag)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if body.Allow != nil && *body.Allow != curFlag {
			if !toggle {
				return &apiError{http.StatusForbidden, "only Admin and DevOps can change allow_non_automation_work"}
			}
			flagChanged, newFlag = true, *body.Allow
		}
		if _, err := tx.ExecContext(ctx, `UPDATE products SET name=?, staging_url=? WHERE id=?`, cur.Name, cur.StagingURL, id); err != nil {
			return err
		}
		if body.KnowledgeFormat != nil {
			var curFormat sql.NullString
			err := tx.QueryRowContext(ctx, `SELECT knowledge_format FROM product_settings WHERE product_id = ?`, id).Scan(&curFormat)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if curFormat.String != *body.KnowledgeFormat {
				if !toggle {
					return &apiError{http.StatusForbidden, "only Admin and DevOps can change knowledge_format"}
				}
				if !knowledge.ValidFormat(*body.KnowledgeFormat) {
					return badRequest("knowledge_format must be empty or one of: " + strings.Join(knowledge.Names(), ", "))
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO product_settings (product_id, knowledge_format) VALUES (?, ?)
				ON DUPLICATE KEY UPDATE knowledge_format = VALUES(knowledge_format)`, id, *body.KnowledgeFormat); err != nil {
					return err
				}
			}
		}
		if body.GaosIntegrated != nil || body.GaosProductID != nil {
			var err error
			if gaosTrigger, err = r.applyGAOSSettings(ctx, tx, id, body.GaosIntegrated, body.GaosProductID, toggle); err != nil {
				return err
			}
		}
		if flagChanged {
			if _, err := tx.ExecContext(ctx, `INSERT INTO product_settings (product_id, allow_non_automation_work) VALUES (?, ?)
				ON DUPLICATE KEY UPDATE allow_non_automation_work = VALUES(allow_non_automation_work)`, id, newFlag); err != nil {
				return err
			}
		}
		if cur.StagingURL != oldStaging {
			// The automation repo's report includes the staging probe: it is stale now. Other repos are unaffected.
			if _, err := tx.ExecContext(ctx, `UPDATE product_repos SET playwright_accessible=0, last_verified_at=NULL, verify_report=NULL
				WHERE product_id=? AND is_automation=1`, id); err != nil {
				return err
			}
			return mirrorAutomationToLegacy(tx, id)
		}
		return nil
	})
	if err != nil {
		writeOpError(w, "update", err)
		return
	}
	if flagChanged {
		r.applyPushPolicies(ctx, d, id)
	}
	if gaosTrigger {
		r.gaos.Trigger(id) // after the commit; never blocks
	}
	r.respondProduct(w, req, id, http.StatusOK)
}

// Push policy retry: a busy checkout (a sync is running) is retried a few times.
const (
	pushPolicyAttempts = 3
	pushPolicyWait     = time.Second
)

// setPushPolicyRetry calls set, retrying on repopool.ErrBusy up to attempts times with wait
// between attempts (ctx permitting). Any other error is returned at once.
func setPushPolicyRetry(ctx context.Context, set func(context.Context, string, bool) error, rel string, allow bool, attempts int, wait time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = set(ctx, rel, allow); !errors.Is(err, repopool.ErrBusy) {
			return err
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(wait):
		}
	}
	return err
}

// applyPushPolicies re-applies the origin push policy of every repo of a product after the DB
// commit (best effort: busy checkouts are retried, then logged and fixed by the next sync; the
// DB write guard stays authoritative). allow = is_automation OR the product flag.
func (r *Router) applyPushPolicies(ctx context.Context, d *db.DB, productID int) {
	if r.repoPool == nil {
		return
	}
	list, err := loadProducts(context.WithoutCancel(ctx), d.SQL, productID)
	if err != nil || len(list) == 0 {
		return
	}
	p := list[0]
	for _, rp := range p.Repos {
		allow := rp.IsAutomation || p.AllowNonAutomationWork
		if err := setPushPolicyRetry(context.WithoutCancel(ctx), r.repoPool.SetPushPolicy, rp.RootDir, allow, pushPolicyAttempts, pushPolicyWait); err != nil {
			log.Printf("products %q: push policy for repo %q: %v", p.Name, rp.Name, err)
		}
	}
}

func (r *Router) loadProduct(w http.ResponseWriter, req *http.Request, id int) (models.Product, bool) {
	p, err := fetchProduct(req.Context(), r.db, id)
	if err != nil {
		writeOpError(w, "load", err)
		return p, false
	}
	return p, true
}

func fetchProduct(ctx context.Context, d *db.DB, id int) (models.Product, error) {
	list, err := loadProducts(ctx, d.SQL, id)
	if err != nil {
		return models.Product{}, err
	}
	if len(list) == 0 {
		return models.Product{}, errProductGone
	}
	return list[0], nil
}

func (r *Router) respondProduct(w http.ResponseWriter, req *http.Request, id, status int) {
	p, ok := r.loadProduct(w, req, id)
	if !ok {
		return
	}
	writeJSON(w, status, p)
}

// verifyAndStore verifies one repo of p and persists its report (and, for the automation repo, the legacy mirror).
// Only the automation repo gets the Playwright check and the staging probe.
func (r *Router) verifyAndStore(ctx context.Context, p models.Product, repo models.ProductRepo) error {
	var rep repopool.Report
	if repo.IsAutomation {
		rep = r.repoPool.Verify(ctx, repo.RepoURL, repo.RootDir, p.StagingURL)
	} else {
		rep = r.repoPool.VerifyRepo(ctx, repo.RepoURL, repo.RootDir, false)
	}
	if rep.Busy {
		return repopool.ErrBusy
	}
	return r.storeVerify(ctx, p.ID, repo, p.StagingURL, rep)
}

// storeVerify persists a report conditionally, in one transaction that starts by locking the
// product row: a result is dropped when the inputs it was computed from changed meanwhile
// (repo_url, root_dir, its automation role; for the automation repo also staging_url).
func (r *Router) storeVerify(ctx context.Context, productID int, repo models.ProductRepo, probedStaging string, rep repopool.Report) error {
	b, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	return productTx(ctx, r.db, productID, func(tx *sql.Tx) error {
		var staging string
		if err := tx.QueryRowContext(ctx, `SELECT staging_url FROM products WHERE id = ?`, productID).Scan(&staging); err != nil {
			return err
		}
		var isAutomation bool
		if err := tx.QueryRowContext(ctx, `SELECT is_automation FROM product_repos WHERE id = ? AND product_id = ?`, repo.ID, productID).Scan(&isAutomation); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil // repo deleted meanwhile
			}
			return err
		}
		if isAutomation != repo.IsAutomation || (isAutomation && staging != probedStaging) {
			return nil // stale: role or staging URL changed since the check ran
		}
		if _, err := tx.ExecContext(ctx, `UPDATE product_repos SET playwright_accessible=?, last_verified_at=UTC_TIMESTAMP(), verify_report=?
			WHERE id=? AND repo_url=? AND root_dir=?`, rep.OK, string(b), repo.ID, repo.RepoURL, repo.RootDir); err != nil {
			return err
		}
		if isAutomation {
			return mirrorAutomationToLegacy(tx, productID)
		}
		return nil
	})
}

// StartProductVerifier re-verifies every repo of every product on an interval until ctx ends.
// interval <= 0 or a missing pool/DB disables it.
func (r *Router) StartProductVerifier(ctx context.Context, interval time.Duration) {
	if interval <= 0 || r.repoPool == nil || r.getDB() == nil {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.verifyAllProducts(ctx)
			}
		}
	}()
	log.Printf("   Products: scheduled verify every %s", interval)
}

// verifyAllProducts probes each product's staging URL once, then verifies every repo
// (the automation repo with the Playwright check and the probe result merged in). Products without
// a main or an automation repo are skipped; busy repos are skipped silently.
func (r *Router) verifyAllProducts(ctx context.Context) {
	list, err := loadProducts(ctx, r.db.SQL, 0)
	if err != nil {
		log.Printf("products verify: %v", err)
		return
	}
	for _, p := range list {
		if !hasMain(p.Repos) || !hasAutomation(p.Repos) {
			continue
		}
		probe := r.repoPool.ProbeStaging(ctx, p.StagingURL)
		for _, rp := range p.Repos {
			var rep repopool.Report
			if rp.IsAutomation {
				rep = r.repoPool.VerifyRepo(ctx, rp.RepoURL, rp.RootDir, true)
				if !rep.Busy {
					rep.Append(probe)
				}
			} else {
				rep = r.repoPool.VerifyRepo(ctx, rp.RepoURL, rp.RootDir, false)
			}
			if rep.Busy {
				continue
			}
			if err := r.storeVerify(ctx, p.ID, rp, p.StagingURL, rep); err != nil {
				log.Printf("products verify %q repo %q: %v", p.Name, rp.Name, err)
			}
		}
	}
}
