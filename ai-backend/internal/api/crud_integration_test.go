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
