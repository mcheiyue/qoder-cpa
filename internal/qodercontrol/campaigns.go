package qodercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

const (
	campaignsPath   = "/sash/api/v1/me/campaigns"
	actionClaim     = "CLAIM_BENEFIT"
	statusClaimable = "CLAIMABLE"
	statusExpired   = "EXPIRED"
	statusClaimed   = "CLAIMED"
)

// Benefit describes a campaign reward.
type Benefit struct {
	Kind   string  `json:"kind"`
	Amount float64 `json:"amount"`
}

// Campaign is a claimable benefit entry from the sash campaigns list.
type Campaign struct {
	CampaignID  string  `json:"campaignId"`
	CampaignKey string  `json:"campaignKey"`
	ActionType  string  `json:"actionType"`
	ClaimStatus string  `json:"claimStatus"`
	Benefit     Benefit `json:"benefit"`
}

// CampaignsResult is a campaigns snapshot with sync timestamp.
type CampaignsResult struct {
	Campaigns []Campaign `json:"campaigns"`
	SyncedAt  time.Time  `json:"synced_at"`
}

// ClaimResult records the outcome of a claim attempt.
type ClaimResult struct {
	Outcome    string    `json:"outcome"` // claimed | replayed | expired | rejected
	Replayed   bool      `json:"replayed"`
	GrantID    string    `json:"grantId,omitempty"`
	Benefit    Benefit   `json:"benefit,omitempty"`
	HTTPStatus int       `json:"http_status,omitempty"`
	SyncedAt   time.Time `json:"synced_at"`
}

// FetchCampaigns lists sash campaigns for the account.
func (c *Client) FetchCampaigns(ctx context.Context, cred qoderauth.Credential) (*CampaignsResult, error) {
	body, err := c.doControlRequest(ctx, http.MethodGet, c.buildURL(campaignsPath), cred.AccessToken)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Campaigns []Campaign `json:"campaigns"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: campaigns: %v", ErrMalformedResponse, err)
	}
	if payload.Campaigns == nil {
		payload.Campaigns = []Campaign{}
	}
	return &CampaignsResult{Campaigns: payload.Campaigns, SyncedAt: time.Now().UTC()}, nil
}

// ClaimCampaign claims a single campaign. Non-CLAIMABLE or non-CLAIM_BENEFIT
// entries are rejected locally without an HTTP request.
func (c *Client) ClaimCampaign(ctx context.Context, cred qoderauth.Credential, campaign Campaign) (*ClaimResult, error) {
	if campaign.ActionType != actionClaim {
		return nil, fmt.Errorf("qodercontrol: campaign %s has action %q, want %s", campaign.CampaignID, campaign.ActionType, actionClaim)
	}
	if campaign.ClaimStatus != statusClaimable {
		outcome := "rejected"
		if campaign.ClaimStatus == statusExpired {
			outcome = "expired"
		}
		return &ClaimResult{Outcome: outcome, SyncedAt: time.Now().UTC()}, nil
	}
	endpoint := c.buildURL(campaignsPath + "/" + campaign.CampaignID + "/claim")
	body, err := c.doControlRequest(ctx, http.MethodPost, endpoint, cred.AccessToken)
	if err != nil {
		var upstream *UpstreamError
		if errors.As(err, &upstream) && upstream.StatusCode != http.StatusUnauthorized && upstream.StatusCode != http.StatusForbidden {
			return &ClaimResult{Outcome: "rejected", HTTPStatus: upstream.StatusCode, SyncedAt: time.Now().UTC()}, nil
		}
		return nil, err
	}
	var payload struct {
		Status   string  `json:"status"`
		Replayed bool    `json:"replayed"`
		GrantID  string  `json:"grantId"`
		Benefit  Benefit `json:"benefit"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: claim: %v", ErrMalformedResponse, err)
	}
	result := &ClaimResult{Replayed: payload.Replayed, GrantID: payload.GrantID, Benefit: payload.Benefit, SyncedAt: time.Now().UTC()}
	switch {
	case payload.Status == statusClaimed && payload.Replayed:
		result.Outcome = "replayed"
	case payload.Status == statusClaimed:
		result.Outcome = "claimed"
	case payload.Status == statusExpired:
		result.Outcome = "expired"
	default:
		result.Outcome = "rejected"
	}
	return result, nil
}
