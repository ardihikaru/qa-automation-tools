package main

import (
	"log"

	"github.com/qa-automation/ai-backend/internal/config"
	"github.com/qa-automation/ai-backend/internal/db"
)

func main() {
	cfg := config.LoadConfig()

	log.Println("📦 QA Automation Database Migration (MySQL)")

	database, err := db.NewConnection(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to connect to MySQL database: %v", err)
	}
	defer database.Close()

	log.Println("✅ MySQL database connected")

	if err := db.RunMigrations(database.SQL); err != nil {
		log.Fatalf("❌ Database migration failed: %v", err)
	}

	log.Println("✅ Database migration completed")
}
