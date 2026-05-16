package llmguard

import (
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/stretchr/testify/assert"
)

func TestEvidenceCatalogFiltersUnknownValues(t *testing.T) {
	t.Parallel()

	catalog := NewEvidenceCatalog([]domain.Evidence{
		{ID: "ev_1", Name: "service_up"},
		{ID: "ev_2", Name: "error_log_count"},
	})

	assert.ElementsMatch(t, []string{"service_up", "error_log_count"}, catalog.Names())
	assert.ElementsMatch(t, []string{"ev_1", "ev_2"}, catalog.IDs())
	assert.Equal(t, []string{"service_up"}, catalog.FilterNames([]string{"service_up", "invented_metric", "service_up"}))
	assert.Equal(t, []string{"ev_2"}, catalog.FilterIDs([]string{"invented-id", "ev_2"}))
}

func TestServiceCatalogFiltersUnknownValues(t *testing.T) {
	t.Parallel()

	catalog := NewServiceCatalog([]string{"checkout-api"}, []domain.Evidence{
		{Service: "payments-api"},
		{Service: "checkout-api"},
	})

	assert.ElementsMatch(t, []string{"checkout-api", "payments-api"}, catalog.Values())
	assert.Equal(t, []string{"checkout-api", "payments-api"}, catalog.Filter([]string{"checkout-api", "invented-api", "payments-api"}))
}

func TestResponseLanguageDetectsPortuguese(t *testing.T) {
	t.Parallel()

	language := ResponseLanguage("Validar o fluxo da API e explicar o erro para o usuário")

	assert.Equal(t, "Portuguese (Brazil)", language)
}

func TestResponseLanguageDetectsEnglish(t *testing.T) {
	t.Parallel()

	language := ResponseLanguage("Which evidence supports this analysis?")

	assert.Equal(t, "English", language)
}
