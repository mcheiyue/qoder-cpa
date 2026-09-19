package bearer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManualQA_Stream(t *testing.T) {
	const model = "qoder-pro"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assertManualRequest(t, r, model)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"thinking\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-qa\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"q\\\":\\\"test\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4,\"total_tokens\":14,\"completion_tokens_details\":{\"reasoning_tokens\":1}}}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	tr, err := NewTransport(Config{
		HTTPClient:        server.Client(),
		Token:             "tok_test",
		BaseURL:           server.URL,
		AllowTestEndpoint: true,
		Timeout:           5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	resp, err := tr.Stream(context.Background(), StreamRequest{
		Model:       model,
		Messages:    []Message{{Role: "user", Content: "hello"}},
		RequestID:   "manual-qa",
		SessionID:   "sess-qa",
		Temperature: ptrFloat64(0.5),
		MaxTokens:   ptrInt(512),
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()

	want := []SSEEventType{SSEReasoningDelta, SSETextDelta, SSEToolDelta, SSEUsage, SSETerminal}
	for index, wantType := range want {
		event, parseErr := resp.Parser.ParseContext(resp.Context)
		if parseErr != nil {
			t.Fatalf("event %d: %v", index, parseErr)
		}
		// Skip role-only deltas that yield SSEUnknown.
		for event.Type == SSEUnknown {
			event, parseErr = resp.Parser.ParseContext(resp.Context)
			if parseErr != nil {
				t.Fatalf("event %d (skip unknown): %v", index, parseErr)
			}
		}
		if event.Type != wantType {
			t.Fatalf("event %d: got %v, want %v", index, event.Type, wantType)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls: got %d, want 1", calls.Load())
	}
}

func TestManualQA_Aggregate(t *testing.T) {
	const model = "qoder-pro"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assertManualRequest(t, r, model)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa2\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa2\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Answer\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa2\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-qa2\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"id\\\":\\\"42\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa2\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-qa2\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\""+model+"\",\"choices\":[],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":3,\"total_tokens\":11}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	tr, err := NewTransport(Config{
		HTTPClient:        server.Client(),
		Token:             "tok_test",
		BaseURL:           server.URL,
		AllowTestEndpoint: true,
		Timeout:           5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	cc, err := Aggregate(tr, context.Background(), StreamRequest{
		Model:       model,
		Messages:    []Message{{Role: "user", Content: "lookup 42"}},
		RequestID:   "manual-qa-agg",
		Temperature: ptrFloat64(0.5),
		MaxTokens:   ptrInt(512),
	})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if cc.ID != "chatcmpl-qa2" {
		t.Errorf("id: got %q, want chatcmpl-qa2", cc.ID)
	}
	if cc.Model != model {
		t.Errorf("model: got %q, want %q", cc.Model, model)
	}
	if len(cc.Choices) != 1 {
		t.Fatalf("choices: got %d, want 1", len(cc.Choices))
	}
	if cc.Choices[0].Message.Content != "Answer" {
		t.Errorf("content: got %q, want Answer", cc.Choices[0].Message.Content)
	}
	if cc.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason: got %q, want stop", cc.Choices[0].FinishReason)
	}
	if len(cc.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("tool_calls: got %d, want 1", len(cc.Choices[0].Message.ToolCalls))
	}
	if cc.Choices[0].Message.ToolCalls[0].ID != "call-qa2" {
		t.Errorf("tool id: got %q, want call-qa2", cc.Choices[0].Message.ToolCalls[0].ID)
	}
	if cc.Choices[0].Message.ToolCalls[0].Function.Name != "lookup" {
		t.Errorf("tool name: got %q, want lookup", cc.Choices[0].Message.ToolCalls[0].Function.Name)
	}
	if cc.Usage == nil {
		t.Fatal("usage is nil")
	}
	if cc.Usage.PromptTokens != 8 || cc.Usage.CompletionTokens != 3 || cc.Usage.TotalTokens != 11 {
		t.Errorf("usage: %+v", cc.Usage)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls: got %d, want 1", calls.Load())
	}
}

func assertManualRequest(t *testing.T, r *http.Request, expectedModel string) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("method: got %q, want POST", r.Method)
	}
	if !strings.HasSuffix(r.URL.Path, "/model/v1/chat/completions") {
		t.Errorf("path: got %q, want .../model/v1/chat/completions", r.URL.Path)
	}
	if r.Header.Get("Authorization") != "Bearer tok_test" {
		t.Errorf("auth: got %q, want Bearer tok_test", r.Header.Get("Authorization"))
	}
	if !strings.Contains(r.Header.Get("User-Agent"), "Qoder") {
		t.Errorf("user-agent: %q", r.Header.Get("User-Agent"))
	}
	if r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("content-type: got %q, want application/json", r.Header.Get("Content-Type"))
	}
	if r.Header.Get("Accept") != "text/event-stream" {
		t.Errorf("accept: got %q, want text/event-stream", r.Header.Get("Accept"))
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var reqBody struct {
		Model         string `json:"model"`
		Stream        bool   `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
		Metadata struct {
			RequestID string `json:"request_id"`
			SessionID string `json:"session_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if reqBody.Model != expectedModel {
		t.Errorf("model: got %q, want %q", reqBody.Model, expectedModel)
	}
	if !reqBody.Stream {
		t.Error("stream: got false, want true")
	}
	if !reqBody.StreamOptions.IncludeUsage {
		t.Error("stream_options.include_usage: got false, want true")
	}
}
