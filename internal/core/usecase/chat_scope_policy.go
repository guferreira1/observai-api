package usecase

import "strings"

type chatScopePolicy interface {
	Allows(question string) bool
}

type compositeChatScopePolicy struct {
	allowed   []string
	refusal   []string
	minLength int
}

func defaultChatScopePolicy() chatScopePolicy {
	return compositeChatScopePolicy{
		minLength: 3,
		allowed: []string{
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
			"signal",
			"sinal",
			"anomaly",
			"anomalia",
			"finding",
			"observation",
		},
		refusal: []string{
			"ignore previous",
			"ignore the previous",
			"ignore all previous",
			"ignore all instructions",
			"ignore the instructions",
			"disregard previous",
			"disregard the previous",
			"disregard the instructions",
			"forget previous",
			"forget the previous",
			"forget your instructions",
			"override your instructions",
			"system prompt",
			"reveal your prompt",
			"show your prompt",
			"jailbreak",
			"act as",
			"you are now",
			"pretend to be",
			"role play",
			"developer mode",
			"do anything now",
			"ignore the rules",
			"ignore the system",
			"ignore as regras",
			"ignore o sistema",
			"esqueça as instruções",
			"esqueca as instrucoes",
			"finja ser",
			"finja que",
			"aja como",
		},
	}
}

func (policy compositeChatScopePolicy) Allows(question string) bool {
	normalized := strings.ToLower(strings.TrimSpace(question))
	if len(normalized) < policy.minLength {
		return false
	}

	for _, pattern := range policy.refusal {
		if strings.Contains(normalized, pattern) {
			return false
		}
	}

	for _, keyword := range policy.allowed {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}

	return false
}
