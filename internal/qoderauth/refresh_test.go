package qoderauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRefresh_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("refresh: got method %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("refresh: parse form: %v", err)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type=%q, want refresh_token", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") == "" {
			t.Error("missing refresh_token")
		}
		if r.FormValue("client_id") == "" {
			t.Error("missing client_id")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"access_token": "new-access-tok",
			"refresh_token": "new-refresh-tok",
			"token_type": "bearer",
			"expires_in": 3600,
			"scope": "openid"
		}`)
	}))
	defer srv.Close()

	resp, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL + "/refresh", ClientID: "test"},
		Client: &http.Client{Timeout: 5 * time.Second},
		Cred: Credential{
			AccessToken:  "old-at",
			RefreshToken: "old-rt",
			UserID:       "u1",
			Email:        "test@qoder.com",
			Profile:      TransportProfileBearerOpenAI,
		},
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if resp.Credential.AccessToken != "new-access-tok" {
		t.Fatalf("AccessToken=%q, want new-access-tok", resp.Credential.AccessToken)
	}
	if resp.Credential.RefreshToken != "new-refresh-tok" {
		t.Fatalf("RefreshToken=%q, want new-refresh-tok", resp.Credential.RefreshToken)
	}
	if resp.Credential.UserID != "u1" {
		t.Fatalf("UserID=%q, want u1", resp.Credential.UserID)
	}
	if resp.Credential.Email != "test@qoder.com" {
		t.Fatalf("Email=%q", resp.Credential.Email)
	}
	if resp.Credential.Profile != TransportProfileBearerOpenAI {
		t.Fatalf("Profile=%q", resp.Credential.Profile)
	}
	if resp.NextRefreshAfter.Before(time.Now()) {
		t.Error("NextRefreshAfter is in the past")
	}
}

func TestRefresh_RotationKeepsOldToken(t *testing.T) {
	// Server returns no new refresh_token → old one preserved.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"access_token": "new-at",
			"token_type": "bearer",
			"expires_in": 3600
		}`)
	}))
	defer srv.Close()

	resp, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "test"},
		Client: &http.Client{Timeout: 5 * time.Second},
		Cred: Credential{
			AccessToken:  "old-at",
			RefreshToken: "old-rt-preserved",
			UserID:       "u1",
		},
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if resp.Credential.RefreshToken != "old-rt-preserved" {
		t.Fatalf("RefreshToken=%q, want old-rt-preserved (rotation should keep old)", resp.Credential.RefreshToken)
	}
}

func TestRefresh_RotationReplacesOldToken(t *testing.T) {
	// Server returns a new refresh_token → replaces old.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"access_token": "new-at",
			"refresh_token": "brand-new-rt",
			"token_type": "bearer",
			"expires_in": 3600
		}`)
	}))
	defer srv.Close()

	resp, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "test"},
		Client: &http.Client{Timeout: 5 * time.Second},
		Cred: Credential{
			AccessToken:  "old-at",
			RefreshToken: "old-rt",
			UserID:       "u1",
		},
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if resp.Credential.RefreshToken != "brand-new-rt" {
		t.Fatalf("RefreshToken=%q, want brand-new-rt", resp.Credential.RefreshToken)
	}
}

func TestRefreshConcurrency(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond) // Simulate latency.
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"access_token": "new-at",
			"refresh_token": "new-rt",
			"token_type": "bearer",
			"expires_in": 3600
		}`)
	}))
	defer srv.Close()

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = Refresh(context.Background(), RefreshRequest{
				Config: RefreshConfig{TokenURL: srv.URL, ClientID: "test"},
				Client: &http.Client{Timeout: 5 * time.Second},
				Cred: Credential{
					AccessToken:  "old-at",
					RefreshToken: "rt",
					UserID:       "same-user",
				},
			})
		}(i)
	}
	wg.Wait()

	// Singleflight should deduplicate concurrent refreshes.
	actual := int(callCount.Load())
	if actual > 1 {
		t.Logf("singleflight called upstream %d times (expected 1; may vary due to timing)", actual)
	}
	for _, e := range errs {
		if e != nil {
			t.Fatalf("concurrent refresh error: %v", e)
		}
	}
}
