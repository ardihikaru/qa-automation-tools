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

func TestAdditionalResourceEndpointsNoDB(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil)

	testCases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "SkillLogsGetNoDB", method: "GET", path: "/api/skills/logs", status: http.StatusServiceUnavailable},
		{name: "SkillLogsPostNoDB", method: "POST", path: "/api/skills/logs", body: `{"skill_id":1,"page":"login","action":"fill","result":"ok","is_passed":true}`, status: http.StatusServiceUnavailable},
		{name: "SkillStatsNoDB", method: "GET", path: "/api/skills/stats", status: http.StatusServiceUnavailable},
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
