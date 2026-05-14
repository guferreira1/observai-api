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

// ProviderConfigRequest is the input accepted by ProviderConfig.Create and
// ProviderConfig.Update.
//
// Credentials carries the plaintext secret material; the use case
// encrypts it before persisting and never stores plaintext. When the
// caller omits credentials on Update the previous ciphertext is preserved.
type ProviderConfigRequest struct {
	Type        domain.ObservabilityProviderType
	Name        string
	URL         string
	Timeout     time.Duration
	Signals     []string
	Options     map[string]string
	Credentials string
	IsActive    bool
}

// ProviderConfig orchestrates CRUD and test-connection flows for
// observability provider configurations.
type ProviderConfig struct {
	repository ports.ProviderConfigRepository
	cipher     ports.Cipher
	tester     ports.ProviderTester
	ids        ports.IDGenerator
	now        func() time.Time
}

// NewProviderConfig creates a ProviderConfig use case.
func NewProviderConfig(repository ports.ProviderConfigRepository, cipher ports.Cipher, tester ports.ProviderTester, ids ports.IDGenerator) *ProviderConfig {
	return &ProviderConfig{repository: repository, cipher: cipher, tester: tester, ids: ids, now: time.Now}
}

// Create persists a new provider configuration.
func (useCase *ProviderConfig) Create(ctx context.Context, request ProviderConfigRequest) (domain.ProviderConfig, error) {
	if err := useCase.validateRequest(request); err != nil {
		return domain.ProviderConfig{}, err
	}
	id, err := useCase.ids.NextID(ctx)
	if err != nil {
		return domain.ProviderConfig{}, fmt.Errorf("generate provider config id: %w", err)
	}
	ciphertext, err := useCase.encryptCredentials(request.Credentials)
	if err != nil {
		return domain.ProviderConfig{}, err
	}
	now := useCase.now().UTC()
	config := domain.ProviderConfig{
		ID:                    id,
		Type:                  request.Type,
		Name:                  strings.TrimSpace(request.Name),
		URL:                   strings.TrimSpace(request.URL),
		Timeout:               request.Timeout,
		Signals:               request.Signals,
		Options:               request.Options,
		CredentialsCiphertext: ciphertext,
		IsActive:              request.IsActive,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := useCase.repository.Create(ctx, config); err != nil {
		return domain.ProviderConfig{}, err
	}
	return config, nil
}

// Get returns the provider configuration matching the identifier.
func (useCase *ProviderConfig) Get(ctx context.Context, id string) (domain.ProviderConfig, error) {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.ProviderConfig{}, domain.ErrProviderConfigNotFound
	}
	return useCase.repository.Find(ctx, cleaned)
}

// List returns persisted provider configurations.
func (useCase *ProviderConfig) List(ctx context.Context, limit, offset int) ([]domain.ProviderConfig, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return useCase.repository.List(ctx, limit, offset)
}

// Update replaces a stored provider configuration. When request.Credentials
// is empty the previous ciphertext is preserved so the caller can edit
// non-secret fields without re-supplying the secret.
func (useCase *ProviderConfig) Update(ctx context.Context, id string, request ProviderConfigRequest) (domain.ProviderConfig, error) {
	if err := useCase.validateRequest(request); err != nil {
		return domain.ProviderConfig{}, err
	}
	current, err := useCase.repository.Find(ctx, id)
	if err != nil {
		return domain.ProviderConfig{}, err
	}
	ciphertext := current.CredentialsCiphertext
	if strings.TrimSpace(request.Credentials) != "" {
		ciphertext, err = useCase.encryptCredentials(request.Credentials)
		if err != nil {
			return domain.ProviderConfig{}, err
		}
	}
	current.Type = request.Type
	current.Name = strings.TrimSpace(request.Name)
	current.URL = strings.TrimSpace(request.URL)
	current.Timeout = request.Timeout
	current.Signals = request.Signals
	current.Options = request.Options
	current.CredentialsCiphertext = ciphertext
	current.UpdatedAt = useCase.now().UTC()
	if err := useCase.repository.Update(ctx, current); err != nil {
		return domain.ProviderConfig{}, err
	}
	return current, nil
}

