package policy

import (
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
)

func TestSeverityPolicyClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    SeverityInput
		expected domain.Severity
	}{
		{
			name:     "returns info floor when there is no evidence",
			input:    SeverityInput{},
			expected: domain.SeverityInfo,
		},
		{
			name: "promotes to high when a single error signal is present",
			input: SeverityInput{
				Evidence: []domain.Evidence{
					{Signal: domain.SignalLogs, Name: "checkout_error_rate", Unit: "errors", Score: 4},
				},
			},
			expected: domain.SeverityHigh,
		},
		{
			name: "promotes to critical when multiple error signals are present",
			input: SeverityInput{
				Evidence: []domain.Evidence{
					{Signal: domain.SignalLogs, Name: "checkout_error_rate", Score: 1, Summary: "error 5xx"},
					{Signal: domain.SignalLogs, Name: "payments_error_rate", Score: 1, Summary: "exception"},
					{Signal: domain.SignalLogs, Name: "shipping_error_rate", Score: 1, Summary: "error"},
				},
			},
			expected: domain.SeverityCritical,
		},
		{
			name: "promotes to medium when multiple services are affected",
			input: SeverityInput{
				Request: domain.AnalysisRequest{AffectedServices: []string{"checkout", "payments", "shipping"}},
				Evidence: []domain.Evidence{
					{Service: "checkout", Name: "service_up", Unit: "ratio", Score: 1},
				},
			},
			expected: domain.SeverityMedium,
		},
		{
			name: "promotes to medium when evidence volume crosses threshold",
			input: SeverityInput{
				Evidence: []domain.Evidence{
					{Name: "p50_latency", Unit: "seconds", Score: 0.1},
					{Name: "p95_latency", Unit: "seconds", Score: 0.2},
					{Name: "p99_latency", Unit: "seconds", Score: 0.3},
					{Name: "request_rate", Unit: "rps", Score: 100},
					{Name: "queue_depth", Unit: "count", Score: 5},
				},
			},
			expected: domain.SeverityMedium,
		},
	}

	policy := NewSeverityPolicy()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.expected, policy.Classify(test.input))
		})
	}
}

func TestSeverityPolicyReconcileKeepsHigherSeverity(t *testing.T) {
	t.Parallel()

	policy := NewSeverityPolicy()
	input := SeverityInput{
		Evidence: []domain.Evidence{
			{Signal: domain.SignalLogs, Name: "checkout_error_rate", Unit: "errors", Score: 4},
		},
	}

	assert.Equal(t, domain.SeverityCritical, policy.Reconcile(domain.SeverityCritical, input))
	assert.Equal(t, domain.SeverityHigh, policy.Reconcile(domain.SeverityLow, input))
	assert.Equal(t, domain.SeverityHigh, policy.Reconcile("", input))
}

func TestSeverityPolicyReconcileNormalizesInvalidSuggestion(t *testing.T) {
	t.Parallel()

	policy := NewSeverityPolicy()
	got := policy.Reconcile(domain.Severity("apocalyptic"), SeverityInput{})
	assert.Equal(t, domain.SeverityInfo, got)
}

func TestErrorSignalSeverityRule(t *testing.T) {
	t.Parallel()

	rule := ErrorSignalSeverityRule{}
	assert.Equal(t, domain.SeverityInfo, rule.Evaluate(SeverityInput{}))
	assert.Equal(t, domain.SeverityHigh, rule.Evaluate(SeverityInput{
		Evidence: []domain.Evidence{{Summary: "exception spike", Score: 2}},
	}))
}

func TestHighScoreSeverityRuleMatchesUnit(t *testing.T) {
	t.Parallel()

	rule := HighScoreSeverityRule{Threshold: 10, Unit: "errors", Severity: domain.SeverityHigh}
	assert.Equal(t, domain.SeverityInfo, rule.Evaluate(SeverityInput{
		Evidence: []domain.Evidence{{Unit: "ratio", Score: 100}},
	}))
	assert.Equal(t, domain.SeverityHigh, rule.Evaluate(SeverityInput{
		Evidence: []domain.Evidence{{Unit: "errors", Score: 10}},
	}))
}
