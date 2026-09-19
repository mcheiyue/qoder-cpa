package cosy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Timeout via internal stream context
// ---------------------------------------------------------------------------

func TestTransport_Timeout_EnforcedViaContext(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		slow.Close()
	})
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{
		HTTPClient:        slow.Client(),
		Endpoint:          EndpointAPI2,
		BaseURL:           slow.URL,
		AllowTestEndpoint: true,
		Timeout:           200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	ctx := context.Background()
	_, err = tr.Stream(ctx, StreamRequest{
		RuntimeFields: fields,
		RequestBody:   []byte(`{}`),
		RequestID:     "r",
		CosyVersion:   "1.1.34",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !contextIsDeadlineExceeded(err) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestTransport_Timeout_RespectsEarlierCallerDeadline(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		slow.Close()
	})
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{
		HTTPClient:        slow.Client(),
		Endpoint:          EndpointAPI2,
		BaseURL:           slow.URL,
		AllowTestEndpoint: true,
		Timeout:           10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = tr.Stream(ctx, StreamRequest{
		RuntimeFields: fields,
		RequestBody:   []byte(`{}`),
		RequestID:     "r",
		CosyVersion:   "1.1.34",
	})
	if err == nil {
		t.Fatal("expected timeout from caller deadline")
	}
	if !contextIsDeadlineExceeded(err) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// StreamResponse.Cancel behavior
// ---------------------------------------------------------------------------

func TestTransport_Cancel_ClosesBodyAndCancelsContext(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		ts.Close()
	})
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
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields,
		RequestBody:   []byte(`{}`),
		RequestID:     "r",
		CosyVersion:   "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	resp.Cancel()
	// After cancel, ParseContext must return the cancellation error.
	_, err = resp.Parser.ParseContext(resp.Context)
	if err == nil {
		t.Error("expected error after Cancel")
	}
}

func TestTransport_Cancel_PreventsFurtherEvents(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		ts.Close()
	})
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
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields,
		RequestBody:   []byte(`{}`),
		RequestID:     "r",
		CosyVersion:   "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	resp.Cancel()
	// Subsequent parses must fail — context cancelled and body closed.
	_, err = resp.Parser.ParseContext(resp.Context)
	if err == nil {
		t.Error("expected error after Cancel")
	}
}

// ---------------------------------------------------------------------------
// BuildHTTPRequest clock seam
// ---------------------------------------------------------------------------

func TestBuildHTTPRequestAt_PredictableDate(t *testing.T) {
	fields, err := DeriveRuntimeFields(
		&fixedEntropy{data: bytes.Repeat([]byte{0x41}, 256)},
		RuntimeFieldInput{UID: "clk", OrganizationTags: []string{}, DataPolicyAgreed: true},
	)
	if err != nil {
		t.Fatalf("DeriveRuntimeFields: %v", err)
	}
	fixed := time.Unix(1700000000, 0)
	parts, err := BuildHTTPRequestAt(EndpointAPI2, []byte(`{"x":1}`), fields, "req", "1.1.34", fixed)
	if err != nil {
		t.Fatalf("BuildHTTPRequestAt: %v", err)
	}
	if parts.Date != "1700000000" {
		t.Errorf("Date: got %q, want 1700000000", parts.Date)
	}
	parts2, _ := BuildHTTPRequestAt(EndpointAPI2, []byte(`{"x":1}`), fields, "req", "1.1.34", fixed)
	if parts.Auth != parts2.Auth {
		t.Error("same clock should produce same Auth")
	}
}

func TestTransport_ClockInject(t *testing.T) {
	fields := deriveTestFields(t)
	fixedTime := time.Unix(1700000042, 0)
	var capturedDate string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDate = r.Header.Get("Cosy-Date")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr, err := NewTransport(Config{
		HTTPClient:        ts.Client(),
		Endpoint:          EndpointAPI2,
		BaseURL:           ts.URL,
		AllowTestEndpoint: true,
		Clock:             func() time.Time { return fixedTime },
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields,
		RequestBody:   []byte(`{}`),
		RequestID:     "r",
		CosyVersion:   "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	resp.Cancel()
	if capturedDate != "1700000042" {
		t.Errorf("Cosy-Date: got %q, want 1700000042", capturedDate)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func contextIsDeadlineExceeded(err error) bool {
	return err == context.DeadlineExceeded
}
