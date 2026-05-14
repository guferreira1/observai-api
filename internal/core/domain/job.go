package domain

import (
	"errors"
	"time"
)

// ErrJobNotFound indicates that an analysis job identifier does not exist.
var ErrJobNotFound = errors.New("analysis job not found")

// ErrInvalidJobTransition indicates that a job status transition is not allowed.
var ErrInvalidJobTransition = errors.New("invalid analysis job transition")

// JobStatus identifies the lifecycle state of an analysis job.
type JobStatus string

const (
	// JobStatusPending indicates the job is enqueued and waiting for a worker.
	JobStatusPending JobStatus = "pending"
	// JobStatusRunning indicates a worker started executing the job.
	JobStatusRunning JobStatus = "running"
	// JobStatusCompleted indicates the job finished successfully and produced a result.
	JobStatusCompleted JobStatus = "completed"
	// JobStatusFailed indicates the job terminated with an unrecoverable error.
	JobStatusFailed JobStatus = "failed"
	// JobStatusCanceled indicates the job was canceled by a client before completion.
	JobStatusCanceled JobStatus = "canceled"
)

var jobStatusSet = map[JobStatus]struct{}{
	JobStatusPending:   {},
	JobStatusRunning:   {},
	JobStatusCompleted: {},
	JobStatusFailed:    {},
	JobStatusCanceled:  {},
}

// IsValidJobStatus reports whether status is part of the public job lifecycle.
func IsValidJobStatus(status JobStatus) bool {
	_, ok := jobStatusSet[status]
	return ok
}

// JobPhase identifies the fine-grained stage a running analysis job is in.
//
// Phases are progress hints surfaced to clients so they can render a progress
// bar without coupling to internal use-case structure. JobStatus remains the
// authoritative lifecycle state; phase is only meaningful when the job is
// running.
type JobPhase string

const (
	// PhaseQueued indicates the job is waiting in the queue.
	PhaseQueued JobPhase = "queued"
	// PhaseCollectingSignals indicates the worker is collecting observability signals.
	PhaseCollectingSignals JobPhase = "collecting_signals"
	// PhaseNormalizing indicates the worker is normalizing collected evidence.
	PhaseNormalizing JobPhase = "normalizing"
	// PhaseCallingLLM indicates the worker is invoking the configured LLM provider.
	PhaseCallingLLM JobPhase = "calling_llm"
	// PhasePersisting indicates the worker is persisting the final analysis result.
	PhasePersisting JobPhase = "persisting"
	// PhaseDone indicates the analysis is fully persisted and ready for retrieval.
	PhaseDone JobPhase = "done"
)

var jobPhaseSet = map[JobPhase]struct{}{
	PhaseQueued:            {},
	PhaseCollectingSignals: {},
	PhaseNormalizing:       {},
	PhaseCallingLLM:        {},
	PhasePersisting:        {},
	PhaseDone:              {},
}

// IsValidJobPhase reports whether phase is part of the public progress taxonomy.
func IsValidJobPhase(phase JobPhase) bool {
	_, ok := jobPhaseSet[phase]
	return ok
}

// AnalysisJob describes the lifecycle and outcome of an asynchronous analysis execution.
type AnalysisJob struct {
	ID              string
	AnalysisID      string
	Status          JobStatus
	Phase           JobPhase
	ProgressPercent int
	Request         AnalysisRequest
	Result          *AnalysisResult
	ErrorMessage    string
	Attempt         int
	CreatedAt       time.Time
	StartedAt       *time.Time
	PhaseStartedAt  *time.Time
	FinishedAt      *time.Time
}
