package cosy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransport_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"code":10605}`)
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err == nil {
		t.Error("expected HTTP error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention 401: %v", err)
	}
}

func TestTransport_BusinessError10605(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"code\":10605,\"message\":\"queue full\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
	evt, err := resp.Parser.Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != SSEError {
		t.Errorf("type: got %d, want SSEError", evt.Type)
	}
	se := evt.StreamError
	if se == nil {
		t.Fatal("StreamError is nil")
	}
	if se.Code != 10605 {
		t.Errorf("code: got %d, want 10605", se.Code)
	}
}

func TestTransport_NoTerminal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n")
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
	if _, err := resp.Parser.Parse(); err != nil {
		t.Fatalf("first parse: %v", err)
	}
	_, err = resp.Parser.Parse()
	if err != ErrMissingTerminal {
		t.Errorf("expected ErrMissingTerminal, got %v", err)
	}
}

func TestTransport_TamperedSignature(t *testing.T) {
	fields := deriveTestFields(t)
	fields.Key = "tampered"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
}

func TestTransport_TamperedBody(t *testing.T) {
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{Endpoint: EndpointAPI2})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`not json`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTransport_429Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "rate limited")
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err == nil {
		t.Error("expected error for 429")
	}
}

func TestTransport_5xxError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err == nil {
		t.Error("expected error for 500")
	}
}

func TestTransport_OversizedEvent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		big := strings.Repeat("A", 2*1024*1024)
		fmt.Fprintf(w, "data: %s\n\n", big)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
	if _, err := resp.Parser.Parse(); err != ErrEventTooLarge && err != io.ErrShortBuffer {
		t.Fatalf("oversized parse: got %v", err)
	}
}
