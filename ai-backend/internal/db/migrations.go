package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// migrations contains all MySQL DDL statements. Translated from the original
// Neon PostgreSQL schema:
//   - SERIAL                  → INT AUTO_INCREMENT PRIMARY KEY
//   - TIMESTAMP DEFAULT NOW() → DATETIME DEFAULT CURRENT_TIMESTAMP
//   - BOOLEAN/JSONB           → TINYINT(1)/JSON
//   - Schemas (qa/ai/logging) → dropped; MySQL uses a single database
//   - DECIMAL(p,s)            → DECIMAL(p,s) (same syntax)
//   - DO $$ ... pg_constraint → handled in Go (see ensureCheckConstraint)
//
// Each migration is executed in order. Idempotent guards (IF NOT EXISTS,
// INSERT IGNORE, ON DUPLICATE KEY) make repeated runs safe.
var migrations = []string{
	// Core tables — Roles
	`CREATE TABLE IF NOT EXISTS roles (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(191) NOT NULL UNIQUE,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Core tables — Flows
	`CREATE TABLE IF NOT EXISTS flows (
		id INT AUTO_INCREMENT PRIMARY KEY,
		role_id INT NULL,
		name VARCHAR(191) NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_flows_role_id (role_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Core tables — Steps
	`CREATE TABLE IF NOT EXISTS steps (
		id INT AUTO_INCREMENT PRIMARY KEY,
		flow_id INT NULL,
		step_order INT NOT NULL,
		action TEXT NOT NULL,
		selector TEXT,
		value TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_steps_flow_id (flow_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// AI Cache tables
	`CREATE TABLE IF NOT EXISTS ai_cache (
		id INT AUTO_INCREMENT PRIMARY KEY,
		input_hash VARCHAR(191) NOT NULL UNIQUE,
		input_text MEDIUMTEXT NOT NULL,
		output_text MEDIUMTEXT NOT NULL,
		model VARCHAR(128) DEFAULT 'MiniMax-M2.5',
		cost_estimate DECIMAL(10,6) DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	`CREATE TABLE IF NOT EXISTS embedding_cache (
		id INT AUTO_INCREMENT PRIMARY KEY,
		text_hash VARCHAR(191) NOT NULL UNIQUE,
		text MEDIUMTEXT NOT NULL,
		embedding JSON,
		model VARCHAR(128) DEFAULT 'emb-embedding-01',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Learning Cache (context-aware)
	`CREATE TABLE IF NOT EXISTS learning_cache (
		id INT AUTO_INCREMENT PRIMARY KEY,
		intent TEXT NOT NULL,
		embedding JSON,
		action TEXT NOT NULL,
		page TEXT NOT NULL,
		success_score DECIMAL(3,2) DEFAULT 0.5,
		usage_count INT DEFAULT 1,
		last_success DATETIME NULL,
		decay_factor DECIMAL(3,2) DEFAULT 1.0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// QA System — Skills
	`CREATE TABLE IF NOT EXISTS skills (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(191) NOT NULL UNIQUE,
		description TEXT,
		check_type VARCHAR(64) NOT NULL,
		check_query TEXT NOT NULL,
		severity VARCHAR(32) DEFAULT 'medium',
		is_builtin TINYINT(1) DEFAULT 1,
		is_active TINYINT(1) DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// QA System — Skill Logs
	`CREATE TABLE IF NOT EXISTS skill_logs (
		id INT AUTO_INCREMENT PRIMARY KEY,
		skill_id INT NULL,
		page TEXT NOT NULL,
		action TEXT NOT NULL,
		result TEXT NOT NULL,
		is_passed TINYINT(1) DEFAULT 0,
		error_message TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_skill_logs_skill_id (skill_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// QA System — Learned Skills
	`CREATE TABLE IF NOT EXISTS learned_skills (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(191) NOT NULL,
		description TEXT,
		pattern TEXT NOT NULL,
		check_type VARCHAR(64) NOT NULL,
		check_query TEXT NOT NULL,
		frequency INT DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// QA System — Raw Issues
	`CREATE TABLE IF NOT EXISTS raw_issues (
		id INT AUTO_INCREMENT PRIMARY KEY,
		page VARCHAR(255) NOT NULL,
		action VARCHAR(255) NOT NULL,
		error_type VARCHAR(128),
		error_text TEXT,
		category VARCHAR(64),
		severity VARCHAR(32) DEFAULT 'medium',
		is_fixed TINYINT(1) DEFAULT 0,
		notes TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_raw_issues_page (page),
		INDEX idx_raw_issues_fixed (is_fixed)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`UPDATE raw_issues SET page = 'unknown-page' WHERE TRIM(COALESCE(page, '')) = '';`,
	`UPDATE raw_issues SET action = 'unknown-action' WHERE TRIM(COALESCE(action, '')) = '';`,
	`UPDATE raw_issues SET severity = 'medium' WHERE TRIM(COALESCE(severity, '')) = '';`,

	// QA System — Issue Suggestions
	`CREATE TABLE IF NOT EXISTS issue_suggestions (
		id INT AUTO_INCREMENT PRIMARY KEY,
		issue_id INT NULL,
		suggestion TEXT NOT NULL,
		confidence DECIMAL(3,2) DEFAULT 0.5,
		target VARCHAR(64),
		file_path VARCHAR(512),
		diff MEDIUMTEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_issue_suggestions_issue_id (issue_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Analytics — Request Logs
	`CREATE TABLE IF NOT EXISTS request_logs (
		id INT AUTO_INCREMENT PRIMARY KEY,
		endpoint VARCHAR(255) NOT NULL,
		method VARCHAR(16) NOT NULL,
		input_tokens INT DEFAULT 0,
		output_tokens INT DEFAULT 0,
		cost_estimate DECIMAL(10,6) DEFAULT 0,
		cache_hit TINYINT(1) DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_request_logs_endpoint (endpoint),
		INDEX idx_request_logs_created (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Analytics — Flow Outcomes
	`CREATE TABLE IF NOT EXISTS flow_outcomes (
		id INT AUTO_INCREMENT PRIMARY KEY,
		flow_id INT NULL,
		status VARCHAR(64) NOT NULL,
		duration_ms INT,
		steps_completed INT,
		issues_found INT DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_flow_outcomes_flow_id (flow_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Job runs — persisted quick-action audit history
	`CREATE TABLE IF NOT EXISTS job_runs (
		id VARCHAR(191) PRIMARY KEY,
		type VARCHAR(64) NOT NULL,
		status VARCHAR(32) NOT NULL,
		payload JSON,
		result JSON,
		error TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at DATETIME NULL,
		completed_at DATETIME NULL,
		duration_ms BIGINT,
		INDEX idx_job_runs_type (type),
		INDEX idx_job_runs_status (status),
		INDEX idx_job_runs_created_at (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Analytics — Goal Events
	`CREATE TABLE IF NOT EXISTS goal_events (
		id INT AUTO_INCREMENT PRIMARY KEY,
		goal_name VARCHAR(191) NOT NULL,
		confidence DECIMAL(3,2) DEFAULT 0,
		page TEXT,
		action TEXT,
		detected_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Learning System — Navigation Edges
	`CREATE TABLE IF NOT EXISTS navigation_edges (
		id INT AUTO_INCREMENT PRIMARY KEY,
		source_page VARCHAR(255) NOT NULL,
		target_page VARCHAR(255) NOT NULL,
		action VARCHAR(191) NOT NULL,
		success_count INT DEFAULT 0,
		failure_count INT DEFAULT 0,
		success_rate DECIMAL(5,4) DEFAULT 0.5,
		avg_duration_ms INT DEFAULT 0,
		last_used DATETIME NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_navigation_edges_source (source_page),
		INDEX idx_navigation_edges_target (target_page)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Learning System — Action Sequences
	`CREATE TABLE IF NOT EXISTS action_sequences (
		id INT AUTO_INCREMENT PRIMARY KEY,
		goal VARCHAR(191) NOT NULL,
		steps JSON NOT NULL,
		success_count INT DEFAULT 0,
		failure_count INT DEFAULT 0,
		avg_duration_ms INT,
		last_used DATETIME NULL,
		is_valid TINYINT(1) DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_action_sequences_goal (goal)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Learning System — Context Cache
	`CREATE TABLE IF NOT EXISTS context_cache (
		id INT AUTO_INCREMENT PRIMARY KEY,
		context_key VARCHAR(191) NOT NULL,
		context_value VARCHAR(191) NOT NULL,
		best_action TEXT,
		success_score DECIMAL(3,2) DEFAULT 0.5,
		usage_count INT DEFAULT 1,
		last_success DATETIME NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_context_cache_key (context_key)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Auth — Users (identity copied once from taskmgr Neo4j; passwords managed locally)
	`CREATE TABLE IF NOT EXISTS users (
		id              CHAR(36) PRIMARY KEY,
		username        VARCHAR(64)  NOT NULL UNIQUE,
		full_name       VARCHAR(191),
		email           VARCHAR(191),
		whatsapp        VARCHAR(32),
		odoo_id         VARCHAR(32),
		role            VARCHAR(32)  NOT NULL,
		password_hash   VARCHAR(128) NOT NULL,
		must_change_pw  TINYINT(1)   NOT NULL DEFAULT 1,
		is_lead         TINYINT(1)   NOT NULL DEFAULT 0,
		active          TINYINT(1)   NOT NULL DEFAULT 1,
		created_at      DATETIME     DEFAULT CURRENT_TIMESTAMP,
		updated_at      DATETIME     DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_users_role (role)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Auth — Sessions (cookie tokens; revoked by deleting the row)
	`CREATE TABLE IF NOT EXISTS sessions (
		token       CHAR(64) PRIMARY KEY,
		user_id     CHAR(36) NOT NULL,
		expires_at  DATETIME NOT NULL,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_sessions_user (user_id),
		INDEX idx_sessions_expiry (expires_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// products: master list of products under test (repo, staging URL, checkout in the repo pool).
	`CREATE TABLE IF NOT EXISTS products (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		repo_url VARCHAR(300) NOT NULL,
		staging_url VARCHAR(500) NOT NULL,
		root_dir VARCHAR(200) NOT NULL,
		branch VARCHAR(200) NOT NULL DEFAULT '',
		playwright_accessible TINYINT(1) NOT NULL DEFAULT 0,
		last_verified_at DATETIME NULL,
		verify_report JSON NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uq_products_name (name),
		UNIQUE KEY uq_products_root_dir (root_dir)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
}

// RunMigrations executes all MySQL migrations sequentially.
func RunMigrations(sqlDB *sql.DB) error {
	log.Println("📦 Running MySQL migrations...")

	for i, migration := range migrations {
		if _, err := sqlDB.ExecContext(context.Background(), migration); err != nil {
			return fmt.Errorf("migration %d failed: %w\nSQL: %s", i, err, summarize(migration))
		}
		log.Printf("  ✅ Migration %d/%d completed", i+1, len(migrations))
	}

	log.Println("✅ All MySQL migrations completed successfully")
	return nil
}

// summarize returns the first non-empty line of a SQL statement for log brevity.
func summarize(sqlStr string) string {
	for _, line := range strings.Split(sqlStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return sqlStr
}
