package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/testfakes"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/usecase"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterLogsCSRFErrorWithHTTPContext(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	signer, users, user := newJWTFixture(t)
	router := NewRouter(nil, nil, RouterOptions{
		Logger:         slog.New(slog.NewTextHandler(&logBuffer, nil)),
		RequestTimeout: 5 * time.Second,
		Auth: AuthConfig{
			Signer: signer,
			Users:  users,
		},
	})
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/admin/providers", bytes.NewBufferString(`{}`))
	request.AddCookie(&stdhttp.Cookie{Name: SessionCookieName, Value: issueAccessToken(t, signer, user)})
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusForbidden, response.Code)
	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "csrf_token_invalid", payload.Data.Code)

	logged := logBuffer.String()
	assert.Contains(t, logged, "http error")
	assert.Contains(t, logged, "component=http")
	assert.Contains(t, logged, "source=http.csrf")
	assert.Contains(t, logged, "operation=\"POST /v1/admin/providers\"")
	assert.Contains(t, logged, "method=POST")
	assert.Contains(t, logged, "path=/v1/admin/providers")
	assert.Contains(t, logged, "route=/v1/admin/providers")
	assert.Contains(t, logged, "code=csrf_token_invalid")
	assert.Contains(t, logged, "userId=user-1")
	assert.Contains(t, logged, "role=admin")
}

func TestRouterLogsAuthorizationErrorWithRequiredRole(t *testing.T) {
	t.Parallel()

	var logBuffer bytes.Buffer
	signer, err := crypto.NewJWTSigner(bytes.Repeat([]byte{0xab}, crypto.MinJWTSecretLength), "observai-api")
	require.NoError(t, err)
	users := inmemory.NewUserRepository()
	viewer := domain.User{
		ID:           "viewer-1",
		Name:         "Viewer User",
		Email:        "viewer@observai.io",
		PasswordHash: "hash",
		Role:         domain.RoleViewer,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	require.NoError(t, users.Create(context.Background(), viewer))

	router := NewRouter(newLoggingTestAnalysis(), nil, RouterOptions{
		Logger:         slog.New(slog.NewTextHandler(&logBuffer, nil)),
		RequestTimeout: 5 * time.Second,
		Auth: AuthConfig{
			Signer: signer,
			Users:  users,
		},
	})
	request := httptest.NewRequest(stdhttp.MethodPost, "/v1/analyses", bytes.NewBufferString(`{
		"goal": "investigate checkout latency",
		"timeWindow": {"start": "2026-05-12T10:00:00Z", "end": "2026-05-12T11:00:00Z"},
		"affectedServices": ["checkout-service"]
	}`))
	request.AddCookie(&stdhttp.Cookie{Name: SessionCookieName, Value: issueAccessToken(t, signer, viewer)})
	request.AddCookie(&stdhttp.Cookie{Name: CSRFCookieName, Value: "csrf-token"})
	request.Header.Set(CSRFHeaderName, "csrf-token")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, stdhttp.StatusForbidden, response.Code)
	var payload WrapperDtoResponde[ErrorResponse]
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, "forbidden", payload.Data.Code)
	require.NotEmpty(t, payload.Data.Details)
	assert.Equal(t, "role", payload.Data.Details[0].Field)
	assert.Equal(t, "required_role", payload.Data.Details[0].Rule)

	logged := logBuffer.String()
	assert.Contains(t, logged, "http error")
	assert.Contains(t, logged, "component=http")
	assert.Contains(t, logged, "source=http.authorization")
	assert.Contains(t, logged, "operation=\"POST /v1/analyses\"")
	assert.Contains(t, logged, "method=POST")
	assert.Contains(t, logged, "path=/v1/analyses")
	assert.Contains(t, logged, "route=/v1/analyses")
	assert.Contains(t, logged, "code=forbidden")
	assert.Contains(t, logged, "userId=viewer-1")
	assert.Contains(t, logged, "role=viewer")
	assert.Contains(t, logged, "required_role")
}

func newLoggingTestAnalysis() *usecase.Analysis {
	repository := inmemory.NewAnalysisRepository()
	return usecase.NewAnalysis(
		testfakes.NewSignalCollector(),
		testfakes.NewAnalysisGenerator(),
		repository,
		inmemory.NewAnalysisContextCache(),
		6*time.Hour,
		testfakes.NewIDGenerator("analysis"),
	)
}
