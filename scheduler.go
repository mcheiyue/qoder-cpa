package main

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// globalSchedulerState is the package-level state instance.
// Initialized once; all goroutine access is safe.
var globalSchedulerState = newSchedulerState()

// handleSchedulerMethod dispatches scheduler.pick RPC.
func handleSchedulerMethod(_ string, raw []byte) ([]byte, error) {
	var req pluginapi.SchedulerPickRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelopeStatus("invalid_request", "invalid scheduler pick request", http.StatusBadRequest), nil
	}
	resp := schedulerPick(req)
	return okEnvelope(resp)
}

// schedulerPick implements quota-aware auth selection with session sticky,
// queue cooldown exclusion and exhausted-account exclusion. Priority order:
//  1. Empty/no-usable candidates → delegate round-robin.
//  2. All candidates cooling or quota-exhausted → delegate.
//  3. Session sticky: if a derivable sticky key maps to a usable candidate → pick it.
//  4. Quota weighting: higher remaining quota = higher selection probability.
//  5. Priority fallback: highest-priority (lowest numeric) usable candidate.
func schedulerPick(req pluginapi.SchedulerPickRequest) pluginapi.SchedulerPickResponse {
	candidates := usableCandidates(req.Candidates)
	if len(candidates) == 0 {
		return delegate()
	}
	eligible := filterUnavailable(candidates, req.Model)
	if len(eligible) == 0 {
		return delegate()
	}

	// Session sticky.
	if key, ok := deriveStickyKey(req); ok {
		if authID, found := globalSchedulerState.stickyGet(key); found {
			if c := findCandidate(eligible, authID); c != nil {
				globalSchedulerState.stickySet(key, c.ID)
				return pick(c.ID)
			}
		}
	}

	// Quota weighting.
	if chosen := quotaWeightedPick(eligible); chosen != "" {
		if key, ok := deriveStickyKey(req); ok {
			globalSchedulerState.stickySet(key, chosen)
		}
		return pick(chosen)
	}

	// Priority fallback (lowest numeric value = highest priority).
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Priority < eligible[j].Priority
	})
	chosen := eligible[0].ID
	if key, ok := deriveStickyKey(req); ok {
		globalSchedulerState.stickySet(key, chosen)
	}
	return pick(chosen)
}

// noteSchedulerFailure arms a model-scoped queue cooldown when the executor
// surfaces a queue_unavailable rejection. TTL honors the structured
// retry-after payload; absent payload falls back to queueCooldownDefault.
// No-op for any other error category.
func noteSchedulerFailure(authID, model string, err error) {
	var sbe *qodertransport.StreamBusinessError
	if !errors.As(err, &sbe) || sbe.Category != qoderstream.CatQueueUnavailable {
		return
	}
	if strings.TrimSpace(authID) == "" || strings.TrimSpace(model) == "" {
		return
	}
	ttl := queueCooldownDefault
	if sbe.Queue != nil && sbe.Queue.RetryAfterSeconds > 0 {
		ttl = time.Duration(sbe.Queue.RetryAfterSeconds) * time.Second
	}
	globalSchedulerState.setCooldown(authID, model, ttl)
}

// noteQuotaSnapshot records quota state for exclusion and weighting.
// Keyed by auth.ID (the same space as SchedulerAuthCandidate.ID).
func noteQuotaSnapshot(authID string, remaining float64, exhausted bool) {
	if strings.TrimSpace(authID) == "" {
		return
	}
	globalSchedulerState.setQuota(authID, remaining, exhausted)
}

// usableCandidates filters to non-empty-ID candidates. The host already
// filters disabled records before sending.
func usableCandidates(all []pluginapi.SchedulerAuthCandidate) []pluginapi.SchedulerAuthCandidate {
	var out []pluginapi.SchedulerAuthCandidate
	for _, c := range all {
		if strings.TrimSpace(c.ID) == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// filterUnavailable removes candidates in queue cooldown or marked exhausted
// by a fresh quota snapshot.
func filterUnavailable(candidates []pluginapi.SchedulerAuthCandidate, model string) []pluginapi.SchedulerAuthCandidate {
	var out []pluginapi.SchedulerAuthCandidate
	for _, c := range candidates {
		if globalSchedulerState.isCoolingDown(c.ID, model) {
			continue
		}
		if globalSchedulerState.isExhausted(c.ID) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// findCandidate returns the candidate with the given authID, or nil.
func findCandidate(candidates []pluginapi.SchedulerAuthCandidate, authID string) *pluginapi.SchedulerAuthCandidate {
	for i := range candidates {
		if candidates[i].ID == authID {
			return &candidates[i]
		}
	}
	return nil
}

// deriveStickyKey extracts a sticky key from request headers or metadata.
// Priority: X-Conversation-ID header > x-session-id header > "conversation_id"
// metadata field. Returns ("", false) if no key derivable.
func deriveStickyKey(req pluginapi.SchedulerPickRequest) (string, bool) {
	if vals := req.Options.Headers["X-Conversation-ID"]; len(vals) > 0 && vals[0] != "" {
		return "conv:" + vals[0], true
	}
	if vals := req.Options.Headers["x-session-id"]; len(vals) > 0 && vals[0] != "" {
		return "sess:" + vals[0], true
	}
	if v, ok := req.Options.Metadata["conversation_id"]; ok {
		if s, ok := v.(string); ok && s != "" {
			return "conv:" + s, true
		}
	}
	return "", false
}

// quotaWeightedPick selects a candidate weighted by remaining quota.
// Returns "" if no candidate has a known quota snapshot.
func quotaWeightedPick(candidates []pluginapi.SchedulerAuthCandidate) string {
	type weighted struct {
		id     string
		weight float64
	}
	var items []weighted
	total := 0.0
	const epsilon = 0.1 // ensures candidates with 0 remaining still get a chance.
	for _, c := range candidates {
		snap, ok := globalSchedulerState.getQuota(c.ID)
		if !ok {
			continue // no snapshot → skip this layer.
		}
		w := math.Max(snap.remaining, 0) + epsilon
		items = append(items, weighted{id: c.ID, weight: w})
		total += w
	}
	if len(items) == 0 || total == 0 {
		return "" // no quota data → fall through to priority.
	}
	r := globalSchedulerState.float64() * total
	for _, item := range items {
		r -= item.weight
		if r <= 0 {
			return item.id
		}
	}
	// Floating-point tail.
	return items[len(items)-1].id
}

func delegate() pluginapi.SchedulerPickResponse {
	return pluginapi.SchedulerPickResponse{
		DelegateBuiltin: pluginapi.SchedulerBuiltinRoundRobin,
		Handled:         false,
	}
}

func pick(authID string) pluginapi.SchedulerPickResponse {
	return pluginapi.SchedulerPickResponse{
		AuthID:  authID,
		Handled: true,
	}
}
