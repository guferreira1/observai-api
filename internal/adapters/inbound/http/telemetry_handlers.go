package http

import (
	"encoding/json"
	stdhttp "net/http"
	"time"
)

// TelemetryAcceptedResponseDto acknowledges client telemetry ingestion.
type TelemetryAcceptedResponseDto struct {
	Accepted   bool `json:"accepted"`
	EventCount int  `json:"eventCount"`
}

func (router *Router) handleTelemetry(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var payload map[string]json.RawMessage
	if err := decodeRequestBody(request, &payload); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusAccepted, TelemetryAcceptedResponseDto{
		Accepted:   true,
		EventCount: telemetryEventCount(payload),
	})
}

func telemetryEventCount(payload map[string]json.RawMessage) int {
	rawEvents, ok := payload["events"]
	if !ok || len(rawEvents) == 0 {
		return 1
	}
	var events []json.RawMessage
	if err := json.Unmarshal(rawEvents, &events); err != nil || len(events) == 0 {
		return 1
	}
	return len(events)
}
