package usecase

import "strings"

type chatScopePolicy interface {
	Evaluate(question string) chatScopeDecision
}

type chatScopeDecision int

const (
	chatScopeDenied chatScopeDecision = iota
	chatScopeAllowed
	chatScopeContextualFollowUp
)

func (decision chatScopeDecision) AllowsActiveAnalysis() bool {
	return decision == chatScopeAllowed || decision == chatScopeContextualFollowUp
}

type compositeChatScopePolicy struct {
	allowed         []string
	contextual      []string
	exactContextual []string
	refusal         []string
	minLength       int
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
		contextual: []string{
			"e agora",
			"agora o que",
			"o que faço",
			"o que faco",
			"o que eu faço",
			"o que eu faco",
			"o que fazer",
			"como resolver",
			"como resolvo",
			"como corrigir",
			"como corrijo",
			"como mitigar",
			"como investigar",
			"por que isso",
			"por quê isso",
			"me explica",
			"explique melhor",
			"explica melhor",
			"detalhe",
			"detalhar",
			"continua",
			"continue",
			"próximo passo",
			"proximo passo",
			"próximos passos",
			"proximos passos",
			"isso é grave",
			"isso e grave",
			"qual prioridade",
			"qual o impacto",
			"tem risco",
			"devo",
			"what now",
			"what should i do",
			"what do i do",
			"next step",
			"next steps",
			"how to fix",
			"how do i fix",
			"how to solve",
			"how should i solve",
			"how to mitigate",
			"how to investigate",
			"why is that",
			"why did it",
			"explain better",
			"explain this",
			"tell me more",
			"go on",
			"is this critical",
			"is this serious",
			"what is the impact",
			"should i",
		},
		exactContextual: []string{
			"por que",
			"por quê",
			"porque",
			"pq",
			"why",
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

func (policy compositeChatScopePolicy) Evaluate(question string) chatScopeDecision {
	normalized := strings.ToLower(strings.TrimSpace(question))
	if len(normalized) < policy.minLength {
		return chatScopeDenied
	}

	for _, pattern := range policy.refusal {
		if strings.Contains(normalized, pattern) {
			return chatScopeDenied
		}
	}

	for _, keyword := range policy.allowed {
		if strings.Contains(normalized, keyword) {
			return chatScopeAllowed
		}
	}

	cleaned := strings.Trim(normalized, " \t\n\r?!.,;:")
	for _, pattern := range policy.exactContextual {
		if cleaned == pattern {
			return chatScopeContextualFollowUp
		}
	}

	for _, pattern := range policy.contextual {
		if strings.Contains(normalized, pattern) {
			return chatScopeContextualFollowUp
		}
	}

	return chatScopeDenied
}
