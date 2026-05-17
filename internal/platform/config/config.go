// Package config loads the runtime configuration for ObservAI API.
//
// Configuration may come from a YAML file referenced by OBSERVAI_CONFIG_FILE
// or directly from environment variables. Environment variables always win
// over YAML values when both are set.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/crypto"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

// ErrEncryptionKeyMissing indicates the operator did not provide an
// encryption key for a non-local mode. The configuration loader catches this
// case via Validate; LoadEncryptionKey returns it as a sentinel so callers
// running in local mode can react explicitly (e.g. by generating a volatile
// key for the session).
var ErrEncryptionKeyMissing = errors.New("encryption key not configured")

// Mode identifies the runtime profile the API is running under.
type Mode string

const (
	// ModeLocal allows in-memory fallbacks for fast iteration.
	ModeLocal Mode = "local"
	// ModeDev runs against real dependencies but tolerates missing optional providers.
	ModeDev Mode = "dev"
	// ModeProd requires every provider to be explicitly configured.
	ModeProd Mode = "prod"
)

var supportedModes = map[Mode]struct{}{
	ModeLocal: {},
	ModeDev:   {},
	ModeProd:  {},
}

type requiredConfigValue struct {
	envName string
	missing func(Config) bool
}

var prodRequiredConfigValues = []requiredConfigValue{
	{envName: "OBSERVAI_DATABASE_DSN", missing: func(cfg Config) bool { return strings.TrimSpace(cfg.DatabaseDSN) == "" }},
	{envName: "OBSERVAI_REDIS_URL", missing: func(cfg Config) bool { return strings.TrimSpace(cfg.RedisURL) == "" }},
	{envName: "OBSERVAI_PROMETHEUS_URL", missing: func(cfg Config) bool { return len(cfg.Observability.Providers) == 0 }},
	{envName: "OBSERVAI_OLLAMA_URL", missing: func(cfg Config) bool { return len(cfg.LLM.Providers) == 0 }},
}

var nonLocalRequiredConfigValues = []requiredConfigValue{
	{envName: "OBSERVAI_ENCRYPTION_KEY", missing: func(cfg Config) bool { return strings.TrimSpace(cfg.EncryptionKey) == "" }},
	{envName: "OBSERVAI_JWT_SECRET", missing: func(cfg Config) bool { return strings.TrimSpace(cfg.JWT.Secret) == "" }},
}

// PrometheusConfig holds Prometheus adapter configuration.
//
// Deprecated: define the provider under Observability.Providers instead. The
// fields are kept so existing OBSERVAI_PROMETHEUS_URL / OBSERVAI_PROMETHEUS_TIMEOUT
// environment variables continue to work; Load migrates them into
// Observability.Providers automatically.
type PrometheusConfig struct {
	URL     string        `yaml:"url" env:"OBSERVAI_PROMETHEUS_URL"`
	Timeout time.Duration `yaml:"timeout" env:"OBSERVAI_PROMETHEUS_TIMEOUT" env-default:"10s"`
}

// OllamaConfig holds Ollama adapter configuration.
//
// Deprecated: define the provider under LLM.Providers instead. The fields
// are kept so existing OBSERVAI_OLLAMA_URL / OBSERVAI_OLLAMA_MODEL /
// OBSERVAI_OLLAMA_TIMEOUT environment variables continue to work; Load
// migrates them into LLM.Providers automatically.
type OllamaConfig struct {
	URL     string        `yaml:"url" env:"OBSERVAI_OLLAMA_URL"`
	Model   string        `yaml:"model" env:"OBSERVAI_OLLAMA_MODEL" env-default:"llama3"`
	Timeout time.Duration `yaml:"timeout" env:"OBSERVAI_OLLAMA_TIMEOUT" env-default:"30s"`
}

// ObservabilityProviderConfig describes a single observability provider
// adapter that should be wired at runtime.
//
// Type is the discriminator used by the outbound factory dispatcher
// (e.g. "prometheus", "loki", "jaeger"). Name is the operator-facing
// identifier reported through capabilities and propagated to
// Evidence.Provider when an adapter omits it; defaults to Type.
type ObservabilityProviderConfig struct {
	Type    string            `yaml:"type"`
	Name    string            `yaml:"name"`
	URL     string            `yaml:"url"`
	Timeout time.Duration     `yaml:"timeout"`
	Signals []string          `yaml:"signals"`
	Options map[string]string `yaml:"options"`
}

