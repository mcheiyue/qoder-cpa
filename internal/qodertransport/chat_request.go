package qodertransport

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

var errInvalidChatPayload = errors.New("qodertransport: invalid chat payload")

// ResolvedModel carries the internal model ID and catalog metadata for a resolved public model.
type ResolvedModel struct {
	InternalID     string
	IsReasoning    bool
	MaxInputTokens int
}

// ModelResolver translates a client-facing Qoder model ID to its upstream key and metadata.
type ModelResolver func(publicID string) ResolvedModel

type chatPayload struct {
	Model               string            `json:"model"`
	PublicModel         string            `json:"-"`
	Messages            []chatMessageWire `json:"messages"`
	Tools               []json.RawMessage `json:"tools"`
	ToolChoice          json.RawMessage   `json:"tool_choice"`
	Temperature         *float64          `json:"temperature"`
	MaxTokens           *int              `json:"max_tokens"`
	ReasoningEffort     *string           `json:"reasoning_effort"`
	MaxCompletionTokens *int              `json:"max_completion_tokens"`
	ParallelToolCalls   *bool             `json:"parallel_tool_calls"`
	IsReasoning         bool              `json:"-"`
	MaxInputTokens      int               `json:"-"`
}

type chatMessageWire struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Reasoning  string          `json:"reasoning_content"`
	ToolCalls  json.RawMessage `json:"tool_calls"`
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
}

func parseChatPayload(raw []byte, resolve ModelResolver) (chatPayload, error) {
	var payload chatPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return chatPayload{}, errInvalidChatPayload
	}
	rawModel := strings.TrimSpace(payload.Model)
	if rawModel == "" {
		return chatPayload{}, errInvalidChatPayload
	}
	payload.PublicModel = rawModel
	if !strings.HasPrefix(rawModel, qoderauth.Provider+"/") {
		payload.PublicModel = qoderauth.Provider + "/" + rawModel
	}
	payload.Model = strings.TrimPrefix(payload.PublicModel, qoderauth.Provider+"/")
	if resolve != nil {
		resolved := resolve(payload.PublicModel)
		if internalID := strings.TrimSpace(resolved.InternalID); internalID != "" {
			payload.Model = internalID
		}
		payload.IsReasoning = resolved.IsReasoning
		payload.MaxInputTokens = resolved.MaxInputTokens
	}
	if payload.Model == "" || len(payload.Messages) == 0 {
		return chatPayload{}, errInvalidChatPayload
	}
	return payload, nil
}

func messageText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", errInvalidChatPayload
	}
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == "text" || part.Type == "input_text" {
			builder.WriteString(part.Text)
		}
	}
	return builder.String(), nil
}

func toBearerRequest(payload chatPayload, req StreamRequest) (bearer.StreamRequest, error) {
	messages := make([]bearer.Message, 0, len(payload.Messages))
	for _, item := range payload.Messages {
		content, err := messageText(item.Content)
		if err != nil {
			return bearer.StreamRequest{}, err
		}
		message := bearer.Message{
			Role: item.Role, Content: content, Reasoning: item.Reasoning,
			ToolCallID: item.ToolCallID, Name: item.Name,
		}
		if len(item.ToolCalls) > 0 {
			if err := json.Unmarshal(item.ToolCalls, &message.ToolCalls); err != nil {
				return bearer.StreamRequest{}, errInvalidChatPayload
			}
		}
		messages = append(messages, message)
	}
	tools := make([]bearer.Tool, 0, len(payload.Tools))
	for _, raw := range payload.Tools {
		var tool bearer.Tool
		if err := json.Unmarshal(raw, &tool); err != nil {
			return bearer.StreamRequest{}, errInvalidChatPayload
		}
		tools = append(tools, tool)
	}
	return bearer.StreamRequest{
		Model: payload.Model, Messages: messages, Tools: tools, ToolChoice: payload.ToolChoice,
		Temperature: payload.Temperature, MaxTokens: payload.MaxTokens,
		ReasoningEffort:     payload.ReasoningEffort,
		MaxCompletionTokens: payload.MaxCompletionTokens,
		ParallelToolCalls:   payload.ParallelToolCalls,
		RequestID:           req.ID, SessionID: req.SessionID,
	}, nil
}

func toCosyRequest(payload chatPayload, req StreamRequest) (cosy.BuildRequestInput, error) {
	messages := make([]cosy.ChatMessageIn, 0, len(payload.Messages))
	var system strings.Builder
	for _, item := range payload.Messages {
		content, err := messageText(item.Content)
		if err != nil {
			return cosy.BuildRequestInput{}, err
		}
		if item.Role == "system" || item.Role == "developer" {
			if system.Len() > 0 {
				system.WriteByte('\n')
			}
			system.WriteString(content)
			continue
		}
		messages = append(messages, cosy.ChatMessageIn{
			Role: item.Role, Content: content, Reasoning: item.Reasoning,
			ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID, Name: item.Name,
		})
	}
	// Build COSY parameters: only allocate when at least one field is set.
	// max_tokens priority: max_completion_tokens (if present) > max_tokens.
	var params *cosy.Parameters
	effort := strings.ToLower(strings.TrimSpace(derefString(payload.ReasoningEffort)))
	enableThinking := payload.IsReasoning && effort != "none"
	contextLength := payload.MaxInputTokens
	if payload.MaxCompletionTokens != nil || payload.MaxTokens != nil ||
		effort != "" ||
		len(payload.ToolChoice) > 0 ||
		payload.ParallelToolCalls != nil ||
		enableThinking ||
		contextLength > 0 {
		p := cosy.Parameters{
			ToolChoice:        payload.ToolChoice,
			ParallelToolCalls: payload.ParallelToolCalls,
		}
		if payload.MaxCompletionTokens != nil {
			p.MaxTokens = payload.MaxCompletionTokens
		} else {
			p.MaxTokens = payload.MaxTokens
		}
		// COSY reasoning semantics: "none" disables thinking; other effort strings are forwarded.
		if payload.IsReasoning || effort == "none" {
			v := enableThinking
			p.EnableThinking = &v
		}
		if effort != "" && effort != "none" {
			p.ReasoningEffort = effort
		}
		if contextLength > 0 {
			p.ContextLength = &contextLength
		}
		params = &p
	}
	return cosy.BuildRequestInput{
		RequestID: req.ID, SessionID: req.SessionID, ModelKey: payload.Model,
		ModelSource: "system", SystemPrompt: system.String(), Messages: messages,
		Tools: payload.Tools, Parameters: params, CosyVersion: defaultCosyVersion,
		ModelConfig: cosy.ModelConfigIn{
			Key: payload.Model, Format: "openai", Source: "system",
			IsReasoning: payload.IsReasoning, MaxInputTokens: payload.MaxInputTokens,
		},
	}, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
