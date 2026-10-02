package main

import (
	"encoding/json"
	"testing"
)

// F6: the non-stream aggregate path must keep the usage capture fields the
// SSE parse retains — cache/read, cache/write, billable — and must not stamp
// estimated over a real usage frame.

func TestF6AggregateKeepsUsageDetails(t *testing.T) {
	chunks := [][]byte{
		[]byte("data: {\"id\":\"c1\",\"model\":\"qoder/model-a\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n"),
		[]byte("data: {\"id\":\"c1\",\"model\":\"qoder/model-a\",\"choices\":[],\"usage\":{\"prompt_tokens\":110,\"completion_tokens\":22,\"total_tokens\":132,\"billable\":false,\"prompt_tokens_details\":{\"cached_tokens\":64,\"cacheable_tokens\":32},\"completion_tokens_details\":{\"reasoning_tokens\":9}}}\n\n"),
		[]byte("data: {\"id\":\"c1\",\"model\":\"qoder/model-a\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"),
		[]byte("data: [DONE]\n\n"),
	}
	raw, err := aggregateChat(&fakeChatHandle{chunks: chunks}, 110)
	if err != nil {
		t.Fatalf("aggregateChat: %v", err)
	}
	var out struct {
		Usage map[string]interface{} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Usage == nil {
		t.Fatalf("usage missing: %s", raw)
	}
	details, _ := out.Usage["prompt_tokens_details"].(map[string]interface{})
	if details == nil {
		t.Fatalf("prompt_tokens_details missing (cache/read dropped): %s", raw)
	}
	if got := details["cached_tokens"]; got != float64(64) {
		t.Errorf("cached_tokens: got %#v, want 64", got)
	}
	if got := details["cacheable_tokens"]; got != float64(32) {
		t.Errorf("cacheable_tokens: got %#v, want 32", got)
	}
	if got, present := out.Usage["billable"]; !present || got != false {
		t.Errorf("billable: got %#v present=%v, want present=false", got, present)
	}
	comp, _ := out.Usage["completion_tokens_details"].(map[string]interface{})
	if comp == nil || comp["reasoning_tokens"] != float64(9) {
		t.Errorf("reasoning_tokens: %#v, want 9", comp)
	}
	// Real usage present → estimated must not be fabricated onto it.
	if _, present := out.Usage["estimated"]; present {
		t.Errorf("estimated stamped over real usage: %s", raw)
	}
}
