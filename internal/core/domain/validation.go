package domain

import (
	"fmt"
	"strings"
)

// Validate verifies whether the analysis request has the minimum required fields.
func (request AnalysisRequest) Validate() error {
	if strings.TrimSpace(request.Goal) == "" {
		return fmt.Errorf("%w: goal is required", ErrInvalidAnalysisRequest)
	}

	if !request.TimeWindow.Start.IsZero() && !request.TimeWindow.End.IsZero() && request.TimeWindow.End.Before(request.TimeWindow.Start) {
		return fmt.Errorf("%w: time window end must be after start", ErrInvalidAnalysisRequest)
	}

	return nil
}
