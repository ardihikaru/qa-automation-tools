package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/gaossync"
)

// gaosWait is how long POST /api/products/{id}/gaos/sync waits for the push before it answers 202.
// It stays below the 300 s of the dashboard nginx and the 330 s of the edge.
const gaosWait = 280 * time.Second

// applyGAOSSettings applies gaos_integrated / gaos_product_id inside the product transaction and
// reports whether a push must start after the commit. Checks, in order: role (403, only when a
// value changes), id shape (400), turning on needs an id and a configured server (400).
func (r *Router) applyGAOSSettings(ctx context.Context, tx queryer, id int, wantOn *bool, wantPID *string, allowed bool) (trigger bool, err error) {
	var wasOn bool
	var wasPID string
	err = tx.QueryRowContext(ctx, `SELECT gaos_integrated, gaos_product_id FROM product_settings WHERE product_id = ?`, id).Scan(&wasOn, &wasPID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	nowOn, nowPID := wasOn, wasPID
	if wantOn != nil {
		nowOn = *wantOn
	}
	if wantPID != nil {
		nowPID = *wantPID
	}
	onChanged, pidChanged := nowOn != wasOn, nowPID != wasPID
	if !onChanged && !pidChanged {
		return false, nil
	}
	if !allowed {
		if onChanged {
			return false, &apiError{http.StatusForbidden, "only Admin and DevOps can change gaos_integrated"}
		}
		return false, &apiError{http.StatusForbidden, "only Admin and DevOps can change gaos_product_id"}
	}
	if nowPID != "" && !gaossync.ValidProductID(nowPID) {
		return false, badRequest("gaos_product_id must look like p-6")
	}
	if nowOn {
		if nowPID == "" {
			return false, badRequest("gaos_product_id is required to turn the integration on")
		}
		if !r.gaos.Configured() {
			return false, badRequest("GAOS integration is not configured on this server (GAOS_API_URL and GAOS_API_TOKEN)")
		}
	}
	q := `INSERT INTO product_settings (product_id, gaos_integrated, gaos_product_id) VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE gaos_integrated = VALUES(gaos_integrated), gaos_product_id = VALUES(gaos_product_id)`
	if pidChanged {
		// A different GAOS product: the last outcome belongs to the old one.
		q += `, gaos_last_sync_at = NULL, gaos_last_status = '', gaos_last_note = ''`
	}
	if _, err := tx.ExecContext(ctx, q, id, nowOn, nowPID); err != nil {
		return false, err
	}
	return nowOn && (!wasOn || pidChanged), nil
}

// handleGAOSSync serves POST /api/products/{id}/gaos/sync: push the product's test cases now.
func (r *Router) handleGAOSSync(w http.ResponseWriter, req *http.Request, id int) {
	if req.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !canToggleWork(req) {
		writeJSONError(w, http.StatusForbidden, "only Admin and DevOps can sync a product to GAOS")
		return
	}
	run, err := r.gaos.Start(id)
	switch {
	case errors.Is(err, gaossync.ErrNotConfigured):
		writeJSONError(w, http.StatusBadRequest, "GAOS integration is not configured on this server (GAOS_API_URL and GAOS_API_TOKEN)")
		return
	case errors.Is(err, gaossync.ErrNotIntegrated):
		writeJSONError(w, http.StatusConflict, "product is not integrated with GAOS")
		return
	case errors.Is(err, gaossync.ErrBusy):
		writeJSONError(w, http.StatusConflict, "a push to GAOS is already running for this product")
		return
	case err != nil:
		writeOpError(w, "gaos sync", err)
		return
	}
	timer := time.NewTimer(gaosWait)
	defer timer.Stop()
	select {
	case <-run.Done:
		writeJSON(w, http.StatusOK, run.Result())
	case <-timer.C:
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "running"})
	}
	// A client that went away does not stop the push: it runs on its own context.
}

// handleGAOSStatus serves GET /api/products/{id}/gaos/status: a live, read-only check that GAOS
// accepts our token and lists this product's GAOS id. It never changes anything.
func (r *Router) handleGAOSStatus(w http.ResponseWriter, req *http.Request, id int) {
	if req.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d := r.getDB()
	if d == nil || r.gaos == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "not available")
		return
	}
	var on bool
	var pid string
	err := d.SQL.QueryRowContext(req.Context(), `SELECT gaos_integrated, gaos_product_id FROM product_settings WHERE product_id = ?`, id).Scan(&on, &pid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeOpError(w, "gaos status", err)
		return
	}
	out := map[string]any{"configured": r.gaos.Configured(), "integrated": on, "product_id": pid, "web_url": r.gaos.WebURL()}
	switch {
	case !on || pid == "":
		out["state"], out["note"] = "not_integrated", "the product is not linked to GAOS"
	case !r.gaos.Configured():
		out["state"], out["note"] = gaossync.LinkUnreachable, "GAOS is not configured on this server"
	default:
		ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
		defer cancel()
		out["state"], out["note"] = r.gaos.CheckLink(ctx, pid)
	}
	writeJSON(w, http.StatusOK, out)
}
