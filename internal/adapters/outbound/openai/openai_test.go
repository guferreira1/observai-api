package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientChatSendsBearerTokenAndJSONResponseFormat(t *testing.T) {
	var capturedAuth, capturedFormat string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if fmt, ok := body["response_format"].(map[string]any); ok {
			capturedFormat, _ = fmt["type"].(string)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"answer\":\"ok\",\"evidence\":[]}"}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-4o-mini",
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
	if capturedAuth != "Bearer test-key" {
		t.Fatalf("expected bearer auth header, got %q", capturedAuth)
	}
	if capturedFormat != "json_object" {
		t.Fatalf("expected json_object response_format, got %q", capturedFormat)
	}
}

func TestNewClientFailsWhenAPIKeyIsMissing(t *testing.T) {
	_, err := NewClient(ClientOptions{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini"})
	if err == nil {
		t.Fatal("expected error when api key is missing")
	}
}

func TestNewClientFailsWhenModelIsMissing(t *testing.T) {
	_, err := NewClient(ClientOptions{BaseURL: "https://api.openai.com/v1", APIKey: "k"})
	if err == nil {
		t.Fatal("expected error when model is missing")
	}
}
