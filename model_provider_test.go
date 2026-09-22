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
	if result.Models[0].ID != "qoder/model-a" || result.Models[1].ID != "qoder/model-b" || result.Models[2].ID != "qoder/model-c" {
		t.Fatalf("model ids=%q %q %q", result.Models[0].ID, result.Models[1].ID, result.Models[2].ID)
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
