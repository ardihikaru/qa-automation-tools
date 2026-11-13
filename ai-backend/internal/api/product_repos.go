package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
)

// ---- Write guard ----

// RepoOp is a write-class operation on a repo: it changes git state outside the local pool
// checkout (remote branch creation, push, pull request). clone, sync and verify are not write-class.
type RepoOp string

const (
	RepoOpCreateBranch RepoOp = "create_branch"
	RepoOpPush         RepoOp = "push"
	RepoOpOpenPR       RepoOp = "open_pr"
)

var (
	// ErrWriteNotAllowed is returned for a write-class op on a sibling repo while the product flag is off.
	ErrWriteNotAllowed = errors.New("repo is read-only: enable allow_non_automation_work on the product")
	// ErrRepoNotFound is returned when a repo id does not exist (deny).
	ErrRepoNotFound = errors.New("repo not found")
)

// writeAllowed is the pure core of repoWriteAllowed: an unknown op is denied; the main repo is
// allowed; a non-main repo is allowed only while the product flag is on.
func writeAllowed(isMain, flag bool, op RepoOp) error {
	switch op {
	case RepoOpCreateBranch, RepoOpPush, RepoOpOpenPR:
	default:
		return ErrWriteNotAllowed
	}
	if isMain || flag {
		return nil
	}
	return ErrWriteNotAllowed
}

// repoWriteAllowed loads the repo and the product flag itself (one query, never cached), so
// callers cannot pass a stale or contradictory pair. Any future endpoint that pushes, creates
// branches or opens PRs MUST call it first; no route does yet.
func repoWriteAllowed(ctx context.Context, d *db.DB, repoID int, op RepoOp) error {
	var isMain, flag bool
	err := d.QueryRow(ctx, `SELECT r.is_main, COALESCE(s.allow_non_automation_work, 0)
		FROM product_repos r LEFT JOIN product_settings s ON s.product_id = r.product_id WHERE r.id = ?`, repoID).Scan(&isMain, &flag)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRepoNotFound
	}
	if err != nil {
		return err
	}
	return writeAllowed(isMain, flag, op)
}

// ---- Input ----

const maxBranchesPerRepo = 10

// repoBody is the JSON form of a repo on create.
type repoBody struct {
	Name          string   `json:"name"`
	RepoURL       string   `json:"repo_url"`
	RootDir       string   `json:"root_dir"`
	IsMain        bool     `json:"is_main"`
	IsCentral     bool     `json:"is_central"`
	Branches      []string `json:"branches"`
	DefaultBranch string   `json:"default_branch"`
}

type repoPatch struct {
	Name      *string `json:"name"`
	RepoURL   *string `json:"repo_url"`
	RootDir   *string `json:"root_dir"`
	IsMain    *bool   `json:"is_main"`
	IsCentral *bool   `json:"is_central"`
}

type branchesBody struct {
	Branches      []string `json:"branches"`
	DefaultBranch string   `json:"default_branch"`
}

// validateRepoFields normalises and validates name, repo_url and root_dir.
func validateRepoFields(name, repoURL, rootDir *string) string {
	*name = strings.TrimSpace(*name)
	*repoURL = strings.TrimSpace(*repoURL)
	*rootDir = strings.Trim(strings.TrimSpace(*rootDir), "/")
	if *name == "" || len(*name) > 100 {
		return "repo name is required (max 100 characters)"
	}
	if err := repopool.ValidateRepoURL(*repoURL); err != nil {
		return err.Error()
	}
	if err := repopool.ValidateRelDir(*rootDir); err != nil {
		return err.Error()
	}
	return ""
}

// normalizeBranches trims and validates a branch list. When strict is false an empty list with
// no default is allowed (legacy); a default without a list becomes the sole branch. A non-empty
// list needs a default that is in it.
func normalizeBranches(branches []string, def string, strict bool) ([]string, string, string) {
	def = strings.TrimSpace(def)
	list := make([]string, 0, len(branches))
	for _, b := range branches {
		list = append(list, strings.TrimSpace(b))
	}
	if len(list) == 0 && def != "" {
		list = []string{def}
	}
	if len(list) == 0 {
		if strict {
			return nil, "", "at least one branch is required"
		}
		return []string{}, "", ""
	}
	if len(list) > maxBranchesPerRepo {
		return nil, "", "at most 10 branches per repo"
	}
	names, slugs := map[string]bool{}, map[string]string{}
	for _, b := range list {
		if err := repopool.ValidateBranch(b); err != nil {
			return nil, "", err.Error() + ": " + b
		}
		slug := repopool.Slug(b)
		if len(slug) > repopool.MaxSlugLen {
			return nil, "", "branch name too long for a worktree"
		}
		if names[b] {
			return nil, "", "duplicate branch " + b
		}
		if other, ok := slugs[slug]; ok {
			return nil, "", "branches " + other + " and " + b + " map to the same worktree directory"
		}
		names[b], slugs[slug] = true, b
	}
	if def == "" {
		return nil, "", "default_branch is required"
	}
	if !names[def] {
		return nil, "", "default_branch must be one of branches"
	}
	return list, def, ""
}

