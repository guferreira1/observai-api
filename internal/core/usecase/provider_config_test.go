package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

type stubCipher struct{}

func (stubCipher) Encrypt(plaintext []byte) (string, error) {
	return "ct:" + string(plaintext), nil
}

func (stubCipher) Decrypt(ciphertext string) ([]byte, error) {
	if len(ciphertext) < 3 {
		return nil, errors.New("invalid ciphertext")
	}
	return []byte(ciphertext[3:]), nil
}

type stubProviderTester struct {
	observability ports.ProviderTestResult
	llm           ports.ProviderTestResult
}

func (stub stubProviderTester) TestObservability(_ context.Context, _ domain.ProviderConfig, _ string) ports.ProviderTestResult {
	return stub.observability
}

func (stub stubProviderTester) TestLLM(_ context.Context, _ domain.LLMConfig, _ string) ports.ProviderTestResult {
	return stub.llm
}

type stubProviderConfigRepo struct {
	mu      sync.Mutex
	configs map[string]domain.ProviderConfig
}

func newStubProviderConfigRepo() *stubProviderConfigRepo {
	return &stubProviderConfigRepo{configs: make(map[string]domain.ProviderConfig)}
}

func (stub *stubProviderConfigRepo) Create(_ context.Context, config domain.ProviderConfig) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	for _, other := range stub.configs {
		if other.Name == config.Name {
			return domain.ErrProviderConfigConflict
		}
	}
	stub.configs[config.ID] = config
	return nil
}

func (stub *stubProviderConfigRepo) Find(_ context.Context, id string) (domain.ProviderConfig, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	config, ok := stub.configs[id]
	if !ok {
		return domain.ProviderConfig{}, domain.ErrProviderConfigNotFound
	}
	return config, nil
}

func (stub *stubProviderConfigRepo) List(_ context.Context, _, _ int) ([]domain.ProviderConfig, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.ProviderConfig, 0, len(stub.configs))
	for _, config := range stub.configs {
		out = append(out, config)
	}
	return out, nil
}

func (stub *stubProviderConfigRepo) ListActive(_ context.Context) ([]domain.ProviderConfig, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.ProviderConfig, 0)
	for _, config := range stub.configs {
		if config.IsActive {
			out = append(out, config)
		}
	}
	return out, nil
}

func (stub *stubProviderConfigRepo) Count(_ context.Context) (int64, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return int64(len(stub.configs)), nil
}

func (stub *stubProviderConfigRepo) Update(_ context.Context, config domain.ProviderConfig) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.configs[config.ID] = config
	return nil
}

func (stub *stubProviderConfigRepo) SetActive(_ context.Context, id string, active bool, _ time.Time) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	config, ok := stub.configs[id]
	if !ok {
		return domain.ErrProviderConfigNotFound
	}
	config.IsActive = active
	stub.configs[id] = config
	return nil
}

func (stub *stubProviderConfigRepo) Delete(_ context.Context, id string) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	delete(stub.configs, id)
	return nil
}

func TestProviderConfigCreatePersistsAndEncrypts(t *testing.T) {
	repo := newStubProviderConfigRepo()
	useCase := NewProviderConfig(repo, stubCipher{}, stubProviderTester{}, &sequentialIDs{})
	config, err := useCase.Create(context.Background(), ProviderConfigRequest{
		Type:        domain.ProviderTypePrometheus,
		Name:        "prom-prod",
		URL:         "http://prometheus:9090",
		Timeout:     10 * time.Second,
		Signals:     []string{"metrics"},
		Credentials: "secret-token",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	stored, _ := repo.Find(context.Background(), config.ID)
	if stored.CredentialsCiphertext != "ct:secret-token" {
		t.Fatalf("credentials not encrypted: %q", stored.CredentialsCiphertext)
	}
}

func TestProviderConfigCreateRejectsInvalidType(t *testing.T) {
	useCase := NewProviderConfig(newStubProviderConfigRepo(), stubCipher{}, stubProviderTester{}, &sequentialIDs{})
	_, err := useCase.Create(context.Background(), ProviderConfigRequest{
		Type: "bogus",
		Name: "x",
		URL:  "http://x",
	})
	if !errors.Is(err, domain.ErrInvalidProviderConfig) {
		t.Fatalf("expected ErrInvalidProviderConfig, got %v", err)
	}
}

func TestProviderConfigUpdatePreservesCredentialsWhenEmpty(t *testing.T) {
	repo := newStubProviderConfigRepo()
	useCase := NewProviderConfig(repo, stubCipher{}, stubProviderTester{}, &sequentialIDs{})
	created, err := useCase.Create(context.Background(), ProviderConfigRequest{
		Type:        domain.ProviderTypePrometheus,
		Name:        "prom",
		URL:         "http://prom",
		Credentials: "initial-secret",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := useCase.Update(context.Background(), created.ID, ProviderConfigRequest{
		Type: domain.ProviderTypePrometheus,
		Name: "prom-renamed",
		URL:  "http://prom-other",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.CredentialsCiphertext != created.CredentialsCiphertext {
		t.Fatalf("credentials should be preserved on update without payload, got %q", updated.CredentialsCiphertext)
	}
}

func TestProviderConfigTestRunsTester(t *testing.T) {
	repo := newStubProviderConfigRepo()
	tester := stubProviderTester{observability: ports.ProviderTestResult{Reached: true, LatencyMs: 42}}
	useCase := NewProviderConfig(repo, stubCipher{}, tester, &sequentialIDs{})
	created, _ := useCase.Create(context.Background(), ProviderConfigRequest{
		Type:        domain.ProviderTypePrometheus,
		Name:        "prom",
		URL:         "http://prom",
		Credentials: "secret",
	})
	result, err := useCase.Test(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if !result.Reached || result.LatencyMs != 42 {
		t.Fatalf("unexpected test result: %+v", result)
	}
}

func TestProviderConfigActivateAndDeactivateTogglesFlag(t *testing.T) {
	repo := newStubProviderConfigRepo()
	useCase := NewProviderConfig(repo, stubCipher{}, stubProviderTester{}, &sequentialIDs{})
	created, _ := useCase.Create(context.Background(), ProviderConfigRequest{
		Type: domain.ProviderTypePrometheus,
		Name: "prom",
		URL:  "http://prom",
	})
	active, err := useCase.Activate(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if !active.IsActive {
		t.Fatalf("expected provider to become active")
	}
	deactivated, err := useCase.Deactivate(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if deactivated.IsActive {
		t.Fatalf("expected provider to become inactive")
	}
}
