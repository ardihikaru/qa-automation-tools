package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/learning"
	"github.com/qa-automation/ai-backend/internal/services/skills"
)

var (
	crudSetupOnce   sync.Once
	crudSetupRouter *Router
	crudSetupDB     *db.DB
	crudSetupErr    error
	crudSkipReason  string
)

func setupCRUDIntegrationRouter(t *testing.T) (*Router, *db.DB) {
	t.Helper()

	crudSetupOnce.Do(func() {
		if reason := integrationOptOut(); reason != "" {
			crudSkipReason = reason
			return
		}
		loadIntegrationEnvFiles()
		config.ClearCache()

		cfg := config.LoadConfig()
		if cfg.GetDatabaseURL() == "" {
			crudSkipReason = "Skipping CRUD integration tests: DATABASE_URL is not configured"
			return
		}

		database, err := db.NewConnection(cfg)
		if err != nil {
			crudSkipReason = fmt.Sprintf("Skipping CRUD integration tests: database not reachable: %v", err)
			return
		}

		if err := db.RunMigrations(database.SQL); err != nil {
			database.Close()
			crudSetupErr = fmt.Errorf("failed to run migrations for CRUD integration tests: %w", err)
			return
		}

		learningService := learning.NewService(database)
		skillsService := skills.NewService(database, nil)

		crudSetupDB = database
		crudSetupRouter = NewRouter(database, nil, learningService, skillsService)
	})

	if crudSetupErr != nil {
		t.Fatalf("%v", crudSetupErr)
	}
	if crudSkipReason != "" {
		t.Skip(crudSkipReason)
	}
	return crudSetupRouter, crudSetupDB
}

func TestMain(m *testing.M) {
	code := m.Run()
	if crudSetupDB != nil {
		crudSetupDB.Close()
	}
	os.Exit(code)
}

func doJSONRequest(t *testing.T, router *Router, method, path string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()

	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}
		body = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestDeleteEndpointsReturn404ForMissingTargetsIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	testCases := []struct {
		name string
		path string
	}{
		{name: "MissingLearning", path: "/api/learning?id=99999999"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSONRequest(t, router, http.MethodDelete, tc.path, nil)
			if resp.Code != http.StatusNotFound {
				t.Fatalf("Expected 404 for %s. status=%d body=%s", tc.path, resp.Code, resp.Body.String())
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
				t.Fatalf("Expected JSON error response for %s: %v body=%s", tc.path, err, resp.Body.String())
			}
			if _, ok := payload["error"].(string); !ok {
				t.Fatalf("Expected error key in response for %s. payload=%v", tc.path, payload)
			}
		})
	}
}

func TestSkillsQueryContractIntegration(t *testing.T) {
	router, database := setupCRUDIntegrationRouter(t)
	ctx := context.Background()

	// Ensure the test can assert limit and filter deterministically.
	seedNameA := fmt.Sprintf("integration-skill-a-%d", time.Now().UnixNano())
	seedNameB := fmt.Sprintf("integration-skill-b-%d", time.Now().UnixNano())
	_, err := database.Exec(ctx, `
		INSERT IGNORE INTO skills (name, description, check_type, check_query, severity, is_builtin, is_active)
		VALUES (?, ?, 'validation', '.a', 'high', false, true),
		       (?, ?, 'validation', '.b', 'low', false, true)
	`, seedNameA, "integration seed A", seedNameB, "integration seed B")
	if err != nil {
		t.Fatalf("Failed seeding skills: %v", err)
	}
	defer database.Exec(ctx, `DELETE FROM skills WHERE name IN (?, ?)`, seedNameA, seedNameB)

	limited := doJSONRequest(t, router, http.MethodGet, "/api/skills?limit=1&page=1&sort_by=name_desc", nil)
	if limited.Code != http.StatusOK {
		t.Fatalf("Skills query failed. status=%d body=%s", limited.Code, limited.Body.String())
	}

	var limitedPayload map[string]interface{}
	if err := json.Unmarshal(limited.Body.Bytes(), &limitedPayload); err != nil {
		t.Fatalf("Failed parsing skills payload: %v", err)
	}
	limitedData, ok := limitedPayload["data"].([]interface{})
	if !ok {
		t.Fatalf("Skills payload missing data array")
	}
	if len(limitedData) != 1 {
		t.Fatalf("Expected exactly 1 row for limit=1, got %d", len(limitedData))
	}
	if _, ok := limitedPayload["pagination"].(map[string]interface{}); !ok {
		t.Fatalf("Skills payload missing pagination object")
	}

	filtered := doJSONRequest(t, router, http.MethodGet, "/api/skills?search="+url.QueryEscape(seedNameA)+"&limit=10&page=1", nil)
	if filtered.Code != http.StatusOK {
		t.Fatalf("Filtered skills query failed. status=%d body=%s", filtered.Code, filtered.Body.String())
	}
	var filteredPayload map[string]interface{}
	if err := json.Unmarshal(filtered.Body.Bytes(), &filteredPayload); err != nil {
		t.Fatalf("Failed parsing filtered skills payload: %v", err)
	}
	filteredData, ok := filteredPayload["data"].([]interface{})
	if !ok {
		t.Fatalf("Filtered skills payload missing data array")
	}
	found := false
	for _, raw := range filteredData {
		item := raw.(map[string]interface{})
		if name, ok := item["name"].(string); ok && name == seedNameA {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Expected to find seeded skill %q in filtered response", seedNameA)
	}
}

func TestSkillStatsAndLogsQueryContractIntegration(t *testing.T) {
	router, database := setupCRUDIntegrationRouter(t)
	ctx := context.Background()

	skillName := fmt.Sprintf("integration-stats-skill-%d", time.Now().UnixNano())
	skillRes, err := database.Exec(ctx, `
		INSERT INTO skills (name, description, check_type, check_query, severity, is_builtin, is_active)
		VALUES (?, 'integration stats/logs seed', 'validation', '.integration', 'medium', false, true)
	`, skillName)
	if err != nil {
		t.Fatalf("Failed creating skill seed: %v", err)
	}
	skillID := int(skillRes.LastInsertId())
	defer database.Exec(ctx, `DELETE FROM skills WHERE id = ?`, skillID)

	_, err = database.Exec(ctx, `
		INSERT INTO skill_logs (skill_id, page, action, result, is_passed, error_message)
		VALUES (?, '/integration/seed-a', 'click', 'ok', true, NULL),
		       (?, '/integration/seed-b', 'type', 'error', false, 'failed seed log')
	`, skillID, skillID)
	if err != nil {
		t.Fatalf("Failed inserting skill logs seed: %v", err)
	}

	statsResp := doJSONRequest(t, router, http.MethodGet, "/api/skills/stats?search="+url.QueryEscape(skillName)+"&limit=1&page=1&sort_by=total_runs_desc", nil)
	if statsResp.Code != http.StatusOK {
		t.Fatalf("Skill stats query failed. status=%d body=%s", statsResp.Code, statsResp.Body.String())
	}

	var statsPayload map[string]interface{}
	if err := json.Unmarshal(statsResp.Body.Bytes(), &statsPayload); err != nil {
		t.Fatalf("Failed parsing skill stats payload: %v", err)
	}
	statsData, ok := statsPayload["data"].([]interface{})
	if !ok {
		t.Fatalf("Skill stats payload missing data array")
	}
	if len(statsData) > 1 {
		t.Fatalf("Expected stats limit=1 to return at most 1 row, got %d", len(statsData))
	}
	if _, ok := statsPayload["pagination"].(map[string]interface{}); !ok {
		t.Fatalf("Skill stats payload missing pagination object")
	}

	logsResp := doJSONRequest(t, router, http.MethodGet, "/api/skills/logs?search="+url.QueryEscape("/integration/seed-")+"&limit=1&page=1", nil)
	if logsResp.Code != http.StatusOK {
		t.Fatalf("Skill logs query failed. status=%d body=%s", logsResp.Code, logsResp.Body.String())
	}

	var logsPayload map[string]interface{}
	if err := json.Unmarshal(logsResp.Body.Bytes(), &logsPayload); err != nil {
		t.Fatalf("Failed parsing skill logs payload: %v", err)
	}
	logsData, ok := logsPayload["data"].([]interface{})
	if !ok {
		t.Fatalf("Skill logs payload missing data array")
	}
	if len(logsData) > 1 {
		t.Fatalf("Expected logs limit=1 to return at most 1 row, got %d", len(logsData))
	}
	if _, ok := logsPayload["pagination"].(map[string]interface{}); !ok {
		t.Fatalf("Skill logs payload missing pagination object")
	}
}

func TestLearningCRUDIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	intent := fmt.Sprintf("integration-intent-%d", time.Now().UnixNano())
	action := "integration-action"
	page := "integration-page"

	create := doJSONRequest(t, router, http.MethodPost, "/api/learning", map[string]interface{}{
		"intent":  intent,
		"action":  action,
		"page":    page,
		"success": true,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("Create learning record failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing create learning response: %v", err)
	}
	learningID := int(created["id"].(float64))

	list := doJSONRequest(t, router, http.MethodGet, "/api/learning?intent="+url.QueryEscape(intent)+"&limit=50", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("List learning records failed. status=%d body=%s", list.Code, list.Body.String())
	}

	var records []map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &records); err != nil {
		t.Fatalf("Failed parsing learning list response: %v", err)
	}

	found := false
	for _, record := range records {
		if int(record["id"].(float64)) == learningID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Created learning record id=%d not found in /api/learning", learningID)
	}

	del := doJSONRequest(t, router, http.MethodDelete, fmt.Sprintf("/api/learning?id=%d", learningID), nil)
	if del.Code != http.StatusOK {
		t.Fatalf("Delete learning record failed. status=%d body=%s", del.Code, del.Body.String())
	}
}

