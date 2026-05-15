package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

type stubLLMConfigRepo struct {
	mu      sync.Mutex
	configs map[string]domain.LLMConfig
}

func newStubLLMConfigRepo() *stubLLMConfigRepo {
	return &stubLLMConfigRepo{configs: make(map[string]domain.LLMConfig)}
}

func (stub *stubLLMConfigRepo) Create(_ context.Context, config domain.LLMConfig) error {
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

func (stub *stubLLMConfigRepo) Find(_ context.Context, id string) (domain.LLMConfig, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	config, ok := stub.configs[id]
	if !ok {
		return domain.LLMConfig{}, domain.ErrLLMConfigNotFound
	}
	return config, nil
}

func (stub *stubLLMConfigRepo) List(_ context.Context, _, _ int) ([]domain.LLMConfig, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	out := make([]domain.LLMConfig, 0, len(stub.configs))
	for _, config := range stub.configs {
		out = append(out, config)
	}
	return out, nil
}

func (stub *stubLLMConfigRepo) FindActive(_ context.Context) (domain.LLMConfig, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	for _, config := range stub.configs {
		if config.IsActive {
			return config, nil
		}
	}
	return domain.LLMConfig{}, domain.ErrLLMConfigNotFound
}

func (stub *stubLLMConfigRepo) Count(_ context.Context) (int64, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return int64(len(stub.configs)), nil
}

func (stub *stubLLMConfigRepo) Update(_ context.Context, config domain.LLMConfig) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.configs[config.ID] = config
	return nil
}

func (stub *stubLLMConfigRepo) Activate(_ context.Context, id string, _ time.Time) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if _, ok := stub.configs[id]; !ok {
		return domain.ErrLLMConfigNotFound
	}
	for otherID, other := range stub.configs {
		other.IsActive = otherID == id
		stub.configs[otherID] = other
	}
	return nil
}

func (stub *stubLLMConfigRepo) Delete(_ context.Context, id string) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	delete(stub.configs, id)
	return nil
}

func TestLLMConfigCreateValidatesInput(t *testing.T) {
	useCase := NewLLMConfig(newStubLLMConfigRepo(), stubCipher{}, stubProviderTester{}, &sequentialIDs{})
	cases := []LLMConfigRequest{
		{Type: domain.LLMProviderTypeOpenAI, Name: "", BaseURL: "http://x", Model: "gpt"},
		{Type: domain.LLMProviderTypeOpenAI, Name: "x", BaseURL: "", Model: "gpt"},
		{Type: domain.LLMProviderTypeOpenAI, Name: "x", BaseURL: "http://x", Model: ""},
		{Type: "bogus", Name: "x", BaseURL: "http://x", Model: "gpt"},
	}
	for _, request := range cases {
		if _, err := useCase.Create(context.Background(), request); !errors.Is(err, domain.ErrInvalidLLMConfig) {
			t.Fatalf("expected ErrInvalidLLMConfig for %+v, got %v", request, err)
		}
	}
}

func TestLLMConfigCreateNormalizesOpenAICompatibleAlias(t *testing.T) {
	useCase := NewLLMConfig(newStubLLMConfigRepo(), stubCipher{}, stubProviderTester{}, &sequentialIDs{})

	created, err := useCase.Create(context.Background(), LLMConfigRequest{
		Type:    domain.LLMProviderType("openai-compatible"),
		Name:    "self-hosted-openai",
		BaseURL: "http://llm-gateway:8080/v1",
		Model:   "qwen2.5-coder",
		Options: map[string]string{"auth": "optional"},
	})

	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Type != domain.LLMProviderTypeOpenAI {
		t.Fatalf("expected canonical openai type, got %q", created.Type)
	}
}

func TestLLMConfigActivateIsMutuallyExclusive(t *testing.T) {
	repo := newStubLLMConfigRepo()
	useCase := NewLLMConfig(repo, stubCipher{}, stubProviderTester{}, &sequentialIDs{})
	first, err := useCase.Create(context.Background(), LLMConfigRequest{
		Type:     domain.LLMProviderTypeOllama,
		Name:     "primary",
		BaseURL:  "http://ollama:11434",
		Model:    "llama3",
		IsActive: true,
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := useCase.Create(context.Background(), LLMConfigRequest{
		Type:    domain.LLMProviderTypeOpenAI,
		Name:    "secondary",
		BaseURL: "https://api.openai.com",
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := useCase.Activate(context.Background(), second.ID); err != nil {
		t.Fatalf("activate second: %v", err)
	}
	stored, _ := repo.Find(context.Background(), first.ID)
	if stored.IsActive {
		t.Fatalf("first config should have been deactivated")
	}
	storedSecond, _ := repo.Find(context.Background(), second.ID)
	if !storedSecond.IsActive {
		t.Fatalf("second config should be active")
	}
}

func TestLLMConfigUpdatePreservesAPIKeyWhenEmpty(t *testing.T) {
	repo := newStubLLMConfigRepo()
	useCase := NewLLMConfig(repo, stubCipher{}, stubProviderTester{}, &sequentialIDs{})
	created, err := useCase.Create(context.Background(), LLMConfigRequest{
		Type:    domain.LLMProviderTypeOpenAI,
		Name:    "openai",
		BaseURL: "https://api.openai.com",
		Model:   "gpt-4o-mini",
		APIKey:  "sk-original",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := useCase.Update(context.Background(), created.ID, LLMConfigRequest{
		Type:    domain.LLMProviderTypeOpenAI,
		Name:    "openai-renamed",
		BaseURL: "https://api.openai.com",
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.APIKeyCipher != created.APIKeyCipher {
		t.Fatalf("api key should be preserved when not supplied: %q vs %q", updated.APIKeyCipher, created.APIKeyCipher)
	}
}
