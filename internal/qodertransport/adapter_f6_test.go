package qodertransport

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

func trimChunkData(t *testing.T, chunk []byte) string {
	t.Helper()
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(chunk)), "data:"))
	if raw == "" || raw == "[DONE]" {
		t.Fatalf("unexpected chunk: %q", chunk)
	}
	return raw
}

// F6: the audit layer observes what the outbound chunk carries — capture
// fields parsed upstream must reach the emitted usage JSON.

func TestF6UsageChunkCarriesCaptureFields(t *testing.T) {
	billable := false
	ev := cosy.SSEEvent{Type: cosy.SSEUsage, Usage: &cosy.Usage{
		PromptTokens:     110,
		CompletionTokens: 22,
		TotalTokens:      132,
		CachedTokens:     64,
		CacheableTokens:  32,
		ReasoningTokens:  9,
		Billable:         &billable,
	}}
	chunk, emit, err := cosyEventChunk("c1", "qoder/model-a", ev, nil)
	if err != nil || !emit {
		t.Fatalf("cosyEventChunk: emit=%v err=%v", emit, err)
	}
	raw := trimChunkData(t, chunk)
	var out struct {
		Usage map[string]interface{} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}
	if out.Usage == nil {
		t.Fatalf("usage missing in chunk: %s", raw)
	}
	if got := out.Usage["billable"]; got != false {
		t.Errorf("billable: got %#v, want false", got)
	}
	details, _ := out.Usage["prompt_tokens_details"].(map[string]interface{})
	if details == nil {
		t.Fatalf("prompt_tokens_details missing: %s", raw)
	}
	if got := details["cached_tokens"]; got != float64(64) {
		t.Errorf("cached_tokens: got %#v, want 64", got)
	}
	if got := details["cacheable_tokens"]; got != float64(32) {
		t.Errorf("cacheable_tokens: got %#v, want 32", got)
	}
}

// TestF6UsageChunkOmitsBillableWhenAbsent: no fabrication — the key only
// appears when upstream actually sent it.

func TestF6UsageChunkOmitsBillableWhenAbsent(t *testing.T) {
	ev := cosy.SSEEvent{Type: cosy.SSEUsage, Usage: &cosy.Usage{
		PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2,
	}}
	chunk, emit, err := cosyEventChunk("c1", "qoder/model-a", ev, nil)
	if err != nil || !emit {
		t.Fatalf("cosyEventChunk: emit=%v err=%v", emit, err)
	}
	raw := trimChunkData(t, chunk)
	var out struct {
		Usage map[string]interface{} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}
	if _, present := out.Usage["billable"]; present {
		t.Errorf("billable present without upstream key: %s", raw)
	}
}
