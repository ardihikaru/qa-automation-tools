package config

import (
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Staging          StagingConfig          `yaml:"staging"`
	Credentials      CredentialsConfig      `yaml:"credentials"`
	Backend          BackendConfig          `yaml:"backend"`
	PlaywrightDocs   PlaywrightDocsConfig   `yaml:"playwright_docs"`
	PlaywrightQA     PlaywrightQAConfig     `yaml:"playwright_qa"`
	Logger           LoggerConfig           `yaml:"logger"`
	Dashboard        DashboardConfig        `yaml:"dashboard"`
	CostOptimization CostOptimizationConfig `yaml:"cost_optimization"`
	Monitoring       MonitoringConfig       `yaml:"monitoring"`
	Development      *EnvOverrides          `yaml:"development,omitempty"`
	Production       *EnvOverrides          `yaml:"production,omitempty"`
}

// StagingConfig holds staging environment settings
type StagingConfig struct {
	URL               string `yaml:"url"`
	GraphAPIURL       string `yaml:"graph_api_url"`
	RestAPIURL        string `yaml:"rest_api_url"`
	Timeout           int    `yaml:"timeout"`
	NavigationTimeout int    `yaml:"navigation_timeout"`
	ActionTimeout     int    `yaml:"action_timeout"`
}

// Credential holds role credentials
type Credential struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	RoleCode string `yaml:"role_code"`
}

// CredentialsConfig holds all credentials
type CredentialsConfig map[string]Credential

// BackendConfig holds backend settings
type BackendConfig struct {
	Host     string         `yaml:"host"`
	Port     int            `yaml:"port"`
	Database DatabaseConfig `yaml:"database"`
	AI       AIConfig       `yaml:"ai"`
	Security SecurityConfig `yaml:"security"`
	Executor ExecutorConfig `yaml:"-"`
	Products ProductsConfig `yaml:"-"`
	GAOS     GAOSConfig     `yaml:"-"`
	// ObjectStore is the MinIO cache of the module checklist export (MODULE_CHECKLIST_EXPORT.md).
	ObjectStore ObjectStoreConfig `yaml:"-"`
}

// ObjectStoreConfig points at the S3-compatible object store (the staging MinIO that SUT uses)
// where generated export files are cached. Env only. Unset or invalid means "not configured": the
// export answers 503 and nothing else is affected.
type ObjectStoreConfig struct {
	Endpoint       string // MINIO_ENDPOINT, host:port without a scheme
	AccessKey      string // MINIO_ACCESS_KEY (secret)
	SecretKey      string // MINIO_SECRET_KEY (secret)
	Bucket         string // MINIO_BUCKET, the permanent bucket (not the temp bucket)
	KeyNamespace   string // MINIO_KEY_NAMESPACE, mandatory key segment: ^[a-z0-9][a-z0-9-]{0,31}$
	UseSSL         bool   // MINIO_USE_SSL
	TimeoutSeconds int    // MINIO_TIMEOUT_SECONDS, per call (default 30)
}

var objectStoreNamespaceRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// A plaintext (non-TLS) object store is accepted only on a private (RFC 1918) address,
// e.g. a WireGuard peer.

// String and GoString use a value receiver so %v, %+v and %#v of a value or a pointer never print a secret.
func (c ObjectStoreConfig) String() string {
	return fmt.Sprintf("ObjectStoreConfig{Endpoint:%s Bucket:%s KeyNamespace:%s UseSSL:%t TimeoutSeconds:%d AccessKey:*** SecretKey:***}",
		c.Endpoint, c.Bucket, c.KeyNamespace, c.UseSSL, c.TimeoutSeconds)
}

// GoString implements fmt.GoStringer (see String).
func (c ObjectStoreConfig) GoString() string { return c.String() }

// Timeout is the per-call timeout (30 s when unset or not positive).
func (c ObjectStoreConfig) Timeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// Configured reports whether every required value is set (validity is Check's job).
func (c ObjectStoreConfig) Configured() bool {
	return c.Endpoint != "" && c.AccessKey != "" && c.SecretKey != "" && c.Bucket != "" && c.KeyNamespace != ""
}

