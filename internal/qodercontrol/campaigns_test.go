package qodercontrol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

const g1CanaryToken = "g1-canary-token"

func g1Credential() qoderauth.Credential {
	return qoderauth.Credential{AccessToken: g1CanaryToken}
}

func assertG1SashHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+g1CanaryToken {
		t.Fatalf("Authorization = %q, want Bearer token", got)
	}
	if got := r.Header.Get("Cosy-ClientType"); got != "10" {
		t.Fatalf("Cosy-ClientType = %q, want 10", got)
	}
	if got := r.Header.Get("Cosy-Version"); got != "1.1.34" {
		t.Fatalf("Cosy-Version = %q, want 1.1.34", got)
	}
}

func TestFetchCampaignsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sash/api/v1/me/campaigns" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		assertG1SashHeaders(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"campaigns":[{"campaignId":"c-1","campaignKey":"daily_checkin","actionType":"CLAIM_BENEFIT","claimStatus":"CLAIMABLE","benefit":{"kind":"credits","amount":50}}]}`))
	}))
	defer srv.Close()

	c, err := newTestClient(t, srv)
	if err != nil {
		t.Fatalf("newTestClient: %v", err)
	}
	result, err := c.FetchCampaigns(context.Background(), g1Credential())
	if err != nil {
		t.Fatalf("FetchCampaigns: %v", err)
	}
	if len(result.Campaigns) != 1 {
		t.Fatalf("campaigns = %d, want 1", len(result.Campaigns))
	}
	got := result.Campaigns[0]
	if got.CampaignID != "c-1" || got.ClaimStatus != "CLAIMABLE" || got.ActionType != "CLAIM_BENEFIT" {
		t.Errorf("campaign = %+v", got)
	}
	if got.Benefit.Kind != "credits" || got.Benefit.Amount != 50 {
		t.Errorf("benefit = %+v", got.Benefit)
	}
	if result.SyncedAt.IsZero() {
		t.Error("SyncedAt is zero")
	}
}

func TestFetchCampaignsEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"campaigns":[]}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	result, err := c.FetchCampaigns(context.Background(), g1Credential())
	if err != nil {
		t.Fatalf("FetchCampaigns empty: %v", err)
	}
	if result.Campaigns == nil || len(result.Campaigns) != 0 {
		t.Fatalf("campaigns = %#v, want empty non-nil", result.Campaigns)
	}
}

func TestFetchCampaignsUnauthorizedLeaksNoCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"TOKEN_INVALID"}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchCampaigns(context.Background(), g1Credential())
	if err == nil {
		t.Fatal("FetchCampaigns 401: want error")
	}
	var upstream *UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusUnauthorized {
		t.Fatalf("err = %v, want UpstreamError 401", err)
	}
	if containsCredential(err.Error()) {
		t.Fatalf("credential leaked in error: %v", err)
	}
}

func TestFetchCampaignsBodyTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"campaigns":["`))
		padding := make([]byte, defaultBodyLimit+1)
		for i := range padding {
			padding[i] = 'a'
		}
		_, _ = w.Write(padding)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchCampaigns(context.Background(), g1Credential())
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}
}
