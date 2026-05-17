package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReadsDefaultsFromEnvironment(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "local", cfg.Env)
	assert.Equal(t, ModeLocal, cfg.Mode)
	assert.Equal(t, "Local", cfg.TimeZone)
	assert.Empty(t, cfg.DatabaseDSN)
	assert.Empty(t, cfg.RedisURL)
	assert.Equal(t, 6*time.Hour, cfg.AnalysisContextCacheTTL)
	assert.Equal(t, 30*time.Second, cfg.HTTPRequestTimeout)
	assert.Equal(t, int64(1048576), cfg.HTTPMaxBodyBytes)
	assert.Equal(t, "llama3", cfg.Ollama.Model)
	assert.Equal(t, 30*time.Second, cfg.Ollama.Timeout)
	assert.Equal(t, 10*time.Second, cfg.Prometheus.Timeout)
	assert.Equal(t, "agents", cfg.Prompts.Dir)
}

func TestLoadReadsDotEnvFromWorkingDirectory(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	tempDir := t.TempDir()
	currentDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(currentDir))
	})
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env"), []byte(`
OBSERVAI_API_PORT=18080
OBSERVAI_DATABASE_DSN=postgres://from-dotenv
OBSERVAI_JWT_SECRET=0123456789abcdef0123456789abcdef
`), 0o600))

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "18080", cfg.Port)
	assert.Equal(t, "postgres://from-dotenv", cfg.DatabaseDSN)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", cfg.JWT.Secret)
}

func TestLoadDoesNotLetDotEnvOverrideExistingEnvironment(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	tempDir := t.TempDir()
	currentDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(currentDir))
	})
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env"), []byte(`
OBSERVAI_API_PORT=18080
OBSERVAI_DATABASE_DSN=postgres://from-dotenv
`), 0o600))
	t.Setenv("OBSERVAI_API_PORT", "19090")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "19090", cfg.Port)
	assert.Equal(t, "postgres://from-dotenv", cfg.DatabaseDSN)
}

func TestLoadReadsYAMLAndAllowsEnvOverride(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	configPath := filepath.Join(t.TempDir(), "observai.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
port: "9090"
env: dev
mode: dev
database_dsn: postgres://from-file
redis_url: redis://from-file
analysis_context_cache_ttl: 2h
http_request_timeout: 45s
http_max_body_bytes: 2097152
prometheus:
  url: http://prometheus:9090
  timeout: 5s
ollama:
  url: http://ollama:11434
  model: llama3
  timeout: 20s
prompts:
  dir: agents
timezone: America/Sao_Paulo
`), 0o600))

	t.Setenv("OBSERVAI_CONFIG_FILE", configPath)
	t.Setenv("OBSERVAI_API_PORT", "7070")
	t.Setenv("OBSERVAI_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("OBSERVAI_JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "7070", cfg.Port)
	assert.Equal(t, "dev", cfg.Env)
	assert.Equal(t, ModeDev, cfg.Mode)
	assert.Equal(t, "postgres://from-file", cfg.DatabaseDSN)
	assert.Equal(t, "redis://from-file", cfg.RedisURL)
	assert.Equal(t, 2*time.Hour, cfg.AnalysisContextCacheTTL)
	assert.Equal(t, 45*time.Second, cfg.HTTPRequestTimeout)
	assert.Equal(t, "America/Sao_Paulo", cfg.TimeZone)
	assert.Equal(t, int64(2097152), cfg.HTTPMaxBodyBytes)
	assert.Equal(t, "http://prometheus:9090", cfg.Prometheus.URL)
	assert.Equal(t, 5*time.Second, cfg.Prometheus.Timeout)
	assert.Equal(t, "http://ollama:11434", cfg.Ollama.URL)
	assert.Equal(t, 20*time.Second, cfg.Ollama.Timeout)
}

func TestLoadProdRequiresProviderConfiguration(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_MODE", "prod")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OBSERVAI_ENCRYPTION_KEY")
	assert.Contains(t, err.Error(), "OBSERVAI_JWT_SECRET")
	assert.Contains(t, err.Error(), "OBSERVAI_DATABASE_DSN")
	assert.Contains(t, err.Error(), "OBSERVAI_REDIS_URL")
	assert.Contains(t, err.Error(), "OBSERVAI_PROMETHEUS_URL")
	assert.Contains(t, err.Error(), "OBSERVAI_OLLAMA_URL")
}

func TestLoadProdAcceptsFullyConfiguredEnvironment(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_MODE", "prod")
	t.Setenv("OBSERVAI_DATABASE_DSN", "postgres://prod")
	t.Setenv("OBSERVAI_REDIS_URL", "redis://prod")
	t.Setenv("OBSERVAI_PROMETHEUS_URL", "http://prometheus:9090")
	t.Setenv("OBSERVAI_OLLAMA_URL", "http://ollama:11434")
	t.Setenv("OBSERVAI_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("OBSERVAI_JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ModeProd, cfg.Mode)
	assert.NotEmpty(t, cfg.EncryptionKey)
}

func TestLoadDevRequiresEncryptionKey(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_MODE", "dev")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OBSERVAI_ENCRYPTION_KEY")
	assert.Contains(t, err.Error(), "OBSERVAI_JWT_SECRET")
	assert.NotContains(t, err.Error(), "OBSERVAI_DATABASE_DSN")
}

func TestLoadRejectsShortJWTSecret(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_MODE", "dev")
	t.Setenv("OBSERVAI_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("OBSERVAI_JWT_SECRET", "short")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OBSERVAI_JWT_SECRET")
}

func TestLoadRejectsInvalidTimezone(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_TIMEZONE", "Mars/Phobos")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OBSERVAI_TIMEZONE")
}

func TestLoadDevAcceptsEncryptionKey(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_MODE", "dev")
	t.Setenv("OBSERVAI_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("OBSERVAI_JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ModeDev, cfg.Mode)
}

func TestLoadRejectsMalformedEncryptionKey(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_MODE", "dev")
	t.Setenv("OBSERVAI_ENCRYPTION_KEY", "not-a-real-key")
	t.Setenv("OBSERVAI_JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OBSERVAI_ENCRYPTION_KEY")
}

func TestLoadLocalDoesNotRequireEncryptionKey(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ModeLocal, cfg.Mode)
	assert.Empty(t, cfg.EncryptionKey)
}

func TestLoadLocalRejectsMalformedEncryptionKeyWhenProvided(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_ENCRYPTION_KEY", "not-a-real-key")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OBSERVAI_ENCRYPTION_KEY")
}

func TestLoadLocalRejectsShortJWTSecretWhenProvided(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_JWT_SECRET", "short")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OBSERVAI_JWT_SECRET")
}

func TestLoadMigratesLegacyPrometheusAndOllamaIntoProviderLists(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_PROMETHEUS_URL", "http://prom:9090")
	t.Setenv("OBSERVAI_OLLAMA_URL", "http://ollama:11434")

	cfg, err := Load()
	require.NoError(t, err)

	require.Len(t, cfg.Observability.Providers, 1)
	assert.Equal(t, "prometheus", cfg.Observability.Providers[0].Type)
	assert.Equal(t, "http://prom:9090", cfg.Observability.Providers[0].URL)
	assert.Contains(t, cfg.Observability.Providers[0].Signals, "metrics")

	require.Len(t, cfg.LLM.Providers, 1)
	assert.Equal(t, "ollama", cfg.LLM.Providers[0].Type)
	assert.Equal(t, "http://ollama:11434", cfg.LLM.Providers[0].URL)
	assert.Equal(t, "ollama", cfg.LLM.Active)
}

