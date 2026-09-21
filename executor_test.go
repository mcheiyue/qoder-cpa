package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type fakeChatHandle struct {
	mu         sync.Mutex
	chunks     [][]byte
	index      int
	cancelled  bool
	cancelCh   chan struct{}
	cancelOnce sync.Once
}

func (h *fakeChatHandle) ReadChunk() ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.index >= len(h.chunks) {
		return nil, io.EOF
	}
	chunk := h.chunks[h.index]
	h.index++
	return chunk, nil
}

func (h *fakeChatHandle) Cancel() {
	h.mu.Lock()
	h.cancelled = true
	h.mu.Unlock()
	if h.cancelCh != nil {
		h.cancelOnce.Do(func() { close(h.cancelCh) })
	}
}

func (h *fakeChatHandle) isCancelled() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cancelled
}

type fakeChatTransport struct {
	handle qodertransport.StreamHandle
	err    error
	calls  int
}

func (t *fakeChatTransport) StreamChat(_ context.Context, _ qodertransport.StreamRequest) (qodertransport.StreamHandle, error) {
	t.calls++
	return t.handle, t.err
}

type fakeSelector struct{ transport qodertransport.ChatTransport }

func (s fakeSelector) Select(string) (qodertransport.ChatTransport, error) { return s.transport, nil }

func TestExecutorExecuteAggregatesSingleStream(t *testing.T) {
	handle := &fakeChatHandle{chunks: executorChatChunks()}
	transport := &fakeChatTransport{handle: handle}
	service := testExecutorService(transport, nil)

	response, err := service.execute(context.Background(), executorRequest(t, false))
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(response.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 || body.Choices[0].Message.Content != "hello" || body.Usage.TotalTokens != 7 {
		t.Fatalf("calls=%d body=%s", transport.calls, response.Payload)
	}
	if !handle.isCancelled() {
		t.Fatal("stream handle was not closed")
	}
}

func TestExecutorExecuteStreamEmitsAndCloses(t *testing.T) {
	handle := &fakeChatHandle{chunks: executorChatChunks()}
	transport := &fakeChatTransport{handle: handle}
	var emitted [][]byte
	closed := make(chan struct{})
	service := testExecutorService(transport, func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostStreamEmit:
			request := payload.(map[string]any)
			emitted = append(emitted, append([]byte(nil), request["payload"].([]byte)...))
		case pluginabi.MethodHostStreamClose:
			close(closed)
		}
		return json.RawMessage(`{}`), nil
	})

	response, err := service.executeStream(context.Background(), executorRequest(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if response.Headers.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("headers=%v", response.Headers)
	}
	<-closed
	if len(emitted) != len(handle.chunks) || string(emitted[len(emitted)-1]) != "data: [DONE]\n\n" {
		t.Fatalf("emitted=%q", emitted)
	}
	if !handle.isCancelled() {
		t.Fatal("stream handle was not closed")
	}
}

func TestExecutorRejectsUnsupportedFormatBeforeTransport(t *testing.T) {
	transport := &fakeChatTransport{}
	service := testExecutorService(transport, nil)
	request := executorRequest(t, false)
	request.Format = "openai-response"
	_, err := service.execute(context.Background(), request)
	if err == nil || transport.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, transport.calls)
	}
}

func TestExecutorAcceptsCPAOpenAIFormatAlias(t *testing.T) {
	handle := &fakeChatHandle{chunks: executorChatChunks()}
	transport := &fakeChatTransport{handle: handle}
	service := testExecutorService(transport, nil)
	request := executorRequest(t, false)
	request.Format = "openai"
	if _, err := service.execute(context.Background(), request); err != nil {
		t.Fatalf("execute with CPA openai format: %v", err)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls=%d, want 1", transport.calls)
	}
}

func TestExecutorCountTokensMarksEstimate(t *testing.T) {
	service := testExecutorService(&fakeChatTransport{}, nil)
	response, err := service.countTokens(executorRequest(t, false))
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		InputTokens int  `json:"input_tokens"`
		Estimated   bool `json:"estimated"`
	}
	if err := json.Unmarshal(response.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.InputTokens == 0 || !body.Estimated {
		t.Fatalf("payload=%s", response.Payload)
	}
}

