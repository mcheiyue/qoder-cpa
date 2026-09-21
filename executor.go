package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const executorID = "qoder"

type rpcExecutorRequest struct {
	pluginapi.ExecutorRequest
	StreamID       string `json:"stream_id,omitempty"`
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type transportSelector interface {
	Select(string) (qodertransport.ChatTransport, error)
}

type executorService struct {
	hostCall        func(string, any) (json.RawMessage, error)
	selectorFactory func(*http.Client, qoderauth.Credential) (transportSelector, error)
}

var defaultExecutorService = executorService{
	hostCall: callHostJSON,
	selectorFactory: func(client *http.Client, cred qoderauth.Credential) (transportSelector, error) {
		return qodertransport.NewCredentialSelector(client, cred)
	},
}

func (s executorService) open(ctx context.Context, req rpcExecutorRequest) (qodertransport.StreamHandle, error) {
	if !isChatExecutorFormat(req.Format) {
		return nil, &executorFailure{code: "unsupported_format", message: "qoder executor only accepts chat-completions", status: http.StatusBadRequest}
	}
	if req.AuthProvider != "" && !strings.EqualFold(req.AuthProvider, qoderauth.Provider) {
		return nil, &executorFailure{code: "invalid_auth", message: "qoder executor requires qoder auth", status: http.StatusUnauthorized}
	}
	cred, err := parseStoredCredential(req.StorageJSON)
	if err != nil {
		return nil, &executorFailure{code: "invalid_auth", message: "invalid qoder credential", status: http.StatusUnauthorized, cause: err}
	}
	client, err := newHostHTTPClientWithCall(req.HostCallbackID, s.hostCall)
	if err != nil {
		return nil, &executorFailure{code: "host_unavailable", message: err.Error(), status: http.StatusBadGateway, cause: err}
	}
	selector, err := s.selectorFactory(client, cred)
	if err != nil {
		return nil, err
	}
	transport, err := selector.Select(string(cred.Profile))
	if err != nil {
		return nil, err
	}
	requestID := strings.TrimSpace(req.Headers.Get("X-Request-Id"))
	if requestID == "" {
		requestID = strings.TrimSpace(req.AuthID)
	}
	sessionID := strings.TrimSpace(req.Headers.Get("X-Session-Id"))
	return transport.StreamChat(ctx, qodertransport.StreamRequest{
		Body: req.Payload, ID: requestID, SessionID: sessionID,
	})
}

func isChatExecutorFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "chat-completions", "chat_completions", "openai", "openai-chat-completions", "openai_chat_completions":
		return true
	default:
		return false
	}
}

func (s executorService) execute(ctx context.Context, req rpcExecutorRequest) (pluginapi.ExecutorResponse, error) {
	handle, err := s.open(ctx, req)
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}
	defer handle.Cancel()
	payload, err := aggregateChat(handle, estimatePayloadTokens(req.Payload))
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}
	return pluginapi.ExecutorResponse{Payload: payload, Headers: http.Header{"Content-Type": {"application/json"}}}, nil
}

func (s executorService) executeStream(ctx context.Context, req rpcExecutorRequest) (pluginapi.ExecutorStreamResponse, error) {
	if strings.TrimSpace(req.StreamID) == "" {
		return pluginapi.ExecutorStreamResponse{}, &executorFailure{code: "invalid_request", message: "stream_id is required", status: http.StatusBadRequest}
	}
	handle, err := s.open(ctx, req)
	if err != nil {
		return pluginapi.ExecutorStreamResponse{}, err
	}
	go s.pumpStream(req.StreamID, handle, estimatePayloadTokens(req.Payload))
	return pluginapi.ExecutorStreamResponse{Headers: http.Header{"Content-Type": {"text/event-stream"}}}, nil
}

