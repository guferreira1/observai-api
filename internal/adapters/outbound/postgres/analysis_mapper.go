package postgres

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/postgres/sqlc"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

func toSaveAnalysisParams(result domain.AnalysisResult) (sqlc.SaveAnalysisParams, error) {
	evidence, err := marshalSlice(result.Evidence)
	if err != nil {
		return sqlc.SaveAnalysisParams{}, fmt.Errorf("marshal analysis evidence: %w", err)
	}

	rootCauses, err := marshalSlice(result.PossibleRootCauses)
	if err != nil {
		return sqlc.SaveAnalysisParams{}, fmt.Errorf("marshal analysis root causes: %w", err)
	}

	recommendations, err := marshalSlice(result.RecommendedActions)
	if err != nil {
		return sqlc.SaveAnalysisParams{}, fmt.Errorf("marshal analysis recommendations: %w", err)
	}

	return sqlc.SaveAnalysisParams{
		ID:                 result.ID,
		Summary:            result.Summary,
		Severity:           string(result.Severity),
		Confidence:         string(result.Confidence),
		AffectedServices:   nonNilStrings(result.AffectedServices),
		Evidence:           evidence,
		DetectedAnomalies:  nonNilStrings(result.DetectedAnomalies),
		PossibleRootCauses: rootCauses,
		RecommendedActions: recommendations,
		CodeLevelInsights:  nonNilStrings(result.CodeLevelInsights),
		MissingEvidence:    nonNilStrings(result.MissingEvidence),
		CreatedAt: pgtype.Timestamptz{
			Time:  result.CreatedAt,
			Valid: true,
		},
	}, nil
}

func toDomainAnalysisResult(row sqlc.FindAnalysisRow) (domain.AnalysisResult, error) {
	var result domain.AnalysisResult

	if err := json.Unmarshal(row.Evidence, &result.Evidence); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("unmarshal analysis evidence: %w", err)
	}

	if err := json.Unmarshal(row.PossibleRootCauses, &result.PossibleRootCauses); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("unmarshal analysis root causes: %w", err)
	}

	if err := json.Unmarshal(row.RecommendedActions, &result.RecommendedActions); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("unmarshal analysis recommendations: %w", err)
	}

	result.ID = row.ID
	result.Summary = row.Summary
	result.Severity = domain.Severity(row.Severity)
	result.Confidence = domain.Confidence(row.Confidence)
	result.AffectedServices = row.AffectedServices
	result.DetectedAnomalies = row.DetectedAnomalies
	result.CodeLevelInsights = row.CodeLevelInsights
	result.MissingEvidence = row.MissingEvidence
	result.CreatedAt = row.CreatedAt.Time.UTC()

	return result, nil
}

func toDomainAnalysisResultFromList(row sqlc.ListAnalysesRow) (domain.AnalysisResult, error) {
	var result domain.AnalysisResult

	if err := json.Unmarshal(row.Evidence, &result.Evidence); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("unmarshal analysis evidence: %w", err)
	}

	if err := json.Unmarshal(row.PossibleRootCauses, &result.PossibleRootCauses); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("unmarshal analysis root causes: %w", err)
	}

	if err := json.Unmarshal(row.RecommendedActions, &result.RecommendedActions); err != nil {
		return domain.AnalysisResult{}, fmt.Errorf("unmarshal analysis recommendations: %w", err)
	}

	result.ID = row.ID
	result.Summary = row.Summary
	result.Severity = domain.Severity(row.Severity)
	result.Confidence = domain.Confidence(row.Confidence)
	result.AffectedServices = row.AffectedServices
	result.DetectedAnomalies = row.DetectedAnomalies
	result.CodeLevelInsights = row.CodeLevelInsights
	result.MissingEvidence = row.MissingEvidence
	result.CreatedAt = row.CreatedAt.Time.UTC()

	return result, nil
}

type analysisFilterParams struct {
	severity pgtype.Text
	service  pgtype.Text
	signal   pgtype.Text
	provider pgtype.Text
	fromAt   pgtype.Timestamptz
	toAt     pgtype.Timestamptz
	query    pgtype.Text
	sortBy   string
	orderAsc bool
}

type analysisStatsParams struct {
	severity pgtype.Text
	service  pgtype.Text
	fromAt   pgtype.Timestamptz
	toAt     pgtype.Timestamptz
}

func toAnalysisStatsParams(filter domain.AnalysisStatsFilter) analysisStatsParams {
	return analysisStatsParams{
		severity: optionalText(string(filter.Severity)),
		service:  optionalText(filter.Service),
		fromAt:   optionalTimestamp(timeOrNil(filter.From)),
		toAt:     optionalTimestamp(timeOrNil(filter.To)),
	}
}

func toAnalysisFilterParams(filter domain.AnalysisListFilter) analysisFilterParams {
	sortBy := string(filter.Sort)
	if sortBy == "" {
		sortBy = string(domain.SortByCreatedAt)
	}
	return analysisFilterParams{
		severity: optionalText(string(filter.Severity)),
		service:  optionalText(filter.Service),
		signal:   optionalText(string(filter.Signal)),
		provider: optionalText(filter.Provider),
		fromAt:   optionalTimestamp(timeOrNil(filter.From)),
		toAt:     optionalTimestamp(timeOrNil(filter.To)),
		query:    optionalText(filter.Query),
		sortBy:   sortBy,
		orderAsc: filter.Order == domain.OrderAsc,
	}
}

func toCreateChatMessageParams(message domain.ChatMessage) (sqlc.CreateChatMessageParams, error) {
	evidence, err := marshalSlice(message.Evidence)
	if err != nil {
		return sqlc.CreateChatMessageParams{}, fmt.Errorf("marshal chat evidence: %w", err)
	}

	return sqlc.CreateChatMessageParams{
		AnalysisID: message.AnalysisID,
		Role:       string(message.Role),
		Content:    message.Content,
		Evidence:   evidence,
	}, nil
}

func toDomainChatMessage(row sqlc.AnalysisChatMessage) (domain.ChatMessage, error) {
	var evidence []string
	if err := json.Unmarshal(row.Evidence, &evidence); err != nil {
		return domain.ChatMessage{}, fmt.Errorf("unmarshal chat evidence: %w", err)
	}

	return domain.ChatMessage{
		ID:         strconv.FormatInt(row.ID, 10),
		AnalysisID: row.AnalysisID,
		Role:       domain.ChatRole(row.Role),
		Content:    row.Content,
		Evidence:   evidence,
		CreatedAt:  row.CreatedAt.Time.UTC(),
	}, nil
}

func marshalSlice[T any](values []T) ([]byte, error) {
	if values == nil {
		values = []T{}
	}

	return json.Marshal(values)
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}

func optionalText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}

	return pgtype.Text{String: value, Valid: true}
}
