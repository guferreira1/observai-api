package policy

// CollectorErrorPolicy decides how the composite signal collector reacts
// when one of its underlying provider collectors fails.
//
// Implementations choose between aborting the request (fail-fast) and
// continuing with partial evidence (best-effort). The composite collector
// asks the policy for each individual collector failure and records its
// decision in the response metadata so callers can tell that one or more
// providers were skipped.
type CollectorErrorPolicy interface {
	// HandleCollectorError is invoked when a single provider collector
	// returned a non-nil error. It returns true when the composite should
	// continue with the remaining collectors and false when it should
	// surface the error and abort the request.
	HandleCollectorError(provider string, err error) bool
}

// FailFastCollectorErrorPolicy aborts the composite request on the first
// provider failure. Use when partial evidence is unsafe (synchronous flows
// that must report all-or-nothing).
type FailFastCollectorErrorPolicy struct{}

// NewFailFastCollectorErrorPolicy builds a fail-fast policy.
func NewFailFastCollectorErrorPolicy() FailFastCollectorErrorPolicy {
	return FailFastCollectorErrorPolicy{}
}

// HandleCollectorError implements CollectorErrorPolicy.
func (FailFastCollectorErrorPolicy) HandleCollectorError(_ string, _ error) bool {
	return false
}

// PartialFailureCollectorErrorPolicy keeps collecting evidence even when one
// or more provider collectors fail. The failed providers are reported back
// to the caller through the composite's observer; the analysis still
// proceeds with whatever evidence has been gathered so far.
type PartialFailureCollectorErrorPolicy struct{}

// NewPartialFailureCollectorErrorPolicy builds a best-effort policy.
func NewPartialFailureCollectorErrorPolicy() PartialFailureCollectorErrorPolicy {
	return PartialFailureCollectorErrorPolicy{}
}

// HandleCollectorError implements CollectorErrorPolicy.
func (PartialFailureCollectorErrorPolicy) HandleCollectorError(_ string, _ error) bool {
	return true
}
