package main

import (
	"context"
	"encoding/json"
	"net/http"
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
	// Scheduler hook: per-AuthID quota snapshot for exhausted exclusion and weighting.
	if aid := strings.TrimSpace(req.AuthID); aid != "" {
		noteQuotaSnapshot(aid, quota.Remaining, quota.Exhausted)
	}
	return okEnvelope(pluginapi.QuotaFetchResponse{
		Summary: []pluginapi.QuotaMetric{
			{Key: "remain", Label: "剩余额度", Value: quota.Remaining, Unit: quota.Unit},
			{Key: "used", Label: "已用额度", Value: quota.Used, Unit: quota.Unit},
			{Key: "limit", Label: "额度上限", Value: quota.Limit, Unit: quota.Unit},
			{Key: "agentLimit", Label: "Agent额度", Value: quota.AgentLimit, Unit: quota.Unit},
		},
	})
}
