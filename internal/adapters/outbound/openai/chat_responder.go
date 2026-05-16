package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/llmguard"
	"github.com/guferreira1/observai-api/internal/adapters/outbound/prompts"
	"github.com/guferreira1/observai-api/internal/core/domain"
)

const chatPromptName = "interaction-chat-agent"

// ChatResponder answers scoped follow-up questions via OpenAI.
type ChatResponder struct {
	client *Client
	loader prompts.Loader
}

// NewChatResponder builds an OpenAI-backed chat responder.
func NewChatResponder(client *Client, loader prompts.Loader) *ChatResponder {
	return &ChatResponder{client: client, loader: loader}
}

type chatAnswerPayload struct {
	Answer   string   `json:"answer"`
	Evidence []string `json:"evidence"`
}

// Answer issues a chat completion call constrained to the active analysis context.
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
		return domain.ChatAnswer{}, fmt.Errorf("openai chat call: %w", err)
	}

	return decodeChatAnswerPayload(content, analysisCtx)
}

func buildChatUserMessage(analysisCtx domain.AnalysisContext, question domain.ChatQuestion) (string, error) {
	catalog := llmguard.NewEvidenceCatalog(analysisCtx.Evidence)
	services := llmguard.NewServiceCatalog(analysisCtx.AffectedServices, analysisCtx.Evidence)
	body := map[string]any{
		"analysisContext":       analysisCtx,
		"question":              question.Question,
		"responseLanguage":      llmguard.ResponseLanguage(question.Question, analysisCtx.Summary),
		"validAffectedServices": services.Values(),
		"validEvidenceNames":    catalog.Names(),
		"validEvidenceIds":      catalog.IDs(),
		"groundingRules":        llmguard.GroundingRules(),
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode chat user payload: %w", err)
	}

	var buffer strings.Builder
	buffer.WriteString("Reply ONLY with JSON {\"answer\": string, \"evidence\": string[]}. Use responseLanguage for the answer. Input:\n")
	buffer.Write(encoded)
	return buffer.String(), nil
}

func decodeChatAnswerPayload(content string, analysisCtx domain.AnalysisContext) (domain.ChatAnswer, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return domain.ChatAnswer{}, fmt.Errorf("openai returned empty chat answer")
	}

	var payload chatAnswerPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return domain.ChatAnswer{}, fmt.Errorf("decode openai chat json: %w", err)
	}
	if payload.Answer == "" {
		return domain.ChatAnswer{}, fmt.Errorf("openai chat answer is empty")
	}

	catalog := llmguard.NewEvidenceCatalog(analysisCtx.Evidence)
	return domain.ChatAnswer{
		AnalysisID: analysisCtx.AnalysisID,
		Answer:     payload.Answer,
		Evidence:   catalog.FilterNames(payload.Evidence),
	}, nil
}
