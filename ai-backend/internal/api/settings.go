package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
	_ "time/tzdata" // zone database for time.LoadLocation on hosts without one
)

// DefaultTimezone is the system timezone until app_settings says otherwise.
const DefaultTimezone = "Asia/Jakarta"

// SettingsInfo is the NON-SECRET configuration snapshot exposed at
// GET /api/settings. Never add credentials, tokens or DSNs here.
type SettingsInfo struct {
	AIProvider      string // AI_PROVIDER
	ManagerURL      string // AI_MANAGER_URL (host only is returned)
	ManagerProvider string // AI_MANAGER_PROVIDER (expected: claude-v3)
	ManagerModel    string // AI_MANAGER_MODEL ("" = upstream default)
	NoFallback      bool   // AI_MANAGER_NO_FALLBACK
	TimeoutSeconds  int    // AI_TIMEOUT_SECONDS
	ExecutorURL     string // PLAYWRIGHT_EXECUTOR_URL (host only is returned)
}

// SetSettings stores the snapshot served by /api/settings.
func (r *Router) SetSettings(s SettingsInfo) { r.settings = s }

type settingsAI struct {
	Provider         string `json:"provider"`
	Gateway          string `json:"gateway"`
	GatewayHost      string `json:"gateway_host"`
	Upstream         string `json:"upstream"`
	Model            string `json:"model"`
	NoFallback       bool   `json:"no_fallback"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	Health           string `json:"health"`
	PinnedToClaudeV3 bool   `json:"pinned_to_claude_v3"`
}

type settingsResponse struct {
	AI       settingsAI        `json:"ai"`
	Executor map[string]string `json:"executor"`
	Database string            `json:"database"`
	Timezone string            `json:"timezone"`
}

// systemTimezone reads app_settings.timezone; a missing table, row or an unknown zone gives
// DefaultTimezone.
func (r *Router) systemTimezone(ctx context.Context) string {
	d := r.getDB()
	if d == nil {
		return DefaultTimezone
	}
	var tz string
	if err := d.QueryRow(ctx, `SELECT v FROM app_settings WHERE k = 'timezone'`).Scan(&tz); err != nil {
		return DefaultTimezone
	}
	if validTimezone(tz) != nil {
		return DefaultTimezone
	}
	return tz
}

func validTimezone(tz string) error {
	if tz == "" || strings.EqualFold(tz, "local") {
		return errBadZone
	}
	_, err := time.LoadLocation(tz)
	return err
}

type zoneError struct{}

func (zoneError) Error() string { return "unknown timezone" }

var errBadZone error = zoneError{}

// handleSettingsTimezone serves PUT /api/settings/timezone {timezone} (Admin, DevOps).
func (r *Router) handleSettingsTimezone(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPut {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !canToggleWork(req) {
		writeJSONError(w, http.StatusForbidden, "only Admin or DevOps can change the timezone")
		return
	}
	var body struct {
		Timezone string `json:"timezone"`
	}
	if !decodeStrictJSON(w, req, 1<<10, &body) {
		return
	}
	body.Timezone = strings.TrimSpace(body.Timezone)
	if validTimezone(body.Timezone) != nil {
		writeJSONError(w, http.StatusBadRequest, "timezone must be an IANA zone name such as Asia/Jakarta")
		return
	}
	d := r.getDB()
	if d == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	if _, err := d.Exec(req.Context(), `INSERT INTO app_settings (k, v, updated_by, updated_at) VALUES ('timezone', ?, ?, UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE v = VALUES(v), updated_by = VALUES(updated_by), updated_at = VALUES(updated_at)`, body.Timezone, userID(req)); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"timezone": body.Timezone})
}

// hostOnly strips scheme, userinfo, path and query so nothing sensitive leaks.
func hostOnly(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func (r *Router) handleSettings(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s := r.settings
	ctx := context.Background()

	aiHealth := "not_configured"
	upstream := ""
	if r.aiService != nil {
		upstream = r.aiService.Upstream()
		aiHealth = r.aiService.Health(ctx)
	}
	model := s.ManagerModel
	if model == "" {
		model = "(claude-v3 default)"
	}
	execStatus := "not_configured"
	if r.executor != nil {
		execStatus = r.executor.Health(ctx)
	}
	db := "disconnected"
	if r.db != nil {
		db = "connected"
	}

	resp := settingsResponse{
		AI: settingsAI{
			Provider:         s.AIProvider,
			Gateway:          "ai-manager",
			GatewayHost:      hostOnly(s.ManagerURL),
			Upstream:         upstream,
			Model:            model,
			NoFallback:       s.NoFallback,
			TimeoutSeconds:   s.TimeoutSeconds,
			Health:           aiHealth,
			PinnedToClaudeV3: s.AIProvider == "ai-manager" && s.ManagerProvider == "claude-v3" && s.NoFallback,
		},
		Executor: map[string]string{
			"host":   hostOnly(s.ExecutorURL),
			"status": execStatus,
		},
		Database: db,
		Timezone: r.systemTimezone(ctx),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
