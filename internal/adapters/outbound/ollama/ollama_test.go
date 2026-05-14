package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubLoader struct {
	prompt string
	err    error
}

func (loader stubLoader) Load(string) (string, error) {
	return loader.prompt, loader.err
}

func TestAnalysisGeneratorDecodesStructuredResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/chat", request.URL.Path)

		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)

		var payload chatPayload
		require.NoError(t, json.Unmarshal(body, &payload))
		assert.Equal(t, "json", payload.Format)
		require.Len(t, payload.Messages, 2)
		assert.Equal(t, "system", payload.Messages[0].Role)
		assert.Equal(t, "system body", payload.Messages[0].Content)
		assert.Contains(t, payload.Messages[1].Content, "checkout-service")

		_, _ = writer.Write([]byte(`{"message":{"role":"assistant","content":"{\"summary\":\"latency increased\",\"severity\":\"high\",\"confidence\":\"medium\",\"affectedServices\":[\"checkout-service\"],\"detectedAnomalies\":[\"p95 spike\"],\"possibleRootCauses\":[{\"cause\":\"db saturation\",\"evidence\":[\"p95_latency\"],\"confidence\":\"medium\"}],\"recommendedActions\":[{\"action\":\"scale db\",\"rationale\":\"reduce queue\",\"priority\":1}],\"codeLevelInsights\":[],\"missingEvidence\":[\"app logs\"]}"},"done":true}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Model: "llama3", Timeout: 2 * time.Second})
	require.NoError(t, err)

	generator := NewAnalysisGenerator(client, stubLoader{prompt: "system body"})
	result, err := generator.Generate(context.Background(), domain.AnalysisRequest{
		Goal:             "investigate checkout latency",
		AffectedServices: []string{"checkout-service"},
	}, []domain.Evidence{{Name: "p95_latency", Service: "checkout-service"}})
	require.NoError(t, err)

	assert.Equal(t, domain.SeverityHigh, result.Severity)
	assert.Equal(t, []string{"checkout-service"}, result.AffectedServices)
	require.Len(t, result.PossibleRootCauses, 1)
	assert.Equal(t, "db saturation", result.PossibleRootCauses[0].Cause)
	require.Len(t, result.Evidence, 1)
	assert.Equal(t, "p95_latency", result.Evidence[0].Name)
}

func TestChatResponderDecodesStructuredResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"message":{"role":"assistant","content":"{\"answer\":\"db saturation\",\"evidence\":[\"p95_latency\"]}"},"done":true}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Model: "llama3"})
	require.NoError(t, err)

	responder := NewChatResponder(client, stubLoader{prompt: "chat system"})
	answer, err := responder.Answer(context.Background(), domain.AnalysisContext{AnalysisID: "analysis-1"}, domain.ChatQuestion{
		AnalysisID: "analysis-1",
		Question:   "Which evidence supports the analysis?",
	})
	require.NoError(t, err)
	assert.Equal(t, "analysis-1", answer.AnalysisID)
	assert.Equal(t, "db saturation", answer.Answer)
	assert.Equal(t, []string{"p95_latency"}, answer.Evidence)
}

func TestClientChatRetriesOn5xx(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			_, _ = writer.Write([]byte(`{"error":"upstream down"}`))
			return
		}
		_, _ = writer.Write([]byte(`{"message":{"role":"assistant","content":"hello"},"done":true}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Model: "llama3"})
	require.NoError(t, err)

	content, err := client.Chat(context.Background(), ChatRequest{
		System:   "ignored",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", content)
	assert.Equal(t, int32(2), calls.Load())
}

func TestClientChatDoesNotRetryOnClientError(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":"bad payload"}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, Model: "llama3"})
	require.NoError(t, err)

	_, err = client.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}
