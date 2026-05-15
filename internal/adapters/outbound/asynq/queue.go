package asynq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	"github.com/hibiken/asynq"
)

// TaskTypeAnalysisRun is the Asynq task type carrying a single analysis
// job identifier; the worker fans it out to the use case handler.
const TaskTypeAnalysisRun = "analysis.run"

// AnalysisEnqueuer satisfies ports.JobEnqueuer over an Asynq client.
type AnalysisEnqueuer struct {
	client   *asynq.Client
	observer observability.ProviderObserver
}

// EnqueuerOptions configures the Asynq enqueuer.
type EnqueuerOptions struct {
	RedisURL string
	Observer observability.ProviderObserver
}

// NewAnalysisEnqueuer dials redis via the supplied URL and returns an
// Asynq-backed enqueuer.
func NewAnalysisEnqueuer(opts EnqueuerOptions) (*AnalysisEnqueuer, error) {
	if opts.RedisURL == "" {
		return nil, fmt.Errorf("asynq enqueuer requires a redis url")
	}
	redisOpt, err := asynq.ParseRedisURI(opts.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse asynq redis url: %w", err)
	}
	observer := opts.Observer
	if observer == nil {
		observer = observability.NoopProviderObserver{}
	}
	return &AnalysisEnqueuer{client: asynq.NewClient(redisOpt), observer: observer}, nil
}

// Close releases the underlying Asynq client.
func (enqueuer *AnalysisEnqueuer) Close() error {
	if enqueuer.client == nil {
		return nil
	}
	return enqueuer.client.Close()
}

// EnqueueAnalysis publishes a single analysis-job task to Asynq. Failures
// are observed under the "asynq" provider so existing dashboards group
// the metric alongside the other queue backends.
func (enqueuer *AnalysisEnqueuer) EnqueueAnalysis(ctx context.Context, jobID string) (err error) {
	startedAt := time.Now()
	defer func() { enqueuer.observer.Observe("asynq", "enqueue_analysis", time.Since(startedAt), err) }()

	payload, err := json.Marshal(analysisRunPayload{JobID: jobID})
	if err != nil {
		return fmt.Errorf("marshal asynq analysis payload: %w", err)
	}
	task := asynq.NewTask(TaskTypeAnalysisRun, payload, asynq.Queue("analysis"))
	_, err = enqueuer.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("enqueue asynq analysis: %w", err)
	}
	return nil
}

// AnalysisWorker subscribes to TaskTypeAnalysisRun and runs the handler
// supplied to Start. The worker reuses the package-wide Asynq server so a
// single server process drives both the cron scheduler and the analysis
// queue when OBSERVAI_QUEUE_BACKEND=asynq is set.
type AnalysisWorker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	logger *slog.Logger
}

// WorkerOptions configures the AnalysisWorker.
type WorkerOptions struct {
	RedisURL    string
	Concurrency int
	Logger      *slog.Logger
}

// NewAnalysisWorker dials redis and prepares the Asynq server + mux.
func NewAnalysisWorker(opts WorkerOptions) (*AnalysisWorker, error) {
	if opts.RedisURL == "" {
		return nil, fmt.Errorf("asynq worker requires a redis url")
	}
	redisOpt, err := asynq.ParseRedisURI(opts.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse asynq redis url: %w", err)
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	server := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: concurrency,
		Queues:      map[string]int{"analysis": 10},
		Logger:      slogAsynqLogger{logger: logger},
	})
	return &AnalysisWorker{server: server, mux: asynq.NewServeMux(), logger: logger}, nil
}

// Start consumes tasks of TaskTypeAnalysisRun, decoding each payload back
// into a job id and invoking the handler. Handler errors are forwarded to
// errorReporter (when supplied) and converted into Asynq retries so the
// existing job repository remains the source of truth for status.
func (worker *AnalysisWorker) Start(ctx context.Context, handler func(context.Context, string) error, errorReporter func(jobID string, err error)) {
	worker.mux.HandleFunc(TaskTypeAnalysisRun, func(taskCtx context.Context, task *asynq.Task) error {
		var payload analysisRunPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("decode asynq analysis payload: %w", err)
		}
		if err := handler(taskCtx, payload.JobID); err != nil {
			if errorReporter != nil {
				errorReporter(payload.JobID, err)
			}
			return err
		}
		return nil
	})
	if err := worker.server.Start(worker.mux); err != nil {
		worker.logger.Error("asynq analysis worker failed to start", "error", err)
		return
	}
	<-ctx.Done()
	worker.Stop()
}

// Stop drains the worker. Safe to call from shutdown.
func (worker *AnalysisWorker) Stop() {
	if worker.server == nil {
		return
	}
	worker.server.Stop()
	worker.server.Shutdown()
}

type analysisRunPayload struct {
	JobID string `json:"jobId"`
}
