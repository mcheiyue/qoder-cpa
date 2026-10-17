package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func claimHostCall(indexes ...string) func(string, any) (json.RawMessage, error) {
	return func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostAuthList:
			files := make([]pluginapi.HostAuthFileEntry, 0, len(indexes))
			for _, idx := range indexes {
				files = append(files, pluginapi.HostAuthFileEntry{
					Provider: qoderauth.Provider, Type: qoderauth.Provider, AuthIndex: idx, ID: idx,
				})
			}
			return json.Marshal(struct {
				Files []pluginapi.HostAuthFileEntry `json:"files"`
			}{Files: files})
		case pluginabi.MethodHostAuthGet:
			return json.Marshal(pluginapi.HostAuthGetResponse{
				AuthIndex: "x",
				JSON:      mustJSON(qoderauth.StorageJSON{AccessToken: "at", UserID: "u"}),
			})
		}
		return nil, nil
	}
}

// 锁：领取时刻必须带抖动——固定分钟位（如每天 10:05）在多账号下是明显的脚本特征。
// 同时锁住跨日滚动：今天已过点必须滚到明日，否则会当天连跑两次。
func TestNextClaimAt(t *testing.T) {
	base := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	for jitter := 0; jitter < 30; jitter++ {
		at := nextClaimAt(base, jitter)
		if at.Hour() != opsClaimHour || at.Minute() != jitter || at.Day() != 18 {
			t.Fatalf("jitter=%d got %v, want 2026-10-18 %02d:%02d", jitter, at, opsClaimHour, jitter)
		}
	}
	after := time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC)
	if at := nextClaimAt(after, 7); at.Day() != 19 {
		t.Fatalf("after-window must roll to next day, got %v", at)
	}
}

// 锁：只对 CLAIMABLE 的 CLAIM_BENEFIT 活动发写请求——已领（CLAIMED）再领会打上游
// 重复领取端点，非 CLAIM_BENEFIT 类型（如签到类）走了会 400。
func TestClaimAllCampaigns_OnlyClaimable(t *testing.T) {
	var claimed []string
	c := &campaignsClaimer{
		hostCall: claimHostCall("a1"),
		fetchCampaigns: func(context.Context, qoderauth.Credential) (*qodercontrol.CampaignsResult, error) {
			return &qodercontrol.CampaignsResult{Campaigns: []qodercontrol.Campaign{
				{CampaignID: "c-claim", ActionType: qodercontrol.ActionClaimBenefit, ClaimStatus: "CLAIMABLE"},
				{CampaignID: "c-done", ActionType: qodercontrol.ActionClaimBenefit, ClaimStatus: "CLAIMED"},
				{CampaignID: "c-signin", ActionType: "DAILY_SIGNIN", ClaimStatus: "CLAIMABLE"},
			}}, nil
		},
		doClaimCampaign: func(_ context.Context, _ qoderauth.Credential, campaign qodercontrol.Campaign) (*qodercontrol.ClaimResult, error) {
			claimed = append(claimed, campaign.CampaignID)
			return &qodercontrol.ClaimResult{}, nil
		},
	}
	if got := c.claimAll(context.Background()); got != 1 {
		t.Fatalf("claimed=%d, want 1", got)
	}
	if len(claimed) != 1 || claimed[0] != "c-claim" {
		t.Fatalf("claimed=%v, want [c-claim]", claimed)
	}
}

// 锁：单账号取活动失败不得中断整轮——多账号场景下必须继续处理其余账号，
// 否则一个坏 token 会让所有 CN 号当天都领不到。
// 锁：单账号取活动失败不得中断整轮——多账号场景下必须继续处理其余账号，
// 否则一个坏 token 会让所有 CN 号当天都领不到。
func TestClaimAllCampaigns_FetchErrorDoesNotAbort(t *testing.T) {
	var claimed []string
	c := &campaignsClaimer{
		hostCall: claimHostCall("bad", "good"),
		fetchCampaigns: func(_ context.Context, cred qoderauth.Credential) (*qodercontrol.CampaignsResult, error) {
			if cred.AccessToken == "" {
				t.Fatal("credential must be resolved before fetch")
			}
			return &qodercontrol.CampaignsResult{Campaigns: []qodercontrol.Campaign{
				{CampaignID: "c-1", ActionType: qodercontrol.ActionClaimBenefit, ClaimStatus: "CLAIMABLE"},
			}}, nil
		},
		doClaimCampaign: func(_ context.Context, _ qoderauth.Credential, campaign qodercontrol.Campaign) (*qodercontrol.ClaimResult, error) {
			claimed = append(claimed, campaign.CampaignID)
			if len(claimed) == 1 {
				return nil, errors.New("upstream claim failed")
			}
			return &qodercontrol.ClaimResult{}, nil
		},
	}
	if got := c.claimAll(context.Background()); got != 1 {
		t.Fatalf("claimed=%d, want 1 (one failure must not abort the loop)", got)
	}
	if len(claimed) != 2 {
		t.Fatalf("attempts=%d, want 2 (both accounts processed)", len(claimed))
	}
}
