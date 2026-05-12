package usecase

import "strings"

type chatScopePolicy interface {
	Allows(question string) bool
}

type keywordChatScopePolicy struct {
	keywords []string
}

func defaultChatScopePolicy() chatScopePolicy {
	return keywordChatScopePolicy{
		keywords: []string{
			"analysis",
			"análise",
			"analise",
			"evidence",
			"evidência",
			"evidencia",
			"hypothesis",
			"hipótese",
			"hipotese",
			"root cause",
			"causa raiz",
			"recommendation",
			"recomendação",
			"recomendacao",
			"service",
			"serviço",
			"servico",
			"log",
			"metric",
			"métrica",
			"metrica",
			"trace",
			"span",
			"apm",
			"latency",
			"latência",
			"latencia",
			"error",
			"erro",
			"incident",
			"incidente",
			"severity",
			"severidade",
			"confidence",
			"confiança",
			"confianca",
			"bottleneck",
			"gargalo",
			"deployment",
			"deploy",
		},
	}
}

func (policy keywordChatScopePolicy) Allows(question string) bool {
	normalized := strings.ToLower(strings.TrimSpace(question))
	if normalized == "" {
		return false
	}

	for _, keyword := range policy.keywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}

	return false
}
