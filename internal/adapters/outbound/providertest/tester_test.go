package providertest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildProbeTargetJoinsProviderPathWithoutDuplicatingSegments(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		path     string
		expected string
	}{
		{
			name:     "root base receives full probe path",
			baseURL:  "https://api.openai.com",
			path:     "/v1/models",
			expected: "https://api.openai.com/v1/models",
		},
		{
			name:     "versioned base does not duplicate version segment",
			baseURL:  "https://api.openai.com/v1",
			path:     "/v1/models",
			expected: "https://api.openai.com/v1/models",
		},
		{
			name:     "nested versioned base keeps provider prefix",
			baseURL:  "https://openrouter.ai/api/v1",
			path:     "/v1/models",
			expected: "https://openrouter.ai/api/v1/models",
		},
		{
			name:     "query string comes from probe path",
			baseURL:  "http://prometheus:9090",
			path:     "/api/v1/query?query=up",
			expected: "http://prometheus:9090/api/v1/query?query=up",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := buildProbeTarget(test.baseURL, test.path)
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestBuildProbeTargetRejectsInvalidBaseURL(t *testing.T) {
	_, err := buildProbeTarget("api.openai.com", "/v1/models")
	require.Error(t, err)
}
