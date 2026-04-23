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

		if err := db.RunMigrations(database.Pool); err != nil {
			database.Close()
			crudSetupErr = fmt.Errorf("failed to run migrations for CRUD integration tests: %w", err)
			return
		}

		learningService := learning.NewService(database.Pool)
		skillsService := skills.NewService(database.Pool, nil)

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

func TestRolesCRUDIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	roleName := fmt.Sprintf("integration-role-%d", time.Now().UnixNano())
	create := doJSONRequest(t, router, http.MethodPost, "/api/roles", map[string]interface{}{
		"name":        roleName,
		"description": "integration test role",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("Create role failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing create role response: %v", err)
	}
	roleID, ok := created["id"].(float64)
	if !ok || roleID <= 0 {
		t.Fatalf("Invalid role id in response: %#v", created["id"])
	}

	list := doJSONRequest(t, router, http.MethodGet, "/api/roles", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("List roles failed. status=%d body=%s", list.Code, list.Body.String())
	}

	var roles []map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &roles); err != nil {
		t.Fatalf("Failed parsing roles response: %v", err)
	}

	found := false
	for _, role := range roles {
		if int(role["id"].(float64)) == int(roleID) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Created role id=%d not found in /api/roles", int(roleID))
	}

	del := doJSONRequest(t, router, http.MethodDelete, fmt.Sprintf("/api/roles?id=%d", int(roleID)), nil)
	if del.Code != http.StatusOK {
		t.Fatalf("Delete role failed. status=%d body=%s", del.Code, del.Body.String())
	}
}

func TestDeleteEndpointsReturn404ForMissingTargetsIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	testCases := []struct {
		name string
		path string
	}{
		{name: "MissingRole", path: "/api/roles?id=99999999"},
		{name: "MissingLearning", path: "/api/learning?id=99999999"},
		{name: "MissingGoalEvent", path: "/api/goal-events?id=99999999"},
		{name: "MissingAICacheByHash", path: "/api/ai-cache?hash=missing-hash-404"},
		{name: "MissingEmbeddingCacheByHash", path: "/api/embedding-cache?hash=missing-hash-404"},
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

func TestFlowsCRUDIntegration(t *testing.T) {
	router, database := setupCRUDIntegrationRouter(t)

	ctx := context.Background()
	roleName := fmt.Sprintf("integration-flow-role-%d", time.Now().UnixNano())
	var roleID int
	if err := database.QueryRow(ctx, `INSERT INTO roles (name, description) VALUES ($1, $2) RETURNING id`, roleName, "flow integration").Scan(&roleID); err != nil {
		t.Fatalf("Failed to create seed role: %v", err)
	}
	defer database.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)

	flowName := fmt.Sprintf("integration-flow-%d", time.Now().UnixNano())
	create := doJSONRequest(t, router, http.MethodPost, "/api/flows", map[string]interface{}{
		"role_id":     roleID,
		"name":        flowName,
		"description": "integration flow",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("Create flow failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing create flow response: %v", err)
	}
	flowID := int(created["id"].(float64))
	defer database.Exec(ctx, `DELETE FROM flows WHERE id = $1`, flowID)

	list := doJSONRequest(t, router, http.MethodGet, "/api/flows", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("List flows failed. status=%d body=%s", list.Code, list.Body.String())
	}

	var flows []map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &flows); err != nil {
		t.Fatalf("Failed parsing flows response: %v", err)
	}

	found := false
	for _, flow := range flows {
		if int(flow["id"].(float64)) == flowID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Created flow id=%d not found in /api/flows", flowID)
	}
}

