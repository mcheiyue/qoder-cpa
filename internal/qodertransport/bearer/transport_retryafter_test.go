package bearer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransport_HTTPError_RetryAfterSeconds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":"rate_limited"}}`)
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("not HTTPError: %v", err)
	}
	if httpErr.RetryAfterSec != 30 {
		t.Errorf("RetryAfterSec: got %d, want 30", httpErr.RetryAfterSec)
	}
}

func TestTransport_HTTPError_RetryAfterHTTPDate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "Wed, 30 Sep 2099 12:00:00 GMT")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("not HTTPError: %v", err)
	}
	if httpErr.RetryAfterSec <= 0 {
		t.Errorf("RetryAfterSec: got %d, want > 0", httpErr.RetryAfterSec)
	}
}
