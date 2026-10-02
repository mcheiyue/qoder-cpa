package cosy

import "testing"

// F6: upstream capture fields (698e8c63) must survive the usage parse —
// billable sits at usage top level, cacheable_tokens nests under
// prompt_tokens_details next to cached_tokens.

func TestF6UsageKeepsCaptureFields(t *testing.T) {
	payload := `{"usage":{"prompt_tokens":110,"completion_tokens":22,"total_tokens":132,` +
		`"billable":false,` +
		`"prompt_tokens_details":{"cached_tokens":64,"cacheable_tokens":32},` +
		`"completion_tokens_details":{"reasoning_tokens":9}}}`
	ev, ok := classifyUsage(payload)
	if !ok || ev.Usage == nil {
		t.Fatalf("usage not parsed: ok=%v", ok)
	}
	u := ev.Usage
	if u.PromptTokens != 110 || u.CompletionTokens != 22 || u.TotalTokens != 132 ||
		u.CachedTokens != 64 || u.ReasoningTokens != 9 {
		t.Errorf("core usage = %+v, want capture core retained", *u)
	}
	// billable:false is what the capture actually carries; a plain bool with
	// omitempty would erase it, so the field must be a pointer that stays
	// non-nil when upstream sent the key.
	if u.Billable == nil || *u.Billable {
		t.Errorf("billable: got %v, want ptr to false", u.Billable)
	}
	if u.CacheableTokens != 32 {
		t.Errorf("cacheable_tokens: got %d, want 32", u.CacheableTokens)
	}
}

func TestF6UsageOmittedCaptureStaysNil(t *testing.T) {
	payload := `{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	ev, ok := classifyUsage(payload)
	if !ok || ev.Usage == nil {
		t.Fatalf("usage not parsed: ok=%v", ok)
	}
	if ev.Usage.Billable != nil {
		t.Errorf("billable: got %v, want nil when upstream omits the key", *ev.Usage.Billable)
	}
	if ev.Usage.CacheableTokens != 0 {
		t.Errorf("cacheable_tokens: got %d, want 0", ev.Usage.CacheableTokens)
	}
}
