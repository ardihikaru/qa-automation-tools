package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/models"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
)

// SetRepoPool enables clone/checkout/verify. A nil pool keeps CRUD only.
func (r *Router) SetRepoPool(p *repopool.Pool) { r.repoPool = p }

const productCols = `id, name, repo_url, staging_url, root_dir, branch, playwright_accessible,
	last_verified_at, verify_report, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

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
	return p, nil
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

func validateProduct(p *models.Product) string {
	p.Name = strings.TrimSpace(p.Name)
	p.RepoURL = strings.TrimSpace(p.RepoURL)
	p.StagingURL = strings.TrimSpace(p.StagingURL)
	p.RootDir = strings.Trim(strings.TrimSpace(p.RootDir), "/")
	p.Branch = strings.TrimSpace(p.Branch)
	if p.Name == "" || len(p.Name) > 100 {
		return "name is required (max 100 characters)"
	}
	if err := repopool.ValidateRepoURL(p.RepoURL); err != nil {
		return err.Error()
	}
	if u, err := url.Parse(p.StagingURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(p.StagingURL) > 500 {
		return "staging_url must be an http(s) URL without credentials"
	}
	if err := repopool.ValidateRelDir(p.RootDir); err != nil {
		return err.Error()
	}
	if p.Branch != "" {
		if err := repopool.ValidateBranch(p.Branch); err != nil {
			return err.Error()
		}
	}
	return ""
}

// rootDirOverlap reports whether rel equals, contains or sits inside another product's root_dir
// (nested checkouts would write into each other's working tree).
func rootDirOverlap(rel string, others []string) bool {
	for _, o := range others {
		if rel == o || strings.HasPrefix(rel, o+"/") || strings.HasPrefix(o, rel+"/") {
			return true
		}
	}
	return false
}

func (r *Router) otherRootDirs(ctx context.Context, exceptID int) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT root_dir FROM products WHERE id <> ?`, exceptID)
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
	return out, nil
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

// handleProducts serves /api/products and /api/products/{id}[/verify|clone|checkout].
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
			rows, err := d.Query(req.Context(), `SELECT `+productCols+` FROM products ORDER BY name`)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			defer rows.Close()
			out := []models.Product{}
			for rows.Next() {
				p, err := scanProduct(rows)
				if err != nil {
					writeJSONError(w, http.StatusInternalServerError, err.Error())
					return
				}
				out = append(out, p)
			}
			if err := rows.Err(); err != nil {
				log.Printf("products list: %v", err)
				writeJSONError(w, http.StatusInternalServerError, "could not list products")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"products": out, "pool_enabled": r.repoPool != nil})
		case http.MethodPost:
			if !canManage {
				writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
				return
			}
			var p models.Product
			if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&p); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
			if msg := validateProduct(&p); msg != "" {
				writeJSONError(w, http.StatusBadRequest, msg)
				return
			}
			others, err := r.otherRootDirs(req.Context(), 0)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if rootDirOverlap(p.RootDir, others) {
				writeJSONError(w, http.StatusConflict, "root_dir overlaps another product's root_dir")
				return
			}
			res, err := d.Exec(req.Context(),
				`INSERT INTO products (name, repo_url, staging_url, root_dir, branch) VALUES (?, ?, ?, ?, ?)`,
				p.Name, p.RepoURL, p.StagingURL, p.RootDir, p.Branch)
			if err != nil {
				writeProductDBError(w, err)
				return
			}
			r.respondProduct(w, req, int(res.LastInsertId()), http.StatusCreated)
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	parts := strings.Split(rest, "/")
	id, err := strconv.Atoi(parts[0])
	if err != nil || id <= 0 || len(parts) > 2 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch {
	case action == "" && req.Method == http.MethodGet:
		r.respondProduct(w, req, id, http.StatusOK)

	case action == "" && req.Method == http.MethodPut:
		if !canManage {
			writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
			return
		}
		var p models.Product
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<16)).Decode(&p); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if msg := validateProduct(&p); msg != "" {
			writeJSONError(w, http.StatusBadRequest, msg)
			return
		}
		old, ok := r.loadProduct(w, req, id)
		if !ok {
			return
		}
		others, err := r.otherRootDirs(req.Context(), id)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if rootDirOverlap(p.RootDir, others) {
			writeJSONError(w, http.StatusConflict, "root_dir overlaps another product's root_dir")
			return
		}
		// Changing the repo, directory or URL invalidates the previous verification.
		// Decided here (not in SQL): MySQL evaluates SET assignments left to right,
		// so a comparison against the column would already see the new value.
		_, err = d.Exec(req.Context(), `UPDATE products SET name=?, repo_url=?, staging_url=?, root_dir=?, branch=? WHERE id=?`,
			p.Name, p.RepoURL, p.StagingURL, p.RootDir, p.Branch, id)
		if err != nil {
			writeProductDBError(w, err)
			return
		}
		if productVerificationStale(old, p) {
			if _, err := d.Exec(req.Context(), `UPDATE products SET playwright_accessible=0, last_verified_at=NULL, verify_report=NULL WHERE id=?`, id); err != nil {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		r.respondProduct(w, req, id, http.StatusOK)

	case action == "" && req.Method == http.MethodDelete:
		if !canManage {
			writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
			return
		}
		// Deletes the registry row only; the checkout stays on disk.
		if _, err := d.Exec(req.Context(), `DELETE FROM products WHERE id=?`, id); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case action == "verify" && req.Method == http.MethodPost:
		if !canVerify {
			writeJSONError(w, http.StatusForbidden, "your role cannot verify products")
			return
		}
		if r.repoPool == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "repo pool is not configured (REPO_POOL_DIR)")
			return
		}
		p, ok := r.loadProduct(w, req, id)
		if !ok {
			return
		}
		if err := r.verifyAndStore(req.Context(), p); err != nil {
			if errors.Is(err, repopool.ErrBusy) {
				writeJSONError(w, http.StatusConflict, repopool.ErrBusy.Error())
				return
			}
			log.Printf("products verify %q: %v", p.Name, err)
			writeJSONError(w, http.StatusInternalServerError, "verify failed")
			return
		}
		r.respondProduct(w, req, id, http.StatusOK)

	case (action == "clone" || action == "checkout") && req.Method == http.MethodPost:
		if !canManage {
			writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
			return
		}
		if r.repoPool == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "repo pool is not configured (REPO_POOL_DIR)")
			return
		}
		p, ok := r.loadProduct(w, req, id)
		if !ok {
			return
		}
		// A client disconnect must not kill gh/git mid-operation (it would leave a partial
		// checkout), so the operation runs on a context that ignores request cancellation.
		opCtx := context.WithoutCancel(req.Context())
		var opErr error
		if action == "clone" {
			opErr = r.repoPool.Clone(opCtx, p.RepoURL, p.RootDir)
			if opErr == nil && p.Branch != "" {
				if _, berr := r.repoPool.Checkout(opCtx, p.RootDir, p.Branch); berr != nil {
					// The clone itself succeeded: say so and point at Switch branch.
					log.Printf("products clone %q: branch step failed: %v", p.Name, berr)
					opErr = &repopool.InputError{Msg: "cloned, but switching to branch " + p.Branch + " failed; use Switch branch"}
				}
			}
		} else {
			var body struct {
				Branch string `json:"branch"`
			}
			_ = json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<12)).Decode(&body)
			branch := strings.TrimSpace(body.Branch)
			if branch == "" {
				branch = p.Branch
			}
			if branch == "" {
				writeJSONError(w, http.StatusBadRequest, "branch is required")
				return
			}
			if _, opErr = r.repoPool.Checkout(opCtx, p.RootDir, branch); opErr == nil {
				_, opErr = d.Exec(req.Context(), `UPDATE products SET branch=? WHERE id=?`, branch, id)
			}
		}
		if opErr != nil {
			var inErr *repopool.InputError
			status := http.StatusBadGateway
			switch {
			case errors.Is(opErr, repopool.ErrBusy):
				status = http.StatusConflict
			case errors.As(opErr, &inErr):
				status = http.StatusBadRequest
			}
			log.Printf("products %s %q failed: %v", action, p.Name, opErr)
			writeJSONError(w, status, firstLine(opErr))
			return
		}
		// Refresh the verification after a successful operation (best effort).
		if p2, err := r.fetchProduct(opCtx, id); err == nil {
			_ = r.verifyAndStore(opCtx, p2)
		}
		r.respondProduct(w, req, id, http.StatusOK)

	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

