package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	"github.com/guferreira1/observai-api/internal/core/domain"
)

const chatPromptName = "interaction-chat-agent"

// ChatResponder answers scoped questions about an analysis using Ollama.
type ChatResponder struct {
	client *Client
	loader prompts.Loader
}

// NewChatResponder builds an Ollama-backed chat responder.
func NewChatResponder(client *Client, loader prompts.Loader) *ChatResponder {
	return &ChatResponder{client: client, loader: loader}
}

type chatAnswerPayload struct {
	Answer   string   `json:"answer"`
	Evidence []string `json:"evidence"`
}

// Answer renders the chat agent prompt + analysis context and returns a scoped reply.
func (responder *ChatResponder) Answer(ctx context.Context, analysisCtx domain.AnalysisContext, question domain.ChatQuestion) (domain.ChatAnswer, error) {
	system, err := responder.loader.Load(chatPromptName)
	if err != nil {
		return domain.ChatAnswer{}, fmt.Errorf("load chat prompt: %w", err)
	}

	userMessage, err := buildChatUserMessage(analysisCtx, question)
	if err != nil {
		return domain.ChatAnswer{}, err
	}

	content, err := responder.client.Chat(ctx, ChatRequest{
		System:         system,
		Messages:       []Message{{Role: "user", Content: userMessage}},
		JSONOutput:     true,
		TemperatureSet: true,
		Temperature:    0,
	})
	if err != nil {
		return domain.ChatAnswer{}, fmt.Errorf("ollama chat call: %w", err)
	}

	return decodeChatAnswerPayload(content, analysisCtx.AnalysisID)
}

func buildChatUserMessage(analysisCtx domain.AnalysisContext, question domain.ChatQuestion) (string, error) {
	body := map[string]any{
		"analysisContext": analysisCtx,
		"question":        question.Question,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode chat user payload: %w", err)
	}

	var buffer strings.Builder
	buffer.WriteString("Reply ONLY with JSON {\"answer\": string, \"evidence\": string[]}. Input:\n")
	buffer.Write(encoded)
	return buffer.String(), nil
}

func decodeChatAnswerPayload(content string, analysisID string) (domain.ChatAnswer, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return domain.ChatAnswer{}, fmt.Errorf("ollama returned empty chat answer")
	}

	var payload chatAnswerPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return domain.ChatAnswer{}, fmt.Errorf("decode ollama chat json: %w", err)
	}
	if payload.Answer == "" {
		return domain.ChatAnswer{}, fmt.Errorf("ollama chat answer is empty")
	}

	return domain.ChatAnswer{
		AnalysisID: analysisID,
		Answer:     payload.Answer,
		Evidence:   payload.Evidence,
	}, nil
}
