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
		// Auth is only wired when the DB is connected, and products need the DB,
		// so an absent user means auth is off in a dev setup: allow.
		return true, true
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
	if u, err := url.Parse(p.StagingURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(p.StagingURL) > 500 {
		return "staging_url must be an http(s) URL"
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
		// Changing the repo, directory or URL invalidates the previous verification.
		// Decided here (not in SQL): MySQL evaluates SET assignments left to right,
		// so a comparison against the column would already see the new value.
		_, err := d.Exec(req.Context(), `UPDATE products SET name=?, repo_url=?, staging_url=?, root_dir=?, branch=? WHERE id=?`,
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
			writeJSONError(w, http.StatusInternalServerError, err.Error())
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
		var opErr error
		if action == "clone" {
			opErr = r.repoPool.Clone(req.Context(), p.RepoURL, p.RootDir)
			if opErr == nil && p.Branch != "" {
				_, opErr = r.repoPool.Checkout(req.Context(), p.RootDir, p.Branch)
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
			if _, opErr = r.repoPool.Checkout(req.Context(), p.RootDir, branch); opErr == nil {
				_, opErr = d.Exec(req.Context(), `UPDATE products SET branch=? WHERE id=?`, branch, id)
			}
		}
		if opErr != nil {
			status := http.StatusBadGateway
			if errors.Is(opErr, repopool.ErrBusy) {
				status = http.StatusConflict
			} else if strings.Contains(opErr.Error(), "invalid") || strings.Contains(opErr.Error(), "already exists") || strings.Contains(opErr.Error(), "not a git repository") {
				status = http.StatusBadRequest
			}
			writeJSONError(w, status, opErr.Error())
			return
		}
		// Refresh the verification after a successful operation (best effort).
		if p2, err := r.fetchProduct(req.Context(), id); err == nil {
			_ = r.verifyAndStore(req.Context(), p2)
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
	b, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `UPDATE products SET playwright_accessible=?, last_verified_at=UTC_TIMESTAMP(), verify_report=? WHERE id=?`,
		rep.OK, string(b), p.ID)
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
		if p, err := scanProduct(rows); err == nil {
			list = append(list, p)
		}
	}
	rows.Close()
	for _, p := range list {
		if err := r.verifyAndStore(ctx, p); err != nil {
			log.Printf("products verify %q: %v", p.Name, err)
		}
	}
}