// validateRepo validates and normalises a repo for creation (branches optional).
func validateRepo(in *repoBody) string {
	if msg := validateRepoFields(&in.Name, &in.RepoURL, &in.RootDir); msg != "" {
		return msg
	}
	list, def, msg := normalizeBranches(in.Branches, in.DefaultBranch, false)
	if msg != "" {
		return msg
	}
	in.Branches, in.DefaultBranch = list, def
	return ""
}

// validateRepoSet validates the repos of a new product: at least one, exactly one main, at most
// one central, unique names, unique normalised URLs and no overlapping footprints.
func validateRepoSet(repos []repoBody) string {
	if len(repos) == 0 {
		return "at least one repo (the main repo) is required"
	}
	mains, centrals := 0, 0
	names, urls := map[string]bool{}, map[string]bool{}
	for i := range repos {
		if msg := validateRepo(&repos[i]); msg != "" {
			return msg
		}
		in := repos[i]
		if in.IsMain {
			mains++
		}
		if in.IsCentral {
			centrals++
		}
		n := repopool.NormalizeRepo(in.RepoURL)
		if names[in.Name] || urls[n] {
			return "repo names and repo_url values must be unique within a product"
		}
		names[in.Name], urls[n] = true, true
		for j := 0; j < i; j++ {
			if rootDirOverlap(in.RootDir, []string{repos[j].RootDir}) {
				return "repo root_dir values must not overlap"
			}
		}
	}
	if mains != 1 {
		return "exactly one repo must have is_main true"
	}
	if centrals > 1 {
		return "at most one repo can be the central repo"
	}
	return ""
}

// ---- DB helpers (all take a transaction or *sql.DB) ----

// otherRootDirs lists every root_dir claimed by a repo other than exceptRepoID, plus the legacy
// root_dir of repo-less products.
func otherRootDirs(ctx context.Context, q queryer, exceptRepoID int) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT root_dir FROM product_repos WHERE id <> ?
		UNION SELECT root_dir FROM products WHERE id NOT IN (SELECT product_id FROM product_repos)`, exceptRepoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// mirrorMainToLegacy copies the main repo's url, dir, default branch and verify state into the
// legacy products columns (kept for rollback). It is the only writer of those columns.
func mirrorMainToLegacy(tx *sql.Tx, productID int) error {
	_, err := tx.Exec(`UPDATE products p
		JOIN product_repos r ON r.product_id = p.id AND r.is_main = 1
		LEFT JOIN product_repo_branches b ON b.repo_id = r.id AND b.is_default = 1
		SET p.repo_url = r.repo_url, p.root_dir = r.root_dir, p.branch = COALESCE(b.branch, ''),
		    p.playwright_accessible = r.playwright_accessible, p.last_verified_at = r.last_verified_at,
		    p.verify_report = r.verify_report
		WHERE p.id = ?`, productID)
	return err
}

func insertRepo(ctx context.Context, q queryer, productID int, in repoBody) (int, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_central)
		VALUES (?, ?, ?, ?, ?, ?)`, productID, in.Name, in.RepoURL, in.RootDir, in.IsMain, in.IsCentral)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), replaceBranches(ctx, q, int(id), in.Branches, in.DefaultBranch)
}

