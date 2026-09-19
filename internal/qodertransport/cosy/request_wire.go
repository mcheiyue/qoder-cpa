package cosy

import (
	"encoding/json"
	"fmt"
)

// Endpoint names the explicit COSY endpoint. Single-call only, no fallback.
type Endpoint string

const (
	EndpointAPI2 Endpoint = "cosy-api2"
	EndpointAPI3 Endpoint = "cosy-api3"
)

// chatBody is the Qoder CLI request payload.
type chatBody struct {
	RequestID    string            `json:"request_id"`
	RequestSetID string            `json:"request_set_id"`
	ChatRecordID string            `json:"chat_record_id"`
	SessionID    string            `json:"session_id"`
	Stream       bool              `json:"stream"`
	ChatTask     string            `json:"chat_task"`
	ChatContext  json.RawMessage   `json:"chat_context"`
	IsReply      bool              `json:"is_reply"`
	IsRetry      bool              `json:"is_retry"`
	Source       int               `json:"source"`
	Version      string            `json:"version"`
	AgentID      string            `json:"agent_id"`
	TaskID       string            `json:"task_id"`
	SessionType  string            `json:"session_type"`
	ModelConfig  modelConfigWire   `json:"model_config"`
	System       string            `json:"system,omitempty"`
	Messages     []chatMessage     `json:"messages"`
	Tools        []json.RawMessage `json:"tools"`
	Parameters   json.RawMessage   `json:"parameters"`
	Business     businessInfo      `json:"business"`
}

type modelConfigWire struct {
	Key            string `json:"key"`
	Format         string `json:"format"`
	Source         string `json:"source"`
	Enable         bool   `json:"enable"`
	DisplayName    string `json:"display_name,omitempty"`
	IsVL           bool   `json:"is_vl"`
	IsReasoning    bool   `json:"is_reasoning"`
	MaxInputTokens int    `json:"max_input_tokens,omitempty"`
}

type businessInfo struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	BeginAt int64  `json:"begin_at"`
	Stage   string `json:"stage"`
}

// chatMessage is one message in the upstream history.
type chatMessage struct {
	Role         string          `json:"role"`
	Content      string          `json:"content,omitempty"`
	ToolCalls    json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID   string          `json:"tool_call_id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Reasoning    string          `json:"reasoning_content,omitempty"`
	ResponseMeta json.RawMessage `json:"response_meta,omitempty"`
}

// httpRequestParts holds the computed request parts for the caller to dispatch.
type httpRequestParts struct {
	URL         string
	Auth        string
	Date        string
	CosyKey     string
	CosyVersion string
	Body        []byte
}

// endpointURL returns the full URL for a given endpoint.
func endpointURL(ep Endpoint) (string, error) {
	switch ep {
	case EndpointAPI2:
		return "https://api2.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1", nil
	case EndpointAPI3:
		return "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1", nil
	default:
		return "", fmt.Errorf("%w: %v", ErrUnknownEndpoint, ep)
	}
}
