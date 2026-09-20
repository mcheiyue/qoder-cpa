package qoderauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestManualQA_FullLifecycle exercises start→poll→refresh with header assertions.
func TestManualQA_FullLifecycle(t *testing.T) {
	resetStore()

	var tokenCalls, refreshCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/deviceToken/poll":
			tokenCalls.Add(1)
			if r.Method != http.MethodGet || r.URL.Query().Get("verifier") == "" {
				t.Errorf("poll request invalid: method=%s query=%s", r.Method, r.URL.RawQuery)
			}
			// Return success immediately.
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"token": "manual-qa-access-token",
				"refresh_token": "manual-qa-refresh-token",
				"user_id": "manual-user",
				"expires_in": 3600,
				"scope": "openid profile"
			}`)

		case "/api/v1/deviceToken/refresh":
			refreshCalls.Add(1)
			if r.Method != http.MethodPost {
				t.Errorf("refresh: method=%s, want POST", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"token": "refreshed-access-token",
				"refresh_token": "refreshed-refresh-token",
				"user_id": "manual-user",
				"expires_in": 3600
			}`)

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := OAuthConfig{
		BaseURL:    srv.URL,
		APIBaseURL: srv.URL, PollPath: "/api/v1/deviceToken/poll",
		ClientID: "manual-qa-test",
	}
	client := &http.Client{Timeout: 5 * time.Second}

	// === STEP 1: DeviceLogin ===
	loginResp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config:    cfg,
		Client:    client,
		MachineID: "m-qa-machine-001",
		TTL:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("DeviceLogin: %v", err)
	}
	t.Logf("Login: VerifyURL=%s, DeviceCode=%s, TxnID=%s", loginResp.VerifyURL, loginResp.DeviceCode, loginResp.Transaction.ID)

	// Assert login response.
	if loginResp.Transaction.Machine != "m-qa-machine-001" {
		t.Errorf("Machine=%q", loginResp.Transaction.Machine)
	}
	if loginResp.Transaction.Verifier == "" {
		t.Error("Verifier is empty")
	}
	if loginResp.Transaction.Nonce == "" {
		t.Error("Nonce is empty")
	}
	if loginResp.Transaction.Status != TransactionPending {
		t.Errorf("Status=%q", loginResp.Transaction.Status)
	}

	// === STEP 2: PollLogin → success ===
	status, err := PollLogin(context.Background(), PollLoginRequest{
		Config:        cfg,
		Client:        client,
		TransactionID: loginResp.Transaction.ID,
	})
	if err != nil {
		t.Fatalf("PollLogin: %v", err)
	}
	if status.Status != TransactionSuccess {
		t.Fatalf("Poll status=%q, want success", status.Status)
	}
	if status.Credential == nil {
		t.Fatal("Credential is nil")
	}

	// Assert credential fields.
	cred := status.Credential
	if cred.AccessToken != "manual-qa-access-token" {
		t.Errorf("AccessToken=%q", cred.AccessToken)
	}
	if cred.RefreshToken != "manual-qa-refresh-token" {
		t.Errorf("RefreshToken=%q", cred.RefreshToken)
	}
	if cred.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt in the past")
	}

	// === STEP 3: Verify AuthID determinism ===
	authID := newAuthID("manual-qa-user")
	authID2 := newAuthID("manual-qa-user")
	if authID != authID2 {
		t.Errorf("AuthID not deterministic: %q != %q", authID, authID2)
	}
	t.Logf("AuthID: %s", authID)

	// === STEP 4: Verify StorageJSON redaction ===
	sj := FromCredential(*cred)
	raw, err := json.Marshal(sj)
	if err != nil {
		t.Fatal(err)
	}
	jsonStr := string(raw)
	for _, forbidden := range []string{"verifier", "nonce", "device_code", "verify_url"} {
		if strings.Contains(jsonStr, forbidden) {
			t.Errorf("StorageJSON contains forbidden key: %s", forbidden)
		}
	}
	t.Logf("StorageJSON (redacted): %s", jsonStr)

	// === STEP 5: Refresh ===
	refreshResp, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL + "/api/v1/deviceToken/refresh", ClientID: cfg.ClientID},
		Client: client,
		Cred:   *cred,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshResp.Credential.AccessToken != "refreshed-access-token" {
		t.Errorf("Refresh AccessToken=%q", refreshResp.Credential.AccessToken)
	}
	if refreshResp.Credential.RefreshToken != "refreshed-refresh-token" {
		t.Errorf("Refresh RefreshToken=%q", refreshResp.Credential.RefreshToken)
	}
	// Metadata preserved.
	if refreshResp.Credential.UserID != cred.UserID {
		t.Errorf("Refresh UserID=%q, want %q", refreshResp.Credential.UserID, cred.UserID)
	}

	// === Assert call counts ===
	tc := int(tokenCalls.Load())
	rc := int(refreshCalls.Load())
	t.Logf("Call counts: poll=%d, refresh=%d", tc, rc)
	if tc != 1 {
		t.Errorf("token calls=%d, want 1", tc)
	}
	if rc != 1 {
		t.Errorf("refresh calls=%d, want 1", rc)
	}

	// === Cleanup ===
	CancelTransaction(loginResp.Transaction.ID)
	if _, ok := GetTransaction(loginResp.Transaction.ID); ok {
		t.Error("transaction should be deleted after cancel")
	}
}
