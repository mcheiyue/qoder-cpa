package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestManagementAccountsRedactsCredentialMaterial(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	defaultManagementService = &managementService{hostCall: func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostAuthList:
			return json.Marshal(struct {
				Files []pluginapi.HostAuthFileEntry `json:"files"`
			}{Files: []pluginapi.HostAuthFileEntry{{
				AuthIndex: "qoder-1", Name: "qoder.json", Provider: "qoder", Label: "Qoder", Email: "a@qoder.test",
			}}})
		case pluginabi.MethodHostAuthGet:
			return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", JSON: mustJSON(qoderauth.StorageJSON{UserID: "user", AccessToken: "redacted", Profile: qoderauth.TransportProfileBearerOpenAI, MachineID: "m-test"})})
		default:
			t.Fatalf("method=%q payload=%v", method, payload)
			return nil, nil
		}
	}}
	response, err := (managementHandler{kind: "accounts"}).HandleManagement(nil, pluginapi.ManagementRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(response.Body) == "" {
		t.Fatalf("response=%+v", response)
	}
	if string(response.Body) != `{"accounts":[{"auth_index":"qoder-1","name":"qoder.json","label":"Qoder","email":"a@qoder.test","transport_profile":"bearer-openai"}]}` {
		t.Fatalf("body=%s", response.Body)
	}
	for _, secret := range []string{"access_token", "refresh_token", "runtime_key"} {
		if string(response.Body) == secret {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
}

func TestManagementProfileUpdatePreservesSecretsAndSavesOnce(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	saveCalls := 0
	defaultManagementService = &managementService{hostCall: func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostAuthGet:
			return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", Name: "qoder.json", JSON: mustJSON(qoderauth.StorageJSON{
				AccessToken: "access", RefreshToken: "refresh", UserID: "user", Profile: qoderauth.TransportProfileCosyAPI2,
			})})
		case pluginabi.MethodHostAuthSave:
			saveCalls++
			request := payload.(pluginapi.HostAuthSaveRequest)
			var storage qoderauth.StorageJSON
			if err := json.Unmarshal(request.JSON, &storage); err != nil {
				t.Fatal(err)
			}
			if storage.AccessToken != "access" || storage.RefreshToken != "refresh" || storage.Profile != qoderauth.TransportProfileBearerOpenAI {
				t.Fatalf("storage=%+v", storage)
			}
			return json.RawMessage(`{}`), nil
		default:
			t.Fatalf("method=%q", method)
			return nil, nil
		}
	}}
	response, err := (managementHandler{kind: "profile"}).HandleManagement(nil, pluginapi.ManagementRequest{
		Body: []byte(`{"auth_index":"qoder-1","transport_profile":"bearer-openai"}`),
	})
	if err != nil || response.StatusCode != http.StatusOK || saveCalls != 1 {
		t.Fatalf("response=%+v err=%v save_calls=%d", response, err, saveCalls)
	}
}

func TestManagementProfileRejectsInvalidProfileWithoutHostCall(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	called := false
	defaultManagementService = &managementService{hostCall: func(string, any) (json.RawMessage, error) {
		called = true
		return nil, nil
	}}
	response, err := (managementHandler{kind: "profile"}).HandleManagement(nil, pluginapi.ManagementRequest{
		Body: []byte(`{"auth_index":"qoder-1","transport_profile":"unknown"}`),
	})
	if err != nil || response.StatusCode != http.StatusBadRequest || called {
		t.Fatalf("response=%+v err=%v called=%v", response, err, called)
	}
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
