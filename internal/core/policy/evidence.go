package policy

import (
	"sort"
	"strings"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// EvidenceSpecification is a composable predicate that selects relevant evidence.
type EvidenceSpecification interface {
	IsSatisfiedBy(evidence domain.Evidence) bool
}

// EvidenceSpecificationFunc adapts a function into an EvidenceSpecification.
type EvidenceSpecificationFunc func(domain.Evidence) bool

// IsSatisfiedBy evaluates the underlying function.
func (fn EvidenceSpecificationFunc) IsSatisfiedBy(evidence domain.Evidence) bool {
	return fn(evidence)
}

// AndSpecification combines specifications using logical AND semantics.
type AndSpecification struct {
	Specs []EvidenceSpecification
}

// IsSatisfiedBy returns true only when every contained specification matches.
func (spec AndSpecification) IsSatisfiedBy(evidence domain.Evidence) bool {
	for _, inner := range spec.Specs {
		if inner == nil {
			continue
		}
		if !inner.IsSatisfiedBy(evidence) {
			return false
		}
	}
	return true
}

// OrSpecification combines specifications using logical OR semantics.
type OrSpecification struct {
	Specs []EvidenceSpecification
}

// IsSatisfiedBy returns true when at least one contained specification matches.
func (spec OrSpecification) IsSatisfiedBy(evidence domain.Evidence) bool {
	if len(spec.Specs) == 0 {
		return true
	}
	for _, inner := range spec.Specs {
		if inner == nil {
			continue
		}
		if inner.IsSatisfiedBy(evidence) {
			return true
		}
	}
	return false
}

// NotSpecification inverts another specification.
type NotSpecification struct {
	Spec EvidenceSpecification
}

// IsSatisfiedBy returns the inverted result of the wrapped specification.
func (spec NotSpecification) IsSatisfiedBy(evidence domain.Evidence) bool {
	if spec.Spec == nil {
		return false
	}
	return !spec.Spec.IsSatisfiedBy(evidence)
}

// WithinTimeWindow keeps evidence observed inside the requested window.
type WithinTimeWindow struct {
	Window domain.TimeWindow
}

// IsSatisfiedBy returns true when the evidence falls inside the window.
func (spec WithinTimeWindow) IsSatisfiedBy(evidence domain.Evidence) bool {
	if spec.Window.Start.IsZero() && spec.Window.End.IsZero() {
		return true
	}
	if evidence.Observed.IsZero() {
		return true
	}
	if !spec.Window.Start.IsZero() && evidence.Observed.Before(spec.Window.Start) {
		return false
	}
	if !spec.Window.End.IsZero() && evidence.Observed.After(spec.Window.End) {
		return false
	}
	return true
}

// ForAffectedServices keeps evidence for the requested services. When the list
// is empty the specification keeps every evidence.
type ForAffectedServices struct {
	Services []string
}

// IsSatisfiedBy returns true when the evidence service matches the requested list.
func (spec ForAffectedServices) IsSatisfiedBy(evidence domain.Evidence) bool {
	if len(spec.Services) == 0 {
		return true
	}
	service := strings.TrimSpace(evidence.Service)
	if service == "" {
		return true
	}
	for _, candidate := range spec.Services {
		if strings.EqualFold(strings.TrimSpace(candidate), service) {
			return true
		}
	}
	return false
}

// HighScore keeps evidence whose score is at or above the threshold.
type HighScore struct {
	Threshold float64
}

// IsSatisfiedBy returns true when the evidence score is high enough.
func (spec HighScore) IsSatisfiedBy(evidence domain.Evidence) bool {
	return evidence.Score >= spec.Threshold
}

// MentionsError keeps evidence whose payload mentions an error or exception.
type MentionsError struct{}

// IsSatisfiedBy returns true when the evidence describes an error condition.
func (MentionsError) IsSatisfiedBy(evidence domain.Evidence) bool {
	return mentionsError(evidence)
}

// FilterEvidence returns the subset of evidence that matches the specification,
// capped at maxItems when maxItems is positive. Evidence is ranked by score
// descending so high-signal items survive the cap.
func FilterEvidence(evidence []domain.Evidence, spec EvidenceSpecification, maxItems int) []domain.Evidence {
	if len(evidence) == 0 {
		return evidence
	}

	filtered := make([]domain.Evidence, 0, len(evidence))
	for _, item := range evidence {
		if spec == nil || spec.IsSatisfiedBy(item) {
			filtered = append(filtered, item)
		}
	}

	if maxItems <= 0 || len(filtered) <= maxItems {
		return filtered
	}

	ranked := make([]domain.Evidence, len(filtered))
	copy(ranked, filtered)
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
	return ranked[:maxItems]
}

// RelevantEvidenceSpecification builds the default specification used to trim
// evidence before sending it to an LLM provider.
func RelevantEvidenceSpecification(request domain.AnalysisRequest) EvidenceSpecification {
	return AndSpecification{
		Specs: []EvidenceSpecification{
			WithinTimeWindow{Window: request.TimeWindow},
			ForAffectedServices{Services: request.AffectedServices},
		},
	}
}
