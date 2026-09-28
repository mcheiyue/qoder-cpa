package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func g1HostCall() func(string, any) (json.RawMessage, error) {
	return func(method string, payload any) (json.RawMessage, error) {
		switch method {
		case pluginabi.MethodHostAuthGet:
			return json.Marshal(pluginapi.HostAuthGetResponse{AuthIndex: "qoder-1", JSON: mustJSON(qoderauth.StorageJSON{
				AccessToken: "g1-canary-token", UserID: "u1", Profile: qoderauth.TransportProfileBearerOpenAI,
			})})
		default:
			return nil, nil
		}
	}
}

func TestManagementCampaignsRefreshSanitized(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	syncedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	defaultManagementService = &managementService{
		hostCall: g1HostCall(),
		fetchCampaigns: func(_ context.Context, cred qoderauth.Credential) (*qodercontrol.CampaignsResult, error) {
			if cred.AccessToken != "g1-canary-token" {
				t.Fatalf("credential=%+v", cred)
			}
			return &qodercontrol.CampaignsResult{
				Campaigns: []qodercontrol.Campaign{
					{CampaignID: "c-1", CampaignKey: "daily", ActionType: "CLAIM_BENEFIT", ClaimStatus: "CLAIMABLE", Benefit: qodercontrol.Benefit{Kind: "credits", Amount: 50}},
					{CampaignID: "c-2", CampaignKey: "streak5", ActionType: "CLAIM_BENEFIT", ClaimStatus: "CLAIMED", Benefit: qodercontrol.Benefit{Kind: "credits", Amount: 100}},
				},
				SyncedAt: syncedAt,
			}, nil
		},
	}
	response, err := (managementHandler{kind: "campaigns-refresh"}).HandleManagement(context.Background(),
		pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1"}`)})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var body struct {
		AuthIndex string `json:"auth_index"`
		Campaigns []struct {
			CampaignID    string  `json:"campaign_id"`
			CampaignKey   string  `json:"campaign_key"`
			ActionType    string  `json:"action_type"`
			ClaimStatus   string  `json:"claim_status"`
			BenefitKind   string  `json:"benefit_kind"`
			BenefitAmount float64 `json:"benefit_amount"`
		} `json:"campaigns"`
		SyncedAt time.Time `json:"synced_at"`
	}
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.AuthIndex != "qoder-1" || len(body.Campaigns) != 2 || body.SyncedAt.IsZero() {
		t.Fatalf("body=%s", response.Body)
	}
	if body.Campaigns[0].CampaignID != "c-1" || body.Campaigns[0].ClaimStatus != "CLAIMABLE" || body.Campaigns[0].BenefitAmount != 50 {
		t.Fatalf("campaigns=%+v", body.Campaigns)
	}
	for _, secret := range []string{"g1-canary-token", "access_token", "refresh_token", "runtime_key"} {
		if strings.Contains(string(response.Body), secret) {
			t.Fatalf("secret leaked: %s in %s", secret, response.Body)
		}
	}
}

func TestManagementCampaignClaimOutcomeAndUnknownID(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	defaultManagementService = &managementService{
		hostCall: g1HostCall(),
		fetchCampaigns: func(context.Context, qoderauth.Credential) (*qodercontrol.CampaignsResult, error) {
			return &qodercontrol.CampaignsResult{Campaigns: []qodercontrol.Campaign{
				{CampaignID: "c-1", ActionType: "CLAIM_BENEFIT", ClaimStatus: "CLAIMABLE", Benefit: qodercontrol.Benefit{Kind: "credits", Amount: 50}},
			}}, nil
		},
		doClaimCampaign: func(_ context.Context, cred qoderauth.Credential, campaign qodercontrol.Campaign) (*qodercontrol.ClaimResult, error) {
			if cred.AccessToken != "g1-canary-token" || campaign.CampaignID != "c-1" {
				t.Fatalf("cred=%+v campaign=%+v", cred, campaign)
			}
			return &qodercontrol.ClaimResult{Outcome: "claimed", GrantID: "g-9", Benefit: campaign.Benefit, SyncedAt: time.Now().UTC()}, nil
		},
	}
	response, err := (managementHandler{kind: "campaign-claim"}).HandleManagement(context.Background(),
		pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1","campaign_id":"c-1"}`)})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var body struct {
		Outcome       string  `json:"outcome"`
		GrantID       string  `json:"grant_id"`
		BenefitAmount float64 `json:"benefit_amount"`
	}
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "claimed" || body.GrantID != "g-9" || body.BenefitAmount != 50 {
		t.Fatalf("body=%s", response.Body)
	}
	if strings.Contains(string(response.Body), "g1-canary-token") {
		t.Fatalf("secret leaked: %s", response.Body)
	}

	missing, err := (managementHandler{kind: "campaign-claim"}).HandleManagement(context.Background(),
		pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1","campaign_id":"nope"}`)})
	if err != nil || missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing response=%+v err=%v", missing, err)
	}
}

func TestManagementActivityFetchCombinesCreditsAndSeat(t *testing.T) {
	previous := defaultManagementService
	t.Cleanup(func() { defaultManagementService = previous })
	syncedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	defaultManagementService = &managementService{
		hostCall: g1HostCall(),
		fetchCreditsSummary: func(_ context.Context, cred qoderauth.Credential) (*qodercontrol.CreditsSummary, error) {
			if cred.AccessToken != "g1-canary-token" {
				t.Fatalf("credential=%+v", cred)
			}
			return &qodercontrol.CreditsSummary{TotalCredits: 1200, PeakCredits: 3400, SyncedAt: syncedAt}, nil
		},
		fetchSeatActivity: func(context.Context, qoderauth.Credential) (*qodercontrol.SeatActivity, error) {
			return &qodercontrol.SeatActivity{CumulativeActiveDays: 42, CurrentConsecutiveDays: 5, MaxConsecutiveDays: 30, LastActiveDate: "2026-09-28", SyncedAt: syncedAt}, nil
		},
	}
	response, err := (managementHandler{kind: "activity-fetch"}).HandleManagement(context.Background(),
		pluginapi.ManagementRequest{Body: []byte(`{"auth_index":"qoder-1"}`)})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var body struct {
		TotalCredits   float64   `json:"total_credits"`
		PeakCredits    float64   `json:"peak_credits"`
		CumulativeDays int       `json:"cumulative_days"`
		CurrentStreak  int       `json:"current_streak"`
		MaxStreak      int       `json:"max_streak"`
		LastActiveDate string    `json:"last_active_date"`
		SyncedAt       time.Time `json:"synced_at"`
	}
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalCredits != 1200 || body.CumulativeDays != 42 || body.CurrentStreak != 5 || body.LastActiveDate != "2026-09-28" || body.SyncedAt.IsZero() {
		t.Fatalf("body=%s", response.Body)
	}
	if strings.Contains(string(response.Body), "g1-canary-token") {
		t.Fatalf("secret leaked: %s", response.Body)
	}
}
