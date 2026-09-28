package qodercontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaimCampaignClaimed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sash/api/v1/me/campaigns/c-1/claim" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		assertG1SashHeaders(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"CLAIMED","replayed":false,"benefit":{"kind":"credits","amount":50},"grantId":"g-9"}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	campaign := Campaign{CampaignID: "c-1", ActionType: "CLAIM_BENEFIT", ClaimStatus: "CLAIMABLE"}
	result, err := c.ClaimCampaign(context.Background(), g1Credential(), campaign)
	if err != nil {
		t.Fatalf("ClaimCampaign: %v", err)
	}
	if result.Outcome != "claimed" {
		t.Errorf("Outcome = %q, want claimed", result.Outcome)
	}
	if result.GrantID != "g-9" {
		t.Errorf("GrantID = %q, want g-9", result.GrantID)
	}
	if result.Benefit.Amount != 50 {
		t.Errorf("Benefit = %+v", result.Benefit)
	}
}

func TestClaimCampaignReplayed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"CLAIMED","replayed":true,"benefit":{"kind":"credits","amount":50},"grantId":"g-1"}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	campaign := Campaign{CampaignID: "c-1", ActionType: "CLAIM_BENEFIT", ClaimStatus: "CLAIMABLE"}
	result, err := c.ClaimCampaign(context.Background(), g1Credential(), campaign)
	if err != nil {
		t.Fatalf("ClaimCampaign replayed: %v", err)
	}
	if result.Outcome != "replayed" {
		t.Errorf("Outcome = %q, want replayed", result.Outcome)
	}
	if !result.Replayed {
		t.Error("Replayed = false, want true")
	}
}

func TestClaimCampaignExpiredStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"EXPIRED"}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	campaign := Campaign{CampaignID: "c-1", ActionType: "CLAIM_BENEFIT", ClaimStatus: "CLAIMABLE"}
	result, err := c.ClaimCampaign(context.Background(), g1Credential(), campaign)
	if err != nil {
		t.Fatalf("ClaimCampaign expired: %v", err)
	}
	if result.Outcome != "expired" {
		t.Errorf("Outcome = %q, want expired", result.Outcome)
	}
}

func TestClaimCampaignRejectsNonClaimableLocally(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	campaign := Campaign{CampaignID: "c-1", ActionType: "CLAIM_BENEFIT", ClaimStatus: "EXPIRED"}
	result, err := c.ClaimCampaign(context.Background(), g1Credential(), campaign)
	if err != nil {
		t.Fatalf("ClaimCampaign local expired: %v", err)
	}
	if called {
		t.Fatal("claim HTTP request sent for non-CLAIMABLE campaign")
	}
	if result.Outcome != "expired" {
		t.Errorf("Outcome = %q, want expired", result.Outcome)
	}
}

func TestClaimCampaignRejectsWrongActionType(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	campaign := Campaign{CampaignID: "c-1", ActionType: "BONUS", ClaimStatus: "CLAIMABLE"}
	_, err := c.ClaimCampaign(context.Background(), g1Credential(), campaign)
	if err == nil {
		t.Fatal("ClaimCampaign wrong actionType: want error")
	}
	if called {
		t.Fatal("claim HTTP request sent for non-CLAIM_BENEFIT campaign")
	}
}

func TestClaimCampaignRejectedByServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"already claimed elsewhere"}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	campaign := Campaign{CampaignID: "c-1", ActionType: "CLAIM_BENEFIT", ClaimStatus: "CLAIMABLE"}
	result, err := c.ClaimCampaign(context.Background(), g1Credential(), campaign)
	if err != nil {
		t.Fatalf("ClaimCampaign 409: %v", err)
	}
	if result.Outcome != "rejected" {
		t.Errorf("Outcome = %q, want rejected", result.Outcome)
	}
	if result.HTTPStatus != http.StatusConflict {
		t.Errorf("HTTPStatus = %d, want 409", result.HTTPStatus)
	}
}

func TestClaimCampaignUnauthorizedIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	campaign := Campaign{CampaignID: "c-1", ActionType: "CLAIM_BENEFIT", ClaimStatus: "CLAIMABLE"}
	_, err := c.ClaimCampaign(context.Background(), g1Credential(), campaign)
	if err == nil {
		t.Fatal("ClaimCampaign 401: want error")
	}
	if containsCredential(err.Error()) {
		t.Fatalf("credential leaked in error: %v", err)
	}
}

func containsCredential(message string) bool {
	return strings.Contains(message, g1CanaryToken)
}