func TestIssuesCRUDIntegration(t *testing.T) {
	router, database := setupCRUDIntegrationRouter(t)

	ctx := context.Background()
	pageName := fmt.Sprintf("integration-page-%d", time.Now().UnixNano())
	create := doJSONRequest(t, router, http.MethodPost, "/api/issues", map[string]interface{}{
		"page":       pageName,
		"action":     "click_submit",
		"error_type": "validation_error",
		"error_text": "integration error message",
		"category":   "backend",
		"severity":   "high",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("Create issue failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing create issue response: %v", err)
	}
	issueID := int(created["id"].(float64))
	defer database.Exec(ctx, `DELETE FROM raw_issues WHERE id = $1`, issueID)

	status := doJSONRequest(t, router, http.MethodPost, "/api/issues/status", map[string]interface{}{
		"id":       issueID,
		"is_fixed": true,
	})
	if status.Code != http.StatusOK {
		t.Fatalf("Update issue status failed. status=%d body=%s", status.Code, status.Body.String())
	}

	note := "integration-note"
	notes := doJSONRequest(t, router, http.MethodPost, "/api/issues/notes", map[string]interface{}{
		"id":    issueID,
		"notes": note,
	})
	if notes.Code != http.StatusOK {
		t.Fatalf("Update issue notes failed. status=%d body=%s", notes.Code, notes.Body.String())
	}

	list := doJSONRequest(t, router, http.MethodGet, "/api/issues?search="+url.QueryEscape(pageName)+"&limit=50", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("List issues failed. status=%d body=%s", list.Code, list.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Failed parsing issues list response: %v", err)
	}

	data, ok := payload["data"].([]interface{})
	if !ok {
		t.Fatalf("Invalid issues list payload: missing data array")
	}

	found := false
	for _, raw := range data {
		item := raw.(map[string]interface{})
		if int(item["id"].(float64)) == issueID {
			found = true
			if fixed, ok := item["is_fixed"].(bool); !ok || !fixed {
				t.Fatalf("Expected issue %d to be fixed", issueID)
			}
			if gotNote, ok := item["notes"].(string); !ok || gotNote != note {
				t.Fatalf("Expected issue note %q, got %#v", note, item["notes"])
			}
		}
	}

	if !found {
		t.Fatalf("Updated issue id=%d not found in /api/issues", issueID)
	}
}

func TestIssueUpdateReturns404ForMissingIDIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	status := doJSONRequest(t, router, http.MethodPost, "/api/issues/status", map[string]interface{}{
		"id":       999999,
		"is_fixed": true,
	})
	if status.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 for missing issue status update. status=%d body=%s", status.Code, status.Body.String())
	}

	notes := doJSONRequest(t, router, http.MethodPost, "/api/issues/notes", map[string]interface{}{
		"id":    999999,
		"notes": "missing-id-note",
	})
	if notes.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 for missing issue notes update. status=%d body=%s", notes.Code, notes.Body.String())
	}
}

func TestIssuesRejectBlankPageAndActionIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	blankPage := doJSONRequest(t, router, http.MethodPost, "/api/issues", map[string]interface{}{
		"page":   "   ",
		"action": "click",
	})
	if blankPage.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for blank page. status=%d body=%s", blankPage.Code, blankPage.Body.String())
	}

	blankAction := doJSONRequest(t, router, http.MethodPost, "/api/issues", map[string]interface{}{
		"page":   "/qa",
		"action": "   ",
	})
	if blankAction.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for blank action. status=%d body=%s", blankAction.Code, blankAction.Body.String())
	}
}

func TestSkillsQueryContractIntegration(t *testing.T) {
	router, database := setupCRUDIntegrationRouter(t)
	ctx := context.Background()

	// Ensure the test can assert limit and filter deterministically.
	seedNameA := fmt.Sprintf("integration-skill-a-%d", time.Now().UnixNano())
	seedNameB := fmt.Sprintf("integration-skill-b-%d", time.Now().UnixNano())
	_, err := database.Exec(ctx, `
		INSERT INTO skills (name, description, check_type, check_query, severity, is_builtin, is_active)
		VALUES ($1, $2, 'validation', '.a', 'high', false, true),
		       ($3, $4, 'validation', '.b', 'low', false, true)
		ON CONFLICT (name) DO NOTHING
	`, seedNameA, "integration seed A", seedNameB, "integration seed B")
	if err != nil {
		t.Fatalf("Failed seeding skills: %v", err)
	}
	defer database.Exec(ctx, `DELETE FROM skills WHERE name IN ($1, $2)`, seedNameA, seedNameB)

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

func TestDocsQueryContractIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	resp := doJSONRequest(t, router, http.MethodGet, "/api/docs?limit=1&page=1&search=login&sort_by=name_desc", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("Docs query failed. status=%d body=%s", resp.Code, resp.Body.String())
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Failed parsing docs payload: %v", err)
	}
	data, ok := payload["data"].([]interface{})
	if !ok {
		t.Fatalf("Docs payload missing data array")
	}
	if len(data) > 1 {
		t.Fatalf("Expected limit=1 to return at most 1 row, got %d", len(data))
	}
	if _, ok := payload["pagination"].(map[string]interface{}); !ok {
		t.Fatalf("Docs payload missing pagination object")
	}
}

func TestJobsEndpointsIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	create := doJSONRequest(t, router, http.MethodPost, "/api/jobs/qa/run", map[string]interface{}{
		"triggered_by": "integration-test",
	})
	if create.Code != http.StatusAccepted {
		t.Fatalf("Create QA job failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing job create response: %v", err)
	}
	jobID, ok := created["id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("Invalid job id in create response: %#v", created["id"])
	}

	var latest map[string]interface{}
	for i := 0; i < 8; i++ {
		time.Sleep(120 * time.Millisecond)
		get := doJSONRequest(t, router, http.MethodGet, "/api/jobs/"+url.PathEscape(jobID), nil)
		if get.Code != http.StatusOK {
			t.Fatalf("Get job failed. status=%d body=%s", get.Code, get.Body.String())
		}
		if err := json.Unmarshal(get.Body.Bytes(), &latest); err != nil {
			t.Fatalf("Failed parsing job status response: %v", err)
		}
		if latest["status"] == "completed" {
			break
		}
	}

	if latest["status"] != "completed" {
		t.Fatalf("Expected job to complete, final status=%v", latest["status"])
	}
}

func TestIssuesListHandlesNullFixedAndNullableFieldsIntegration(t *testing.T) {
	router, database := setupCRUDIntegrationRouter(t)
	ctx := context.Background()

	pageName := fmt.Sprintf("integration-null-issue-%d", time.Now().UnixNano())
	var issueID int
	err := database.QueryRow(ctx, `
		INSERT INTO raw_issues (page, action, error_type, error_text, category, severity, is_fixed, notes)
		VALUES ($1, $2, NULL, $3, $4, NULL, NULL, NULL)
		RETURNING id
	`, pageName, "click_save", "nullable fields regression", "backend").Scan(&issueID)
	if err != nil {
		t.Fatalf("Failed to seed nullable issue row: %v", err)
	}
	defer database.Exec(ctx, `DELETE FROM raw_issues WHERE id = $1`, issueID)

	unfixed := doJSONRequest(t, router, http.MethodGet, "/api/issues?is_fixed=false&search="+url.QueryEscape(pageName)+"&limit=50", nil)
	if unfixed.Code != http.StatusOK {
		t.Fatalf("List unfixed issues failed. status=%d body=%s", unfixed.Code, unfixed.Body.String())
	}

	var unfixedPayload map[string]interface{}
	if err := json.Unmarshal(unfixed.Body.Bytes(), &unfixedPayload); err != nil {
		t.Fatalf("Failed parsing unfixed payload: %v", err)
	}
	unfixedData, ok := unfixedPayload["data"].([]interface{})
	if !ok {
		t.Fatalf("Invalid unfixed payload: missing data array")
	}

	foundInUnfixed := false
	for _, raw := range unfixedData {
		item := raw.(map[string]interface{})
		if int(item["id"].(float64)) == issueID {
			foundInUnfixed = true
			if fixed, ok := item["is_fixed"].(bool); !ok || fixed {
				t.Fatalf("Expected nullable is_fixed issue %d to be returned as unfixed", issueID)
			}
		}
	}
	if !foundInUnfixed {
		t.Fatalf("Seeded nullable issue id=%d not found in /api/issues?is_fixed=false", issueID)
	}

	allIssues := doJSONRequest(t, router, http.MethodGet, "/api/issues?search="+url.QueryEscape(pageName)+"&limit=50", nil)
	if allIssues.Code != http.StatusOK {
		t.Fatalf("List all issues failed. status=%d body=%s", allIssues.Code, allIssues.Body.String())
	}

	var allPayload map[string]interface{}
	if err := json.Unmarshal(allIssues.Body.Bytes(), &allPayload); err != nil {
		t.Fatalf("Failed parsing all issues payload: %v", err)
	}
	allData, ok := allPayload["data"].([]interface{})
	if !ok {
		t.Fatalf("Invalid all issues payload: missing data array")
	}

	foundInAll := false
	for _, raw := range allData {
		item := raw.(map[string]interface{})
		if int(item["id"].(float64)) == issueID {
			foundInAll = true
			break
		}
	}
	if !foundInAll {
		t.Fatalf("Seeded nullable issue id=%d not found in /api/issues", issueID)
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

func TestGoalEventsCRUDIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	goal := fmt.Sprintf("integration-goal-%d", time.Now().UnixNano())
	create := doJSONRequest(t, router, http.MethodPost, "/api/goal-events", map[string]interface{}{
		"goal_name":  goal,
		"confidence": 0.9,
		"page":       "login",
		"action":     "click_submit",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("Create goal event failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing create goal event response: %v", err)
	}
	goalID := int(created["id"].(float64))

	list := doJSONRequest(t, router, http.MethodGet, "/api/goal-events?goal_name="+url.QueryEscape(goal)+"&limit=50", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("List goal events failed. status=%d body=%s", list.Code, list.Body.String())
	}

	var events []map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &events); err != nil {
		t.Fatalf("Failed parsing goal events response: %v", err)
	}

	found := false
	for _, event := range events {
		if int(event["id"].(float64)) == goalID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Created goal event id=%d not found in /api/goal-events", goalID)
	}

	del := doJSONRequest(t, router, http.MethodDelete, fmt.Sprintf("/api/goal-events?id=%d", goalID), nil)
	if del.Code != http.StatusOK {
		t.Fatalf("Delete goal event failed. status=%d body=%s", del.Code, del.Body.String())
	}
}

func TestAICacheCRUDIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	inputText := fmt.Sprintf("integration-ai-cache-%d", time.Now().UnixNano())
	create := doJSONRequest(t, router, http.MethodPost, "/api/ai-cache", map[string]interface{}{
		"input_text":  inputText,
		"output_text": "integration output",
		"model":       "MiniMax-M2.5",
		"cost":        0.001,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("Create ai-cache failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing ai-cache create response: %v", err)
	}
	hash, ok := created["hash"].(string)
	if !ok || hash == "" {
		t.Fatalf("Invalid ai-cache hash in response: %#v", created["hash"])
	}

	get := doJSONRequest(t, router, http.MethodGet, "/api/ai-cache?hash="+url.QueryEscape(hash), nil)
	if get.Code != http.StatusOK {
		t.Fatalf("Get ai-cache failed. status=%d body=%s", get.Code, get.Body.String())
	}

	del := doJSONRequest(t, router, http.MethodDelete, "/api/ai-cache?hash="+url.QueryEscape(hash), nil)
	if del.Code != http.StatusOK {
		t.Fatalf("Delete ai-cache failed. status=%d body=%s", del.Code, del.Body.String())
	}
}

func TestEmbeddingCacheCRUDIntegration(t *testing.T) {
	router, _ := setupCRUDIntegrationRouter(t)

	text := fmt.Sprintf("integration-embedding-cache-%d", time.Now().UnixNano())
	create := doJSONRequest(t, router, http.MethodPost, "/api/embedding-cache", map[string]interface{}{
		"text":      text,
		"embedding": []float64{0.12, -0.34, 0.56},
		"model":     "emb-embedding-01",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("Create embedding-cache failed. status=%d body=%s", create.Code, create.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed parsing embedding-cache create response: %v", err)
	}
	hash, ok := created["hash"].(string)
	if !ok || hash == "" {
		t.Fatalf("Invalid embedding-cache hash in response: %#v", created["hash"])
	}

	get := doJSONRequest(t, router, http.MethodGet, "/api/embedding-cache?hash="+url.QueryEscape(hash), nil)
	if get.Code != http.StatusOK {
		t.Fatalf("Get embedding-cache failed. status=%d body=%s", get.Code, get.Body.String())
	}

	del := doJSONRequest(t, router, http.MethodDelete, "/api/embedding-cache?hash="+url.QueryEscape(hash), nil)
	if del.Code != http.StatusOK {
		t.Fatalf("Delete embedding-cache failed. status=%d body=%s", del.Code, del.Body.String())
	}
}
