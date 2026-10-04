package main

import (
	"math/rand"
	"sync"
	"time"
)

const (
	// quotaTTL is how long a quota snapshot stays valid.
	quotaTTL = 5 * time.Minute
	// queueCooldownDefault is the cooldown when a queue rejection carries no retry-after.
	queueCooldownDefault = 60 * time.Second
	// stickyMax is the maximum number of sticky key→authID bindings.
	stickyMax = 1024
)

// schedulerState holds all mutable state for the scheduler.
// All fields are goroutine-safe via mu.
type schedulerState struct {
	mu       sync.RWMutex
	quota    map[string]quotaSnapshot // authID → snapshot
	cooldown map[string]time.Time     // authID:model → expiry
	sticky   *lruMap                  // stickyKey → authID
	now      func() time.Time         // test seam; nil = time.Now
	rngMu    sync.Mutex               // guards rng (math/rand.Rand is not concurrent-safe)
	rng      *rand.Rand               // test seam; nil = global rand
}

type quotaSnapshot struct {
	remaining float64
	exhausted bool
	obtained  time.Time
}

func newSchedulerState() *schedulerState {
	return &schedulerState{
		quota:    make(map[string]quotaSnapshot),
		cooldown: make(map[string]time.Time),
		sticky:   newLRUMap(stickyMax),
	}
}

// float64 returns a random float64 in [0, 1). Thread-safe.
func (s *schedulerState) float64() float64 {
	s.rngMu.Lock()
	defer s.rngMu.Unlock()
	if s.rng != nil {
		return s.rng.Float64()
	}
	return rand.Float64()
}

func (s *schedulerState) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// setQuota stores a quota snapshot for an authID.
func (s *schedulerState) setQuota(authID string, remaining float64, exhausted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quota[authID] = quotaSnapshot{remaining: remaining, exhausted: exhausted, obtained: s.clock()}
}

// getQuota returns the snapshot for an authID if still within quotaTTL.
func (s *schedulerState) getQuota(authID string) (quotaSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.quota[authID]
	if !ok || s.clock().Sub(snap.obtained) > quotaTTL {
		return quotaSnapshot{}, false
	}
	return snap, true
}

// isExhausted reports whether a fresh snapshot marks the account exhausted.
func (s *schedulerState) isExhausted(authID string) bool {
	snap, ok := s.getQuota(authID)
	return ok && snap.exhausted
}

// setCooldown records a model-scoped cooldown for (authID, model) with a TTL.
func (s *schedulerState) setCooldown(authID, model string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldown[cooldownKey(authID, model)] = s.clock().Add(ttl)
}

// isCoolingDown reports whether (authID, model) is in cooldown.
func (s *schedulerState) isCoolingDown(authID, model string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exp, ok := s.cooldown[cooldownKey(authID, model)]
	if !ok {
		return false
	}
	if s.clock().After(exp) {
		return false // expired; entry is harmless and bounded (auth×model)
	}
	return true
}

func cooldownKey(authID, model string) string {
	return authID + ":" + model
}

// stickyGet returns the authID bound to key, if present.
// Full lock: lruMap.get touches recency order (mutates).
func (s *schedulerState) stickyGet(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sticky.get(key)
}

// stickySet binds key to authID. Evicts oldest if at capacity.
func (s *schedulerState) stickySet(key, authID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sticky.set(key, authID)
}

// lruMap is a simple LRU: map + ring of keys for eviction order.
type lruMap struct {
	cap   int
	m     map[string]string
	order []string // insertion/update order; index of key tracked separately
	idx   map[string]int
}

func newLRUMap(cap int) *lruMap {
	return &lruMap{
		cap:   cap,
		m:     make(map[string]string),
		order: make([]string, 0, cap),
		idx:   make(map[string]int),
	}
}

func (l *lruMap) get(key string) (string, bool) {
	v, ok := l.m[key]
	if ok {
		l.touch(key)
	}
	return v, ok
}

func (l *lruMap) set(key, val string) {
	if _, exists := l.m[key]; exists {
		l.m[key] = val
		l.touch(key)
		return
	}
	if len(l.m) >= l.cap {
		l.evictOldest()
	}
	l.m[key] = val
	idx := len(l.order)
	l.order = append(l.order, key)
	l.idx[key] = idx
}

func (l *lruMap) touch(key string) {
	i, ok := l.idx[key]
	if !ok || i >= len(l.order)-1 {
		return // already at end
	}
	// Remove from current position and append.
	l.order = append(l.order[:i], l.order[i+1:]...)
	// Rebuild index for moved keys.
	for j := i; j < len(l.order); j++ {
		l.idx[l.order[j]] = j
	}
	l.order = append(l.order, key)
	l.idx[key] = len(l.order) - 1
}

func (l *lruMap) evictOldest() {
	if len(l.order) == 0 {
		return
	}
	oldest := l.order[0]
	l.order = l.order[1:]
	delete(l.m, oldest)
	delete(l.idx, oldest)
	// Rebuild index.
	for j := 0; j < len(l.order); j++ {
		l.idx[l.order[j]] = j
	}
}
