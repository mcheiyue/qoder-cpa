package main

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

// scriptedRead is one ReadChunk outcome for a fake handle.
type scriptedRead struct {
	chunk []byte
	err   error
}

// scriptedHandle replays a fixed read sequence then EOF (or repeats the last
// read when repeat is set, for multi-attempt refusal tests).
type scriptedHandle struct {
	mu        sync.Mutex
	reads     []scriptedRead
	idx       int
	repeat    bool
	cancelled bool
}

func (h *scriptedHandle) ReadChunk() ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.idx >= len(h.reads) {
		if h.repeat && len(h.reads) > 0 {
			return h.reads[len(h.reads)-1].chunk, h.reads[len(h.reads)-1].err
		}
		return nil, io.EOF
	}
	r := h.reads[h.idx]
	h.idx++
	return r.chunk, r.err
}

func (h *scriptedHandle) Cancel() {
	h.mu.Lock()
	h.cancelled = true
	h.mu.Unlock()
}

// seqTransport returns handles/errors in order; last entry repeats.
type seqTransport struct {
	mu    sync.Mutex
	steps []transportStep
	calls int
}

type transportStep struct {
	handle qodertransport.StreamHandle
	err    error
}

func (t *seqTransport) StreamChat(context.Context, qodertransport.StreamRequest) (qodertransport.StreamHandle, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls++
	i := t.calls - 1
	if i >= len(t.steps) {
		i = len(t.steps) - 1
	}
	return t.steps[i].handle, t.steps[i].err
}

// queueRefusal builds a structured queue_unavailable error (SSE frame path).
func queueRefusal(retryAfter, waitTime int) error {
	return &qodertransport.StreamBusinessError{
		Code:     10605,
		Category: qoderstream.CatQueueUnavailable,
		Queue: &qoderstream.QueuePayload{
			IsQueued:          true,
			QueueType:         "p3",
			ServiceAvailable:  false,
			RetryAfterSeconds: retryAfter,
			WaitTime:          waitTime,
		},
	}
}

func contentChunk(text string) []byte {
	return []byte("data: {\"choices\":[{\"delta\":{\"content\":\"" + text + "\"}}]}\n\n")
}

// TestQueueAwaitRetriesOnStructuredQueueRefusal: first stream refuses with
// structured 10605, second attempt succeeds; wait must be bounded by policy
// (hint honored) and the content must flow through.
func TestQueueAwaitRetriesOnStructuredQueueRefusal(t *testing.T) {
	good := &scriptedHandle{reads: []scriptedRead{{chunk: contentChunk("ok")}}}
	refuse := &scriptedHandle{reads: []scriptedRead{{err: queueRefusal(1, 1)}}}
	tr := &seqTransport{steps: []transportStep{{handle: refuse}, {handle: good}}}

	policy := defaultQueueWaitPolicy()
	policy.sleep = func(ctx context.Context, d time.Duration) error {
		if d <= 0 {
			t.Fatalf("wait duration must be positive, got %v", d)
		}
		return nil // record, do not actually sleep
	}

	h, err := queueAwait(context.Background(), tr, qodertransport.StreamRequest{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := h.ReadChunk()
	if err != nil {
		t.Fatalf("expected content after bounded wait, got %v", err)
	}
	if len(chunk) == 0 {
		t.Fatal("expected non-empty chunk")
	}
	if tr.calls != 2 {
		t.Fatalf("expected 2 upstream attempts, got %d", tr.calls)
	}
	if !refuse.cancelled {
		t.Fatal("refused stream must be cancelled before reopen")
	}
	if _, err := h.ReadChunk(); err != io.EOF {
		t.Fatalf("expected EOF after content, got %v", err)
	}
	h.Cancel()
}

// TestQueueAwaitNoRetryOnModelUnavailable: 112 / pricingUrl model_unavailable
// must return immediately — no wait, no second outbound call.
func TestQueueAwaitNoRetryOnModelUnavailable(t *testing.T) {
	mu := &scriptedHandle{reads: []scriptedRead{{err: &qodertransport.StreamBusinessError{
		Code:     112,
		Category: qoderstream.CatModelUnavailable,
	}}}}
	tr := &seqTransport{steps: []transportStep{{handle: mu}, {handle: mu}}}

	policy := defaultQueueWaitPolicy()
	policy.sleep = func(context.Context, time.Duration) error {
		t.Fatal("model_unavailable must not wait")
		return nil
	}

	h, err := queueAwait(context.Background(), tr, qodertransport.StreamRequest{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.ReadChunk()
	if err == nil {
		t.Fatal("expected model_unavailable error")
	}
	var sbe *qodertransport.StreamBusinessError
	if !errors.As(err, &sbe) || sbe.Code != 112 {
		t.Fatalf("expected 112 to pass through, got %v", err)
	}
	if tr.calls != 1 {
		t.Fatalf("model_unavailable must not reopen, calls=%d", tr.calls)
	}
}

// TestQueueAwaitNoRetryOnOrdinaryError: generic 403 / invalid JSON / client
// errors are not structured queue refusals — pass through with a single call.
func TestQueueAwaitNoRetryOnOrdinaryError(t *testing.T) {
	ordinary := &scriptedHandle{reads: []scriptedRead{{err: errors.New("http 403 forbidden")}}}
	tr := &seqTransport{steps: []transportStep{{handle: ordinary}}}

	policy := defaultQueueWaitPolicy()
	policy.sleep = func(context.Context, time.Duration) error {
		t.Fatal("ordinary errors must not wait")
		return nil
	}

	h, err := queueAwait(context.Background(), tr, qodertransport.StreamRequest{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.ReadChunk(); err == nil || err.Error() != "http 403 forbidden" {
		t.Fatalf("expected original error, got %v", err)
	}
	if tr.calls != 1 {
		t.Fatalf("ordinary error must not reopen, calls=%d", tr.calls)
	}
}
