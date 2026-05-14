package http

import (
	"encoding/json"
	stdhttp "net/http"

	"github.com/guferreira1/observai-api/internal/core/usecase"
)

type sseChatSink struct {
	writer  stdhttp.ResponseWriter
	flusher stdhttp.Flusher
}

func newSSEChatSink(writer stdhttp.ResponseWriter, flusher stdhttp.Flusher) *sseChatSink {
	return &sseChatSink{writer: writer, flusher: flusher}
}

// Send implements usecase.ChatStreamSink. Each chat event becomes a single
// SSE message whose `event:` field carries the event type and whose `data:`
// field carries the JSON-encoded payload.
func (sink *sseChatSink) Send(event usecase.ChatStreamEvent) error {
	return sink.write(event.Type, sseChatTokenPayload{
		Token:    event.Token,
		Evidence: event.Evidence,
	})
}

// SendError emits a final `error` SSE event so clients can distinguish
// streamed failures from network disconnects.
func (sink *sseChatSink) SendError(code string, message string) error {
	return sink.write("error", sseChatErrorPayload{
		Code:    code,
		Message: message,
	})
}

type sseChatTokenPayload struct {
	Token    string   `json:"token,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type sseChatErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (sink *sseChatSink) write(eventType string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if _, err := sink.writer.Write([]byte("event: " + eventType + "\n")); err != nil {
		return err
	}
	if _, err := sink.writer.Write(append([]byte("data: "), encoded...)); err != nil {
		return err
	}
	if _, err := sink.writer.Write([]byte("\n\n")); err != nil {
		return err
	}
	sink.flusher.Flush()
	return nil
}
