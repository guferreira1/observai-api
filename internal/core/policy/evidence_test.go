package policy

import (
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithinTimeWindow(t *testing.T) {
	t.Parallel()

	window := domain.TimeWindow{
		Start: time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC),
	}
	spec := WithinTimeWindow{Window: window}

	assert.True(t, spec.IsSatisfiedBy(domain.Evidence{Observed: window.Start.Add(15 * time.Minute)}))
	assert.False(t, spec.IsSatisfiedBy(domain.Evidence{Observed: window.End.Add(time.Hour)}))
	assert.False(t, spec.IsSatisfiedBy(domain.Evidence{Observed: window.Start.Add(-time.Hour)}))
	assert.True(t, spec.IsSatisfiedBy(domain.Evidence{}))
}

func TestForAffectedServicesMatchesCaseInsensitive(t *testing.T) {
	t.Parallel()

	spec := ForAffectedServices{Services: []string{"checkout-service"}}
	assert.True(t, spec.IsSatisfiedBy(domain.Evidence{Service: "Checkout-Service"}))
	assert.False(t, spec.IsSatisfiedBy(domain.Evidence{Service: "payments-service"}))
}

func TestForAffectedServicesEmptyListPassesEverything(t *testing.T) {
	t.Parallel()

	spec := ForAffectedServices{}
	assert.True(t, spec.IsSatisfiedBy(domain.Evidence{Service: "anything"}))
}

func TestCompositeSpecifications(t *testing.T) {
	t.Parallel()

	high := HighScore{Threshold: 5}
	errors := MentionsError{}

	and := AndSpecification{Specs: []EvidenceSpecification{high, errors}}
	or := OrSpecification{Specs: []EvidenceSpecification{high, errors}}
	not := NotSpecification{Spec: high}

	highError := domain.Evidence{Score: 10, Summary: "exception detected"}
	highOnly := domain.Evidence{Score: 10}
	lowError := domain.Evidence{Score: 1, Summary: "error"}
	noMatch := domain.Evidence{Score: 1}

	assert.True(t, and.IsSatisfiedBy(highError))
	assert.False(t, and.IsSatisfiedBy(highOnly))

	assert.True(t, or.IsSatisfiedBy(highOnly))
	assert.True(t, or.IsSatisfiedBy(lowError))
	assert.False(t, or.IsSatisfiedBy(noMatch))

	assert.False(t, not.IsSatisfiedBy(highOnly))
	assert.True(t, not.IsSatisfiedBy(noMatch))
}

func TestFilterEvidenceCapsByScore(t *testing.T) {
	t.Parallel()

	evidence := []domain.Evidence{
		{Name: "low", Score: 1},
		{Name: "mid", Score: 5},
		{Name: "high", Score: 9},
	}

	out := FilterEvidence(evidence, EvidenceSpecificationFunc(func(domain.Evidence) bool { return true }), 2)
	require.Len(t, out, 2)
	assert.Equal(t, "high", out[0].Name)
	assert.Equal(t, "mid", out[1].Name)
}

func TestFilterEvidenceSkipsCapWhenZero(t *testing.T) {
	t.Parallel()

	evidence := []domain.Evidence{{Name: "a", Score: 1}, {Name: "b", Score: 9}}
	out := FilterEvidence(evidence, nil, 0)
	require.Len(t, out, 2)
	assert.Equal(t, "a", out[0].Name)
}

func TestRelevantEvidenceSpecificationFiltersByWindowAndService(t *testing.T) {
	t.Parallel()

	window := domain.TimeWindow{
		Start: time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC),
	}
	spec := RelevantEvidenceSpecification(domain.AnalysisRequest{
		TimeWindow:       window,
		AffectedServices: []string{"checkout"},
	})

	inside := domain.Evidence{Service: "checkout", Observed: window.Start.Add(15 * time.Minute)}
	wrongService := domain.Evidence{Service: "payments", Observed: window.Start.Add(15 * time.Minute)}
	outsideWindow := domain.Evidence{Service: "checkout", Observed: window.End.Add(time.Hour)}

	assert.True(t, spec.IsSatisfiedBy(inside))
	assert.False(t, spec.IsSatisfiedBy(wrongService))
	assert.False(t, spec.IsSatisfiedBy(outsideWindow))
}
