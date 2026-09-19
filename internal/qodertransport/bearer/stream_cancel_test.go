package bearer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStream_ContextCancel(t *testing.T) {
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
	ctx, cancel := context.WithCancel(context.Background())
	resp, err := tr.Stream(ctx, StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	cancel()
	resp.Cancel()
	_, err = resp.Parser.ParseContext(resp.Context)
	if err == nil {
		t.Error("expected error after Cancel")
	}
}

func TestStream_Timeout(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		slow.Close()
	})
	tr, err := NewTransport(Config{
		HTTPClient:        slow.Client(),
		Token:             "tok_test",
		BaseURL:           slow.URL,
		AllowTestEndpoint: true,
		Timeout:           200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestStream_CallerDeadlineShorter(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		slow.Close()
	})
	tr, err := NewTransport(Config{
		HTTPClient:        slow.Client(),
		Token:             "tok_test",
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
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected timeout from caller deadline")
	}
}