func TestLoadHonoursExplicitProvidersListInYAML(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	configPath := filepath.Join(t.TempDir(), "observai.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
mode: dev
observability:
  providers:
    - type: prometheus
      name: prom-prod
      url: http://prom.svc:9090
      timeout: 8s
      signals: [metrics]
    - type: loki
      name: loki-prod
      url: http://loki:3100
      signals: [logs]
llm:
  providers:
    - type: ollama
      name: local-ollama
      url: http://localhost:11434
      model: llama3
  active: local-ollama
`), 0o600))

	t.Setenv("OBSERVAI_CONFIG_FILE", configPath)
	t.Setenv("OBSERVAI_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("OBSERVAI_JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	require.NoError(t, err)

	require.Len(t, cfg.Observability.Providers, 2)
	assert.Equal(t, "prom-prod", cfg.Observability.Providers[0].Name)
	assert.Equal(t, "loki", cfg.Observability.Providers[1].Type)

	require.Len(t, cfg.LLM.Providers, 1)
	assert.Equal(t, "local-ollama", cfg.LLM.Active)
}

func TestLoadDoesNotDuplicateLegacyProviderWhenAlsoDeclaredInList(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	configPath := filepath.Join(t.TempDir(), "observai.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
prometheus:
  url: http://legacy:9090
observability:
  providers:
    - type: prometheus
      name: explicit
      url: http://new:9090
      signals: [metrics]
`), 0o600))

	t.Setenv("OBSERVAI_CONFIG_FILE", configPath)

	cfg, err := Load()
	require.NoError(t, err)

	require.Len(t, cfg.Observability.Providers, 1)
	assert.Equal(t, "explicit", cfg.Observability.Providers[0].Name)
	assert.Equal(t, "http://new:9090", cfg.Observability.Providers[0].URL)
}

func TestLoadMergesCORSOriginsFromEnv(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	configPath := filepath.Join(t.TempDir(), "observai.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
cors:
  origins:
    - https://yaml.example.com
`), 0o600))

	t.Setenv("OBSERVAI_CONFIG_FILE", configPath)
	t.Setenv("OBSERVAI_ALLOWED_ORIGINS", "https://env.example.com, https://other.example.com")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, []string{
		"https://yaml.example.com",
		"https://env.example.com",
		"https://other.example.com",
	}, cfg.CORS.Origins)
}

func TestLoadLeavesCORSOriginsEmptyWhenUnset(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Empty(t, cfg.CORS.Origins)
}

func TestNormalizeModeFallsBackToLocalForUnknownValues(t *testing.T) {
	unsetEnv(t, allConfigEnvKeys()...)

	t.Setenv("OBSERVAI_MODE", "staging")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ModeLocal, cfg.Mode)
}

func allConfigEnvKeys() []string {
	return []string{
		"OBSERVAI_CONFIG_FILE",
		"OBSERVAI_API_PORT",
		"OBSERVAI_ENV",
		"OBSERVAI_MODE",
		"OBSERVAI_TIMEZONE",
		"OBSERVAI_DATABASE_DSN",
		"OBSERVAI_REDIS_URL",
		"OBSERVAI_ANALYSIS_CONTEXT_CACHE_TTL",
		"OBSERVAI_HTTP_REQUEST_TIMEOUT",
		"OBSERVAI_HTTP_MAX_BODY_BYTES",
		"OBSERVAI_PROMETHEUS_URL",
		"OBSERVAI_PROMETHEUS_TIMEOUT",
		"OBSERVAI_OLLAMA_URL",
		"OBSERVAI_OLLAMA_MODEL",
		"OBSERVAI_OLLAMA_TIMEOUT",
		"OBSERVAI_PROMPTS_DIR",
		"OBSERVAI_ENCRYPTION_KEY",
		"OBSERVAI_JWT_SECRET",
		"OBSERVAI_JWT_ISSUER",
		"OBSERVAI_JWT_ACCESS_TTL",
		"OBSERVAI_JWT_REFRESH_TTL",
		"OBSERVAI_AUTH_COOKIE_DOMAIN",
		"OBSERVAI_AUTH_COOKIE_SECURE",
		"OBSERVAI_ALLOWED_ORIGINS",
	}
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()

	previous := make(map[string]string, len(keys))
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if ok {
			previous[key] = value
			present[key] = true
		}
		require.NoError(t, os.Unsetenv(key))
	}

	t.Cleanup(func() {
		for _, key := range keys {
			if !present[key] {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, previous[key])
		}
	})
}
