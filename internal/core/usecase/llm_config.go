package usecase

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// LLMConfigRequest is the input accepted by LLMConfig.Create and
// LLMConfig.Update.
//
// Type accepts any spelling recognized by
// ports.LLMProviderRegistry.Normalize (canonical name or alias); the
// use case stores the canonical form so a stored configuration always
// uses one stable identifier.
type LLMConfigRequest struct {
	Type     string
	Name     string
	BaseURL  string
	Model    string
	Timeout  time.Duration
	Options  map[string]string
	APIKey   string
	IsActive bool
}

// LLMConfig orchestrates CRUD, test-connection and activation flows for
// LLM provider configurations. Exactly one configuration may be active at
// a time; Activate atomically deactivates every other entry.
type LLMConfig struct {
	repository ports.LLMConfigRepository
	cipher     ports.Cipher
	tester     ports.ProviderTester
	registry   ports.LLMProviderRegistry
	ids        ports.IDGenerator
	reload     ReloadHook
	now        func() time.Time
}

// NewLLMConfig creates an LLMConfig use case.
//
// registry is the source of truth for supported LLM provider types; the
// use case calls Normalize so aliases (for example "openai-compatible")
// are stored under their canonical key.
func NewLLMConfig(repository ports.LLMConfigRepository, cipher ports.Cipher, tester ports.ProviderTester, registry ports.LLMProviderRegistry, ids ports.IDGenerator) *LLMConfig {
	return &LLMConfig{repository: repository, cipher: cipher, tester: tester, registry: registry, ids: ids, now: time.Now}
}

// WithReloadHook installs a hook fired after mutations so the composition
// root can rebuild the live adapter set.
func (useCase *LLMConfig) WithReloadHook(hook ReloadHook) *LLMConfig {
	useCase.reload = hook
	return useCase
}

func (useCase *LLMConfig) fireReload(ctx context.Context) {
	if useCase.reload != nil {
		useCase.reload(ctx)
	}
}

// Create persists a new LLM configuration.
func (useCase *LLMConfig) Create(ctx context.Context, request LLMConfigRequest) (domain.LLMConfig, error) {
	providerType, err := useCase.validateRequest(request)
	if err != nil {
		return domain.LLMConfig{}, err
	}
	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.LLMConfig{}, fmt.Errorf("generate llm config id: %w", err)
	}
	ciphertext, err := useCase.encryptAPIKey(request.APIKey)
	if err != nil {
		return domain.LLMConfig{}, err
	}
	now := useCase.now().UTC()
	config := domain.LLMConfig{
		ID:           id,
		Type:         providerType,
		Name:         strings.TrimSpace(request.Name),
		BaseURL:      strings.TrimSpace(request.BaseURL),
		Model:        strings.TrimSpace(request.Model),
		Timeout:      request.Timeout,
		Options:      request.Options,
		APIKeyCipher: ciphertext,
		IsActive:     false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := useCase.repository.Create(ctx, config); err != nil {
		return domain.LLMConfig{}, err
	}
	if request.IsActive {
		if err := useCase.repository.Activate(ctx, config.ID, now); err != nil {
			return domain.LLMConfig{}, err
		}
		config.IsActive = true
	}
	useCase.fireReload(ctx)
	return config, nil
}

// Get returns the LLM configuration matching the identifier.
func (useCase *LLMConfig) Get(ctx context.Context, id string) (domain.LLMConfig, error) {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.LLMConfig{}, domain.ErrLLMConfigNotFound
	}
	return useCase.repository.Find(ctx, cleaned)
}

// List returns persisted LLM configurations.
func (useCase *LLMConfig) List(ctx context.Context, limit, offset int) ([]domain.LLMConfig, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return useCase.repository.List(ctx, limit, offset)
}

// Update replaces a stored LLM configuration. When request.APIKey is
// empty the previous ciphertext is preserved.
func (useCase *LLMConfig) Update(ctx context.Context, id string, request LLMConfigRequest) (domain.LLMConfig, error) {
	providerType, err := useCase.validateRequest(request)
	if err != nil {
		return domain.LLMConfig{}, err
	}
	current, err := useCase.repository.Find(ctx, id)
	if err != nil {
		return domain.LLMConfig{}, err
	}
	ciphertext := current.APIKeyCipher
	if strings.TrimSpace(request.APIKey) != "" {
		ciphertext, err = useCase.encryptAPIKey(request.APIKey)
		if err != nil {
			return domain.LLMConfig{}, err
		}
	}
	current.Type = providerType
	current.Name = strings.TrimSpace(request.Name)
	current.BaseURL = strings.TrimSpace(request.BaseURL)
	current.Model = strings.TrimSpace(request.Model)
	current.Timeout = request.Timeout
	current.Options = request.Options
	current.APIKeyCipher = ciphertext
	current.UpdatedAt = useCase.now().UTC()
	if err := useCase.repository.Update(ctx, current); err != nil {
		return domain.LLMConfig{}, err
	}
	useCase.fireReload(ctx)
	return current, nil
}

