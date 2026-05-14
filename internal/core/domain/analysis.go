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

// ErrProviderNotConfigured indicates that an outbound provider adapter has no
// real backend wired and cannot serve requests. Returned by null adapters in
// place of synthetic data when the operator runs the API in local/dev mode
// without configuring the corresponding observability or LLM provider.
var ErrProviderNotConfigured = errors.New("provider not configured")

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

var severityRanks = map[Severity]int{
	SeverityInfo:     0,
	SeverityLow:      1,
	SeverityMedium:   2,
	SeverityHigh:     3,
	SeverityCritical: 4,
}

// IsValidSeverity reports whether severity belongs to the public severity enum.
func IsValidSeverity(severity Severity) bool {
	_, ok := severityRanks[severity]
	return ok
}

// NormalizeSeverity returns severity when valid or SeverityInfo otherwise.
func NormalizeSeverity(severity Severity) Severity {
	if IsValidSeverity(severity) {
		return severity
	}
	return SeverityInfo
}

// SeverityRank returns the relative operational impact of severity.
func SeverityRank(severity Severity) int {
	return severityRanks[NormalizeSeverity(severity)]
}

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

var signalSet = map[SignalType]struct{}{
	SignalLogs:    {},
	SignalMetrics: {},
	SignalTraces:  {},
	SignalAPM:     {},
}

// IsValidSignal reports whether signal is part of the public signal taxonomy.
func IsValidSignal(signal SignalType) bool {
	_, ok := signalSet[signal]
	return ok
}

var analysisListSortSet = map[AnalysisListSort]struct{}{
	SortByCreatedAt:  {},
	SortBySeverity:   {},
	SortByConfidence: {},
}

// IsValidAnalysisListSort reports whether sort is a supported ordering column.
func IsValidAnalysisListSort(sort AnalysisListSort) bool {
	_, ok := analysisListSortSet[sort]
	return ok
}

var analysisListOrderSet = map[AnalysisListOrder]struct{}{
	OrderAsc:  {},
	OrderDesc: {},
}

// IsValidAnalysisListOrder reports whether order is a supported ordering direction.
func IsValidAnalysisListOrder(order AnalysisListOrder) bool {
	_, ok := analysisListOrderSet[order]
	return ok
}

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
//
// ID is assigned by the analysis use case after evidence collection and is
// stable within the scope of a single analysis. It is the value clients use
// to cite evidence in chat answers and to link root-cause hypotheses to
// supporting observations.
type Evidence struct {
	ID         string
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
//
// TraceID, when present, is the distributed trace identifier captured
// during evidence collection. Trace providers (Jaeger, Tempo, OTLP) use
// this value to retrieve spans for `GET /v1/analyses/{id}/traces`.
type AnalysisResult struct {
	ID                 string
	TraceID            string
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

// AnalysisListSort identifies which column the list endpoint orders by.
type AnalysisListSort string

const (
	// SortByCreatedAt orders by analysis creation time. Default.
	SortByCreatedAt AnalysisListSort = "createdAt"
	// SortBySeverity orders by severity rank.
	SortBySeverity AnalysisListSort = "severity"
	// SortByConfidence orders by confidence rank.
	SortByConfidence AnalysisListSort = "confidence"
)

// AnalysisListOrder identifies the direction of the list ordering.
type AnalysisListOrder string

const (
	// OrderAsc sorts in ascending order.
	OrderAsc AnalysisListOrder = "asc"
	// OrderDesc sorts in descending order. Default for createdAt and severity.
	OrderDesc AnalysisListOrder = "desc"
)

// AnalysisListFilter describes provider-agnostic analysis list filters.
type AnalysisListFilter struct {
	Limit    int
	Offset   int
	Severity Severity
	Service  string
	Signal   SignalType
	Provider string
	From     time.Time
	To       time.Time
	Query    string
	Sort     AnalysisListSort
	Order    AnalysisListOrder
}

// AnalysisList describes a paginated list of stored analyses.
type AnalysisList struct {
	Items  []AnalysisResult
	Limit  int
	Offset int
	Total  int
}

// AnalysisStatsFilter describes the optional bounds applied to aggregated stats.
type AnalysisStatsFilter struct {
	From     time.Time
	To       time.Time
	Service  string
	Severity Severity
}

// AnalysisStats describes aggregated analysis counts for the requested filter.
//
// All counts include analyses that match the filter. Distributions are
// returned as full maps with zero entries omitted so the frontend can render
// stable charts without re-deriving keys.
type AnalysisStats struct {
	Total               int
	BySeverity          map[Severity]int
	ByConfidence        map[Confidence]int
	TopAffectedServices []AnalysisStatsServiceCount
	TrendBuckets        []AnalysisStatsTrendBucket
	From                time.Time
	To                  time.Time
}

// AnalysisStatsServiceCount describes how many analyses mention a given service.
type AnalysisStatsServiceCount struct {
	Service string
	Count   int
}

// AnalysisStatsTrendBucket describes the count of analyses inside a time bucket.
type AnalysisStatsTrendBucket struct {
	BucketStart time.Time
	Count       int
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
	Citations  []ChatCitation
}

// ChatCitation references evidence that supports a scoped chat answer.
//
// EvidenceID matches the stable Evidence.ID inside the parent analysis.
// Snippet is an optional human-readable preview the frontend can render
// next to the citation without re-fetching the evidence list.
type ChatCitation struct {
	EvidenceID string
	Snippet    string
}

// ChatFeedback describes user feedback for a persisted chat message.
//
// Useful captures the binary thumbs-up/thumbs-down signal; Reason is an
// optional free-form note. AnalysisID and MessageID identify the message the
// feedback applies to. Authoritative feedback must be persisted via a
// repository implementation so it can feed quality dashboards.
type ChatFeedback struct {
	AnalysisID string
	MessageID  string
	Useful     bool
	Reason     string
	CreatedAt  time.Time
}

// ErrChatMessageNotFound indicates that the supplied chat message does not exist.
var ErrChatMessageNotFound = errors.New("chat message not found")

// ChatHistoryFilter describes pagination options for chat history retrieval.
//
// Before is exclusive: only messages strictly older than the cursor are
// returned. A zero Before means "start from the most recent message".
// Limit zero or negative falls back to a reasonable default in the use case.
type ChatHistoryFilter struct {
	Before time.Time
	Limit  int
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
	Citations  []ChatCitation
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
