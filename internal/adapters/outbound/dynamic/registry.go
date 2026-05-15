// Package dynamic exposes atomic-pointer adapter wrappers so the
// composition root can swap the active SignalCollector / AnalysisGenerator
// / ChatResponder / TraceProvider after a provider/LLM CRUD operation
// without restarting the API.
//
// Each wrapper satisfies the corresponding core port and delegates the
// call to the value currently held in atomic.Pointer. Set replaces the
// current value; in-flight calls observe the previous value because the
// pointer load happens at call time.
package dynamic

import (
	"context"
	"sync/atomic"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// Collector wraps a ports.SignalCollector behind an atomic pointer.
type Collector struct {
	inner atomic.Pointer[ports.SignalCollector]
}

// NewCollector returns a Collector initialised with the supplied value.
func NewCollector(initial ports.SignalCollector) *Collector {
	c := &Collector{}
	c.Set(initial)
	return c
}

// Set replaces the current collector. A nil value makes subsequent calls
// fail with domain.ErrProviderNotConfigured.
func (c *Collector) Set(value ports.SignalCollector) {
	if value == nil {
		c.inner.Store(nil)
		return
	}
	c.inner.Store(&value)
}

// Collect delegates to the currently-held collector.
func (c *Collector) Collect(ctx context.Context, request domain.AnalysisRequest) ([]domain.Evidence, error) {
	current := c.inner.Load()
	if current == nil || *current == nil {
		return nil, domain.ErrProviderNotConfigured
	}
	return (*current).Collect(ctx, request)
}

// Generator wraps a ports.AnalysisGenerator behind an atomic pointer.
type Generator struct {
	inner atomic.Pointer[ports.AnalysisGenerator]
}

// NewGenerator returns a Generator initialised with the supplied value.
func NewGenerator(initial ports.AnalysisGenerator) *Generator {
	g := &Generator{}
	g.Set(initial)
	return g
}

// Set replaces the current generator.
func (g *Generator) Set(value ports.AnalysisGenerator) {
	if value == nil {
		g.inner.Store(nil)
		return
	}
	g.inner.Store(&value)
}

// Generate delegates to the currently-held generator.
func (g *Generator) Generate(ctx context.Context, request domain.AnalysisRequest, evidence []domain.Evidence) (domain.AnalysisResult, error) {
	current := g.inner.Load()
	if current == nil || *current == nil {
		return domain.AnalysisResult{}, domain.ErrProviderNotConfigured
	}
	return (*current).Generate(ctx, request, evidence)
}

// Responder wraps a ports.ChatResponder behind an atomic pointer.
type Responder struct {
	inner atomic.Pointer[ports.ChatResponder]
}

// NewResponder returns a Responder initialised with the supplied value.
func NewResponder(initial ports.ChatResponder) *Responder {
	r := &Responder{}
	r.Set(initial)
	return r
}

// Set replaces the current responder.
func (r *Responder) Set(value ports.ChatResponder) {
	if value == nil {
		r.inner.Store(nil)
		return
	}
	r.inner.Store(&value)
}

// Answer delegates to the currently-held responder.
func (r *Responder) Answer(ctx context.Context, analysis domain.AnalysisContext, question domain.ChatQuestion) (domain.ChatAnswer, error) {
	current := r.inner.Load()
	if current == nil || *current == nil {
		return domain.ChatAnswer{}, domain.ErrProviderNotConfigured
	}
	return (*current).Answer(ctx, analysis, question)
}

// TraceProvider wraps a ports.TraceProvider behind an atomic pointer.
type TraceProvider struct {
	inner atomic.Pointer[ports.TraceProvider]
}

// NewTraceProvider returns a TraceProvider initialised with the supplied value.
func NewTraceProvider(initial ports.TraceProvider) *TraceProvider {
	t := &TraceProvider{}
	t.Set(initial)
	return t
}

// Set replaces the current trace provider.
func (t *TraceProvider) Set(value ports.TraceProvider) {
	if value == nil {
		t.inner.Store(nil)
		return
	}
	t.inner.Store(&value)
}

// FetchSpans delegates to the currently-held trace provider.
func (t *TraceProvider) FetchSpans(ctx context.Context, analysisID string) ([]domain.Span, error) {
	current := t.inner.Load()
	if current == nil || *current == nil {
		return nil, domain.ErrProviderNotConfigured
	}
	return (*current).FetchSpans(ctx, analysisID)
}
