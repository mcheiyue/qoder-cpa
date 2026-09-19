package bearer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStream_HTTPError_401(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "token expired secret_abc123")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
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

func TestStream_HTTPError_403(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("not HTTPError: %v", err)
	}
	if httpErr.Category() != CatForbidden {
		t.Errorf("Category: got %q, want %q", httpErr.Category(), CatForbidden)
	}
}

func TestStream_HTTPError_429(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("not HTTPError: %v", err)
	}
	if httpErr.Category() != CatRateLimited {
		t.Errorf("Category: got %q, want %q", httpErr.Category(), CatRateLimited)
	}
}

func TestStream_HTTPError_5xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("not HTTPError: %v", err)
	}
	if httpErr.Category() != CatUpstreamError {
		t.Errorf("Category: got %q, want %q", httpErr.Category(), CatUpstreamError)
	}
}

func TestStream_HTTPError_BodyNotInErrorString(t *testing.T) {
	canary := "sk-canary-TOKEN_LEAK_12345"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, canary)
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), canary) {
		t.Errorf("error string leaks token canary: %s", err.Error())
	}
}

func TestStream_NonSSEContentTypeRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"error":"unexpected json response"}`)
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	var ctErr *InvalidContentTypeError
	if !errors.As(err, &ctErr) {
		t.Fatalf("content type error: %v", err)
	}
}

func TestStream_SSEContentTypeAccepted(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	resp, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Cancel()
}

func TestStream_CancelClosesBody(t *testing.T) {
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
	tr := newTestTransport(t, ts)
	resp, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	resp.Cancel()
	_, err = resp.Parser.ParseContext(resp.Context)
	if err == nil {
		t.Error("expected error after Cancel")
	}
}

func TestStream_MissingTerminal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	resp, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
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

func TestStream_OversizedEvent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		big := strings.Repeat("A", 2*1024*1024)
		fmt.Fprintf(w, "data: %s\n\n", big)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	resp, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
	_, err = resp.Parser.Parse()
	if err != ErrEventTooLarge && err != io.ErrShortBuffer {
		t.Fatalf("oversized parse: got %v", err)
	}
}
