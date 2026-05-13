package domain

import (
	"errors"
	"time"
)

// ErrInvalidAnalysisRequest indicates that an analysis request cannot be executed.
var ErrInvalidAnalysisRequest = errors.New("invalid analysis request")

// ErrInvalidAnalysisFilter indicates that analysis listing parameters are invalid.
var ErrInvalidAnalysisFilter = errors.New("invalid analysis filter")

// ErrAnalysisNotFound indicates that an analysis identifier does not exist.
var ErrAnalysisNotFound = errors.New("analysis not found")

// ErrInvalidChatQuestion indicates that a chat question payload is invalid.
var ErrInvalidChatQuestion = errors.New("invalid chat question")

// ErrQuestionOutOfScope indicates that a chat question is unrelated to the active analysis.
var ErrQuestionOutOfScope = errors.New("question out of analysis scope")

// ErrAnalysisContextNotFound indicates that cached analysis context is unavailable.
var ErrAnalysisContextNotFound = errors.New("analysis context not found")

// Severity describes the operational impact detected by an analysis.
type Severity string

const (
	// SeverityInfo represents informational findings.
	SeverityInfo Severity = "info"
	// SeverityLow represents low-impact findings.
	SeverityLow Severity = "low"
	// SeverityMedium represents medium-impact findings.
	SeverityMedium Severity = "medium"
	// SeverityHigh represents high-impact findings.
	SeverityHigh Severity = "high"
	// SeverityCritical represents critical findings.
	SeverityCritical Severity = "critical"
)

// Confidence describes how strongly the available evidence supports the analysis.
type Confidence string

const (
	// ConfidenceLow represents weak or incomplete evidence.
	ConfidenceLow Confidence = "low"
	// ConfidenceMedium represents enough evidence for a plausible conclusion.
	ConfidenceMedium Confidence = "medium"
	// ConfidenceHigh represents strong evidence for a conclusion.
	ConfidenceHigh Confidence = "high"
)

// SignalType identifies the kind of observability signal.
type SignalType string

const (
	// SignalLogs represents log evidence.
	SignalLogs SignalType = "logs"
	// SignalMetrics represents metric evidence.
	SignalMetrics SignalType = "metrics"
	// SignalTraces represents trace evidence.
	SignalTraces SignalType = "traces"
	// SignalAPM represents APM evidence.
	SignalAPM SignalType = "apm"
)

// TimeWindow defines the time range used to collect evidence.
type TimeWindow struct {
	Start time.Time
	End   time.Time
}

// AnalysisRequest describes the provider-agnostic intent for an observability analysis.
type AnalysisRequest struct {
	Goal             string
	TimeWindow       TimeWindow
	AffectedServices []string
	Signals          []SignalType
	Context          string
}

// Evidence describes a normalized observation used by the analysis engine.
//
// Fields are provider-agnostic: adapters must translate any provider-specific
// payload into these fields before returning evidence to the use case.
type Evidence struct {
	Signal     SignalType
	Service    string
	Source     string
	Name       string
	Summary    string
	Observed   time.Time
	Score      float64
	Unit       string
	Reference  string
	Provider   string
	Query      string
	Attributes map[string]string
}

// RootCauseHypothesis describes a possible cause and the evidence that supports it.
type RootCauseHypothesis struct {
	Cause      string
	Evidence   []string
	Confidence Confidence
}

// Recommendation describes an actionable next step for the investigation.
type Recommendation struct {
	Action    string
	Rationale string
	Priority  int
}

// AnalysisResult describes the normalized output of an observability analysis.
type AnalysisResult struct {
	ID                 string
	Summary            string
	Severity           Severity
	Confidence         Confidence
	AffectedServices   []string
	Evidence           []Evidence
	DetectedAnomalies  []string
	PossibleRootCauses []RootCauseHypothesis
	RecommendedActions []Recommendation
	CodeLevelInsights  []string
	MissingEvidence    []string
	CreatedAt          time.Time
}

// AnalysisListFilter describes provider-agnostic analysis list filters.
type AnalysisListFilter struct {
	Limit    int
	Offset   int
	Severity Severity
	Service  string
}

// AnalysisList describes a paginated list of stored analyses.
type AnalysisList struct {
	Items  []AnalysisResult
	Limit  int
	Offset int
	Total  int
}

// AnalysisContext describes the compact analysis state used for scoped follow-up chat.
type AnalysisContext struct {
	AnalysisID          string
	Summary             string
	Severity            Severity
	Confidence          Confidence
	AffectedServices    []string
	Evidence            []Evidence
	DetectedAnomalies   []string
	PossibleRootCauses  []RootCauseHypothesis
	RecommendedActions  []Recommendation
	CodeLevelInsights   []string
	MissingEvidence     []string
	AnalysisCompletedAt time.Time
}

// ChatQuestion describes a follow-up question about an existing analysis.
type ChatQuestion struct {
	AnalysisID string
	Question   string
}

// ChatAnswer describes a scoped answer for an active analysis.
type ChatAnswer struct {
	AnalysisID string
	Answer     string
	Evidence   []string
}

// ChatRole identifies who produced a persistent chat message.
type ChatRole string

const (
	// ChatRoleUser represents a persisted user question.
	ChatRoleUser ChatRole = "user"
	// ChatRoleAssistant represents a persisted assistant answer.
	ChatRoleAssistant ChatRole = "assistant"
)

// ChatMessage describes a persisted chat message for an analysis.
type ChatMessage struct {
	ID         string
	AnalysisID string
	Role       ChatRole
	Content    string
	Evidence   []string
	CreatedAt  time.Time
}

// NewAnalysisContext creates chat context from an analysis result.
func NewAnalysisContext(result AnalysisResult) AnalysisContext {
	return AnalysisContext{
		AnalysisID:          result.ID,
		Summary:             result.Summary,
		Severity:            result.Severity,
		Confidence:          result.Confidence,
		AffectedServices:    result.AffectedServices,
		Evidence:            result.Evidence,
		DetectedAnomalies:   result.DetectedAnomalies,
		PossibleRootCauses:  result.PossibleRootCauses,
		RecommendedActions:  result.RecommendedActions,
		CodeLevelInsights:   result.CodeLevelInsights,
		MissingEvidence:     result.MissingEvidence,
		AnalysisCompletedAt: result.CreatedAt,
	}
}
