package main

import (
	"encoding/json"
	"testing"
)

func TestRegistrationSchema6QoderCapabilities(t *testing.T) {
	// Given: registration must roundtrip through JSON like a real ABI call.
	var reg struct {
		SchemaVersion uint32 `json:"schema_version"`
		Metadata      struct {
			Name    string `json:"Name"`
			Version string `json:"Version"`
		} `json:"metadata"`
		Capabilities struct {
			AuthProvider       bool     `json:"auth_provider"`
			ModelProvider      bool     `json:"model_provider"`
			Executor           bool     `json:"executor"`
			ManagementAPI      bool     `json:"management_api"`
			ExecutorModelScope string   `json:"executor_model_scope"`
			InputFormats       []string `json:"executor_input_formats"`
			OutputFormats      []string `json:"executor_output_formats"`
			ModelRouter        *bool    `json:"model_router,omitempty"`
			RequestInterceptor *bool    `json:"request_interceptor,omitempty"`
		} `json:"capabilities"`
	}

	// When: serialize and parse the registration result.
	raw, err := json.Marshal(registration())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatal(err)
	}

	// Then: schema = 6, provider = qoder, no model_router, no request_interceptor.
	if reg.SchemaVersion != 6 {
		t.Fatalf("schema=%d, want 6", reg.SchemaVersion)
	}
	if reg.Metadata.Name != "qoder" {
		t.Fatalf("Name=%q, want %q", reg.Metadata.Name, "qoder")
	}
	if !reg.Capabilities.AuthProvider {
		t.Fatal("auth_provider should be true")
	}
	if !reg.Capabilities.ModelProvider {
		t.Fatal("model_provider should be true")
	}
	if !reg.Capabilities.Executor {
		t.Fatal("executor should be true")
	}
	if !reg.Capabilities.ManagementAPI {
		t.Fatal("management_api should be true")
	}
	if reg.Capabilities.ExecutorModelScope != "oauth" {
		t.Fatalf("executor_model_scope=%q, want %q", reg.Capabilities.ExecutorModelScope, "oauth")
	}
	wantInput := []string{"chat-completions"}
	wantOutput := []string{"chat-completions"}
	if len(reg.Capabilities.InputFormats) != 1 || reg.Capabilities.InputFormats[0] != wantInput[0] {
		t.Fatalf("input_formats=%v, want %v", reg.Capabilities.InputFormats, wantInput)
	}
	if len(reg.Capabilities.OutputFormats) != 1 || reg.Capabilities.OutputFormats[0] != wantOutput[0] {
		t.Fatalf("output_formats=%v, want %v", reg.Capabilities.OutputFormats, wantOutput)
	}
	if reg.Capabilities.ModelRouter != nil {
		t.Fatalf("model_router=%v, want absent/nil", *reg.Capabilities.ModelRouter)
	}
	if reg.Capabilities.RequestInterceptor != nil {
		t.Fatalf("request_interceptor=%v, want absent/nil", *reg.Capabilities.RequestInterceptor)
	}
}

func TestRegistrationSchema5FailsForQoder(t *testing.T) {
	// Given: a registration fixture that claims schema 5.
	fake := map[string]any{
		"schema_version": uint32(5),
		"metadata":       map[string]any{"Name": "qoder"},
		"capabilities":   map[string]any{},
	}

	// When: it must NOT match our schema 6 registration.
	raw, _ := json.Marshal(fake)
	var reg struct {
		SchemaVersion uint32 `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatal(err)
	}

	// Then: schema 5 ≠ 6.
	if reg.SchemaVersion == 6 {
		t.Fatal("schema 5 should not match schema 6")
	}
}

func TestRegistrationCapabilitiesNoUnexpectedKeys(t *testing.T) {
	// Given: the registration map.
	reg := registration()

	// When: serialize and inspect.
	raw, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}

	// Then: the raw JSON must not contain model_router or request_interceptor keys.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	capsJSON, ok := m["capabilities"]
	if !ok {
		t.Fatal("missing capabilities key")
	}
	var caps map[string]json.RawMessage
	if err := json.Unmarshal(capsJSON, &caps); err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"model_router", "request_interceptor"}
	for _, key := range forbidden {
		if _, exists := caps[key]; exists {
			t.Fatalf("capabilities must not contain %q", key)
		}
	}
}
