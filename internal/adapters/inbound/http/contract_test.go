package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
)

// TestContractSuccessEnvelope verifies every frontend-consumed endpoint
// returns the canonical WrapperDtoResponde envelope on success: a `data`
// object and a `metadata` object with at least requestId and
// processingTimeMs. The envelope is the public contract documented in
// CLAUDE.md / AGENTS.md.
func TestContractSuccessEnvelope(t *testing.T) {
	t.Parallel()
	router := newTestRouter()

	cases := []struct {
		method string
		path   string
		expect int
	}{
		{stdhttp.MethodGet, "/v1/openapi.yaml", stdhttp.StatusOK},
		{stdhttp.MethodGet, "/v1/capabilities", stdhttp.StatusOK},
		{stdhttp.MethodGet, "/v1/analyses", stdhttp.StatusOK},
		{stdhttp.MethodGet, "/v1/analyses/stats", stdhttp.StatusOK},
		{stdhttp.MethodGet, "/v1/services", stdhttp.StatusOK},
	}

	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.expect {
				t.Fatalf("%s %s expected %d, got %d (body=%s)", tt.method, tt.path, tt.expect, response.Code, response.Body.String())
			}
			if tt.path == "/v1/openapi.yaml" {
				return
			}
			var envelope struct {
				Data     json.RawMessage `json:"data"`
				Metadata struct {
					RequestID        string `json:"requestId"`
					ProcessingTimeMs int64  `json:"processingTimeMs"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("invalid envelope: %v body=%s", err, response.Body.String())
			}
			if len(envelope.Data) == 0 {
				t.Fatalf("data field missing in response: %s", response.Body.String())
			}
			if envelope.Metadata.RequestID == "" {
				t.Fatalf("metadata.requestId missing in response: %s", response.Body.String())
			}
		})
	}
}

// TestContractErrorEnvelope verifies the error path also wraps responses
// in the standard envelope with `data.code` + `data.message`.
func TestContractErrorEnvelope(t *testing.T) {
	t.Parallel()
	router := newTestRouter()

	request := httptest.NewRequest(stdhttp.MethodGet, "/v1/analyses/does-not-exist", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%s)", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"data"`
		Metadata struct {
			RequestID string `json:"requestId"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid error envelope: %v", err)
	}
	if envelope.Data.Code == "" || envelope.Data.Message == "" {
		t.Fatalf("error envelope missing code/message: %+v", envelope)
	}
	if envelope.Metadata.RequestID == "" {
		t.Fatalf("metadata.requestId missing in error response")
	}
}
