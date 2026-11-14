package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
)

// TestHealthEndpointWithDB tests the health endpoint with database connection
func TestHealthEndpointWithDB(t *testing.T) {
	loadIntegrationEnvFiles()
	config.ClearCache()
	t.Cleanup(config.ClearCache)

	// Get database URL
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		cfg := config.LoadConfig()
		databaseURL = cfg.GetDatabaseURL()
	}

	if databaseURL == "" {
		t.Skip("No database URL found")
	}

	// Connect to database
	database, err := db.NewConnection(config.LoadConfig())
	if err != nil {
		t.Skipf("Skipping integration test: database not reachable: %v", err)
	}
	defer database.Close()

	// Create router
	router := NewRouter(database, nil, nil, nil)

	// Test health endpoint
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	// Verify database is connected
	if response["database"] != "connected" {
		t.Errorf("Expected database 'connected', got '%v'", response["database"])
	}

	t.Log("✓ Health endpoint reports database connected")
}

// TestConfigLoading tests that configuration is loaded correctly from config.yaml
func TestConfigLoading(t *testing.T) {
	cfg := config.LoadConfig()

	// Verify database configuration
	if cfg.Backend.Database.Host == "" && os.Getenv("DATABASE_URL") == "" {
		t.Log("⚠ No database host in config.yaml")
	}

	// Verify AI configuration
	if cfg.Backend.AI.APIKey == "" && os.Getenv("MINIMAX_API_KEY") == "" {
		t.Log("⚠ No AI API key in config.yaml")
	} else {
		t.Log("✓ AI API key configured")
	}

	t.Logf("Config loaded - Backend Port: %d, Database: %s", cfg.Backend.Port, cfg.Backend.Database.Host)
}
