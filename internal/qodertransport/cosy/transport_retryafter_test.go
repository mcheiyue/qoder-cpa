package cosy

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
		fmt.Fprint(w, `{"code":"rate_limited","message":"slow down"}`)
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{
		HTTPClient:        ts.Client(),
		Endpoint:          EndpointAPI2,
		BaseURL:           ts.URL,
		AllowTestEndpoint: true,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields,
		RequestBody:   []byte(`{}`),
		RequestID:     "r",
		CosyVersion:   "1.1.34",
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
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{
		HTTPClient:        ts.Client(),
		Endpoint:          EndpointAPI2,
		BaseURL:           ts.URL,
		AllowTestEndpoint: true,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields,
		RequestBody:   []byte(`{}`),
		RequestID:     "r",
		CosyVersion:   "1.1.34",
	})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("not HTTPError: %v", err)
	}
	if httpErr.RetryAfterSec <= 0 {
		t.Errorf("RetryAfterSec: got %d, want > 0", httpErr.RetryAfterSec)
	}
}
