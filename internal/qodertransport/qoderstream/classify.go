package qoderstream

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Safe business categories emitted by ClassifyBusiness. These are the only
// category strings the transports may put on StreamError.Message; anything
// else must fall through to SafeErrorCategory/StatusCategory.
const (
	CatQueueUnavailable = "queue_unavailable" // upstream queue rejection (e.g. 10605), NOT quota exhaustion
	CatDailyLimit       = "daily_limit_exceeded"
	CatPlanRequired     = "plan_required"
	CatModelUnavailable = "model_unavailable" // model behind paywall / no entitlement (112 + pricingUrl)
)

// QueuePayload carries the structured queue-rejection payload that rides
// inside upstream error bodies (10605-style). All fields are safe to expose.
type QueuePayload struct {
	IsQueued          bool
	QueueType         string
	ServiceAvailable  bool
	RetryAfterSeconds int
	WaitTime          int
}

type queueProbe struct {
	IsQueued          *bool           `json:"isQueued"`
	QueueType         *string         `json:"queueType"`
	ServiceAvailable  *bool           `json:"serviceAvailable"`
	RetryAfterSeconds *int            `json:"retryAfterSeconds"`
	WaitTime          *int            `json:"waitTime"`
	Body              json.RawMessage `json:"body"`
	Message           json.RawMessage `json:"message"`
	Payload           json.RawMessage `json:"payload"`
}

// ClassifyBusiness maps an upstream business error (code + message + raw
// payload) to a safe category plus any queue payload found in the raw JSON.
// Raw may nest JSON-in-string wrappers up to depth 4 (statusCodeValue envelope,
// body string, message string, payload object).
func ClassifyBusiness(code int, message, raw string) (string, *QueuePayload) {
	q := extractQueue(raw)
	switch {
	case code == 10605:
		return CatQueueUnavailable, q
	case q != nil && (q.RetryAfterSeconds > 0 || q.WaitTime > 0 || q.QueueType != "" || !q.ServiceAvailable || q.IsQueued):
		// Queue-shaped payload without a recognizable code is still a queue
		// rejection — never quota exhaustion.
		return CatQueueUnavailable, q
	case code == 429:
		return CatRateLimited, q
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "billing daily count exceeded"):
		return CatDailyLimit, q
	case strings.Contains(lower, "no usable plan or allowance"):
		return CatPlanRequired, q
	}
	if code == 112 && strings.Contains(strings.ToLower(raw), "pricingurl") {
		return CatModelUnavailable, q
	}
	if strings.TrimSpace(message) == "" {
		return StatusCategory(code), q
	}
	return SafeErrorCategory(message), q
}

// CatRateLimited mirrors the legacy category string used across transports.
const CatRateLimited = "rate_limited"

// extractQueue walks raw JSON up to 4 levels through body/message/payload
// wrappers (string or object) and merges any queue fields found.
func extractQueue(raw string) *QueuePayload {
	var q QueuePayload
	found := false
	current := raw
	for range 4 {
		var probe queueProbe
		if err := json.Unmarshal([]byte(current), &probe); err != nil {
			break
		}
		if probe.IsQueued != nil {
			q.IsQueued = *probe.IsQueued
			found = true
		}
		if probe.QueueType != nil && *probe.QueueType != "" {
			q.QueueType = *probe.QueueType
			found = true
		}
		if probe.ServiceAvailable != nil {
			q.ServiceAvailable = *probe.ServiceAvailable
			found = true
		}
		if probe.RetryAfterSeconds != nil && *probe.RetryAfterSeconds > 0 {
			q.RetryAfterSeconds = *probe.RetryAfterSeconds
			found = true
		}
		if probe.WaitTime != nil && *probe.WaitTime > 0 {
			q.WaitTime = *probe.WaitTime
			found = true
		}
		next := descendJSON(probe.Body, probe.Message, probe.Payload)
		if next == "" {
			break
		}
		current = next
	}
	if !found {
		return nil
	}
	return &q
}

// descendJSON returns the first candidate that is (or string-decodes to) a
// JSON object, so queue fields can live inside a JSON-in-string wrapper.
func descendJSON(candidates ...json.RawMessage) string {
	for _, cand := range candidates {
		if len(cand) == 0 || string(cand) == "null" {
			continue
		}
		trimmed := strings.TrimSpace(string(cand))
		if strings.HasPrefix(trimmed, "{") {
			return trimmed
		}
		var s string
		if json.Unmarshal(cand, &s) == nil {
			s = strings.TrimSpace(s)
			if strings.HasPrefix(s, "{") {
				return s
			}
		}
	}
	return ""
}

// ParseRetryAfter converts a Retry-After header (delta-seconds or HTTP-date)
// into seconds from now. Returns 0 when absent, malformed, or already past.
func ParseRetryAfter(header string, now time.Time) int {
	h := strings.TrimSpace(header)
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs < 0 {
			return 0
		}
		return secs
	}
	if t, err := http.ParseTime(h); err == nil {
		d := int(t.Sub(now).Round(time.Second).Seconds())
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}
