package ports

import (
	"context"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// Cipher encrypts and decrypts byte payloads used to protect provider
// credentials at rest.
type Cipher interface {
	Encrypt(plaintext []byte) (string, error)
	Decrypt(ciphertext string) ([]byte, error)
}

// ProviderConfigRepository persists observability provider configurations.
type ProviderConfigRepository interface {
	Create(ctx context.Context, config domain.ProviderConfig) error
	Find(ctx context.Context, id string) (domain.ProviderConfig, error)
	List(ctx context.Context, limit int, offset int) ([]domain.ProviderConfig, error)
	ListActive(ctx context.Context) ([]domain.ProviderConfig, error)
	Count(ctx context.Context) (int64, error)
	Update(ctx context.Context, config domain.ProviderConfig) error
	SetActive(ctx context.Context, id string, active bool, when time.Time) error
	Delete(ctx context.Context, id string) error
}

// LLMConfigRepository persists LLM provider configurations.
type LLMConfigRepository interface {
	Create(ctx context.Context, config domain.LLMConfig) error
	Find(ctx context.Context, id string) (domain.LLMConfig, error)
	List(ctx context.Context, limit int, offset int) ([]domain.LLMConfig, error)
	FindActive(ctx context.Context) (domain.LLMConfig, error)
	Count(ctx context.Context) (int64, error)
	Update(ctx context.Context, config domain.LLMConfig) error
	Activate(ctx context.Context, id string, when time.Time) error
	Delete(ctx context.Context, id string) error
}

// ProviderTestResult bundles the outcome of a Test connection call.
type ProviderTestResult struct {
	Reached   bool
	LatencyMs int64
	Code      string
	Error     string
}

// ProviderTester runs a lightweight liveness probe against a stored
// provider configuration without touching the live adapter set.
type ProviderTester interface {
	TestObservability(ctx context.Context, config domain.ProviderConfig, plaintextCredentials string) ProviderTestResult
	TestLLM(ctx context.Context, config domain.LLMConfig, plaintextAPIKey string) ProviderTestResult
}