// ObservabilityConfig groups every observability provider the API should wire.
type ObservabilityConfig struct {
	Providers []ObservabilityProviderConfig `yaml:"providers"`
}

// LLMProviderConfig describes a single LLM adapter that should be wired.
//
// APIKeyEnv names the environment variable that holds the bearer token for
// hosted providers (OpenAI, Anthropic, Azure OpenAI, OpenRouter). It is
// read at adapter construction time so secrets stay out of YAML.
type LLMProviderConfig struct {
	Type      string            `yaml:"type"`
	Name      string            `yaml:"name"`
	URL       string            `yaml:"url"`
	BaseURL   string            `yaml:"base_url"`
	Model     string            `yaml:"model"`
	APIKeyEnv string            `yaml:"api_key_env"`
	Timeout   time.Duration     `yaml:"timeout"`
	Options   map[string]string `yaml:"options"`
}

// LLMConfig groups the LLM providers wired into the API and the active selection.
//
// Active resolves to a provider name or, if no provider with that name
// exists, to a provider type. When empty, the first declared provider is used.
type LLMConfig struct {
	Providers []LLMProviderConfig `yaml:"providers"`
	Active    string              `yaml:"active"`
}

// PromptsConfig holds the path to the public agent prompts directory.
type PromptsConfig struct {
	Dir string `yaml:"dir" env:"OBSERVAI_PROMPTS_DIR" env-default:"agents"`
}

// TracingConfig holds OpenTelemetry tracing configuration.
//
// When Endpoint is empty the API uses a no-op tracer and does not export spans.
// SampleRatio in [0,1] applies a parent-based ratio sampler; 0 disables sampling
// entirely and 1 samples every trace.
type TracingConfig struct {
	Endpoint    string        `yaml:"endpoint" env:"OBSERVAI_OTEL_EXPORTER_OTLP_ENDPOINT"`
	Insecure    bool          `yaml:"insecure" env:"OBSERVAI_OTEL_EXPORTER_OTLP_INSECURE" env-default:"true"`
	ServiceName string        `yaml:"service_name" env:"OBSERVAI_OTEL_SERVICE_NAME" env-default:"observai-api"`
	SampleRatio float64       `yaml:"sample_ratio" env:"OBSERVAI_OTEL_TRACES_SAMPLER_RATIO" env-default:"1.0"`
	Timeout     time.Duration `yaml:"timeout" env:"OBSERVAI_OTEL_EXPORTER_OTLP_TIMEOUT" env-default:"5s"`
}

// HTTPRateLimitConfig configures per-IP HTTP rate limiting.
type HTTPRateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second" env:"OBSERVAI_HTTP_RATE_LIMIT_RPS" env-default:"0"`
	Burst             int     `yaml:"burst" env:"OBSERVAI_HTTP_RATE_LIMIT_BURST" env-default:"0"`
}

// MigrationConfig (inline fields on Config) controls automatic schema migrations.
//
// MigrateOnStart toggles the runner; when true the API blocks startup until
// migrations reach the latest version. MigrationsDir is the filesystem path
// passed to golang-migrate; the Docker image bundles /app/migrations.

// CORSConfig controls cross-origin browser access to the HTTP API.
//
// Origins lists exact allowed origins (e.g. "https://app.example.com").
// EnvOrigins accepts a comma-separated list from OBSERVAI_ALLOWED_ORIGINS and
// is merged with the YAML list at load time. When the resulting list is empty
// the CORS middleware behaves as a no-op, preserving the bundled same-origin
// deployment where the browser talks to the Next.js proxy and never issues a
// cross-origin request to the API.
//
// Wildcards are not supported: the middleware sends
// Access-Control-Allow-Credentials so cookie-based auth works for split
// deployments, and the CORS spec forbids credentials with "*".
type CORSConfig struct {
	Origins    []string `yaml:"origins"`
	EnvOrigins string   `yaml:"-" env:"OBSERVAI_ALLOWED_ORIGINS"`
}

// HTTPAuthConfig configures bearer-token authentication for HTTP requests.
//
// Keys lists accepted API keys. The OBSERVAI_HTTP_AUTH_KEYS environment
// variable accepts a comma-separated list; values from this variable are
// merged with the YAML keys. Skip enumerates URL paths that bypass
// authentication; operational endpoints (/health, /healthz, /readyz,
// /metrics) are skipped by default and need not be listed.
type HTTPAuthConfig struct {
	Keys         []string `yaml:"keys"`
	AdminKeys    []string `yaml:"admin_keys"`
	EnvKeys      string   `yaml:"-" env:"OBSERVAI_HTTP_AUTH_KEYS"`
	EnvAdminKeys string   `yaml:"-" env:"OBSERVAI_HTTP_AUTH_ADMIN_KEYS"`
	Skip         []string `yaml:"skip"`
}

