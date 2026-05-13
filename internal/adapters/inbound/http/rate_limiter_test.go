package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/fake"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiterAllowsBurstThenBlocks(t *testing.T) {
	t.Parallel()

	limiter := newRateLimiter(RateLimitConfig{RequestsPerSecond: 1, Burst: 2})
	require.NotNil(t, limiter)

	assert.True(t, limiter.Allow("10.0.0.1"))
	assert.True(t, limiter.Allow("10.0.0.1"))
	assert.False(t, limiter.Allow("10.0.0.1"))
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	t.Parallel()

	limiter := newRateLimiter(RateLimitConfig{RequestsPerSecond: 1, Burst: 1})
	require.NotNil(t, limiter)

	current := time.Now()
	limiter.now = func() time.Time { return current }

	assert.True(t, limiter.Allow("10.0.0.2"))
	assert.False(t, limiter.Allow("10.0.0.2"))

	current = current.Add(2 * time.Second)
	assert.True(t, limiter.Allow("10.0.0.2"))
}

func TestRateLimiterFailsOpenWhenKeyEmpty(t *testing.T) {
	t.Parallel()

	limiter := newRateLimiter(RateLimitConfig{RequestsPerSecond: 0.1, Burst: 1})
	require.NotNil(t, limiter)
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.Allow(""))
	}
}

func TestNewRateLimiterReturnsNilWhenDisabled(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newRateLimiter(RateLimitConfig{}))
	assert.Nil(t, newRateLimiter(RateLimitConfig{RequestsPerSecond: 0, Burst: 5}))
	assert.Nil(t, newRateLimiter(RateLimitConfig{RequestsPerSecond: 5, Burst: 0}))
}

func TestRouterEnforcesRateLimit(t *testing.T) {
	t.Parallel()

	repository := fake.NewAnalysisRepository()
	analysis := usecase.NewAnalysis(
		fake.NewSignalCollector(),
		fake.NewAnalysisGenerator(),
		repository,
		fake.NewAnalysisContextCache(),
		6*time.Hour,
		fake.NewIDGenerator("analysis"),
	)
	chat := usecase.NewChat(repository, fake.NewAnalysisContextCache(), 6*time.Hour, repository, fake.NewChatResponder())

	router := NewRouter(analysis, chat, RouterOptions{
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout:     5 * time.Second,
		MaxRequestBodyByte: 1 << 20,
		RateLimit:          RateLimitConfig{RequestsPerSecond: 1, Burst: 1},
	})

	first := httptest.NewRequest(stdhttp.MethodGet, "/health", nil)
	first.RemoteAddr = "203.0.113.1:1000"
	firstResponse := httptest.NewRecorder()
	router.ServeHTTP(firstResponse, first)
	require.Equal(t, stdhttp.StatusOK, firstResponse.Code)

	second := httptest.NewRequest(stdhttp.MethodGet, "/health", nil)
	second.RemoteAddr = "203.0.113.1:1001"
	secondResponse := httptest.NewRecorder()
	router.ServeHTTP(secondResponse, second)
	require.Equal(t, stdhttp.StatusTooManyRequests, secondResponse.Code)

	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(secondResponse.Body.Bytes(), &payload))
	assert.Equal(t, "rate_limited", payload.Data.Code)
}

func TestClientIPStripsPort(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(stdhttp.MethodGet, "/", bytes.NewBufferString(""))
	request.RemoteAddr = "192.0.2.1:54321"
	assert.Equal(t, "192.0.2.1", clientIP(request))

	request.RemoteAddr = "[2001:db8::1]:54321"
	assert.Equal(t, "2001:db8::1", clientIP(request))

	request.RemoteAddr = ""
	assert.Equal(t, "", clientIP(request))
}
