package policy

import (
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoEvidenceRecommendationRule(t *testing.T) {
	t.Parallel()

	rule := NoEvidenceRecommendationRule{}
	out := rule.Evaluate(RecommendationInput{})
	require.Len(t, out, 1)
	assert.Contains(t, out[0].Action, "observability adapter")

	out = rule.Evaluate(RecommendationInput{Evidence: []domain.Evidence{{Name: "anything"}}})
	assert.Empty(t, out)
}

func TestErrorEvidenceRecommendationRule(t *testing.T) {
	t.Parallel()

	rule := ErrorEvidenceRecommendationRule{}
	out := rule.Evaluate(RecommendationInput{
		Evidence: []domain.Evidence{{Summary: "5xx error rate increased"}},
	})
	require.Len(t, out, 1)
	assert.Contains(t, out[0].Action, "error logs")

	out = rule.Evaluate(RecommendationInput{
		Evidence: []domain.Evidence{{Summary: "healthy"}},
	})
	assert.Empty(t, out)
}

func TestMultiServiceRecommendationRule(t *testing.T) {
	t.Parallel()

	rule := MultiServiceRecommendationRule{Threshold: 2}
	out := rule.Evaluate(RecommendationInput{
		Request: domain.AnalysisRequest{AffectedServices: []string{"checkout", "payments"}},
	})
	require.Len(t, out, 1)
	assert.Contains(t, out[0].Action, "dependencies")

	out = rule.Evaluate(RecommendationInput{
		Request: domain.AnalysisRequest{AffectedServices: []string{"checkout"}},
	})
	assert.Empty(t, out)
}

func TestMissingSignalRecommendationRuleListsGaps(t *testing.T) {
	t.Parallel()

	rule := MissingSignalRecommendationRule{}
	out := rule.Evaluate(RecommendationInput{
		Request: domain.AnalysisRequest{
			Signals: []domain.SignalType{domain.SignalLogs, domain.SignalMetrics, domain.SignalTraces},
		},
		Evidence: []domain.Evidence{{Signal: domain.SignalMetrics}},
	})
	require.Len(t, out, 2)
	assert.Contains(t, out[0].Action, "logs")
	assert.Contains(t, out[1].Action, "traces")
}

func TestRecommendationPolicyDedupesAndOrders(t *testing.T) {
	t.Parallel()

	policy := NewRecommendationPolicy()
	input := RecommendationInput{
		Request:  domain.AnalysisRequest{AffectedServices: []string{"checkout", "payments"}},
		Evidence: []domain.Evidence{{Summary: "exception detected", Score: 1}},
		Result: domain.AnalysisResult{
			RecommendedActions: []domain.Recommendation{
				{Action: "Inspect error logs and failing spans for the affected services in the requested window", Rationale: "duplicate", Priority: 1},
				{Action: "Inspect deployment events around the incident", Rationale: "from LLM", Priority: 5},
			},
		},
	}

	out := policy.Apply(input)

	actions := make([]string, 0, len(out))
	for _, recommendation := range out {
		actions = append(actions, recommendation.Action)
	}

	assert.Contains(t, actions, "Inspect error logs and failing spans for the affected services in the requested window")
	assert.Contains(t, actions, "Inspect deployment events around the incident")
	assert.Contains(t, actions, "Map upstream/downstream dependencies and correlate spans across the affected services")
	assert.Equal(t, len(actions), len(uniqueActions(actions)))

	for i := 1; i < len(out); i++ {
		assert.LessOrEqual(t, out[i-1].Priority, out[i].Priority)
	}
}

func uniqueActions(actions []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	return out
}
