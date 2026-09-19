package cosy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// HTTP non-2xx typed errors
// ---------------------------------------------------------------------------

func TestTransport_HTTPError_TypedStatus401(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "token expired secret_abc123")
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
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error is not *HTTPError: %v", err)
	}
	if httpErr.StatusCode != 401 {
		t.Errorf("StatusCode: got %d, want 401", httpErr.StatusCode)
	}
	if httpErr.Category() != CatAuthFailure {
		t.Errorf("Category: got %q, want %q", httpErr.Category(), CatAuthFailure)
	}
	if strings.Contains(err.Error(), "secret_abc123") {
		t.Error("error string leaks response body")
	}
}

func TestTransport_HTTPError_TypedStatus403(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
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
	if httpErr.Category() != CatForbidden {
		t.Errorf("Category: %q, want %q", httpErr.Category(), CatForbidden)
	}
}

func TestTransport_HTTPError_TypedStatus429(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
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
	if httpErr.Category() != CatRateLimited {
		t.Errorf("Category: %q, want %q", httpErr.Category(), CatRateLimited)
	}
}

func TestTransport_HTTPError_TypedStatus5xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
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
	if httpErr.Category() != CatUpstreamError {
		t.Errorf("Category: %q, want %q", httpErr.Category(), CatUpstreamError)
	}
}

func TestTransport_HTTPError_BodyNotInErrorString(t *testing.T) {
	canary := "sk-canary-TOKEN_LEAK_12345"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, canary)
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
	if err == nil {
		t.Fatal("expected error")
	}
	errStr := err.Error()
	if strings.Contains(errStr, canary) {
		t.Errorf("error string leaks token canary: %s", errStr)
	}
}
