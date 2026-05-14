package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// AnalysisExportFormat identifies the canonical export target.
type AnalysisExportFormat string

const (
	// AnalysisExportFormatJSON renders the analysis as a JSON document.
	AnalysisExportFormatJSON AnalysisExportFormat = "json"
	// AnalysisExportFormatMarkdown renders the analysis as a Markdown report.
	AnalysisExportFormatMarkdown AnalysisExportFormat = "md"
)

// ErrUnsupportedExportFormat indicates the requested export format is unknown.
var ErrUnsupportedExportFormat = errors.New("unsupported analysis export format")

// AnalysisExport bundles the rendered analysis with its content type so the
// inbound adapter can set HTTP headers without re-deriving them.
type AnalysisExport struct {
	Body        []byte
	ContentType string
	Format      AnalysisExportFormat
}

// analysisExportRenderer renders a single analysis into the format-specific
// payload. Concrete renderers live in this file and are registered through
// the strategy map below; the use case never branches on the format itself.
type analysisExportRenderer interface {
	Render(result domain.AnalysisResult) (AnalysisExport, error)
}

var analysisExportRenderers = map[AnalysisExportFormat]analysisExportRenderer{
	AnalysisExportFormatJSON:     jsonAnalysisExportRenderer{},
	AnalysisExportFormatMarkdown: markdownAnalysisExportRenderer{},
}

// Export renders a previously stored analysis in the requested format.
func (useCase *Analysis) Export(ctx context.Context, analysisID string, format AnalysisExportFormat) (AnalysisExport, error) {
	analysisID = strings.TrimSpace(analysisID)
	if analysisID == "" {
		return AnalysisExport{}, fmt.Errorf("%w: analysis id is required", domain.ErrAnalysisNotFound)
	}

	renderer, ok := analysisExportRenderers[format]
	if !ok {
		return AnalysisExport{}, fmt.Errorf("%w: %s", ErrUnsupportedExportFormat, format)
	}

	result, err := useCase.repository.Find(ctx, analysisID)
	if err != nil {
		return AnalysisExport{}, fmt.Errorf("find analysis: %w", err)
	}

	return renderer.Render(result)
}

type jsonAnalysisExportRenderer struct{}

func (jsonAnalysisExportRenderer) Render(result domain.AnalysisResult) (AnalysisExport, error) {
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return AnalysisExport{}, fmt.Errorf("render analysis json export: %w", err)
	}
	return AnalysisExport{
		Body:        payload,
		ContentType: "application/json",
		Format:      AnalysisExportFormatJSON,
	}, nil
}

type markdownAnalysisExportRenderer struct{}

func (markdownAnalysisExportRenderer) Render(result domain.AnalysisResult) (AnalysisExport, error) {
	var builder strings.Builder
	builder.WriteString("# ObservAI analysis " + result.ID + "\n\n")
	builder.WriteString("- Severity: " + string(result.Severity) + "\n")
	builder.WriteString("- Confidence: " + string(result.Confidence) + "\n")
	builder.WriteString("- Created at: " + result.CreatedAt.Format("2006-01-02T15:04:05Z07:00") + "\n\n")

	builder.WriteString("## Summary\n\n")
	builder.WriteString(result.Summary + "\n\n")

	if len(result.AffectedServices) > 0 {
		builder.WriteString("## Affected services\n\n")
		for _, service := range result.AffectedServices {
			builder.WriteString("- " + service + "\n")
		}
		builder.WriteString("\n")
	}

	if len(result.DetectedAnomalies) > 0 {
		builder.WriteString("## Detected anomalies\n\n")
		for _, anomaly := range result.DetectedAnomalies {
			builder.WriteString("- " + anomaly + "\n")
		}
		builder.WriteString("\n")
	}

	if len(result.PossibleRootCauses) > 0 {
		builder.WriteString("## Possible root causes\n\n")
		for _, hypothesis := range result.PossibleRootCauses {
			builder.WriteString("- **" + hypothesis.Cause + "** (confidence: " + string(hypothesis.Confidence) + ")\n")
			for _, evidenceID := range hypothesis.Evidence {
				builder.WriteString("  - evidence: " + evidenceID + "\n")
			}
		}
		builder.WriteString("\n")
	}

	if len(result.RecommendedActions) > 0 {
		builder.WriteString("## Recommended actions\n\n")
		for _, action := range result.RecommendedActions {
			builder.WriteString("- (P" + strconv.Itoa(action.Priority) + ") " + action.Action + " — " + action.Rationale + "\n")
		}
		builder.WriteString("\n")
	}

	if len(result.Evidence) > 0 {
		builder.WriteString("## Evidence\n\n")
		for _, evidence := range result.Evidence {
			builder.WriteString("- `" + evidence.ID + "` " + string(evidence.Signal) + " / " + evidence.Service + ": " + evidence.Summary + "\n")
		}
		builder.WriteString("\n")
	}

	if len(result.MissingEvidence) > 0 {
		builder.WriteString("## Missing evidence\n\n")
		for _, missing := range result.MissingEvidence {
			builder.WriteString("- " + missing + "\n")
		}
	}

	return AnalysisExport{
		Body:        []byte(builder.String()),
		ContentType: "text/markdown; charset=utf-8",
		Format:      AnalysisExportFormatMarkdown,
	}, nil
}
