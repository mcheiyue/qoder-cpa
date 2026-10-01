package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

// TestQueueAwaitEmptyStreamNoRetry: streaming empty response (immediate EOF)
// must not trigger repeat outbound calls.
func TestQueueAwaitEmptyStreamNoRetry(t *testing.T) {
	empty := &scriptedHandle{}
	tr := &seqTransport{steps: []transportStep{{handle: empty}}}

	h, err := queueAwait(context.Background(), tr, qodertransport.StreamRequest{}, defaultQueueWaitPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ReadChunk(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
	if tr.calls != 1 {
		t.Fatalf("empty stream must not reopen, calls=%d", tr.calls)
	}
}

// TestQueueAwaitCancelDuringWait: client cancellation while waiting must abort
// immediately with context error and never reopen upstream.
func TestQueueAwaitCancelDuringWait(t *testing.T) {
	refuse := &scriptedHandle{reads: []scriptedRead{{err: queueRefusal(30, 30)}}}
	tr := &seqTransport{steps: []transportStep{{handle: refuse}}}

	ctx, cancel := context.WithCancel(context.Background())
	policy := defaultQueueWaitPolicy()
	policy.sleep = func(sleepCtx context.Context, d time.Duration) error {
		cancel() // client goes away during the wait
		<-sleepCtx.Done()
		return sleepCtx.Err()
	}

	h, err := queueAwait(ctx, tr, qodertransport.StreamRequest{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.ReadChunk()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if tr.calls != 1 {
		t.Fatalf("cancelled wait must not reopen, calls=%d", tr.calls)
	}
}

// TestQueueAwaitBudgetBounded: hint exceeding the total wait budget must fail
// fast with the original refusal instead of sleeping past the bound.
func TestQueueAwaitBudgetBounded(t *testing.T) {
	refuse := &scriptedHandle{reads: []scriptedRead{{err: queueRefusal(120, 120)}}}
	tr := &seqTransport{steps: []transportStep{{handle: refuse}}}

	policy := defaultQueueWaitPolicy()
	policy.maxWait = 30 * time.Second
	policy.budget = 10 * time.Second
	policy.sleep = func(context.Context, time.Duration) error {
		t.Fatal("over-budget hint must not sleep at all")
		return nil
	}

	h, err := queueAwait(context.Background(), tr, qodertransport.StreamRequest{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.ReadChunk()
	var sbe *qodertransport.StreamBusinessError
	if !errors.As(err, &sbe) || sbe.Category != qoderstream.CatQueueUnavailable {
		t.Fatalf("expected original queue refusal, got %v", err)
	}
	if tr.calls != 1 {
		t.Fatalf("over-budget must not reopen, calls=%d", tr.calls)
	}
}

// TestQueueAwaitAttemptCap: structured refusals must stop at the attempt cap
// (no infinite retry) and surface the last refusal.
func TestQueueAwaitAttemptCap(t *testing.T) {
	refuse := &scriptedHandle{repeat: true, reads: []scriptedRead{{err: queueRefusal(1, 1)}}}
	tr := &seqTransport{steps: []transportStep{{handle: refuse}}}

	policy := defaultQueueWaitPolicy()
	policy.maxAttempts = 3
	policy.sleep = func(context.Context, time.Duration) error { return nil }

	h, err := queueAwait(context.Background(), tr, qodertransport.StreamRequest{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.ReadChunk()
	var sbe *qodertransport.StreamBusinessError
	if !errors.As(err, &sbe) || sbe.Code != 10605 {
		t.Fatalf("expected queue refusal after cap, got %v", err)
	}
	if tr.calls != 3 {
		t.Fatalf("expected exactly maxAttempts=%d calls, got %d", policy.maxAttempts, tr.calls)
	}
}
