package inmemory

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrAnalysisQueueClosed is returned when callers interact with a closed queue.
var ErrAnalysisQueueClosed = errors.New("analysis queue closed")

// AnalysisQueue is a bounded in-memory FIFO of analysis job identifiers.
//
// The same instance backs the [JobEnqueuer] (producer) and the [QueueWorker]
// (consumer) so local mode and tests do not need an external broker.
type AnalysisQueue struct {
	jobs   chan string
	once   sync.Once
	closed chan struct{}
}

// NewAnalysisQueue creates a queue with the supplied buffer size.
func NewAnalysisQueue(buffer int) *AnalysisQueue {
	if buffer <= 0 {
		buffer = 64
	}
	return &AnalysisQueue{
		jobs:   make(chan string, buffer),
		closed: make(chan struct{}),
	}
}

// Close stops accepting new jobs and unblocks consumers.
func (queue *AnalysisQueue) Close() {
	queue.once.Do(func() {
		close(queue.closed)
		close(queue.jobs)
	})
}

// JobEnqueuer publishes analysis jobs into an in-memory AnalysisQueue.
type JobEnqueuer struct {
	queue *AnalysisQueue
}

// NewJobEnqueuer creates an in-memory analysis job enqueuer.
func NewJobEnqueuer(queue *AnalysisQueue) *JobEnqueuer {
	return &JobEnqueuer{queue: queue}
}

// EnqueueAnalysis publishes jobID into the underlying queue.
func (enqueuer *JobEnqueuer) EnqueueAnalysis(ctx context.Context, jobID string) error {
	select {
	case <-enqueuer.queue.closed:
		return ErrAnalysisQueueClosed
	case <-ctx.Done():
		return ctx.Err()
	case enqueuer.queue.jobs <- jobID:
		return nil
	}
}

// SynchronousJobEnqueuer runs the registered handler inline on EnqueueAnalysis.
//
// It is intended for HTTP integration tests where async polling would only add
// flakiness. Production code uses [JobEnqueuer] together with [QueueWorker].
type SynchronousJobEnqueuer struct {
	mu      sync.Mutex
	handler func(context.Context, string) error
}

// NewSynchronousJobEnqueuer creates an enqueuer that executes inline once a handler is registered.
func NewSynchronousJobEnqueuer() *SynchronousJobEnqueuer {
	return &SynchronousJobEnqueuer{}
}

// SetHandler registers the job handler invoked by EnqueueAnalysis.
func (enqueuer *SynchronousJobEnqueuer) SetHandler(handler func(context.Context, string) error) {
	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	enqueuer.handler = handler
}

// EnqueueAnalysis runs the registered handler with the supplied jobID.
func (enqueuer *SynchronousJobEnqueuer) EnqueueAnalysis(ctx context.Context, jobID string) error {
	enqueuer.mu.Lock()
	handler := enqueuer.handler
	enqueuer.mu.Unlock()
	if handler == nil {
		return nil
	}
	return handler(ctx, jobID)
}

// QueueWorker consumes analysis jobs from an AnalysisQueue using a bounded worker pool.
type QueueWorker struct {
	queue       *AnalysisQueue
	concurrency int
}

// NewQueueWorker creates an in-memory worker pool with the supplied concurrency.
func NewQueueWorker(queue *AnalysisQueue, concurrency int) *QueueWorker {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &QueueWorker{queue: queue, concurrency: concurrency}
}

// Start consumes jobs until ctx is canceled or the queue is closed.
//
// Each consumed jobID is dispatched to handler. Handler errors are returned to
// the supplied errorReporter so the caller decides whether to log or retry; the
// worker never panics out of handler.
func (worker *QueueWorker) Start(ctx context.Context, handler func(context.Context, string) error, errorReporter func(jobID string, err error)) {
	if handler == nil {
		return
	}
	if errorReporter == nil {
		errorReporter = func(string, error) {}
	}

	var workersWaitGroup sync.WaitGroup
	for index := 0; index < worker.concurrency; index++ {
		workersWaitGroup.Add(1)
		go func() {
			defer workersWaitGroup.Done()
			worker.consume(ctx, handler, errorReporter)
		}()
	}
	workersWaitGroup.Wait()
}

func (worker *QueueWorker) consume(ctx context.Context, handler func(context.Context, string) error, errorReporter func(string, error)) {
	for {
		select {
		case <-ctx.Done():
			return
		case jobID, ok := <-worker.queue.jobs:
			if !ok {
				return
			}
			if err := handler(ctx, jobID); err != nil {
				errorReporter(jobID, fmt.Errorf("handle analysis job: %w", err))
			}
		}
	}
}
