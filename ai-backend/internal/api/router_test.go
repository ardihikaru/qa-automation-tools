package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", response["status"])
	}

	if response["database"] != "disconnected" {
		t.Errorf("Expected database 'disconnected', got '%v'", response["database"])
	}

	if response["minimax"] != "not_configured" {
		t.Errorf("Expected minimax 'not_configured', got '%v'", response["minimax"])
	}
}

func TestFlowsEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/flows", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestIssuesEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/issues", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestSkillsEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/skills", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestSearchEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/search?q=test", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestCostOverviewEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/cost/overview", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestCostTrendEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/cost/trend", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestCostEfficiencyEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/cost/efficiency", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestCostByFeatureEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/cost/by-feature", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestNotFoundEndpoint(t *testing.T) {
	// Create router
	router := NewRouter(nil, nil, nil, nil)

	// Create test request for unknown endpoint
	req := httptest.NewRequest("GET", "/api/unknown", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status NotFound, got %d", w.Code)
	}
}

func TestDocsEndpoint(t *testing.T) {
	// Create router
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/docs", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should be OK and return array
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	// Response should be valid JSON array, or object with "docs"/"data" field
	var asArray []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &asArray); err == nil {
		return
	}

	var asObject map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &asObject); err != nil {
		t.Errorf("Failed to parse docs response: %v", err)
		return
	}
	if _, ok := asObject["docs"]; ok {
		return
	}
	if _, ok := asObject["data"]; !ok {
		t.Errorf("Expected docs response to contain 'docs' or 'data' key")
	}
}

func TestVideosEndpoint(t *testing.T) {
	// Create router
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/videos", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should be OK and return array
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	// Response should be valid JSON array, or object with "videos" field
	var asArray []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &asArray); err == nil {
		return
	}

	var asObject map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &asObject); err != nil {
		t.Errorf("Failed to parse videos response: %v", err)
		return
	}
	if _, ok := asObject["videos"]; !ok {
		t.Errorf("Expected videos response to contain 'videos' key")
	}
}

func TestAnalyticsEndpoint(t *testing.T) {
	// Create router without database - now requires database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/analytics", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestLearningEndpoint(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/learning", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestCORSHeaders(t *testing.T) {
	// Create router
	router := NewRouter(nil, nil, nil, nil)

	// Create OPTIONS test request
	req := httptest.NewRequest("OPTIONS", "/health", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	// Check CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected CORS origin '*', got '%s'", w.Header().Get("Access-Control-Allow-Origin"))
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("Expected CORS methods header")
	}
}

func TestSearchMissingQuery(t *testing.T) {
	// Create router
	router := NewRouter(nil, nil, nil, nil)

	// Create test request without query param
	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Missing query should fail validation before service checks
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status BadRequest, got %d", w.Code)
	}
}

func TestAdditionalResourceEndpointsNoDB(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil)

	testCases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "IssueStatusNoDB", method: "POST", path: "/api/issues/status", body: `{"id":1,"is_fixed":true}`, status: http.StatusServiceUnavailable},
		{name: "IssueNotesNoDB", method: "POST", path: "/api/issues/notes", body: `{"id":1,"notes":"note"}`, status: http.StatusServiceUnavailable},
		{name: "SkillLogsGetNoDB", method: "GET", path: "/api/skills/logs", status: http.StatusServiceUnavailable},
		{name: "SkillLogsPostNoDB", method: "POST", path: "/api/skills/logs", body: `{"skill_id":1,"page":"login","action":"fill","result":"ok","is_passed":true}`, status: http.StatusServiceUnavailable},
		{name: "SkillStatsNoDB", method: "GET", path: "/api/skills/stats", status: http.StatusServiceUnavailable},
		{name: "AICacheGetNoDB", method: "GET", path: "/api/ai-cache?hash=test", status: http.StatusServiceUnavailable},
		{name: "AICachePostNoDB", method: "POST", path: "/api/ai-cache", body: `{"input_text":"a","output_text":"b"}`, status: http.StatusServiceUnavailable},
		{name: "AICacheDeleteNoDB", method: "DELETE", path: "/api/ai-cache?hash=test", status: http.StatusServiceUnavailable},
		{name: "EmbeddingCacheGetNoDB", method: "GET", path: "/api/embedding-cache?hash=test", status: http.StatusServiceUnavailable},
		{name: "EmbeddingCachePostNoDB", method: "POST", path: "/api/embedding-cache", body: `{"text":"patient login","embedding":[0.1,0.2]}`, status: http.StatusServiceUnavailable},
		{name: "EmbeddingCacheDeleteNoDB", method: "DELETE", path: "/api/embedding-cache?hash=test", status: http.StatusServiceUnavailable},
		{name: "LearningStatsNoDB", method: "GET", path: "/api/learning/stats", status: http.StatusServiceUnavailable},
		{name: "LearningContextGetNoDB", method: "GET", path: "/api/learning/context?key=a&value=b", status: http.StatusServiceUnavailable},
		{name: "LearningContextPostNoDB", method: "POST", path: "/api/learning/context", body: `{"context_key":"a","context_value":"b","best_action":"x","success":true}`, status: http.StatusServiceUnavailable},
		{name: "RunQAJobNoDB", method: "POST", path: "/api/jobs/qa/run", body: `{"triggered_by":"unit-test"}`, status: http.StatusAccepted},
		{name: "RunDocsJobNoDB", method: "POST", path: "/api/jobs/docs/generate", body: `{"triggered_by":"unit-test"}`, status: http.StatusAccepted},
		{name: "GetUnknownJobNoDB", method: "GET", path: "/api/jobs/non-existing-id", status: http.StatusNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tc.body == "" {
				bodyReader = strings.NewReader("")
			} else {
				bodyReader = strings.NewReader(tc.body)
			}

			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tc.status {
				t.Errorf("Expected status %d, got %d. Body: %s", tc.status, w.Code, w.Body.String())
			}
		})
	}
}

func TestCreateFlowNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request with JSON body
	body := strings.NewReader(`{"name": "test-flow", "role_id": 1}`)
	req := httptest.NewRequest("POST", "/api/flows", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestGoalEventsNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/goal-events", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestGoalEventsPostNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request with JSON body
	body := strings.NewReader(`{"goal_name": "create_user", "confidence": 0.8}`)
	req := httptest.NewRequest("POST", "/api/goal-events", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestLearningNavigationNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/learning/navigation?source=/dashboard", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestLearningNavigationPostNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request with JSON body
	body := strings.NewReader(`{"source_page": "/dashboard", "target_page": "/patients", "action": "click_menu", "success": true}`)
	req := httptest.NewRequest("POST", "/api/learning/navigation", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestLearningSequencesNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/learning/sequences?goal=create_user", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestLearningSequencesPostNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request with JSON body
	body := strings.NewReader(`{"goal": "create_user", "steps": "[\"login\", \"submit\"]", "success": true}`)
	req := httptest.NewRequest("POST", "/api/learning/sequences", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestRolesEndpointNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/roles", nil)
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}

func TestRolesPostNoDB(t *testing.T) {
	// Create router without database
	router := NewRouter(nil, nil, nil, nil)

	// Create test request with JSON body
	body := strings.NewReader(`{"name": "nurse", "description": "Nursing staff"}`)
	req := httptest.NewRequest("POST", "/api/roles", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Check response - should return 503 when no database
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
	}
}
