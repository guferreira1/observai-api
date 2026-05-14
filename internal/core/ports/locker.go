package ports

import "context"

// AnalysisLocker serializes access to a critical section keyed by analysis identifier.
//
// Implementations must guarantee that, while a holder owns the lock for a given
// analysisID, no other caller observes the lock as available. The returned release
// function must be safe to call once when the holder is done.
type AnalysisLocker interface {
	Acquire(ctx context.Context, analysisID string) (release func(), err error)
}
