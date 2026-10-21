package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func (s *managementService) refreshQuota(ctx context.Context, raw []byte) (pluginapi.ManagementResponse, error) {
	var request quotaRefreshRequest
	if err := json.Unmarshal(raw, &request); err != nil || strings.TrimSpace(request.AuthIndex) == "" {
		return jsonManagementError(http.StatusBadRequest, "auth_index is required"), nil
	}
	if s.fetchQuota == nil {
		return jsonManagementError(http.StatusNotImplemented, "quota refresh is unavailable"), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rawAuth, err := s.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: strings.TrimSpace(request.AuthIndex)})
	if err != nil {
		return jsonManagementError(http.StatusNotFound, "account not found"), nil
	}
	var auth pluginapi.HostAuthGetResponse
	var storage qoderauth.StorageJSON
	if json.Unmarshal(rawAuth, &auth) != nil || json.Unmarshal(auth.JSON, &storage) != nil || storage.AccessToken == "" {
		return jsonManagementError(http.StatusBadRequest, "account is not a qoder credential"), nil
	}
	quota, quotaErr := s.fetchQuota(ctxOrBackground(ctx), storage.ToCredential())
	response := managementQuotaResponse{AuthIndex: strings.TrimSpace(request.AuthIndex)}
	applyManagementQuota(&response.managementQuota, quota, quotaErr)
	if quota == nil {
		return jsonManagementError(http.StatusBadGateway, "quota refresh failed"), nil
	}
	// Persist only the real UserType reported by quota/profile endpoints.
	// Missing (empty) never overwrites storage with a static account class.
	changed := false
	if quota.UserType != "" && quota.UserType != storage.UserType {
		storage.UserType = quota.UserType
		changed = true
	}
	// 显示名只写一次：Label 已有值即为用户自定义，不覆盖。
	// 宿主与 cpamp 读顶层 label/email，缺失时显示文件名；国内站上游 email 恒空，
	// 手机号是唯一可读标识，故 email 位置写 Mobile。
	if storage.Label == "" && quota.Nickname != "" {
		storage.Label = quota.Nickname
		changed = true
	}
	if storage.Email == "" {
		if ident := quota.Email; ident != "" {
			storage.Email = ident
			changed = true
		} else if quota.Mobile != "" {
			storage.Email = quota.Mobile
			changed = true
		}
	}
	if changed && auth.Name != "" {
		if encoded, err := json.Marshal(storage); err == nil {
			_ = s.saveAuthPreservingHostKeys(auth.Name, auth.JSON, encoded)
		}
	}
	return jsonManagementResponse(http.StatusOK, response)
}

// controlConfigForProfile 按凭证 transport profile 选控制面基址：
// cosy-cn 走 CN 站，其余走国际站（与 models.go 的端点选择同源）。
func controlConfigForProfile(profile qoderauth.TransportProfile) qodercontrol.Config {
	if profile == qoderauth.TransportProfileCosyCN {
		return qodercontrol.DefaultConfigCN()
	}
	return qodercontrol.DefaultConfig()
}

func fetchManagementQuota(ctx context.Context, cred qoderauth.Credential) (*qodercontrol.Quota, error) {
	client, err := qodercontrol.NewClient(nil, controlConfigForProfile(cred.Profile))
	if err != nil {
		return nil, err
	}
	return client.FetchQuota(ctx, cred)
}