// productVerificationStale reports whether an edit changes anything verification depends on.
func productVerificationStale(old, next models.Product) bool {
	return old.RepoURL != next.RepoURL || old.StagingURL != next.StagingURL || old.RootDir != next.RootDir
}

func writeProductDBError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "Duplicate entry") {
		writeJSONError(w, http.StatusConflict, "a product with this name or root_dir already exists")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, err.Error())
}

func (r *Router) fetchProduct(ctx context.Context, id int) (models.Product, error) {
	return scanProduct(r.db.QueryRow(ctx, `SELECT `+productCols+` FROM products WHERE id=?`, id))
}

func (r *Router) loadProduct(w http.ResponseWriter, req *http.Request, id int) (models.Product, bool) {
	p, err := r.fetchProduct(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, "product not found")
		return p, false
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return p, false
	}
	return p, true
}

func (r *Router) respondProduct(w http.ResponseWriter, req *http.Request, id, status int) {
	p, ok := r.loadProduct(w, req, id)
	if !ok {
		return
	}
	writeJSON(w, status, p)
}

// verifyAndStore runs the pool checks for p and persists the report.
func (r *Router) verifyAndStore(ctx context.Context, p models.Product) error {
	rep := r.repoPool.Verify(ctx, p.RepoURL, p.RootDir, p.StagingURL)
	if rep.Busy {
		return repopool.ErrBusy
	}
	b, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	// Conditional on the inputs we verified: a PUT that changed repo/URL/dir meanwhile
	// must not be overwritten by a stale result.
	_, err = r.db.Exec(ctx, `UPDATE products SET playwright_accessible=?, last_verified_at=UTC_TIMESTAMP(), verify_report=?
		WHERE id=? AND repo_url=? AND staging_url=? AND root_dir=?`,
		rep.OK, string(b), p.ID, p.RepoURL, p.StagingURL, p.RootDir)
	return err
}

// StartProductVerifier re-verifies every product on an interval until ctx ends.
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

func (r *Router) verifyAllProducts(ctx context.Context) {
	rows, err := r.db.Query(ctx, `SELECT `+productCols+` FROM products`)
	if err != nil {
		log.Printf("products verify: %v", err)
		return
	}
	var list []models.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			log.Printf("products verify: scan: %v", err)
			continue
		}
		list = append(list, p)
	}
	rows.Close()
	for _, p := range list {
		if err := r.verifyAndStore(ctx, p); err != nil && !errors.Is(err, repopool.ErrBusy) {
			log.Printf("products verify %q: %v", p.Name, err)
		}
	}
}
