package cosy

import (
	"encoding/json"
	"testing"
)

// F0b usage + tool-history fixtures: pin what the current wire parses and
// carries, next to the QoderWork capture additions (698e8c63, ebb98ebb).
// Targets are recorded for F6, never enabled here.

// TestF0bUsageFixture pins the SSE usage parse: the five fields the current
// parser keeps, and the four capture fields it drops (added only in F6).
func TestF0bUsageFixture(t *testing.T) {
	payload := `{"usage":{"prompt_tokens":110,"completion_tokens":22,"total_tokens":132,` +
		`"prompt_tokens_details":{"cached_tokens":64},` +
		`"completion_tokens_details":{"reasoning_tokens":9},` +
		`"billable":true,"cacheable_tokens":64,` +
		`"firstTokenMs":180,"totalDurationMs":2400}}`
	ev, ok := classifyUsage(payload)
	if !ok || ev.Usage == nil {
		t.Fatalf("usage not parsed: ok=%v ev=%#v", ok, ev)
	}
	u := *ev.Usage
	if u.PromptTokens != 110 || u.CompletionTokens != 22 || u.TotalTokens != 132 ||
		u.CachedTokens != 64 || u.ReasoningTokens != 9 {
		t.Errorf("usage = %+v", u)
	}

	// The parsed struct must stay field-for-field: adding any of the capture
	// fields below before F6 changes what the audit layer can observe.
	raw, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"billable", "cacheable_tokens", "first_token_ms", "total_duration_ms"} {
		if _, present := got[absent]; present {
			t.Errorf("usage carries %q, want absent until F6 (698e8c63)", absent)
		}
	}
	if len(got) != 5 {
		t.Errorf("usage fields = %d, want exactly the 5 current ones: %s", len(got), raw)
	}
}

// TestF0bToolHistoryFixture pins the outbound history message shape for tool
// and reasoning turns: raw passthrough of tool_calls, reasoning_content kept,
// reasoning_item / cache_control / tool index absent until F6.
func TestF0bToolHistoryFixture(t *testing.T) {
	toolCalls := json.RawMessage(`[{"id":"call-1","type":"function","function":{"name":"f","arguments":"{}"}}]`)
	in := f0bInput()
	in.Messages = []ChatMessageIn{
		{Role: "user", Content: "run it"},
		{Role: "assistant", Content: "", ToolCalls: toolCalls, Reasoning: "step 1"},
		{Role: "tool", ToolCallID: "call-1", Content: "done", Name: "f"},
	}
	raw, err := BuildChatBody(in)
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var body struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(body.Messages))
	}
	assist := body.Messages[1]
	if _, present := assist["tool_calls"]; !present {
		t.Fatalf("assistant tool_calls missing: %#v", assist)
	}
	if assist["reasoning_content"] != "step 1" {
		t.Errorf("reasoning_content = %#v, want step 1", assist["reasoning_content"])
	}
	toolMsg := body.Messages[2]
	if toolMsg["tool_call_id"] != "call-1" || toolMsg["name"] != "f" || toolMsg["content"] != "done" {
		t.Errorf("tool result = %#v", toolMsg)
	}

	// Capture additions the current wire must not carry yet.
	for _, absent := range []string{"reasoning_item", "cache_control"} {
		if _, present := assist[absent]; present {
			t.Errorf("assistant message carries %q, want absent until F6 (698e8c63/ebb98ebb)", absent)
		}
	}
	// tool_calls is raw passthrough (json.RawMessage): whatever keys the caller
	// sends flow out unchanged, so F6 can add index without a wire struct edit.
	// Pin the passthrough contract on the current shape.
	entry, ok := assist["tool_calls"].([]interface{})
	if !ok || len(entry) != 1 {
		t.Fatalf("tool_calls = %#v, want one raw entry", assist["tool_calls"])
	}
	call, ok := entry[0].(map[string]interface{})
	if !ok || call["id"] != "call-1" {
		t.Fatalf("tool_calls[0] = %#v, want id call-1", entry[0])
	}
	if _, hasIndex := call["index"]; hasIndex {
		t.Errorf("tool_calls[0] carries index from a caller that sent none")
	}
}
