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
	Total  int    `json:"total"`
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

// CapabilitiesResponse describes non-sensitive runtime capabilities exposed to clients.
//
// Clients use this payload to render the runtime panel, gate features that depend on
// optional providers and decide which signals or LLM features are available.
type CapabilitiesResponse struct {
	Mode          string               `json:"mode"`
	Version       string               `json:"version,omitempty"`
	LLM           CapabilityLLM        `json:"llm"`
	Observability []CapabilityProvider `json:"observability"`
	Limits        CapabilityLimits     `json:"limits"`
}

// CapabilityLLM describes the active LLM adapter without leaking secrets.
type CapabilityLLM struct {
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
}

// CapabilityProvider describes an active observability adapter and the signals it supports.
type CapabilityProvider struct {
	Provider string   `json:"provider"`
	Signals  []string `json:"signals"`
}

// CapabilityLimits exposes runtime guards that affect client request shape.
type CapabilityLimits struct {
	HTTPRequestTimeoutMs int64   `json:"httpRequestTimeoutMs"`
	HTTPMaxBodyBytes     int64   `json:"httpMaxBodyBytes"`
	RateLimitRPS         float64 `json:"rateLimitRps,omitempty"`
	RateLimitBurst       int     `json:"rateLimitBurst,omitempty"`
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

// ServicesResponseDto lists services derived from stored analyses for autocomplete.
type ServicesResponseDto struct {
	Items []string `json:"items"`
}

// AnalysisStatsResponseDto describes aggregated counts for a stats request.
type AnalysisStatsResponseDto struct {
	Total               int                       `json:"total"`
	BySeverity          map[string]int            `json:"bySeverity"`
	ByConfidence        map[string]int            `json:"byConfidence"`
	TopAffectedServices []AnalysisStatsServiceDto `json:"topAffectedServices"`
	TrendBuckets        []AnalysisStatsTrendDto   `json:"trendBuckets"`
	From                *time.Time                `json:"from,omitempty"`
	To                  *time.Time                `json:"to,omitempty"`
}

// AnalysisStatsServiceDto describes a single (service, count) row.
type AnalysisStatsServiceDto struct {
	Service string `json:"service"`
	Count   int    `json:"count"`
}

// AnalysisStatsTrendDto describes a single (bucketStart, count) row.
type AnalysisStatsTrendDto struct {
	BucketStart time.Time `json:"bucketStart"`
	Count       int       `json:"count"`
}

// EvidenceDto describes normalized evidence returned to API clients.
type EvidenceDto struct {
	ID             string            `json:"id"`
	Signal         string            `json:"signal"`
	Service        string            `json:"service"`
	Source         string            `json:"source"`
	Name           string            `json:"name"`
	Summary        string            `json:"summary"`
	Observed       time.Time         `json:"observed"`
	Score          float64           `json:"score"`
	Confidence     float64           `json:"confidence,omitempty"`
	Unit           string            `json:"unit,omitempty"`
	Reference      string            `json:"reference,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Query          string            `json:"query,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	RedactedFields []string          `json:"redactedFields,omitempty"`
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
	JobID           string     `json:"jobId"`
	Status          string     `json:"status"`
	Phase           string     `json:"phase"`
	ProgressPercent int        `json:"progressPercent"`
	AnalysisID      string     `json:"analysisId,omitempty"`
	AnalysisURL     string     `json:"analysisUrl,omitempty"`
	ErrorMessage    string     `json:"errorMessage,omitempty"`
	Attempt         int        `json:"attempt"`
	CreatedAt       time.Time  `json:"createdAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	PhaseStartedAt  *time.Time `json:"phaseStartedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
}

// TraceInsightsResponseDto describes structured trace spans and derived insights.
type TraceInsightsResponseDto struct {
	Spans               []SpanDto                `json:"spans"`
	CriticalPathSpanIDs []string                 `json:"criticalPathSpanIds"`
	SlowestSpanIDs      []string                 `json:"slowestSpanIds"`
	DependencyEdges     []TraceDependencyEdgeDto `json:"dependencyEdges"`
}

// SpanDto describes a single normalized span returned to API clients.
type SpanDto struct {
	TraceID      string            `json:"traceId"`
	SpanID       string            `json:"spanId"`
	ParentSpanID string            `json:"parentSpanId,omitempty"`
	Service      string            `json:"service"`
	Operation    string            `json:"operation"`
	StartTime    time.Time         `json:"startTime"`
	DurationMs   float64           `json:"durationMs"`
	SelfTimeMs   float64           `json:"selfTimeMs"`
	Status       string            `json:"status"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// TraceDependencyEdgeDto describes an aggregated service dependency.
type TraceDependencyEdgeDto struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	CallCount int     `json:"callCount"`
	P95Ms     float64 `json:"p95Ms"`
}

// ChatRequestDto describes a chat question for an active analysis.
type ChatRequestDto struct {
	Question string `json:"question" validate:"required"`
}

// ChatResponseDto describes a scoped chat answer.
type ChatResponseDto struct {
	AnalysisID string            `json:"analysisId"`
	Answer     string            `json:"answer"`
	Evidence   []string          `json:"evidence"`
	Citations  []ChatCitationDto `json:"citations,omitempty"`
}

// ChatCitationDto describes a single citation tying an answer fragment to evidence.
type ChatCitationDto struct {
	EvidenceID string `json:"evidenceId"`
	Snippet    string `json:"snippet,omitempty"`
}

// ChatFeedbackRequestDto describes user feedback for a previously delivered assistant message.
type ChatFeedbackRequestDto struct {
	Useful *bool  `json:"useful" validate:"required"`
	Reason string `json:"reason,omitempty"`
}

// ChatFeedbackResponseDto echoes the persisted feedback.
type ChatFeedbackResponseDto struct {
	AnalysisID string    `json:"analysisId"`
	MessageID  string    `json:"messageId"`
	Useful     bool      `json:"useful"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ChatHistoryResponseDto describes persisted chat history for an analysis.
type ChatHistoryResponseDto struct {
	Messages []ChatMessageDto `json:"messages"`
}

// ChatMessageDto describes a persisted chat message returned to API clients.
type ChatMessageDto struct {
	ID         string            `json:"id"`
	AnalysisID string            `json:"analysisId"`
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	Evidence   []string          `json:"evidence,omitempty"`
	Citations  []ChatCitationDto `json:"citations,omitempty"`
	CreatedAt  time.Time         `json:"createdAt"`
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
			ID:             item.ID,
			Signal:         string(item.Signal),
			Service:        item.Service,
			Source:         item.Source,
			Name:           item.Name,
			Summary:        item.Summary,
			Observed:       item.Observed,
			Score:          item.Score,
			Confidence:     item.Confidence,
			Unit:           item.Unit,
			Reference:      item.Reference,
			Provider:       item.Provider,
			Query:          item.Query,
			Attributes:     item.Attributes,
			RedactedFields: item.RedactedFields,
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

func toAnalysisStatsResponseDto(stats domain.AnalysisStats) AnalysisStatsResponseDto {
	bySeverity := make(map[string]int, len(stats.BySeverity))
	for severity, count := range stats.BySeverity {
		bySeverity[string(severity)] = count
	}

	byConfidence := make(map[string]int, len(stats.ByConfidence))
	for confidence, count := range stats.ByConfidence {
		byConfidence[string(confidence)] = count
	}

	services := make([]AnalysisStatsServiceDto, 0, len(stats.TopAffectedServices))
	for _, item := range stats.TopAffectedServices {
		services = append(services, AnalysisStatsServiceDto{Service: item.Service, Count: item.Count})
	}

	buckets := make([]AnalysisStatsTrendDto, 0, len(stats.TrendBuckets))
	for _, bucket := range stats.TrendBuckets {
		buckets = append(buckets, AnalysisStatsTrendDto{
			BucketStart: bucket.BucketStart,
			Count:       bucket.Count,
		})
	}

	dto := AnalysisStatsResponseDto{
		Total:               stats.Total,
		BySeverity:          bySeverity,
		ByConfidence:        byConfidence,
		TopAffectedServices: services,
		TrendBuckets:        buckets,
	}
	if !stats.From.IsZero() {
		from := stats.From
		dto.From = &from
	}
	if !stats.To.IsZero() {
		to := stats.To
		dto.To = &to
	}
	return dto
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
	phase := string(job.Phase)
	if phase == "" {
		phase = string(domain.PhaseQueued)
	}
	dto := AnalysisJobStatusDto{
		JobID:           job.ID,
		Status:          string(job.Status),
		Phase:           phase,
		ProgressPercent: job.ProgressPercent,
		AnalysisID:      job.AnalysisID,
		ErrorMessage:    job.ErrorMessage,
		Attempt:         job.Attempt,
		CreatedAt:       job.CreatedAt,
		StartedAt:       job.StartedAt,
		PhaseStartedAt:  job.PhaseStartedAt,
		FinishedAt:      job.FinishedAt,
	}
	if job.AnalysisID != "" {
		dto.AnalysisURL = "/v1/analyses/" + job.AnalysisID
	}
	return dto
}

func toTraceInsightsResponseDto(insights domain.TraceInsights) TraceInsightsResponseDto {
	spans := make([]SpanDto, 0, len(insights.Spans))
	for _, span := range insights.Spans {
		spans = append(spans, SpanDto{
			TraceID:      span.TraceID,
			SpanID:       span.SpanID,
			ParentSpanID: span.ParentSpanID,
			Service:      span.Service,
			Operation:    span.Operation,
			StartTime:    span.StartTime,
			DurationMs:   span.DurationMs,
			SelfTimeMs:   span.SelfTimeMs,
			Status:       string(span.Status),
			Attributes:   span.Attributes,
		})
	}

	edges := make([]TraceDependencyEdgeDto, 0, len(insights.DependencyEdges))
	for _, edge := range insights.DependencyEdges {
		edges = append(edges, TraceDependencyEdgeDto{
			From:      edge.From,
			To:        edge.To,
			CallCount: edge.CallCount,
			P95Ms:     edge.P95Ms,
		})
	}

	return TraceInsightsResponseDto{
		Spans:               spans,
		CriticalPathSpanIDs: ensureStringSlice(insights.CriticalPathSpanIDs),
		SlowestSpanIDs:      ensureStringSlice(insights.SlowestSpanIDs),
		DependencyEdges:     edges,
	}
}

func ensureStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func toChatResponseDto(answer domain.ChatAnswer) ChatResponseDto {
	return ChatResponseDto{
		AnalysisID: answer.AnalysisID,
		Answer:     answer.Answer,
		Evidence:   answer.Evidence,
		Citations:  toChatCitationDtos(answer.Citations),
	}
}

func toChatCitationDtos(citations []domain.ChatCitation) []ChatCitationDto {
	if len(citations) == 0 {
		return nil
	}
	dtos := make([]ChatCitationDto, 0, len(citations))
	for _, citation := range citations {
		dtos = append(dtos, ChatCitationDto{
			EvidenceID: citation.EvidenceID,
			Snippet:    citation.Snippet,
		})
	}
	return dtos
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
			Citations:  toChatCitationDtos(message.Citations),
			CreatedAt:  message.CreatedAt,
		})
	}

	return ChatHistoryResponseDto{Messages: dtos}
}
