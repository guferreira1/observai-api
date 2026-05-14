package policy

import (
	"sort"
	"strings"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// RecommendationInput describes the data available to recommendation rules.
type RecommendationInput struct {
	Request  domain.AnalysisRequest
	Evidence []domain.Evidence
	Result   domain.AnalysisResult
}

// RecommendationRule produces zero or more recommendations from analysis input.
type RecommendationRule interface {
	Evaluate(input RecommendationInput) []domain.Recommendation
}

// RecommendationPolicy composes deterministic recommendation rules.
type RecommendationPolicy struct {
	rules []RecommendationRule
}

// NewRecommendationPolicy builds the default recommendation policy.
func NewRecommendationPolicy() RecommendationPolicy {
	return RecommendationPolicy{
		rules: []RecommendationRule{
			NoEvidenceRecommendationRule{},
			ErrorEvidenceRecommendationRule{},
			MultiServiceRecommendationRule{Threshold: 2},
			MissingSignalRecommendationRule{},
		},
	}
}

// Apply executes every rule against the input and merges deterministic
// recommendations with the LLM-suggested ones, deduplicating by action.
func (policy RecommendationPolicy) Apply(input RecommendationInput) []domain.Recommendation {
	merged := make([]domain.Recommendation, 0, len(input.Result.RecommendedActions))
	seen := make(map[string]struct{})
	for _, suggested := range input.Result.RecommendedActions {
		key := recommendationKey(suggested.Action)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, suggested)
	}

	for _, rule := range policy.rules {
		for _, recommendation := range rule.Evaluate(input) {
			key := recommendationKey(recommendation.Action)
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, recommendation)
		}
	}

	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Priority < merged[j].Priority
	})
	return merged
}

// NoEvidenceRecommendationRule suggests connecting a provider when the analysis
// has no evidence to investigate.
type NoEvidenceRecommendationRule struct{}

// Evaluate returns a recommendation only when there is no usable evidence.
func (NoEvidenceRecommendationRule) Evaluate(input RecommendationInput) []domain.Recommendation {
	if len(input.Evidence) > 0 {
		return nil
	}
	return []domain.Recommendation{
		{
			Action:    "Connect a read-only observability adapter and rerun the analysis",
			Rationale: "the analysis collected no evidence, so the diagnosis cannot be grounded in real telemetry",
			Priority:  1,
		},
	}
}

// ErrorEvidenceRecommendationRule suggests log/trace deep dives when error
// signals are present in the evidence.
type ErrorEvidenceRecommendationRule struct{}

// Evaluate returns a recommendation only when evidence describes errors.
func (ErrorEvidenceRecommendationRule) Evaluate(input RecommendationInput) []domain.Recommendation {
	for _, evidence := range input.Evidence {
		if mentionsError(evidence) {
			return []domain.Recommendation{
				{
					Action:    "Inspect error logs and failing spans for the affected services in the requested window",
					Rationale: "evidence contains error signals that likely point at the failing dependency",
					Priority:  2,
				},
			}
		}
	}
	return nil
}

// MultiServiceRecommendationRule suggests dependency-graph inspection when
// the analysis spans multiple services.
type MultiServiceRecommendationRule struct {
	Threshold int
}

// Evaluate returns a recommendation only when enough distinct services are present.
func (rule MultiServiceRecommendationRule) Evaluate(input RecommendationInput) []domain.Recommendation {
	threshold := rule.Threshold
	if threshold <= 0 {
		threshold = 2
	}
	services := distinctServices(SeverityInput{Request: input.Request, Evidence: input.Evidence})
	if len(services) < threshold {
		return nil
	}
	return []domain.Recommendation{
		{
			Action:    "Map upstream/downstream dependencies and correlate spans across the affected services",
			Rationale: "multiple services are involved, so the root cause is more likely in a shared dependency or call path",
			Priority:  3,
		},
	}
}

// MissingSignalRecommendationRule suggests broadening signal collection when a
// requested signal type did not return any evidence.
type MissingSignalRecommendationRule struct{}

// Evaluate returns a recommendation for each missing requested signal type.
func (MissingSignalRecommendationRule) Evaluate(input RecommendationInput) []domain.Recommendation {
	if len(input.Request.Signals) == 0 || len(input.Evidence) == 0 {
		return nil
	}
	present := make(map[domain.SignalType]struct{})
	for _, evidence := range input.Evidence {
		present[evidence.Signal] = struct{}{}
	}

	recommendations := make([]domain.Recommendation, 0)
	for _, signal := range input.Request.Signals {
		if _, ok := present[signal]; ok {
			continue
		}
		recommendations = append(recommendations, domain.Recommendation{
			Action:    "Collect " + string(signal) + " evidence for the affected services in the requested window",
			Rationale: "the requested " + string(signal) + " signal returned no evidence, which leaves a blind spot in the analysis",
			Priority:  4,
		})
	}
	return recommendations
}

func recommendationKey(action string) string {
	return strings.ToLower(strings.TrimSpace(action))
}