// Activate marks the supplied LLM configuration as the active one,
// deactivating every other entry atomically.
func (useCase *LLMConfig) Activate(ctx context.Context, id string) (domain.LLMConfig, error) {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.LLMConfig{}, domain.ErrLLMConfigNotFound
	}
	now := useCase.now().UTC()
	if err := useCase.repository.Activate(ctx, cleaned, now); err != nil {
		return domain.LLMConfig{}, err
	}
	config, err := useCase.repository.Find(ctx, cleaned)
	if err != nil {
		return domain.LLMConfig{}, err
	}
	useCase.fireReload(ctx)
	return config, nil
}

// Delete removes the LLM configuration.
func (useCase *LLMConfig) Delete(ctx context.Context, id string) error {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.ErrLLMConfigNotFound
	}
	if err := useCase.repository.Delete(ctx, cleaned); err != nil {
		return err
	}
	useCase.fireReload(ctx)
	return nil
}

// Test runs a liveness probe against the supplied LLM configuration.
func (useCase *LLMConfig) Test(ctx context.Context, id string) (ports.ProviderTestResult, error) {
	config, err := useCase.repository.Find(ctx, id)
	if err != nil {
		return ports.ProviderTestResult{}, err
	}
	if useCase.tester == nil {
		return ports.ProviderTestResult{}, fmt.Errorf("provider tester not configured")
	}
	apiKey, err := useCase.decryptAPIKey(config.APIKeyCipher)
	if err != nil {
		return ports.ProviderTestResult{}, err
	}
	return useCase.tester.TestLLM(ctx, config, apiKey), nil
}

// FindActiveConfig returns the currently-active LLM configuration, if any.
func (useCase *LLMConfig) FindActiveConfig(ctx context.Context) (domain.LLMConfig, error) {
	return useCase.repository.FindActive(ctx)
}

// DecryptAPIKey exposes the decrypted API key for the composition root.
func (useCase *LLMConfig) DecryptAPIKey(config domain.LLMConfig) (string, error) {
	return useCase.decryptAPIKey(config.APIKeyCipher)
}

func (useCase *LLMConfig) validateRequest(request LLMConfigRequest) (string, error) {
	if useCase.registry == nil {
		return "", fmt.Errorf("%w: type %q is not supported", domain.ErrInvalidLLMConfig, request.Type)
	}
	providerType, ok := useCase.registry.Normalize(request.Type)
	if !ok {
		return "", fmt.Errorf("%w: type %q is not supported", domain.ErrInvalidLLMConfig, request.Type)
	}
	if strings.TrimSpace(request.Name) == "" {
		return "", fmt.Errorf("%w: name is required", domain.ErrInvalidLLMConfig)
	}
	if strings.TrimSpace(request.Model) == "" {
		return "", fmt.Errorf("%w: model is required", domain.ErrInvalidLLMConfig)
	}
	parsed, err := url.Parse(strings.TrimSpace(request.BaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: base url must be absolute http(s)", domain.ErrInvalidLLMConfig)
	}
	if request.Timeout < 0 {
		return "", fmt.Errorf("%w: timeout must be non-negative", domain.ErrInvalidLLMConfig)
	}
	return providerType, nil
}

func (useCase *LLMConfig) encryptAPIKey(plaintext string) (string, error) {
	cleaned := strings.TrimSpace(plaintext)
	if cleaned == "" {
		return "", nil
	}
	if useCase.cipher == nil {
		return "", fmt.Errorf("%w: encryption cipher is not configured", domain.ErrInvalidLLMConfig)
	}
	ciphertext, err := useCase.cipher.Encrypt([]byte(cleaned))
	if err != nil {
		return "", fmt.Errorf("encrypt llm api key: %w", err)
	}
	return ciphertext, nil
}

func (useCase *LLMConfig) decryptAPIKey(ciphertext string) (string, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return "", nil
	}
	if useCase.cipher == nil {
		return "", fmt.Errorf("%w: encryption cipher is not configured", domain.ErrInvalidLLMConfig)
	}
	plaintext, err := useCase.cipher.Decrypt(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt llm api key: %w", err)
	}
	return string(plaintext), nil
}
