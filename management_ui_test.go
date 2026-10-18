package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func uiHostCall(files []pluginapi.HostAuthFileEntry, profiles map[string]qoderauth.TransportProfile) func(string, any) (json.RawMessage, error) {
	return func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostAuthList:
			return json.Marshal(struct {
				Files []pluginapi.HostAuthFileEntry `json:"files"`
			}{Files: files})
		case pluginabi.MethodHostAuthGet:
			req, _ := payload.(pluginapi.HostAuthGetRequest)
			for _, f := range files {
				if f.AuthIndex == req.AuthIndex {
					return json.Marshal(pluginapi.HostAuthGetResponse{
						AuthIndex: f.AuthIndex, Name: f.Name,
						JSON: mustJSON(qoderauth.StorageJSON{
							AccessToken: "at-" + f.AuthIndex, UserID: "u-" + f.AuthIndex, Profile: profiles[f.AuthIndex],
						}),
					})
				}
			}
		}
		return nil, nil
	}
}

// 锁：qoder 的 auth 文件没有 email/label 元数据，宿主回落到文件名
// （qoder-<sha256>.json），面板整列都是长串。上游 /api/v3/user/status 其实给了
// nickname/email，必须用它们覆盖兜底标签——但只覆盖兜底值，不覆盖用户自定义标签。
func TestManagementAccountsUsesUpstreamNicknameOverFileName(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	fallback := "qoder-98412b9f552c6bf89815cd146467c52f115b041f9ee1968b602f4be3dab0bbe9"
	defaultManagementService = &managementService{
		hostCall: uiHostCall([]pluginapi.HostAuthFileEntry{
			{AuthIndex: "a1", Name: fallback + ".json", Provider: "qoder", Label: fallback},
			{AuthIndex: "a2", Name: "x.json", Provider: "qoder", Label: "我的号", Email: "keep@me"},
		}, nil),
		fetchQuota: func(_ context.Context, cred qoderauth.Credential) (*qodercontrol.Quota, error) {
			if cred.AccessToken == "at-a1" {
				return &qodercontrol.Quota{Nickname: "nick5805205645", SyncedAt: time.Now().UTC()}, nil
			}
			return &qodercontrol.Quota{Nickname: "上游昵称", Email: "up@stream", SyncedAt: time.Now().UTC()}, nil
		},
	}
	response, err := (managementHandler{kind: "accounts"}).HandleManagement(context.Background(), pluginapi.ManagementRequest{})
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var body managementAccountsResponse
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Accounts) != 2 {
		t.Fatalf("accounts=%+v", body.Accounts)
	}
	if body.Accounts[0].Label != "nick5805205645" {
		t.Errorf("fallback label must be replaced by nickname, got %q", body.Accounts[0].Label)
	}
	if body.Accounts[1].Label != "我的号" || body.Accounts[1].Email != "keep@me" {
		t.Errorf("user-set label/email must win: %+v", body.Accounts[1])
	}
}

// 锁：模型目录合并必须按 profile 分区。qoder 国际站与 CN 站的同一模型倍率不同
// （实测 deepseek-v4.1-flash 国际 x0.00 / 国内 x0.11），按调用 ID 去重会让先遍历的
// 账号独占该行、另一站的真实倍率被静默丢弃。
func TestManagementModelsKeepsBothProfiles(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	defaultManagementService = &managementService{
		hostCall: uiHostCall([]pluginapi.HostAuthFileEntry{
			{AuthIndex: "intl", Name: "intl.json", Provider: "qoder"},
			{AuthIndex: "cn", Name: "cn.json", Provider: "qoder"},
		}, map[string]qoderauth.TransportProfile{
			"intl": qoderauth.TransportProfileCosyAPI2,
			"cn":   qoderauth.TransportProfileCosyCN,
		}),
		fetchModels: func(_ context.Context, cred qoderauth.Credential) ([]qodercontrol.Model, error) {
			if cred.AccessToken == "at-intl" {
				return []qodercontrol.Model{{ID: "qfmodel", Name: "Qwen3.8-Flash", PriceFactor: 0, IsFree: true}}, nil
			}
			return []qodercontrol.Model{{ID: "qfmodel", Name: "Qwen3.8-Flash", PriceFactor: 0.11}}, nil
		},
	}
	response, err := (managementHandler{kind: "models"}).HandleManagement(context.Background(), pluginapi.ManagementRequest{})
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var body managementModelsResponse
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Models) != 2 {
		t.Fatalf("want one row per profile, got %+v", body.Models)
	}
	byProfile := map[string]managementModel{}
	for _, model := range body.Models {
		byProfile[model.Profile] = model
	}
	if byProfile["cosy-api2"].Rate != 0 || !byProfile["cosy-api2"].IsFree {
		t.Errorf("intl row=%+v", byProfile["cosy-api2"])
	}
	if byProfile["cosy-cn"].Rate != 0.11 || byProfile["cosy-cn"].IsFree {
		t.Errorf("cn row=%+v", byProfile["cosy-cn"])
	}
}
