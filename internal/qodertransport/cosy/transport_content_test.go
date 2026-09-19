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

func TestTransport_NonSSEContentTypeRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>Hello</body></html>")
	}))
	defer ts.Close()
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), streamRequestForTest(t))
	if err == nil {
		t.Fatal("expected error for HTML Content-Type")
	}
	var ctErr *InvalidContentTypeError
	if !errors.As(err, &ctErr) || ctErr.ContentType != "text/html" {
		t.Fatalf("content type error: %v", err)
	}
	if strings.Contains(err.Error(), "<html>") {
		t.Error("error leaks HTML body")
	}
}

func TestTransport_JSONContentTypeRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"error":"unexpected json response"}`)
	}))
	defer ts.Close()
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), streamRequestForTest(t))
	var ctErr *InvalidContentTypeError
	if !errors.As(err, &ctErr) {
		t.Fatalf("content type error: %v", err)
	}
}

func TestTransport_SSEContentTypeAccepted(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	resp, err := tr.Stream(context.Background(), streamRequestForTest(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Cancel()
}

func streamRequestForTest(t *testing.T) StreamRequest {
	t.Helper()
	return StreamRequest{
		RuntimeFields: deriveTestFields(t),
		RequestBody:   []byte(`{}`),
		RequestID:     "r",
		CosyVersion:   "1.1.34",
	}
}