// Check returns nil when the config is usable. The error names the setting, never a value.
func (c ObjectStoreConfig) Check() error {
	if !c.Configured() {
		return fmt.Errorf("MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY, MINIO_BUCKET and MINIO_KEY_NAMESPACE must all be set")
	}
	if !objectStoreNamespaceRe.MatchString(c.KeyNamespace) {
		return fmt.Errorf("MINIO_KEY_NAMESPACE must match ^[a-z0-9][a-z0-9-]{0,31}$")
	}
	if strings.Contains(c.Endpoint, "://") || strings.ContainsAny(c.Endpoint, "/ ") {
		return fmt.Errorf("MINIO_ENDPOINT must be host:port without a scheme or path")
	}
	if !c.UseSSL {
		host := c.Endpoint
		if h, _, err := net.SplitHostPort(c.Endpoint); err == nil {
			host = h
		}
		if ip := net.ParseIP(host); ip == nil || !ip.IsPrivate() {
			return fmt.Errorf("MINIO_USE_SSL=false is accepted only for an endpoint on a private (RFC 1918) IP address")
		}
	}
	return nil
}

// GAOSConfig points at the GAOS REST API (GAOS_INTEGRATION.md). Env only; unset = not configured.
type GAOSConfig struct {
	APIURL   string // GAOS_API_URL, e.g. https://gaos.mfardiansyah.id/api/v1
	APIToken string // GAOS_API_TOKEN (QA bearer, gaos_...); never logged
	// WebURL is the public GAOS web app the /products badge links to (GAOS_WEB_URL, e.g.
	// https://gaos.mfardiansyah.id/app/). Optional; empty derives it from an https APIURL.
	WebURL string
	// InboundToken is the bearer GAOS presents to /api/v1/gaos/* (GAOS_INBOUND_TOKEN); never logged.
	InboundToken string
	// ResultsToken is the bearer CI presents to POST /api/v1/results/playwright (RESULTS_INGEST_TOKEN); never logged.
	ResultsToken string
}

// ExecutorConfig points at playwright-executor (systemd service on app-host).
// Env only. An empty URL makes case_run jobs fail with "executor not configured".
type ExecutorConfig struct {
	URL            string // PLAYWRIGHT_EXECUTOR_URL, e.g. http://10.8.0.9:8141
	Token          string // PLAYWRIGHT_EXECUTOR_TOKEN (shared bearer secret)
	TimeoutMinutes int    // PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES: max wait per job, incl. queueing
	// EnvPrefix is EXECUTOR_ENV_PREFIX: instance prefix of the executor env file name. Empty gives
	// automation-<productId>.env; `personal` gives automation-personal-<productId>.env. Needed when
	// several instances share one executor (product ids collide across their databases).
	EnvPrefix string
}

var executorEnvPrefixRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,20}$`)

// Check returns nil when EnvPrefix is empty or matches ^[a-z][a-z0-9-]{0,20}$.
func (c ExecutorConfig) Check() error {
	if c.EnvPrefix != "" && !executorEnvPrefixRe.MatchString(c.EnvPrefix) {
		return fmt.Errorf("EXECUTOR_ENV_PREFIX must be empty or match ^[a-z][a-z0-9-]{0,20}$")
	}
	return nil
}

// ProductsConfig configures the products repo pool. Env only.
// GH_TOKEN (read by gh/git directly, never logged) authenticates clones.
type ProductsConfig struct {
	PoolDir string // REPO_POOL_DIR: pool root inside the process (empty disables clone/verify)
	// PoolHostDir is REPO_POOL_HOST_DIR: the same pool as playwright-executor sees it on app-host
	// (PoolDir is a container path). case_run / case_create send repo_dir under it (CASE_AUTOMATION.md).
	PoolHostDir           string
	VerifyIntervalMinutes int // PRODUCTS_VERIFY_INTERVAL_MINUTES: scheduled verify, 0 disables
	// KnowledgeSyncIntervalMinutes is KB_SYNC_INTERVAL_MINUTES: scheduled knowledge sync, 0 disables (PRODUCT_KNOWLEDGE_SYNC.md).
	KnowledgeSyncIntervalMinutes int
	PoolOwner                    string // REPO_POOL_OWNER: uid:gid given to everything the pool creates (the executor's host user)
	AllowedOwners                string // REPO_POOL_ALLOWED_OWNERS: comma-separated GitHub owners allowed to clone (empty = any)
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	Username         string `yaml:"username"`
	Password         string `yaml:"password"`
	Name             string `yaml:"name"`
	ConnectionString string `yaml:"connection_string,omitempty"`
}

// AIConfig holds AI settings
type AIConfig struct {
	// Provider selects the LLM gateway: "ai-manager" (default) or "minimax".
	Provider       string `yaml:"provider,omitempty"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty"`

	// ai-manager (production gateway, reached over WireGuard).
	ManagerURL       string `yaml:"manager_url,omitempty"`
	ManagerLoginID   string `yaml:"-"`
	ManagerSecretKey string `yaml:"-"`
	ManagerProvider  string `yaml:"manager_provider,omitempty"`
	ManagerModel     string `yaml:"manager_model,omitempty"`
	// ManagerNoFallback pins every call to ManagerProvider (no cross-provider fallback).
	ManagerNoFallback bool `yaml:"-"`

	// MiniMax direct (fallback when Provider == "minimax").
	APIKey       string `yaml:"api_key"`
	Model        string `yaml:"model"`
	BaseURL      string `yaml:"base_url,omitempty"`
	ChatEndpoint string `yaml:"chat_endpoint,omitempty"`
}

// SecurityConfig holds security settings
type SecurityConfig struct {
	APISecretKey string `yaml:"api_secret_key"`
	CORSEnabled  bool   `yaml:"cors_enabled"`
	CORSOrigins  string `yaml:"cors_origins"`
}

// PlaywrightDocsConfig holds playwright-docs settings
type PlaywrightDocsConfig struct {
	Browser BrowserConfig `yaml:"browser"`
	Output  OutputConfig  `yaml:"output"`
	Flows   FlowsConfig   `yaml:"flows"`
}

// BrowserConfig holds browser settings
type BrowserConfig struct {
	Type     string `yaml:"type"`
	Headless bool   `yaml:"headless"`
	SlowMo   int    `yaml:"slow_mo"`
	Viewport struct {
		Width  int `yaml:"width"`
		Height int `yaml:"height"`
	} `yaml:"viewport"`
	Args []string `yaml:"args,omitempty"`
}

// OutputConfig holds output settings
type OutputConfig struct {
	VideoDir       string `yaml:"video_dir"`
	DocsDir        string `yaml:"docs_dir"`
	ScreenshotsDir string `yaml:"screenshots_dir"`
	RecordVideo    bool   `yaml:"record_video"`
	VideoFormat    string `yaml:"video_format"`
}

// FlowsConfig holds flow settings
type FlowsConfig struct {
	Roles           []string `yaml:"roles"`
	MaxSteps        int      `yaml:"max_steps"`
	ContinueOnError bool     `yaml:"continue_on_error"`
}

// PlaywrightQAConfig holds playwright-qa settings
type PlaywrightQAConfig struct {
	Browser     BrowserConfig     `yaml:"browser"`
	Exploration ExplorationConfig `yaml:"exploration"`
	Learning    LearningConfig    `yaml:"learning"`
	Goals       GoalsConfig       `yaml:"goals"`
	Skills      SkillsConfig      `yaml:"skills"`
}

// ExplorationConfig holds exploration settings
type ExplorationConfig struct {
	Mode             string  `yaml:"mode"`
	MaxSteps         int     `yaml:"max_steps"`
	MaxIterations    int     `yaml:"max_iterations"`
	ExplorationRatio float64 `yaml:"exploration_ratio"`
	PlanDepth        int     `yaml:"plan_depth"`
}

// LearningConfig holds learning settings
type LearningConfig struct {
	Enabled          bool    `yaml:"enabled"`
	Endpoint         string  `yaml:"endpoint"`
	SearchEndpoint   string  `yaml:"search_endpoint"`
	SuccessIncrement float64 `yaml:"success_increment"`
	FailureDecrement float64 `yaml:"failure_decrement"`
	DecayFactor      float64 `yaml:"decay_factor"`
	MinUsageCount    int     `yaml:"min_usage_count"`
}

