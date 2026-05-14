package policy

import "testing"

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: ""},
		{name: "whitespace_only", input: "   ", expected: ""},
		{name: "single_char", input: "a", expected: "*"},
		{name: "four_chars", input: "abcd", expected: "****"},
		{name: "short_token", input: "abcdef", expected: "a*****"},
		{name: "twelve_chars", input: "abcdefghijkl", expected: "a***********"},
		{name: "long_token", input: "sk-prod-supersecret", expected: "sk-****cret"},
		{name: "trims_whitespace", input: "  sk-prod-supersecret  ", expected: "sk-****cret"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskSecret(tt.input); got != tt.expected {
				t.Fatalf("MaskSecret(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
