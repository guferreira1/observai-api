package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisRepository stores analyses in memory for local execution and deterministic tests.
type AnalysisRepository struct {
	mu       sync.RWMutex
	analyses map[string]domain.AnalysisResult
	messages map[string][]domain.ChatMessage
	nextID   int64
}

// NewAnalysisRepository creates an in-memory analysis repository.
func NewAnalysisRepository() *AnalysisRepository {
	return &AnalysisRepository{
		analyses: make(map[string]domain.AnalysisResult),
		messages: make(map[string][]domain.ChatMessage),
	}
}

// Save stores an analysis result by identifier.
func (repository *AnalysisRepository) Save(_ context.Context, result domain.AnalysisResult) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.analyses[result.ID] = result
	return nil
}

// Find returns an analysis result by identifier.
func (repository *AnalysisRepository) Find(_ context.Context, id string) (domain.AnalysisResult, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()

	result, ok := repository.analyses[id]
	if !ok {
		return domain.AnalysisResult{}, fmt.Errorf("%w: %s", domain.ErrAnalysisNotFound, id)
	}

	return result, nil
}

// ListAnalyses returns analyses ordered by the supplied filter.
func (repository *AnalysisRepository) ListAnalyses(_ context.Context, filter domain.AnalysisListFilter) (domain.AnalysisList, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()

	filtered := make([]domain.AnalysisResult, 0, len(repository.analyses))
	for _, result := range repository.analyses {
		if !matchesAnalysisFilter(result, filter) {
			continue
		}
		filtered = append(filtered, result)
	}

	sortAnalyses(filtered, filter)

	total := len(filtered)
	start := min(filter.Offset, total)
	end := min(start+filter.Limit, total)
	items := make([]domain.AnalysisResult, end-start)
	copy(items, filtered[start:end])

	return domain.AnalysisList{
		Items:  items,
		Limit:  filter.Limit,
		Offset: filter.Offset,
		Total:  total,
	}, nil
}

type analysisLess func(left, right domain.AnalysisResult) bool

var analysisSortStrategies = map[domain.AnalysisListSort]analysisLess{
	domain.SortByCreatedAt: func(left, right domain.AnalysisResult) bool {
		if left.CreatedAt.Equal(right.CreatedAt) {
			return left.ID < right.ID
		}
		return left.CreatedAt.Before(right.CreatedAt)
	},
	domain.SortBySeverity: func(left, right domain.AnalysisResult) bool {
		leftRank := domain.SeverityRank(left.Severity)
		rightRank := domain.SeverityRank(right.Severity)
		if leftRank == rightRank {
			return left.ID < right.ID
		}
		return leftRank < rightRank
	},
	domain.SortByConfidence: func(left, right domain.AnalysisResult) bool {
		leftRank := confidenceRank(left.Confidence)
		rightRank := confidenceRank(right.Confidence)
		if leftRank == rightRank {
			return left.ID < right.ID
		}
		return leftRank < rightRank
	},
}

func sortAnalyses(items []domain.AnalysisResult, filter domain.AnalysisListFilter) {
	less, ok := analysisSortStrategies[filter.Sort]
	if !ok {
		less = analysisSortStrategies[domain.SortByCreatedAt]
	}

	descending := filter.Order != domain.OrderAsc
	sort.Slice(items, func(left, right int) bool {
		if descending {
			return less(items[right], items[left])
		}
		return less(items[left], items[right])
	})
}

func confidenceRank(confidence domain.Confidence) int {
	switch confidence {
	case domain.ConfidenceLow:
		return 1
	case domain.ConfidenceMedium:
		return 2
	case domain.ConfidenceHigh:
		return 3
	default:
		return 0
	}
}

// SaveExchange stores a user question and assistant answer for an analysis.
func (repository *AnalysisRepository) SaveExchange(_ context.Context, question domain.ChatMessage, answer domain.ChatMessage) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.nextID++
	question.ID = strconv.FormatInt(repository.nextID, 10)
	if question.CreatedAt.IsZero() {
		question.CreatedAt = time.Now().UTC()
	}

	repository.nextID++
	answer.ID = strconv.FormatInt(repository.nextID, 10)
	if answer.CreatedAt.IsZero() {
		answer.CreatedAt = time.Now().UTC()
	}

	repository.messages[question.AnalysisID] = append(repository.messages[question.AnalysisID], question, answer)

	return nil
}

// List returns persisted chat messages for an analysis honoring the supplied filter.
func (repository *AnalysisRepository) List(_ context.Context, analysisID string, filter domain.ChatHistoryFilter) ([]domain.ChatMessage, error) {
	repository.mu.RLock()
	defer repository.mu.RUnlock()

	messages := repository.messages[analysisID]
	filtered := make([]domain.ChatMessage, 0, len(messages))
	for _, message := range messages {
		if !filter.Before.IsZero() && !message.CreatedAt.Before(filter.Before) {
			continue
		}
		filtered = append(filtered, message)
	}

	if filter.Limit > 0 && len(filtered) > filter.Limit {
		filtered = filtered[len(filtered)-filter.Limit:]
	}

	return filtered, nil
}

func matchesAnalysisFilter(result domain.AnalysisResult, filter domain.AnalysisListFilter) bool {
	for _, predicate := range analysisFilterPredicates {
		if !predicate(result, filter) {
			return false
		}
	}
	return true
}

type analysisFilterPredicate func(result domain.AnalysisResult, filter domain.AnalysisListFilter) bool

var analysisFilterPredicates = []analysisFilterPredicate{
	severityMatches,
	serviceMatches,
	signalMatches,
	providerMatches,
	timeWindowMatches,
	textQueryMatches,
}

func severityMatches(result domain.AnalysisResult, filter domain.AnalysisListFilter) bool {
	return filter.Severity == "" || result.Severity == filter.Severity
}

func serviceMatches(result domain.AnalysisResult, filter domain.AnalysisListFilter) bool {
	return filter.Service == "" || containsString(result.AffectedServices, filter.Service)
}

func signalMatches(result domain.AnalysisResult, filter domain.AnalysisListFilter) bool {
	if filter.Signal == "" {
		return true
	}
	for _, evidence := range result.Evidence {
		if evidence.Signal == filter.Signal {
			return true
		}
	}
	return false
}

func providerMatches(result domain.AnalysisResult, filter domain.AnalysisListFilter) bool {
	if filter.Provider == "" {
		return true
	}
	for _, evidence := range result.Evidence {
		if evidence.Provider == filter.Provider {
			return true
		}
	}
	return false
}

func timeWindowMatches(result domain.AnalysisResult, filter domain.AnalysisListFilter) bool {
	if !filter.From.IsZero() && result.CreatedAt.Before(filter.From) {
		return false
	}
	if !filter.To.IsZero() && result.CreatedAt.After(filter.To) {
		return false
	}
	return true
}

func textQueryMatches(result domain.AnalysisResult, filter domain.AnalysisListFilter) bool {
	if filter.Query == "" {
		return true
	}
	needle := strings.ToLower(filter.Query)
	return strings.Contains(strings.ToLower(result.Summary), needle) ||
		strings.Contains(strings.ToLower(result.ID), needle)
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}

	return false
}
