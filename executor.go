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
	if req.Format != "chat-completions" {
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

func (s executorService) execute(ctx context.Context, req rpcExecutorRequest) (pluginapi.ExecutorResponse, error) {
	handle, err := s.open(ctx, req)
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}
	defer handle.Cancel()
	payload, err := aggregateChat(handle)
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
	go s.pumpStream(req.StreamID, handle)
	return pluginapi.ExecutorStreamResponse{Headers: http.Header{"Content-Type": {"text/event-stream"}}}, nil
}

func (s executorService) pumpStream(streamID string, handle qodertransport.StreamHandle) {
	defer handle.Cancel()
	for {
		chunk, err := handle.ReadChunk()
		if err != nil {
			if err == io.EOF {
				s.closeStream(streamID, "")
			} else {
				s.closeStream(streamID, err.Error())
			}
			return
		}
		if _, err := s.hostCall(pluginabi.MethodHostStreamEmit, map[string]any{"stream_id": streamID, "payload": chunk}); err != nil {
			s.closeStream(streamID, err.Error())
			return
		}
	}
}

func (s executorService) closeStream(streamID, message string) {
	payload := map[string]any{"stream_id": streamID}
	if message != "" {
		payload["error"] = message
	}
	_, _ = s.hostCall(pluginabi.MethodHostStreamClose, payload)
}

func (s executorService) countTokens(req rpcExecutorRequest) (pluginapi.ExecutorResponse, error) {
	if req.Format != "chat-completions" {
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
