package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/learning"
	"github.com/qa-automation/ai-backend/internal/services/skills"
)

// TestSearchIntegration tests the /api/search endpoint with real database and AI service
// This test requires:
// - a reachable MySQL database via DB_* or DATABASE_URL (use a disposable one)
// - a configured AI provider (ai-manager)
func TestSearchIntegration(t *testing.T) {
	loadIntegrationEnvFiles()
	config.ClearCache()
	t.Cleanup(config.ClearCache)

	// Load configuration from config.yaml
	cfg := config.LoadConfig()

	// Verify database configuration exists
	if reason := integrationOptOut(); reason != "" {
		t.Skip(reason)
	}
	if cfg.Backend.Database.Host == "" && cfg.GetDatabaseURL() == "" {
		t.Skip("Skipping integration test: No database configured in config.yaml")
	}

	// Initialize database connection
	database, err := db.NewConnection(cfg)
	if err != nil {
		t.Skipf("Skipping integration test: database not reachable: %v", err)
	}
	defer database.Close()

	// Verify database connection
	ctx := context.Background()
	if err := database.Ping(ctx); err != nil {
		t.Fatalf("Database ping failed: %v", err)
	}
	t.Log("✓ Database connected successfully")

	if cfg.Backend.AI.APIKey == "" {
		t.Skip("Skipping integration test: MINIMAX API key is not configured")
	}
	aiService := ai.NewServiceWithConfig(
		cfg.Backend.AI.APIKey,
		cfg.Backend.AI.BaseURL,
		cfg.Backend.AI.Model,
		cfg.Backend.AI.ChatEndpoint,
	)
	t.Log("✓ AI service initialized")

	// Initialize other services
	learningService := learning.NewService(database)
	skillsService := skills.NewService(database, aiService)

	// Create router with all services
	router := NewRouter(database, aiService, learningService, skillsService)

	// Test 1: Search with valid query
	t.Run("SearchWithValidQuery", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search?q=test+query", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should return OK even if no results (empty array)
		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Parse response
		var results []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		t.Logf("Search returned %d results", len(results))
	})

	// Test 2: Search with empty query (missing parameter)
	t.Run("SearchWithEmptyQuery", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should return 400 Bad Request
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status BadRequest, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Test 3: Search with various query types
	searchQueries := []string{
		"login",
		"create+patient",
		"dashboard",
		"patient+record",
	}

	for _, q := range searchQueries {
		t.Run(fmt.Sprintf("Search_%s", q), func(t *testing.T) {
			req := httptest.NewRequest("GET", fmt.Sprintf("/api/search?q=%s", q), nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Query '%s' - Expected status OK, got %d. Body: %s", q, w.Code, w.Body.String())
			}

			var results []map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
				t.Errorf("Query '%s' - Failed to parse response: %v", q, err)
			}

			t.Logf("Query '%s' returned %d results", q, len(results))
		})
	}
}

// TestSearchWithDatabase tests the search endpoint against the MySQL database
func TestSearchWithDatabase(t *testing.T) {
	loadIntegrationEnvFiles()
	config.ClearCache()
	t.Cleanup(config.ClearCache)

	// Get database URL from environment (supports DATABASE_URL format)
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		// Try loading from config
		cfg := config.LoadConfig()
		databaseURL = cfg.GetDatabaseURL()
	}

	if databaseURL == "" {
		t.Skip("No database URL found - set DATABASE_URL or configure in config.yaml")
	}

	t.Logf("Connecting to database...")

	// Connect to database
	database, err := db.NewConnection(config.LoadConfig())
	if err != nil {
		t.Skipf("Skipping integration test: database not reachable: %v", err)
	}
	defer database.Close()

	t.Log("✓ Database connected")

	// Test database query - check if learning_cache table exists and has data
	var count int
	err = database.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM learning_cache").Scan(&count)

	if err != nil {
		t.Logf("⚠ learning_cache table might not exist or has no data: %v", err)
		t.Log("This is OK - the search will return empty results")
	} else {
		t.Logf("✓ learning_cache table has %d records", count)
	}

	// Initialize AI service
	cfg := config.LoadConfig()
	if cfg.Backend.AI.APIKey == "" {
		t.Skip("Skipping integration test: MINIMAX API key is not configured")
	}
	aiService := ai.NewServiceWithConfig(
		cfg.Backend.AI.APIKey,
		cfg.Backend.AI.BaseURL,
		cfg.Backend.AI.Model,
		cfg.Backend.AI.ChatEndpoint,
	)

	// Create router
	learningService := learning.NewService(database)
	skillsService := skills.NewService(database, aiService)
	router := NewRouter(database, aiService, learningService, skillsService)

	// Make actual HTTP request to test endpoint
	req := httptest.NewRequest("GET", "/api/search?q=login", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

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

// BenchmarkSearchEndpoint benchmarks the search endpoint
func BenchmarkSearchEndpoint(b *testing.B) {
	// Load config
	cfg := config.LoadConfig()

	// Connect to database
	database, err := db.NewConnection(cfg)
	if err != nil {
		b.Skip("Database not available for benchmarking")
	}
	defer database.Close()

	// Create router
	aiService := ai.NewServiceWithConfig(
		cfg.Backend.AI.APIKey,
		cfg.Backend.AI.BaseURL,
		cfg.Backend.AI.Model,
		cfg.Backend.AI.ChatEndpoint,
	)
	learningService := learning.NewService(database)
	skillsService := skills.NewService(database, aiService)
	router := NewRouter(database, aiService, learningService, skillsService)

	// Benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/search?q=benchmark+test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