// GoalConfig holds goal definitions
type GoalConfig struct {
	Name     string   `yaml:"name"`
	Signals  []string `yaml:"signals"`
	Priority int      `yaml:"priority"`
}

// GoalsConfig holds goals settings
type GoalsConfig struct {
	Known               []GoalConfig `yaml:"known"`
	ConfidenceThreshold float64      `yaml:"confidence_threshold"`
}

// SkillsConfig holds skills settings
type SkillsConfig struct {
	Enabled           bool     `yaml:"enabled"`
	RunOnNavigate     bool     `yaml:"run_on_navigate"`
	Builtin           []string `yaml:"builtin"`
	SeverityThreshold string   `yaml:"severity_threshold"`
}

// LoggerConfig holds logger settings
type LoggerConfig struct {
	APIURL        string      `yaml:"api_url"`
	BatchSize     int         `yaml:"batch_size"`
	FlushInterval int         `yaml:"flush_interval"`
	Retry         RetryConfig `yaml:"retry"`
}

// RetryConfig holds retry settings
type RetryConfig struct {
	Count int `yaml:"count"`
	Delay int `yaml:"delay"`
}

// DashboardConfig holds dashboard settings
type DashboardConfig struct {
	DevServer DevServerConfig `yaml:"dev_server"`
	Proxy     ProxyConfig     `yaml:"proxy"`
	UI        UIConfig        `yaml:"ui"`
}

// DevServerConfig holds dev server settings
type DevServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// ProxyConfig holds proxy settings
type ProxyConfig struct {
	Enabled bool   `yaml:"enabled"`
	Target  string `yaml:"target"`
}

// UIConfig holds UI settings
type UIConfig struct {
	Theme        string `yaml:"theme"`
	PrimaryColor string `yaml:"primary_color"`
	Debug        bool   `yaml:"debug"`
}

// CostOptimizationConfig holds cost optimization settings
type CostOptimizationConfig struct {
	Caching         CachingConfig         `yaml:"caching"`
	ExpectedSavings ExpectedSavingsConfig `yaml:"expected_savings"`
}

// CachingConfig holds caching settings
type CachingConfig struct {
	Enabled         bool `yaml:"enabled"`
	AICache         bool `yaml:"ai_cache"`
	EmbeddingCache  bool `yaml:"embedding_cache"`
	SimilarityCache bool `yaml:"similarity_cache"`
}

// ExpectedSavingsConfig holds expected savings
type ExpectedSavingsConfig struct {
	HashCache          float64 `yaml:"hash_cache"`
	InputNormalization float64 `yaml:"input_normalization"`
	EmbeddingReuse     float64 `yaml:"embedding_reuse"`
	SimilarityReuse    float64 `yaml:"similarity_reuse"`
	Total              float64 `yaml:"total"`
}

// MonitoringConfig holds monitoring settings
type MonitoringConfig struct {
	LogLevel          string `yaml:"log_level"`
	StructuredLogging bool   `yaml:"structured_logging"`
	MetricsEnabled    bool   `yaml:"metrics_enabled"`
}

// EnvOverrides holds environment overrides
type EnvOverrides struct {
	Staging          *StagingConfig          `yaml:"staging,omitempty"`
	Backend          *BackendConfig          `yaml:"backend,omitempty"`
	CostOptimization *CostOptimizationConfig `yaml:"cost_optimization,omitempty"`
}

var cfg *Config

