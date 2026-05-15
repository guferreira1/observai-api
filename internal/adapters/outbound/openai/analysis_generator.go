package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	"github.com/guferreira1/observai-api/internal/core/domain"
)

const analysisPromptName = "observability-analysis-agent"

// AnalysisGenerator turns normalized evidence into an analysis result via OpenAI.
type AnalysisGenerator struct {
	client *Client
	loader prompts.Loader
}

// NewAnalysisGenerator builds an OpenAI-backed analysis generator.
func NewAnalysisGenerator(client *Client, loader prompts.Loader) *AnalysisGenerator {
	return &AnalysisGenerator{client: client, loader: loader}
}

type analysisPayload struct {
	Summary            string                  `json:"summary"`
	Severity           string                  `json:"severity"`
	Confidence         string                  `json:"confidence"`
	AffectedServices   []string                `json:"affectedServices"`
	DetectedAnomalies  []string                `json:"detectedAnomalies"`
	PossibleRootCauses []rootCausePayload      `json:"possibleRootCauses"`
	RecommendedActions []recommendationPayload `json:"recommendedActions"`
	CodeLevelInsights  []string                `json:"codeLevelInsights"`
	MissingEvidence    []string                `json:"missingEvidence"`
}

type rootCausePayload struct {
	Cause      string   `json:"cause"`
	Evidence   []string `json:"evidence"`
	Confidence string   `json:"confidence"`
}

type recommendationPayload struct {
	Action      string   `json:"action"`
	Rationale   string   `json:"rationale"`
	Priority    int      `json:"priority"`
	EvidenceIDs []string `json:"evidenceIds"`
}

// Generate renders the analysis prompt, sends the request and decodes the
// JSON response into a domain.AnalysisResult.
func (generator *AnalysisGenerator) Generate(ctx context.Context, request domain.AnalysisRequest, evidence []domain.Evidence) (domain.AnalysisResult, error) {
	system, err := generator.loader.Load(analysisPromptName)
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("load analysis prompt: %w", err)
	}

	userMessage, err := buildAnalysisUserMessage(request, evidence)
	if err != nil {
		return domain.AnalysisResult{}, err
	}

	content, err := generator.client.Chat(ctx, ChatRequest{
		System:         system,
		Messages:       []Message{{Role: "user", Content: userMessage}},
		JSONOutput:     true,
		TemperatureSet: true,
		Temperature:    0,
	})
	if err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("openai analysis call: %w", err)
	}

	return decodeAnalysisPayload(content, evidence)
}

func buildAnalysisUserMessage(request domain.AnalysisRequest, evidence []domain.Evidence) (string, error) {
	body := map[string]any{
		"goal":             request.Goal,
		"timeWindow":       map[string]any{"start": request.TimeWindow.Start, "end": request.TimeWindow.End},
		"affectedServices": request.AffectedServices,
		"signals":          request.Signals,
		"context":          request.Context,
		"evidence":         evidence,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode analysis user payload: %w", err)
	}

	var buffer strings.Builder
	buffer.WriteString("Return ONLY a JSON object matching the documented analysis schema. Input:\n")
	buffer.Write(encoded)
	return buffer.String(), nil
}

func decodeAnalysisPayload(content string, evidence []domain.Evidence) (domain.AnalysisResult, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return domain.AnalysisResult{}, fmt.Errorf("openai returned empty analysis content")
	}

	var payload analysisPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("decode openai analysis json: %w", err)
	}

	rootCauses := make([]domain.RootCauseHypothesis, 0, len(payload.PossibleRootCauses))
	for _, item := range payload.PossibleRootCauses {
		rootCauses = append(rootCauses, domain.RootCauseHypothesis{
			Cause:      item.Cause,
			Evidence:   item.Evidence,
			Confidence: domain.Confidence(item.Confidence),
		})
	}

	recommendations := make([]domain.Recommendation, 0, len(payload.RecommendedActions))
	for _, item := range payload.RecommendedActions {
		recommendations = append(recommendations, domain.Recommendation{
			Action:      item.Action,
			Rationale:   item.Rationale,
			Priority:    item.Priority,
			EvidenceIDs: item.EvidenceIDs,
		})
	}

	return domain.AnalysisResult{
		Summary:            payload.Summary,
		Severity:           domain.Severity(payload.Severity),
		Confidence:         domain.Confidence(payload.Confidence),
		AffectedServices:   payload.AffectedServices,
		Evidence:           evidence,
		DetectedAnomalies:  payload.DetectedAnomalies,
		PossibleRootCauses: rootCauses,
		RecommendedActions: recommendations,
		CodeLevelInsights:  payload.CodeLevelInsights,
		MissingEvidence:    payload.MissingEvidence,
	}, nil
}