// JWTConfig configures JWT signing and token lifetimes for the user
// authentication flow.
type JWTConfig struct {
	Secret          string        `yaml:"secret" env:"OBSERVAI_JWT_SECRET"`
	Issuer          string        `yaml:"issuer" env:"OBSERVAI_JWT_ISSUER" env-default:"observai-api"`
	AccessTokenTTL  time.Duration `yaml:"access_token_ttl" env:"OBSERVAI_JWT_ACCESS_TTL" env-default:"15m"`
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl" env:"OBSERVAI_JWT_REFRESH_TTL" env-default:"168h"`
}

// CookieConfig controls how authentication cookies are issued.
type CookieConfig struct {
	Domain string `yaml:"domain" env:"OBSERVAI_AUTH_COOKIE_DOMAIN"`
	Secure bool   `yaml:"secure" env:"OBSERVAI_AUTH_COOKIE_SECURE"`
}

// RedactionConfig drives which redaction rules participate in the chain
// applied to evidence before it reaches the LLM.
//
// Rules is a comma-separated list (e.g. "email,bearer,jwt,ipv4").
// An empty value enables every supported rule. Unknown rules are ignored.
type RedactionConfig struct {
	Rules string `yaml:"rules" env:"OBSERVAI_REDACTION_RULES"`
}

// SchedulerConfig configures the Asynq-backed scheduler that runs
// retention purges and webhook retry sweeps on a cron tick.
//
// Enabled defaults to false so installations without Redis or background
// workers keep the existing manual-purge / fire-and-forget webhook
// semantics. RetentionDays caps how old an analysis can become before the
// purge job sweeps it; zero disables age-based purge.
type SchedulerConfig struct {
	Enabled            bool          `yaml:"enabled" env:"OBSERVAI_SCHEDULER_ENABLED" env-default:"false"`
	RetentionDays      int           `yaml:"retention_days" env:"OBSERVAI_RETENTION_DAYS" env-default:"90"`
	RetentionQuantity  int           `yaml:"retention_quantity" env:"OBSERVAI_RETENTION_QUANTITY" env-default:"0"`
	RetentionCron      string        `yaml:"retention_cron" env:"OBSERVAI_RETENTION_CRON" env-default:"0 3 * * *"`
	WebhookRetryCron   string        `yaml:"webhook_retry_cron" env:"OBSERVAI_WEBHOOK_RETRY_CRON" env-default:"*/5 * * * *"`
	WebhookSweepLookup time.Duration `yaml:"webhook_sweep_lookup" env:"OBSERVAI_WEBHOOK_SWEEP_LOOKUP" env-default:"5m"`
	Concurrency        int           `yaml:"concurrency" env:"OBSERVAI_SCHEDULER_CONCURRENCY" env-default:"5"`
}

// QueueConfig configures the asynchronous analysis worker pool.
//
// Concurrency caps how many analyses may be running at once on this instance.
// ChatLockTTL bounds how long a chat critical section may hold the per-analysis
// lock before it is forcibly released; ChatLockWait is the maximum time a chat
// request will wait for the lock before failing.
type QueueConfig struct {
	Concurrency    int           `yaml:"concurrency" env:"OBSERVAI_QUEUE_CONCURRENCY" env-default:"5"`
	DequeueTimeout time.Duration `yaml:"dequeue_timeout" env:"OBSERVAI_QUEUE_DEQUEUE_TIMEOUT" env-default:"5s"`
	ChatLockTTL    time.Duration `yaml:"chat_lock_ttl" env:"OBSERVAI_CHAT_LOCK_TTL" env-default:"60s"`
	ChatLockWait   time.Duration `yaml:"chat_lock_wait" env:"OBSERVAI_CHAT_LOCK_WAIT" env-default:"30s"`
	Backend        string        `yaml:"backend" env:"OBSERVAI_QUEUE_BACKEND" env-default:"legacy"`
}

