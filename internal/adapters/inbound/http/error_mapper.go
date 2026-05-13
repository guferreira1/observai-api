package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/guferreira1/observai-api/internal/core/domain"
)

const clientClosedRequestStatus = 499

type httpErrorResponse struct {
	status  int
	code    string
	message string
	details []ErrorFieldDetail
}

type domainErrorRule struct {
	match    func(error) bool
	build    func(error) httpErrorResponse
	response httpErrorResponse
}

var domainErrorRules = []domainErrorRule{
	{
		match: func(err error) bool { return errors.Is(err, domain.ErrInvalidAnalysisRequest) },
		build: func(err error) httpErrorResponse {
			return httpErrorResponse{
				status:  stdhttp.StatusBadRequest,
				code:    "invalid_analysis_request",
				message: extractReason(err, domain.ErrInvalidAnalysisRequest, "analysis request is invalid"),
			}
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, domain.ErrInvalidAnalysisFilter) },
		build: func(err error) httpErrorResponse {
			return httpErrorResponse{
				status:  stdhttp.StatusBadRequest,
				code:    "invalid_analysis_filter",
				message: extractReason(err, domain.ErrInvalidAnalysisFilter, "analysis filter is invalid"),
			}
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, domain.ErrInvalidChatQuestion) },
		build: func(err error) httpErrorResponse {
			return httpErrorResponse{
				status:  stdhttp.StatusBadRequest,
				code:    "invalid_chat_question",
				message: extractReason(err, domain.ErrInvalidChatQuestion, "chat question is invalid"),
			}
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, domain.ErrQuestionOutOfScope) },
		response: httpErrorResponse{
			status:  stdhttp.StatusBadRequest,
			code:    "question_out_of_scope",
			message: "I can only answer questions about the active ObservAI analysis. Ask about the evidence, hypotheses, affected services or recommended investigation steps.",
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, domain.ErrAnalysisNotFound) },
		response: httpErrorResponse{
			status:  stdhttp.StatusNotFound,
			code:    "analysis_not_found",
			message: "analysis not found",
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, domain.ErrAnalysisContextNotFound) },
		response: httpErrorResponse{
			status:  stdhttp.StatusNotFound,
			code:    "analysis_context_not_found",
			message: "analysis context not found",
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, context.DeadlineExceeded) },
		response: httpErrorResponse{
			status:  stdhttp.StatusGatewayTimeout,
			code:    "request_timeout",
			message: "request exceeded the server-side processing budget",
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, context.Canceled) },
		response: httpErrorResponse{
			status:  clientClosedRequestStatus,
			code:    "request_canceled",
			message: "request canceled by the client",
		},
	},
}

func mapHTTPError(err error) httpErrorResponse {
	if err == nil {
		return httpErrorResponse{
			status:  stdhttp.StatusInternalServerError,
			code:    "internal_error",
			message: "internal server error",
		}
	}

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		return validationErrorResponse(validationErrors)
	}

	for _, rule := range domainErrorRules {
		if !rule.match(err) {
			continue
		}
		if rule.build != nil {
			return rule.build(err)
		}
		return rule.response
	}

	return httpErrorResponse{
		status:  stdhttp.StatusInternalServerError,
		code:    "internal_error",
		message: "internal server error",
	}
}

func validationErrorResponse(validationErrors validator.ValidationErrors) httpErrorResponse {
	details := make([]ErrorFieldDetail, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		details = append(details, ErrorFieldDetail{
			Field:   fieldPath(validationError),
			Rule:    validationError.Tag(),
			Message: validationError.Param(),
		})
	}

	return httpErrorResponse{
		status:  stdhttp.StatusBadRequest,
		code:    "invalid_request",
		message: "request validation failed",
		details: details,
	}
}

func fieldPath(err validator.FieldError) string {
	namespace := err.Namespace()
	if dot := strings.IndexByte(namespace, '.'); dot >= 0 {
		return namespace[dot+1:]
	}
	return err.Field()
}

func extractReason(err error, base error, fallback string) string {
	message := err.Error()
	prefix := base.Error() + ": "
	if strings.HasPrefix(message, prefix) {
		reason := strings.TrimPrefix(message, prefix)
		if reason != "" {
			return reason
		}
	}
	return fallback
}
