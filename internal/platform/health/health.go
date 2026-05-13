// Package health exposes liveness and readiness HTTP handlers backed by a small
// Probe contract. Each adapter that the API depends on contributes a probe so
// the readiness response reflects real dependency state.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status names a readiness outcome.
type Status string

const (
	// StatusOk indicates the dependency answered the probe successfully.
	StatusOk Status = "ok"
	// StatusFailed indicates the dependency answered with an error or timed out.
	StatusFailed Status = "failed"
)

// Probe verifies that a single dependency is reachable.
type Probe interface {
	Name() string
	Check(ctx context.Context) error
}

// ProbeFunc adapts a plain function to the Probe interface.
type ProbeFunc struct {
	ProbeName string
	Fn        func(ctx context.Context) error
}

// Name returns the probe display name.
func (probe ProbeFunc) Name() string { return probe.ProbeName }

// Check runs the underlying function.
func (probe ProbeFunc) Check(ctx context.Context) error { return probe.Fn(ctx) }

// Checker runs all configured probes in parallel under a shared timeout.
type Checker struct {
	probes  []Probe
	timeout time.Duration
}

// NewChecker creates a checker that fans out the supplied probes with a per-call timeout.
func NewChecker(timeout time.Duration, probes ...Probe) *Checker {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Checker{probes: probes, timeout: timeout}
}

// Result describes the readiness outcome for the entire system.
type Result struct {
	Status Status        `json:"status"`
	Checks []ProbeResult `json:"checks"`
}

// ProbeResult describes the outcome for a single probe.
type ProbeResult struct {
	Name       string `json:"name"`
	Status     Status `json:"status"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// Run executes every probe in parallel and aggregates the results.
//
// A single failed probe is enough to flip the overall status to failed.
func (checker *Checker) Run(ctx context.Context) Result {
	if len(checker.probes) == 0 {
		return Result{Status: StatusOk, Checks: []ProbeResult{}}
	}

	results := make([]ProbeResult, len(checker.probes))

	var wg sync.WaitGroup
	for index, probe := range checker.probes {
		wg.Add(1)
		go func(index int, probe Probe) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, checker.timeout)
			defer cancel()

			startedAt := time.Now()
			err := probe.Check(probeCtx)
			result := ProbeResult{
				Name:       probe.Name(),
				Status:     StatusOk,
				DurationMs: time.Since(startedAt).Milliseconds(),
			}
			if err != nil {
				result.Status = StatusFailed
				result.Error = err.Error()
			}
			results[index] = result
		}(index, probe)
	}
	wg.Wait()

	overall := StatusOk
	for _, result := range results {
		if result.Status != StatusOk {
			overall = StatusFailed
			break
		}
	}

	return Result{Status: overall, Checks: results}
}

// LivenessHandler returns 200 OK so external schedulers can confirm the process is alive.
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	})
}

// ReadinessHandler returns the aggregated probe result. Any failed probe makes
// the response 503 so load balancers can stop routing traffic to the instance.
func ReadinessHandler(checker *Checker) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		result := checker.Run(request.Context())
		status := http.StatusOK
		if result.Status != StatusOk {
			status = http.StatusServiceUnavailable
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_ = json.NewEncoder(writer).Encode(result)
	})
}
