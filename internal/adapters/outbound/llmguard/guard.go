package llmguard

import (
	"sort"
	"strings"
	"unicode"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// EvidenceCatalog indexes normalized evidence identifiers accepted from LLM output.
type EvidenceCatalog struct {
	names map[string]struct{}
	ids   map[string]struct{}
}

// ServiceCatalog indexes service names accepted from LLM output.
type ServiceCatalog struct {
	services map[string]struct{}
}

// NewEvidenceCatalog builds an EvidenceCatalog from normalized ObservAI evidence.
func NewEvidenceCatalog(evidence []domain.Evidence) EvidenceCatalog {
	catalog := EvidenceCatalog{
		names: make(map[string]struct{}, len(evidence)),
		ids:   make(map[string]struct{}, len(evidence)),
	}
	for _, item := range evidence {
		if name := strings.TrimSpace(item.Name); name != "" {
			catalog.names[name] = struct{}{}
		}
		if id := strings.TrimSpace(item.ID); id != "" {
			catalog.ids[id] = struct{}{}
		}
	}
	return catalog
}

// Names returns the known Evidence.name values.
func (catalog EvidenceCatalog) Names() []string {
	return keys(catalog.names)
}

// IDs returns the known Evidence.id values.
func (catalog EvidenceCatalog) IDs() []string {
	return keys(catalog.ids)
}

// FilterNames keeps only Evidence.name values present in the catalog.
func (catalog EvidenceCatalog) FilterNames(values []string) []string {
	return filterKnown(values, catalog.names)
}

// FilterIDs keeps only Evidence.id values present in the catalog.
func (catalog EvidenceCatalog) FilterIDs(values []string) []string {
	return filterKnown(values, catalog.ids)
}

// NewServiceCatalog builds a ServiceCatalog from request services and evidence.
func NewServiceCatalog(serviceNames []string, evidence []domain.Evidence) ServiceCatalog {
	catalog := ServiceCatalog{services: make(map[string]struct{}, len(serviceNames)+len(evidence))}
	for _, raw := range serviceNames {
		if service := strings.TrimSpace(raw); service != "" {
			catalog.services[service] = struct{}{}
		}
	}
	for _, item := range evidence {
		if service := strings.TrimSpace(item.Service); service != "" {
			catalog.services[service] = struct{}{}
		}
	}
	return catalog
}

// Values returns the known service names.
func (catalog ServiceCatalog) Values() []string {
	return keys(catalog.services)
}

// Filter keeps only service names present in the catalog.
func (catalog ServiceCatalog) Filter(values []string) []string {
	return filterKnown(values, catalog.services)
}

// GroundingRules returns provider-agnostic LLM constraints for grounded output.
func GroundingRules() []string {
	return []string{
		"Write every natural-language field in responseLanguage.",
		"Use only the normalized evidence in the payload as source of truth.",
		"Do not invent metrics, services, logs, spans, timestamps, deployments, code paths, evidence names or evidence ids.",
		"Only cite values present in validEvidenceNames, validEvidenceIds and validAffectedServices.",
		"When evidence is insufficient, state the gap explicitly and keep unsupported conclusions empty.",
	}
}

// ResponseLanguage infers the natural language expected for LLM output.
func ResponseLanguage(texts ...string) string {
	normalized := normalizeLanguageInput(strings.Join(texts, " "))
	if normalized == "" {
		return "same language as the user input"
	}
	if looksPortuguese(normalized) {
		return "Portuguese (Brazil)"
	}
	if looksEnglish(normalized) {
		return "English"
	}
	return "same language as the user input"
}

func keys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func filterKnown(values []string, known map[string]struct{}) []string {
	if len(values) == 0 || len(known) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := known[value]; !ok {
			continue
		}
		if _, duplicated := seen[value]; duplicated {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeLanguageInput(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	builder.Grow(len(value))
	for _, current := range value {
		switch {
		case unicode.IsLetter(current), unicode.IsNumber(current):
			builder.WriteRune(current)
		default:
			builder.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func looksPortuguese(value string) bool {
	if strings.ContainsAny(value, "áàâãéêíóôõúç") {
		return true
	}
	words := wordSet(value)
	for _, candidate := range []string{
		"analise", "análise", "validar", "fluxo", "propria", "própria",
		"servico", "serviço", "metrica", "métrica", "evidencia", "evidência",
		"erro", "erros", "agora", "resolver", "corrigir", "investigar",
		"porque", "impacto", "grave", "recomendacao", "recomendação",
		"proximo", "próximo", "passo", "nao", "não", "usuario", "usuário",
	} {
		if _, ok := words[candidate]; ok {
			return true
		}
	}
	return strings.Contains(value, "por que") || strings.Contains(value, "como resolver")
}

func looksEnglish(value string) bool {
	words := wordSet(value)
	for _, candidate := range []string{
		"analysis", "evidence", "service", "metric", "log", "trace",
		"error", "incident", "why", "what", "which", "how", "resolve",
		"fix", "impact", "severity", "confidence", "recommendation",
		"investigate", "latency", "failure", "failed", "issue",
	} {
		if _, ok := words[candidate]; ok {
			return true
		}
	}
	return false
}

func wordSet(value string) map[string]struct{} {
	fields := strings.Fields(value)
	words := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		words[field] = struct{}{}
	}
	return words
}
