package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
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

func TestManagementAccountsIncludesQuotaSnapshot(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	resetAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	syncedAt := time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)
	defaultManagementService = &managementService{
		hostCall: func(method string, payload any) (json.RawMessage, error) {
			switch method {
			case pluginabi.MethodHostAuthList:
				return json.Marshal(struct {
					Files []pluginapi.HostAuthFileEntry `json:"files"`
				}{Files: []pluginapi.HostAuthFileEntry{{AuthIndex: "qoder-1", Name: "qoder.json", Provider: "qoder"}}})
			case pluginabi.MethodHostAuthGet:
				return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", JSON: mustJSON(qoderauth.StorageJSON{AccessToken: "secret", UserID: "u1", MachineID: "m1", Profile: qoderauth.TransportProfileBearerOpenAI})})
			default:
				t.Fatalf("unexpected host method=%q payload=%v", method, payload)
				return nil, nil
			}
		},
		fetchQuota: func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error) {
			return &qodercontrol.Quota{PlanTier: "Pro", Remaining: 12, Limit: 100, Used: 88, Unit: "credits", ResetAt: resetAt, SyncedAt: syncedAt}, nil
		},
	}
	response, err := (managementHandler{kind: "accounts"}).HandleManagement(context.Background(), pluginapi.ManagementRequest{})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var body managementAccountsResponse
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Accounts) != 1 || body.Accounts[0].PlanTier != "Pro" || body.Accounts[0].Remaining != 12 || body.Accounts[0].ResetAt == nil {
		t.Fatalf("accounts=%+v", body.Accounts)
	}
	if containsAny(string(response.Body), "access_token", "refresh_token", "runtime_key") {
		t.Fatalf("secret leaked in response=%s", response.Body)
	}
}

func TestManagementQuotaRefreshReturnsSnapshotAndErrorState(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	defaultManagementService = &managementService{
		hostCall: func(method string, payload any) (json.RawMessage, error) {
			if method != pluginabi.MethodHostAuthGet {
				t.Fatalf("unexpected host method=%q payload=%v", method, payload)
			}
			return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", JSON: mustJSON(qoderauth.StorageJSON{AccessToken: "secret", UserID: "u1"})})
		},
		fetchQuota: func(_ context.Context, cred qoderauth.Credential) (*qodercontrol.Quota, error) {
			if cred.AccessToken != "secret" {
				t.Fatalf("credential=%+v", cred)
			}
			return &qodercontrol.Quota{PlanTier: "Free", Remaining: 0, Limit: 10, Exhausted: true, Error: "upstream HTTP 429", SyncedAt: time.Now().UTC()}, qodercontrol.ErrQuotaUnknown
		},
	}
	response, err := (managementHandler{kind: "quota-refresh"}).HandleManagement(context.Background(), pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1"}`)})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var body managementQuotaResponse
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.AuthIndex != "qoder-1" || !body.Exhausted || body.QuotaError != "upstream HTTP 429" {
		t.Fatalf("quota response=%+v", body)
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
