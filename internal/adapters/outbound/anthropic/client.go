// Package anthropic implements LLM adapters backed by Anthropic's
// /v1/messages API.
//
// Authentication uses x-api-key (Anthropic does not accept Bearer). The
// API requires an explicit anthropic-version header; the client pins a
// known-good version so upstream changes do not break clients silently.
// JSON output is requested via a strict system suffix because /v1/messages
// does not yet support OpenAI-style response_format=json_object.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/guferreira1/observai-api/internal/platform/retry"
)

// DefaultAnthropicVersion is the API version header value the client sends.
const DefaultAnthropicVersion = "2023-06-01"

// ClientOptions configures the Anthropic HTTP client.
type ClientOptions struct {
	BaseURL         string
	APIKey          string
	Model           string
	Version         string
	MaxOutputTokens int
	Timeout         time.Duration
	Observer        observability.ProviderObserver
	RetryPolicy     retry.Policy
}

// Client talks to Anthropic's Messages API.
type Client struct {
	baseURL         *url.URL
	apiKey          string
	model           string
	version         string
	maxOutputTokens int
	httpClient      *http.Client
	observer        observability.ProviderObserver
	retryPolicy     retry.Policy
}

// NewClient validates the supplied options and returns a ready-to-use client.
func NewClient(opts ClientOptions) (*Client, error) {
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse anthropic base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("anthropic base url must include scheme and host")
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("anthropic api key is required")
	}
	if strings.TrimSpace(opts.Model) == "" {
		return nil, errors.New("anthropic model is required")
	}

	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = DefaultAnthropicVersion
	}

	maxTokens := opts.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
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
		baseURL:         parsed,
		apiKey:          opts.APIKey,
		model:           opts.Model,
		version:         version,
		maxOutputTokens: maxTokens,
		httpClient:      &http.Client{Timeout: timeout},
		observer:        observer,
		retryPolicy:     retryPolicy,
	}, nil
}

// Message describes a single conversation turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest describes the inputs of an Anthropic messages call.
type ChatRequest struct {
	System         string
	Messages       []Message
	JSONOutput     bool
	TemperatureSet bool
	Temperature    float64
}

type messagesPayload struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	Messages    []messagesPayloadM `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type messagesPayloadM struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Ping verifies connectivity. Anthropic does not expose a free probe so the
// client issues a 1-token completion against the configured model; this is
// the smallest verifiable call without overcharging the operator.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("anthropic", "ping", time.Since(startedAt), err) }()

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/messages")

	payload := messagesPayload{
		Model:     client.model,
		MaxTokens: 1,
		Messages:  []messagesPayloadM{{Role: "user", Content: "ping"}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode anthropic ping payload: %w", err)
	}

	_, err = client.do(ctx, endpoint.String(), body)
	return err
}

// Chat performs a Messages API call and returns the assistant's first text
// block. JSON output is enforced through a strict system suffix when
// requested; failures (network, 429, 5xx) are retried.
func (client *Client) Chat(ctx context.Context, request ChatRequest) (content string, err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("anthropic", "messages", time.Since(startedAt), err) }()

	system := request.System
	if request.JSONOutput {
		const jsonSuffix = "\n\nReturn ONLY a JSON object with no surrounding prose, no markdown fences and no commentary."
		if system == "" {
			system = strings.TrimSpace(jsonSuffix)
		} else if !strings.Contains(system, "Return ONLY a JSON") {
			system = system + jsonSuffix
		}
	}

	messages := make([]messagesPayloadM, 0, len(request.Messages))
	for _, message := range request.Messages {
		messages = append(messages, messagesPayloadM{Role: message.Role, Content: message.Content})
	}

	payload := messagesPayload{
		Model:     client.model,
		System:    system,
		Messages:  messages,
		MaxTokens: client.maxOutputTokens,
	}
	if request.TemperatureSet {
		temperature := request.Temperature
		payload.Temperature = &temperature
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode anthropic payload: %w", err)
	}

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/messages")

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
		return "", fmt.Errorf("build anthropic request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("x-api-key", client.apiKey)
	request.Header.Set("anthropic-version", client.version)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return "", &transientError{err: fmt.Errorf("call anthropic: %w", err)}
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", &transientError{err: fmt.Errorf("read anthropic response: %w", err)}
	}

	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		return "", &transientError{err: fmt.Errorf("anthropic status %d: %s", response.StatusCode, truncate(string(raw), 200))}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("anthropic status %d: %s", response.StatusCode, truncate(string(raw), 200))
	}

	var parsed messagesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode anthropic response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}
	for _, block := range parsed.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic returned no text content")
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
