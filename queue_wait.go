package main

import (
	"context"
	"errors"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

// queueWaitPolicy bounds F5 structured queue (10605) waiting: per-round cap,
// cumulative budget, total attempt count. All three are explicit so the wait
// can never become an unbounded retry loop.
type queueWaitPolicy struct {
	maxWait     time.Duration // cap for one wait round (upstream waitTime observed 30s)
	budget      time.Duration // cumulative wait budget across the stream
	maxAttempts int           // total StreamChat calls including the first
	sleep       func(ctx context.Context, d time.Duration) error
}

func defaultQueueWaitPolicy() queueWaitPolicy {
	return queueWaitPolicy{
		maxWait:     35 * time.Second,
		budget:      70 * time.Second,
		maxAttempts: 3,
		sleep:       sleepWithContext,
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// queueAwait opens the stream once and wraps it so a structured queue refusal
// observed before any content byte triggers one bounded wait + reopen with the
// identical StreamRequest. Refusals without timing, over-cap/budget hints,
// model_unavailable, ordinary errors, EOF and post-content errors all pass
// through untouched (no second outbound).
type queueAwaitHandle struct {
	inner    qodertransport.StreamHandle
	ctx      context.Context
	cancel   context.CancelFunc
	tr       qodertransport.ChatTransport
	req      qodertransport.StreamRequest
	policy   queueWaitPolicy
	attempts int
	waited   time.Duration
	started  bool
}

func queueAwait(ctx context.Context, tr qodertransport.ChatTransport, req qodertransport.StreamRequest, policy queueWaitPolicy) (qodertransport.StreamHandle, error) {
	hctx, cancel := context.WithCancel(ctx)
	inner, err := tr.StreamChat(hctx, req)
	if err != nil {
		cancel()
		return nil, err
	}
	return &queueAwaitHandle{
		inner: inner, ctx: hctx, cancel: cancel,
		tr: tr, req: req, policy: policy, attempts: 1,
	}, nil
}

func (h *queueAwaitHandle) ReadChunk() ([]byte, error) {
	for {
		chunk, err := h.inner.ReadChunk()
		if err == nil {
			h.started = true
			return chunk, nil
		}
		if h.started {
			return nil, err
		}
		var sbe *qodertransport.StreamBusinessError
		if !errors.As(err, &sbe) || sbe.Category != qoderstream.CatQueueUnavailable || sbe.Queue == nil {
			return nil, err
		}
		if h.attempts >= h.policy.maxAttempts {
			return nil, err
		}
		hint := sbe.Queue.RetryAfterSeconds
		if hint <= 0 {
			hint = sbe.Queue.WaitTime
		}
		if hint <= 0 {
			return nil, err // structured refusal without timing: no wait basis
		}
		delay := time.Duration(hint) * time.Second
		if delay > h.policy.maxWait || h.waited+delay > h.policy.budget {
			return nil, err // over bound: surface the original refusal
		}
		h.inner.Cancel()
		if serr := h.policy.sleep(h.ctx, delay); serr != nil {
			return nil, serr // client cancel/deadline aborts the wait immediately
		}
		h.waited += delay
		next, rerr := h.tr.StreamChat(h.ctx, h.req)
		if rerr != nil {
			return nil, rerr
		}
		h.inner = next
		h.attempts++
	}
}

func (h *queueAwaitHandle) Cancel() {
	h.cancel()
	h.inner.Cancel()
}
