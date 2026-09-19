package bearer

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStream_WireRequestPOST(t *testing.T) {
	ts, a := fakeBearerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:     "qoder-1",
		Messages:  []Message{{Role: "user", Content: "hi"}},
		RequestID: "req-1",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.calls != 1 {
		t.Errorf("calls: got %d, want 1", a.calls)
	}
	if a.method != "POST" {
		t.Errorf("method: got %q, want POST", a.method)
	}
}

func TestStream_WireRequestEndpoint(t *testing.T) {
	var capturedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !strings.Contains(capturedPath, "/model/v1/chat/completions") {
		t.Errorf("path: got %q, want /model/v1/chat/completions", capturedPath)
	}
}

func TestStream_BearerToken(t *testing.T) {
	var capturedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if capturedAuth != "Bearer tok_test" {
		t.Errorf("Authorization: got %q, want Bearer tok_test", capturedAuth)
	}
}

func TestStream_QoderUserAgent(t *testing.T) {
	var capturedUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !strings.Contains(capturedUA, "Qoder") {
		t.Errorf("User-Agent: got %q, want Qoder", capturedUA)
	}
}

func TestStream_ContentTypeJSON(t *testing.T) {
	var capturedCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCT = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !strings.Contains(capturedCT, "application/json") {
		t.Errorf("Content-Type: got %q, want application/json", capturedCT)
	}
}

func TestStream_EmptyModelRejected(t *testing.T) {
	ts, _ := fakeBearerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestStream_EmptyMessagesRejected(t *testing.T) {
	ts, _ := fakeBearerServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model: "qoder-1",
	})
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
}

func TestStream_SingleCallNoRetry(t *testing.T) {
	var calls int
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	mu.Lock()
	n := calls
	mu.Unlock()
	if n != 1 {
		t.Errorf("calls: got %d, want 1", n)
	}
}

func TestStream_InvalidContentTypeRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>Hello</body></html>")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for HTML Content-Type")
	}
	if strings.Contains(err.Error(), "<html>") {
		t.Error("error leaks HTML body")
	}
}

// --- helpers ---

func newTestTransport(t *testing.T, ts *httptest.Server) *Transport {
	t.Helper()
	tr, err := NewTransport(Config{
		HTTPClient:        ts.Client(),
		Token:             "tok_test",
		BaseURL:           ts.URL,
		AllowTestEndpoint: true,
		Timeout:           5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return tr
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }
