package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeverityHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		severity   Severity
		valid      bool
		normalized Severity
		rank       int
	}{
		{
			name:       "accepts public severity",
			severity:   SeverityHigh,
			valid:      true,
			normalized: SeverityHigh,
			rank:       3,
		},
		{
			name:       "normalizes unsupported severity to info",
			severity:   Severity("urgent"),
			valid:      false,
			normalized: SeverityInfo,
			rank:       0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.valid, IsValidSeverity(test.severity))
			assert.Equal(t, test.normalized, NormalizeSeverity(test.severity))
			assert.Equal(t, test.rank, SeverityRank(test.severity))
		})
	}
}
