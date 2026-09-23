package cosy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

var (
	// ErrEmptyMessages is returned when BuildChatBody receives no messages.
	ErrEmptyMessages = errors.New("cosy: messages must not be empty")
	// ErrZeroBeginAt is returned when BuildChatBody receives a zero BeginAt.
	ErrZeroBeginAt = errors.New("cosy: begin_at must be set explicitly")
	// ErrUnknownEndpoint is returned for endpoints other than cosy-api2/cosy-api3.
	ErrUnknownEndpoint = errors.New("cosy: unknown endpoint")
	// ErrIncompleteRuntimeFields is returned when runtime fields are missing.
	ErrIncompleteRuntimeFields = errors.New("cosy: runtime fields incomplete")
)

// Parameters carries optional LLM generation parameters for the COSY request.
// Nil pointer fields are omitted from JSON; explicit zero/false values are sent.
// ToolChoice uses json.RawMessage because the upstream accepts both a plain
// string ("auto", "none") and a structured object ({ "type":"function", ... }).
type Parameters struct {
	MaxTokens         *int            `json:"max_tokens,omitempty"`
	ReasoningEffort   string          `json:"reasoning_effort,omitempty"`
	ToolChoice        json.RawMessage `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	EnableThinking    *bool           `json:"enable_thinking,omitempty"`
	ContextLength     *int            `json:"context_length,omitempty"`
}

// BuildRequestInput is the caller-supplied request parameters.
type BuildRequestInput struct {
	RequestID    string
	SessionID    string
	ModelKey     string
	ModelSource  string
	SystemPrompt string
	Messages     []ChatMessageIn
	Tools        []json.RawMessage
	Parameters   *Parameters
	ModelConfig  ModelConfigIn
	CosyVersion  string
	BeginAt      time.Time
}

// CatalogRequestParts contains the signed, bodyless model-catalog request.
type CatalogRequestParts struct {
	URL           string
	Authorization string
	Date          string
	CosyKey       string
	CosyVersion   string
}

// BuildCatalogRequestAt builds the signed model-catalog request used by the
// Global Qoder control plane.
func BuildCatalogRequestAt(ep Endpoint, fields RuntimeFields, requestID, cosyVersion string, now time.Time) (CatalogRequestParts, error) {
	if !fields.Complete() {
		return CatalogRequestParts{}, ErrIncompleteRuntimeFields
	}
	url, err := catalogURL(ep)
	if err != nil {
		return CatalogRequestParts{}, err
	}
	payload, err := BuildCOSYPayload(requestID, fields.EncryptUserInfo, cosyVersion)
	if err != nil {
		return CatalogRequestParts{}, fmt.Errorf("cosy: build catalog payload: %w", err)
	}
	date := strconv.FormatInt(now.Unix(), 10)
	signature := SignRequest(payload, fields.Key, date, "", SignPath(url))
	return CatalogRequestParts{
		URL: url, Authorization: ComposeBearer(payload, signature),
		Date: date, CosyKey: fields.Key, CosyVersion: cosyVersion,
	}, nil
}

// ChatMessageIn is an incoming chat message.
type ChatMessageIn struct {
	Role       string
	Content    string
	ToolCalls  json.RawMessage
	ToolCallID string
	Name       string
	Reasoning  string
}

// ModelConfigIn holds model metadata.
type ModelConfigIn struct {
	Key            string
	Format         string
	Source         string
	DisplayName    string
	IsReasoning    bool
	MaxInputTokens int
}

// BuildChatBody constructs the COSY request body JSON (un-encoded).
func BuildChatBody(in BuildRequestInput) ([]byte, error) {
	if len(in.Messages) == 0 {
		return nil, ErrEmptyMessages
	}
	if in.BeginAt.IsZero() {
		return nil, ErrZeroBeginAt
	}
	msgs := make([]chatMessage, 0, len(in.Messages))
	for _, m := range in.Messages {
		cm := chatMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
			Reasoning:  m.Reasoning,
		}
		msgs = append(msgs, cm)
	}
	format := in.ModelConfig.Format
	if format == "" {
		format = "openai"
	}
	source := in.ModelConfig.Source
	if source == "" {
		source = "system"
	}
	beginAt := in.BeginAt.UnixMilli()
	var paramsRaw json.RawMessage = json.RawMessage("{}")
	if in.Parameters != nil {
		serialized, err := json.Marshal(in.Parameters)
		if err != nil {
			return nil, fmt.Errorf("cosy: marshal parameters: %w", err)
		}
		paramsRaw = serialized
	}
	body := chatBody{
		RequestID:    in.RequestID,
		RequestSetID: in.RequestID,
		ChatRecordID: in.RequestID,
		SessionID:    in.SessionID,
		Stream:       true,
		ChatTask:     "FREE_INPUT",
		ChatContext:  json.RawMessage("{}"),
		IsReply:      true,
		IsRetry:      false,
		Source:       1,
		Version:      "3",
		AgentID:      "agent_common",
		TaskID:       "common",
		SessionType:  "qodercli",
		System:       in.SystemPrompt,
		Messages:     msgs,
		Tools:        in.Tools,
		Parameters:   paramsRaw,
		ModelConfig: modelConfigWire{
			Key:            in.ModelKey,
			Format:         format,
			Source:         source,
			Enable:         true,
			DisplayName:    in.ModelConfig.DisplayName,
			IsReasoning:    in.ModelConfig.IsReasoning,
			MaxInputTokens: in.ModelConfig.MaxInputTokens,
		},
		Business: businessInfo{
			Product: "cli",
			Version: in.CosyVersion,
			Type:    "agent",
			ID:      in.RequestID,
			Name:    truncateRunes(in.SystemPrompt, 10),
			BeginAt: beginAt,
			Stage:   "start",
		},
	}
	if body.Tools == nil {
		body.Tools = []json.RawMessage{}
	}
	return json.Marshal(body)
}

// BuildHTTPRequestAt constructs the signed HTTP request with an explicit timestamp.
func BuildHTTPRequestAt(ep Endpoint, body []byte, fields RuntimeFields, requestID, cosyVersion string, now time.Time) (*httpRequestParts, error) {
	if !fields.Complete() {
		return nil, ErrIncompleteRuntimeFields
	}
	url, err := endpointURL(ep)
	if err != nil {
		return nil, err
	}
	encodedBody := EncodeBody(body)
	payloadBase64, err := BuildCOSYPayload(requestID, fields.EncryptUserInfo, cosyVersion)
	if err != nil {
		return nil, fmt.Errorf("cosy: build payload: %w", err)
	}
	unixSec := strconv.FormatInt(now.Unix(), 10)
	signature := SignRequest(payloadBase64, fields.Key, unixSec, string(encodedBody), SignPath(url))
	auth := ComposeBearer(payloadBase64, signature)
	return &httpRequestParts{
		URL:         url,
		Auth:        auth,
		Date:        unixSec,
		CosyKey:     fields.Key,
		CosyVersion: cosyVersion,
		Body:        []byte(encodedBody),
	}, nil
}

func catalogURL(ep Endpoint) (string, error) {
	switch ep {
	case EndpointAPI2:
		return "https://api2.qoder.sh/algo/api/v2/model/list", nil
	case EndpointAPI3:
		return "https://api3.qoder.sh/algo/api/v2/model/list", nil
	default:
		return "", fmt.Errorf("%w: %v", ErrUnknownEndpoint, ep)
	}
}

// BuildHTTPRequest constructs the signed HTTP request. Uses current time.
func BuildHTTPRequest(ep Endpoint, body []byte, fields RuntimeFields, requestID, cosyVersion string) (*httpRequestParts, error) {
	return BuildHTTPRequestAt(ep, body, fields, requestID, cosyVersion, time.Now())
}

// Complete reports whether both halves of the runtime fields are present.
func (f RuntimeFields) Complete() bool {
	return f.EncryptUserInfo != "" && f.Key != ""
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
