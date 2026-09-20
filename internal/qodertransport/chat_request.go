package qodertransport

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

var errInvalidChatPayload = errors.New("qodertransport: invalid chat payload")

type chatPayload struct {
	Model       string            `json:"model"`
	Messages    []chatMessageWire `json:"messages"`
	Tools       []json.RawMessage `json:"tools"`
	ToolChoice  json.RawMessage   `json:"tool_choice"`
	Temperature *float64          `json:"temperature"`
	MaxTokens   *int              `json:"max_tokens"`
}

type chatMessageWire struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Reasoning  string          `json:"reasoning_content"`
	ToolCalls  json.RawMessage `json:"tool_calls"`
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
}

func parseChatPayload(raw []byte) (chatPayload, error) {
	var payload chatPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return chatPayload{}, errInvalidChatPayload
	}
	payload.Model = strings.TrimPrefix(strings.TrimSpace(payload.Model), "qoder/")
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
		RequestID: req.ID, SessionID: req.SessionID,
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
	return cosy.BuildRequestInput{
		RequestID: req.ID, SessionID: req.SessionID, ModelKey: payload.Model,
		ModelSource: "system", SystemPrompt: system.String(), Messages: messages,
		Tools: payload.Tools, CosyVersion: defaultCosyVersion,
		ModelConfig: cosy.ModelConfigIn{Key: payload.Model, Format: "openai", Source: "system"},
	}, nil
}