func TestExecutorABIIdentifierAndUnsupportedHTTP(t *testing.T) {
	identifierEnvelope, err := handleMethod(pluginabi.MethodExecutorIdentifier, nil)
	if err != nil {
		t.Fatal(err)
	}
	identifier := decodeResult[struct {
		Identifier string `json:"identifier"`
	}](t, identifierEnvelope)
	if identifier.Identifier != executorID {
		t.Fatalf("identifier=%q", identifier.Identifier)
	}

	httpEnvelope, err := handleMethod(pluginabi.MethodExecutorHTTPRequest, nil)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testEnvelope[struct{}]
	if err := json.Unmarshal(httpEnvelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OK || decoded.Error == nil || decoded.Error.HTTPStatus != http.StatusNotImplemented {
		t.Fatalf("envelope=%s", httpEnvelope)
	}
}

func TestExecutorStreamEmitFailureCancelsAndCloses(t *testing.T) {
	handle := &fakeChatHandle{chunks: executorChatChunks(), cancelCh: make(chan struct{})}
	transport := &fakeChatTransport{handle: handle}
	closed := make(chan string, 1)
	service := testExecutorService(transport, func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostStreamEmit:
			return nil, errors.New("downstream closed")
		case pluginabi.MethodHostStreamClose:
			request := payload.(map[string]any)
			closed <- request["error"].(string)
		}
		return json.RawMessage(`{}`), nil
	})
	if _, err := service.executeStream(context.Background(), executorRequest(t, true)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handle.cancelCh:
	case <-time.After(time.Second):
		t.Fatal("stream handle was not canceled")
	}
	if message := <-closed; message != "downstream closed" {
		t.Fatalf("close error=%q", message)
	}
}

func TestExecutorABIPropagatesUpstream503(t *testing.T) {
	previous := defaultExecutorService
	t.Cleanup(func() { defaultExecutorService = previous })
	defaultExecutorService = testExecutorService(
		&fakeChatTransport{err: &bearer.HTTPError{StatusCode: http.StatusServiceUnavailable}}, nil,
	)
	raw, _ := json.Marshal(executorRequest(t, false))
	envelope, err := handleMethod(pluginabi.MethodExecutorExecute, raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testEnvelope[pluginapi.ExecutorResponse]
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OK || decoded.Error == nil || decoded.Error.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("envelope=%s", envelope)
	}
}

func TestExecutorABIStreamResponseIsJSONSafe(t *testing.T) {
	previous := defaultExecutorService
	t.Cleanup(func() { defaultExecutorService = previous })
	closed := make(chan struct{})
	defaultExecutorService = testExecutorService(
		&fakeChatTransport{handle: &fakeChatHandle{chunks: executorChatChunks()}},
		func(method string, _ any) (json.RawMessage, error) {
			if method == pluginabi.MethodHostStreamClose {
				close(closed)
			}
			return json.RawMessage(`{}`), nil
		},
	)
	raw, _ := json.Marshal(executorRequest(t, true))
	envelope, err := handleMethod(pluginabi.MethodExecutorExecuteStream, raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testEnvelope[struct {
		Headers http.Header `json:"headers"`
	}]
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK {
		t.Fatalf("envelope=%s", envelope)
	}
	var result struct {
		Headers http.Header `json:"headers"`
	}
	if err := json.Unmarshal(decoded.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Headers.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("headers=%v", result.Headers)
	}
	<-closed
}

func testExecutorService(transport qodertransport.ChatTransport, hostCall func(string, any) (json.RawMessage, error)) executorService {
	if hostCall == nil {
		hostCall = func(string, any) (json.RawMessage, error) { return json.RawMessage(`{}`), nil }
	}
	return executorService{
		hostCall: hostCall,
		selectorFactory: func(*http.Client, qoderauth.Credential) (transportSelector, error) {
			return fakeSelector{transport: transport}, nil
		},
	}
}

func executorRequest(t *testing.T, stream bool) rpcExecutorRequest {
	t.Helper()
	credential := qoderauth.Credential{
		AccessToken: "access", UserID: "user", Profile: qoderauth.TransportProfileBearerOpenAI,
	}
	data, err := authData(credential, "")
	if err != nil {
		t.Fatal(err)
	}
	return rpcExecutorRequest{
		ExecutorRequest: pluginapi.ExecutorRequest{
			AuthID: "qoder-user", AuthProvider: "qoder", Model: "qoder/model-a",
			Format: "chat-completions", Stream: stream, StorageJSON: data.StorageJSON,
			Payload: []byte(`{"model":"qoder/model-a","messages":[{"role":"user","content":"hi"}]}`),
			Headers: http.Header{"X-Request-Id": {"req-1"}},
		},
		StreamID: "stream-1", HostCallbackID: "callback-1",
	}
}

func executorChatChunks() [][]byte {
	return [][]byte{
		[]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"qoder/model-a\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n"),
		[]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"qoder/model-a\",\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":3,\"total_tokens\":7}}\n\n"),
		[]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"model\":\"qoder/model-a\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"),
		[]byte("data: [DONE]\n\n"),
	}
}
