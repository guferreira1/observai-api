package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientChatSendsAPIKeyHeaderAndAnthropicVersion(t *testing.T) {
	var capturedKey, capturedVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("x-api-key")
		capturedVersion = r.Header.Get("anthropic-version")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"answer\":\"ok\",\"evidence\":[]}"}]}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{
		BaseURL: server.URL,
		APIKey:  "anth-key",
		Model:   "claude-sonnet-4-6",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	content, err := client.Chat(context.Background(), ChatRequest{
		Messages:   []Message{{Role: "user", Content: "hi"}},
		JSONOutput: true,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !strings.Contains(content, "ok") {
		t.Fatalf("unexpected content: %q", content)
	}
	if capturedKey != "anth-key" {
		t.Fatalf("expected x-api-key header, got %q", capturedKey)
	}
	if capturedVersion != DefaultAnthropicVersion {
		t.Fatalf("expected anthropic-version %q, got %q", DefaultAnthropicVersion, capturedVersion)
	}
}

func TestClientChatPropagatesSystemPromptInPayload(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
	}))
	defer server.Close()

	client, _ := NewClient(ClientOptions{
		BaseURL: server.URL,
		APIKey:  "k",
		Model:   "claude-sonnet-4-6",
	})
	_, _ = client.Chat(context.Background(), ChatRequest{
		System:   "be brief",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})

	system, _ := captured["system"].(string)
	if !strings.Contains(system, "be brief") {
		t.Fatalf("expected system field to contain prompt, got %q", system)
	}
}

func TestNewClientFailsWithoutAPIKey(t *testing.T) {
	_, err := NewClient(ClientOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error when api key is missing")
	}
}

func TestNewClientFailsWithoutModel(t *testing.T) {
	_, err := NewClient(ClientOptions{APIKey: "k"})
	if err == nil {
		t.Fatal("expected error when model is missing")
	}
}
