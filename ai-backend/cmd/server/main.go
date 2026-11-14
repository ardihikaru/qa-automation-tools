package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qa-automation/ai-backend/internal/api"
	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
	"github.com/qa-automation/ai-backend/internal/seed"
	"github.com/qa-automation/ai-backend/internal/services/ai"
	"github.com/qa-automation/ai-backend/internal/services/executor"
	"github.com/qa-automation/ai-backend/internal/services/gaossync"
	"github.com/qa-automation/ai-backend/internal/services/knowledge"
	"github.com/qa-automation/ai-backend/internal/services/learning"
	"github.com/qa-automation/ai-backend/internal/services/repopool"
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
	aiService := ai.NewFromConfig(cfg.Backend.AI)
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
			aiService.SetRecorder(&ai.SQLRecorder{DB: database.SQL})
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

	// Playwright jobs run in playwright-executor (systemd service on rnd-v3).
	// Without PLAYWRIGHT_EXECUTOR_URL, /api/jobs/* stay simulated.
	if exe := executor.New(executor.Config{URL: cfg.Backend.Executor.URL, Token: cfg.Backend.Executor.Token}); exe != nil {
		router.SetExecutor(exe, time.Duration(cfg.Backend.Executor.TimeoutMinutes)*time.Minute)
	}
	router.SetSettings(api.SettingsInfo{
		AIProvider:      cfg.Backend.AI.Provider,
		ManagerURL:      cfg.Backend.AI.ManagerURL,
		ManagerProvider: cfg.Backend.AI.ManagerProvider,
		ManagerModel:    cfg.Backend.AI.ManagerModel,
		NoFallback:      cfg.Backend.AI.ManagerNoFallback,
		TimeoutSeconds:  cfg.Backend.AI.TimeoutSeconds,
		ExecutorURL:     cfg.Backend.Executor.URL,
	})
	pool := repopool.New(cfg.Backend.Products.PoolDir)
	if pool != nil {
		for _, o := range strings.Split(cfg.Backend.Products.AllowedOwners, ",") {
			if o = strings.ToLower(strings.TrimSpace(o)); o != "" {
				pool.AllowedOwners = append(pool.AllowedOwners, o)
			}
		}
	}
	router.SetRepoPool(pool)
	router.StartProductVerifier(context.Background(), time.Duration(cfg.Backend.Products.VerifyIntervalMinutes)*time.Minute)
	router.FailOrphanedJobs(context.Background())
	// Product knowledge base: sync of the automation repo's docs/ catalog (PRODUCT_KNOWLEDGE_SYNC.md).
	if syncer := knowledge.NewSyncer(database, pool, aiService); syncer != nil {
		router.SetKnowledge(syncer)
		// GAOS integration (GAOS_INTEGRATION.md): push flagged products' test cases after each sync.
		// The hooks are set before the scheduler starts. Unset URL/token = never sends.
		if database != nil {
			gaos := gaossync.New(database, cfg.Backend.GAOS.APIURL, cfg.Backend.GAOS.APIToken)
			router.SetGAOS(gaos)
			syncer.OnSucceeded, syncer.OnUnchanged = gaos.Trigger, gaos.Retry
			log.Printf("   GAOS integration: configured=%v", gaos.Configured())
		}
		syncer.FailOrphanedSyncs(context.Background())
		syncer.StartScheduler(context.Background(), time.Duration(cfg.Backend.Products.KnowledgeSyncIntervalMinutes)*time.Minute)
	}

	// Wire authentication when the database is connected. Without a DB we have
	// no users table; the app stays open (current pre-auth behaviour) and logs
	// a warning so it's obvious in journalctl.
	if database != nil {
		seedCount, err := seed.SeedUsers(context.Background(), database.SQL, seed.Options{
			Neo4jURI:        os.Getenv("TASKMGR_NEO4J_URI"),
			Neo4jUser:       os.Getenv("TASKMGR_NEO4J_USERNAME"),
			Neo4jPassword:   os.Getenv("TASKMGR_NEO4J_PASSWORD"),
			Neo4jDatabase:   getEnv("TASKMGR_NEO4J_DATABASE", "neo4j"),
			AdminUsername:   os.Getenv("ADMIN_USERNAME"),
			AdminPassword:   os.Getenv("ADMIN_PASSWORD"),
			ServiceUsername: os.Getenv("AI_SERVICE_USERNAME"),
			ServicePassword: os.Getenv("AI_SERVICE_PASSWORD"),
		})
		if err != nil {
			log.Printf("⚠️  seed failed: %v", err)
		} else {
			log.Printf("🔐 Auth: %d users present in `users` table", seedCount)
		}

		cookieName := getEnv("SESSION_COOKIE_NAME", "qa_session")
		ttlHours := getEnvInt("SESSION_TTL_HOURS", 12)
		secureCookie := getEnvBool("SESSION_COOKIE_SECURE", false)

		authHandlers := &api.AuthHandlers{
			DB:         database.SQL,
			CookieName: cookieName,
			SessionTTL: time.Duration(ttlHours) * time.Hour,
			Secure:     secureCookie,
		}
		requireAuth := auth.Require(auth.Config{
			DB:         database.SQL,
			CookieName: cookieName,
		})
		router.SetAuth(authHandlers, requireAuth, getEnv("DASHBOARD_ORIGIN", "http://localhost:6173"))
		log.Printf("🔐 Auth: enabled (cookie=%s, ttl=%dh)", cookieName, ttlHours)
	} else {
		log.Println("⚠️  Auth: disabled (no database — backend is OPEN)")
	}

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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	switch v {
	case "1", "true", "TRUE", "True":
		return true
	case "0", "false", "FALSE", "False":
		return false
	}
	return fallback
}
