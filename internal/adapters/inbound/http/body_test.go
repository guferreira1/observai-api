package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateDecodeError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		assert func(t *testing.T, err error)
	}{
		{
			name: "returns empty body sentinel for EOF",
			err:  io.EOF,
			assert: func(t *testing.T, err error) {
				t.Helper()
				assert.ErrorIs(t, err, errRequestBodyEmpty)
			},
		},
		{
			name: "returns body too large sentinel for max bytes error",
			err:  &stdhttp.MaxBytesError{Limit: 10},
			assert: func(t *testing.T, err error) {
				t.Helper()
				assert.ErrorIs(t, err, errRequestBodyTooLarge)
			},
		},
		{
			name: "returns malformed body for syntax error",
			err:  &json.SyntaxError{Offset: 7},
			assert: func(t *testing.T, err error) {
				t.Helper()
				var malformed errRequestBodyMalformed
				require.ErrorAs(t, err, &malformed)
				assert.Contains(t, malformed.Reason, "byte 7")
			},
		},
		{
			name: "returns malformed body for type error",
			err:  &json.UnmarshalTypeError{Field: "limit", Type: reflect.TypeOf(0)},
			assert: func(t *testing.T, err error) {
				t.Helper()
				var malformed errRequestBodyMalformed
				require.ErrorAs(t, err, &malformed)
				assert.Contains(t, malformed.Reason, "limit")
			},
		},
		{
			name: "returns unknown field error",
			err:  errors.New(`json: unknown field "unexpected"`),
			assert: func(t *testing.T, err error) {
				t.Helper()
				var unknownField errRequestBodyUnknownField
				require.ErrorAs(t, err, &unknownField)
				assert.Equal(t, "unexpected", unknownField.Field)
			},
		},
		{
			name: "returns malformed body for unknown decode error",
			err:  errors.New("custom decode failure"),
			assert: func(t *testing.T, err error) {
				t.Helper()
				var malformed errRequestBodyMalformed
				require.ErrorAs(t, err, &malformed)
				assert.Equal(t, "custom decode failure", malformed.Reason)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.assert(t, translateDecodeError(test.err))
		})
	}
}

func TestDecodeRequestBodyRejectsExtraTopLevelDocument(t *testing.T) {
	t.Parallel()

	var payload struct {
		Goal string `json:"goal"`
	}
	request := httptest.NewRequest(stdhttp.MethodPost, "/", bytes.NewBufferString(`{"goal":"latency"} {"goal":"extra"}`))

	err := decodeRequestBody(request, &payload)

	var extraData errRequestBodyExtraData
	require.ErrorAs(t, err, &extraData)
}