// LoadConfig loads configuration from .env file and environment variables
func LoadConfig() *Config {
	if cfg != nil {
		return cfg
	}

	// Load .env file if exists
	_ = godotenv.Load()

	// Start with defaults that can be overridden by YAML config
	cfg = &Config{
		Backend: BackendConfig{
			Host: getEnvString("BACKEND_HOST", "0.0.0.0"),
			Port: getEnvInt("PORT", 8080),
			Database: DatabaseConfig{
				Host:     getEnvString("DB_HOST", "127.0.0.1"),
				Port:     getEnvInt("DB_PORT", 5306),
				Username: getEnvString("DB_USER", "qa_automation"),
				Password: getEnvString("DB_PASSWORD", ""),
				Name:     getEnvString("DB_NAME", "qa_automation"),
			},
			AI: AIConfig{
				Provider:          getEnvString("AI_PROVIDER", "ai-manager"),
				TimeoutSeconds:    getEnvInt("AI_TIMEOUT_SECONDS", 180),
				ManagerURL:        getEnvString("AI_MANAGER_URL", "http://127.0.0.1:8099"),
				ManagerLoginID:    getEnvString("AI_MANAGER_LOGIN_ID", ""),
				ManagerSecretKey:  getEnvString("AI_MANAGER_SECRET_KEY", ""),
				ManagerProvider:   getEnvString("AI_MANAGER_PROVIDER", "claude-v3"),
				ManagerModel:      getEnvString("AI_MANAGER_MODEL", ""),
				ManagerNoFallback: getEnvBool("AI_MANAGER_NO_FALLBACK", true),
				APIKey:            getEnvString("MINIMAX_API_KEY", ""),
				Model:             getEnvString("AI_MODEL", "MiniMax-M2.5"),
				BaseURL:           getEnvString("AI_BASE_URL", "https://api.minimax.io/v1"),
				ChatEndpoint:      getEnvString("AI_CHAT_ENDPOINT", "/chat/completions"),
			},
			Security: SecurityConfig{
				APISecretKey: getEnvString("API_SECRET_KEY", ""),
				CORSEnabled:  getEnvBool("CORS_ENABLED", true),
				CORSOrigins:  getEnvString("CORS_ORIGINS", "*"),
			},
			Products: ProductsConfig{
				PoolDir:                      getEnvString("REPO_POOL_DIR", ""),
				PoolHostDir:                  getEnvString("REPO_POOL_HOST_DIR", ""),
				VerifyIntervalMinutes:        getEnvInt("PRODUCTS_VERIFY_INTERVAL_MINUTES", 360),
				KnowledgeSyncIntervalMinutes: getEnvInt("KB_SYNC_INTERVAL_MINUTES", 360),
				PoolOwner:                    getEnvString("REPO_POOL_OWNER", ""),
				AllowedOwners:                getEnvString("REPO_POOL_ALLOWED_OWNERS", ""),
			},
			GAOS: GAOSConfig{
				APIURL:       getEnvString("GAOS_API_URL", ""),
				APIToken:     getEnvString("GAOS_API_TOKEN", ""),
				WebURL:       getEnvString("GAOS_WEB_URL", ""),
				InboundToken: getEnvString("GAOS_INBOUND_TOKEN", ""),
				ResultsToken: getEnvString("RESULTS_INGEST_TOKEN", ""),
			},
			ObjectStore: ObjectStoreConfig{
				Endpoint:       getEnvString("MINIO_ENDPOINT", ""),
				AccessKey:      getEnvString("MINIO_ACCESS_KEY", ""),
				SecretKey:      getEnvString("MINIO_SECRET_KEY", ""),
				Bucket:         getEnvString("MINIO_BUCKET", ""),
				KeyNamespace:   getEnvString("MINIO_KEY_NAMESPACE", ""),
				UseSSL:         getEnvBool("MINIO_USE_SSL", true),
				TimeoutSeconds: getEnvInt("MINIO_TIMEOUT_SECONDS", 30),
			},
			Executor: ExecutorConfig{
				URL:            getEnvString("PLAYWRIGHT_EXECUTOR_URL", ""),
				Token:          getEnvString("PLAYWRIGHT_EXECUTOR_TOKEN", ""),
				TimeoutMinutes: getEnvInt("PLAYWRIGHT_EXECUTOR_TIMEOUT_MINUTES", 30),
				EnvPrefix:      strings.TrimSpace(getEnvString("EXECUTOR_ENV_PREFIX", "")),
			},
		},
		Staging: StagingConfig{
			URL:               getEnvString("STAGING_URL", ""),
			GraphAPIURL:       getEnvString("GRAPH_API_URL", ""),
			RestAPIURL:        getEnvString("REST_API_URL", ""),
			Timeout:           getEnvInt("STAGING_TIMEOUT", 30000),
			NavigationTimeout: getEnvInt("NAVIGATION_TIMEOUT", 30000),
			ActionTimeout:     getEnvInt("ACTION_TIMEOUT", 5000),
		},
	}

	// Try to load from config.yaml for any additional overrides
	// This allows users to keep yaml config for non-sensitive settings
	loadYAMLConfig(cfg)

	// Override with DATABASE_URL if provided (for convenience)
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		cfg.Backend.Database.ConnectionString = databaseURL
	}

	log.Println("📦 QA Automation Backend - Configuration loaded from .env")
	log.Printf("   Server Port: %d", cfg.Backend.Port)
	log.Printf("   Database: %s", cfg.Backend.Database.Host)
	switch cfg.Backend.AI.Provider {
	case "minimax":
		if cfg.Backend.AI.APIKey != "" {
			log.Printf("   AI: minimax direct (Model: %s)", cfg.Backend.AI.Model)
		} else {
			log.Printf("   AI: minimax selected but MINIMAX_API_KEY is empty - AI disabled")
		}
	default:
		if cfg.Backend.AI.ManagerLoginID != "" && cfg.Backend.AI.ManagerSecretKey != "" {
			log.Printf("   AI: ai-manager at %s (upstream provider: %q)", cfg.Backend.AI.ManagerURL, cfg.Backend.AI.ManagerProvider)
		} else {
			log.Printf("   AI: ai-manager selected but AI_MANAGER_LOGIN_ID/AI_MANAGER_SECRET_KEY empty - AI disabled")
		}
	}

	if cfg.Backend.Executor.URL != "" {
		log.Printf("   Playwright: executor at %s", cfg.Backend.Executor.URL)
	} else {
		log.Printf("   Playwright: PLAYWRIGHT_EXECUTOR_URL empty - case_run jobs will fail")
	}

	// Apply environment overrides
	env := os.Getenv("NODE_ENV")
	if env == "development" && cfg.Development != nil {
		applyOverrides(cfg, cfg.Development)
	} else if env == "production" && cfg.Production != nil {
		applyOverrides(cfg, cfg.Production)
	}

	return cfg
}

