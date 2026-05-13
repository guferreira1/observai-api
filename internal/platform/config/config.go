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

// Config contains application runtime configuration.
type Config struct {
	Port                    string           `yaml:"port" env:"OBSERVAI_API_PORT" env-default:"8080"`
	Env                     string           `yaml:"env" env:"OBSERVAI_ENV" env-default:"local"`
	Mode                    Mode             `yaml:"mode" env:"OBSERVAI_MODE" env-default:"local"`
	DatabaseDSN             string           `yaml:"database_dsn" env:"OBSERVAI_DATABASE_DSN"`
	RedisURL                string           `yaml:"redis_url" env:"OBSERVAI_REDIS_URL"`
	AnalysisContextCacheTTL time.Duration    `yaml:"analysis_context_cache_ttl" env:"OBSERVAI_ANALYSIS_CONTEXT_CACHE_TTL" env-default:"6h"`
	HTTPRequestTimeout      time.Duration    `yaml:"http_request_timeout" env:"OBSERVAI_HTTP_REQUEST_TIMEOUT" env-default:"30s"`
	HTTPMaxBodyBytes        int64            `yaml:"http_max_body_bytes" env:"OBSERVAI_HTTP_MAX_BODY_BYTES" env-default:"1048576"`
	Prometheus              PrometheusConfig `yaml:"prometheus"`
	Ollama                  OllamaConfig     `yaml:"ollama"`
	Prompts                 PromptsConfig    `yaml:"prompts"`
	Tracing                 TracingConfig    `yaml:"tracing"`
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
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		missing = append(missing, "OBSERVAI_DATABASE_DSN")
	}
	if strings.TrimSpace(cfg.RedisURL) == "" {
		missing = append(missing, "OBSERVAI_REDIS_URL")
	}
	if strings.TrimSpace(cfg.Prometheus.URL) == "" {
		missing = append(missing, "OBSERVAI_PROMETHEUS_URL")
	}
	if strings.TrimSpace(cfg.Ollama.URL) == "" {
		missing = append(missing, "OBSERVAI_OLLAMA_URL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("mode=prod requires: %s", strings.Join(missing, ", "))
	}
	return nil
}

func normalizeMode(mode Mode) Mode {
	switch Mode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case ModeLocal, ModeDev, ModeProd:
		return Mode(strings.ToLower(strings.TrimSpace(string(mode))))
	default:
		return ModeLocal
	}
}
