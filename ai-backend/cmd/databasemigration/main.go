package main

import (
	"log"

	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
)

func main() {
	cfg := config.LoadConfig()

	log.Println("📦 QA Automation Database Migration")

	database, err := db.NewConnection(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to connect to Neon database: %v", err)
	}
	defer database.Close()

	log.Println("✅ Neon database connected")

	if err := db.RunMigrations(database.Pool); err != nil {
		log.Fatalf("❌ Database migration failed: %v", err)
	}

	log.Println("✅ Database migration completed")
}
