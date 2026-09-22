package qodertransport

import (
	"encoding/json"
	"strings"
	"testing"

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
