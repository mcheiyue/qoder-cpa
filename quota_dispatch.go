package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// quotaFetchFn is the quota upstream seam (shared with management quota refresh); tests stub it.
var quotaFetchFn = fetchManagementQuota

// handleQuotaMethod dispatches quota.* RPC methods for the host native quota panel.
func handleQuotaMethod(method string, raw []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodQuotaIdentifier:
		return okEnvelope(struct {
			Identifier string `json:"identifier"`
		}{Identifier: qoderauth.Provider})
	case pluginabi.MethodQuotaDescribe:
		return okEnvelope(pluginapi.QuotaDescribeResponse{
			SupportedProviders: []string{qoderauth.Provider},
			DisplayName:        "Qoder",
			SupportsReset:      false,
		})
	case pluginabi.MethodQuotaFetch:
		return handleQuotaFetch(raw)
	case pluginabi.MethodQuotaReset:
		return okEnvelope(pluginapi.QuotaResetResponse{
			Success: false,
			Message: "qoder does not support quota reset",
		})
	default:
		return errorEnvelope("unknown_method", "unknown quota method: "+method), nil
	}
}

// handleQuotaFetch queries upstream quota from provider-owned storage JSON.
func handleQuotaFetch(raw []byte) ([]byte, error) {
	var req pluginapi.QuotaFetchRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorEnvelopeStatus("invalid_request", "invalid quota fetch request", http.StatusBadRequest), nil
	}
	if req.Provider != "" && !strings.EqualFold(req.Provider, qoderauth.Provider) {
		return errorEnvelope("unsupported_provider", "unsupported provider: "+req.Provider), nil
	}
	var storage qoderauth.StorageJSON
	if err := json.Unmarshal(req.StorageJSON, &storage); err != nil || storage.AccessToken == "" {
		return errorEnvelope("auth_error", "invalid qoder credential"), nil
	}
	quota, err := quotaFetchFn(context.Background(), storage.ToCredential())
	if err != nil {
		// Upstream errors surface as an ok envelope with a masked fixed label
		// (qodercontrolError never echoes upstream text or tokens).
		return okEnvelope(pluginapi.QuotaFetchResponse{
			Summary: []pluginapi.QuotaMetric{{
				Key: "error", Label: "额度查询失败", Value: 0, Unit: qodercontrolError(err),
			}},
		})
	}
	if quota == nil {
		return okEnvelope(pluginapi.QuotaFetchResponse{
			Summary: []pluginapi.QuotaMetric{{
				Key: "error", Label: "额度查询失败", Value: 0, Unit: "额度状态未知",
			}},
		})
	}
	// Scheduler weighting intentionally remains based on personal and add-on pools.
	if aid := strings.TrimSpace(req.AuthID); aid != "" {
		noteQuotaSnapshot(aid, quota.Remaining+quota.AddOnRemaining, quota.Exhausted)
	}
	remaining := quota.Remaining + quota.AddOnRemaining
	used := quota.Used + quota.AddOnUsed
	limit := quota.Limit + quota.AddOn
	if quota.Shared != nil && quota.Shared.Available && !quota.Shared.Unlimited {
		remaining += quota.Shared.Remaining
		used += quota.Shared.Used
		limit += quota.Shared.Limit
	}
	summary := []pluginapi.QuotaMetric{
		{Key: "remain", Label: "通用剩余额度", Value: remaining, Unit: quota.Unit},
		{Key: "used", Label: "通用已用额度", Value: used, Unit: quota.Unit},
		{Key: "limit", Label: "通用额度上限", Value: limit, Unit: quota.Unit},
		{Key: "agentLimit", Label: "Agent额度", Value: quota.AgentLimit, Unit: quota.Unit},
		{Key: "planRemain", Label: "主额度剩余", Value: quota.Remaining, Unit: quota.Unit},
		{Key: "addOnRemain", Label: "资源包剩余", Value: quota.AddOnRemaining, Unit: quota.Unit},
	}
	if shared := quota.Shared; shared != nil {
		if shared.Unlimited {
			summary = append(summary, pluginapi.QuotaMetric{Key: "shared", Label: "组织共享额度（不限量）已用", Value: shared.Used, Unit: quota.Unit})
		} else {
			label := "组织共享"
			if !shared.Available {
				label += "（不可用）"
			}
			summary = append(summary,
				pluginapi.QuotaMetric{Key: "sharedLimit", Label: label + "上限", Value: shared.Limit, Unit: quota.Unit},
				pluginapi.QuotaMetric{Key: "sharedUsed", Label: label + "已用", Value: shared.Used, Unit: quota.Unit},
				pluginapi.QuotaMetric{Key: "sharedRemain", Label: label + "剩余", Value: shared.Remaining, Unit: quota.Unit},
			)
		}
	}
	for i, pack := range quota.Dedicated {
		label := pack.Name
		if !pack.Available {
			label += "（不可用）"
		}
		if !pack.ExpiresAt.IsZero() {
			label += " · " + pack.ExpiresAt.Format("2006-01-02") + " 到期"
		}
		summary = append(summary, pluginapi.QuotaMetric{Key: "dedicated" + strconv.Itoa(i), Label: label, Value: pack.Remaining, Unit: quota.Unit})
	}
	return okEnvelope(pluginapi.QuotaFetchResponse{Summary: summary})
}
