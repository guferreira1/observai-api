package testfakes

import (
	"context"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisGenerator produces deterministic analysis results for tests.
type AnalysisGenerator struct{}

// NewAnalysisGenerator creates a deterministic analysis generator for tests.
func NewAnalysisGenerator() *AnalysisGenerator {
	return &AnalysisGenerator{}
}

// Generate converts normalized evidence into a deterministic analysis result.
func (generator *AnalysisGenerator) Generate(_ context.Context, request domain.AnalysisRequest, evidence []domain.Evidence) (domain.AnalysisResult, error) {
	severity := domain.SeverityMedium
	confidence := domain.ConfidenceMedium
	if len(evidence) >= 3 {
		severity = domain.SeverityHigh
		confidence = domain.ConfidenceHigh
	}

	services := request.AffectedServices
	if len(services) == 0 && len(evidence) > 0 {
		services = []string{evidence[0].Service}
	}

	return domain.AnalysisResult{
		Summary:          "Deterministic analysis for " + request.Goal,
		Severity:         severity,
		Confidence:       confidence,
		AffectedServices: services,
		DetectedAnomalies: []string{
			"normalized evidence indicates a reproducible investigation path",
		},
		PossibleRootCauses: []domain.RootCauseHypothesis{
			{
				Cause:      "provider-agnostic evidence requires deeper adapter-backed collection",
				Evidence:   evidenceNames(evidence),
				Confidence: confidence,
			},
		},
		RecommendedActions: []domain.Recommendation{
			{
				Action:      "connect a read-only observability adapter and rerun the analysis",
				Rationale:   "deterministic test evidence proves the flow but does not represent production telemetry",
				Priority:    1,
				EvidenceIDs: evidenceIDs(evidence),
			},
		},
		CodeLevelInsights: []string{
			"review the services and operations with the strongest normalized evidence",
		},
		MissingEvidence: []string{
			"real provider logs, metrics, traces or APM events",
		},
	}, nil
}

func evidenceNames(evidence []domain.Evidence) []string {
	names := make([]string, 0, len(evidence))
	for _, item := range evidence {
		names = append(names, item.Name)
	}
	return names
}

func evidenceIDs(evidence []domain.Evidence) []string {
	ids := make([]string, 0, len(evidence))
	for _, item := range evidence {
		if item.ID == "" {
			continue
		}
		ids = append(ids, item.ID)
	}
	return ids
}
