package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestModelProviderABIUsesDynamicCatalog(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService.controlConfig = qodercontrol.Config{BaseURL: "https://qoder.test", AllowedHosts: []string{"qoder.test"}}
	hostJSONCall = func(_ string, _ any) (json.RawMessage, error) {
		return json.Marshal(pluginapi.HTTPResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"data":[{"id":"model-a","name":"Model A"},{"id":"model-a","name":"duplicate"},{"id":"qoder/model-b","name":"Model B"},{"id":"model-c"},{"id":"","name":"empty"}]}`),
		})
	}
	data := modelTestAuth(t)
	raw, _ := json.Marshal(rpcAuthModelRequest{
		AuthModelRequest: pluginapi.AuthModelRequest{AuthProvider: "qoder", StorageJSON: data.StorageJSON},
		HostCallbackID:   "cb-models",
	})
	envelope, err := handleMethod(pluginabi.MethodModelForAuth, raw)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[pluginapi.ModelResponse](t, envelope)
	if result.Provider != "qoder" || len(result.Models) != 3 {
		t.Fatalf("models=%#v", result)
	}
	if result.Models[0].ID != "qoder/Model A" || result.Models[1].ID != "qoder/Model B" || result.Models[2].ID != "qoder/model-c" {
		t.Fatalf("model ids=%q %q %q", result.Models[0].ID, result.Models[1].ID, result.Models[2].ID)
	}
	authID := string(qoderauth.AuthIDForUser("user"))
	if got := defaultModelRegistry.resolve(authID, "qoder/Model A"); got != "model-a" {
		t.Fatalf("model mapping=%q", got)
	}
	if result.Models[2].DisplayName != "model-c" {
		t.Fatalf("fallback display name=%q, want model-c", result.Models[2].DisplayName)
	}
	staticEnvelope, _ := handleMethod(pluginabi.MethodModelStatic, nil)
	static := decodeResult[pluginapi.ModelResponse](t, staticEnvelope)
	if static.Provider != "qoder" || len(static.Models) != 0 {
		t.Fatalf("static models=%#v", static)
	}
}

func TestModelProviderCatalogMetadataSetsInputTokenLimitAndThinking(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService.controlConfig = qodercontrol.Config{BaseURL: "https://qoder.test", AllowedHosts: []string{"qoder.test"}}
	hostJSONCall = func(_ string, _ any) (json.RawMessage, error) {
		return json.Marshal(pluginapi.HTTPResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"data":[{"key":"r1","name":"R1-Reasoning","is_reasoning":true,"max_input_tokens":131072},{"key":"p1","name":"P1-Plain"}]}`),
		})
	}
	data := modelTestAuth(t)
	raw, _ := json.Marshal(rpcAuthModelRequest{
		AuthModelRequest: pluginapi.AuthModelRequest{AuthProvider: "qoder", StorageJSON: data.StorageJSON},
		HostCallbackID:   "cb-meta",
	})
	envelope, err := handleMethod(pluginabi.MethodModelForAuth, raw)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[pluginapi.ModelResponse](t, envelope)
	if len(result.Models) != 2 {
		t.Fatalf("models=%#v", result.Models)
	}
	var reasoning, plain pluginapi.ModelInfo
	for _, m := range result.Models {
		if m.ID == "qoder/R1-Reasoning" {
			reasoning = m
		} else {
			plain = m
		}
	}
	if reasoning.InputTokenLimit != 131072 {
		t.Errorf("reasoning InputTokenLimit=%d, want 131072", reasoning.InputTokenLimit)
	}
	if reasoning.Thinking == nil || !reasoning.Thinking.ZeroAllowed {
		t.Errorf("reasoning Thinking=%+v, want ZeroAllowed=true", reasoning.Thinking)
	}
	if plain.InputTokenLimit != 0 {
		t.Errorf("plain InputTokenLimit=%d, want 0", plain.InputTokenLimit)
	}
	if plain.Thinking != nil {
		t.Errorf("plain Thinking=%+v, want nil", plain.Thinking)
	}
}

