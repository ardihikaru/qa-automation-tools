/**
 * Config Loader - Centralized configuration for all projects
 * 
 * Loads config.yaml and provides typed access to all settings.
 * Used by: playwright-docs, playwright-qa, ai-backend, dashboard
 */

import * as fs from 'fs';
import * as path from 'path';
import * as yaml from 'yaml';

// ============================================================
// CONFIGURATION TYPES
// ============================================================

export interface StagingConfig {
  url: string;
  graph_api_url: string;
  rest_api_url: string;
  timeout: number;
  navigation_timeout: number;
  action_timeout: number;
}

export interface Credential {
  username: string;
  password: string;
  role_code: string;
}

export interface CredentialsConfig {
  [key: string]: Credential;
}

export interface BackendConfig {
  host: string;
  port: number;
  database: DatabaseConfig;
  ai: AIConfig;
  security: SecurityConfig;
}

export interface DatabaseConfig {
  host: string;
  port: number;
  username: string;
  password: string;
  name: string;
  connection_string?: string;
}

export interface AIConfig {
  /** "ai-manager" (default, production gateway over WireGuard) or "minimax" (direct fallback). */
  provider?: 'ai-manager' | 'minimax';
  /** ai-manager base URL, e.g. http://10.10.0.4:8099 (env AI_MANAGER_URL). */
  manager_url?: string;
  /** ai-manager HTTP Basic credentials (env AI_MANAGER_LOGIN_ID / AI_MANAGER_SECRET_KEY). */
  manager_login_id?: string;
  manager_secret_key?: string;
  /** Upstream provider inside ai-manager. This repo pins "claude-v3" (Claude CLI on rnd-v3). */
  manager_provider?: string;
  /** Forbid ai-manager cross-provider fallback (env AI_MANAGER_NO_FALLBACK, default true). */
  manager_no_fallback?: boolean;
  /** Optional upstream model inside ai-manager (env AI_MANAGER_MODEL). */
  manager_model?: string;
  /** Per-call timeout in seconds (env AI_TIMEOUT_SECONDS, default 180). */
  timeout_seconds?: number;
  /** MiniMax direct settings (only used when provider = "minimax"). */
  api_key: string;
  model: string;
  embedding_model: string;
  base_url?: string;
}

export interface SecurityConfig {
  api_secret_key: string;
  cors_enabled: boolean;
  cors_origins: string;
}

export interface BrowserConfig {
  type: 'chromium' | 'firefox' | 'webkit';
  headless: boolean;
  slow_mo: number;
  viewport: {
    width: number;
    height: number;
  };
  args?: string[];
}

export interface OutputConfig {
  video_dir: string;
  docs_dir: string;
  screenshots_dir: string;
  record_video: boolean;
  video_format: string;
}

export interface FlowsConfig {
  roles: string[];
  max_steps: number;
  continue_on_error: boolean;
}

export interface PlaywrightDocsConfig {
  browser: BrowserConfig;
  output: OutputConfig;
  flows: FlowsConfig;
}

export interface ExplorationConfig {
  mode: 'random' | 'guided' | 'planning';
  max_steps: number;
  max_iterations: number;
  exploration_ratio: number;
  plan_depth: number;
}

export interface LearningConfig {
  enabled: boolean;
  endpoint: string;
  search_endpoint: string;
  success_increment: number;
  failure_decrement: number;
  decay_factor: number;
  min_usage_count: number;
}

export interface GoalConfig {
  name: string;
  signals: string[];
  priority: number;
}

export interface GoalsConfig {
  known: GoalConfig[];
  confidence_threshold: number;
}

export interface SkillsConfig {
  enabled: boolean;
  run_on_navigate: boolean;
  builtin: string[];
  severity_threshold: string;
}

export interface PlaywrightQAConfig {
  browser: BrowserConfig;
  exploration: ExplorationConfig;
  learning: LearningConfig;
  goals: GoalsConfig;
  skills: SkillsConfig;
}