// Activate marks the configuration as active.
func (useCase *ProviderConfig) Activate(ctx context.Context, id string) (domain.ProviderConfig, error) {
	return useCase.setActive(ctx, id, true)
}

// Deactivate marks the configuration as inactive.
func (useCase *ProviderConfig) Deactivate(ctx context.Context, id string) (domain.ProviderConfig, error) {
	return useCase.setActive(ctx, id, false)
}

// Delete removes the provider configuration.
func (useCase *ProviderConfig) Delete(ctx context.Context, id string) error {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.ErrProviderConfigNotFound
	}
	return useCase.repository.Delete(ctx, cleaned)
}

// Test runs a liveness probe against the supplied configuration using the
// configured ProviderTester. Credentials are decrypted in memory and never
// returned to the caller.
func (useCase *ProviderConfig) Test(ctx context.Context, id string) (ports.ProviderTestResult, error) {
	config, err := useCase.repository.Find(ctx, id)
	if err != nil {
		return ports.ProviderTestResult{}, err
	}
	if useCase.tester == nil {
		return ports.ProviderTestResult{}, fmt.Errorf("provider tester not configured")
	}
	credentials, err := useCase.decryptCredentials(config.CredentialsCiphertext)
	if err != nil {
		return ports.ProviderTestResult{}, err
	}
	return useCase.tester.TestObservability(ctx, config, credentials), nil
}

// ListActiveConfigs returns the provider configurations currently flagged
// as active. Used by the composition root to bootstrap adapters.
func (useCase *ProviderConfig) ListActiveConfigs(ctx context.Context) ([]domain.ProviderConfig, error) {
	return useCase.repository.ListActive(ctx)
}

// DecryptCredentials exposes the decrypted credential for callers that
// need to instantiate an adapter (composition root and the tester
// internals). Returns an empty string when the configuration has no
// stored credential.
func (useCase *ProviderConfig) DecryptCredentials(config domain.ProviderConfig) (string, error) {
	return useCase.decryptCredentials(config.CredentialsCiphertext)
}

func (useCase *ProviderConfig) setActive(ctx context.Context, id string, active bool) (domain.ProviderConfig, error) {
	cleaned := strings.TrimSpace(id)
	if cleaned == "" {
		return domain.ProviderConfig{}, domain.ErrProviderConfigNotFound
	}
	now := useCase.now().UTC()
	if err := useCase.repository.SetActive(ctx, cleaned, active, now); err != nil {
		return domain.ProviderConfig{}, err
	}
	return useCase.repository.Find(ctx, cleaned)
}

func (useCase *ProviderConfig) validateRequest(request ProviderConfigRequest) error {
	if !domain.IsValidObservabilityProviderType(request.Type) {
		return fmt.Errorf("%w: type %q is not supported", domain.ErrInvalidProviderConfig, request.Type)
	}
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("%w: name is required", domain.ErrInvalidProviderConfig)
	}
	parsed, err := url.Parse(strings.TrimSpace(request.URL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: url must be absolute http(s)", domain.ErrInvalidProviderConfig)
	}
	if request.Timeout < 0 {
		return fmt.Errorf("%w: timeout must be non-negative", domain.ErrInvalidProviderConfig)
	}
	return nil
}

func (useCase *ProviderConfig) encryptCredentials(plaintext string) (string, error) {
	cleaned := strings.TrimSpace(plaintext)
	if cleaned == "" {
		return "", nil
	}
	if useCase.cipher == nil {
		return "", fmt.Errorf("%w: encryption cipher is not configured", domain.ErrInvalidProviderConfig)
	}
	ciphertext, err := useCase.cipher.Encrypt([]byte(cleaned))
	if err != nil {
		return "", fmt.Errorf("encrypt provider credentials: %w", err)
	}
	return ciphertext, nil
}

func (useCase *ProviderConfig) decryptCredentials(ciphertext string) (string, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return "", nil
	}
	if useCase.cipher == nil {
		return "", fmt.Errorf("%w: encryption cipher is not configured", domain.ErrInvalidProviderConfig)
	}
	plaintext, err := useCase.cipher.Decrypt(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt provider credentials: %w", err)
	}
	return string(plaintext), nil
}
