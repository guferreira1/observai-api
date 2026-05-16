package usecase

import (
	"strings"
	"unicode"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

func newOutOfScopeChatAnswer(analysisID string, question string) domain.ChatAnswer {
	return domain.ChatAnswer{
		AnalysisID: analysisID,
		Answer:     outOfScopeChatMessage(question),
		Evidence:   []string{},
		Citations:  []domain.ChatCitation{},
	}
}

func outOfScopeChatMessage(question string) string {
	if looksLikePortugueseQuestion(question) {
		return "Posso responder apenas sobre a análise ativa da ObservAI. Pergunte sobre evidências, hipóteses, serviços afetados ou próximos passos da investigação."
	}
	return "I can only answer questions about the active ObservAI analysis. Ask about the evidence, hypotheses, affected services or recommended investigation steps."
}

func looksLikePortugueseQuestion(question string) bool {
	normalized := normalizeChatQuestionLanguage(question)
	if normalized == "" {
		return false
	}
	if strings.ContainsAny(strings.ToLower(question), "áàâãéêíóôõúç") {
		return true
	}
	words := make(map[string]struct{})
	for _, word := range strings.Fields(normalized) {
		words[word] = struct{}{}
	}
	for _, candidate := range []string{
		"qual", "quais", "quem", "quando", "onde", "porque", "como",
		"pode", "voce", "você", "sobre", "fora", "contexto", "receita",
		"bolo", "franca", "frança", "cidade", "pais", "país", "clima",
	} {
		if _, ok := words[candidate]; ok {
			return true
		}
	}
	return strings.Contains(normalized, "por que")
}

func normalizeChatQuestionLanguage(question string) string {
	question = strings.ToLower(strings.TrimSpace(question))
	var builder strings.Builder
	builder.Grow(len(question))
	for _, current := range question {
		switch {
		case unicode.IsLetter(current), unicode.IsNumber(current):
			builder.WriteRune(current)
		default:
			builder.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}
