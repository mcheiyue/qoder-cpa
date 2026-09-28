package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type managementCampaignEntry struct {
	CampaignID    string  `json:"campaign_id"`
	CampaignKey   string  `json:"campaign_key"`
	ActionType    string  `json:"action_type"`
	ClaimStatus   string  `json:"claim_status"`
	BenefitKind   string  `json:"benefit_kind"`
	BenefitAmount float64 `json:"benefit_amount"`
}

type managementCampaignsResponse struct {
	AuthIndex string                    `json:"auth_index"`
	Campaigns []managementCampaignEntry `json:"campaigns"`
	SyncedAt  time.Time                 `json:"synced_at"`
}

type managementClaimRequest struct {
	AuthIndex  string `json:"auth_index"`
	CampaignID string `json:"campaign_id"`
}

type managementClaimResponse struct {
	AuthIndex     string    `json:"auth_index"`
	CampaignID    string    `json:"campaign_id"`
	Outcome       string    `json:"outcome"`
	Replayed      bool      `json:"replayed,omitempty"`
	GrantID       string    `json:"grant_id,omitempty"`
	BenefitKind   string    `json:"benefit_kind,omitempty"`
	BenefitAmount float64   `json:"benefit_amount,omitempty"`
	HTTPStatus    int       `json:"http_status,omitempty"`
	SyncedAt      time.Time `json:"synced_at"`
}

type managementActivityResponse struct {
	AuthIndex      string    `json:"auth_index"`
	TotalCredits   float64   `json:"total_credits"`
	PeakCredits    float64   `json:"peak_credits"`
	CumulativeDays int       `json:"cumulative_days"`
	CurrentStreak  int       `json:"current_streak"`
	MaxStreak      int       `json:"max_streak"`
	LastActiveDate string    `json:"last_active_date"`
	SyncedAt       time.Time `json:"synced_at"`
}

// resolveCredential loads the qoder credential for authIndex under s.mu.
// Returns a non-nil error response on failure.
func (s *managementService) resolveCredential(authIndex string) (qoderauth.Credential, pluginapi.ManagementResponse, bool) {
	rawAuth, err := s.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: authIndex})
	if err != nil {
		return qoderauth.Credential{}, jsonManagementError(http.StatusNotFound, "account not found"), false
	}
	var auth pluginapi.HostAuthGetResponse
	var storage qoderauth.StorageJSON
	if json.Unmarshal(rawAuth, &auth) != nil || json.Unmarshal(auth.JSON, &storage) != nil || storage.AccessToken == "" {
		return qoderauth.Credential{}, jsonManagementError(http.StatusBadRequest, "account is not a qoder credential"), false
	}
	return storage.ToCredential(), pluginapi.ManagementResponse{}, true
}

