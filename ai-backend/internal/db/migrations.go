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

	// One-time seed: HISv3 uses the HISv3 test catalog. Skipped forever once any product has a
	// value (including an admin's ''), so it never overrides a decision.
	`-- skip-if: SELECT COUNT(*) FROM product_settings WHERE knowledge_format IS NOT NULL
INSERT INTO product_settings (product_id, knowledge_format)
SELECT id, 'hisv3-test-catalog-v1' FROM products WHERE name = 'HISv3'
ON DUPLICATE KEY UPDATE product_settings.knowledge_format = COALESCE(product_settings.knowledge_format, 'hisv3-test-catalog-v1');`,
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
  `+"`at`"+` DATETIME(3) NOT NULL,
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
