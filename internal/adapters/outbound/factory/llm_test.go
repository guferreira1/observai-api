package factory

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/guferreira1/observai-api/internal/platform/config"
)

func TestBuildLLMAcceptsOpenAICompatibleAliasWithOptionalAuth(t *testing.T) {
	result, err := BuildLLM(config.Config{
		LLM: config.LLMConfig{
			Providers: []config.LLMProviderConfig{{
				Type:    "openai-compatible",
				Name:    "self-hosted-openai",
				BaseURL: "http://llm-gateway:8080/v1",
				Model:   "qwen2.5-coder",
				Options: map[string]string{"auth": "optional"},
			}},
		},
	}, Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	if err != nil {
		t.Fatalf("BuildLLM: %v", err)
	}
	if result.Provider != "self-hosted-openai" {
		t.Fatalf("expected provider name, got %q", result.Provider)
	}
	if result.Clients.OpenAI == nil {
		t.Fatal("expected openai-compatible client")
	}
}

func TestBuildLLMRequiresOpenAICompatibleKeyByDefault(t *testing.T) {
	_, err := BuildLLM(config.Config{
		LLM: config.LLMConfig{
			Providers: []config.LLMProviderConfig{{
				Type:    "openai-compatible",
				Name:    "hosted-openai",
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-4o-mini",
			}},
		},
	}, Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	if err == nil {
		t.Fatal("expected missing api key error")
	}
	if !strings.Contains(err.Error(), "requires api_key_env") {
		t.Fatalf("unexpected error: %v", err)
	}
}
