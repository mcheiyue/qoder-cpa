package qodertransport

import (
	"encoding/json"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

// --- helpers ---

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
func boolPtr(b bool) *bool    { return &b }

func minimalJSON(extra string) []byte {
	if extra == "" {
		return []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)
	}
	return []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}],` + extra + `}`)
}

func pMust(t *testing.T, raw []byte) chatPayload {
	t.Helper()
	p, err := parseChatPayload(raw, nil)
	if err != nil {
		t.Fatalf("parseChatPayload: %v", err)
	}
	return p
}

// --- derefString tests ---

func TestDerefString_Nil(t *testing.T) {
	if got := derefString(nil); got != "" {
		t.Errorf("derefString(nil): got %q, want empty", got)
	}
}

func TestDerefString_NonNil(t *testing.T) {
	s := "hello"
	if got := derefString(&s); got != "hello" {
		t.Errorf("derefString: got %q, want %q", got, "hello")
	}
}

// --- messageText tests (regression) ---

func TestMessageText_NullRaw(t *testing.T) {
	text, err := messageText(nil)
	if err != nil {
		t.Fatalf("messageText(nil): %v", err)
	}
	if text != "" {
		t.Errorf("messageText(nil): got %q, want empty", text)
	}
}

func TestMessageText_EmptyString(t *testing.T) {
	text, err := messageText(json.RawMessage(`""`))
	if err != nil {
		t.Fatalf("messageText: %v", err)
	}
	if text != "" {
		t.Errorf("messageText: got %q, want empty", text)
	}
}

func TestMessageText_PlainString(t *testing.T) {
	text, err := messageText(json.RawMessage(`"hello world"`))
	if err != nil {
		t.Fatalf("messageText: %v", err)
	}
	if text != "hello world" {
		t.Errorf("messageText: got %q, want %q", text, "hello world")
	}
}

func TestMessageText_PartsArray(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"hello "},{"type":"text","text":"world"}]`)
	text, err := messageText(raw)
	if err != nil {
		t.Fatalf("messageText: %v", err)
	}
	if text != "hello world" {
		t.Errorf("messageText: got %q, want %q", text, "hello world")
	}
}

// --- cosy.Parameters serialisation sanity ---

func TestCosyParameters_SerialisationEmpty(t *testing.T) {
	p := cosy.Parameters{}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != "{}" {
		t.Errorf("empty Parameters: got %s, want {}", raw)
	}
}

func TestCosyParameters_SerialisationWithNilPointer(t *testing.T) {
	p := cosy.Parameters{ReasoningEffort: "high"}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(m["reasoning_effort"]) != `"high"` {
		t.Errorf("reasoning_effort: %s", m["reasoning_effort"])
	}
	if _, ok := m["max_tokens"]; ok {
		t.Error("max_tokens should be omitted when nil")
	}
	if _, ok := m["parallel_tool_calls"]; ok {
		t.Error("parallel_tool_calls should be omitted when nil")
	}
}

func TestCosyParameters_SerialisationExplicitZero(t *testing.T) {
	v := 0
	p := cosy.Parameters{MaxTokens: &v}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(m["max_tokens"]) != "0" {
		t.Errorf("max_tokens: got %s, want 0", m["max_tokens"])
	}
}