// Config contains application runtime configuration.
type Config struct {
	Port                    string              `yaml:"port" env:"OBSERVAI_API_PORT" env-default:"8080"`
	Env                     string              `yaml:"env" env:"OBSERVAI_ENV" env-default:"local"`
	Mode                    Mode                `yaml:"mode" env:"OBSERVAI_MODE" env-default:"local"`
	TimeZone                string              `yaml:"timezone" env:"OBSERVAI_TIMEZONE" env-default:"Local"`
	DatabaseDSN             string              `yaml:"database_dsn" env:"OBSERVAI_DATABASE_DSN"`
	RedisURL                string              `yaml:"redis_url" env:"OBSERVAI_REDIS_URL"`
	EncryptionKey           string              `yaml:"encryption_key" env:"OBSERVAI_ENCRYPTION_KEY"`
	AnalysisContextCacheTTL time.Duration       `yaml:"analysis_context_cache_ttl" env:"OBSERVAI_ANALYSIS_CONTEXT_CACHE_TTL" env-default:"6h"`
	HTTPRequestTimeout      time.Duration       `yaml:"http_request_timeout" env:"OBSERVAI_HTTP_REQUEST_TIMEOUT" env-default:"30s"`
	HTTPMaxBodyBytes        int64               `yaml:"http_max_body_bytes" env:"OBSERVAI_HTTP_MAX_BODY_BYTES" env-default:"1048576"`
	HTTPShutdownTimeout     time.Duration       `yaml:"http_shutdown_timeout" env:"OBSERVAI_HTTP_SHUTDOWN_TIMEOUT" env-default:"30s"`
	HTTPRateLimit           HTTPRateLimitConfig `yaml:"http_rate_limit"`
	HTTPAuth                HTTPAuthConfig      `yaml:"http_auth"`
	CORS                    CORSConfig          `yaml:"cors"`
	JWT                     JWTConfig           `yaml:"jwt"`
	Cookies                 CookieConfig        `yaml:"cookies"`
	Scheduler               SchedulerConfig     `yaml:"scheduler"`
	Redaction               RedactionConfig     `yaml:"redaction"`
	MigrateOnStart          bool                `yaml:"migrate_on_start" env:"OBSERVAI_MIGRATE_ON_START" env-default:"false"`
	MigrationsDir           string              `yaml:"migrations_dir" env:"OBSERVAI_MIGRATIONS_DIR" env-default:"migrations"`
	Queue                   QueueConfig         `yaml:"queue"`
	Prometheus              PrometheusConfig    `yaml:"prometheus"`
	Ollama                  OllamaConfig        `yaml:"ollama"`
	Observability           ObservabilityConfig `yaml:"observability"`
	LLM                     LLMConfig           `yaml:"llm"`
	Prompts                 PromptsConfig       `yaml:"prompts"`
	Tracing                 TracingConfig       `yaml:"tracing"`
}

// Load reads application configuration from YAML when configured and environment variables.
func Load() (Config, error) {
	var cfg Config

	if err := loadDotEnv(); err != nil {
		return Config{}, err
	}

	configFile := strings.TrimSpace(os.Getenv("OBSERVAI_CONFIG_FILE"))
	if configFile != "" {
		if err := cleanenv.ReadConfig(configFile, &cfg); err != nil {
			return Config{}, fmt.Errorf("load configuration file %q: %w", configFile, err)
		}
	} else {
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			return Config{}, fmt.Errorf("load configuration: %w", err)
		}
	}

	cfg.Mode = normalizeMode(cfg.Mode)
	migrateLegacyProviderConfig(&cfg)
	mergeAuthKeysFromEnv(&cfg)
	mergeCORSOriginsFromEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadDotEnv() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	return nil
}

// migrateLegacyProviderConfig promotes the deprecated PrometheusConfig and
// OllamaConfig blocks into the new Observability/LLM provider lists when
// the operator has not already declared an equivalent provider.
//
// The legacy blocks remain authoritative for backwards compatibility: the
// migration adds an entry only when no provider of the same type is present
// in the new schema, which lets operators run with either layout during the
// transition.
func migrateLegacyProviderConfig(cfg *Config) {
	prometheusURL := strings.TrimSpace(cfg.Prometheus.URL)
	if prometheusURL != "" && !hasObservabilityProviderType(cfg.Observability.Providers, "prometheus") {
		cfg.Observability.Providers = append(cfg.Observability.Providers, ObservabilityProviderConfig{
			Type:    "prometheus",
			Name:    "prometheus",
			URL:     prometheusURL,
			Timeout: cfg.Prometheus.Timeout,
			Signals: []string{"metrics"},
		})
	}

	ollamaURL := strings.TrimSpace(cfg.Ollama.URL)
	if ollamaURL != "" && !hasLLMProviderType(cfg.LLM.Providers, "ollama") {
		cfg.LLM.Providers = append(cfg.LLM.Providers, LLMProviderConfig{
			Type:    "ollama",
			Name:    "ollama",
			URL:     ollamaURL,
			Model:   cfg.Ollama.Model,
			Timeout: cfg.Ollama.Timeout,
		})
		if strings.TrimSpace(cfg.LLM.Active) == "" {
			cfg.LLM.Active = "ollama"
		}
	}
}

