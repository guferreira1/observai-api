package inmemory

import (
	"context"
	"sync"
)

// AnalysisLocker serializes critical sections per analysis identifier in memory.
//
// It is suitable for single-instance deployments and tests. Concurrent acquires
// on the same analysisID are queued; different analysisIDs run in parallel.
type AnalysisLocker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewAnalysisLocker creates an in-memory analysis locker.
func NewAnalysisLocker() *AnalysisLocker {
	return &AnalysisLocker{locks: make(map[string]*sync.Mutex)}
}

// Acquire blocks until the lock for analysisID is granted or ctx is canceled.
func (locker *AnalysisLocker) Acquire(ctx context.Context, analysisID string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	lock := locker.lockFor(analysisID)

	acquired := make(chan struct{})
	go func() {
		lock.Lock()
		close(acquired)
	}()

	select {
	case <-acquired:
		return lock.Unlock, nil
	case <-ctx.Done():
		go func() {
			<-acquired
			lock.Unlock()
		}()
		return nil, ctx.Err()
	}
}

func (locker *AnalysisLocker) lockFor(analysisID string) *sync.Mutex {
	locker.mu.Lock()
	defer locker.mu.Unlock()

	lock, ok := locker.locks[analysisID]
	if !ok {
		lock = &sync.Mutex{}
		locker.locks[analysisID] = lock
	}
	return lock
}
