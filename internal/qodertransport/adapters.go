package qodertransport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

const defaultCosyVersion = "1.1.34"

// NewCredentialSelector builds the three explicit adapters for one account.
func NewCredentialSelector(client *http.Client, cred qoderauth.Credential, resolve ModelResolver) (*Selector, error) {
	cosyConfig := func(endpoint cosy.Endpoint) cosy.Config {
		return cosy.Config{HTTPClient: client, Endpoint: endpoint, MachineID: string(cred.MachineID), UserID: cred.UserID, OrganizationID: cred.OrganizationID, OrganizationTags: cred.OrganizationTags, DataPolicy: "agree"}
	}
	cosy2, err := cosy.NewTransport(cosyConfig(cosy.EndpointAPI2))
	if err != nil {
		return nil, err
	}
	cosy3, err := cosy.NewTransport(cosyConfig(cosy.EndpointAPI3))
	if err != nil {
		return nil, err
	}
	bearerTransport, err := bearer.NewTransport(bearer.Config{HTTPClient: client, Token: cred.AccessToken})
	if err != nil {
		return nil, err
	}
	runtime := cosy.RuntimeFields{EncryptUserInfo: cred.RuntimeInfo, Key: cred.RuntimeKey}
	return NewSelector(
		&cosyAdapter{transport: cosy2, runtime: runtime, resolve: resolve},
		&cosyAdapter{transport: cosy3, runtime: runtime, resolve: resolve},
		&bearerAdapter{transport: bearerTransport, resolve: resolve},
	)
}

type cosyAdapter struct {
	transport *cosy.Transport
	runtime   cosy.RuntimeFields
	resolve   ModelResolver
}

func (a *cosyAdapter) StreamChat(ctx context.Context, req StreamRequest) (StreamHandle, error) {
	payload, err := parseChatPayload(req.Body, a.resolve)
	if err != nil {
		return nil, err
	}
	input, err := toCosyRequest(payload, req)
	if err != nil {
		return nil, err
	}
	input.BeginAt = time.Now()
	body, err := cosy.BuildChatBody(input)
	if err != nil {
		return nil, err
	}
	response, err := a.transport.Stream(ctx, cosy.StreamRequest{
		RuntimeFields: a.runtime, RequestBody: body, RequestID: req.ID, CosyVersion: defaultCosyVersion,
		ModelKey: input.ModelKey, ModelSource: input.ModelSource,
	})
	if err != nil {
		return nil, err
	}
	return newTextToolFallback(&cosyHandle{
		response:    response,
		handleState: handleState{id: completionID(req.ID), model: payload.PublicModel},
	}, len(payload.Tools) > 0, completionID(req.ID), payload.PublicModel), nil
}

type bearerAdapter struct {
	transport *bearer.Transport
	resolve   ModelResolver
}

func (a *bearerAdapter) StreamChat(ctx context.Context, req StreamRequest) (StreamHandle, error) {
	payload, err := parseChatPayload(req.Body, a.resolve)
	if err != nil {
		return nil, err
	}
	input, err := toBearerRequest(payload, req)
	if err != nil {
		return nil, err
	}
	response, err := a.transport.Stream(ctx, input)
	if err != nil {
		return nil, err
	}
	return newTextToolFallback(&bearerHandle{
		response:    response,
		handleState: handleState{id: completionID(req.ID), model: payload.PublicModel},
	}, len(payload.Tools) > 0, completionID(req.ID), payload.PublicModel), nil
}

func completionID(requestID string) string {
	if requestID == "" {
		requestID = "qoder"
	}
	return "chatcmpl-" + requestID
}

type handleState struct {
	id       string
	model    string
	terminal bool
	done     bool
}

func (s *handleState) finishChunk() ([]byte, error) {
	reason := "stop"
	s.terminal = true
	return marshalSSE(chatChunk{
		ID: s.id, Object: "chat.completion.chunk", Model: s.model,
		Choices: []chatChoice{{Index: 0, Delta: chatDelta{}, FinishReason: &reason}},
	})
}

func (s *handleState) nextTerminal() ([]byte, bool) {
	if s.terminal && !s.done {
		s.done = true
		return doneSSE(), true
	}
	return nil, false
}

type cosyHandle struct {
	response *cosy.StreamResponse
	handleState
}

func (h *cosyHandle) ReadChunk() ([]byte, error) {
	if chunk, ok := h.handleState.nextTerminal(); ok {
		return chunk, nil
	}
	if h.handleState.done {
		return nil, io.EOF
	}
	for {
		event, err := h.response.Parser.ParseContext(h.response.Context)
		if err != nil {
			return nil, err
		}
		chunk, emit, err := cosyEventChunk(h.id, h.model, event, &h.handleState)
		if err != nil || emit {
			return chunk, err
		}
	}
}

func (h *cosyHandle) Cancel() { h.response.Cancel() }

type bearerHandle struct {
	response *bearer.StreamResponse
	handleState
}

func (h *bearerHandle) ReadChunk() ([]byte, error) {
	if chunk, ok := h.handleState.nextTerminal(); ok {
		return chunk, nil
	}
	if h.handleState.done {
		return nil, io.EOF
	}
	for {
		event, err := h.response.Parser.ParseContext(h.response.Context)
		if err != nil {
			return nil, err
		}
		chunk, emit, err := bearerEventChunk(h.id, h.model, event, &h.handleState)
		if err != nil || emit {
			return chunk, err
		}
	}
}

func (h *bearerHandle) Cancel() { h.response.Cancel() }

func streamError(code int, category string) error {
	if category == "" {
		return fmt.Errorf("qodertransport: upstream stream error %d", code)
	}
	return fmt.Errorf("qodertransport: upstream stream error %d (%s)", code, category)
}

var errUnknownStream = errors.New("qodertransport: upstream stream error")
