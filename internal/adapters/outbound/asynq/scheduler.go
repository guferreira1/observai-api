// Package asynq exposes scheduled task plumbing built on hibiken/asynq.
//
// Today the scheduler runs only cron-driven maintenance jobs (retention
// purge, webhook retry sweeps). Migrating the analysis queue to Asynq is
// a follow-up; the legacy Redis/in-memory queue continues to power the
// analysis worker untouched.
package asynq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// TaskTypeRetentionPurge enqueues a retention.purge task.
const TaskTypeRetentionPurge = "retention.purge"

// TaskTypeWebhookRetrySweep enqueues a webhook.retry_sweep task.
const TaskTypeWebhookRetrySweep = "webhook.retry_sweep"

// RetentionPurgeHandler executes the retention purge job. The implementation
// lives in usecase.AnalysisRetention.
type RetentionPurgeHandler func(ctx context.Context) error

// WebhookRetrySweepHandler executes the webhook retry sweep job.
type WebhookRetrySweepHandler func(ctx context.Context) error

// SchedulerOptions configures the Asynq scheduler + worker.
//
// RedisURL must be a redis:// URL that asynq can resolve. RetentionCron
// and WebhookRetryCron use the standard cron syntax (no seconds field);
// zero values fall back to sensible defaults.
type SchedulerOptions struct {
	RedisURL            string
	RetentionCron       string
	WebhookRetryCron    string
	RetentionHandler    RetentionPurgeHandler
	WebhookRetryHandler WebhookRetrySweepHandler
	Logger              *slog.Logger
	Concurrency         int
}

// Scheduler bundles an Asynq scheduler (cron triggers) with a worker
// (task executor). Start must be called from the composition root; Stop
// is wired into the graceful shutdown sequence.
type Scheduler struct {
	scheduler *asynq.Scheduler
	server    *asynq.Server
	mux       *asynq.ServeMux
	logger    *slog.Logger
}

// NewScheduler builds a Scheduler from the supplied options. The function
// returns an error when redis is unreachable.
func NewScheduler(opts SchedulerOptions) (*Scheduler, error) {
	if opts.RedisURL == "" {
		return nil, errors.New("scheduler requires OBSERVAI_REDIS_URL")
	}
	redisOpt, err := asynq.ParseRedisURI(opts.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse scheduler redis url: %w", err)
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}

	mux := asynq.NewServeMux()
	if opts.RetentionHandler != nil {
		mux.HandleFunc(TaskTypeRetentionPurge, func(ctx context.Context, _ *asynq.Task) error {
			if err := opts.RetentionHandler(ctx); err != nil {
				logger.Warn("retention purge task failed", "error", err)
				return err
			}
			return nil
		})
	}
	if opts.WebhookRetryHandler != nil {
		mux.HandleFunc(TaskTypeWebhookRetrySweep, func(ctx context.Context, _ *asynq.Task) error {
			if err := opts.WebhookRetryHandler(ctx); err != nil {
				logger.Warn("webhook retry sweep task failed", "error", err)
				return err
			}
			return nil
		})
	}

	server := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: concurrency,
		Logger:      slogAsynqLogger{logger: logger},
	})

	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{
		Logger: slogAsynqLogger{logger: logger},
	})

	retentionCron := opts.RetentionCron
	if retentionCron == "" {
		retentionCron = "0 3 * * *"
	}
	webhookCron := opts.WebhookRetryCron
	if webhookCron == "" {
		webhookCron = "*/5 * * * *"
	}
	if opts.RetentionHandler != nil {
		payload, _ := json.Marshal(struct{}{})
		if _, err := scheduler.Register(retentionCron, asynq.NewTask(TaskTypeRetentionPurge, payload)); err != nil {
			return nil, fmt.Errorf("register retention.purge schedule: %w", err)
		}
	}
	if opts.WebhookRetryHandler != nil {
		payload, _ := json.Marshal(struct{}{})
		if _, err := scheduler.Register(webhookCron, asynq.NewTask(TaskTypeWebhookRetrySweep, payload)); err != nil {
			return nil, fmt.Errorf("register webhook.retry_sweep schedule: %w", err)
		}
	}

	return &Scheduler{scheduler: scheduler, server: server, mux: mux, logger: logger}, nil
}

// Start launches the scheduler and worker. The call returns immediately;
// failures are surfaced through asynq's internal logger.
func (scheduler *Scheduler) Start() error {
	if err := scheduler.scheduler.Start(); err != nil {
		return fmt.Errorf("start asynq scheduler: %w", err)
	}
	if err := scheduler.server.Start(scheduler.mux); err != nil {
		return fmt.Errorf("start asynq server: %w", err)
	}
	scheduler.logger.Info("asynq scheduler started")
	return nil
}

// Stop drains the scheduler and worker.
func (scheduler *Scheduler) Stop() {
	scheduler.scheduler.Shutdown()
	scheduler.server.Stop()
	scheduler.server.Shutdown()
}

type slogAsynqLogger struct {
	logger *slog.Logger
}

func (adapter slogAsynqLogger) Debug(args ...any) { adapter.logger.Debug(fmt.Sprint(args...)) }
func (adapter slogAsynqLogger) Info(args ...any)  { adapter.logger.Info(fmt.Sprint(args...)) }
func (adapter slogAsynqLogger) Warn(args ...any)  { adapter.logger.Warn(fmt.Sprint(args...)) }
func (adapter slogAsynqLogger) Error(args ...any) { adapter.logger.Error(fmt.Sprint(args...)) }
func (adapter slogAsynqLogger) Fatal(args ...any) { adapter.logger.Error(fmt.Sprint(args...)) }
