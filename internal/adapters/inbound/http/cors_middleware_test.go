package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	corsTestAllowedOrigin    = "https://app.example.com"
	corsTestDisallowedOrigin = "https://evil.example.com"
)

func newCORSTestHandler() stdhttp.Handler {
	return stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writer.WriteHeader(stdhttp.StatusOK)
	})
}

func TestCORSMiddlewareIsNoopWhenAllowedOriginsEmpty(t *testing.T) {
	t.Parallel()

	handler := corsMiddleware(nil)(newCORSTestHandler())

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/capabilities", nil)
	request.Header.Set("Origin", corsTestAllowedOrigin)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, stdhttp.StatusOK, recorder.Code)
	assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSMiddlewareEchoesAllowedOrigin(t *testing.T) {
	t.Parallel()

	handler := corsMiddleware([]string{corsTestAllowedOrigin})(newCORSTestHandler())

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/capabilities", nil)
	request.Header.Set("Origin", corsTestAllowedOrigin)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, stdhttp.StatusOK, recorder.Code)
	assert.Equal(t, corsTestAllowedOrigin, recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, recorder.Header().Values("Vary"), "Origin")
}

func TestCORSMiddlewareSkipsDisallowedOrigin(t *testing.T) {
	t.Parallel()

	handler := corsMiddleware([]string{corsTestAllowedOrigin})(newCORSTestHandler())

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/capabilities", nil)
	request.Header.Set("Origin", corsTestDisallowedOrigin)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, stdhttp.StatusOK, recorder.Code)
	assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddlewareHandlesPreflight(t *testing.T) {
	t.Parallel()

	handler := corsMiddleware([]string{corsTestAllowedOrigin})(newCORSTestHandler())

	request := httptest.NewRequest(stdhttp.MethodOptions, "/v1/analyses", nil)
	request.Header.Set("Origin", corsTestAllowedOrigin)
	request.Header.Set("Access-Control-Request-Method", stdhttp.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-csrf-token")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	require.Equal(t, stdhttp.StatusOK, recorder.Code)
	assert.Equal(t, corsTestAllowedOrigin, recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, recorder.Header().Get("Access-Control-Allow-Methods"), stdhttp.MethodPost)
	allowedHeaders := recorder.Header().Get("Access-Control-Allow-Headers")
	assert.Contains(t, allowedHeaders, "Authorization")
	assert.Contains(t, allowedHeaders, "X-Csrf-Token")
	assert.Equal(t, strconv.Itoa(corsPreflightMaxAgeSeconds), recorder.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddlewareRejectsPreflightFromDisallowedOrigin(t *testing.T) {
	t.Parallel()

	handler := corsMiddleware([]string{corsTestAllowedOrigin})(newCORSTestHandler())

	request := httptest.NewRequest(stdhttp.MethodOptions, "/v1/analyses", nil)
	request.Header.Set("Origin", corsTestDisallowedOrigin)
	request.Header.Set("Access-Control-Request-Method", stdhttp.MethodPost)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, recorder.Header().Get("Access-Control-Allow-Methods"))
}
