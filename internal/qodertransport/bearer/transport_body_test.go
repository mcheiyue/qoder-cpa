package bearer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStream_StreamEnabledInBody(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
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
	var body map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if string(body["stream"]) != "true" {
		t.Errorf("stream: got %s, want true", body["stream"])
	}
	var opts struct {
		IncludeUsage bool `json:"include_usage"`
	}
	if err := json.Unmarshal(body["stream_options"], &opts); err != nil {
		t.Fatalf("unmarshal stream_options: %v", err)
	}
	if !opts.IncludeUsage {
		t.Error("stream_options.include_usage: got false, want true")
	}
}

func TestStream_RequestIDInMetadata(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:     "qoder-1",
		Messages:  []Message{{Role: "user", Content: "hi"}},
		RequestID: "my-req-id",
		SessionID: "my-sess-id",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var body struct {
		Metadata struct {
			RequestID string `json:"request_id"`
			SessionID string `json:"session_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Metadata.RequestID != "my-req-id" {
		t.Errorf("metadata.request_id: got %q, want my-req-id", body.Metadata.RequestID)
	}
	if body.Metadata.SessionID != "my-sess-id" {
		t.Errorf("metadata.session_id: got %q, want my-sess-id", body.Metadata.SessionID)
	}
}

func TestStream_MessagesForwarded(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model: "qoder-1",
		Messages: []Message{
			{Role: "system", Content: "be helpful"},
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var body struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Messages) != 2 {
		t.Fatalf("messages: got %d, want 2", len(body.Messages))
	}
	if body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
		t.Errorf("message roles: %q, %q", body.Messages[0].Role, body.Messages[1].Role)
	}
}

func TestStream_ToolsForwarded(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	tool := Tool{
		Type: "function",
		Function: FunctionDef{
			Name:        "get_weather",
			Description: "get weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "weather?"}},
		Tools:    []Tool{tool},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var body struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Tools) != 1 {
		t.Fatalf("tools: got %d, want 1", len(body.Tools))
	}
	if body.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tool name: got %q, want get_weather", body.Tools[0].Function.Name)
	}
}

func TestStream_TemperatureAndMaxTokens(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:       "qoder-1",
		Messages:    []Message{{Role: "user", Content: "hi"}},
		Temperature: ptrFloat64(0.7),
		MaxTokens:   ptrInt(1024),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var body struct {
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Temperature != 0.7 {
		t.Errorf("temperature: got %f, want 0.7", body.Temperature)
	}
	if body.MaxTokens != 1024 {
		t.Errorf("max_tokens: got %d, want 1024", body.MaxTokens)
	}
}

func TestStream_ToolChoiceForwarded(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:      "qoder-1",
		Messages:   []Message{{Role: "user", Content: "hi"}},
		ToolChoice: json.RawMessage(`"auto"`),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var body struct {
		ToolChoice string `json:"tool_choice"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ToolChoice != "auto" {
		t.Errorf("tool_choice: got %q, want auto", body.ToolChoice)
	}
}
