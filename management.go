package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type managementService struct {
	mu         sync.Mutex
	hostCall   func(string, any) (json.RawMessage, error)
	fetchQuota func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error)
}

var defaultManagementService = &managementService{hostCall: callHostJSON, fetchQuota: fetchManagementQuota}

type managementHandler struct {
	kind string
}

type managementAccountsResponse struct {
	Accounts []managementAccount `json:"accounts"`
}

type managementAccount struct {
	AuthIndex     string     `json:"auth_index"`
	Name          string     `json:"name"`
	Label         string     `json:"label,omitempty"`
	Email         string     `json:"email,omitempty"`
	Profile       string     `json:"transport_profile,omitempty"`
	Status        string     `json:"status,omitempty"`
	Disabled      bool       `json:"disabled,omitempty"`
	NeedsAuth     bool       `json:"needs_reauth,omitempty"`
	PlanTier      string     `json:"plan_tier,omitempty"`
	UserType      string     `json:"user_type,omitempty"`
	PaidPlan      bool       `json:"paid_plan,omitempty"`
	Remaining     float64    `json:"remaining,omitempty"`
	Limit         float64    `json:"limit,omitempty"`
	Used          float64    `json:"used,omitempty"`
	AgentLimit    float64    `json:"agent_limit,omitempty"`
	Exhausted     bool       `json:"exhausted,omitempty"`
	Unit          string     `json:"unit,omitempty"`
	ResetAt       *time.Time `json:"reset_at,omitempty"`
	PeriodEnd     *time.Time `json:"period_end,omitempty"`
	UpgradeURL    string     `json:"upgrade_url,omitempty"`
	QuotaError    string     `json:"quota_error,omitempty"`
	QuotaSyncedAt *time.Time `json:"quota_synced_at,omitempty"`
}

type profileUpdateRequest struct {
	AuthIndex string                     `json:"auth_index"`
	Profile   qoderauth.TransportProfile `json:"transport_profile"`
}

type quotaRefreshRequest struct {
	AuthIndex string `json:"auth_index"`
}

type managementQuotaResponse struct {
	AuthIndex string `json:"auth_index"`
	managementQuota
}

type managementQuota struct {
	PlanTier      string     `json:"plan_tier,omitempty"`
	UserType      string     `json:"user_type,omitempty"`
	PaidPlan      bool       `json:"paid_plan,omitempty"`
	Remaining     float64    `json:"remaining,omitempty"`
	Limit         float64    `json:"limit,omitempty"`
	Used          float64    `json:"used,omitempty"`
	AgentLimit    float64    `json:"agent_limit,omitempty"`
	Exhausted     bool       `json:"exhausted,omitempty"`
	Unit          string     `json:"unit,omitempty"`
	ResetAt       *time.Time `json:"reset_at,omitempty"`
	PeriodEnd     *time.Time `json:"period_end,omitempty"`
	UpgradeURL    string     `json:"upgrade_url,omitempty"`
	QuotaError    string     `json:"quota_error,omitempty"`
	QuotaSyncedAt *time.Time `json:"quota_synced_at,omitempty"`
}

func managementRegister() map[string]any {
	return map[string]any{
		"routes": []map[string]string{
			{"method": http.MethodGet, "path": "/qoder/accounts"},
			{"method": http.MethodPost, "path": "/qoder/accounts/profile"},
			{"method": http.MethodPost, "path": "/qoder/accounts/quota/refresh"},
		},
		"resources": []map[string]string{{"path": "/index.html", "menu": "Qoder"}},
	}
}

func (h managementHandler) HandleManagement(ctx context.Context, request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	switch h.kind {
	case "accounts":
		return defaultManagementService.accounts(ctx)
	case "profile":
		return defaultManagementService.updateProfile(ctx, request.Body)
	case "quota-refresh":
		return defaultManagementService.refreshQuota(ctx, request.Body)
	case "web":
		return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{
			"Content-Type":  {"text/html; charset=utf-8"},
			"Cache-Control": {"no-store"},
		}, Body: qoderWebUI}, nil
	default:
		return pluginapi.ManagementResponse{StatusCode: http.StatusNotFound}, nil
	}
}

func (s *managementService) accounts(ctx context.Context) (pluginapi.ManagementResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := s.hostCall(pluginabi.MethodHostAuthList, nil)
	if err != nil {
		return pluginapi.ManagementResponse{}, err
	}
	var result struct {
		Files []pluginapi.HostAuthFileEntry `json:"files"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return pluginapi.ManagementResponse{}, fmt.Errorf("decode auth list: %w", err)
	}
	accounts := make([]managementAccount, 0, len(result.Files))
	for _, file := range result.Files {
		if !strings.EqualFold(file.Provider, qoderauth.Provider) && !strings.EqualFold(file.Type, qoderauth.Provider) {
			continue
		}
		profile := qoderauth.TransportProfileCosyAPI2
		account := managementAccount{
			AuthIndex: file.AuthIndex, Name: file.Name, Label: file.Label,
			Email: file.Email, Profile: string(profile), Status: file.Status, Disabled: file.Disabled,
		}
		if file.AuthIndex != "" {
			if rawAuth, getErr := s.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: file.AuthIndex}); getErr == nil {
				var auth pluginapi.HostAuthGetResponse
				var storage qoderauth.StorageJSON
				if json.Unmarshal(rawAuth, &auth) == nil && json.Unmarshal(auth.JSON, &storage) == nil {
					if qoderauth.IsValidProfile(storage.Profile) {
						account.Profile = string(storage.Profile)
					}
					account.NeedsAuth = storage.MachineID == ""
					if s.fetchQuota != nil && storage.AccessToken != "" {
						quota, quotaErr := s.fetchQuota(ctxOrBackground(ctx), storage.ToCredential())
						applyManagementQuota(&account, quota, quotaErr)
					}
				}
			}
		}
		accounts = append(accounts, account)
	}
	return jsonManagementResponse(http.StatusOK, managementAccountsResponse{Accounts: accounts})
}

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
	return jsonManagementResponse(http.StatusOK, response)
}

func fetchManagementQuota(ctx context.Context, cred qoderauth.Credential) (*qodercontrol.Quota, error) {
	client, err := qodercontrol.NewClient(nil, qodercontrol.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return client.FetchQuota(ctx, cred)
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
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
	a.Exhausted, a.Unit, a.ResetAt, a.PeriodEnd = quota.Exhausted, quota.Unit, quota.ResetAt, quota.PeriodEnd
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
	if _, err := s.hostCall(pluginabi.MethodHostAuthSave, pluginapi.HostAuthSaveRequest{Name: name, JSON: encoded}); err != nil {
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
