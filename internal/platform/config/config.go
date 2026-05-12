package config

import (
	"fmt"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config contains application runtime configuration.
type Config struct {
	Port                    string        `env:"OBSERVAI_API_PORT" env-default:"8080"`
	Env                     string        `env:"OBSERVAI_ENV" env-default:"local"`
	DatabaseDSN             string        `env:"OBSERVAI_DATABASE_DSN"`
	RedisURL                string        `env:"OBSERVAI_REDIS_URL"`
	AnalysisContextCacheTTL time.Duration `env:"OBSERVAI_ANALYSIS_CONTEXT_CACHE_TTL" env-default:"6h"`
}

// Load reads application configuration from environment variables.
func Load() (Config, error) {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return Config{}, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}
