// Package ollama implements LLM adapters backed by an Ollama HTTP server.
//
// The package targets the /api/chat endpoint with structured (json) output so
// downstream code can deserialize responses without ad-hoc parsing. Each call
// applies the configured timeout and retries transient failures with bounded
// exponential backoff and full jitter.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/guferreira1/observai-api/internal/platform/retry"
)

// ClientOptions configures Ollama HTTP behavior.
type ClientOptions struct {
	BaseURL     string
	Model       string
	Timeout     time.Duration
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// Client is a minimal Ollama HTTP client tailored for chat completions.
type Client struct {
	baseURL     *url.URL
	model       string
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// NewClient validates the base URL and returns a configured Ollama client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse ollama base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("ollama base url must include scheme and host")
	}
	if opts.Model == "" {
		return nil, errors.New("ollama model is required")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	observer := opts.Observer
	if observer == nil {
		observer = observability.NoopProviderObserver{}
	}

	retryPolicy := opts.RetryPolicy
	if retryPolicy.MaxAttempts <= 0 {
		retryPolicy = retry.Default()
	}

	return &Client{
		baseURL:     parsed,
		model:       opts.Model,
		httpClient:  &http.Client{Timeout: timeout},
		observer:    observer,
		retryPolicy: retryPolicy,
	}, nil
}

// Message describes a single chat message exchanged with the model.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest describes the inputs for an Ollama chat completion call.
type ChatRequest struct {
	System         string
	Messages       []Message
	JSONOutput     bool
	TemperatureSet bool
	Temperature    float64
}

type chatPayload struct {
	Model    string            `json:"model"`
	Messages []Message         `json:"messages"`
	Stream   bool              `json:"stream"`
	Format   string            `json:"format,omitempty"`
	Options  map[string]any    `json:"options,omitempty"`
	Metadata map[string]string `json:"-"`
}

type chatResponse struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
	Error   string  `json:"error"`
}

// Ping verifies the Ollama server is reachable and accepting requests.
//
// It hits the GET /api/tags endpoint, which Ollama exposes for model listing
// and which does not exercise any heavy LLM inference path.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() {
		client.observer.Observe("ollama", "ping", time.Since(startedAt), err)
	}()

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/api/tags")

	request, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if reqErr != nil {
		return fmt.Errorf("build ollama health request: %w", reqErr)
	}

	response, doErr := client.httpClient.Do(request)
	if doErr != nil {
		return fmt.Errorf("call ollama health: %w", doErr)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("ollama health returned status %d", response.StatusCode)
	}
	return nil
}

// Chat performs an Ollama chat completion. Transient network failures and 5xx
// responses are retried using bounded exponential backoff with full jitter,
// governed by the client retry policy. The per-attempt deadline is bounded by
// the client timeout.
func (client *Client) Chat(ctx context.Context, request ChatRequest) (content string, err error) {
	startedAt := time.Now()
	defer func() {
		client.observer.Observe("ollama", "chat", time.Since(startedAt), err)
	}()

	messages := make([]Message, 0, len(request.Messages)+1)
	if request.System != "" {
		messages = append(messages, Message{Role: "system", Content: request.System})
	}
	messages = append(messages, request.Messages...)

	payload := chatPayload{
		Model:    client.model,
		Messages: messages,
		Stream:   false,
	}
	if request.JSONOutput {
		payload.Format = "json"
	}
	if request.TemperatureSet {
		payload.Options = map[string]any{"temperature": request.Temperature}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode ollama payload: %w", err)
	}

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/api/chat")

	err = retry.Do(ctx, client.retryPolicy, isRetryable, func(int) error {
		response, attemptErr := client.do(ctx, endpoint.String(), body)
		if attemptErr != nil {
			return attemptErr
		}
		content = response
		return nil
	})
	if err != nil {
		return "", err
	}
	return content, nil
}

func (client *Client) do(ctx context.Context, endpoint string, body []byte) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build ollama request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return "", &transientError{err: fmt.Errorf("call ollama: %w", err)}
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", &transientError{err: fmt.Errorf("read ollama response: %w", err)}
	}

	if response.StatusCode >= http.StatusInternalServerError {
		return "", &transientError{err: fmt.Errorf("ollama status %d: %s", response.StatusCode, truncate(string(raw), 200))}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("ollama status %d: %s", response.StatusCode, truncate(string(raw), 200))
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	if parsed.Error != "" {
		return "", fmt.Errorf("ollama error: %s", parsed.Error)
	}

	return parsed.Message.Content, nil
}

type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var transient *transientError
	return errors.As(err, &transient)
}

func joinPath(base, suffix string) string {
	if base == "" {
		return suffix
	}
	if base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	if suffix == "" {
		return base
	}
	if suffix[0] != '/' {
		return base + "/" + suffix
	}
	return base + suffix
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
