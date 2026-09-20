package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestHandleMethodUnknownReturnsTypedError(t *testing.T) {
	// Given: an unknown method name.
	method := "nonexistent.method"

	// When: handleMethod dispatches it.
	raw, err := handleMethod(method, nil)
	if err != nil {
		t.Fatalf("handleMethod returned error: %v", err)
	}

	// Then: an error envelope with code "unknown_method".
	var env struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.OK {
		t.Fatal("expected ok=false for unknown method")
	}
	if env.Error == nil {
		t.Fatal("expected error in envelope")
	}
	if env.Error.Code != "unknown_method" {
		t.Fatalf("code=%q, want %q", env.Error.Code, "unknown_method")
	}
	if !strings.Contains(env.Error.Message, method) {
		t.Fatalf("message=%q should contain %q", env.Error.Message, method)
	}
}

func TestHandleMethodEmptyMethod(t *testing.T) {
	// Given: an empty string method.
	// When: handleMethod dispatches it.
	// Then: it must return unknown_method error, not panic.
	raw, err := handleMethod("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.OK {
		t.Fatal("expected ok=false for empty method")
	}
	if env.Error == nil || env.Error.Code != "unknown_method" {
		t.Fatalf("expected unknown_method error, got: %v", env.Error)
	}
}

func TestHandleMethodMalformedJSON(t *testing.T) {
	// Given: a register method with malformed JSON body.
	// When: handleMethod processes it.
	// Then: it still returns ok (registration ignores body), not a parse error.
	raw, err := handleMethod("plugin.register", []byte("{invalid json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatal("plugin.register should succeed even with malformed body")
	}
}

func TestHandleMethodRegisterReturnsSchema6(t *testing.T) {
	// Given: the plugin.register method.
	// When: handleMethod processes it.
	raw, err := handleMethod("plugin.register", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Then: the result must contain schema_version 6.
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatal("expected ok=true for plugin.register")
	}
	var reg struct {
		SchemaVersion uint32 `json:"schema_version"`
	}
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatal(err)
	}
	if reg.SchemaVersion != 6 {
		t.Fatalf("schema=%d, want 6", reg.SchemaVersion)
	}
}

func TestManagementRegisterReturnsSerializableRoutesAndResource(t *testing.T) {
	raw, err := handleMethod(pluginabi.MethodManagementRegister, nil)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("envelope=%s", raw)
	}
	var result struct {
		Routes    []struct{ Method, Path string } `json:"routes"`
		Resources []struct{ Path, Menu string }   `json:"resources"`
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Routes) != 2 || result.Routes[0].Path == "" || len(result.Resources) != 1 || result.Resources[0].Path != "/index.html" {
		t.Fatalf("registration=%s", envelope.Result)
	}
}

func TestManagementHandleAcceptsCPAFullPaths(t *testing.T) {
	original := defaultManagementService
	defaultManagementService = &managementService{hostCall: func(method string, _ any) (json.RawMessage, error) {
		if method != pluginabi.MethodHostAuthList {
			t.Fatalf("host method = %q", method)
		}
		return json.RawMessage(`{"files":[]}`), nil
	}}
	defer func() { defaultManagementService = original }()

	for _, path := range []string{
		"/v0/management/qoder/accounts",
		"/qoder/accounts",
	} {
		rawRequest, err := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: path})
		if err != nil {
			t.Fatal(err)
		}
		rawResponse, err := handleMethod(pluginabi.MethodManagementHandle, rawRequest)
		if err != nil {
			t.Fatal(err)
		}
		response := decodeManagementResponse(t, rawResponse)
		if !strings.Contains(string(response.Body), `"accounts":[]`) {
			t.Fatalf("path %q body = %s", path, response.Body)
		}
	}
}

func TestManagementHandleAcceptsCPAResourcePath(t *testing.T) {
	rawRequest, err := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/qoder/index.html",
	})
	if err != nil {
		t.Fatal(err)
	}
	rawResponse, err := handleMethod(pluginabi.MethodManagementHandle, rawRequest)
	if err != nil {
		t.Fatal(err)
	}
	response := decodeManagementResponse(t, rawResponse)
	if !strings.Contains(string(response.Body), "Qoder 账号") {
		t.Fatalf("resource body = %s", response.Body)
	}
}

func decodeManagementResponse(t *testing.T, raw []byte) pluginapi.ManagementResponse {
	t.Helper()
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("envelope = %s", raw)
	}
	var response pluginapi.ManagementResponse
	if err := json.Unmarshal(envelope.Result, &response); err != nil {
		t.Fatal(err)
	}
	return response
}
