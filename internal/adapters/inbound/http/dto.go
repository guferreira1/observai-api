package http

import (
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// WrapperDtoResponde wraps successful API responses in a stable frontend contract.
type WrapperDtoResponde[T any] struct {
	Data     T                `json:"data"`
	Metadata ResponseMetadata `json:"metadata"`
}

// ResponseMetadata describes metadata included with every successful response.
type ResponseMetadata struct {
	RequestID        string          `json:"requestId"`
	ProcessingTimeMs int64           `json:"processingTimeMs"`
	Provider         ProviderSummary `json:"provider"`
	Warnings         []string        `json:"warnings,omitempty"`
	Pagination       *Pagination     `json:"pagination,omitempty"`
}

// ProviderSummary describes provider information without leaking provider-specific response shape.
type ProviderSummary struct {
	Observability []string `json:"observability,omitempty"`
	LLM           string   `json:"llm,omitempty"`
	Mode          string   `json:"mode"`
}

// Pagination describes paginated response metadata.
type Pagination struct {
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Next   string `json:"next,omitempty"`
}

// ErrorResponse describes an API error.
type ErrorResponse struct {
	Code    string             `json:"code"`
	Message string             `json:"message"`
	Details []ErrorFieldDetail `json:"details,omitempty"`
}

// ErrorFieldDetail describes a per-field validation failure.
type ErrorFieldDetail struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message,omitempty"`
}

// HealthResponse describes service health.
type HealthResponse struct {
	Status string `json:"status"`
}

// AnalysisRequestDto describes a provider-agnostic analysis request.
type AnalysisRequestDto struct {
	Goal             string   `json:"goal" validate:"required"`
	TimeWindow       TimeDto  `json:"timeWindow"`
	AffectedServices []string `json:"affectedServices" validate:"dive,required"`
	Signals          []string `json:"signals" validate:"dive,oneof=logs metrics traces apm"`
	Context          string   `json:"context"`
}

// TimeDto describes a request time window.
type TimeDto struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// AnalysisResponseDto describes an analysis response.
type AnalysisResponseDto struct {
	ID                 string                   `json:"id"`
	Summary            string                   `json:"summary"`
	Severity           string                   `json:"severity"`
	Confidence         string                   `json:"confidence"`
	AffectedServices   []string                 `json:"affectedServices"`
	Evidence           []EvidenceDto            `json:"evidence"`
	DetectedAnomalies  []string                 `json:"detectedAnomalies"`
	PossibleRootCauses []RootCauseHypothesisDto `json:"possibleRootCauses"`
	RecommendedActions []RecommendationDto      `json:"recommendedActions"`
	CodeLevelInsights  []string                 `json:"codeLevelInsights"`
	MissingEvidence    []string                 `json:"missingEvidence"`
	CreatedAt          time.Time                `json:"createdAt"`
}

// AnalysisListResponseDto describes a paginated analysis list response.
type AnalysisListResponseDto struct {
	Items []AnalysisResponseDto `json:"items"`
}

