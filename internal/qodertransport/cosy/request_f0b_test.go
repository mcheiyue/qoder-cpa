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

// F0b: freeze the current default wire as a sanitized fixture, side by side
// with the QoderWork capture contract (Orchids 21936fc5/698e8c63/ebb98ebb).
// "current" values are asserted as a drift guard; "qoder_work" targets are
// recorded only and ship only after F2b qualification proves them on the wire.

func f0bInput() BuildRequestInput {
	in := f3WireInput()
	in.ModelSource = "system"
	in.CosyVersion = "1.1.34"
	return in
}

func f0bDecodeBody(t *testing.T) map[string]interface{} {
	t.Helper()
	raw, err := BuildChatBody(f0bInput())
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return body
}

// TestF0bBodyFixture pins the default request body field for field. Absent
// keys are asserted absent (want=nil): the capture shows the QoderWork client
// sends some of them, the current CLI wire must not until F3b says so.
func TestF0bBodyFixture(t *testing.T) {
	body := f0bDecodeBody(t)
	beginAt := f0bInput().BeginAt.UnixMilli()

	for _, tc := range []struct {
		name      string
		want      interface{}
		qoderWork string
		evidence  string
	}{
		{"request_id", "req-1", "attempt id (same rule)", "21936fc5"},
		{"request_set_id", "req-1", "independent task UUID", "21936fc5"},
		{"chat_record_id", "req-1", "attempt id (same rule)", "21936fc5"},
		{"session_id", "sess-1", "derived per account+conversation", "ebb98ebb"},
		{"stream", true, "true (same)", ""},
		{"chat_task", "FREE_INPUT", "FREE_INPUT (same)", ""},
		{"is_reply", true, "true (same)", ""},
		{"is_retry", false, "false always", "ebb98ebb"},
		{"source", float64(1), "1 (same)", ""},
		{"version", "3", "3 (same)", ""},
		{"agent_id", "agent_common", "agent_common (same)", ""},
		{"task_id", "common", "common (same)", ""},
		{"session_type", "qodercli", "qoder_work", "21936fc5"},
		{"aliyun_user_type", nil, "present but empty string", "21936fc5"},
		{"image_urls", nil, "absent (same)", "21936fc5"},
		{"code_language", nil, "absent (same)", "21936fc5"},
		{"chat_prompt", nil, "absent (same)", "21936fc5"},
		{"custom_model", nil, "absent (same)", "21936fc5"},
	} {
		got, present := body[tc.name]
		if tc.want == nil {
			if present {
				t.Errorf("%s = %#v, want absent (qoder_work: %s @%s)", tc.name, got, tc.qoderWork, tc.evidence)
			}
			continue
		}
		if !present || got != tc.want {
			t.Errorf("%s = %#v (present=%v), want %#v (qoder_work: %s @%s)",
				tc.name, got, present, tc.want, tc.qoderWork, tc.evidence)
		}
	}

	biz, ok := body["business"].(map[string]interface{})
	if !ok {
		t.Fatalf("business missing or not an object: %#v", body["business"])
	}
	// chat_context is an object (never comparable with ==): current wire sends
	// the empty object; the capture shows {text, extra.originalContent, ...}
	// with plain strings (21936fc5).
	if ctx, present := body["chat_context"].(map[string]interface{}); !present || len(ctx) != 0 {
		t.Errorf("chat_context = %#v, want empty object (qoder_work: text/extra plain strings @21936fc5)", body["chat_context"])
	}
	for _, tc := range []struct {
		name, qoderWork, evidence string
		want                      interface{}
	}{
		{"product", "qoder_work", "21936fc5", "cli"},
		{"version", "client version 1.0.45", "21936fc5", "1.1.34"},
		{"type", "agent (same)", "", "agent"},
		{"id", "request_set_id (task id, not attempt id)", "21936fc5", "req-1"},
		{"name", "prompt head <=10 runes (empty in this fixture)", "same rule", ""},
		{"begin_at", "epoch millis (same rule)", "", float64(beginAt)},
		{"stage", "start (same)", "", "start"},
		{"sub_task", "ws_builtin_general", "21936fc5", nil},
	} {
		got, present := biz[tc.name]
		if tc.want == nil {
			if present {
				t.Errorf("business.%s = %#v, want absent (qoder_work: %s @%s)", tc.name, got, tc.qoderWork, tc.evidence)
			}
			continue
		}
		if !present || got != tc.want {
			t.Errorf("business.%s = %#v (present=%v), want %#v (qoder_work: %s @%s)",
				tc.name, got, present, tc.want, tc.qoderWork, tc.evidence)
		}
	}

	mc, ok := body["model_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("model_config missing: %#v", body["model_config"])
	}
	if mc["key"] != "qfmodel" || mc["format"] != "openai" || mc["source"] != "system" || mc["enable"] != true {
		t.Errorf("model_config = %#v", mc)
	}
	// qoder_work reports the model's own catalog capability, not the caller flag.
	if mc["is_reasoning"] != false {
		t.Errorf("is_reasoning = %#v, current wire carries the caller flag (qoder_work: model catalog @21936fc5)", mc["is_reasoning"])
	}
}

