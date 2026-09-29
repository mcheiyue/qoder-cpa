package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type managementService struct {
	mu                  sync.Mutex
	hostCall            func(string, any) (json.RawMessage, error)
	fetchQuota          func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error)
	fetchModels         func(context.Context, qoderauth.Credential) ([]qodercontrol.Model, error)
	fetchCampaigns      func(context.Context, qoderauth.Credential) (*qodercontrol.CampaignsResult, error)
	doClaimCampaign     func(context.Context, qoderauth.Credential, qodercontrol.Campaign) (*qodercontrol.ClaimResult, error)
	fetchCreditsSummary func(context.Context, qoderauth.Credential) (*qodercontrol.CreditsSummary, error)
	fetchSeatActivity   func(context.Context, qoderauth.Credential) (*qodercontrol.SeatActivity, error)
}

var defaultManagementService = &managementService{
	hostCall: callHostJSON, fetchQuota: fetchManagementQuota, fetchModels: fetchManagementModels,
	fetchCampaigns: fetchManagementCampaigns, doClaimCampaign: claimManagementCampaign,
	fetchCreditsSummary: fetchManagementCreditsSummary, fetchSeatActivity: fetchManagementSeatActivity,
}

type managementHandler struct {
	kind string
}

type managementAccountsResponse struct {
	Accounts []managementAccount `json:"accounts"`
}

type managementModelsResponse struct {
	Models []managementModel `json:"models"`
}

type managementModel struct {
	AuthIndex   string `json:"auth_index"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
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
			{"method": http.MethodGet, "path": "/qoder/models"},
			{"method": http.MethodPost, "path": "/qoder/accounts/campaigns/refresh"},
			{"method": http.MethodPost, "path": "/qoder/accounts/campaigns/claim"},
			{"method": http.MethodPost, "path": "/qoder/accounts/activity/fetch"},
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
	case "models":
		return defaultManagementService.models(ctx)
	case "campaigns-refresh":
		return defaultManagementService.refreshCampaigns(ctx, request.Body)
	case "campaign-claim":
		return defaultManagementService.claimCampaign(ctx, request.Body)
	case "activity-fetch":
		return defaultManagementService.fetchActivity(ctx, request.Body)
	case "web":
		return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{
			"Content-Type":  {"text/html; charset=utf-8"},
			"Cache-Control": {"no-store"},
		}, Body: qoderWebUI}, nil
	default:
		return pluginapi.ManagementResponse{StatusCode: http.StatusNotFound}, nil
	}
}

func (s *managementService) models(ctx context.Context) (pluginapi.ManagementResponse, error) {
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
	merged := make(map[string]managementModel)
	for _, file := range result.Files {
		if !strings.EqualFold(file.Provider, qoderauth.Provider) && !strings.EqualFold(file.Type, qoderauth.Provider) || file.AuthIndex == "" {
			continue
		}
		rawAuth, getErr := s.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: file.AuthIndex})
		if getErr != nil {
			continue
		}
		var auth pluginapi.HostAuthGetResponse
		var storage qoderauth.StorageJSON
		if json.Unmarshal(rawAuth, &auth) != nil || json.Unmarshal(auth.JSON, &storage) != nil || storage.AccessToken == "" || s.fetchModels == nil {
			continue
		}
		models, fetchErr := s.fetchModels(ctxOrBackground(ctx), storage.ToCredential())
		if fetchErr != nil {
			continue
		}
		for _, model := range models {
			id := strings.TrimSpace(model.ID)
			if id == "" {
				continue
			}
			name := displayNameForModel(id, model.Name)
			key := publicModelID(name)
			if _, exists := merged[key]; exists {
				continue
			}
			merged[key] = managementModel{AuthIndex: file.AuthIndex, ID: key, DisplayName: name}
		}
	}
	models := make([]managementModel, 0, len(merged))
	for _, model := range merged {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return jsonManagementResponse(http.StatusOK, managementModelsResponse{Models: models})
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
