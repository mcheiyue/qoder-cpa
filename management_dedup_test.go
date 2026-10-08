package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func dedupTestHostCall(t *testing.T, files []pluginapi.HostAuthFileEntry, getCalls *int) func(string, any) (json.RawMessage, error) {
	t.Helper()
	return func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostAuthList:
			return json.Marshal(struct {
				Files []pluginapi.HostAuthFileEntry `json:"files"`
			}{Files: files})
		case pluginabi.MethodHostAuthGet:
			*getCalls++
			req, ok := payload.(pluginapi.HostAuthGetRequest)
			if !ok {
				t.Fatalf("auth.get payload type=%T", payload)
			}
			return json.Marshal(pluginapi.HostAuthGetResponse{
				AuthIndex: req.AuthIndex,
				JSON: mustJSON(qoderauth.StorageJSON{
					UserID: "u-dedup", AccessToken: "tok", Profile: qoderauth.TransportProfileCosyAPI2,
					MachineID: "m-test", Email: "e@qoder.test",
				}),
			})
		default:
			return nil, errors.New("unexpected method " + method)
		}
	}
}

// TestAccountsHandler_DedupsDuplicateAuthIndex 同一文件被宿主双注册（扫描 AuthParse
// 与保活写回路径各一条、auth_index 相同）时：行按 auth_index 塌缩、兜底标签/空邮箱
// 升级为真实值；且 auth.get 与 fetchQuota 仍按每条记录执行（quota 快照按 file.ID=记录 ID，
// scheduler 的 exhausted 排除依赖每记录一份，重复记录不得跳过）。
func TestAccountsHandler_DedupsDuplicateAuthIndex(t *testing.T) {
	files := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "dup1", Name: "qoder-d1.json", ID: "qoder-d1.json", Provider: "qoder", Label: "qoder"},
		{AuthIndex: "dup1", Name: "qoder-d1.json", ID: "qoder-d1", Provider: "qoder", Label: "真实昵称", Email: "real@qoder.test"},
		{AuthIndex: "uniq1", Name: "qoder-d2.json", ID: "qoder-d2", Provider: "qoder", Label: "uniq", Email: "u@qoder.test"},
		{AuthIndex: "other", Name: "wb-x.json", ID: "wb-x", Provider: "workbuddy", Label: "wb"},
	}
	getCalls, quotaCalls := 0, 0
	svc := &managementService{
		hostCall: dedupTestHostCall(t, files, &getCalls),
		fetchQuota: func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error) {
			quotaCalls++
			return &qodercontrol.Quota{Remaining: 42}, nil
		},
	}
	resp, err := svc.accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, resp.Body)
	}
	var out managementAccountsResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Accounts) != 2 {
		t.Fatalf("rows=%d want 2 (dup collapsed, workbuddy filtered): %s", len(out.Accounts), resp.Body)
	}
	if out.Accounts[0].AuthIndex != "dup1" || out.Accounts[1].AuthIndex != "uniq1" {
		t.Fatalf("rows=%s", resp.Body)
	}
	if out.Accounts[0].Label != "真实昵称" {
		t.Fatalf("dup label=%q want 真实昵称 (fallback upgraded)", out.Accounts[0].Label)
	}
	if out.Accounts[0].Email != "real@qoder.test" {
		t.Fatalf("dup email=%q want real@qoder.test (empty upgraded)", out.Accounts[0].Email)
	}
	if getCalls != 3 {
		t.Fatalf("auth.get=%d want 3 (per-record processing must NOT be skipped by dedup)", getCalls)
	}
	if quotaCalls != 3 {
		t.Fatalf("fetchQuota=%d want 3 (quota snapshot per record ID preserved)", quotaCalls)
	}
}

// TestModelsHandler_DedupsDuplicateAuthIndex 模型目录对重复注册的同一 auth_index
// 只 auth.get + fetchModels 一次（输出本就按 public ID 合并，重复只是浪费上游拉取）。
func TestModelsHandler_DedupsDuplicateAuthIndex(t *testing.T) {
	files := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "dup1", Name: "qoder-d1.json", ID: "qoder-d1.json", Provider: "qoder", Label: "qoder"},
		{AuthIndex: "dup1", Name: "qoder-d1.json", ID: "qoder-d1", Provider: "qoder", Label: "真实昵称"},
	}
	getCalls, fetchCalls := 0, 0
	svc := &managementService{
		hostCall: dedupTestHostCall(t, files, &getCalls),
		fetchModels: func(context.Context, qoderauth.Credential) ([]qodercontrol.Model, error) {
			fetchCalls++
			return []qodercontrol.Model{{ID: "m-ctl-a", Name: "Model A"}}, nil
		},
	}
	resp, err := svc.models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, resp.Body)
	}
	if getCalls != 1 {
		t.Fatalf("auth.get=%d want 1 (duplicate auth_index fetched once)", getCalls)
	}
	if fetchCalls != 1 {
		t.Fatalf("fetchModels=%d want 1 (duplicate must skip upstream fetch)", fetchCalls)
	}
	var out managementModelsResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Models) != 1 {
		t.Fatalf("models=%d want 1: %s", len(out.Models), resp.Body)
	}
}
