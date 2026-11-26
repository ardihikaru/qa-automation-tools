package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// migrations contains all MySQL 8 DDL statements for the qa_automation
// schema (InnoDB, utf8mb4, a single database with no sub-schemas).
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

	// product_settings (PRODUCT_REPOS.md entry 1): per-product flags. A missing row means defaults (false).
	`CREATE TABLE IF NOT EXISTS product_settings (
  product_id INT PRIMARY KEY,
  allow_non_automation_work TINYINT(1) NOT NULL DEFAULT 0,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_product_settings_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// product_repos (entry 2): the repos of a product. main_flag/central_flag are generated from the
	// flags (not from product_id, which is in the CASCADE foreign key; MySQL error 3104) so that
	// UNIQUE(product_id, flag) allows at most one main and one central repo per product.
	`CREATE TABLE IF NOT EXISTS product_repos (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  name VARCHAR(100) NOT NULL,
  repo_url VARCHAR(300) NOT NULL,
  root_dir VARCHAR(200) NOT NULL,
  is_main TINYINT(1) NOT NULL DEFAULT 0,
  is_central TINYINT(1) NOT NULL DEFAULT 0,
  main_flag    TINYINT GENERATED ALWAYS AS (IF(is_main = 1, 1, NULL)) STORED,
  central_flag TINYINT GENERATED ALWAYS AS (IF(is_central = 1, 1, NULL)) STORED,
  playwright_accessible TINYINT(1) NOT NULL DEFAULT 0,
  last_verified_at DATETIME NULL,
  verify_report JSON NULL,
  sync_report JSON NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_product_repos_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_product_repos_root_dir (root_dir),
  UNIQUE KEY uq_product_repos_name (product_id, name),
  UNIQUE KEY uq_product_repos_url (product_id, repo_url),
  UNIQUE KEY uq_product_repos_main (product_id, main_flag),
  UNIQUE KEY uq_product_repos_central (product_id, central_flag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// product_repo_branches (entry 3): branches kept checked out per repo; one default at most.
	// Binary collation: git refs are case-sensitive.
	`CREATE TABLE IF NOT EXISTS product_repo_branches (
  id INT AUTO_INCREMENT PRIMARY KEY,
  repo_id INT NOT NULL,
  branch VARCHAR(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  slug   VARCHAR(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  is_default TINYINT(1) NOT NULL DEFAULT 0,
  default_flag TINYINT GENERATED ALWAYS AS (IF(is_default = 1, 1, NULL)) STORED,
  CONSTRAINT fk_product_repo_branches_repo FOREIGN KEY (repo_id) REFERENCES product_repos(id) ON DELETE CASCADE,
  UNIQUE KEY uq_product_repo_branches (repo_id, branch),
  UNIQUE KEY uq_product_repo_branch_slug (repo_id, slug),
  UNIQUE KEY uq_product_repo_branch_default (repo_id, default_flag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Backfill 1 (entry 4): one main repo per legacy product. Skips (does not abort on) products whose
	// root_dir has a ".branches" segment or equals an existing repo's root_dir. Re-runs are no-ops.
	`INSERT INTO product_repos (product_id, name, repo_url, root_dir, is_main, is_central,
                           playwright_accessible, last_verified_at, verify_report)
SELECT p.id, 'main', p.repo_url, p.root_dir, 1, 0, p.playwright_accessible, p.last_verified_at, p.verify_report
FROM products p
WHERE NOT EXISTS (SELECT 1 FROM product_repos r WHERE r.product_id = p.id)
  AND p.root_dir NOT REGEXP '(^|/)[^/]*\\.branches(/|$)'
  AND NOT EXISTS (SELECT 1 FROM product_repos r2 WHERE r2.root_dir = p.root_dir)
ON DUPLICATE KEY UPDATE product_id = product_id;`,

	// Backfill 2 (entry 5): the legacy branch becomes the default and sole branch, only for main repos
	// that have no branch rows yet.
	`INSERT INTO product_repo_branches (repo_id, branch, slug, is_default)
SELECT r.id, p.branch, REPLACE(p.branch, '/', '--'), 1
FROM product_repos r JOIN products p ON p.id = r.product_id
WHERE r.is_main = 1 AND p.branch <> ''
  AND NOT EXISTS (SELECT 1 FROM product_repo_branches b WHERE b.repo_id = r.id)
ON DUPLICATE KEY UPDATE product_repo_branches.branch = product_repo_branches.branch;`,
	// Impersonation (docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md).
	// One row per session that is currently acting as another user, keyed by
	// the real session's token (the cookie never changes value).
	`CREATE TABLE IF NOT EXISTS active_impersonations (
		parent_token    CHAR(64) NOT NULL PRIMARY KEY,
		impersonator_id CHAR(36) NOT NULL,
		target_user_id  CHAR(36) NOT NULL,
		audit_id        BIGINT   NOT NULL,
		expires_at      DATETIME NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Append-only audit of every impersonation. Never purge.
	// ended_reason: stop | logout | superseded; NULL with expires_at in the
	// past means the impersonation expired.
	`CREATE TABLE IF NOT EXISTS impersonation_audit (
		id              BIGINT AUTO_INCREMENT PRIMARY KEY,
		impersonator_id CHAR(36)     NOT NULL,
		target_user_id  CHAR(36)     NOT NULL,
		started_at      DATETIME     NOT NULL,
		expires_at      DATETIME     NOT NULL,
		ended_at        DATETIME     NULL,
		ended_reason    VARCHAR(16)  NULL,
		ip              VARCHAR(64)  NULL,
		user_agent      VARCHAR(255) NULL,
		KEY idx_ia_impersonator (impersonator_id),
		KEY idx_ia_target (target_user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// product_repos_add_automation_role (PRODUCT_REPOS_AUTOMATION_ROLE.md): the automation repo role.
	// A STORED generated column cannot be added in place, so ALGORITHM=COPY, LOCK=SHARED is explicit.
	// One ALTER is all-or-nothing, so the presence of automation_flag proves the whole change; the
	// leading skip-if guard (see parseSkipIf) makes the entry idempotent.
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'product_repos' AND column_name = 'automation_flag'
ALTER TABLE product_repos ALGORITHM=COPY, LOCK=SHARED,
  ADD COLUMN is_automation TINYINT(1) NOT NULL DEFAULT 0 AFTER is_central,
  ADD COLUMN automation_flag TINYINT GENERATED ALWAYS AS (IF(is_automation = 1, 1, NULL)) STORED AFTER central_flag,
  ADD UNIQUE KEY uq_product_repos_automation (product_id, automation_flag);`,

	// product_repos_backfill_automation_role: products that have no automation repo yet get
	// automation = main. Set-based and a no-op on re-run and where automation != main.
	`UPDATE product_repos m
LEFT JOIN product_repos a ON a.product_id = m.product_id AND a.is_automation = 1
SET m.is_automation = 1
WHERE m.is_main = 1 AND a.id IS NULL;`,

	// ---- PRODUCT_KNOWLEDGE_SYNC.md: product knowledge base. Codes are case-sensitive (utf8mb4_bin);
	// links between entities are kept by code. kb_syncs.running_flag is generated from status (not
	// from the FK column, MySQL error 3104) so UNIQUE(product_id, running_flag) allows one running sync.
	`CREATE TABLE IF NOT EXISTS kb_syncs (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  repo_id INT NULL,
  repo_name VARCHAR(100) NOT NULL DEFAULT '',
  repo_url VARCHAR(300) NOT NULL DEFAULT '',
  branch VARCHAR(200) NOT NULL DEFAULT '',
  format VARCHAR(50) NOT NULL DEFAULT '',
  parser_version INT NOT NULL DEFAULT 0,
  commit_sha CHAR(40) NULL,
  status ENUM('running','succeeded','failed','skipped') NOT NULL,
  trigger_kind ENUM('manual','schedule') NOT NULL,
  triggered_by CHAR(36) NULL,
  started_at DATETIME NOT NULL,
  finished_at DATETIME NULL,
  counts JSON NULL,
  diff JSON NULL,
  problems JSON NULL,
  ai_status ENUM('none','pending','done','failed') NOT NULL DEFAULT 'none',
  ai_summary MEDIUMTEXT NULL,
  ai_error VARCHAR(500) NULL,
  error VARCHAR(1000) NULL,
  running_flag TINYINT GENERATED ALWAYS AS (IF(status = 'running', 1, NULL)) STORED,
  CONSTRAINT fk_kb_syncs_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_syncs_running (product_id, running_flag),
  INDEX idx_kb_syncs_product (product_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_parts (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  name VARCHAR(300) NOT NULL DEFAULT '',
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  content_hash CHAR(64) NOT NULL,
  first_sync_id INT NULL,
  changed_sync_id INT NULL,
  removed_at DATETIME NULL,
  removed_sync_id INT NULL,
  source_file VARCHAR(300) NULL,
  source_line INT NULL,
  created_by CHAR(36) NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_kb_parts_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_parts_code (product_id, code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_modules (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  part_code CHAR(1) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  name VARCHAR(300) NOT NULL DEFAULT '',
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  content_hash CHAR(64) NOT NULL,
  first_sync_id INT NULL,
  changed_sync_id INT NULL,
  removed_at DATETIME NULL,
  removed_sync_id INT NULL,
  source_file VARCHAR(300) NULL,
  source_line INT NULL,
  created_by CHAR(36) NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_kb_modules_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_modules_code (product_id, code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_submodules (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  module_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  name VARCHAR(300) NOT NULL DEFAULT '',
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  content_hash CHAR(64) NOT NULL,
  first_sync_id INT NULL,
  changed_sync_id INT NULL,
  removed_at DATETIME NULL,
  removed_sync_id INT NULL,
  source_file VARCHAR(300) NULL,
  source_line INT NULL,
  created_by CHAR(36) NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_kb_submodules_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_submodules_code (product_id, code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_features (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  module_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  submodule_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
  name TEXT NULL,
  repo_entry VARCHAR(300) NOT NULL DEFAULT '',
  release_status VARCHAR(20) NOT NULL DEFAULT 'UNKNOWN',
  status_as_of VARCHAR(20) NOT NULL DEFAULT '',
  evidence VARCHAR(500) NOT NULL DEFAULT '',
  notes TEXT NULL,
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  content_hash CHAR(64) NOT NULL,
  first_sync_id INT NULL,
  changed_sync_id INT NULL,
  removed_at DATETIME NULL,
  removed_sync_id INT NULL,
  source_file VARCHAR(300) NULL,
  source_line INT NULL,
  created_by CHAR(36) NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_kb_features_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_features_code (product_id, code),
  INDEX idx_kb_features_module (product_id, module_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_epics (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  title VARCHAR(500) NOT NULL DEFAULT '',
  service VARCHAR(100) NOT NULL DEFAULT '',
  area VARCHAR(200) NOT NULL DEFAULT '',
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  content_hash CHAR(64) NOT NULL,
  first_sync_id INT NULL,
  changed_sync_id INT NULL,
  removed_at DATETIME NULL,
  removed_sync_id INT NULL,
  source_file VARCHAR(300) NULL,
  source_line INT NULL,
  created_by CHAR(36) NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_kb_epics_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_epics_code (product_id, code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_stories (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  epic_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  text TEXT NULL,
  unmapped_reason VARCHAR(500) NULL,
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  content_hash CHAR(64) NOT NULL,
  first_sync_id INT NULL,
  changed_sync_id INT NULL,
  removed_at DATETIME NULL,
  removed_sync_id INT NULL,
  source_file VARCHAR(300) NULL,
  source_line INT NULL,
  created_by CHAR(36) NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_kb_stories_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_stories_code (product_id, code),
  INDEX idx_kb_stories_epic (product_id, epic_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_test_cases (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  epic_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  story_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
  scenario TEXT NULL,
  polarity VARCHAR(4) NOT NULL DEFAULT '',
  level VARCHAR(8) NOT NULL DEFAULT '',
  priority VARCHAR(4) NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT '',
  automated TINYINT(1) NOT NULL DEFAULT 0,
  spec_paths JSON NOT NULL,
  reason_category VARCHAR(30) NOT NULL DEFAULT '',
  reason_detail VARCHAR(1000) NOT NULL DEFAULT '',
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  content_hash CHAR(64) NOT NULL,
  first_sync_id INT NULL,
  changed_sync_id INT NULL,
  removed_at DATETIME NULL,
  removed_sync_id INT NULL,
  source_file VARCHAR(300) NULL,
  source_line INT NULL,
  created_by CHAR(36) NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_kb_test_cases_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_test_cases_code (product_id, code),
  INDEX idx_kb_test_cases_epic (product_id, epic_code),
  INDEX idx_kb_test_cases_story (product_id, story_code),
  INDEX idx_kb_test_cases_status (product_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_story_features (
  product_id INT NOT NULL,
  story_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  feature_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  PRIMARY KEY (product_id, story_code, feature_code),
  INDEX idx_kb_story_features_feature (product_id, feature_code),
  CONSTRAINT fk_kb_story_features_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`CREATE TABLE IF NOT EXISTS kb_annotations (
  product_id INT NOT NULL,
  entity_type ENUM('part','module','submodule','feature','epic','story','test_case') NOT NULL,
  entity_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  note TEXT NULL,
  tags JSON NULL,
  updated_by CHAR(36) NULL,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (product_id, entity_type, entity_code),
  CONSTRAINT fk_kb_annotations_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// product_settings.knowledge_format: NULL = never set, '' = sync deliberately off.
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'product_settings' AND column_name = 'knowledge_format'
ALTER TABLE product_settings ADD COLUMN knowledge_format VARCHAR(50) NULL DEFAULT NULL;`,

	// One-time seed: SUT uses the SUT test catalog. Skipped forever once any product has a
	// value (including an admin's ''), so it never overrides a decision.
	`-- skip-if: SELECT COUNT(*) FROM product_settings WHERE knowledge_format IS NOT NULL
INSERT INTO product_settings (product_id, knowledge_format)
SELECT id, 'test-catalog-v1' FROM products WHERE name = 'SUT'
ON DUPLICATE KEY UPDATE product_settings.knowledge_format = COALESCE(product_settings.knowledge_format, 'test-catalog-v1');`,
	// COST_TRACKING: one row per AI call attempt made by this backend (success or failure), with
	// the feature that issued it. Replaces the never-written request_logs for the /cost page.
	`CREATE TABLE IF NOT EXISTS ai_call_log (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  feature VARCHAR(64) NOT NULL DEFAULT 'unknown',
  caller VARCHAR(64) NOT NULL DEFAULT '',
  provider VARCHAR(64) NOT NULL DEFAULT '',
  model VARCHAR(128) NOT NULL DEFAULT '',
  requested_provider VARCHAR(64) NOT NULL DEFAULT '',
  status ENUM('ok','error') NOT NULL,
  error VARCHAR(1000) NULL,
  fallback TINYINT(1) NOT NULL DEFAULT 0,
  input_tokens INT NOT NULL DEFAULT 0,
  output_tokens INT NOT NULL DEFAULT 0,
  cost_usd DECIMAL(12,6) NOT NULL DEFAULT 0,
  cost_idr DECIMAL(16,2) NOT NULL DEFAULT 0,
  cost_source VARCHAR(32) NOT NULL DEFAULT '',
  latency_ms INT NOT NULL DEFAULT 0,
  user_id CHAR(36) NULL,
  job_id VARCHAR(64) NULL,
  INDEX idx_ai_call_log_created (created_at),
  INDEX idx_ai_call_log_feature (feature, created_at),
  INDEX idx_ai_call_log_status (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// ---- PRODUCT_KNOWLEDGE_SYNC.md "Assistant files": agents, skills, rules, commands and
	// CLAUDE.md/AGENTS.md of the automation repo, synced with the catalog.
	`CREATE TABLE IF NOT EXISTS kb_assistants (
  id INT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  kind ENUM('agent','skill','rule','command','instructions') NOT NULL,
  name VARCHAR(300) NOT NULL DEFAULT '',
  description VARCHAR(1000) NOT NULL DEFAULT '',
  path VARCHAR(300) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  frontmatter JSON NOT NULL,
  body MEDIUMTEXT NULL,
  origin ENUM('sync','local') NOT NULL DEFAULT 'sync',
  content_hash CHAR(64) NOT NULL,
  first_sync_id INT NULL,
  changed_sync_id INT NULL,
  removed_at DATETIME NULL,
  removed_sync_id INT NULL,
  source_file VARCHAR(300) NULL,
  source_line INT NULL,
  created_by CHAR(36) NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  CONSTRAINT fk_kb_assistants_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  UNIQUE KEY uq_kb_assistants_code (product_id, code),
  INDEX idx_kb_assistants_kind (product_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,

	// Annotations on assistant files.
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'kb_annotations' AND column_name = 'entity_type' AND column_type LIKE '%assistant%'
ALTER TABLE kb_annotations MODIFY entity_type ENUM('part','module','submodule','feature','epic','story','test_case','assistant') NOT NULL;`,

	// QA check skills may belong to one product (NULL = global, shown for every product).
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'skills' AND column_name = 'product_id'
ALTER TABLE skills ADD COLUMN product_id INT NULL, ADD INDEX idx_skills_product (product_id);`,

	// GAOS_INTEGRATION.md: per-product "integrated with GAOS" settings and the last push outcome.
	// One ALTER, so it is atomic; the guard tests its first column.
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'product_settings' AND column_name = 'gaos_integrated'
ALTER TABLE product_settings
  ADD COLUMN gaos_integrated TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN gaos_product_id VARCHAR(40) NOT NULL DEFAULT '',
  ADD COLUMN gaos_last_sync_at DATETIME NULL,
  ADD COLUMN gaos_last_status VARCHAR(20) NOT NULL DEFAULT '',
  ADD COLUMN gaos_last_note VARCHAR(300) NOT NULL DEFAULT '';`,

	// GAOS_INTEGRATION.md (durable push): the outbox marker. gaos_push_requested_at is "a push is owed"
	// (NULL = nothing owed); gaos_push_attempts counts failed passes since the marker was set.
	// One ALTER, so it is atomic; the guard tests its first column.
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'product_settings' AND column_name = 'gaos_push_requested_at'
ALTER TABLE product_settings
  ADD COLUMN gaos_push_requested_at DATETIME(6) NULL,
  ADD COLUMN gaos_push_attempts INT NOT NULL DEFAULT 0;`,

	// GAOS_INBOUND.md: planning decisions GAOS pushes to POST /api/v1/gaos/catalog. Never touches kb_*.
	`CREATE TABLE IF NOT EXISTS gaos_decisions (
  product_id INT NOT NULL,
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  epic VARCHAR(16) NOT NULL DEFAULT '',
  module VARCHAR(16) NOT NULL DEFAULT '',
  features JSON NOT NULL,
  user_story VARCHAR(32) NOT NULL DEFAULT '',
  polarity VARCHAR(4) NOT NULL DEFAULT '',
  level VARCHAR(8) NOT NULL DEFAULT '',
  pri VARCHAR(4) NOT NULL DEFAULT '',
  decision ENUM('planned','blocked','manual-only','wont') NOT NULL,
  reason VARCHAR(300) NOT NULL DEFAULT '',
  cat_rev BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (product_id, case_code),
  CONSTRAINT fk_gaos_decisions_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// CASE_RESULTS.md: the latest automated result of each test case, from a Playwright JSON report.
	`CREATE TABLE IF NOT EXISTS kb_case_results (
  product_id INT NOT NULL,
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  result ENUM('pass','fail','skipped') NOT NULL,
  run_at DATETIME NOT NULL COMMENT 'UTC start of the run that produced the result',
  run_ref VARCHAR(120) NOT NULL DEFAULT '',
  source VARCHAR(40) NOT NULL DEFAULT '',
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (product_id, case_code),
  CONSTRAINT fk_kb_case_results_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// CASE_AUTOMATION.md: the live event log of a case_run / case_create job (polled by the dashboard).
	`CREATE TABLE IF NOT EXISTS job_events (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  job_id VARCHAR(64) NOT NULL,
  seq INT NOT NULL,
  ` + "`at`" + ` DATETIME(3) NOT NULL,
  level VARCHAR(8) NOT NULL,
  step VARCHAR(32) NOT NULL,
  message VARCHAR(1000) NOT NULL,
  UNIQUE KEY uq_job_seq (job_id, seq),
  KEY idx_job (job_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
	// CASE_AUTOMATION.md: one approval gate per generated spec; two doors (dashboard, GAOS Inbox), one record.
	`CREATE TABLE IF NOT EXISTS case_approvals (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  job_id VARCHAR(64) NOT NULL,
  product_id INT NOT NULL,
  case_code VARCHAR(64) NOT NULL,
  external_id VARCHAR(80) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'open',
  answered_by VARCHAR(16) NULL,
  answer_text VARCHAR(1000) NULL,
  gaos_created TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  answered_at DATETIME NULL,
  UNIQUE KEY uq_ext (external_id),
  KEY idx_case_approvals_job (job_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
	// EPIC_AUTOMATION.md: a snapshot queue of one epic's planned cases, worked through two at a time by
	// the driver. Case state is derived from job_runs (the jobs carry batch_id in their payload);
	// running_flag is generated from status so UNIQUE(product_id, running_flag) allows one running batch.
	`CREATE TABLE IF NOT EXISTS epic_batches (
  id VARCHAR(40) PRIMARY KEY,
  product_id INT NOT NULL,
  epic_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  status VARCHAR(12) NOT NULL DEFAULT 'running',
  cases JSON NOT NULL,
  started_by VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(300) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  running_flag TINYINT GENERATED ALWAYS AS (IF(status = 'running', 1, NULL)) STORED,
  UNIQUE KEY uq_epic_batches_running (product_id, running_flag),
  INDEX idx_epic_batches_product (product_id, epic_code, created_at),
  CONSTRAINT fk_epic_batches_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// MODULE_CHECKLIST_EXPORT.md: when each feature's release status changed. Written by the sync
	// (store.go apply) in the sync's transaction. No FK to kb_features: codes are the link.
	`CREATE TABLE IF NOT EXISTS kb_feature_status_history (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  feature_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  release_status VARCHAR(20) NOT NULL,
  prev_status VARCHAR(20) NULL,
  sync_id INT NULL,
  commit_sha CHAR(40) NULL,
  changed_at DATETIME NOT NULL,
  source ENUM('sync','backfill') NOT NULL,
  CONSTRAINT fk_kb_fsh_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  INDEX idx_kb_fsh_feature (product_id, feature_code, changed_at),
  UNIQUE KEY uq_kb_fsh_event (product_id, feature_code, sync_id, source)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// MODULE_CHECKLIST_EXPORT.md: backfill of a starting (approximate) date for existing features.
	// Unguarded on purpose: the status-aware NOT EXISTS makes a second run a no-op and also repairs
	// the deploy window (a status changed by the old binary after the first run). The no-op UPDATE
	// qualifies the column: a bare "id" is ambiguous in an INSERT ... SELECT over kb_features.
	`INSERT INTO kb_feature_status_history
  (product_id, feature_code, release_status, prev_status, sync_id, commit_sha, changed_at, source)
SELECT f.product_id, f.code, f.release_status, NULL, f.changed_sync_id, s.commit_sha, s.started_at, 'backfill'
FROM kb_features f
JOIN kb_syncs s ON s.id = f.changed_sync_id AND s.product_id = f.product_id
WHERE f.origin = 'sync' AND f.removed_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM kb_feature_status_history h
                  WHERE h.product_id = f.product_id AND h.feature_code = f.code
                    AND h.release_status = f.release_status)
ON DUPLICATE KEY UPDATE kb_feature_status_history.id = kb_feature_status_history.id;`,
	// RUN_HISTORY.md: every Playwright report the backend receives (CI, upload, case_run and each
	// case_create executor run), append-only. kb_case_results (latest per case, sent to GAOS) is untouched.
	`CREATE TABLE IF NOT EXISTS pw_runs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  source VARCHAR(16) NOT NULL,
  run_ref VARCHAR(120) NOT NULL DEFAULT '',
  job_id VARCHAR(64) NOT NULL DEFAULT '',
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
  run_index INT NULL,
  started_at DATETIME(3) NOT NULL,
  duration_ms BIGINT NULL,
  tests INT NOT NULL DEFAULT 0,
  expected INT NOT NULL DEFAULT 0,
  unexpected INT NOT NULL DEFAULT 0,
  flaky INT NOT NULL DEFAULT 0,
  skipped INT NOT NULL DEFAULT 0,
  cases INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uq_pw_runs_report (product_id, source, run_ref, started_at),
  INDEX idx_pw_runs_started (started_at),
  INDEX idx_pw_runs_product (product_id, started_at),
  INDEX idx_pw_runs_job (job_id),
  CONSTRAINT fk_pw_runs_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// RUN_HISTORY.md: one row per case code of a pw_runs report (result and time of its tests).
	`CREATE TABLE IF NOT EXISTS pw_run_cases (
  run_id BIGINT NOT NULL,
  product_id INT NOT NULL,
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  result ENUM('pass','fail','skipped') NOT NULL,
  duration_ms BIGINT NOT NULL DEFAULT 0,
  tests INT NOT NULL DEFAULT 0,
  attempts INT NOT NULL DEFAULT 0,
  PRIMARY KEY (run_id, case_code),
  INDEX idx_pw_run_cases_case (product_id, case_code, run_id),
  CONSTRAINT fk_pw_run_cases_run FOREIGN KEY (run_id) REFERENCES pw_runs(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// AI_CONVERSATIONS.md: one conversation per job, knowledge sync or single AI call. last_hashes is
	// the previous call (sent + reply) for the append-only dedupe; archived rows are what prune deletes.
	`CREATE TABLE IF NOT EXISTS ai_conversations (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  kind VARCHAR(8) NOT NULL,
  ref VARCHAR(64) NOT NULL,
  product_id INT NULL,
  feature VARCHAR(64) NOT NULL DEFAULT '',
  title VARCHAR(200) NOT NULL DEFAULT '',
  provider VARCHAR(64) NOT NULL DEFAULT '',
  model VARCHAR(128) NOT NULL DEFAULT '',
  message_count INT NOT NULL DEFAULT 0,
  bytes BIGINT NOT NULL DEFAULT 0,
  last_hashes JSON NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  archived_at DATETIME(3) NULL,
  archived_by VARCHAR(64) NULL,
  UNIQUE KEY uq_ai_conversations_ref (kind, ref),
  INDEX idx_ai_conversations_list (archived_at, updated_at),
  INDEX idx_ai_conversations_product (product_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// AI_CONVERSATIONS.md: the messages of a conversation, redacted, in order; deleted with it.
	`CREATE TABLE IF NOT EXISTS ai_messages (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  conversation_id BIGINT NOT NULL,
  seq INT NOT NULL,
  role VARCHAR(12) NOT NULL,
  kind VARCHAR(12) NOT NULL,
  content MEDIUMTEXT NOT NULL,
  content_hash CHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uq_ai_messages_seq (conversation_id, seq),
  CONSTRAINT fk_ai_messages_conversation FOREIGN KEY (conversation_id) REFERENCES ai_conversations(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// AI_CONVERSATIONS.md: append-only record of archive, unarchive and prune (the real actor).
	`CREATE TABLE IF NOT EXISTS ai_conversation_audit (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  action ENUM('archive','unarchive','prune') NOT NULL,
  actor_id VARCHAR(64) NOT NULL,
  actor_name VARCHAR(100) NOT NULL DEFAULT '',
  as_user_id VARCHAR(64) NULL,
  conversation_id BIGINT NULL,
  detail JSON NOT NULL,
  INDEX idx_ai_conversation_audit_action (action, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// LEARNING_LOOP.md: findings observed in real automation work (reviewer answers, repairs,
	// repeated failures, manual entries), deduplicated by fingerprint per product. A proposal is a
	// policy_propose job (job_runs); proposal_job_id links the active or last one.
	`CREATE TABLE IF NOT EXISTS learning_findings (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  kind VARCHAR(24) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'new',
  title VARCHAR(200) NOT NULL,
  detail TEXT NOT NULL,
  fingerprint CHAR(64) NOT NULL,
  occurrences INT NOT NULL DEFAULT 1,
  proposal_job_id VARCHAR(191) NULL,
  created_by VARCHAR(64) NOT NULL DEFAULT 'system',
  status_note VARCHAR(300) NOT NULL DEFAULT '',
  first_seen_at DATETIME NOT NULL,
  last_seen_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_lf_product (product_id, status, last_seen_at),
  INDEX idx_lf_fp (product_id, fingerprint),
  INDEX idx_lf_job (proposal_job_id),
  CONSTRAINT fk_lf_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// LEARNING_LOOP.md: one row per piece of evidence; source_key makes a re-harvest a no-op.
	`CREATE TABLE IF NOT EXISTS learning_evidence (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  finding_id BIGINT NOT NULL,
  source_key VARCHAR(160) NOT NULL,
  job_id VARCHAR(191) NOT NULL DEFAULT '',
  case_code VARCHAR(64) NOT NULL DEFAULT '',
  run_ref VARCHAR(120) NOT NULL DEFAULT '',
  excerpt VARCHAR(1000) NOT NULL DEFAULT '',
  observed_at DATETIME NOT NULL,
  UNIQUE KEY uq_le_source (source_key),
  INDEX idx_le_finding (finding_id, observed_at),
  CONSTRAINT fk_le_finding FOREIGN KEY (finding_id) REFERENCES learning_findings(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// NAVIGATION_MAP.md: the product's route and page inventory, replaced as a whole by each
	// navigation build (after a knowledge sync). route_key = sha1(surface NUL app NUL path).
	`CREATE TABLE IF NOT EXISTS nav_routes (
  product_id INT NOT NULL,
  route_key CHAR(40) NOT NULL,
  surface VARCHAR(8) NOT NULL,
  app VARCHAR(100) NOT NULL,
  path VARCHAR(300) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  methods JSON NOT NULL,
  sources JSON NOT NULL,
  page_objects JSON NOT NULL,
  PRIMARY KEY (product_id, route_key),
  CONSTRAINT fk_nav_routes_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// NAVIGATION_MAP.md: feature, case and spec links to nav_routes (same lifecycle).
	`CREATE TABLE IF NOT EXISTS nav_links (
  product_id INT NOT NULL,
  route_key CHAR(40) NOT NULL,
  ref_type VARCHAR(8) NOT NULL,
  ref VARCHAR(300) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  source VARCHAR(10) NOT NULL,
  method VARCHAR(8) NOT NULL DEFAULT '',
  PRIMARY KEY (product_id, route_key, ref_type, ref, source, method),
  INDEX idx_nav_links_ref (product_id, ref_type, ref),
  CONSTRAINT fk_nav_links_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// NAVIGATION_MAP.md: the last navigation build per product (its version triggers a rebuild).
	`CREATE TABLE IF NOT EXISTS nav_builds (
  product_id INT NOT NULL PRIMARY KEY,
  sync_id INT NULL,
  commit_sha CHAR(40) NULL,
  nav_version INT NOT NULL,
  status VARCHAR(10) NOT NULL,
  error VARCHAR(500) NOT NULL DEFAULT '',
  routes INT NOT NULL DEFAULT 0,
  links INT NOT NULL DEFAULT 0,
  problems JSON NULL,
  built_at DATETIME NOT NULL,
  CONSTRAINT fk_nav_builds_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// NAVIGATION_MAP.md: routes a real executor run visited, per job, executor run and case. Only
	// matched route keys are stored (never a raw path); rows older than 180 days are pruned.
	`CREATE TABLE IF NOT EXISTS nav_observations (
  product_id INT NOT NULL,
  route_key CHAR(40) NOT NULL,
  job_id VARCHAR(64) NOT NULL,
  run_id VARCHAR(64) NOT NULL DEFAULT '',
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  method VARCHAR(8) NOT NULL DEFAULT '',
  hits INT NOT NULL DEFAULT 1,
  observed_at DATETIME NOT NULL,
  PRIMARY KEY (job_id, run_id, case_code, route_key, method),
  INDEX idx_nav_obs_route (product_id, route_key, observed_at),
  CONSTRAINT fk_nav_obs_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// HISTORY_ANSWER.md: where an approval's answer came from (gaos, qa-tool:history, qa-tool:job-page,
	// qa-tool:learning, system), who gave it, and GAOS's own source object. One ALTER, so it is
	// atomic; the guard tests its first column.
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'case_approvals' AND column_name = 'answer_source'
ALTER TABLE case_approvals
  ADD COLUMN answer_source VARCHAR(32) NULL,
  ADD COLUMN answer_actor VARCHAR(120) NULL,
  ADD COLUMN answer_source_detail JSON NULL;`,
	// QA_DEFECT_TRIAGE.md: the last defect state of each case (xfail | xpass | fail) and what was
	// pushed to GAOS. hash is the content that matters (kind, defect id, title, case, spec); a push is
	// owed while hash differs from pushed_hash and rejected_hash.
	`CREATE TABLE IF NOT EXISTS qa_defect_state (
  product_id INT NOT NULL,
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  kind VARCHAR(8) NOT NULL,
  defect_id VARCHAR(12) NOT NULL DEFAULT '',
  title VARCHAR(255) NOT NULL DEFAULT '',
  excerpt TEXT NULL,
  run_ref VARCHAR(120) NOT NULL DEFAULT '',
  run_at DATETIME NOT NULL COMMENT 'UTC start of the run that last showed this state',
  hash CHAR(64) NOT NULL,
  pushed_hash CHAR(64) NULL,
  pushed_at DATETIME NULL,
  rejected_hash CHAR(64) NULL,
  rejected_reason VARCHAR(80) NOT NULL DEFAULT '',
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (product_id, case_code),
  CONSTRAINT fk_qa_defect_state_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// QA_DEFECT_TRIAGE.md: when an older GAOS (404/405) or a daily cap makes the defect push wait.
	`CREATE TABLE IF NOT EXISTS qa_defect_push (
  product_id INT NOT NULL PRIMARY KEY,
  backoff_until DATETIME NULL,
  note VARCHAR(120) NOT NULL DEFAULT '',
  CONSTRAINT fk_qa_defect_push_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// QA_DEFECT_TRIAGE.md: which product repositories (by product_repos.name) a service or app directory
	// of the automation repo's tests/ tree stands for. Only e2e cases read it, for their backends.
	`CREATE TABLE IF NOT EXISTS product_service_repos (
  product_id INT NOT NULL,
  service_dir VARCHAR(100) NOT NULL,
  repo_name VARCHAR(100) NOT NULL,
  PRIMARY KEY (product_id, service_dir, repo_name),
  CONSTRAINT fk_product_service_repos_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// The test-catalog knowledge format lost its product prefix ("<product>-test-catalog-v1" is now
	// "test-catalog-v1"). Idempotent: a second run matches no rows.
	`UPDATE product_settings SET knowledge_format = 'test-catalog-v1'
WHERE knowledge_format LIKE '%-test-catalog-v1';`,
	// MODULE_CHECKLIST_EXPORT.md (PROD scope): the catalogue's optional "Prod since" cell, kept raw
	// (YYYY-MM, YYYY-MM..YYYY-MM, <YYYY-MM, ? or '' when absent); the export parses it.
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'kb_features' AND column_name = 'prod_since'
ALTER TABLE kb_features ADD COLUMN prod_since VARCHAR(20) NOT NULL DEFAULT '' AFTER status_as_of;`,
	// MODULE_CHECKLIST_EXPORT.md (Amendment 2): the catalogue's optional "Periode pelaksanaan" cell
	// (development period), raw; modules and sub-modules roll it up from their features.
	`-- skip-if: SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'kb_features' AND column_name = 'dev_period'
ALTER TABLE kb_features ADD COLUMN dev_period VARCHAR(20) NOT NULL DEFAULT '' AFTER prod_since;`,
	// CASE_VERIFICATION_AND_EPIC_TAB.md: the latest human verdict of a test case. Kept apart from
	// kb_test_cases so a knowledge sync never overwrites it.
	`CREATE TABLE IF NOT EXISTS kb_case_verifications (
  product_id INT NOT NULL,
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  verdict ENUM('valid','invalid') NOT NULL,
  note VARCHAR(500) NOT NULL DEFAULT '',
  verified_by CHAR(36) NOT NULL,
  verified_by_name VARCHAR(191) NOT NULL DEFAULT '',
  verified_at DATETIME NOT NULL,
  PRIMARY KEY (product_id, case_code),
  INDEX idx_kb_case_verifications_by (product_id, verified_by),
  CONSTRAINT fk_kb_case_verifications_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// Append-only history of every verification change ('cleared' = the verdict was removed).
	`CREATE TABLE IF NOT EXISTS kb_case_verification_events (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  product_id INT NOT NULL,
  case_code VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
  verdict ENUM('valid','invalid','cleared') NOT NULL,
  note VARCHAR(500) NOT NULL DEFAULT '',
  verified_by CHAR(36) NOT NULL,
  verified_by_name VARCHAR(191) NOT NULL DEFAULT '',
  verified_at DATETIME NOT NULL,
  INDEX idx_kb_case_verification_events_case (product_id, case_code, id),
  CONSTRAINT fk_kb_case_verification_events_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	// System-wide settings (key/value). timezone is the zone the dashboard formats times in.
	`CREATE TABLE IF NOT EXISTS app_settings (
  k VARCHAR(64) NOT NULL PRIMARY KEY,
  v TEXT NOT NULL,
  updated_by CHAR(36) NULL,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;`,
	`INSERT IGNORE INTO app_settings (k, v) VALUES ('timezone', 'Asia/Jakarta');`,
}

const skipIfPrefix = "-- skip-if:"

// parseSkipIf reads the optional guard of a migration entry. Leading whitespace, newlines and a
// UTF-8 BOM are ignored; ok is true iff the first line starts with "-- skip-if:" (case-sensitive,
// one space after the dashes). query is the rest of that line, trimmed (it may be empty).
func parseSkipIf(sqlStr string) (query string, ok bool) {
	t := strings.TrimLeft(sqlStr, " \t\r\n\ufeff")
	line := t
	if i := strings.IndexByte(t, '\n'); i >= 0 {
		line = t[:i]
	}
	if !strings.HasPrefix(line, skipIfPrefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, skipIfPrefix)), true
}

// RunMigrations executes all MySQL migrations sequentially. An entry whose first line is a
// "-- skip-if: <query>" directive runs <query> first (outside any transaction); a value > 0 skips
// the entry, 0 runs it, anything else (error, no row, several rows, NULL) aborts the run. The whole
// entry text, comment included, is still what MySQL receives.
func RunMigrations(sqlDB *sql.DB) error {
	return RunMigrationList(sqlDB, migrations)
}

// RunMigrationList is RunMigrations over an explicit list (used by the runner tests).
func RunMigrationList(sqlDB *sql.DB, list []string) error {
	log.Println("📦 Running MySQL migrations...")
	ctx := context.Background()

	for i, migration := range list {
		if query, ok := parseSkipIf(migration); ok {
			if query == "" {
				return fmt.Errorf("migration %d skip-if guard failed: empty skip-if guard", i)
			}
			n, err := runGuard(ctx, sqlDB, query)
			if err != nil {
				return fmt.Errorf("migration %d skip-if guard failed: %w", i, err)
			}
			if n > 0 {
				log.Printf("  ⏭️  Migration %d/%d skipped (guard > 0)", i+1, len(list))
				continue
			}
			conn, err := sqlDB.Conn(ctx)
			if err != nil {
				return fmt.Errorf("migration %d failed: %w", i, err)
			}
			_, err = conn.ExecContext(ctx, "SET SESSION lock_wait_timeout = 30")
			if err == nil {
				_, err = conn.ExecContext(ctx, migration)
			}
			_ = conn.Close()
			if err != nil {
				return fmt.Errorf("migration %d failed: %w\nSQL: %s", i, err, summarize(migration))
			}
		} else if _, err := sqlDB.ExecContext(ctx, migration); err != nil {
			return fmt.Errorf("migration %d failed: %w\nSQL: %s", i, err, summarize(migration))
		}
		log.Printf("  ✅ Migration %d/%d completed", i+1, len(list))
	}

	log.Println("✅ All MySQL migrations completed successfully")
	return nil
}

// runGuard runs a skip-if query that must return exactly one row and one non-NULL integer column.
func runGuard(ctx context.Context, sqlDB *sql.DB, query string) (int64, error) {
	rows, err := sqlDB.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("guard returned no row")
	}
	var v sql.NullInt64
	if err := rows.Scan(&v); err != nil {
		return 0, err
	}
	if rows.Next() {
		return 0, fmt.Errorf("guard returned more than one row")
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, fmt.Errorf("guard returned NULL")
	}
	return v.Int64, nil
}

// summarize returns the first non-empty line of a SQL statement for log brevity, skipping a
// leading "-- skip-if:" directive.
func summarize(sqlStr string) string {
	for _, line := range strings.Split(sqlStr, "\n") {
		trimmed := strings.TrimSpace(strings.TrimLeft(line, "\ufeff"))
		if trimmed == "" || strings.HasPrefix(trimmed, skipIfPrefix) {
			continue
		}
		return trimmed
	}
	return sqlStr
}
