package db

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrations contains all database migrations
var migrations = []string{
	// Create extensions
	`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`,
	`CREATE EXTENSION IF NOT EXISTS "vector";`,

	// Core tables - Roles
	`CREATE TABLE IF NOT EXISTS roles (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// Core tables - Flows
	`CREATE TABLE IF NOT EXISTS flows (
		id SERIAL PRIMARY KEY,
		role_id INTEGER REFERENCES roles(id),
		name TEXT NOT NULL,
		description TEXT,
		created_at TIMESTAMP DEFAULT NOW(),
		updated_at TIMESTAMP DEFAULT NOW()
	);`,

	// Core tables - Steps
	`CREATE TABLE IF NOT EXISTS steps (
		id SERIAL PRIMARY KEY,
		flow_id INTEGER REFERENCES flows(id) ON DELETE CASCADE,
		step_order INTEGER NOT NULL,
		action TEXT NOT NULL,
		selector TEXT,
		value TEXT,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// AI Cache tables
	`CREATE TABLE IF NOT EXISTS ai_cache (
		id SERIAL PRIMARY KEY,
		input_hash TEXT NOT NULL UNIQUE,
		input_text TEXT NOT NULL,
		output_text TEXT NOT NULL,
		model TEXT DEFAULT 'abab6.5s-chat',
		cost_estimate DECIMAL(10, 6) DEFAULT 0,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	`CREATE TABLE IF NOT EXISTS embedding_cache (
		id SERIAL PRIMARY KEY,
		text_hash TEXT NOT NULL UNIQUE,
		text TEXT NOT NULL,
		embedding VECTOR(1536),
		model TEXT DEFAULT 'emb-embedding-01',
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// Learning Cache (context-aware)
	`CREATE TABLE IF NOT EXISTS learning_cache (
		id SERIAL PRIMARY KEY,
		intent TEXT NOT NULL,
		embedding VECTOR(1536),
		action TEXT NOT NULL,
		page TEXT NOT NULL,
		success_score DECIMAL(3, 2) DEFAULT 0.5,
		usage_count INTEGER DEFAULT 1,
		last_success TIMESTAMP,
		decay_factor DECIMAL(3, 2) DEFAULT 1.0,
		created_at TIMESTAMP DEFAULT NOW(),
		updated_at TIMESTAMP DEFAULT NOW()
	);`,

	// QA System - Skills
	`CREATE TABLE IF NOT EXISTS skills (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		check_type TEXT NOT NULL,
		check_query TEXT NOT NULL,
		severity TEXT DEFAULT 'medium',
		is_builtin BOOLEAN DEFAULT TRUE,
		is_active BOOLEAN DEFAULT TRUE,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// QA System - Skill Logs
	`CREATE TABLE IF NOT EXISTS skill_logs (
		id SERIAL PRIMARY KEY,
		skill_id INTEGER REFERENCES skills(id),
		page TEXT NOT NULL,
		action TEXT NOT NULL,
		result TEXT NOT NULL,
		is_passed BOOLEAN DEFAULT FALSE,
		error_message TEXT,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// QA System - Learned Skills
	`CREATE TABLE IF NOT EXISTS learned_skills (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		pattern TEXT NOT NULL,
		check_type TEXT NOT NULL,
		check_query TEXT NOT NULL,
		frequency INTEGER DEFAULT 1,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// QA System - Raw Issues
	`CREATE TABLE IF NOT EXISTS raw_issues (
		id SERIAL PRIMARY KEY,
		page TEXT NOT NULL,
		action TEXT NOT NULL,
		error_type TEXT,
		error_text TEXT,
		category TEXT,
		severity TEXT DEFAULT 'medium',
		is_fixed BOOLEAN DEFAULT FALSE,
		notes TEXT,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// QA System - Issue Suggestions
	`CREATE TABLE IF NOT EXISTS issue_suggestions (
		id SERIAL PRIMARY KEY,
		issue_id INTEGER REFERENCES raw_issues(id) ON DELETE CASCADE,
		suggestion TEXT NOT NULL,
		confidence DECIMAL(3, 2) DEFAULT 0.5,
		target TEXT,
		file_path TEXT,
		diff TEXT,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// Analytics - Request Logs
	`CREATE TABLE IF NOT EXISTS request_logs (
		id SERIAL PRIMARY KEY,
		endpoint TEXT NOT NULL,
		method TEXT NOT NULL,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		cost_estimate DECIMAL(10, 6) DEFAULT 0,
		cache_hit BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// Analytics - Flow Outcomes
	`CREATE TABLE IF NOT EXISTS flow_outcomes (
		id SERIAL PRIMARY KEY,
		flow_id INTEGER REFERENCES flows(id),
		status TEXT NOT NULL,
		duration_ms INTEGER,
		steps_completed INTEGER,
		issues_found INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT NOW()
	);`,

	// Analytics - Goal Events
	`CREATE TABLE IF NOT EXISTS goal_events (
		id SERIAL PRIMARY KEY,
		goal_name TEXT NOT NULL,
		confidence DECIMAL(3, 2) DEFAULT 0,
		detected_at TIMESTAMP DEFAULT NOW()
	);`,

	// Create indices for better performance
	`CREATE INDEX IF NOT EXISTS idx_ai_cache_hash ON ai_cache(input_hash);`,
	`CREATE INDEX IF NOT EXISTS idx_embedding_cache_hash ON embedding_cache(text_hash);`,
	`CREATE INDEX IF NOT EXISTS idx_learning_cache_intent ON learning_cache(intent);`,
	`CREATE INDEX IF NOT EXISTS idx_learning_cache_embedding ON learning_cache embedding vector_cosine_ops;`,
	`CREATE INDEX IF NOT EXISTS idx_skills_name ON skills(name);`,
	`CREATE INDEX IF NOT EXISTS idx_raw_issues_page ON raw_issues(page);`,
	`CREATE INDEX IF NOT EXISTS idx_raw_issues_fixed ON raw_issues(is_fixed);`,
	`CREATE INDEX IF NOT EXISTS idx_request_logs_endpoint ON request_logs(endpoint);`,
	`CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at);`,
}

// RunMigrations executes all database migrations
func RunMigrations(db *pgxpool.Pool) error {
	log.Println("📦 Running database migrations...")

	for i, migration := range migrations {
		if _, err := db.Exec(context.Background(), migration); err != nil {
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
		log.Printf("  ✅ Migration %d/%d completed", i+1, len(migrations))
	}

	log.Println("✅ All migrations completed successfully")
	return nil
}