// replaceBranches replaces a repo's branch rows (old rows, including the old default flag, go first).
func replaceBranches(ctx context.Context, q queryer, repoID int, branches []string, def string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM product_repo_branches WHERE repo_id = ?`, repoID); err != nil {
		return err
	}
	for _, b := range branches {
		if _, err := q.ExecContext(ctx, `INSERT INTO product_repo_branches (repo_id, branch, slug, is_default) VALUES (?, ?, ?, ?)`,
			repoID, b, repopool.Slug(b), b == def); err != nil {
			return err
		}
	}
	return nil
}

// checkRepoConflicts is the advisory (clean 409) check run inside the product-locked transaction.
func checkRepoConflicts(ctx context.Context, q queryer, repos []models.ProductRepo, exceptID int, name, repoURL, rootDir string) error {
	norm := repopool.NormalizeRepo(repoURL)
	for _, o := range repos {
		if o.ID == exceptID {
			continue
		}
		if o.Name == name {
			return conflict(duplicateKeyMessages["uq_product_repos_name"])
		}
		if repopool.NormalizeRepo(o.RepoURL) == norm {
			return conflict(duplicateKeyMessages["uq_product_repos_url"])
		}
	}
	others, err := otherRootDirs(ctx, q, exceptID)
	if err != nil {
		return err
	}
	if rootDirOverlap(rootDir, others) {
		return conflict("root_dir overlaps another repo's root_dir")
	}
	return nil
}

func clearVerify(ctx context.Context, q queryer, repoID int) error {
	_, err := q.ExecContext(ctx, `UPDATE product_repos SET playwright_accessible=0, last_verified_at=NULL, verify_report=NULL WHERE id=?`, repoID)
	return err
}

func findRepo(repos []models.ProductRepo, id int) (models.ProductRepo, bool) {
	for _, rp := range repos {
		if rp.ID == id {
			return rp, true
		}
	}
	return models.ProductRepo{}, false
}

func hasMain(repos []models.ProductRepo) bool {
	for _, rp := range repos {
		if rp.IsMain {
			return true
		}
	}
	return false
}

// ---- Routing ----

// handleProductRepos serves /api/products/{id}/repos[/{rid}[/branches|verify|clone|checkout]]; parts is the path after "repos".
func (r *Router) handleProductRepos(w http.ResponseWriter, req *http.Request, d *db.DB, pid int, parts []string, canManage, canVerify bool) {
	methodNotAllowed := func() { writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed") }
	forbidManage := func() { writeJSONError(w, http.StatusForbidden, "your role cannot manage products") }
	if len(parts) == 0 {
		if req.Method != http.MethodPost {
			methodNotAllowed()
			return
		}
		if !canManage {
			forbidManage()
			return
		}
		r.addRepo(w, req, d, pid)
		return
	}
	rid, err := strconv.Atoi(parts[0])
	if err != nil || rid <= 0 || len(parts) > 2 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
		if sub != "branches" && sub != "verify" && sub != "clone" && sub != "checkout" {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
	}
	switch sub {
	case "":
		switch req.Method {
		case http.MethodPut:
			if !canManage {
				forbidManage()
				return
			}
			r.updateRepo(w, req, d, pid, rid)
		case http.MethodDelete:
			if !canManage {
				forbidManage()
				return
			}
			r.deleteRepo(w, req, d, pid, rid)
		default:
			methodNotAllowed()
		}
	case "branches":
		if req.Method != http.MethodPut {
			methodNotAllowed()
			return
		}
		if !canManage {
			forbidManage()
			return
		}
		r.putBranches(w, req, d, pid, rid)
	case "verify":
		if req.Method != http.MethodPost {
			methodNotAllowed()
			return
		}
		if !canVerify {
			writeJSONError(w, http.StatusForbidden, "your role cannot verify products")
			return
		}
		if !r.requirePool(w) {
			return
		}
		p, repo, err := loadRepoForRoute(req.Context(), d, pid, rid)
		if err != nil {
			writeOpError(w, "verify", err)
			return
		}
		r.doVerify(w, req, p, repo)
	default: // clone, checkout
		if req.Method != http.MethodPost {
			methodNotAllowed()
			return
		}
		if !canManage {
			forbidManage()
			return
		}
		if !r.requirePool(w) {
			return
		}
		p, repo, err := loadRepoForRoute(req.Context(), d, pid, rid)
		if err != nil {
			writeOpError(w, sub, err)
			return
		}
		ctx, cancel := detachedCtx(req)
		defer cancel()
		results, err := r.runSync(ctx, d, p, repo, sub == "clone")
		if err != nil {
			writeOpError(w, sub, err)
			return
		}
		fresh, err := fetchProduct(ctx, d, pid)
		if err != nil {
			writeOpError(w, sub, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"product": fresh, "results": results})
	}
}

func (r *Router) requirePool(w http.ResponseWriter) bool {
	if r.repoPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "repo pool is not configured (REPO_POOL_DIR)")
		return false
	}
	return true
}

// detachedCtx ignores client disconnects (a cancelled gh/git would leave a partial checkout)
// and bounds a whole sync at 10 minutes.
func detachedCtx(req *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(req.Context()), 10*time.Minute)
}

// loadRepoForRoute returns the product and repo {rid} of {pid}: 404 for a missing product or a repo
// that is not this product's, 409 when the product has no main repo.
func loadRepoForRoute(ctx context.Context, d *db.DB, pid, rid int) (models.Product, models.ProductRepo, error) {
	p, err := fetchProduct(ctx, d, pid)
	if err != nil {
		return p, models.ProductRepo{}, err
	}
	if !hasMain(p.Repos) {
		return p, models.ProductRepo{}, errNoMain
	}
	repo, ok := findRepo(p.Repos, rid)
	if !ok {
		return p, repo, errRepoNotFound
	}
	return p, repo, nil
}

func (r *Router) doVerify(w http.ResponseWriter, req *http.Request, p models.Product, repo models.ProductRepo) {
	if err := r.verifyAndStore(req.Context(), p, repo); err != nil {
		if errors.Is(err, repopool.ErrBusy) {
			writeJSONError(w, http.StatusConflict, repopool.ErrBusy.Error())
			return
		}
		log.Printf("products verify %q repo %q: %v", p.Name, repo.Name, err)
		writeJSONError(w, http.StatusInternalServerError, "verify failed")
		return
	}
	r.respondProduct(w, req, p.ID, http.StatusOK)
}

// ---- Repo mutations ----

func (r *Router) addRepo(w http.ResponseWriter, req *http.Request, d *db.DB, pid int) {
	var in repoBody
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if msg := validateRepo(&in); msg != "" {
		writeJSONError(w, http.StatusBadRequest, msg)
		return
	}
	ctx := req.Context()
	err := productTx(ctx, d, pid, func(tx *sql.Tx) error {
		repos, err := loadRepos(ctx, tx, `r.product_id = ?`, pid)
		if err != nil {
			return err
		}
		if !hasMain(repos) && !in.IsMain {
			return errNoMain
		}
		if err := checkRepoConflicts(ctx, tx, repos, 0, in.Name, in.RepoURL, in.RootDir); err != nil {
			return err
		}
		if in.IsMain { // promotion: the old main becomes a sibling first
			if _, err := tx.ExecContext(ctx, `UPDATE product_repos SET is_main=0 WHERE product_id=? AND is_main=1`, pid); err != nil {
				return err
			}
		}
		if in.IsCentral {
			if _, err := tx.ExecContext(ctx, `UPDATE product_repos SET is_central=0 WHERE product_id=? AND is_central=1`, pid); err != nil {
				return err
			}
		}
		if _, err := insertRepo(ctx, tx, pid, in); err != nil {
			return err
		}
		return mirrorMainToLegacy(tx, pid)
	})
	if err != nil {
		writeOpError(w, "add repo", err)
		return
	}
	if in.IsMain {
		r.applyPushPolicies(ctx, d, pid)
	}
	r.respondProduct(w, req, pid, http.StatusCreated)
}

func (r *Router) updateRepo(w http.ResponseWriter, req *http.Request, d *db.DB, pid, rid int) {
	var patch repoPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx := req.Context()
	mainChanged := false
	err := productTx(ctx, d, pid, func(tx *sql.Tx) error {
		mainChanged = false
		repos, err := loadRepos(ctx, tx, `r.product_id = ?`, pid)
		if err != nil {
			return err
		}
		if !hasMain(repos) {
			return errNoMain
		}
		cur, ok := findRepo(repos, rid)
		if !ok {
			return errRepoNotFound
		}
		name, repoURL, rootDir := cur.Name, cur.RepoURL, cur.RootDir
		if patch.Name != nil {
			name = *patch.Name
		}
		if patch.RepoURL != nil {
			repoURL = *patch.RepoURL
		}
		if patch.RootDir != nil {
			rootDir = *patch.RootDir
		}
		if msg := validateRepoFields(&name, &repoURL, &rootDir); msg != "" {
			return badRequest(msg)
		}
		if patch.IsMain != nil && !*patch.IsMain && cur.IsMain {
			return conflict("this is the main repo: promote another repo first")
		}
		if err := checkRepoConflicts(ctx, tx, repos, rid, name, repoURL, rootDir); err != nil {
			return err
		}
		promote := patch.IsMain != nil && *patch.IsMain && !cur.IsMain
		if promote {
			if _, err := tx.ExecContext(ctx, `UPDATE product_repos SET is_main=0 WHERE product_id=? AND is_main=1`, pid); err != nil {
				return err
			}
		}
		if patch.IsCentral != nil && *patch.IsCentral && !cur.IsCentral {
			if _, err := tx.ExecContext(ctx, `UPDATE product_repos SET is_central=0 WHERE product_id=? AND is_central=1`, pid); err != nil {
				return err
			}
		}
		isMain, isCentral := cur.IsMain || promote, cur.IsCentral
		if patch.IsCentral != nil {
			isCentral = *patch.IsCentral
		}
		if _, err := tx.ExecContext(ctx, `UPDATE product_repos SET name=?, repo_url=?, root_dir=?, is_main=?, is_central=? WHERE id=?`,
			name, repoURL, rootDir, isMain, isCentral, rid); err != nil {
			return err
		}
		// Verification depends on the URL and directory; a sibling's report also lacks the main-only
		// checks, so a newly promoted repo starts unverified as well.
		if repoURL != cur.RepoURL || rootDir != cur.RootDir || promote {
			if err := clearVerify(ctx, tx, rid); err != nil {
				return err
			}
		}
		mainChanged = promote
		return mirrorMainToLegacy(tx, pid)
	})
	if err != nil {
		writeOpError(w, "update repo", err)
		return
	}
	if mainChanged {
		r.applyPushPolicies(ctx, d, pid)
	}
	r.respondProduct(w, req, pid, http.StatusOK)
}

func (r *Router) deleteRepo(w http.ResponseWriter, req *http.Request, d *db.DB, pid, rid int) {
	ctx := req.Context()
	err := productTx(ctx, d, pid, func(tx *sql.Tx) error {
		repos, err := loadRepos(ctx, tx, `r.product_id = ?`, pid)
		if err != nil {
			return err
		}
		if !hasMain(repos) {
			return errNoMain
		}
		cur, ok := findRepo(repos, rid)
		if !ok {
			return errRepoNotFound
		}
		if cur.IsMain {
			return conflict("the main repo cannot be deleted: promote another repo first, or delete the product")
		}
		// Registry row only; the checkout stays on disk.
		_, err = tx.ExecContext(ctx, `DELETE FROM product_repos WHERE id=? AND product_id=?`, rid, pid)
		return err
	})
	if err != nil {
		writeOpError(w, "delete repo", err)
		return
	}
	r.respondProduct(w, req, pid, http.StatusOK)
}

func (r *Router) putBranches(w http.ResponseWriter, req *http.Request, d *db.DB, pid, rid int) {
	var body branchesBody
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx := req.Context()
	err := productTx(ctx, d, pid, func(tx *sql.Tx) error {
		repos, err := loadRepos(ctx, tx, `r.product_id = ?`, pid)
		if err != nil {
			return err
		}
		if !hasMain(repos) {
			return errNoMain
		}
		cur, ok := findRepo(repos, rid)
		if !ok {
			return errRepoNotFound
		}
		// A legacy repo with no branches may keep its empty list.
		if len(cur.Branches) == 0 && len(body.Branches) == 0 && strings.TrimSpace(body.DefaultBranch) == "" {
			return nil
		}
		list, def, msg := normalizeBranches(body.Branches, body.DefaultBranch, true)
		if msg != "" {
			return badRequest(msg)
		}
		if err := replaceBranches(ctx, tx, rid, list, def); err != nil {
			return err
		}
		if cur.IsMain {
			return mirrorMainToLegacy(tx, pid)
		}
		return nil
	})
	if err != nil {
		writeOpError(w, "branches", err)
		return
	}
	r.respondProduct(w, req, pid, http.StatusOK)
}

// ---- Pool operations ----

// runSync (optionally clones, then) syncs every listed branch of repo, stores the sync report and
// makes a best-effort re-verify. Git work happens outside any DB transaction.
func (r *Router) runSync(ctx context.Context, d *db.DB, p models.Product, repo models.ProductRepo, cloneFirst bool) ([]models.BranchResult, error) {
	if cloneFirst {
		if err := r.repoPool.Clone(ctx, repo.RepoURL, repo.RootDir); err != nil {
			log.Printf("products clone %q repo %q failed: %v", p.Name, repo.Name, err)
			return nil, poolError(err)
		}
	}
	specs := make([]repopool.BranchSpec, 0, len(repo.Branches))
	for _, b := range repo.Branches {
		specs = append(specs, repopool.BranchSpec{Name: b, Default: b == repo.DefaultBranch})
	}
	results, err := r.repoPool.SyncBranches(ctx, repo.RepoURL, repo.RootDir, specs, repo.IsMain || p.AllowNonAutomationWork)
	if err != nil {
		log.Printf("products sync %q repo %q failed: %v", p.Name, repo.Name, err)
		pe := poolError(err)
		if cloneFirst {
			if ae, ok := pe.(*apiError); ok && ae.status == http.StatusBadGateway {
				return nil, &apiError{ae.status, "cloned, but syncing branches failed: " + ae.msg}
			}
		}
		return nil, pe
	}
	report, _ := json.Marshal(map[string]any{"synced_at": time.Now().UTC().Format(time.RFC3339), "results": results})
	err = productTx(ctx, d, p.ID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE product_repos SET sync_report=? WHERE id=? AND repo_url=? AND root_dir=?`,
			string(report), repo.ID, repo.RepoURL, repo.RootDir)
		return err
	})
	if err != nil {
		return nil, err
	}
	// Refresh the verification after a successful operation (best effort).
	if fresh, err := fetchProduct(ctx, d, p.ID); err == nil {
		if rp, ok := findRepo(fresh.Repos, repo.ID); ok {
			_ = r.verifyAndStore(ctx, fresh, rp)
		}
	}
	return results, nil
}

