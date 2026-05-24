package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/qa-automation/ai-backend/internal/api"
	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/learning"
	"github.com/qa-automation/ai-backend/internal/services/skills"
)

func main() {
	// Load configuration from .env file
	cfg := config.LoadConfig()

	log.Println("📦 QA Automation Backend")
	log.Printf("   Port: %d", cfg.Backend.Port)
	log.Printf("   Database: %s", cfg.Backend.Database.Host)

	// Check for database connection - make optional for deployment without DB
	var database *db.DB
	aiService := ai.NewServiceWithConfig(
		cfg.Backend.AI.APIKey,
		cfg.Backend.AI.BaseURL,
		cfg.Backend.AI.Model,
		cfg.Backend.AI.ChatEndpoint,
	)
	var learningService *learning.Service
	var skillsService *skills.Service

	// Try to initialize database if credentials are available
	dbURL := cfg.GetDatabaseURL()
	// Check both ConnectionString (from DATABASE_URL env var) and Host (from config)
	if dbURL != "" || cfg.Backend.Database.Host != "" {
		var err error
		database, err = db.NewConnection(cfg)
		if err != nil {
			log.Printf("⚠️  Database connection failed: %v", err)
			log.Printf("   Running without database (limited functionality)")
		} else {
			defer database.Close()
			log.Println("   Database: Connected ✓")

			// Initialize DB-backed services (MySQL-backed *db.DB wrapper).
			learningService = learning.NewService(database)
			skillsService = skills.NewService(database, aiService)

			// Ensure built-in skills exist
			ctx := context.Background()
			if err := skillsService.EnsureBuiltinSkills(ctx); err != nil {
				log.Printf("Warning: Failed to ensure builtin skills: %v", err)
			}
		}
	} else {
		log.Println("   Database: Not configured (running in standalone mode)")
	}

	// Initialize router with possibly nil database
	router := api.NewRouter(database, aiService, learningService, skillsService)

	// Get server address - use PORT env var if available (Cloud Run standard)
	port := cfg.Backend.Port
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", &port)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Backend.Host, port)

	// Start server
	log.Printf("🚀 Server starting on %s...", addr)
	log.Fatal(http.ListenAndServe(addr, router))
}
