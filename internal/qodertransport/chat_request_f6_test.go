package qodertransport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

// F6 tail: reasoning-only history retention. Orchids 698e8c63 fixed the same
// failure mode in their channel — an assistant turn with empty content and no
// tool_calls must NOT be dropped from the outbound history. This locks the
// full chain (parseChatPayload -> toCosyRequest -> BuildChatBody) for both
// content shapes the upstream may send ("", null).

func TestF6ReasoningOnlyHistoryNotDropped(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"empty string", `""`},
		{"null", `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"model":"qfmodel","messages":[` +
				`{"role":"system","content":"sys"},` +
				`{"role":"user","content":"hi"},` +
				`{"role":"assistant","content":` + tc.content + `,"reasoning_content":"step 1"},` +
				`{"role":"user","content":"next"}]}`)
			p, err := parseChatPayload(raw, nil)
			if err != nil {
				t.Fatalf("parseChatPayload: %v", err)
			}
			c, err := toCosyRequest(p, StreamRequest{})
			if err != nil {
				t.Fatalf("toCosyRequest: %v", err)
			}
			// system is extracted; the reasoning-only assistant must stay.
			if len(c.Messages) != 3 {
				t.Fatalf("messages = %d, want 3 (reasoning-only assistant dropped)", len(c.Messages))
			}
			assist := c.Messages[1]
			if assist.Role != "assistant" || assist.Reasoning != "step 1" ||
				assist.Content != "" || assist.ToolCalls != nil {
				t.Fatalf("assistant = %+v, want reasoning-only turn kept verbatim", assist)
			}

			c.BeginAt = time.Now()
			body, err := cosy.BuildChatBody(c)
			if err != nil {
				t.Fatalf("BuildChatBody: %v", err)
			}
			var out struct {
				Messages []map[string]interface{} `json:"messages"`
			}
			if err := json.Unmarshal(body, &out); err != nil {
				t.Fatalf("unmarshal body: %v", err)
			}
			if len(out.Messages) != 3 {
				t.Fatalf("wire messages = %d, want 3", len(out.Messages))
			}
			wireAssist := out.Messages[1]
			if wireAssist["reasoning_content"] != "step 1" {
				t.Errorf("wire reasoning_content = %#v, want step 1", wireAssist["reasoning_content"])
			}
			if _, present := wireAssist["tool_calls"]; present {
				t.Errorf("wire tool_calls = %#v, want absent", wireAssist["tool_calls"])
			}
			// Current wire omits an empty content key (omitempty). Locked as
			// observed behavior, not asserted as an upstream requirement.
			if _, present := wireAssist["content"]; present {
				t.Errorf("wire content = %#v, want absent (empty omitempty)", wireAssist["content"])
			}
		})
	}
}
