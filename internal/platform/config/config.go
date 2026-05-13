// Package config loads the runtime configuration for ObservAI API.
//
// Configuration may come from a YAML file referenced by OBSERVAI_CONFIG_FILE
// or directly from environment variables. Environment variables always win
// over YAML values when both are set.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

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
	value   func(Config) string
}

var prodRequiredConfigValues = []requiredConfigValue{
	{envName: "OBSERVAI_DATABASE_DSN", value: func(cfg Config) string { return cfg.DatabaseDSN }},
	{envName: "OBSERVAI_REDIS_URL", value: func(cfg Config) string { return cfg.RedisURL }},
	{envName: "OBSERVAI_PROMETHEUS_URL", value: func(cfg Config) string { return cfg.Prometheus.URL }},
	{envName: "OBSERVAI_OLLAMA_URL", value: func(cfg Config) string { return cfg.Ollama.URL }},
}

// PrometheusConfig holds Prometheus adapter configuration.
type PrometheusConfig struct {
	URL     string        `yaml:"url" env:"OBSERVAI_PROMETHEUS_URL"`
	Timeout time.Duration `yaml:"timeout" env:"OBSERVAI_PROMETHEUS_TIMEOUT" env-default:"10s"`
}

// OllamaConfig holds Ollama adapter configuration.
type OllamaConfig struct {
	URL     string        `yaml:"url" env:"OBSERVAI_OLLAMA_URL"`
	Model   string        `yaml:"model" env:"OBSERVAI_OLLAMA_MODEL" env-default:"llama3"`
	Timeout time.Duration `yaml:"timeout" env:"OBSERVAI_OLLAMA_TIMEOUT" env-default:"30s"`
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
}

// Config contains application runtime configuration.
type Config struct {
	Port                    string              `yaml:"port" env:"OBSERVAI_API_PORT" env-default:"8080"`
	Env                     string              `yaml:"env" env:"OBSERVAI_ENV" env-default:"local"`
	Mode                    Mode                `yaml:"mode" env:"OBSERVAI_MODE" env-default:"local"`
	DatabaseDSN             string              `yaml:"database_dsn" env:"OBSERVAI_DATABASE_DSN"`
	RedisURL                string              `yaml:"redis_url" env:"OBSERVAI_REDIS_URL"`
	AnalysisContextCacheTTL time.Duration       `yaml:"analysis_context_cache_ttl" env:"OBSERVAI_ANALYSIS_CONTEXT_CACHE_TTL" env-default:"6h"`
	HTTPRequestTimeout      time.Duration       `yaml:"http_request_timeout" env:"OBSERVAI_HTTP_REQUEST_TIMEOUT" env-default:"30s"`
	HTTPMaxBodyBytes        int64               `yaml:"http_max_body_bytes" env:"OBSERVAI_HTTP_MAX_BODY_BYTES" env-default:"1048576"`
	HTTPShutdownTimeout     time.Duration       `yaml:"http_shutdown_timeout" env:"OBSERVAI_HTTP_SHUTDOWN_TIMEOUT" env-default:"30s"`
	HTTPRateLimit           HTTPRateLimitConfig `yaml:"http_rate_limit"`
	Queue                   QueueConfig         `yaml:"queue"`
	Prometheus              PrometheusConfig    `yaml:"prometheus"`
	Ollama                  OllamaConfig        `yaml:"ollama"`
	Prompts                 PromptsConfig       `yaml:"prompts"`
	Tracing                 TracingConfig       `yaml:"tracing"`
}

// Load reads application configuration from YAML when configured and environment variables.
func Load() (Config, error) {
	var cfg Config

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
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate enforces mode-specific configuration requirements.
//
// In production every external dependency must be explicitly configured so the
// API never silently falls back to in-memory or fake adapters.
func (cfg Config) Validate() error {
	if cfg.Mode != ModeProd {
		return nil
	}

	var missing []string
	for _, requirement := range prodRequiredConfigValues {
		if strings.TrimSpace(requirement.value(cfg)) == "" {
			missing = append(missing, requirement.envName)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("mode=prod requires: %s", strings.Join(missing, ", "))
	}
	return nil
}

func normalizeMode(mode Mode) Mode {
	normalized := Mode(strings.ToLower(strings.TrimSpace(string(mode))))
	if _, ok := supportedModes[normalized]; ok {
		return normalized
	}
	return ModeLocal
}
