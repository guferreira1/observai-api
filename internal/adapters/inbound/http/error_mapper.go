package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

type domainErrorResponse struct {
	status  int
	code    string
	message string
}

type domainErrorRule struct {
	match    func(error) bool
	response domainErrorResponse
}

var domainErrorRules = []domainErrorRule{
	{
		match: func(err error) bool {
			return errors.Is(err, domain.ErrInvalidAnalysisRequest)
		},
		response: domainErrorResponse{
			status: stdhttp.StatusBadRequest,
			code:   "invalid_analysis_request",
		},
	},
	{
		match: func(err error) bool {
			return errors.Is(err, domain.ErrQuestionOutOfScope)
		},
		response: domainErrorResponse{
			status:  stdhttp.StatusBadRequest,
			code:    "question_out_of_scope",
			message: "I can only answer questions about the active ObservAI analysis. Ask about the evidence, hypotheses, affected services or recommended investigation steps.",
		},
	},
	{
		match: func(err error) bool {
			return errors.Is(err, domain.ErrAnalysisNotFound)
		},
		response: domainErrorResponse{
			status:  stdhttp.StatusNotFound,
			code:    "analysis_not_found",
			message: "analysis not found",
		},
	},
}

func mapDomainError(err error) domainErrorResponse {
	for _, rule := range domainErrorRules {
		if rule.match(err) {
			response := rule.response
			if response.message == "" {
				response.message = err.Error()
			}
			return response
		}
	}

	return domainErrorResponse{
		status:  stdhttp.StatusInternalServerError,
		code:    "internal_error",
		message: "internal server error",
	}
}
