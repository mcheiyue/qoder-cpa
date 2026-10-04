package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// (a) registration: scheduler declared; model_router/request_interceptor forbidden.
func TestSchedulerCapabilityDeclared(t *testing.T) {
	raw, err := json.Marshal(registration())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"scheduler":`) {
		t.Fatal("capabilities must contain scheduler")
	}
	for _, forbidden := range []string{"model_router", "request_interceptor"} {
		if strings.Contains(string(raw), `"`+forbidden+`":`) {
			t.Fatalf("capabilities must not contain %q", forbidden)
		}
	}
}

// (b) RPC round-trip through handleMethod dispatch.
func TestHandleSchedulerMethod_RoundTrip(t *testing.T) {
	testSchedulerState(t)
	req := pluginapi.SchedulerPickRequest{
		Provider:   "qoder",
		Providers:  []string{"qoder"},
		Model:      "qwen3.8-plus",
		Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "auth-1", Priority: 10}},
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	out, err := handleMethod(pluginabi.MethodSchedulerPick, raw)
	if err != nil {
		t.Fatalf("handleMethod: %v", err)
	}
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("envelope: %v (%s)", err, out)
	}
	if !env.OK {
		t.Fatalf("expected ok envelope, got %s", out)
	}
	var resp pluginapi.SchedulerPickResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("resp: %v", err)
	}
	if !resp.Handled || resp.AuthID != "auth-1" {
		t.Fatalf("resp = %+v, want handled auth-1", resp)
	}
}