export interface LoggerConfig {
  api_url: string;
  batch_size: number;
  flush_interval: number;
  retry: {
    count: number;
    delay: number;
  };
}

export interface DashboardConfig {
  dev_server: {
    host: string;
    port: number;
  };
  proxy: {
    enabled: boolean;
    target: string;
  };
  ui: {
    theme: string;
    primary_color: string;
    debug: boolean;
  };
}

export interface CostOptimizationConfig {
  caching: {
    enabled: boolean;
    ai_cache: boolean;
    embedding_cache: boolean;
    similarity_cache: boolean;
  };
  expected_savings: {
    hash_cache: number;
    input_normalization: number;
    embedding_reuse: number;
    similarity_reuse: number;
    total: number;
  };
}

export interface MonitoringConfig {
  log_level: 'debug' | 'info' | 'warn' | 'error';
  structured_logging: boolean;
  metrics_enabled: boolean;
}

export interface EnvOverrides {
  staging?: Partial<StagingConfig>;
  backend?: Partial<BackendConfig>;
  cost_optimization?: Partial<CostOptimizationConfig>;
}

// Main config interface
export interface Config {
  staging: StagingConfig;
  credentials: CredentialsConfig;
  backend: BackendConfig;
  playwright_docs: PlaywrightDocsConfig;
  playwright_qa: PlaywrightQAConfig;
  logger: LoggerConfig;
  dashboard: DashboardConfig;
  cost_optimization: CostOptimizationConfig;
  monitoring: MonitoringConfig;
  development?: EnvOverrides;
  production?: EnvOverrides;
}

// ============================================================
// CONFIG LOADER
// ============================================================

let cachedConfig: Config | null = null;

/**
 * Get config file path
 */
function getConfigPath(): string {
  // Try multiple locations
  const paths = [
    path.join(process.cwd(), 'config.yaml'),
    path.join(process.cwd(), '..', 'config.yaml'),
    path.join(__dirname, '..', '..', 'config.yaml'),
    path.join(process.env.HOME || '', '.config', 'qa-automation', 'config.yaml'),
  ];
  
  for (const p of paths) {
    if (fs.existsSync(p)) {
      return p;
    }
  }
  
  // Default to current directory
  return paths[0];
}

/**
 * Load configuration from YAML file
 */
export function loadConfig(): Config {
  if (cachedConfig) {
    return cachedConfig;
  }
  
  const configPath = getConfigPath();
  
  if (!fs.existsSync(configPath)) {
    throw new Error(`Config file not found: ${configPath}`);
  }
  
  const content = fs.readFileSync(configPath, 'utf8');
  const config = yaml.parse(content) as Config;
  
  // Apply environment-specific overrides
  const env = process.env.NODE_ENV || 'development';
  if (env === 'development' && config.development) {
    Object.assign(config, config.development);
  } else if (env === 'production' && config.production) {
    Object.assign(config, config.production);
  }
  
  cachedConfig = config;
  return config;
}

/**
 * Get staging configuration
 */
export function getStaging(): StagingConfig {
  return loadConfig().staging;
}

/**
 * Get credentials for a specific role
 */
export function getCredentials(role: string): Credential | undefined {
  return loadConfig().credentials[role];
}

/**
 * Get all credentials
 */
export function getAllCredentials(): CredentialsConfig {
  return loadConfig().credentials;
}

/**
 * Get backend configuration
 */
export function getBackend(): BackendConfig {
  return loadConfig().backend;
}

/**
 * Get database configuration
 */
export function getDatabase(): DatabaseConfig {
  return loadConfig().backend.database;
}

/**
 * Get AI configuration
 */
