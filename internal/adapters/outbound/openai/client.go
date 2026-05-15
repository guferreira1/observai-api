// Package openai implements LLM adapters compatible with OpenAI's
// /v1/chat/completions API.
//
// The same client also serves OpenAI-compatible providers such as Azure
// OpenAI (point BaseURL at the Azure resource), OpenRouter and any
// self-hosted gateway that mirrors the OpenAI schema. The adapter
// authenticates with a bearer token, requests JSON output via
// response_format and retries transient failures with bounded exponential
// backoff.
package openai

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

// ClientOptions configures the OpenAI-compatible HTTP client.
type ClientOptions struct {
	BaseURL          string
	APIKey           string
	AllowEmptyAPIKey bool
	Model            string
	Timeout          time.Duration
	Observer         observability.ProviderObserver
	RetryPolicy      retry.Policy
}

// Client talks to an OpenAI-compatible Chat Completions endpoint.
type Client struct {
	baseURL     *url.URL
	apiKey      string
	model       string
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// NewClient validates the supplied options and returns a ready-to-use client.
func NewClient(opts ClientOptions) (*Client, error) {
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse openai base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("openai base url must include scheme and host")
	}
	apiKey := strings.TrimSpace(opts.APIKey)
	if apiKey == "" && !opts.AllowEmptyAPIKey {
		return nil, errors.New("openai api key is required")
	}
	if strings.TrimSpace(opts.Model) == "" {
		return nil, errors.New("openai model is required")
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
		baseURL:     parsed,
		apiKey:      apiKey,
		model:       opts.Model,
		httpClient:  &http.Client{Timeout: timeout},
		observer:    observer,
		retryPolicy: retryPolicy,
	}, nil
}

// Message describes a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest describes the inputs of a chat completion call.
type ChatRequest struct {
	System         string
	Messages       []Message
	JSONOutput     bool
	TemperatureSet bool
	Temperature    float64
}

type chatPayload struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	ResponseFormat *responseFmt   `json:"response_format,omitempty"`
	Temperature    *float64       `json:"temperature,omitempty"`
	Stream         bool           `json:"stream"`
	Options        map[string]any `json:"options,omitempty"`
}

type responseFmt struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Ping issues a low-cost GET /models call to verify reachability and
// authentication. It does not consume tokens.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("openai", "ping", time.Since(startedAt), err) }()

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/models")

	request, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if reqErr != nil {
		return fmt.Errorf("build openai health request: %w", reqErr)
	}
	client.authorize(request)

	response, doErr := client.httpClient.Do(request)
	if doErr != nil {
		return fmt.Errorf("call openai health: %w", doErr)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("openai health returned status %d", response.StatusCode)
	}
	return nil
}

// Chat performs a chat completion call and returns the assistant content
// string. Transient failures (network errors, 5xx, 429) are retried.
func (client *Client) Chat(ctx context.Context, request ChatRequest) (content string, err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("openai", "chat", time.Since(startedAt), err) }()

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
		payload.ResponseFormat = &responseFmt{Type: "json_object"}
	}
	if request.TemperatureSet {
		temperature := request.Temperature
		payload.Temperature = &temperature
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode openai payload: %w", err)
	}

	endpoint := *client.baseURL
	endpoint.Path = joinPath(endpoint.Path, "/chat/completions")

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
		return "", fmt.Errorf("build openai request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client.authorize(request)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return "", &transientError{err: fmt.Errorf("call openai: %w", err)}
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", &transientError{err: fmt.Errorf("read openai response: %w", err)}
	}

	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		return "", &transientError{err: fmt.Errorf("openai status %d: %s", response.StatusCode, truncate(string(raw), 200))}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("openai status %d: %s", response.StatusCode, truncate(string(raw), 200))
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("openai error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai response contained no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (client *Client) authorize(request *http.Request) {
	if client.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+client.apiKey)
	}
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
