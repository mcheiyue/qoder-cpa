package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// TestAccountsExposePriority accounts 响应必须带宿主 priority（wire 不带
// omitempty，0 必回显）；name 已承载宿主文件名，供 WebUI 直接
// PATCH /v0/management/auth-files/fields 定位。语义：数值越大越优先
// （宿主 conductor 取最高层）。
func TestAccountsExposePriority(t *testing.T) {
	filesJSON := json.RawMessage(`{"files":[
		{"auth_index":"p1","provider":"qoder","label":"global1","name":"qoder-p1.json","id":"qoder-p1.json","priority":10},
		{"auth_index":"p2","provider":"qoder","label":"cn1","name":"qoder-p2.json","id":"qoder-p2.json"}]}`)
	svc := &managementService{
		hostCall: func(method string, payload any) (json.RawMessage, error) {
			switch method {
			case pluginabi.MethodHostAuthList:
				return filesJSON, nil
			case pluginabi.MethodHostAuthGet:
				req, _ := payload.(pluginapi.HostAuthGetRequest)
				raw, mErr := json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: req.AuthIndex, JSON: []byte(`{}`)})
				return raw, mErr
			default:
				return nil, errors.New("unexpected method " + method)
			}
		},
	}
	resp, err := svc.accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var out managementAccountsResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Accounts) != 2 {
		t.Fatalf("accounts=%d want 2: %s", len(out.Accounts), resp.Body)
	}
	byIdx := map[string]managementAccount{}
	for _, a := range out.Accounts {
		byIdx[a.AuthIndex] = a
	}
	if byIdx["p1"].Priority != 10 {
		t.Errorf("p1.Priority=%d want 10", byIdx["p1"].Priority)
	}
	if byIdx["p1"].Name != "qoder-p1.json" {
		t.Errorf("p1.Name=%q want qoder-p1.json (PATCH name target)", byIdx["p1"].Name)
	}
	if byIdx["p2"].Priority != 0 {
		t.Errorf("p2.Priority=%d want 0", byIdx["p2"].Priority)
	}
	if !strings.Contains(string(resp.Body), `"priority":0`) {
		t.Errorf("wire missing priority:0 (omitempty would swallow zero): %s", resp.Body)
	}
}
