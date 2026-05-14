package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckerReturnsOkWhenAllProbesSucceed(t *testing.T) {
	t.Parallel()

	checker := NewChecker(time.Second,
		ProbeFunc{ProbeName: "first", Fn: func(context.Context) error { return nil }},
		ProbeFunc{ProbeName: "second", Fn: func(context.Context) error { return nil }},
	)

	result := checker.Run(context.Background())
	assert.Equal(t, StatusOk, result.Status)
	require.Len(t, result.Checks, 2)
	for _, check := range result.Checks {
		assert.Equal(t, StatusOk, check.Status)
		assert.Empty(t, check.Error)
	}
}

func TestCheckerFlipsToFailedWhenAnyProbeReturnsError(t *testing.T) {
	t.Parallel()

	checker := NewChecker(time.Second,
		ProbeFunc{ProbeName: "ok", Fn: func(context.Context) error { return nil }},
		ProbeFunc{ProbeName: "broken", Fn: func(context.Context) error { return errors.New("boom") }},
	)

	result := checker.Run(context.Background())
	assert.Equal(t, StatusFailed, result.Status)
	require.Len(t, result.Checks, 2)
	failed := result.Checks[1]
	assert.Equal(t, "broken", failed.Name)
	assert.Equal(t, StatusFailed, failed.Status)
	assert.Equal(t, "boom", failed.Error)
}

func TestReadinessHandlerReturns503OnFailure(t *testing.T) {
	t.Parallel()

	checker := NewChecker(time.Second,
		ProbeFunc{ProbeName: "broken", Fn: func(context.Context) error { return errors.New("boom") }},
	)
	server := httptest.NewServer(ReadinessHandler(checker))
	defer server.Close()

	response, err := http.Get(server.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
}

func TestLivenessHandlerAlwaysReturns200(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(LivenessHandler())
	defer server.Close()

	response, err := http.Get(server.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
}