type f0bHeaderProbe struct {
	mu sync.Mutex
	h  http.Header
}

func f0bServer(t *testing.T) (*httptest.Server, *f0bHeaderProbe) {
	t.Helper()
	probe := &f0bHeaderProbe{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		probe.mu.Lock()
		probe.h = r.Header.Clone()
		probe.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, probe
}

func f0bStream(t *testing.T, ts *httptest.Server) {
	t.Helper()
	tr, err := NewTransport(Config{
		HTTPClient: ts.Client(), Endpoint: EndpointAPI2, BaseURL: ts.URL, AllowTestEndpoint: true,
		MachineID: "machine-1", UserID: "user-1", OrganizationID: "org-1",
	})
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	body, err := BuildChatBody(f0bInput())
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := tr.Stream(ctx, StreamRequest{
		RuntimeFields: deriveTestFields(t), RequestBody: body, RequestID: "req-1",
		CosyVersion: "1.1.34", ModelKey: "qfmodel", ModelSource: "system",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	resp.Cancel()
}

// TestF0bHeaderFixture pins the default outbound headers. Dynamic signature
// headers are only checked for presence; values never appear in the fixture.
func TestF0bHeaderFixture(t *testing.T) {
	ts, probe := f0bServer(t)
	f0bStream(t, ts)
	probe.mu.Lock()
	h := probe.h
	probe.mu.Unlock()
	if h == nil {
		t.Fatal("server saw no request")
	}

	for _, tc := range []struct {
		name, want, qoderWork, evidence string
	}{
		{"Cosy-Business-Product", "cli", "qoder_work", "21936fc5"},
		{"Cosy-Business-Type", "agent", "agent (same)", ""},
		{"Cosy-ClientType", "5", "6", "21936fc5"},
		{"Cosy-Data-Policy", "agree", "agree (same)", ""},
		{"Cosy-Scene", "assistant", "qwork", "21936fc5"},
		{"Cosy-Version", "1.1.34", "client version 1.0.45", "21936fc5"},
		{"Cosy-MachineId", "machine-1", "machine-1 (same)", "698e8c63"},
		{"Cosy-MachineToken", "machine-1", "machine-1 (same)", "698e8c63"},
		{"Cosy-MachineType", "5", "5 (same, decoupled from ClientType)", "21936fc5"},
		{"Login-Version", "v2", "v2 (same)", ""},
		{"X-Model-Key", "qfmodel", "qfmodel (same)", ""},
		{"X-Model-Source", "system", "system (same)", ""},
		{"Cosy-MachineOS", "", "x86_64_win32", "21936fc5"},
		{"User-Agent", "", "node", "21936fc5"},
		{"Traceparent", "", "present", "b4ebddd8"},
		{"Accept-Language", "", "present", "b4ebddd8"},
		{"Sec-Fetch-Mode", "", "present", "b4ebddd8"},
	} {
		got := h.Get(tc.name)
		if tc.want == "" {
			if got != "" && tc.name == "User-Agent" && strings.HasPrefix(got, "Go-http-client") {
				continue // Go default UA is expected while qoder_work wants "node".
			}
			if got != "" && (tc.name == "User-Agent" || tc.name == "Cosy-MachineOS" ||
				tc.name == "Traceparent" || tc.name == "Accept-Language" || tc.name == "Sec-Fetch-Mode") {
				t.Errorf("%s = %q, want absent on the CLI wire (qoder_work: %s @%s)", tc.name, got, tc.qoderWork, tc.evidence)
			}
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q (qoder_work: %s @%s)", tc.name, got, tc.want, tc.qoderWork, tc.evidence)
		}
	}
	// Go's transport supplies the default UA; assert it so an explicit UA
	// switch is a deliberate fixture edit rather than silent drift.
	if ua := h.Get("User-Agent"); !strings.HasPrefix(ua, "Go-http-client") {
		t.Errorf("User-Agent = %q, want Go default (qoder_work: node @21936fc5)", ua)
	}
	for _, dyn := range []string{"Authorization", "Cosy-Key", "Cosy-Date"} {
		if h.Get(dyn) == "" {
			t.Errorf("%s absent or empty", dyn)
		}
	}
}
