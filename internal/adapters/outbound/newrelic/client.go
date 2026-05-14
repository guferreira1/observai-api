// Package newrelic implements a read-only New Relic signal collector.
//
// The adapter calls the GraphQL endpoint at /graphql with an NRQL query
// scoped to a single account. Authentication uses the "API-Key" header
// holding a User API key.
package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/guferreira1/observai-api/internal/platform/retry"
)

// Client is a minimal New Relic HTTP client.
type Client struct {
	baseURL     *url.URL
	apiKey      string
	accountID   string
	httpClient  *http.Client
	observer    observability.ProviderObserver
	retryPolicy retry.Policy
}

// ClientOptions configures New Relic HTTP behavior.
//
// AccountID is required so the NRQL query can target the right account.
type ClientOptions struct {
	BaseURL     string
	APIKey      string
	AccountID   string
	Timeout     time.Duration
	Observer    observability.ProviderObserver
	RetryPolicy retry.Policy
}

// ErrUnauthorized indicates the supplied API key was rejected.
var ErrUnauthorized = errors.New("newrelic api key unauthorized")

// ErrAccountMissing indicates the account id was not supplied.
var ErrAccountMissing = errors.New("newrelic account id is required")

// NewClient validates the base URL and returns a configured New Relic client.
func NewClient(opts ClientOptions) (*Client, error) {
	parsed, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse newrelic base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("newrelic base url must include scheme and host")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
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
		apiKey:      opts.APIKey,
		accountID:   strings.TrimSpace(opts.AccountID),
		httpClient:  &http.Client{Timeout: timeout},
		observer:    observer,
		retryPolicy: retryPolicy,
	}, nil
}

// Ping issues a minimal GraphQL query to validate credentials.
func (client *Client) Ping(ctx context.Context) (err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("newrelic", "ping", time.Since(startedAt), err) }()

	_, err = client.executeGraphQL(ctx, `{ actor { user { email } } }`)
	return err
}

// NRQLSample is a single row returned by an NRQL query.
type NRQLSample struct {
	Service  string
	Value    float64
	Label    string
	Observed time.Time
}

// QueryNRQL runs an NRQL query and returns the flattened sample list.
//
// The query is wrapped in a GraphQL `actor.account.nrql` field so the
// caller only supplies the NRQL fragment. {service} substitution is the
// collector's responsibility.
func (client *Client) QueryNRQL(ctx context.Context, nrql string) (samples []NRQLSample, err error) {
	startedAt := time.Now()
	defer func() { client.observer.Observe("newrelic", "query_nrql", time.Since(startedAt), err) }()

	if client.accountID == "" {
		return nil, ErrAccountMissing
	}
	graphql := fmt.Sprintf(
		`{ actor { account(id: %s) { nrql(query: %s) { results } } } }`,
		client.accountID,
		strconv.Quote(nrql),
	)
	body, err := client.executeGraphQL(ctx, graphql)
	if err != nil {
		return nil, err
	}
	return parseNRQLResponse(body)
}

func (client *Client) executeGraphQL(ctx context.Context, query string) ([]byte, error) {
	envelope := map[string]string{"query": query}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode newrelic graphql query: %w", err)
	}

	var body []byte
	err = retry.Do(ctx, client.retryPolicy, isRetryableNewRelic, func(int) error {
		request, requestErr := client.newRequest(ctx, http.MethodPost, "/graphql", bytes.NewReader(payload))
		if requestErr != nil {
			return requestErr
		}
		request.Header.Set("Content-Type", "application/json")
		response, doErr := client.httpClient.Do(request)
		if doErr != nil {
			return &transientError{err: fmt.Errorf("newrelic graphql: %w", doErr)}
		}
		defer response.Body.Close()
		raw, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return &transientError{err: fmt.Errorf("read newrelic response: %w", readErr)}
		}
		switch {
		case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
			return ErrUnauthorized
		case response.StatusCode >= 500:
			return &transientError{err: fmt.Errorf("newrelic returned status %d", response.StatusCode)}
		case response.StatusCode >= 400:
			return fmt.Errorf("newrelic returned status %d", response.StatusCode)
		}
		body = raw
		return nil
	})
	return body, err
}

func (client *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	target := *client.baseURL
	target.Path = joinPath(target.Path, path)
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build newrelic request: %w", err)
	}
	if client.apiKey != "" {
		request.Header.Set("API-Key", client.apiKey)
	}
	request.Header.Set("Accept", "application/json")
	return request, nil
}

type graphqlResponse struct {
	Data struct {
		Actor struct {
			Account struct {
				NRQL struct {
					Results []map[string]any `json:"results"`
				} `json:"nrql"`
			} `json:"account"`
		} `json:"actor"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func parseNRQLResponse(payload []byte) ([]NRQLSample, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	var decoded graphqlResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("decode newrelic response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		return nil, fmt.Errorf("newrelic returned error: %s", decoded.Errors[0].Message)
	}
	samples := make([]NRQLSample, 0, len(decoded.Data.Actor.Account.NRQL.Results))
	for _, result := range decoded.Data.Actor.Account.NRQL.Results {
		sample := NRQLSample{
			Service: stringFromMap(result, "service.name", "service", "appName"),
			Label:   stringFromMap(result, "facet", "name"),
		}
		sample.Value = floatFromMap(result, "average", "value", "count", "rate")
		if timestamp, ok := result["timestamp"].(float64); ok {
			sample.Observed = time.UnixMilli(int64(timestamp)).UTC()
		}
		samples = append(samples, sample)
	}
	return samples, nil
}

func stringFromMap(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			if s, isString := raw.(string); isString && s != "" {
				return s
			}
		}
	}
	return ""
}

func floatFromMap(values map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			if value, isFloat := raw.(float64); isFloat {
				return value
			}
		}
	}
	return 0
}

func joinPath(left, right string) string {
	if left == "" || left == "/" {
		return right
	}
	if right == "" {
		return left
	}
	return strings.TrimRight(left, "/") + "/" + strings.TrimLeft(right, "/")
}

type transientError struct{ err error }

func (transient *transientError) Error() string     { return transient.err.Error() }
func (transient *transientError) Unwrap() error     { return transient.err }
func (transient *transientError) IsTransient() bool { return true }

func isRetryableNewRelic(err error) bool {
	var transient *transientError
	return errors.As(err, &transient)
}