// loadYAMLConfig loads additional config from yaml file
func loadYAMLConfig(cfg *Config) {
	paths := []string{
		"config.yaml",
		"../config.yaml",
		"./config.yaml",
	}

	var data []byte
	var err error

	for _, p := range paths {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}

	if err != nil {
		// No yaml config found, use env-only config
		return
	}

	// Parse YAML into a temp struct
	temp := &struct {
		Staging          *StagingConfig          `yaml:"staging"`
		Backend          *BackendConfig          `yaml:"backend"`
		Monitoring       *MonitoringConfig       `yaml:"monitoring"`
		CostOptimization *CostOptimizationConfig `yaml:"cost_optimization"`
	}{}

	if err := yaml.Unmarshal(data, temp); err != nil {
		log.Printf("Warning: Failed to parse config.yaml: %v", err)
		return
	}

	// Override env config with yaml (except secrets which come from env)
	if temp.Staging != nil {
		if temp.Staging.URL != "" && os.Getenv("STAGING_URL") == "" {
			cfg.Staging.URL = temp.Staging.URL
		}
		if temp.Staging.GraphAPIURL != "" && os.Getenv("GRAPH_API_URL") == "" {
			cfg.Staging.GraphAPIURL = temp.Staging.GraphAPIURL
		}
		if temp.Staging.RestAPIURL != "" && os.Getenv("REST_API_URL") == "" {
			cfg.Staging.RestAPIURL = temp.Staging.RestAPIURL
		}
	}

	if temp.Monitoring != nil {
		cfg.Monitoring = *temp.Monitoring
	}

	if temp.CostOptimization != nil {
		cfg.CostOptimization = *temp.CostOptimization
	}

	// Backend settings (non-sensitive only)
	if temp.Backend != nil {
		if temp.Backend.Host != "" && os.Getenv("BACKEND_HOST") == "" {
			cfg.Backend.Host = temp.Backend.Host
		}
		if temp.Backend.Port != 0 && os.Getenv("PORT") == "" {
			cfg.Backend.Port = temp.Backend.Port
		}
		if temp.Backend.AI.Model != "" && os.Getenv("AI_MODEL") == "" {
			cfg.Backend.AI.Model = temp.Backend.AI.Model
		}
		if temp.Backend.AI.Provider != "" && os.Getenv("AI_PROVIDER") == "" {
			cfg.Backend.AI.Provider = temp.Backend.AI.Provider
		}
		if temp.Backend.AI.ManagerURL != "" && os.Getenv("AI_MANAGER_URL") == "" {
			cfg.Backend.AI.ManagerURL = temp.Backend.AI.ManagerURL
		}
		if temp.Backend.AI.ManagerProvider != "" && os.Getenv("AI_MANAGER_PROVIDER") == "" {
			cfg.Backend.AI.ManagerProvider = temp.Backend.AI.ManagerProvider
		}
		if temp.Backend.AI.ChatEndpoint != "" && os.Getenv("AI_CHAT_ENDPOINT") == "" {
			cfg.Backend.AI.ChatEndpoint = temp.Backend.AI.ChatEndpoint
		}
		if temp.Backend.Security.CORSEnabled {
			cfg.Backend.Security.CORSEnabled = true
		}
		if temp.Backend.Security.CORSOrigins != "" && os.Getenv("CORS_ORIGINS") == "" {
			cfg.Backend.Security.CORSOrigins = temp.Backend.Security.CORSOrigins
		}
	}
}

