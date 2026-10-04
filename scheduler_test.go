package main

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// queueErr builds a queue_unavailable stream error carrying structured retry-after.
func queueErr(retryAfterSec int) error {
	return &qodertransport.StreamBusinessError{
		Code:     10605,
		Category: qoderstream.CatQueueUnavailable,
		Queue:    &qoderstream.QueuePayload{IsQueued: true, RetryAfterSeconds: retryAfterSec},
	}
}

// testSchedulerState swaps the global scheduler state with a deterministic
// instance (seeded rng + frozen clock) and restores it on cleanup.
func testSchedulerState(t *testing.T) *schedulerState {
	t.Helper()
	prev := globalSchedulerState
	s := newSchedulerState()
	base := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }
	s.rng = rand.New(rand.NewSource(42))
	globalSchedulerState = s
	t.Cleanup(func() { globalSchedulerState = prev })
	return s
}

// (c) empty candidates → delegate round-robin.
func TestSchedulerPick_EmptyDelegate(t *testing.T) {
	testSchedulerState(t)
	resp := schedulerPick(pluginapi.SchedulerPickRequest{})
	if resp.Handled || resp.DelegateBuiltin != pluginapi.SchedulerBuiltinRoundRobin {
		t.Fatalf("resp = %+v, want delegate round-robin", resp)
	}
}

// (d) queue_unavailable cooldown excludes the account for RetryAfter seconds.
func TestSchedulerPick_QueueCooldownExclusion(t *testing.T) {
	s := testSchedulerState(t)
	// Hook: queue rejection with structured retry-after arms the cooldown.
	noteSchedulerFailure("auth-1", "qwen3.8-plus", queueErr(30))
	if !globalSchedulerState.isCoolingDown("auth-1", "qwen3.8-plus") {
		t.Fatal("expected cooldown armed by queue_unavailable failure")
	}
	req := pluginapi.SchedulerPickRequest{
		Model: "qwen3.8-plus",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "auth-1", Priority: 10}, {ID: "auth-2", Priority: 20},
		},
	}
	resp := schedulerPick(req)
	if !resp.Handled || resp.AuthID != "auth-2" {
		t.Fatalf("resp = %+v, want auth-2 (auth-1 cooling)", resp)
	}
	// RetryAfter honored: still cooling before 30s, expired after.
	if !globalSchedulerState.isCoolingDown("auth-1", "qwen3.8-plus") {
		t.Fatal("still within retry-after window")
	}
	s.now = func() time.Time {
		return time.Date(2026, 10, 3, 12, 0, 31, 0, time.UTC)
	}
	if globalSchedulerState.isCoolingDown("auth-1", "qwen3.8-plus") {
		t.Fatal("cooldown must expire after retry-after seconds")
	}
	// All cooling → delegate (re-arm both under the new clock).
	noteSchedulerFailure("auth-1", "qwen3.8-plus", queueErr(60))
	noteSchedulerFailure("auth-2", "qwen3.8-plus", queueErr(60))
	resp = schedulerPick(req)
	if resp.Handled || resp.DelegateBuiltin != pluginapi.SchedulerBuiltinRoundRobin {
		t.Fatalf("resp = %+v, want delegate when all cooling", resp)
	}
}

// (e) exhausted quota snapshot excludes the account; all exhausted → delegate.
func TestSchedulerPick_ExhaustedExclusion(t *testing.T) {
	s := testSchedulerState(t)
	noteQuotaSnapshot("auth-1", 0, true)
	noteQuotaSnapshot("auth-2", 500, false)
	req := pluginapi.SchedulerPickRequest{
		Model: "qwen3.8-plus",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "auth-1", Priority: 10}, {ID: "auth-2", Priority: 20},
		},
	}
	resp := schedulerPick(req)
	if !resp.Handled || resp.AuthID != "auth-2" {
		t.Fatalf("resp = %+v, want auth-2 (auth-1 exhausted)", resp)
	}
	noteQuotaSnapshot("auth-2", 0, true)
	resp = schedulerPick(req)
	if resp.Handled || resp.DelegateBuiltin != pluginapi.SchedulerBuiltinRoundRobin {
		t.Fatalf("resp = %+v, want delegate when all exhausted", resp)
	}
	// Expired snapshot stops excluding (TTL via clock seam).
	s.now = func() time.Time {
		return time.Date(2026, 10, 3, 12, 6, 0, 0, time.UTC)
	}
	resp = schedulerPick(pluginapi.SchedulerPickRequest{
		Model:      "qwen3.8-plus",
		Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "auth-1"}},
	})
	if !resp.Handled || resp.AuthID != "auth-1" {
		t.Fatalf("resp = %+v, want auth-1 after snapshot TTL", resp)
	}
}

