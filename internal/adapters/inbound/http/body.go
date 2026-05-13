package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"
)

var errRequestBodyTooLarge = errors.New("request body too large")

var errRequestBodyEmpty = errors.New("request body is empty")

type errRequestBodyMalformed struct{ Reason string }

func (err errRequestBodyMalformed) Error() string { return "malformed json: " + err.Reason }

type errRequestBodyUnknownField struct{ Field string }

func (err errRequestBodyUnknownField) Error() string { return "unknown field: " + err.Field }

type errRequestBodyExtraData struct{}

func (errRequestBodyExtraData) Error() string {
	return "request body must contain a single JSON document"
}

type decodeErrorRule interface {
	translate(err error) (error, bool)
}

type decodeErrorRuleFunc func(error) (error, bool)

func (fn decodeErrorRuleFunc) translate(err error) (error, bool) {
	return fn(err)
}

var decodeErrorRules = []decodeErrorRule{
	decodeErrorRuleFunc(translateEmptyBodyError),
	decodeErrorRuleFunc(translateMaxBytesError),
	decodeErrorRuleFunc(translateSyntaxError),
	decodeErrorRuleFunc(translateTypeError),
	decodeErrorRuleFunc(translateUnknownFieldError),
}

func decodeRequestBody(request *stdhttp.Request, target any) error {
	if request.Body == nil {
		return errRequestBodyEmpty
	}

	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return translateDecodeError(err)
	}

	var extraDocument json.RawMessage
	err := decoder.Decode(&extraDocument)
	if err == nil {
		return errRequestBodyExtraData{}
	}
	if !errors.Is(err, io.EOF) {
		return translateDecodeError(err)
	}

	return nil
}

func translateDecodeError(err error) error {
	for _, rule := range decodeErrorRules {
		translated, ok := rule.translate(err)
		if ok {
			return translated
		}
	}

	return errRequestBodyMalformed{Reason: err.Error()}
}

func translateEmptyBodyError(err error) (error, bool) {
	if !errors.Is(err, io.EOF) {
		return nil, false
	}
	return errRequestBodyEmpty, true
}

func translateMaxBytesError(err error) (error, bool) {
	var maxBytesError *stdhttp.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return errRequestBodyTooLarge, true
	}
	if strings.Contains(err.Error(), "http: request body too large") {
		return errRequestBodyTooLarge, true
	}
	return nil, false
}

func translateSyntaxError(err error) (error, bool) {
	var syntaxError *json.SyntaxError
	if !errors.As(err, &syntaxError) {
		return nil, false
	}
	return errRequestBodyMalformed{Reason: fmt.Sprintf("invalid JSON syntax at byte %d", syntaxError.Offset)}, true
}

func translateTypeError(err error) (error, bool) {
	var typeError *json.UnmarshalTypeError
	if !errors.As(err, &typeError) {
		return nil, false
	}

	field := typeError.Field
	if field == "" {
		field = typeError.Type.String()
	}
	return errRequestBodyMalformed{Reason: fmt.Sprintf("field %q expects %s", field, typeError.Type.String())}, true
}

func translateUnknownFieldError(err error) (error, bool) {
	if !strings.HasPrefix(err.Error(), "json: unknown field ") {
		return nil, false
	}

	field := strings.TrimPrefix(err.Error(), "json: unknown field ")
	field = strings.Trim(field, "\"")
	return errRequestBodyUnknownField{Field: field}, true
}