func TestLearningUpdateExistingRecordRegression(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	intent := fmt.Sprintf("learning-update-regression-%d", time.Now().UnixNano())
	action := "click"
	page := "/qa"

	create := doJSONRequest(t, router, http.MethodPost, "/api/learning", map[string]interface{}{
		"intent":  intent,
		"action":  action,
		"page":    page,
		"success": true,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("Create learning record failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing create learning response: %v", err)
	}
	learningID := int(created["id"].(float64))

	update1 := doJSONRequest(t, router, http.MethodPost, "/api/learning", map[string]interface{}{
		"intent":  intent,
		"action":  action,
		"page":    page,
		"success": true,
	})
	if update1.Code != http.StatusOK {
		t.Fatalf("First update failed. status=%d body=%s", update1.Code, update1.Body.String())
	}

	listAfterUpdate1 := doJSONRequest(t, router, http.MethodGet, "/api/learning?intent="+url.QueryEscape(intent)+"&limit=50", nil)
	if listAfterUpdate1.Code != http.StatusOK {
		t.Fatalf("List after first update failed. status=%d body=%s", listAfterUpdate1.Code, listAfterUpdate1.Body.String())
	}

	var recordsAfterUpdate1 []map[string]interface{}
	if err := json.Unmarshal(listAfterUpdate1.Body.Bytes(), &recordsAfterUpdate1); err != nil {
		t.Fatalf("Failed parsing learning list after first update: %v", err)
	}

	var foundAfterUpdate1 map[string]interface{}
	for _, record := range recordsAfterUpdate1 {
		if int(record["id"].(float64)) == learningID {
			foundAfterUpdate1 = record
			break
		}
	}
	if foundAfterUpdate1 == nil {
		t.Fatalf("Learning record id=%d not found after first update", learningID)
	}

	scoreAfterUpdate1 := foundAfterUpdate1["success_score"].(float64)
	usageAfterUpdate1 := int(foundAfterUpdate1["usage_count"].(float64))
	if scoreAfterUpdate1 != 1.0 {
		t.Fatalf("Expected success_score=1.0 after successful update, got %.4f", scoreAfterUpdate1)
	}
	if usageAfterUpdate1 != 2 {
		t.Fatalf("Expected usage_count=2 after first update, got %d", usageAfterUpdate1)
	}

	update2 := doJSONRequest(t, router, http.MethodPost, "/api/learning", map[string]interface{}{
		"intent":  intent,
		"action":  action,
		"page":    page,
		"success": false,
	})
	if update2.Code != http.StatusOK {
		t.Fatalf("Second update failed. status=%d body=%s", update2.Code, update2.Body.String())
	}

	listAfterUpdate2 := doJSONRequest(t, router, http.MethodGet, "/api/learning?intent="+url.QueryEscape(intent)+"&limit=50", nil)
	if listAfterUpdate2.Code != http.StatusOK {
		t.Fatalf("List after second update failed. status=%d body=%s", listAfterUpdate2.Code, listAfterUpdate2.Body.String())
	}

	var recordsAfterUpdate2 []map[string]interface{}
	if err := json.Unmarshal(listAfterUpdate2.Body.Bytes(), &recordsAfterUpdate2); err != nil {
		t.Fatalf("Failed parsing learning list after second update: %v", err)
	}

	var foundAfterUpdate2 map[string]interface{}
	for _, record := range recordsAfterUpdate2 {
		if int(record["id"].(float64)) == learningID {
			foundAfterUpdate2 = record
			break
		}
	}
	if foundAfterUpdate2 == nil {
		t.Fatalf("Learning record id=%d not found after second update", learningID)
	}

	scoreAfterUpdate2 := foundAfterUpdate2["success_score"].(float64)
	usageAfterUpdate2 := int(foundAfterUpdate2["usage_count"].(float64))
	if scoreAfterUpdate2 >= scoreAfterUpdate1 {
		t.Fatalf("Expected success_score to decrease after failed update. before=%.4f after=%.4f", scoreAfterUpdate1, scoreAfterUpdate2)
	}
	if usageAfterUpdate2 != 3 {
		t.Fatalf("Expected usage_count=3 after second update, got %d", usageAfterUpdate2)
	}

	del := doJSONRequest(t, router, http.MethodDelete, fmt.Sprintf("/api/learning?id=%d", learningID), nil)
	if del.Code != http.StatusOK {
		t.Fatalf("Delete learning record failed. status=%d body=%s", del.Code, del.Body.String())
	}
}

func TestJobsListIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	list := doJSONRequest(t, router, http.MethodGet, "/api/jobs?limit=5&page=1", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("List jobs failed. status=%d body=%s", list.Code, list.Body.String())
	}
	var listPayload map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("Failed parsing jobs list response: %v", err)
	}
	if _, ok := listPayload["data"].([]interface{}); !ok {
		t.Fatalf("Jobs list payload missing data array")
	}
	if _, ok := listPayload["pagination"].(map[string]interface{}); !ok {
		t.Fatalf("Jobs list payload missing pagination object")
	}
}
