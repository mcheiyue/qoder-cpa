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
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// 锁：宿主/用量统计（cpamp）从 auth 文件顶层 label/email 取显示名，
// 缺失时回落到文件名（qoder-<64位hex>）。国内站上游 email 恒为空，
// 手机号是唯一可读标识，故 email 位置写 security_mobile。
func TestQuotaRefreshPersistsIdentityLabel(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	var saved json.RawMessage
	defaultManagementService = &managementService{
		hostCall: func(method string, payload any) (json.RawMessage, error) {
			switch method {
			case pluginabi.MethodHostAuthGet:
				return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", Name: "qoder-1.json", JSON: mustJSON(qoderauth.StorageJSON{AccessToken: "secret", UserID: "u1"})})
			case pluginabi.MethodHostAuthSave:
				saved = payload.(pluginapi.HostAuthSaveRequest).JSON
				return nil, nil
			default:
				t.Fatalf("unexpected host method=%q", method)
				return nil, nil
			}
		},
		fetchQuota: func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error) {
			return &qodercontrol.Quota{Nickname: "nick9504777579", Email: "", Mobile: "18074073866", SyncedAt: time.Now().UTC()}, nil
		},
	}
	response, err := (managementHandler{kind: "quota-refresh"}).HandleManagement(context.Background(), pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1"}`)})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if len(saved) == 0 {
		t.Fatal("identity from quota must be persisted so host/cpamp can show it")
	}
	if !strings.Contains(string(saved), `"label":"nick9504777579"`) {
		t.Fatalf("saved JSON missing label: %s", saved)
	}
	if !strings.Contains(string(saved), `"email":"18074073866"`) {
		t.Fatalf("CN account must fall back to security_mobile for email: %s", saved)
	}
	if !strings.Contains(string(saved), `"access_token":"secret"`) {
		t.Fatalf("save must round-trip existing fields: %s", saved)
	}
}

// 锁：用户自定义 label 优先，配额刷新不得覆盖。
func TestQuotaRefreshKeepsUserLabel(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	saveCount := 0
	defaultManagementService = &managementService{
		hostCall: func(method string, payload any) (json.RawMessage, error) {
			switch method {
			case pluginabi.MethodHostAuthGet:
				return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", Name: "qoder-1.json", JSON: mustJSON(qoderauth.StorageJSON{AccessToken: "secret", UserID: "u1", Label: "我的号", Email: "1@x.com"})})
			case pluginabi.MethodHostAuthSave:
				saveCount++
				req := payload.(pluginapi.HostAuthSaveRequest)
				if strings.Contains(string(req.JSON), "nick9504777579") {
					t.Fatalf("user label must not be overwritten: %s", req.JSON)
				}
				return nil, nil
			default:
				t.Fatalf("unexpected host method=%q", method)
				return nil, nil
			}
		},
		fetchQuota: func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error) {
			return &qodercontrol.Quota{Nickname: "nick9504777579", Email: "2@x.com", Mobile: "18074073866", SyncedAt: time.Now().UTC()}, nil
		},
	}
	response, err := (managementHandler{kind: "quota-refresh"}).HandleManagement(context.Background(), pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1"}`)})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if saveCount != 0 {
		t.Fatalf("nothing changed means no write, saves=%d", saveCount)
	}
}
