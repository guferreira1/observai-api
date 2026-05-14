package http

import "testing"

func TestSanitizeExternalMessage(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bearer_header",
			input:    "request prometheus: 401 Unauthorized Authorization: Bearer sk-prod-abcdef123456",
			expected: "request prometheus: 401 Unauthorized [redacted]",
		},
		{
			name:     "api_token_header",
			input:    "Api-Token: dt0c01.foo.bar baz",
			expected: "[redacted] baz",
		},
		{
			name:     "query_string_token",
			input:    "GET https://datadog/api/v1/query?token=DEADBEEFCAFE failed",
			expected: "GET https://datadog/api/v1/query?[redacted] failed",
		},
		{
			name:     "basic_auth_url",
			input:    "dial https://user:p4ssw0rd@elasticsearch.local/_search",
			expected: "dial [redacted]",
		},
		{
			name:     "no_secret",
			input:    "context deadline exceeded after 5s",
			expected: "context deadline exceeded after 5s",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeExternalMessage(tt.input); got != tt.expected {
				t.Fatalf("SanitizeExternalMessage(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
