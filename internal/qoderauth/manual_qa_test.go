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

	var deviceCodeCalls, tokenCalls, refreshCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/device/code":
			deviceCodeCalls.Add(1)
			if r.Method != http.MethodPost {
				t.Errorf("device code: method=%s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("device code: parse form: %v", err)
			}
			// Assert PKCE challenge present and S256 method.
			challenge := r.FormValue("code_challenge")
			if challenge == "" {
				t.Error("device code: missing code_challenge")
			}
			method := r.FormValue("code_challenge_method")
			if method != "S256" {
				t.Errorf("device code: method=%q, want S256", method)
			}
			// Assert nonce present.
			nonce := r.FormValue("nonce")
			if nonce == "" {
				t.Error("device code: missing nonce")
			}
			// Assert machine_id present.
			machineID := r.FormValue("machine_id")
			if machineID == "" {
				t.Error("device code: missing machine_id")
			}
			if !strings.HasPrefix(machineID, "m-") {
				t.Errorf("device code: machine_id=%q, missing m- prefix", machineID)
			}
			// Assert client_id.
			if r.FormValue("client_id") == "" {
				t.Error("device code: missing client_id")
			}

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"device_code": "dc-manual-qa",
				"user_code": "QA12-3456",
				"verification_uri": "https://qoder.com/activate",
				"expires_in": 900,
				"interval": 5
			}`)

		case "/oauth/token":
			tokenCalls.Add(1)
			if r.Method != http.MethodPost {
				t.Errorf("token: method=%s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("token: parse form: %v", err)
			}
			if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
				t.Errorf("token: grant_type=%q", r.FormValue("grant_type"))
			}
			if r.FormValue("device_code") != "dc-manual-qa" {
				t.Errorf("token: device_code=%q", r.FormValue("device_code"))
			}
			// Return success immediately.
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"access_token": "manual-qa-access-token",
				"refresh_token": "manual-qa-refresh-token",
				"token_type": "bearer",
				"expires_in": 3600,
				"scope": "openid profile"
			}`)

		case "/oauth/refresh":
			refreshCalls.Add(1)
			if r.Method != http.MethodPost {
				t.Errorf("refresh: method=%s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("refresh: parse form: %v", err)
			}
			if r.FormValue("grant_type") != "refresh_token" {
				t.Errorf("refresh: grant_type=%q", r.FormValue("grant_type"))
			}
			if r.FormValue("refresh_token") != "manual-qa-refresh-token" {
				t.Errorf("refresh: refresh_token=%q", r.FormValue("refresh_token"))
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"access_token": "refreshed-access-token",
				"refresh_token": "refreshed-refresh-token",
				"token_type": "bearer",
				"expires_in": 3600
			}`)

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := OAuthConfig{
		BaseURL:        srv.URL,
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "manual-qa-test",
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
	if loginResp.VerifyURL != "https://qoder.com/activate" {
		t.Errorf("VerifyURL=%q", loginResp.VerifyURL)
	}
	if loginResp.DeviceCode != "dc-manual-qa" {
		t.Errorf("DeviceCode=%q", loginResp.DeviceCode)
	}
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
		Config: RefreshConfig{TokenURL: srv.URL + "/oauth/refresh", ClientID: cfg.ClientID},
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
	dc := int(deviceCodeCalls.Load())
	tc := int(tokenCalls.Load())
	rc := int(refreshCalls.Load())
	t.Logf("Call counts: device_code=%d, token=%d, refresh=%d", dc, tc, rc)
	if dc != 1 {
		t.Errorf("device_code calls=%d, want 1", dc)
	}
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
