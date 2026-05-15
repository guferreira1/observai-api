package policy

import "github.com/guferreira1/observai-api/internal/core/domain"

// EvidenceSeverityRule classifies the impact of a single normalized evidence item.
type EvidenceSeverityRule interface {
	Evaluate(evidence domain.Evidence) domain.Severity
}

// EvidenceSeverityPolicy enriches normalized evidence with a stable severity value.
type EvidenceSeverityPolicy struct {
	rules []EvidenceSeverityRule
	floor domain.Severity
}

// NewEvidenceSeverityPolicy builds the default evidence severity policy.
func NewEvidenceSeverityPolicy() EvidenceSeverityPolicy {
	return EvidenceSeverityPolicy{
		floor: domain.SeverityInfo,
		rules: []EvidenceSeverityRule{
			ExplicitEvidenceSeverityRule{},
			ErrorEvidenceSeverityRule{},
			ScoreEvidenceSeverityRule{Threshold: 1, Severity: domain.SeverityMedium},
		},
	}
}

// Apply assigns a normalized severity to every evidence item in place.
func (policy EvidenceSeverityPolicy) Apply(evidence []domain.Evidence) {
	for index := range evidence {
		evidence[index].Severity = policy.Classify(evidence[index])
	}
}

// Classify returns the highest severity proposed by the configured rules.
func (policy EvidenceSeverityPolicy) Classify(evidence domain.Evidence) domain.Severity {
	severity := policy.floor
	if severity == "" {
		severity = domain.SeverityInfo
	}
	for _, rule := range policy.rules {
		candidate := rule.Evaluate(evidence)
		if domain.SeverityRank(candidate) > domain.SeverityRank(severity) {
			severity = candidate
		}
	}
	return severity
}

// ExplicitEvidenceSeverityRule preserves adapter-provided severity values.
type ExplicitEvidenceSeverityRule struct{}

// Evaluate returns the evidence severity when it is part of the public enum.
func (ExplicitEvidenceSeverityRule) Evaluate(evidence domain.Evidence) domain.Severity {
	return domain.NormalizeSeverity(evidence.Severity)
}

// ErrorEvidenceSeverityRule promotes evidence that describes error conditions.
type ErrorEvidenceSeverityRule struct{}

// Evaluate returns high severity when the evidence mentions an error.
func (ErrorEvidenceSeverityRule) Evaluate(evidence domain.Evidence) domain.Severity {
	if mentionsError(evidence) {
		return domain.SeverityHigh
	}
	return domain.SeverityInfo
}

// ScoreEvidenceSeverityRule promotes evidence whose score crosses a threshold.
type ScoreEvidenceSeverityRule struct {
	Threshold float64
	Severity  domain.Severity
}

// Evaluate returns the configured severity when the score crosses the threshold.
func (rule ScoreEvidenceSeverityRule) Evaluate(evidence domain.Evidence) domain.Severity {
	if evidence.Score >= rule.Threshold {
		return domain.NormalizeSeverity(rule.Severity)
	}
	return domain.SeverityInfo
}
