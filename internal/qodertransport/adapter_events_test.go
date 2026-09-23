package qodertransport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

func TestCosyEventSequenceProducesCompleteChatSSE(t *testing.T) {
	state := handleState{id: "chatcmpl-1", model: "qoder/model-a"}
	text, emit, err := cosyEventChunk(state.id, state.model, cosy.SSEEvent{
		Type: cosy.SSETextDelta, TextDelta: &cosy.TextDelta{Content: "hello"},
	}, &state)
	if err != nil || !emit || !strings.Contains(string(text), `"content":"hello"`) {
		t.Fatalf("text=%q emit=%v err=%v", text, emit, err)
	}
	usage, emit, err := cosyEventChunk(state.id, state.model, cosy.SSEEvent{
		Type: cosy.SSEUsage, Usage: &cosy.Usage{PromptTokens: 4, CompletionTokens: 3, TotalTokens: 7, ReasoningTokens: 2},
	}, &state)
	if err != nil || !emit || !strings.Contains(string(usage), `"reasoning_tokens":2`) {
		t.Fatalf("usage=%q emit=%v err=%v", usage, emit, err)
	}
	finish, emit, err := cosyEventChunk(state.id, state.model, cosy.SSEEvent{Type: cosy.SSETerminal}, &state)
	if err != nil || !emit || !strings.Contains(string(finish), `"finish_reason":"stop"`) {
		t.Fatalf("finish=%q emit=%v err=%v", finish, emit, err)
	}
	done, ok := state.nextTerminal()
	if !ok || string(done) != "data: [DONE]\n\n" {
		t.Fatalf("done=%q ok=%v", done, ok)
	}
}

func TestCosyEventChunk_BusinessError_PropagatesTyped(t *testing.T) {
	state := handleState{id: "chatcmpl-1", model: "qoder/model-a"}
	_, emit, err := cosyEventChunk(state.id, state.model, cosy.SSEEvent{
		Type: cosy.SSEError,
		StreamError: &cosy.StreamError{
			Code:    10605,
			Message: "queue_full",
		},
	}, &state)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if emit {
		t.Error("expected emit=false for error")
	}
	var sb *StreamBusinessError
	if !asStreamBusinessError(err, &sb) {
		t.Fatalf("expected *StreamBusinessError, got %T: %v", err, err)
	}
	if sb.Code != 10605 {
		t.Errorf("code: got %d, want 10605", sb.Code)
	}
	if sb.Category != "queue_full" {
		t.Errorf("category: got %q, want %q", sb.Category, "queue_full")
	}
}

func TestCosyEventChunk_BusinessError_WithResetAt(t *testing.T) {
	state := handleState{id: "chatcmpl-1", model: "qoder/model-a"}
	resetAt := time.UnixMilli(1727000000000)
	_, _, err := cosyEventChunk(state.id, state.model, cosy.SSEEvent{
		Type: cosy.SSEError,
		StreamError: &cosy.StreamError{
			Code:    429,
			Message: "rate_limited",
			ResetAt: resetAt,
		},
	}, &state)
	var sb *StreamBusinessError
	if !asStreamBusinessError(err, &sb) {
		t.Fatalf("expected *StreamBusinessError, got %T: %v", err, err)
	}
	if sb.Code != 429 {
		t.Errorf("code: %d", sb.Code)
	}
	if sb.Category != "rate_limited" {
		t.Errorf("category: %q", sb.Category)
	}
	if sb.ResetAt.IsZero() || sb.ResetAt.UnixMilli() != 1727000000000 {
		t.Errorf("resetAt: got %v, want epoch 1727000000000", sb.ResetAt)
	}
}

func TestBearerEventChunk_BusinessError_PropagatesTyped(t *testing.T) {
	state := handleState{id: "chatcmpl-1", model: "qoder/model-a"}
	_, emit, err := bearerEventChunk(state.id, state.model, bearer.SSEEvent{
		Type: bearer.SSEError,
		StreamError: &bearer.StreamError{
			Code:    112,
			Message: "access_denied",
		},
	}, &state)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if emit {
		t.Error("expected emit=false for error")
	}
	var sb *StreamBusinessError
	if !asStreamBusinessError(err, &sb) {
		t.Fatalf("expected *StreamBusinessError, got %T: %v", err, err)
	}
	if sb.Code != 112 {
		t.Errorf("code: got %d, want 112", sb.Code)
	}
	if sb.Category != "access_denied" {
		t.Errorf("category: got %q, want %q", sb.Category, "access_denied")
	}
}

func TestBearerEventChunk_BusinessError_WithResetAt(t *testing.T) {
	state := handleState{id: "chatcmpl-1", model: "qoder/model-a"}
	resetAt := time.UnixMilli(1727000000000)
	_, _, err := bearerEventChunk(state.id, state.model, bearer.SSEEvent{
		Type: bearer.SSEError,
		StreamError: &bearer.StreamError{
			Code:    429,
			Message: "rate_limited",
			ResetAt: resetAt,
		},
	}, &state)
	var sb *StreamBusinessError
	if !asStreamBusinessError(err, &sb) {
		t.Fatalf("expected *StreamBusinessError, got %T: %v", err, err)
	}
	if sb.ResetAt.IsZero() || sb.ResetAt.UnixMilli() != 1727000000000 {
		t.Errorf("resetAt: got %v, want epoch 1727000000000", sb.ResetAt)
	}
}

func TestChatPayloadPreservesToolsAndTextParts(t *testing.T) {
	raw := []byte(`{
		"model":"qoder/model-a",
		"messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"input_text","text":" world"}]}],
		"tools":[{"type":"function","function":{"name":"search","parameters":{"type":"object"}}}]
	}`)
	payload, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := toBearerRequest(payload, StreamRequest{ID: "req-1"})
	if err != nil {
		t.Fatal(err)
	}
	if request.Model != "model-a" || request.Messages[0].Content != "hello world" || len(request.Tools) != 1 {
		encoded, _ := json.Marshal(request)
		t.Fatalf("request=%s", encoded)
	}
}

func TestChatPayloadResolvesDynamicPublicModelAndPreservesResponseID(t *testing.T) {
	raw := []byte(`{"model":"qoder/Qwen3.8-Flash","messages":[{"role":"user","content":"hi"}]}`)
	payload, err := parseChatPayload(raw, func(publicID string) string {
		if publicID == "qoder/Qwen3.8-Flash" {
			return "qfmodel"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload.Model != "qfmodel" || payload.PublicModel != "qoder/Qwen3.8-Flash" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestChatPayloadUnknownModelPassesThrough(t *testing.T) {
	raw := []byte(`{"model":"qoder/new-model","messages":[{"role":"user","content":"hi"}]}`)
	payload, err := parseChatPayload(raw, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if payload.Model != "new-model" || payload.PublicModel != "qoder/new-model" {
		t.Fatalf("payload=%+v", payload)
	}
}

// asStreamBusinessError extracts *StreamBusinessError from an error.
func asStreamBusinessError(err error, target **StreamBusinessError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*StreamBusinessError); ok {
		*target = e
		return true
	}
	return false
}
