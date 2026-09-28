package qodertransport

import (
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

func TestCosyEventChunk_ErrorCarriesStructuredQueueFields(t *testing.T) {
	state := handleState{id: "i", model: "m"}
	_, _, err := cosyEventChunk(state.id, state.model, cosy.SSEEvent{
		Type: cosy.SSEError,
		StreamError: &cosy.StreamError{
			Code:      10605,
			Message:   "queue_unavailable",
			OuterCode: 403,
			Queue: &qoderstream.QueuePayload{
				IsQueued: true, QueueType: "p3", ServiceAvailable: false,
				RetryAfterSeconds: 30, WaitTime: 30,
			},
		},
	}, &state)
	var sb *StreamBusinessError
	if !asStreamBusinessError(err, &sb) {
		t.Fatalf("expected *StreamBusinessError, got %T: %v", err, err)
	}
	if sb.OuterCode != 403 {
		t.Errorf("outer code: got %d, want 403", sb.OuterCode)
	}
	if sb.Queue == nil || sb.Queue.QueueType != "p3" || sb.Queue.RetryAfterSeconds != 30 || sb.Queue.WaitTime != 30 || sb.Queue.ServiceAvailable {
		t.Errorf("queue payload: %+v", sb.Queue)
	}
	if !strings.Contains(sb.Error(), "retry=30s") {
		t.Errorf("error string should carry retry wait: %q", sb.Error())
	}
}

func TestBearerEventChunk_ErrorCarriesStructuredQueueFields(t *testing.T) {
	state := handleState{id: "i", model: "m"}
	_, _, err := bearerEventChunk(state.id, state.model, bearer.SSEEvent{
		Type: bearer.SSEError,
		StreamError: &bearer.StreamError{
			Code:    10605,
			Message: "queue_unavailable",
			Queue: &qoderstream.QueuePayload{
				IsQueued: true, QueueType: "p3", ServiceAvailable: false,
				RetryAfterSeconds: 30, WaitTime: 30,
			},
		},
	}, &state)
	var sb *StreamBusinessError
	if !asStreamBusinessError(err, &sb) {
		t.Fatalf("expected *StreamBusinessError, got %T: %v", err, err)
	}
	if sb.Queue == nil || sb.Queue.QueueType != "p3" || sb.Queue.RetryAfterSeconds != 30 {
		t.Errorf("queue payload: %+v", sb.Queue)
	}
}

func TestUsageChunk_EmitsCachedTokens(t *testing.T) {
	state := handleState{id: "i", model: "m"}
	chunk, emit, err := cosyEventChunk(state.id, state.model, cosy.SSEEvent{
		Type: cosy.SSEUsage,
		Usage: &cosy.Usage{
			PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
			ReasoningTokens: 2, CachedTokens: 4,
		},
	}, &state)
	if err != nil || !emit {
		t.Fatalf("emit=%v err=%v", emit, err)
	}
	if !strings.Contains(string(chunk), `"cached_tokens":4`) {
		t.Errorf("usage chunk missing cached_tokens: %s", chunk)
	}
	if !strings.Contains(string(chunk), `"reasoning_tokens":2`) {
		t.Errorf("usage chunk missing reasoning_tokens: %s", chunk)
	}
}