func fetchManagementModels(ctx context.Context, cred qoderauth.Credential) ([]qodercontrol.Model, error) {
	client, err := qodercontrol.NewClient(nil, controlConfigForProfile(cred.Profile))
	if err != nil {
		return nil, err
	}
	return client.FetchModels(ctx, cred)
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// applyUpstreamIdentity 用上游 nickname/email 补齐展示身份。只覆盖兜底值
// （空、或等于 provider/文件名），用户自定义的 label/email 优先保留。
func applyUpstreamIdentity(account *managementAccount, quota *qodercontrol.Quota) {
	if quota == nil {
		return
	}
	if nickname := strings.TrimSpace(quota.Nickname); nickname != "" && isFallbackIdentity(account) {
		account.Label = nickname
	}
	if email := strings.TrimSpace(quota.Email); email != "" && account.Email == "" {
		account.Email = email
	}
	// 国内站账号 email 为空，用 security_mobile 填 email 列，否则面板只剩无区分度的昵称。
	if account.Email == "" {
		if mobile := strings.TrimSpace(quota.Mobile); mobile != "" {
			account.Email = mobile
		}
	}
}

func isFallbackIdentity(account *managementAccount) bool {
	if fallbackQoderLabel(account.Label) {
		return true
	}
	return account.Label == strings.TrimSuffix(account.Name, ".json")
}

func applyManagementQuota(target interface{ setQuota(managementQuota) }, quota *qodercontrol.Quota, quotaErr error) {
	if quota == nil {
		if quotaErr != nil {
			target.setQuota(managementQuota{QuotaError: qodercontrolError(quotaErr)})
		}
		return
	}
	value := managementQuota{
		PlanTier: quota.PlanTier, UserType: quota.UserType, PaidPlan: quota.PaidPlan,
		Remaining: quota.Remaining, Limit: quota.Limit, Used: quota.Used, AgentLimit: quota.AgentLimit,
		AddOn: quota.AddOn, AddOnRemaining: quota.AddOnRemaining,
		Exhausted: quota.Exhausted, Unit: quota.Unit, UpgradeURL: quota.UpgradeURL,
		QuotaError: quota.Error,
	}
	if quotaErr != nil && value.QuotaError == "" {
		value.QuotaError = qodercontrolError(quotaErr)
	}
	if !quota.ResetAt.IsZero() {
		resetAt := quota.ResetAt
		value.ResetAt = &resetAt
	}
	if !quota.PeriodStart.IsZero() {
		periodStart := quota.PeriodStart
		value.PeriodStart = &periodStart
	}
	if !quota.PeriodEnd.IsZero() {
		periodEnd := quota.PeriodEnd
		value.PeriodEnd = &periodEnd
	}
	if !quota.SyncedAt.IsZero() {
		syncedAt := quota.SyncedAt
		value.QuotaSyncedAt = &syncedAt
	}
	target.setQuota(value)
}

func qodercontrolError(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(err.Error(), "quota status unknown") {
		return "额度状态未知"
	}
	return "额度刷新失败"
}

func (a *managementAccount) setQuota(quota managementQuota) {
	a.PlanTier, a.UserType, a.PaidPlan = quota.PlanTier, quota.UserType, quota.PaidPlan
	a.Remaining, a.Limit, a.Used, a.AgentLimit = quota.Remaining, quota.Limit, quota.Used, quota.AgentLimit
	a.AddOn, a.AddOnRemaining = quota.AddOn, quota.AddOnRemaining
	a.Exhausted, a.Unit, a.ResetAt = quota.Exhausted, quota.Unit, quota.ResetAt
	a.PeriodStart, a.PeriodEnd = quota.PeriodStart, quota.PeriodEnd
	a.UpgradeURL, a.QuotaError, a.QuotaSyncedAt = quota.UpgradeURL, quota.QuotaError, quota.QuotaSyncedAt
}

func (q *managementQuota) setQuota(value managementQuota) { *q = value }

func (s *managementService) updateProfile(_ context.Context, raw []byte) (pluginapi.ManagementResponse, error) {
	var request profileUpdateRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return jsonManagementError(http.StatusBadRequest, "invalid JSON body"), nil
	}
	request.AuthIndex = strings.TrimSpace(request.AuthIndex)
	request.Profile = qoderauth.TransportProfile(strings.TrimSpace(string(request.Profile)))
	if request.AuthIndex == "" || !qoderauth.IsValidProfile(request.Profile) {
		return jsonManagementError(http.StatusBadRequest, "auth_index and a valid transport_profile are required"), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rawAuth, err := s.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: request.AuthIndex})
	if err != nil {
		return jsonManagementError(http.StatusNotFound, "account not found"), nil
	}
	var auth pluginapi.HostAuthGetResponse
	if err := json.Unmarshal(rawAuth, &auth); err != nil {
		return pluginapi.ManagementResponse{}, fmt.Errorf("decode auth: %w", err)
	}
	var storage qoderauth.StorageJSON
	if err := json.Unmarshal(auth.JSON, &storage); err != nil {
		return jsonManagementError(http.StatusBadRequest, "account is not a qoder credential"), nil
	}
	if storage.UserID == "" || storage.AccessToken == "" {
		return jsonManagementError(http.StatusBadRequest, "account is not a qoder credential"), nil
	}
	storage.Profile = request.Profile
	encoded, err := json.Marshal(storage)
	if err != nil {
		return pluginapi.ManagementResponse{}, fmt.Errorf("encode auth: %w", err)
	}
	name := auth.Name
	if name == "" {
		return jsonManagementError(http.StatusBadRequest, "account has no writable name"), nil
	}
	if err := s.saveAuthPreservingHostKeys(name, auth.JSON, encoded); err != nil {
		return jsonManagementError(http.StatusBadGateway, "account update failed"), nil
	}
	return jsonManagementResponse(http.StatusOK, map[string]string{
		"auth_index": request.AuthIndex, "transport_profile": string(request.Profile),
	})
}

func jsonManagementResponse(status int, value any) (pluginapi.ManagementResponse, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return pluginapi.ManagementResponse{}, err
	}
	return pluginapi.ManagementResponse{StatusCode: status, Headers: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
}

func jsonManagementError(status int, message string) pluginapi.ManagementResponse {
	response, _ := jsonManagementResponse(status, map[string]string{"error": message})
	return response
}
