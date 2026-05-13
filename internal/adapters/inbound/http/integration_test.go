//go:build integration

package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// TestAnalysisChatEndToEnd_Integration drives the public API end-to-end against
// a real PostgreSQL container, exercising the analysis creation, retrieval,
// listing and scoped chat flows through HTTP.
func TestAnalysisChatEndToEnd_Integration(t *testing.T) {
	if os.Getenv("OBSERVAI_TEST_TESTCONTAINERS") == "0" {
		t.Skip("OBSERVAI_TEST_TESTCONTAINERS=0 skips Docker-based integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	dsn := startPostgresContainer(ctx, t)
	runMigrations(t, dsn)

	server := buildIntegrationServer(ctx, t, dsn)
	t.Cleanup(server.Close)

	client := server.Client()
	client.Timeout = 10 * time.Second
	baseURL := server.URL

	var createdID string

	t.Run("POST /v1/analyses creates analysis", func(t *testing.T) {
		body := []byte(`{
			"goal": "investigate checkout latency spike",
			"timeWindow": {"start": "2026-05-13T08:00:00Z", "end": "2026-05-13T08:30:00Z"},
			"affectedServices": ["checkout-api"],
			"signals": ["logs", "metrics", "traces"],
			"context": "deploy completed at 07:55 UTC"
		}`)

		response := postJSON(t, client, baseURL+"/v1/analyses", body)
		defer response.Body.Close()

		require.Equal(t, stdhttp.StatusCreated, response.StatusCode)
		payload := decodeWrapper(t, response)
		data := payload["data"].(map[string]any)
		id, _ := data["id"].(string)
		require.NotEmpty(t, id, "analysis id must be returned")
		createdID = id
		assert.NotEmpty(t, data["summary"])
		assert.Contains(t, []any{"low", "medium", "high", "critical"}, data["severity"])
	})

	t.Run("GET /v1/analyses/{id} returns the same analysis", func(t *testing.T) {
		require.NotEmpty(t, createdID)

		response, err := client.Get(baseURL + "/v1/analyses/" + createdID)
		require.NoError(t, err)
		defer response.Body.Close()

		require.Equal(t, stdhttp.StatusOK, response.StatusCode)
		payload := decodeWrapper(t, response)
		data := payload["data"].(map[string]any)
		assert.Equal(t, createdID, data["id"])
	})

	t.Run("GET /v1/analyses lists the created analysis", func(t *testing.T) {
		require.NotEmpty(t, createdID)

		response, err := client.Get(baseURL + "/v1/analyses?service=checkout-api")
		require.NoError(t, err)
		defer response.Body.Close()

		require.Equal(t, stdhttp.StatusOK, response.StatusCode)
		payload := decodeWrapper(t, response)
		data := payload["data"].(map[string]any)
		items, _ := data["items"].([]any)
		require.NotEmpty(t, items)

		found := false
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if item["id"] == createdID {
				found = true
				break
			}
		}
		assert.True(t, found, "list must include the created analysis")
	})

	t.Run("POST /v1/analyses/{id}/chat answers in-scope question", func(t *testing.T) {
		require.NotEmpty(t, createdID)

		body := []byte(`{"question": "Which evidence supports this analysis?"}`)
		response := postJSON(t, client, baseURL+"/v1/analyses/"+createdID+"/chat", body)
		defer response.Body.Close()

		require.Equal(t, stdhttp.StatusOK, response.StatusCode)
		payload := decodeWrapper(t, response)
		data := payload["data"].(map[string]any)
		assert.Equal(t, createdID, data["analysisId"])
		assert.NotEmpty(t, data["answer"])
	})

	t.Run("GET /v1/analyses/{id}/chat returns persisted history", func(t *testing.T) {
		require.NotEmpty(t, createdID)

		response, err := client.Get(baseURL + "/v1/analyses/" + createdID + "/chat")
		require.NoError(t, err)
		defer response.Body.Close()

		require.Equal(t, stdhttp.StatusOK, response.StatusCode)
		payload := decodeWrapper(t, response)
		data := payload["data"].(map[string]any)
		messages, _ := data["messages"].([]any)
		require.Len(t, messages, 2, "history must contain the user question and assistant answer")

		first := messages[0].(map[string]any)
		second := messages[1].(map[string]any)
		assert.Equal(t, "user", first["role"])
		assert.Equal(t, "assistant", second["role"])
	})

	t.Run("POST /v1/analyses/{id}/chat rejects out-of-scope questions", func(t *testing.T) {
		require.NotEmpty(t, createdID)

		body := []byte(`{"question": "Can you write a Python script to scrape Wikipedia?"}`)
		response := postJSON(t, client, baseURL+"/v1/analyses/"+createdID+"/chat", body)
		defer response.Body.Close()

		require.Equal(t, stdhttp.StatusBadRequest, response.StatusCode)
		payload := decodeWrapper(t, response)
		data := payload["data"].(map[string]any)
		assert.Equal(t, "question_out_of_scope", data["code"])
	})
}

func startPostgresContainer(ctx context.Context, t *testing.T) string {
	t.Helper()

	container, err := tcpostgres.Run(
		ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("observai"),
		tcpostgres.WithUsername("observai"),
		tcpostgres.WithPassword("observai"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err, "start postgres testcontainer")

	t.Cleanup(func() {
		terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = container.Terminate(terminateCtx)
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "fetch container dsn")
	return dsn
}

func runMigrations(t *testing.T, dsn string) {
	t.Helper()

	migrationsPath := migrationsDir(t)
	migrator, err := migrate.New("file://"+migrationsPath, dsn)
	require.NoError(t, err, "init golang-migrate")
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})

	require.NoError(t, migrator.Up(), "apply migrations")
}

func migrationsDir(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve caller path")

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	dir := filepath.Join(projectRoot, "migrations")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("migrations directory not found at %s: %v", dir, err)
	}
	return dir
}

func postJSON(t *testing.T, client *stdhttp.Client, url string, body []byte) *stdhttp.Response {
	t.Helper()

	request, err := stdhttp.NewRequest(stdhttp.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	require.NoError(t, err)
	return response
}

func decodeWrapper(t *testing.T, response *stdhttp.Response) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload), "decode response wrapper")
	require.Contains(t, payload, "data", "missing data field")
	require.Contains(t, payload, "metadata", "missing metadata field")
	metadata := payload["metadata"].(map[string]any)
	if _, ok := metadata["requestId"].(string); !ok {
		t.Fatalf("metadata.requestId missing: %v", metadata)
	}
	return payload
}
