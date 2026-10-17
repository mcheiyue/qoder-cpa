package main

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// campaignsClaimer 自动领取 qoder 每日 credits 活动（CN 站每日 10:00 UTC+8 刷新）。
// 纯响应式插件没有后台循环，这里自带一个按小时 tick 的 goroutine，每天在 hour 槽跑一次。
type campaignsClaimer struct {
	hostCall        func(string, any) (json.RawMessage, error)
	fetchCampaigns  func(context.Context, qoderauth.Credential) (*qodercontrol.CampaignsResult, error)
	doClaimCampaign func(context.Context, qoderauth.Credential, qodercontrol.Campaign) (*qodercontrol.ClaimResult, error)
	now             func() time.Time
}

var opsOnce sync.Once

var defaultCampaignsClaimer = &campaignsClaimer{
	hostCall:        callHostJSON,
	fetchCampaigns:  fetchManagementCampaigns,
	doClaimCampaign: claimManagementCampaign,
	now:             time.Now,
}

// claimAll 遍历全部 qoder 账号，只对 CLAIMABLE 的 CLAIM_BENEFIT 活动发领取请求，
// 返回成功领取数。单账号失败只记日志，不中断整轮（坏 token 不得让其余号当天漏领）。
func (c *campaignsClaimer) claimAll(ctx context.Context) int {
	raw, err := c.hostCall(pluginabi.MethodHostAuthList, nil)
	if err != nil {
		log.Printf("[qoder-ops] claim: list auth failed: %v", err)
		return 0
	}
	var list struct {
		Files []pluginapi.HostAuthFileEntry `json:"files"`
	}
	if json.Unmarshal(raw, &list) != nil {
		log.Printf("[qoder-ops] claim: decode auth list failed")
		return 0
	}
	claimed := 0
	seen := make(map[string]bool, len(list.Files))
	for _, file := range list.Files {
		if !strings.EqualFold(file.Provider, qoderauth.Provider) && !strings.EqualFold(file.Type, qoderauth.Provider) {
			continue
		}
		authIndex := strings.TrimSpace(file.AuthIndex)
		if authIndex == "" || seen[authIndex] {
			continue
		}
		seen[authIndex] = true
		cred, ok := c.credential(authIndex)
		if !ok {
			continue
		}
		result, err := c.fetchCampaigns(ctx, cred)
		if err != nil || result == nil {
			log.Printf("[qoder-ops] claim: campaigns fetch failed auth=%s err=%v", tail(authIndex), err)
			continue
		}
		for _, campaign := range result.Campaigns {
			if campaign.ActionType != qodercontrol.ActionClaimBenefit || campaign.ClaimStatus != "CLAIMABLE" {
				continue
			}
			if _, err := c.doClaimCampaign(ctx, cred, campaign); err != nil {
				log.Printf("[qoder-ops] claim: auth=%s campaign=%s failed: %v", tail(authIndex), campaign.CampaignID, err)
				continue
			}
			claimed++
			log.Printf("[qoder-ops] claim: auth=%s campaign=%s claimed %.0f %s",
				tail(authIndex), campaign.CampaignKey, campaign.Benefit.Amount, campaign.Benefit.Kind)
		}
	}
	return claimed
}

func (c *campaignsClaimer) credential(authIndex string) (qoderauth.Credential, bool) {
	raw, err := c.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: authIndex})
	if err != nil {
		return qoderauth.Credential{}, false
	}
	var auth pluginapi.HostAuthGetResponse
	var storage qoderauth.StorageJSON
	if json.Unmarshal(raw, &auth) != nil || json.Unmarshal(auth.JSON, &storage) != nil || storage.AccessToken == "" {
		return qoderauth.Credential{}, false
	}
	return storage.ToCredential(), true
}

func tail(s string) string {
	if len(s) > 8 {
		return s[len(s)-8:]
	}
	return s
}

// startCampaignsOps 启动每日领取循环（幂等）。每次重算触发时刻并抽新的抖动分钟。
func startCampaignsOps() {
	opsOnce.Do(func() {
		go func() {
			time.Sleep(2 * time.Second)
			for {
				next := nextClaimAt(defaultCampaignsClaimer.now(), rand.Intn(30))
				time.Sleep(time.Until(next))
				defaultCampaignsClaimer.claimAll(context.Background())
			}
		}()
	})
}

// nextClaimAt 返回下一个领取时刻：当天 opsClaimHour 点 + jitter 分钟；已过点则滚到明日。
func nextClaimAt(now time.Time, jitter int) time.Time {
	at := time.Date(now.Year(), now.Month(), now.Day(), opsClaimHour, jitter, 0, 0, now.Location())
	if !at.After(now) {
		at = at.Add(24 * time.Hour)
	}
	return at
}

const opsClaimHour = 10
