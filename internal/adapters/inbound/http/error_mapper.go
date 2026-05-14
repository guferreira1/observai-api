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
			response := httpErrorResponse{
				status:  stdhttp.StatusBadRequest,
				code:    "invalid_analysis_filter",
				message: extractReason(err, domain.ErrInvalidAnalysisFilter, "analysis filter is invalid"),
			}
			var filterErr errInvalidAnalysisFilter
			if errors.As(err, &filterErr) && filterErr.Field != "" {
				response.details = []ErrorFieldDetail{{
					Field:   filterErr.Field,
					Rule:    filterErr.Rule,
					Message: filterErr.Reason,
				}}
			}
			return response
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
		match: func(err error) bool { return errors.Is(err, domain.ErrJobNotFound) },
		response: httpErrorResponse{
			status:  stdhttp.StatusNotFound,
			code:    "analysis_job_not_found",
			message: "analysis job not found",
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, domain.ErrChatMessageNotFound) },
		response: httpErrorResponse{
			status:  stdhttp.StatusNotFound,
			code:    "chat_message_not_found",
			message: "chat message not found",
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, errRequestBodyTooLarge) },
		response: httpErrorResponse{
			status:  stdhttp.StatusRequestEntityTooLarge,
			code:    "request_body_too_large",
			message: "request body exceeds the configured size limit",
		},
	},
	{
		match: func(err error) bool { return errors.Is(err, errRequestBodyEmpty) },
		response: httpErrorResponse{
			status:  stdhttp.StatusBadRequest,
			code:    "invalid_json",
			message: "request body is empty",
		},
	},
	{
		match: func(err error) bool {
			var unknownField errRequestBodyUnknownField
			return errors.As(err, &unknownField)
		},
		build: func(err error) httpErrorResponse {
			var unknownField errRequestBodyUnknownField
			_ = errors.As(err, &unknownField)
			return httpErrorResponse{
				status:  stdhttp.StatusBadRequest,
				code:    "invalid_json",
				message: "request body contains unknown field",
				details: []ErrorFieldDetail{{Field: unknownField.Field, Rule: "unknown_field"}},
			}
		},
	},
	{
		match: func(err error) bool {
			var extraData errRequestBodyExtraData
			return errors.As(err, &extraData)
		},
		response: httpErrorResponse{
			status:  stdhttp.StatusBadRequest,
			code:    "invalid_json",
			message: "request body must contain a single JSON document",
		},
	},
	{
		match: func(err error) bool {
			var malformed errRequestBodyMalformed
			return errors.As(err, &malformed)
		},
		build: func(err error) httpErrorResponse {
			var malformed errRequestBodyMalformed
			_ = errors.As(err, &malformed)
			response := httpErrorResponse{
				status:  stdhttp.StatusBadRequest,
				code:    "invalid_json",
				message: malformed.Reason,
			}
			if malformed.Field != "" {
				response.details = []ErrorFieldDetail{{Field: malformed.Field, Rule: malformed.Rule}}
			}
			return response
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
