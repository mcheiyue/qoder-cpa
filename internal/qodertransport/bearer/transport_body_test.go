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

func TestStream_ReasoningEffortForwarded(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:           "qoder-1",
		Messages:        []Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: ptrString("high"),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var body struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.ReasoningEffort != "high" {
		t.Errorf("reasoning_effort: got %q, want high", body.ReasoningEffort)
	}
}

func TestStream_MaxCompletionTokensForwarded(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:               "qoder-1",
		Messages:            []Message{{Role: "user", Content: "hi"}},
		MaxCompletionTokens: ptrInt(4096),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var body struct {
		MaxCompletionTokens int `json:"max_completion_tokens"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.MaxCompletionTokens != 4096 {
		t.Errorf("max_completion_tokens: got %d, want 4096", body.MaxCompletionTokens)
	}
}

func TestStream_ParallelToolCallsFalseForwarded(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:             "qoder-1",
		Messages:          []Message{{Role: "user", Content: "hi"}},
		ParallelToolCalls: ptrBool(false),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := raw["parallel_tool_calls"]
	if !ok {
		t.Fatal("parallel_tool_calls: key missing from wire body")
	}
	if string(v) != "false" {
		t.Errorf("parallel_tool_calls: got %s, want false", v)
	}
}

func TestStream_AdvancedFieldsOmittedWhenNil(t *testing.T) {
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
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"reasoning_effort", "max_completion_tokens", "parallel_tool_calls"} {
		if _, ok := raw[key]; ok {
			t.Errorf("%s: should be omitted when nil, but present in wire body", key)
		}
	}
}

func TestStream_AllAdvancedFieldsTogether(t *testing.T) {
	var bodyBytes []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := tr.Stream(context.Background(), StreamRequest{
		Model:               "qoder-1",
		Messages:            []Message{{Role: "user", Content: "hi"}},
		Temperature:         ptrFloat64(0.5),
		MaxTokens:           ptrInt(2048),
		ReasoningEffort:     ptrString("medium"),
		MaxCompletionTokens: ptrInt(8192),
		ParallelToolCalls:   ptrBool(true),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var body struct {
		Temperature         float64 `json:"temperature"`
		MaxTokens           int     `json:"max_tokens"`
		ReasoningEffort     string  `json:"reasoning_effort"`
		MaxCompletionTokens int     `json:"max_completion_tokens"`
		ParallelToolCalls   bool    `json:"parallel_tool_calls"`
	}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Temperature != 0.5 {
		t.Errorf("temperature: got %f, want 0.5", body.Temperature)
	}
	if body.MaxTokens != 2048 {
		t.Errorf("max_tokens: got %d, want 2048", body.MaxTokens)
	}
	if body.ReasoningEffort != "medium" {
		t.Errorf("reasoning_effort: got %q, want medium", body.ReasoningEffort)
	}
	if body.MaxCompletionTokens != 8192 {
		t.Errorf("max_completion_tokens: got %d, want 8192", body.MaxCompletionTokens)
	}
	if !body.ParallelToolCalls {
		t.Error("parallel_tool_calls: got false, want true")
	}
}