func TestModelProviderCatalogMetadataStoredPerAuthID(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService.controlConfig = qodercontrol.Config{BaseURL: "https://qoder.test", AllowedHosts: []string{"qoder.test"}}
	hostJSONCall = func(_ string, _ any) (json.RawMessage, error) {
		return json.Marshal(pluginapi.HTTPResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"data":[{"key":"r1","name":"R1-Reasoning","is_reasoning":true,"max_input_tokens":64000}]}`),
		})
	}
	data := modelTestAuth(t)
	authID := "auth-meta-check"
	raw, _ := json.Marshal(rpcAuthModelRequest{
		AuthModelRequest: pluginapi.AuthModelRequest{AuthID: authID, AuthProvider: "qoder", StorageJSON: data.StorageJSON},
		HostCallbackID:   "cb-meta2",
	})
	_, err := handleMethod(pluginabi.MethodModelForAuth, raw)
	if err != nil {
		t.Fatal(err)
	}
	publicID := "qoder/R1-Reasoning"
	if got := defaultModelRegistry.resolve(authID, publicID); got != "r1" {
		t.Fatalf("internal ID=%q, want r1", got)
	}
	meta, ok := defaultModelRegistry.resolveMeta(authID, publicID)
	if !ok {
		t.Fatal("metadata not stored")
	}
	if !meta.IsReasoning || meta.MaxInputTokens != 64000 {
		t.Fatalf("metadata=%+v", meta)
	}
}

func TestModelProviderUsesUpstreamDisplayNameAndRegistersReverseMapping(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService.controlConfig = qodercontrol.Config{BaseURL: "https://qoder.test", AllowedHosts: []string{"qoder.test"}}
	hostJSONCall = func(_ string, _ any) (json.RawMessage, error) {
		return json.Marshal(pluginapi.HTTPResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"data":[{"id":"qfmodel","name":"qfmodel","display_name":"Qwen3.8-Flash"},{"id":"qnewmodel","display_name":"Qoder New"}]}`),
		})
	}
	data := modelTestAuth(t)
	authID := "auth-dynamic"
	raw, _ := json.Marshal(rpcAuthModelRequest{
		AuthModelRequest: pluginapi.AuthModelRequest{AuthID: authID, AuthProvider: "qoder", StorageJSON: data.StorageJSON},
		HostCallbackID:   "cb-models",
	})
	envelope, err := handleMethod(pluginabi.MethodModelForAuth, raw)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[pluginapi.ModelResponse](t, envelope)
	if len(result.Models) != 2 {
		t.Fatalf("models=%#v", result.Models)
	}
	byID := make(map[string]pluginapi.ModelInfo, len(result.Models))
	for _, model := range result.Models {
		byID[model.ID] = model
	}
	if byID["qoder/Qwen3.8-Flash"].ID == "" || byID["qoder/Qoder New"].ID == "" {
		t.Fatalf("models=%#v", result.Models)
	}
	if byID["qoder/Qwen3.8-Flash"].Name != "qfmodel" || byID["qoder/Qwen3.8-Flash"].DisplayName != "Qwen3.8-Flash" {
		t.Fatalf("qfmodel metadata=%#v", byID["qoder/Qwen3.8-Flash"])
	}
	if got := defaultModelRegistry.resolve(authID, "qoder/Qwen3.8-Flash"); got != "qfmodel" {
		t.Fatalf("qfmodel mapping=%q", got)
	}
	if got := defaultModelRegistry.resolve(authID, "qoder/Qoder New"); got != "qnewmodel" {
		t.Fatalf("new model mapping=%q", got)
	}
}

func TestModelProviderFailureDoesNotFabricateModels(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService.controlConfig = qodercontrol.Config{BaseURL: "https://qoder.test", AllowedHosts: []string{"qoder.test"}}
	hostJSONCall = func(_ string, _ any) (json.RawMessage, error) {
		return json.Marshal(pluginapi.HTTPResponse{StatusCode: http.StatusServiceUnavailable})
	}
	data := modelTestAuth(t)
	raw, _ := json.Marshal(rpcAuthModelRequest{
		AuthModelRequest: pluginapi.AuthModelRequest{AuthProvider: "qoder", StorageJSON: data.StorageJSON},
		HostCallbackID:   "cb-models",
	})
	envelope, _ := handleMethod(pluginabi.MethodModelForAuth, raw)
	var result testEnvelope[pluginapi.ModelResponse]
	if err := json.Unmarshal(envelope, &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error == nil || result.Error.Code != "model_error" {
		t.Fatalf("envelope=%s", envelope)
	}
}

func TestDisplayNameForModelUsesUpstreamName(t *testing.T) {
	if got := displayNameForModel("qfmodel", "Qwen3.8-Flash"); got != "Qwen3.8-Flash" {
		t.Fatalf("display name=%q", got)
	}
	if got := displayNameForModel("qfmodel", "qfmodel"); got != "qfmodel" {
		t.Fatalf("missing upstream display name=%q", got)
	}
	if got := displayNameForModel("new-model", "New Model"); got != "New Model" {
		t.Fatalf("upstream display name=%q", got)
	}
	if got := displayNameForModel("new-model", ""); got != "new-model" {
		t.Fatalf("fallback display name=%q", got)
	}
}

func TestPublicModelIDUsesProvidedDisplayName(t *testing.T) {
	if got := publicModelID("Qwen3.8-Flash"); got != "qoder/Qwen3.8-Flash" {
		t.Fatalf("public model id=%q", got)
	}
	if got := publicModelID("new-model"); got != "qoder/new-model" {
		t.Fatalf("public model id=%q", got)
	}
}

func TestUniquePublicModelIDAvoidsDisplayNameCollision(t *testing.T) {
	mapping := map[string]string{}
	first := uniquePublicModelID("Same Name", "model-a", mapping)
	mapping[first] = "model-a"
	second := uniquePublicModelID("Same Name", "model-b", mapping)
	if first != "qoder/Same Name" || second != "qoder/model-b" {
		t.Fatalf("public ids=%q %q", first, second)
	}
}

func modelTestAuth(t *testing.T) pluginapi.AuthData {
	t.Helper()
	credential := qoderauth.Credential{
		AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour),
		UserID: "user", Profile: qoderauth.TransportProfileCosyAPI2,
	}
	data, err := authData(credential, "")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
