package cosy

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

func fakeServer(t *testing.T) (*httptest.Server, *requestAssertions) {
	t.Helper()
	a := &requestAssertions{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.calls++
		a.method = r.Method
		a.url = r.URL.String()
		a.authHeader = r.Header.Get("Authorization")
		a.cosyKey = r.Header.Get("Cosy-Key")
		a.dataPolicy = r.Header.Get("Cosy-Data-Policy")
		a.userID = r.Header.Get("Cosy-User")
		a.orgID = r.Header.Get("Cosy-Organization-Id")
		a.orgTags = r.Header.Get("Cosy-Organization-Tags")
		a.modelKey = r.Header.Get("X-Model-Key")
		a.modelSource = r.Header.Get("X-Model-Source")
		a.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, a
}

type requestAssertions struct {
	mu          sync.Mutex
	calls       int
	method      string
	url         string
	authHeader  string
	cosyKey     string
	dataPolicy  string
	userID      string
	orgID       string
	orgTags     string
	modelKey    string
	modelSource string
}

func TestTransport_Stream_FakeServer(t *testing.T) {
	ts, a := fakeServer(t)
	fields := deriveTestFields(t)
	cfg := Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true,
		UserID: "user-1", OrganizationID: "org-1", OrganizationTags: []string{"a", "b"}}
	tr, err := NewTransport(cfg)
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := tr.Stream(ctx, StreamRequest{
		RuntimeFields: fields,
		RequestBody:   []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		RequestID:     "r1",
		CosyVersion:   "1.1.34",
		ModelKey:      "qfmodel", ModelSource: "system",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()

	var events []SSEEvent
	for {
		evt, err := resp.Parser.Parse()
		if err != nil {
			break
		}
		events = append(events, evt)
	}
	a.mu.Lock()
	calls := a.calls
	method := a.method
	auth := a.authHeader
	key := a.cosyKey
	a.mu.Unlock()

	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
	if method != "POST" {
		t.Errorf("method: %q, want POST", method)
	}
	if !strings.HasPrefix(auth, "Bearer COSY.") {
		t.Errorf("auth: %q", auth)
	}
	if key == "" {
		t.Error("Cosy-Key missing")
	}
	a.mu.Lock()
	dataPolicy, userID, orgID, orgTags, modelKey, modelSource := a.dataPolicy, a.userID, a.orgID, a.orgTags, a.modelKey, a.modelSource
	a.mu.Unlock()
	for name, values := range map[string][2]string{
		"Cosy-Data-Policy": {dataPolicy, "agree"}, "Cosy-User": {userID, "user-1"},
		"Cosy-Organization-Id": {orgID, "org-1"}, "Cosy-Organization-Tags": {orgTags, "a,b"},
		"X-Model-Key": {modelKey, "qfmodel"}, "X-Model-Source": {modelSource, "system"},
	} {
		if values[0] != values[1] {
			t.Errorf("%s: got %q, want %q", name, values[0], values[1])
		}
	}
	types := make(map[SSEEventType]int)
	for _, e := range events {
		types[e.Type]++
	}
	if types[SSEReasoningDelta] != 1 {
		t.Errorf("reasoning: %d", types[SSEReasoningDelta])
	}
	if types[SSETextDelta] != 1 {
		t.Errorf("text: %d", types[SSETextDelta])
	}
	if types[SSEUsage] != 1 {
		t.Errorf("usage: %d", types[SSEUsage])
	}
	if types[SSETerminal] != 1 {
		t.Errorf("terminal: %d", types[SSETerminal])
	}
}

func TestTransport_MissingFields(t *testing.T) {
	tr, err := NewTransport(Config{Endpoint: EndpointAPI2})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	_, err = tr.Stream(context.Background(), StreamRequest{
		RequestBody: []byte("{}"), RequestID: "r",
	})
	if err == nil {
		t.Error("expected error for missing runtime fields")
	}
}

func TestTransport_ContextCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"p\"}}]}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	ctx, cancel := context.WithCancel(context.Background())
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	resp, err := tr.Stream(ctx, StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	evt, err := resp.Parser.Parse()
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	if evt.Type != SSETextDelta {
		t.Errorf("type: %d", evt.Type)
	}
	cancel()
	resp.Cancel()
}

func TestTransport_EndpointAPI3(t *testing.T) {
	var capturedURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI3, BaseURL: ts.URL, AllowTestEndpoint: true})
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
	if _, err := resp.Parser.Parse(); err != nil {
		t.Fatalf("parse terminal: %v", err)
	}
	if !strings.Contains(capturedURL, "Encode=1") {
		t.Errorf("URL missing Encode=1: %s", capturedURL)
	}
}

func TestTransport_OnlyOneEndpointCalled(t *testing.T) {
	var calls int
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	fields := deriveTestFields(t)
	tr, err := NewTransport(Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true})
	resp, err := tr.Stream(context.Background(), StreamRequest{
		RuntimeFields: fields, RequestBody: []byte(`{}`),
		RequestID: "r", CosyVersion: "1.1.34",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
	for {
		if _, err := resp.Parser.Parse(); err != nil {
			break
		}
	}
	mu.Lock()
	n := calls
	mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 call, got %d", n)
	}
}

func deriveTestFields(t *testing.T) RuntimeFields {
	t.Helper()
	entropy := &fixedEntropy{data: make([]byte, 256)}
	for i := range entropy.data {
		entropy.data[i] = byte(i % 251)
	}
	fields, err := DeriveRuntimeFields(entropy, RuntimeFieldInput{
		UID: "test-uid", OrganizationTags: []string{}, DataPolicyAgreed: true,
	})
	if err != nil {
		t.Fatalf("DeriveRuntimeFields: %v", err)
	}
	return fields
}