func hasObservabilityProviderType(providers []ObservabilityProviderConfig, providerType string) bool {
	for _, provider := range providers {
		if strings.EqualFold(strings.TrimSpace(provider.Type), providerType) {
			return true
		}
	}
	return false
}

func mergeAuthKeysFromEnv(cfg *Config) {
	cfg.HTTPAuth.Keys = appendCSV(cfg.HTTPAuth.Keys, cfg.HTTPAuth.EnvKeys)
	cfg.HTTPAuth.AdminKeys = appendCSV(cfg.HTTPAuth.AdminKeys, cfg.HTTPAuth.EnvAdminKeys)
}

func mergeCORSOriginsFromEnv(cfg *Config) {
	cfg.CORS.Origins = appendCSV(cfg.CORS.Origins, cfg.CORS.EnvOrigins)
}

func appendCSV(existing []string, csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return existing
	}
	for _, raw := range strings.Split(csv, ",") {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		existing = append(existing, value)
	}
	return existing
}

func hasLLMProviderType(providers []LLMProviderConfig, providerType string) bool {
	for _, provider := range providers {
		if strings.EqualFold(strings.TrimSpace(provider.Type), providerType) {
			return true
		}
	}
	return false
}

// Validate enforces mode-specific configuration requirements.
//
// Every non-local mode must supply an encryption key so credential material
// never leaks to disk in plaintext. Production additionally requires every
// external dependency to be explicitly configured, ensuring the API never
// silently falls back to in-memory or fake adapters.
func (cfg Config) Validate() error {
	if _, err := cfg.TimeLocation(); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.EncryptionKey) != "" {
		if _, err := crypto.LoadKey(cfg.EncryptionKey); err != nil {
			return fmt.Errorf("OBSERVAI_ENCRYPTION_KEY must decode to %d bytes (hex or base64): %w", crypto.KeyLength, err)
		}
	}
	if jwtSecret := strings.TrimSpace(cfg.JWT.Secret); jwtSecret != "" && len(jwtSecret) < crypto.MinJWTSecretLength {
		return fmt.Errorf("OBSERVAI_JWT_SECRET must be at least %d bytes", crypto.MinJWTSecretLength)
	}

	if cfg.Mode == ModeLocal {
		return nil
	}

	var missing []string
	for _, requirement := range nonLocalRequiredConfigValues {
		if requirement.missing(cfg) {
			missing = append(missing, requirement.envName)
		}
	}
	if cfg.Mode == ModeProd {
		for _, requirement := range prodRequiredConfigValues {
			if requirement.missing(cfg) {
				missing = append(missing, requirement.envName)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("mode=%s requires: %s", cfg.Mode, strings.Join(missing, ", "))
	}
	return nil
}

// TimeLocation loads the configured IANA timezone used for HTTP response timestamps.
func (cfg Config) TimeLocation() (*time.Location, error) {
	timeZone := strings.TrimSpace(cfg.TimeZone)
	if timeZone == "" {
		timeZone = "Local"
	}
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return nil, fmt.Errorf("OBSERVAI_TIMEZONE must be a valid IANA timezone: %w", err)
	}
	return location, nil
}

// LoadEncryptionKey decodes the configured encryption key into raw bytes.
//
// Returns ErrEncryptionKeyMissing when the key is absent. Returns
// crypto.ErrInvalidKey wrapped with mode context when the encoded value is
// not a valid 32-byte key (hex or base64).
func (cfg Config) LoadEncryptionKey() ([]byte, error) {
	encoded := strings.TrimSpace(cfg.EncryptionKey)
	if encoded == "" {
		return nil, ErrEncryptionKeyMissing
	}
	key, err := crypto.LoadKey(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode OBSERVAI_ENCRYPTION_KEY for mode=%s: %w", cfg.Mode, err)
	}
	return key, nil
}

func normalizeMode(mode Mode) Mode {
	normalized := Mode(strings.ToLower(strings.TrimSpace(string(mode))))
	if _, ok := supportedModes[normalized]; ok {
		return normalized
	}
	return ModeLocal
}
