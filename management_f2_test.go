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

func TestQuotaRefreshPersistsRealUserType(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	var saved json.RawMessage
	defaultManagementService = &managementService{
		hostCall: func(method string, payload any) (json.RawMessage, error) {
			switch method {
			case pluginabi.MethodHostAuthGet:
				return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", Name: "qoder-1.json", JSON: mustJSON(qoderauth.StorageJSON{AccessToken: "secret", UserID: "u1"})})
			case pluginabi.MethodHostAuthSave:
				req, ok := payload.(pluginapi.HostAuthSaveRequest)
				if !ok {
					t.Fatalf("payload type=%T", payload)
				}
				saved = req.JSON
				return nil, nil
			default:
				t.Fatalf("unexpected host method=%q", method)
				return nil, nil
			}
		},
		fetchQuota: func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error) {
			return &qodercontrol.Quota{PlanTier: "Free", Remaining: 3, UserType: "personal_standard", SyncedAt: time.Now().UTC()}, nil
		},
	}
	response, err := (managementHandler{kind: "quota-refresh"}).HandleManagement(context.Background(), pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1"}`)})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if len(saved) == 0 {
		t.Fatal("real UserType from quota must be persisted via host.auth.save")
	}
	if !strings.Contains(string(saved), `"user_type":"personal_standard"`) {
		t.Fatalf("saved JSON missing real user_type: %s", saved)
	}
	if !strings.Contains(string(saved), `"access_token":"secret"`) {
		t.Fatalf("save must round-trip existing fields: %s", saved)
	}
}

func TestQuotaRefreshMissingUserTypeStaysMissing(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	saveCount := 0
	defaultManagementService = &managementService{
		hostCall: func(method string, payload any) (json.RawMessage, error) {
			switch method {
			case pluginabi.MethodHostAuthGet:
				return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", Name: "qoder-1.json", JSON: mustJSON(qoderauth.StorageJSON{AccessToken: "secret", UserID: "u1"})})
			case pluginabi.MethodHostAuthSave:
				saveCount++
				req := payload.(pluginapi.HostAuthSaveRequest)
				if strings.Contains(string(req.JSON), "user_type") {
					t.Fatalf("missing UserType must not be written as a value: %s", req.JSON)
				}
				return nil, nil
			default:
				t.Fatalf("unexpected host method=%q", method)
				return nil, nil
			}
		},
		fetchQuota: func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error) {
			return &qodercontrol.Quota{PlanTier: "Free", Remaining: 3, SyncedAt: time.Now().UTC()}, nil
		},
	}
	response, err := (managementHandler{kind: "quota-refresh"}).HandleManagement(context.Background(), pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1"}`)})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if saveCount != 0 {
		t.Fatalf("no real value means no write (stays missing), saves=%d", saveCount)
	}
}
