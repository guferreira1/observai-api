// Package composite wires several SignalCollector implementations behind a
// single ports.SignalCollector interface.
//
// The use case continues to depend on a singular collector port. The
// composite fans out the request to every registered named collector,
// applies a CollectorErrorPolicy when one of them fails and concatenates
// the evidence each adapter returns. Each piece of evidence keeps its
// originating provider name in Evidence.Provider so consumers can attribute
// findings without breaking the contract.
package composite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/policy"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// NamedCollector pairs a SignalCollector with the operator-facing identity
// of the provider it talks to. The name is propagated to Evidence.Provider
// when an adapter leaves that field empty.
type NamedCollector struct {
	Name      string
	Collector ports.SignalCollector
}

// SignalCollector fans the analysis request out to several underlying collectors.
type SignalCollector struct {
	collectors  []NamedCollector
	errorPolicy policy.CollectorErrorPolicy
	logger      *slog.Logger
}

// Options configures the composite SignalCollector.
type Options struct {
	// ErrorPolicy decides whether a collector failure aborts the composite
	// request or is treated as a partial failure. Defaults to
	// PartialFailureCollectorErrorPolicy when nil.
	ErrorPolicy policy.CollectorErrorPolicy
	// Logger receives structured warnings for partial failures so operators
	// can attribute missing evidence to the correct provider. A nil logger
	// silences the warnings.
	Logger *slog.Logger
}

// NewSignalCollector creates a composite collector that wraps the supplied
// named collectors. The order of collectors is preserved when evidence is
// returned so deterministic ordering is achievable in tests.
func NewSignalCollector(collectors []NamedCollector, opts Options) *SignalCollector {
	errorPolicy := opts.ErrorPolicy
	if errorPolicy == nil {
		errorPolicy = policy.NewPartialFailureCollectorErrorPolicy()
	}
	return &SignalCollector{
		collectors:  collectors,
		errorPolicy: errorPolicy,
		logger:      opts.Logger,
	}
}

// Collect satisfies ports.SignalCollector by aggregating evidence from
// every registered collector. A failing collector is surfaced through the
// error policy; the composite returns the first unrecoverable error or the
// concatenated evidence when every failure is tolerated.
func (collector *SignalCollector) Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	if len(collector.collectors) == 0 {
		return nil, errors.New("composite signal collector has no registered providers")
	}

	aggregated := make([]domain.Evidence, 0)
	for _, named := range collector.collectors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		evidence, err := named.Collector.Collect(ctx, request)
		if err != nil {
			if !collector.errorPolicy.HandleCollectorError(named.Name, err) {
				return nil, fmt.Errorf("collect from %s: %w", named.Name, err)
			}
			collector.warnPartialFailure(named.Name, err)
			continue
		}
		aggregated = append(aggregated, attributeProvider(evidence, named.Name)...)
	}

	return aggregated, nil
}

func (collector *SignalCollector) warnPartialFailure(provider string, err error) {
	if collector.logger == nil {
		return
	}
	collector.logger.Warn("signal collector failed; continuing with partial evidence",
		"provider", provider,
		"error", err,
	)
}

// attributeProvider stamps the provider name on every piece of evidence
// that did not already carry one. Adapters that already set Provider keep
// their value, which preserves rich identifiers when an adapter wraps
// multiple backends.
func attributeProvider(evidence []domain.Evidence, provider string) []domain.Evidence {
	for index := range evidence {
		if evidence[index].Provider == "" {
			evidence[index].Provider = provider
		}
	}
	return evidence
}
