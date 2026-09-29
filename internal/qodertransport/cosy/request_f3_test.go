package cosy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func f3WireInput() BuildRequestInput {
	return BuildRequestInput{
		RequestID: "req-1",
		SessionID: "sess-1",
		ModelKey:  "qfmodel",
		BeginAt:   time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
		Messages:  []ChatMessageIn{{Role: "user", Content: "hi"}},
	}
}

// F3 default guard: business.product must stay "cli" byte-identical to
// current production when no override is requested.
func TestBuildChatBodyDefaultProductIsCLI(t *testing.T) {
	raw, err := BuildChatBody(f3WireInput())
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Business struct {
			Product string `json:"product"`
		} `json:"business"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.Business.Product != "cli" {
		t.Fatalf("business.product=%q, want cli", body.Business.Product)
	}
}

// F3 override: BusinessProduct reaches the signed body as ide.
func TestBuildChatBodyBusinessProductOverride(t *testing.T) {
	in := f3WireInput()
	in.BusinessProduct = "ide"
	raw, err := BuildChatBody(in)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Business struct {
			Product string `json:"product"`
		} `json:"business"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.Business.Product != "ide" {
		t.Fatalf("business.product=%q, want ide", body.Business.Product)
	}
	// The product lives inside the signed encoded body.
	decoded := DecodeBody(EncodeBody(raw))
	if !strings.Contains(string(decoded), "ide") {
		t.Fatalf("product not inside signed body: %s", decoded)
	}
}

type f3HeaderProbe struct {
	mu      sync.Mutex
	product string
}

// F3 transport header default: Cosy-Business-Product stays cli.
func TestStreamHeaderDefaultProductIsCLI(t *testing.T) {
	probe := &f3HeaderProbe{}
	ts := f3FakeServer(t, probe)
	tr := f3TestTransport(t, ts)
	resp, err := tr.Stream(f3Ctx(t), f3StreamRequest(t, "r-default", ""))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
	if probe.product != "cli" {
		t.Fatalf("header Cosy-Business-Product=%q, want cli", probe.product)
	}
}

// F3 transport header override: Cosy-Business-Product flips to ide.
func TestStreamHeaderBusinessProductOverride(t *testing.T) {
	probe := &f3HeaderProbe{}
	ts := f3FakeServer(t, probe)
	tr := f3TestTransport(t, ts)
	resp, err := tr.Stream(f3Ctx(t), f3StreamRequest(t, "r-ide", "ide"))
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer resp.Cancel()
	if probe.product != "ide" {
		t.Fatalf("header Cosy-Business-Product=%q, want ide", probe.product)
	}
}

// F3 agreement guard: one input drives body and header; the wire must never
// carry business.product=ide with header cli (or vice versa).
func TestBusinessProductBodyAndHeaderAgree(t *testing.T) {
	for _, product := range []string{"", "cli", "ide"} {
		probe := &f3HeaderProbe{}
		ts := f3FakeServer(t, probe)
		tr := f3TestTransport(t, ts)
		in := f3WireInput()
		in.BusinessProduct = product
		raw, err := BuildChatBody(in)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := tr.Stream(f3Ctx(t), f3StreamRequest(t, "r-agree", product))
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		var body struct {
			Business struct {
				Product string `json:"product"`
			} `json:"business"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		resp.Cancel()
		want := product
		if want == "" {
			want = "cli"
		}
		if body.Business.Product != want || probe.product != want {
			t.Fatalf("product=%q: body=%q header=%q, both want %q",
				product, body.Business.Product, probe.product, want)
		}
	}
}

func f3FakeServer(t *testing.T, probe *f3HeaderProbe) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		probe.mu.Lock()
		probe.product = r.Header.Get("Cosy-Business-Product")
		probe.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func f3TestTransport(t *testing.T, ts *httptest.Server) *Transport {
	t.Helper()
	cfg := Config{HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true,
		UserID: "user-1", OrganizationID: "org-1", OrganizationTags: []string{"a", "b"}}
	tr, err := NewTransport(cfg)
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return tr
}

func f3Ctx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func f3StreamRequest(t *testing.T, id, product string) StreamRequest {
	return StreamRequest{
		RuntimeFields:   deriveTestFields(t),
		RequestBody:     []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
		RequestID:       id,
		CosyVersion:     "1.1.34",
		ModelKey:        "qfmodel",
		ModelSource:     "system",
		BusinessProduct: product,
	}
}
