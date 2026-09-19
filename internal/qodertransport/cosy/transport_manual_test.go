package cosy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransport_ManualQA(t *testing.T) {
	const rawBody = `{"messages":[{"role":"user","content":"hello"}]}`
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assertManualRequest(t, r, rawBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"search\",\"arguments\":\"{}\"}}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4,\"total_tokens\":14}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	transport, err := NewTransport(Config{
		HTTPClient:        server.Client(),
		Endpoint:          EndpointAPI2,
		BaseURL:           server.URL,
		AllowTestEndpoint: true,
		Clock:             func() time.Time { return time.Unix(1700000000, 0) },
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	response, err := transport.Stream(context.Background(), StreamRequest{
		RuntimeFields: deriveTestFields(t),
		RequestBody:   []byte(rawBody),
		RequestID:     "manual-qa",
		CosyVersion:   "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer response.Cancel()

	want := []SSEEventType{SSEReasoningDelta, SSETextDelta, SSEToolDelta, SSEUsage, SSETerminal}
	for index, wantType := range want {
		event, parseErr := response.Parser.ParseContext(response.Context)
		if parseErr != nil {
			t.Fatalf("event %d: %v", index, parseErr)
		}
		if event.Type != wantType {
			t.Fatalf("event %d: got %v, want %v", index, event.Type, wantType)
		}
	}
	if _, err := response.Parser.ParseContext(response.Context); err != io.EOF {
		t.Fatalf("after terminal: got %v, want EOF", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls: got %d, want 1", calls.Load())
	}
}

func assertManualRequest(t *testing.T, r *http.Request, rawBody string) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/algo/api/v2/service/pro/sse/agent_chat_generation" {
		t.Errorf("request target: %s %s", r.Method, r.URL.Path)
	}
	query := r.URL.Query()
	if query.Get("FetchKeys") != "llm_model_result" || query.Get("AgentId") != "agent_common" || query.Get("Encode") != "1" {
		t.Errorf("query: %s", r.URL.RawQuery)
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer COSY.") || r.Header.Get("Cosy-Key") == "" {
		t.Error("missing COSY authentication headers")
	}
	if r.Header.Get("Cosy-Date") != "1700000000" || r.Header.Get("Accept") != "text/event-stream" {
		t.Errorf("headers: date=%q accept=%q", r.Header.Get("Cosy-Date"), r.Header.Get("Accept"))
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if decoded := DecodeBody(body); string(decoded) != rawBody {
		t.Errorf("decoded body: got %q, want %q", decoded, rawBody)
	}
}
