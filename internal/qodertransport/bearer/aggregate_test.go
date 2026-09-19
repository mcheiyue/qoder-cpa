package bearer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAggregate_TextOnly(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qoder-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qoder-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello \"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qoder-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"world\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qoder-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qoder-1\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	cc, err := Aggregate(tr, context.Background(), StreamRequest{
		Model:    "qoder-1",
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if cc.ID != "chatcmpl-1" {
		t.Errorf("id: got %q, want chatcmpl-1", cc.ID)
	}
	if cc.Model != "qoder-1" {
		t.Errorf("model: got %q, want qoder-1", cc.Model)
	}
	if len(cc.Choices) != 1 {
		t.Fatalf("choices: got %d, want 1", len(cc.Choices))
	}
	if cc.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason: got %q, want stop", cc.Choices[0].FinishReason)
	}
	if cc.Choices[0].Message.Content != "Hello world" {
		t.Errorf("content: got %q, want %q", cc.Choices[0].Message.Content, "Hello world")
	}
	if cc.Usage == nil {
		t.Fatal("usage is nil")
	}
	if cc.Usage.PromptTokens != 10 {
		t.Errorf("prompt_tokens: got %d, want 10", cc.Usage.PromptTokens)
	}
	if cc.Usage.CompletionTokens != 5 {
		t.Errorf("completion_tokens: got %d, want 5", cc.Usage.CompletionTokens)
	}
	if cc.Usage.TotalTokens != 15 {
		t.Errorf("total_tokens: got %d, want 15", cc.Usage.TotalTokens)
	}
}

func TestAggregate_WithReasoning(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"thinking...\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8,\"completion_tokens_details\":{\"reasoning_tokens\":1}}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	cc, err := Aggregate(tr, context.Background(), StreamRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if cc.Choices[0].Message.Reasoning != "thinking..." {
		t.Errorf("reasoning: got %q, want thinking...", cc.Choices[0].Message.Reasoning)
	}
	if cc.Choices[0].Message.Content != "answer" {
		t.Errorf("content: got %q, want answer", cc.Choices[0].Message.Content)
	}
	if cc.Usage == nil || cc.Usage.ReasoningTokens != 1 {
		t.Errorf("reasoning_tokens: %v", cc.Usage)
	}
}

func TestAggregate_ToolCalls(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"q\\\":\\\"test\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":10,\"total_tokens\":15}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	cc, err := Aggregate(tr, context.Background(), StreamRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "search for test"}},
	})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(cc.Choices) != 1 {
		t.Fatalf("choices: got %d, want 1", len(cc.Choices))
	}
	toolCalls := cc.Choices[0].Message.ToolCalls
	if len(toolCalls) != 1 {
		t.Fatalf("tool_calls: got %d, want 1", len(toolCalls))
	}
	if toolCalls[0].ID != "call_1" {
		t.Errorf("tool id: got %q, want call_1", toolCalls[0].ID)
	}
	if toolCalls[0].Function.Name != "search" {
		t.Errorf("tool name: got %q, want search", toolCalls[0].Function.Name)
	}
	if !strings.Contains(toolCalls[0].Function.Arguments, "test") {
		t.Errorf("tool arguments: got %q, want contains 'test'", toolCalls[0].Function.Arguments)
	}
}

func TestAggregate_SingleCallOnly(t *testing.T) {
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := Aggregate(tr, context.Background(), StreamRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls: got %d, want 1", calls)
	}
}

func TestAggregate_HTTPErrorPropagated(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "unauthorized")
	}))
	defer ts.Close()
	tr := newTestTransport(t, ts)
	_, err := Aggregate(tr, context.Background(), StreamRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("not HTTPError: %v", err)
	}
	if httpErr.StatusCode != 401 {
		t.Errorf("status: got %d, want 401", httpErr.StatusCode)
	}
}

func TestAggregate_TimeoutPropagated(t *testing.T) {
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
	tr, err := NewTransport(Config{
		HTTPClient:        ts.Client(),
		Token:             "tok_test",
		BaseURL:           ts.URL,
		AllowTestEndpoint: true,
		Timeout:           200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = Aggregate(tr, context.Background(), StreamRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
