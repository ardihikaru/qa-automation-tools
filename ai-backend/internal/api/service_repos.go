package api

// QA_DEFECT_TRIAGE.md: the product-level mapping "service directory of the automation repo -> product
// repositories". An e2e case adds the repositories of its app directory's backends to its target
// list in the defect push. Admin route, dashboard-free:
//   GET /api/products/{id}/service-repos   -> {"items":[{"service_dir","repos":[name...]}]}
//   PUT /api/products/{id}/service-repos   body {"service_dir","repos":[name...]} replaces that directory's
//                                          repos; an empty list removes the directory. Admin, DevOps, Developer.

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

var serviceDirRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

const maxServiceRepos = 8

type serviceReposBody struct {
	ServiceDir string   `json:"service_dir"`
	Repos      []string `json:"repos"`
}

func (r *Router) handleServiceRepos(w http.ResponseWriter, req *http.Request, pid int, canManage bool) {
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx := req.Context()
	switch req.Method {
	case http.MethodGet:
		rows, err := d.Query(ctx, `SELECT service_dir, repo_name FROM product_service_repos WHERE product_id = ? ORDER BY service_dir, repo_name`, pid)
		if err != nil {
			writeOpError(w, "service repos", err)
			return
		}
		defer rows.Close()
		items := []serviceReposBody{}
		for rows.Next() {
			var dir, name string
			if err := rows.Scan(&dir, &name); err != nil {
				writeOpError(w, "service repos", err)
				return
			}
			if n := len(items); n > 0 && items[n-1].ServiceDir == dir {
				items[n-1].Repos = append(items[n-1].Repos, name)
			} else {
				items = append(items, serviceReposBody{ServiceDir: dir, Repos: []string{name}})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPut:
		if !canManage {
			writeJSONError(w, http.StatusForbidden, "your role cannot manage products")
			return
		}
		var in serviceReposBody
		if !decodeStrictJSON(w, req, 1<<14, &in) {
			return
		}
		if !serviceDirRe.MatchString(in.ServiceDir) || len(in.Repos) > maxServiceRepos {
			writeJSONError(w, http.StatusUnprocessableEntity, "service_dir must be a directory name and repos at most 8 repository names")
			return
		}
		seen := map[string]bool{}
		for _, n := range in.Repos {
			var one int
			if seen[n] || d.QueryRow(ctx, `SELECT 1 FROM product_repos WHERE product_id = ? AND name = ?`, pid, n).Scan(&one) != nil {
				writeJSONError(w, http.StatusUnprocessableEntity, "every repo must be a distinct repository of this product (by name)")
				return
			}
			seen[n] = true
		}
		err := d.WithTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `DELETE FROM product_service_repos WHERE product_id = ? AND service_dir = ?`, pid, in.ServiceDir); err != nil {
				return err
			}
			for _, n := range in.Repos {
				if _, err := tx.ExecContext(ctx, `INSERT INTO product_service_repos (product_id, service_dir, repo_name) VALUES (?,?,?)`, pid, in.ServiceDir, n); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			writeOpError(w, "service repos", err)
			return
		}
		if in.Repos == nil {
			in.Repos = []string{}
		}
		writeJSON(w, http.StatusOK, in)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// SpecReader reads a file of the product's automation repository from the repo pool, for the defect
// push's operation extraction. A path outside the repository, a link out of it or a missing pool is an error.
func (r *Router) SpecReader() gaossync.SpecReader {
	return func(ctx context.Context, pid int, rel string) ([]byte, error) {
		d := r.getDB()
		if d == nil || r.repoPool == nil {
			return nil, os.ErrNotExist
		}
		if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, `\`) {
			return nil, os.ErrInvalid
		}
		for _, seg := range strings.Split(rel, "/") {
			if seg == ".." {
				return nil, os.ErrInvalid
			}
		}
		repo, err := loadAutomationRepo(ctx, d, pid)
		if err != nil || repo == nil {
			return nil, os.ErrNotExist
		}
		root, err := r.repoPool.ResolveDir(repo.RootDir)
		if err != nil {
			return nil, err
		}
		full, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		if full != realRoot && !strings.HasPrefix(full, realRoot+string(filepath.Separator)) {
			return nil, os.ErrInvalid
		}
		st, err := os.Stat(full)
		if err != nil || !st.Mode().IsRegular() || st.Size() > 256<<10 {
			return nil, os.ErrInvalid
		}
		return os.ReadFile(full)
	}
}