// legacyRepoAction serves the old /{id}/verify|clone|checkout routes against the main repo.
func (r *Router) legacyRepoAction(w http.ResponseWriter, req *http.Request, d *db.DB, pid int, action string, canManage, canVerify bool) {
	if action == "verify" {
		if !canVerify {
			writeJSONError(w, http.StatusForbidden, "your role cannot verify products")
			return
		}
	} else if !canManage {
		writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
		return
	}
	if !r.requirePool(w) {
		return
	}
	p, err := fetchProduct(req.Context(), d, pid)
	if err != nil {
		writeOpError(w, action, err)
		return
	}
	var main models.ProductRepo
	for _, rp := range p.Repos {
		if rp.IsMain {
			main = rp
		}
	}
	if main.ID == 0 {
		writeOpError(w, action, errNoMain)
		return
	}
	if action == "verify" {
		r.doVerify(w, req, p, main)
		return
	}

	branch := ""
	if action == "checkout" {
		var body struct {
			Branch string `json:"branch"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<12)).Decode(&body)
		branch = strings.TrimSpace(body.Branch)
		if branch == "" {
			branch = main.DefaultBranch
		}
		if branch == "" {
			writeJSONError(w, http.StatusBadRequest, "branch is required")
			return
		}
	}
	ctx, cancel := detachedCtx(req)
	defer cancel()
	if action == "checkout" && branch != main.DefaultBranch {
		// Make it the main repo's default branch (added to the list when absent), mirrored to products.branch.
		err := productTx(ctx, d, pid, func(tx *sql.Tx) error {
			repos, err := loadRepos(ctx, tx, `r.id = ? AND r.product_id = ? AND r.is_main = 1`, main.ID, pid)
			if err != nil {
				return err
			}
			if len(repos) == 0 {
				return errNoMain
			}
			list := repos[0].Branches
			found := false
			for _, b := range list {
				found = found || b == branch
			}
			if !found {
				list = append(append([]string{}, list...), branch)
			}
			list, def, msg := normalizeBranches(list, branch, true)
			if msg != "" {
				return badRequest(msg)
			}
			if err := replaceBranches(ctx, tx, main.ID, list, def); err != nil {
				return err
			}
			return mirrorMainToLegacy(tx, pid)
		})
		if err != nil {
			writeOpError(w, action, err)
			return
		}
		if p, err = fetchProduct(ctx, d, pid); err != nil {
			writeOpError(w, action, err)
			return
		}
		main, _ = findRepo(p.Repos, main.ID)
	}
	if _, err := r.runSync(ctx, d, p, main, action == "clone"); err != nil {
		writeOpError(w, action, err)
		return
	}
	r.respondProduct(w, req, pid, http.StatusOK)
}
