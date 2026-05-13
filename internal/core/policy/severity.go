// Package policy contains deterministic business rules that classify, filter
// and enrich analysis output independently of any LLM provider.
package policy

import (
	"strings"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// SeverityInput describes the data available to severity rules.
type SeverityInput struct {
	Request  domain.AnalysisRequest
	Evidence []domain.Evidence
}

// SeverityRule produces a candidate severity from analysis input.
type SeverityRule interface {
	Evaluate(input SeverityInput) domain.Severity
}

// SeverityPolicy combines deterministic rules to classify the final analysis severity.
//
// The policy always returns the highest candidate produced by its rules. When
// no rule fires the policy returns the floor severity, which defaults to
// domain.SeverityInfo.
type SeverityPolicy struct {
	rules []SeverityRule
	floor domain.Severity
}

// NewSeverityPolicy builds the default deterministic severity policy.
func NewSeverityPolicy() SeverityPolicy {
	return SeverityPolicy{
		floor: domain.SeverityInfo,
		rules: []SeverityRule{
			ErrorSignalSeverityRule{},
			HighScoreSeverityRule{
				Threshold: 1,
				Unit:      "errors",
				Severity:  domain.SeverityHigh,
			},
			HighScoreSeverityRule{
				Threshold: 1,
				Unit:      "ratio",
				Severity:  domain.SeverityMedium,
			},
			MultiServiceSeverityRule{Threshold: 2, Severity: domain.SeverityMedium},
			EvidenceVolumeSeverityRule{Threshold: 5, Severity: domain.SeverityMedium},
		},
	}
}

// Classify returns the deterministic severity for the provided analysis input.
func (policy SeverityPolicy) Classify(input SeverityInput) domain.Severity {
	severity := policy.floor
	if severity == "" {
		severity = domain.SeverityInfo
	}
	for _, rule := range policy.rules {
		candidate := rule.Evaluate(input)
		if domain.SeverityRank(candidate) > domain.SeverityRank(severity) {
			severity = candidate
		}
	}
	return severity
}

// Reconcile combines an LLM-suggested severity with the deterministic severity
// and returns the higher of the two so the API never under-reports impact.
func (policy SeverityPolicy) Reconcile(suggested domain.Severity, input SeverityInput) domain.Severity {
	derived := policy.Classify(input)
	if domain.SeverityRank(suggested) > domain.SeverityRank(derived) {
		return domain.NormalizeSeverity(suggested)
	}
	return derived
}

// ErrorSignalSeverityRule promotes severity when evidence describes errors.
type ErrorSignalSeverityRule struct{}

type errorSignalSeverityThreshold struct {
	count    int
	severity domain.Severity
}

var errorSignalSeverityThresholds = []errorSignalSeverityThreshold{
	{count: 3, severity: domain.SeverityCritical},
	{count: 1, severity: domain.SeverityHigh},
}

// Evaluate inspects evidence names, summaries and units for error signals.
func (ErrorSignalSeverityRule) Evaluate(input SeverityInput) domain.Severity {
	errorCount := 0
	for _, evidence := range input.Evidence {
		if mentionsError(evidence) {
			errorCount++
		}
	}
	return severityForErrorCount(errorCount)
}

// HighScoreSeverityRule fires when any evidence with the configured unit
// has a score at or above the threshold.
type HighScoreSeverityRule struct {
	Threshold float64
	Unit      string
	Severity  domain.Severity
}

// Evaluate returns the configured severity when at least one matching evidence
// crosses the threshold.
func (rule HighScoreSeverityRule) Evaluate(input SeverityInput) domain.Severity {
	for _, evidence := range input.Evidence {
		if rule.Unit != "" && !strings.EqualFold(evidence.Unit, rule.Unit) {
			continue
		}
		if evidence.Score >= rule.Threshold {
			return domain.NormalizeSeverity(rule.Severity)
		}
	}
	return domain.SeverityInfo
}

// MultiServiceSeverityRule fires when at least Threshold services are affected.
type MultiServiceSeverityRule struct {
	Threshold int
	Severity  domain.Severity
}

// Evaluate returns the configured severity when enough distinct services are present.
func (rule MultiServiceSeverityRule) Evaluate(input SeverityInput) domain.Severity {
	services := distinctServices(input)
	if len(services) >= rule.Threshold {
		return domain.NormalizeSeverity(rule.Severity)
	}
	return domain.SeverityInfo
}

// EvidenceVolumeSeverityRule fires when evidence volume crosses the threshold.
type EvidenceVolumeSeverityRule struct {
	Threshold int
	Severity  domain.Severity
}

// Evaluate returns the configured severity when there is enough evidence.
func (rule EvidenceVolumeSeverityRule) Evaluate(input SeverityInput) domain.Severity {
	if len(input.Evidence) >= rule.Threshold {
		return domain.NormalizeSeverity(rule.Severity)
	}
	return domain.SeverityInfo
}

func mentionsError(evidence domain.Evidence) bool {
	if strings.EqualFold(evidence.Unit, "errors") && evidence.Score > 0 {
		return true
	}
	candidates := []string{evidence.Name, evidence.Summary, string(evidence.Signal)}
	for _, value := range candidates {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "error") || strings.Contains(lower, "exception") || strings.Contains(lower, "5xx") {
			return true
		}
	}
	for _, value := range evidence.Attributes {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "error") || strings.Contains(lower, "exception") {
			return true
		}
	}
	return false
}

func distinctServices(input SeverityInput) map[string]struct{} {
	services := make(map[string]struct{})
	for _, service := range input.Request.AffectedServices {
		trimmed := strings.TrimSpace(service)
		if trimmed != "" {
			services[trimmed] = struct{}{}
		}
	}
	for _, evidence := range input.Evidence {
		trimmed := strings.TrimSpace(evidence.Service)
		if trimmed != "" {
			services[trimmed] = struct{}{}
		}
	}
	return services
}

func severityForErrorCount(errorCount int) domain.Severity {
	for _, threshold := range errorSignalSeverityThresholds {
		if errorCount >= threshold.count {
			return threshold.severity
		}
	}
	return domain.SeverityInfo
}
