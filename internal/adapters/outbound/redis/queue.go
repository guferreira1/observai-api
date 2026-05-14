package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	redisclient "github.com/redis/go-redis/v9"
)

const (
	analysisQueueKey      = "observai:queue:analysis-jobs:v1"
	defaultDequeueTimeout = 5 * time.Second
)

// JobEnqueuer publishes analysis job identifiers to a Redis list.
type JobEnqueuer struct {
	client   *redisclient.Client
	queueKey string
	observer observability.ProviderObserver
}

// EnqueuerOptions configures the Redis job enqueuer.
type EnqueuerOptions struct {
	QueueKey string
	Observer observability.ProviderObserver
}

// NewJobEnqueuer creates a Redis-backed enqueuer.
//
// The provided client must already be initialized and pinged. The QueueWorker
// can share the same client to reduce connection overhead.
func NewJobEnqueuer(client *redisclient.Client, opts EnqueuerOptions) *JobEnqueuer {
	queueKey := opts.QueueKey
	if queueKey == "" {
		queueKey = analysisQueueKey
	}
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if opts.Observer != nil {
		observer = opts.Observer
	}
	return &JobEnqueuer{client: client, queueKey: queueKey, observer: observer}
}

// EnqueueAnalysis pushes jobID onto the tail of the analysis queue.
func (enqueuer *JobEnqueuer) EnqueueAnalysis(ctx context.Context, jobID string) (err error) {
	startedAt := time.Now()
	defer func() {
		enqueuer.observer.Observe("redis", "enqueue_analysis_job", time.Since(startedAt), err)
	}()

	if err := enqueuer.client.RPush(ctx, enqueuer.queueKey, jobID).Err(); err != nil {
		return fmt.Errorf("rpush analysis job: %w", err)
	}
	return nil
}

// QueueWorker consumes analysis job identifiers from a Redis list using BLPOP.
type QueueWorker struct {
	client      *redisclient.Client
	queueKey    string
	concurrency int
	timeout     time.Duration
	observer    observability.ProviderObserver
}

// WorkerOptions configures the Redis queue worker pool.
type WorkerOptions struct {
	QueueKey       string
	Concurrency    int
	DequeueTimeout time.Duration
	Observer       observability.ProviderObserver
}

// NewQueueWorker creates a Redis-backed worker pool.
func NewQueueWorker(client *redisclient.Client, opts WorkerOptions) *QueueWorker {
	queueKey := opts.QueueKey
	if queueKey == "" {
		queueKey = analysisQueueKey
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	timeout := opts.DequeueTimeout
	if timeout <= 0 {
		timeout = defaultDequeueTimeout
	}
	observer := observability.ProviderObserver(observability.NoopProviderObserver{})
	if opts.Observer != nil {
		observer = opts.Observer
	}
	return &QueueWorker{
		client:      client,
		queueKey:    queueKey,
		concurrency: concurrency,
		timeout:     timeout,
		observer:    observer,
	}
}

// Start consumes jobs until ctx is canceled.
//
// Handler errors are surfaced through errorReporter. The worker keeps running
// after handler failures so a single bad job cannot freeze the queue.
func (worker *QueueWorker) Start(ctx context.Context, handler func(context.Context, string) error, errorReporter func(jobID string, err error)) {
	if handler == nil {
		return
	}
	if errorReporter == nil {
		errorReporter = func(string, error) {}
	}

	var pool sync.WaitGroup
	for index := 0; index < worker.concurrency; index++ {
		pool.Add(1)
		go func() {
			defer pool.Done()
			worker.consume(ctx, handler, errorReporter)
		}()
	}
	pool.Wait()
}

func (worker *QueueWorker) consume(ctx context.Context, handler func(context.Context, string) error, errorReporter func(string, error)) {
	for {
		if ctx.Err() != nil {
			return
		}

		jobID, err := worker.dequeue(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			errorReporter("", fmt.Errorf("dequeue analysis job: %w", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(worker.timeout):
			}
			continue
		}
		if jobID == "" {
			continue
		}

		if err := handler(ctx, jobID); err != nil {
			errorReporter(jobID, fmt.Errorf("handle analysis job: %w", err))
		}
	}
}

func (worker *QueueWorker) dequeue(ctx context.Context) (string, error) {
	startedAt := time.Now()
	var dequeueErr error
	defer func() {
		worker.observer.Observe("redis", "dequeue_analysis_job", time.Since(startedAt), dequeueErr)
	}()

	values, err := worker.client.BLPop(ctx, worker.timeout, worker.queueKey).Result()
	if err != nil {
		if errors.Is(err, redisclient.Nil) {
			return "", nil
		}
		dequeueErr = err
		return "", err
	}
	if len(values) < 2 {
		return "", nil
	}
	return values[1], nil
}