func (s *managementService) refreshCampaigns(ctx context.Context, raw []byte) (pluginapi.ManagementResponse, error) {
	var request quotaRefreshRequest
	if err := json.Unmarshal(raw, &request); err != nil || strings.TrimSpace(request.AuthIndex) == "" {
		return jsonManagementError(http.StatusBadRequest, "auth_index is required"), nil
	}
	if s.fetchCampaigns == nil {
		return jsonManagementError(http.StatusNotImplemented, "campaign refresh is unavailable"), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cred, failure, ok := s.resolveCredential(strings.TrimSpace(request.AuthIndex))
	if !ok {
		return failure, nil
	}
	result, err := s.fetchCampaigns(ctxOrBackground(ctx), cred)
	if err != nil || result == nil {
		return jsonManagementError(http.StatusBadGateway, "campaign refresh failed"), nil
	}
	entries := make([]managementCampaignEntry, 0, len(result.Campaigns))
	for _, campaign := range result.Campaigns {
		entries = append(entries, managementCampaignEntry{
			CampaignID: campaign.CampaignID, CampaignKey: campaign.CampaignKey,
			ActionType: campaign.ActionType, ClaimStatus: campaign.ClaimStatus,
			BenefitKind: campaign.Benefit.Kind, BenefitAmount: campaign.Benefit.Amount,
		})
	}
	return jsonManagementResponse(http.StatusOK, managementCampaignsResponse{
		AuthIndex: strings.TrimSpace(request.AuthIndex), Campaigns: entries, SyncedAt: result.SyncedAt,
	})
}

func (s *managementService) claimCampaign(ctx context.Context, raw []byte) (pluginapi.ManagementResponse, error) {
	var request managementClaimRequest
	if err := json.Unmarshal(raw, &request); err != nil || strings.TrimSpace(request.AuthIndex) == "" || strings.TrimSpace(request.CampaignID) == "" {
		return jsonManagementError(http.StatusBadRequest, "auth_index and campaign_id are required"), nil
	}
	if s.fetchCampaigns == nil || s.doClaimCampaign == nil {
		return jsonManagementError(http.StatusNotImplemented, "campaign claim is unavailable"), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cred, failure, ok := s.resolveCredential(strings.TrimSpace(request.AuthIndex))
	if !ok {
		return failure, nil
	}
	campaigns, err := s.fetchCampaigns(ctxOrBackground(ctx), cred)
	if err != nil || campaigns == nil {
		return jsonManagementError(http.StatusBadGateway, "campaign refresh failed"), nil
	}
	var target *qodercontrol.Campaign
	for i := range campaigns.Campaigns {
		if campaigns.Campaigns[i].CampaignID == strings.TrimSpace(request.CampaignID) {
			target = &campaigns.Campaigns[i]
			break
		}
	}
	if target == nil {
		return jsonManagementError(http.StatusNotFound, "campaign not found"), nil
	}
	result, claimErr := s.doClaimCampaign(ctxOrBackground(ctx), cred, *target)
	if claimErr != nil || result == nil {
		return jsonManagementError(http.StatusBadGateway, "campaign claim failed"), nil
	}
	return jsonManagementResponse(http.StatusOK, managementClaimResponse{
		AuthIndex: strings.TrimSpace(request.AuthIndex), CampaignID: target.CampaignID,
		Outcome: result.Outcome, Replayed: result.Replayed, GrantID: result.GrantID,
		BenefitKind: result.Benefit.Kind, BenefitAmount: result.Benefit.Amount,
		HTTPStatus: result.HTTPStatus, SyncedAt: result.SyncedAt,
	})
}

func (s *managementService) fetchActivity(ctx context.Context, raw []byte) (pluginapi.ManagementResponse, error) {
	var request quotaRefreshRequest
	if err := json.Unmarshal(raw, &request); err != nil || strings.TrimSpace(request.AuthIndex) == "" {
		return jsonManagementError(http.StatusBadRequest, "auth_index is required"), nil
	}
	if s.fetchCreditsSummary == nil || s.fetchSeatActivity == nil {
		return jsonManagementError(http.StatusNotImplemented, "activity fetch is unavailable"), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cred, failure, ok := s.resolveCredential(strings.TrimSpace(request.AuthIndex))
	if !ok {
		return failure, nil
	}
	credits, creditsErr := s.fetchCreditsSummary(ctxOrBackground(ctx), cred)
	seat, seatErr := s.fetchSeatActivity(ctxOrBackground(ctx), cred)
	if (credits == nil || creditsErr != nil) && (seat == nil || seatErr != nil) {
		return jsonManagementError(http.StatusBadGateway, "activity fetch failed"), nil
	}
	response := managementActivityResponse{AuthIndex: strings.TrimSpace(request.AuthIndex)}
	if credits != nil && creditsErr == nil {
		response.TotalCredits, response.PeakCredits = credits.TotalCredits, credits.PeakCredits
		response.SyncedAt = credits.SyncedAt
	}
	if seat != nil && seatErr == nil {
		response.CumulativeDays = seat.CumulativeActiveDays
		response.CurrentStreak = seat.CurrentConsecutiveDays
		response.MaxStreak = seat.MaxConsecutiveDays
		response.LastActiveDate = seat.LastActiveDate
		if response.SyncedAt.IsZero() {
			response.SyncedAt = seat.SyncedAt
		}
	}
	return jsonManagementResponse(http.StatusOK, response)
}

func fetchManagementCampaigns(ctx context.Context, cred qoderauth.Credential) (*qodercontrol.CampaignsResult, error) {
	client, err := qodercontrol.NewClient(nil, qodercontrol.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return client.FetchCampaigns(ctx, cred)
}

func claimManagementCampaign(ctx context.Context, cred qoderauth.Credential, campaign qodercontrol.Campaign) (*qodercontrol.ClaimResult, error) {
	client, err := qodercontrol.NewClient(nil, qodercontrol.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return client.ClaimCampaign(ctx, cred, campaign)
}

func fetchManagementCreditsSummary(ctx context.Context, cred qoderauth.Credential) (*qodercontrol.CreditsSummary, error) {
	client, err := qodercontrol.NewClient(nil, qodercontrol.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return client.FetchCreditsSummary(ctx, cred)
}

func fetchManagementSeatActivity(ctx context.Context, cred qoderauth.Credential) (*qodercontrol.SeatActivity, error) {
	client, err := qodercontrol.NewClient(nil, qodercontrol.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return client.FetchSeatActivity(ctx, cred)
}