export function getAI(): AIConfig {
  const ai: AIConfig = { ...(loadConfig().backend?.ai || ({} as AIConfig)) };
  // Environment always wins over config.yaml (secrets should live in env only).
  const env = process.env;
  ai.provider = (env.AI_PROVIDER as AIConfig['provider']) || ai.provider || 'ai-manager';
  ai.manager_url = env.AI_MANAGER_URL || ai.manager_url || 'http://10.10.0.4:8099';
  ai.manager_login_id = env.AI_MANAGER_LOGIN_ID || ai.manager_login_id || '';
  ai.manager_secret_key = env.AI_MANAGER_SECRET_KEY || ai.manager_secret_key || '';
  ai.manager_provider = env.AI_MANAGER_PROVIDER || ai.manager_provider || 'claude-v3';
  ai.manager_no_fallback = env.AI_MANAGER_NO_FALLBACK ? env.AI_MANAGER_NO_FALLBACK !== 'false' : ai.manager_no_fallback ?? true;
  ai.manager_model = env.AI_MANAGER_MODEL || ai.manager_model || '';
  ai.timeout_seconds = Number(env.AI_TIMEOUT_SECONDS) || ai.timeout_seconds || 180;
  ai.api_key = env.MINIMAX_API_KEY || ai.api_key || '';
  return ai;
}

/**
 * Get playwright-docs configuration
 */
export function getPlaywrightDocs(): PlaywrightDocsConfig {
  return loadConfig().playwright_docs;
}

/**
 * Get playwright-qa configuration
 */
export function getPlaywrightQA(): PlaywrightQAConfig {
  return loadConfig().playwright_qa;
}

/**
 * Get logger configuration
 */
export function getLogger(): LoggerConfig {
  return loadConfig().logger;
}

/**
 * Get dashboard configuration
 */
export function getDashboard(): DashboardConfig {
  return loadConfig().dashboard;
}

/**
 * Get cost optimization configuration
 */
export function getCostOptimization(): CostOptimizationConfig {
  return loadConfig().cost_optimization;
}

/**
 * Get monitoring configuration
 */
export function getMonitoring(): MonitoringConfig {
  return loadConfig().monitoring;
}

/**
 * Get the MySQL connection URL (`mysql://user:pass@host:port/name`).
 */
export function getDatabaseURL(): string {
  const db = getDatabase();
  if (db.connection_string) {
    return db.connection_string;
  }
  const user = encodeURIComponent(db.username);
  const pass = encodeURIComponent(db.password);
  return `mysql://${user}:${pass}@${db.host}:${db.port}/${db.name}`;
}

/**
 * Get backend URL with API path
 */
export function getBackendURL(path: string = ''): string {
  const backend = getBackend();
  return `http://${backend.host}:${backend.port}${path}`;
}

/**
 * Get API key for AI service
 */
export function getAPIKey(): string {
  return getAI().api_key;
}

/**
 * Clear cached configuration (for testing)
 */
export function clearCache(): void {
  cachedConfig = null;
}

/**
 * Validate configuration
 */
export function validateConfig(): { valid: boolean; errors: string[] } {
  const config = loadConfig();
  const errors: string[] = [];
  
  // Check required fields
  if (!config.staging?.url) {
    errors.push('staging.url is required');
  }
  
  if (!config.credentials || Object.keys(config.credentials).length === 0) {
    errors.push('At least one credential is required');
  }
  
  if (!config.backend?.database?.host) {
    errors.push('backend.database.host is required');
  }
  
  const ai = getAI();
  if (ai.provider === 'minimax') {
    if (!ai.api_key) errors.push('backend.ai.api_key (MINIMAX_API_KEY) is required when provider = minimax');
  } else if (!ai.manager_login_id || !ai.manager_secret_key) {
    errors.push('AI_MANAGER_LOGIN_ID / AI_MANAGER_SECRET_KEY are required when provider = ai-manager');
  }
  
  return {
    valid: errors.length === 0,
    errors,
  };
}

// ============================================================
// DEFAULT EXPORTS
// ============================================================

export default {
  loadConfig,
  getStaging,
  getCredentials,
  getAllCredentials,
  getBackend,
  getDatabase,
  getAI,
  getPlaywrightDocs,
  getPlaywrightQA,
  getLogger,
  getDashboard,
  getCostOptimization,
  getMonitoring,
  getDatabaseURL,
  getBackendURL,
  getAPIKey,
  clearCache,
  validateConfig,
};