// EvidenceDto describes normalized evidence returned to API clients.
type EvidenceDto struct {
	Signal     string            `json:"signal"`
	Service    string            `json:"service"`
	Source     string            `json:"source"`
	Name       string            `json:"name"`
	Summary    string            `json:"summary"`
	Observed   time.Time         `json:"observed"`
	Score      float64           `json:"score"`
	Unit       string            `json:"unit,omitempty"`
	Reference  string            `json:"reference,omitempty"`
	Provider   string            `json:"provider,omitempty"`
	Query      string            `json:"query,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// RootCauseHypothesisDto describes a possible root cause returned to API clients.
type RootCauseHypothesisDto struct {
	Cause      string   `json:"cause"`
	Evidence   []string `json:"evidence"`
	Confidence string   `json:"confidence"`
}

// RecommendationDto describes a recommended action returned to API clients.
type RecommendationDto struct {
	Action    string `json:"action"`
	Rationale string `json:"rationale"`
	Priority  int    `json:"priority"`
}

// AnalysisJobAcceptedDto describes a job acceptance response (HTTP 202).
type AnalysisJobAcceptedDto struct {
	JobID     string `json:"jobId"`
	Status    string `json:"status"`
	StatusURL string `json:"statusUrl"`
}

// AnalysisJobStatusDto describes the lifecycle state of an asynchronous analysis job.
type AnalysisJobStatusDto struct {
	JobID        string     `json:"jobId"`
	Status       string     `json:"status"`
	AnalysisID   string     `json:"analysisId,omitempty"`
	AnalysisURL  string     `json:"analysisUrl,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	Attempt      int        `json:"attempt"`
	CreatedAt    time.Time  `json:"createdAt"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

// ChatRequestDto describes a chat question for an active analysis.
type ChatRequestDto struct {
	Question string `json:"question" validate:"required"`
}

// ChatResponseDto describes a scoped chat answer.
type ChatResponseDto struct {
	AnalysisID string   `json:"analysisId"`
	Answer     string   `json:"answer"`
	Evidence   []string `json:"evidence"`
}

// ChatHistoryResponseDto describes persisted chat history for an analysis.
type ChatHistoryResponseDto struct {
	Messages []ChatMessageDto `json:"messages"`
}

// ChatMessageDto describes a persisted chat message returned to API clients.
type ChatMessageDto struct {
	ID         string    `json:"id"`
	AnalysisID string    `json:"analysisId"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	Evidence   []string  `json:"evidence,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

func toDomainAnalysisRequest(dto AnalysisRequestDto) domain.AnalysisRequest {
	signals := make([]domain.SignalType, 0, len(dto.Signals))
	for _, signal := range dto.Signals {
		signals = append(signals, domain.SignalType(signal))
	}

	return domain.AnalysisRequest{
		Goal: dto.Goal,
		TimeWindow: domain.TimeWindow{
			Start: dto.TimeWindow.Start,
			End:   dto.TimeWindow.End,
		},
		AffectedServices: dto.AffectedServices,
		Signals:          signals,
		Context:          dto.Context,
	}
}

func toAnalysisResponseDto(result domain.AnalysisResult) AnalysisResponseDto {
	evidence := make([]EvidenceDto, 0, len(result.Evidence))
	for _, item := range result.Evidence {
		evidence = append(evidence, EvidenceDto{
			Signal:     string(item.Signal),
			Service:    item.Service,
			Source:     item.Source,
			Name:       item.Name,
			Summary:    item.Summary,
			Observed:   item.Observed,
			Score:      item.Score,
			Unit:       item.Unit,
			Reference:  item.Reference,
			Provider:   item.Provider,
			Query:      item.Query,
			Attributes: item.Attributes,
		})
	}

	rootCauses := make([]RootCauseHypothesisDto, 0, len(result.PossibleRootCauses))
	for _, item := range result.PossibleRootCauses {
		rootCauses = append(rootCauses, RootCauseHypothesisDto{
			Cause:      item.Cause,
			Evidence:   item.Evidence,
			Confidence: string(item.Confidence),
		})
	}

	recommendations := make([]RecommendationDto, 0, len(result.RecommendedActions))
	for _, item := range result.RecommendedActions {
		recommendations = append(recommendations, RecommendationDto{
			Action:    item.Action,
			Rationale: item.Rationale,
			Priority:  item.Priority,
		})
	}

	return AnalysisResponseDto{
		ID:                 result.ID,
		Summary:            result.Summary,
		Severity:           string(result.Severity),
		Confidence:         string(result.Confidence),
		AffectedServices:   result.AffectedServices,
		Evidence:           evidence,
		DetectedAnomalies:  result.DetectedAnomalies,
		PossibleRootCauses: rootCauses,
		RecommendedActions: recommendations,
		CodeLevelInsights:  result.CodeLevelInsights,
		MissingEvidence:    result.MissingEvidence,
		CreatedAt:          result.CreatedAt,
	}
}

func toAnalysisListResponseDto(result domain.AnalysisList) AnalysisListResponseDto {
	items := make([]AnalysisResponseDto, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, toAnalysisResponseDto(item))
	}

	return AnalysisListResponseDto{Items: items}
}

func toAnalysisJobAcceptedDto(job domain.AnalysisJob) AnalysisJobAcceptedDto {
	return AnalysisJobAcceptedDto{
		JobID:     job.ID,
		Status:    string(job.Status),
		StatusURL: "/v1/jobs/" + job.ID,
	}
}

func toAnalysisJobStatusDto(job domain.AnalysisJob) AnalysisJobStatusDto {
	dto := AnalysisJobStatusDto{
		JobID:        job.ID,
		Status:       string(job.Status),
		AnalysisID:   job.AnalysisID,
		ErrorMessage: job.ErrorMessage,
		Attempt:      job.Attempt,
		CreatedAt:    job.CreatedAt,
		StartedAt:    job.StartedAt,
		FinishedAt:   job.FinishedAt,
	}
	if job.AnalysisID != "" {
		dto.AnalysisURL = "/v1/analyses/" + job.AnalysisID
	}
	return dto
}

func toChatResponseDto(answer domain.ChatAnswer) ChatResponseDto {
	return ChatResponseDto{
		AnalysisID: answer.AnalysisID,
		Answer:     answer.Answer,
		Evidence:   answer.Evidence,
	}
}

func toChatHistoryResponseDto(messages []domain.ChatMessage) ChatHistoryResponseDto {
	dtos := make([]ChatMessageDto, 0, len(messages))
	for _, message := range messages {
		dtos = append(dtos, ChatMessageDto{
			ID:         message.ID,
			AnalysisID: message.AnalysisID,
			Role:       string(message.Role),
			Content:    message.Content,
			Evidence:   message.Evidence,
			CreatedAt:  message.CreatedAt,
		})
	}

	return ChatHistoryResponseDto{Messages: dtos}
}