// (f) session sticky honored; cooling sticky target falls through to weighting.
func TestSchedulerPick_Sticky(t *testing.T) {
	testSchedulerState(t)
	// No derivable key → plain pick, no sticky side effects.
	plain := schedulerPick(pluginapi.SchedulerPickRequest{
		Model:      "qwen3.8-plus",
		Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "auth-1"}},
	})
	if !plain.Handled || plain.AuthID != "auth-1" {
		t.Fatalf("plain pick: %+v", plain)
	}
	req := pluginapi.SchedulerPickRequest{
		Model:   "qwen3.8-plus",
		Options: pluginapi.SchedulerOptions{Headers: map[string][]string{"X-Conversation-ID": {"conv-1"}}},
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "auth-1", Priority: 10}, {ID: "auth-2", Priority: 20},
		},
	}
	resp := schedulerPick(req)
	if !resp.Handled {
		t.Fatalf("first pick not handled: %+v", resp)
	}
	for i := 0; i < 10; i++ {
		if got := schedulerPick(req); got.AuthID != resp.AuthID {
			t.Fatalf("sticky broken: got %s want %s", got.AuthID, resp.AuthID)
		}
	}
	// Sticky target cooling → falls through to the other candidate.
	noteSchedulerFailure(resp.AuthID, "qwen3.8-plus", queueErr(60))
	got := schedulerPick(req)
	if !got.Handled || got.AuthID == resp.AuthID {
		t.Fatalf("sticky must skip cooling target, got %+v", got)
	}
}

// (g) quota remaining weighting: richer account dominates under seeded rng.
func TestSchedulerPick_QuotaWeighted(t *testing.T) {
	testSchedulerState(t)
	noteQuotaSnapshot("auth-1", 1000, false)
	noteQuotaSnapshot("auth-2", 10, false)
	req := pluginapi.SchedulerPickRequest{
		Model: "qwen3.8-plus",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "auth-1", Priority: 10}, {ID: "auth-2", Priority: 20},
		},
	}
	counts := map[string]int{}
	for i := range 200 {
		resp := schedulerPick(req)
		if !resp.Handled {
			t.Fatalf("iteration %d: not handled", i)
		}
		counts[resp.AuthID]++
	}
	if counts["auth-1"] < 150 {
		t.Fatalf("auth-1 should dominate, got %v", counts)
	}
}

// (h) no quota snapshot → priority fallback picks the highest-priority candidate.
func TestSchedulerPick_NoSnapshotPriorityFallback(t *testing.T) {
	testSchedulerState(t)
	resp := schedulerPick(pluginapi.SchedulerPickRequest{
		Model: "qwen3.8-plus",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "auth-1", Priority: 10}, {ID: "auth-2", Priority: 20},
		},
	})
	if !resp.Handled || resp.AuthID != "auth-1" {
		t.Fatalf("resp = %+v, want auth-1 (priority 10 wins)", resp)
	}
}

// (j) concurrent pick smoke — no data race, always handled or delegated.
func TestSchedulerPick_Concurrent(t *testing.T) {
	testSchedulerState(t)
	noteQuotaSnapshot("auth-2", 50, false)
	req := pluginapi.SchedulerPickRequest{
		Model: "qwen3.8-plus",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "auth-1", Priority: 10}, {ID: "auth-2", Priority: 20}, {ID: "auth-3", Priority: 30},
		},
	}
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := schedulerPick(req)
			if !resp.Handled && resp.DelegateBuiltin == "" {
				t.Errorf("expected handled or delegate")
			}
		}()
	}
	wg.Wait()
}
