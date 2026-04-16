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

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	if response["docs"] == nil {
		t.Errorf("Expected docs key in response")
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

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	if response["videos"] == nil {
		t.Errorf("Expected videos key in response")
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
	// Create router
	router := NewRouter(nil, nil, nil, nil)

	// Create test request
	req := httptest.NewRequest("GET", "/api/learning", nil)
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

	if response["learning_records"] == nil {
		t.Errorf("Expected learning_records key in response")
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

	// Check response - when no database, returns 503 instead of 400
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status ServiceUnavailable, got %d", w.Code)
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