func (s executorService) pumpStream(streamID string, handle qodertransport.StreamHandle, inputTokens int) {
	defer handle.Cancel()
	tracker := streamUsageTracker{inputTokens: inputTokens}
	for {
		chunk, err := handle.ReadChunk()
		if err != nil {
			if err == io.EOF {
				if usage, ok := tracker.estimatedChunk(); ok {
					if _, emitErr := s.hostCall(pluginabi.MethodHostStreamEmit, map[string]any{"stream_id": streamID, "payload": usage}); emitErr != nil {
						s.closeStream(streamID, emitErr.Error())
						return
					}
				}
				s.closeStream(streamID, "")
			} else {
				s.closeStream(streamID, err.Error())
			}
			return
		}
		payload, done, err := executorStreamPayload(chunk)
		if err != nil {
			s.closeStream(streamID, err.Error())
			return
		}
		if done {
			continue
		}
		tracker.observe(payload)
		if _, err := s.hostCall(pluginabi.MethodHostStreamEmit, map[string]any{"stream_id": streamID, "payload": payload}); err != nil {
			s.closeStream(streamID, err.Error())
			return
		}
	}
}

type streamUsageTracker struct {
	inputTokens  int
	outputTokens int
	realUsage    bool
}

func (t *streamUsageTracker) observe(payload []byte) {
	var chunk struct {
		Usage *struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal(payload, &chunk) != nil {
		return
	}
	if chunk.Usage != nil {
		t.realUsage = true
	}
	for _, choice := range chunk.Choices {
		t.outputTokens += estimatePayloadTokens([]byte(choice.Delta.Content))
		t.outputTokens += estimatePayloadTokens([]byte(choice.Delta.ReasoningContent))
		for _, tool := range choice.Delta.ToolCalls {
			t.outputTokens += estimatePayloadTokens([]byte(tool.Function.Name))
			t.outputTokens += estimatePayloadTokens([]byte(tool.Function.Arguments))
		}
	}
}

func (t streamUsageTracker) estimatedChunk() ([]byte, bool) {
	if t.realUsage || t.inputTokens == 0 && t.outputTokens == 0 {
		return nil, false
	}
	payload, err := json.Marshal(map[string]any{
		"choices": []struct{}{},
		"usage": map[string]any{
			"prompt_tokens":     t.inputTokens,
			"completion_tokens": t.outputTokens,
			"total_tokens":      t.inputTokens + t.outputTokens,
			"estimated":         true,
		},
	})
	if err != nil {
		return nil, false
	}
	return payload, true
}

func estimatePayloadTokens(payload []byte) int {
	if len(payload) == 0 {
		return 0
	}
	tokens := len(payload) / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

func executorStreamPayload(chunk []byte) ([]byte, bool, error) {
	payload := strings.TrimSpace(string(chunk))
	if payload == "data: [DONE]" || payload == "[DONE]" {
		return nil, true, nil
	}
	if strings.HasPrefix(payload, "data:") {
		payload = strings.TrimSpace(strings.TrimPrefix(payload, "data:"))
	}
	if !json.Valid([]byte(payload)) {
		return nil, false, fmt.Errorf("invalid chat stream payload")
	}
	return []byte(payload), false, nil
}

func (s executorService) closeStream(streamID, message string) {
	payload := map[string]any{"stream_id": streamID}
	if message != "" {
		payload["error"] = message
	}
	_, _ = s.hostCall(pluginabi.MethodHostStreamClose, payload)
}

func (s executorService) countTokens(req rpcExecutorRequest) (pluginapi.ExecutorResponse, error) {
	if !isChatExecutorFormat(req.Format) {
		return pluginapi.ExecutorResponse{}, &executorFailure{code: "unsupported_format", message: "qoder executor only accepts chat-completions", status: http.StatusBadRequest}
	}
	tokens := len(req.Payload) / 4
	if tokens == 0 && len(req.Payload) > 0 {
		tokens = 1
	}
	payload, err := json.Marshal(struct {
		TotalTokens int  `json:"total_tokens"`
		InputTokens int  `json:"input_tokens"`
		Estimated   bool `json:"estimated"`
	}{TotalTokens: tokens, InputTokens: tokens, Estimated: true})
	if err != nil {
		return pluginapi.ExecutorResponse{}, fmt.Errorf("encode token estimate: %w", err)
	}
	return pluginapi.ExecutorResponse{Payload: payload}, nil
}