// applyOverrides applies environment-specific overrides
func applyOverrides(cfg *Config, overrides *EnvOverrides) {
	if overrides.Staging != nil {
		if overrides.Staging.URL != "" {
			cfg.Staging.URL = overrides.Staging.URL
		}
		if overrides.Staging.GraphAPIURL != "" {
			cfg.Staging.GraphAPIURL = overrides.Staging.GraphAPIURL
		}
		if overrides.Staging.RestAPIURL != "" {
			cfg.Staging.RestAPIURL = overrides.Staging.RestAPIURL
		}
	}
	if overrides.Backend != nil {
		if overrides.Backend.Host != "" {
			cfg.Backend.Host = overrides.Backend.Host
		}
		if overrides.Backend.Port != 0 {
			cfg.Backend.Port = overrides.Backend.Port
		}
	}
	if overrides.CostOptimization != nil {
		cfg.CostOptimization = *overrides.CostOptimization
	}
}

// GetDatabaseURL returns the MySQL DSN (go-sql-driver/mysql format).
//
// Priority:
//  1. Backend.Database.ConnectionString (yaml `connection_string`) — used verbatim.
//  2. DATABASE_URL env var — used verbatim (must be a go-sql-driver/mysql DSN).
//  3. Constructed from DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME.
//
// db.NewConnection pins every DSN (including verbatim ones) to UTC via
// db.NormalizeDSN. The constructed DSN format is:
//
//	user:pass@tcp(host:port)/dbname?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_general_ci&multiStatements=true
func (c *Config) GetDatabaseURL() string {
	if c.Backend.Database.ConnectionString != "" {
		return c.Backend.Database.ConnectionString
	}

	if envURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); envURL != "" {
		return envURL
	}

	host := c.Backend.Database.Host
	port := c.Backend.Database.Port
	user := c.Backend.Database.Username
	pass := c.Backend.Database.Password
	dbname := c.Backend.Database.Name

	if host == "" || user == "" || dbname == "" {
		return ""
	}

	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC&charset=utf8mb4&collation=utf8mb4_general_ci&multiStatements=true",
		user, pass, host, port, dbname,
	)
}

// GetBackendAddress returns the backend server address
func (c *Config) GetBackendAddress() string {
	return fmt.Sprintf("%s:%d", c.Backend.Host, c.Backend.Port)
}

// ClearCache clears the cached config (for testing)
func ClearCache() {
	cfg = nil
}

// Helper functions for reading environment variables

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		lower := strings.ToLower(value)
		return lower == "true" || lower == "1" || lower == "yes"
	}
	return defaultValue
}
