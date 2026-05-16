package http

import (
	"context"
	"errors"
	"fmt"
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
	cause   string
}

type domainErrorRule struct {
	match    func(error) bool
	build    func(error) httpErrorResponse
	response httpErrorResponse
}

type validationMessageBuilder func(validator.FieldError) string

var validationMessageBuilders = map[string]validationMessageBuilder{
	"required": func(_ validator.FieldError) string {
		return "This field is required."
	},
	"email": func(_ validator.FieldError) string {
		return "Enter a valid email address."
	},
	"min": func(validationError validator.FieldError) string {
		return fmt.Sprintf("Must be at least %s characters.", validationError.Param())
	},
	"oneof": func(validationError validator.FieldError) string {
		return "Use one of: " + strings.ReplaceAll(validationError.Param(), " ", ", ") + "."
	},
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
		match: func(err error) bool { return errors.Is(err, domain.ErrTraceNotFound) },
		response: httpErrorResponse{
			status:  stdhttp.StatusNotFound,
			code:    "trace_not_found",
			message: "analysis does not contain a trace reference",
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
		match: func(err error) bool { return errors.Is(err, domain.ErrProviderNotConfigured) },
		response: httpErrorResponse{
			status:  stdhttp.StatusServiceUnavailable,
			code:    "provider_not_configured",
			message: "the requested provider is not configured on this instance",
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
			message := "The request includes an unsupported field. Remove it and try again."
			if unknownField.Field != "" {
				message = fmt.Sprintf("Remove unsupported field %q from the request and try again.", unknownField.Field)
			}
			return httpErrorResponse{
				status:  stdhttp.StatusBadRequest,
				code:    "invalid_json",
				message: message,
				details: []ErrorFieldDetail{{
					Field:   unknownField.Field,
					Rule:    "unknown_field",
					Message: "This field is not accepted by this endpoint.",
				}},
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
		return withErrorCause(validationErrorResponse(validationErrors), err)
	}

	for _, rule := range domainErrorRules {
		if !rule.match(err) {
			continue
		}
		var response httpErrorResponse
		if rule.build != nil {
			response = rule.build(err)
		} else {
			response = rule.response
		}
		return withErrorCause(response, err)
	}

	return withErrorCause(httpErrorResponse{
		status:  stdhttp.StatusInternalServerError,
		code:    "internal_error",
		message: "internal server error",
	}, err)
}

func validationErrorResponse(validationErrors validator.ValidationErrors) httpErrorResponse {
	details := make([]ErrorFieldDetail, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		details = append(details, ErrorFieldDetail{
			Field:   fieldPath(validationError),
			Rule:    validationError.Tag(),
			Message: validationErrorMessage(validationError),
		})
	}

	return httpErrorResponse{
		status:  stdhttp.StatusBadRequest,
		code:    "invalid_request",
		message: "Some submitted fields are invalid. Review the highlighted fields and try again.",
		details: details,
	}
}

func validationErrorMessage(validationError validator.FieldError) string {
	if buildMessage, ok := validationMessageBuilders[validationError.Tag()]; ok {
		return buildMessage(validationError)
	}
	return "Review this field and try again."
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

func withErrorCause(response httpErrorResponse, err error) httpErrorResponse {
	if err != nil {
		response.cause = SanitizeExternalMessage(err.Error())
	}
	return response
}
