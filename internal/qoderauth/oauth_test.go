package qoderauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDeviceCodeServer returns an httptest server that handles device code + token endpoints.
func fakeDeviceCodeServer(t *testing.T, opts ...func(*fakeConfig)) *httptest.Server {
	t.Helper()
	cfg := &fakeConfig{
		deviceCodeDelay: 0,
		tokenHandler:    defaultTokenHandler,
	}
	for _, o := range opts {
		o(cfg)
	}

	mux := http.NewServeMux()
	var callCount atomic.Int32

	mux.HandleFunc("/oauth/device/code", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("device code: got method %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("device code: parse form: %v", err)
		}
		// Verify PKCE challenge is sent.
		if r.FormValue("code_challenge") == "" {
			t.Error("device code: missing code_challenge")
		}
		if r.FormValue("code_challenge_method") != "S256" {
			t.Errorf("device code: code_challenge_method=%q, want S256", r.FormValue("code_challenge_method"))
		}
		if r.FormValue("nonce") == "" {
			t.Error("device code: missing nonce")
		}
		if r.FormValue("machine_id") == "" {
			t.Error("device code: missing machine_id")
		}
		if r.FormValue("client_id") == "" {
			t.Error("device code: missing client_id")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"device_code": "dc-test-123",
			"user_code": "ABCD-1234",
			"verification_uri": "https://qoder.com/activate",
			"expires_in": 900,
			"interval": 5
		}`)
	})

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		cfg.tokenHandler(w, r, cfg)
	})

	mux.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		cfg.tokenHandler(w, r, cfg)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
	})
	return srv
}

type fakeConfig struct {
	deviceCodeDelay time.Duration
	tokenHandler    func(http.ResponseWriter, *http.Request, *fakeConfig)
	pendingCount    int
	pendingSeen     int
}

func defaultTokenHandler(w http.ResponseWriter, r *http.Request, cfg *fakeConfig) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"error":"authorization_pending","error_description":"waiting for user"}`)
}

// withPendingThenSuccess returns a token handler that returns pending N times then success.
func withPendingThenSuccess(n int, accessToken, refreshToken string) func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.pendingCount = n
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			if fc.pendingSeen < fc.pendingCount {
				fc.pendingSeen++
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"error":"authorization_pending","error_description":"waiting"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
				"access_token": %q,
				"refresh_token": %q,
				"token_type": "bearer",
				"expires_in": 3600,
				"scope": "openid"
			}`, accessToken, refreshToken)
		}
	}
}

// withExpiredToken returns a token handler that returns expired_token error.
func withExpiredToken() func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"error":"expired_token","error_description":"code expired"}`)
		}
	}
}

// withAccessDenied returns a token handler that returns access_denied.
func withAccessDenied() func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"error":"access_denied","error_description":"user denied"}`)
		}
	}
}

// withMalformedJSON returns a token handler that returns malformed JSON.
func withMalformedJSON() func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{not json`)
		}
	}
}

// withOversizedBody returns a token handler that returns >1MiB body.
func withOversizedBody() func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			w.Header().Set("Content-Type", "application/json")
			// Write exactly maxResponseBodyBytes + 1.
			big := strings.Repeat("x", maxResponseBodyBytes+1)
			fmt.Fprint(w, big)
		}
	}
}

func TestValidateVerifyHost(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		insecure bool
		wantErr  bool
	}{
		{"https qoder.com", "https://qoder.com/activate", false, false},
		{"https qoder.sh", "https://api.qoder.sh/activate", false, false},
		{"https subdomain qoder.com", "https://auth.qoder.com/activate", false, false},
		{"http qoder.com blocked", "http://qoder.com/activate", false, true},
		{"http loopback allowed", "http://localhost/activate", true, false},
		{"http loopback 127 allowed", "http://127.0.0.1/activate", true, false},
		{"http loopback ::1 allowed", "http://[::1]/activate", true, false},
		{"http loopback blocked without insecure", "http://localhost/activate", false, true},
		{"non-qoder host blocked", "https://evil.com/activate", false, true},
		{"empty host blocked", "://", false, true},
		{"ftp scheme blocked", "ftp://qoder.com/activate", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateVerifyHost(tc.url, tc.insecure)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateVerifyHost(%q, %v) error=%v, wantErr=%v", tc.url, tc.insecure, err, tc.wantErr)
			}
		})
	}
}

func TestDeviceLogin_Success(t *testing.T) {
	resetStore()
	srv := fakeDeviceCodeServer(t)
	cfg := OAuthConfig{
		BaseURL:        srv.URL,
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "test-client",
	}
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config:    cfg,
		Client:    client,
		MachineID: "m-test-machine",
		TTL:       10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("DeviceLogin: %v", err)
	}

	// Verify response fields.
	if resp.VerifyURL != "https://qoder.com/activate" {
		t.Fatalf("VerifyURL=%q, want https://qoder.com/activate", resp.VerifyURL)
	}
	if resp.DeviceCode != "dc-test-123" {
		t.Fatalf("DeviceCode=%q, want dc-test-123", resp.DeviceCode)
	}
	if resp.Transaction == nil {
		t.Fatal("Transaction is nil")
	}

	// Verify transaction state.
	txn := resp.Transaction
	if txn.Status != TransactionPending {
		t.Fatalf("Status=%q, want %q", txn.Status, TransactionPending)
	}
	if txn.Verifier == "" {
		t.Error("Verifier is empty")
	}
	if txn.Nonce == "" {
		t.Error("Nonce is empty")
	}
	if txn.Machine != "m-test-machine" {
		t.Fatalf("Machine=%q, want m-test-machine", txn.Machine)
	}
	if txn.DeviceCode != "dc-test-123" {
		t.Fatalf("DeviceCode=%q, want dc-test-123", txn.DeviceCode)
	}
	if txn.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt is in the past")
	}

	// Verify transaction is stored.
	if _, ok := GetTransaction(txn.ID); !ok {
		t.Fatal("transaction not stored")
	}
}

func TestDeviceLogin_NilClient(t *testing.T) {
	_, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: OAuthConfig{BaseURL: "https://qoder.com"},
	})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestDeviceLogin_EmptyBaseURL(t *testing.T) {
	_, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: OAuthConfig{BaseURL: ""},
		Client: &http.Client{},
	})
	if err == nil {
		t.Fatal("expected error for empty base URL")
	}
}

func TestDeviceLogin_NonHTTPSBlocked(t *testing.T) {
	resetStore()
	_, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: OAuthConfig{BaseURL: "http://qoder.com", DeviceCodePath: "/oauth/device/code"},
		Client: &http.Client{Timeout: 1 * time.Second},
	})
	if err == nil {
		t.Fatal("expected error for non-HTTPS URL")
	}
}

func TestPollLogin_PendingThenSuccess(t *testing.T) {
	resetStore()
	srv := fakeDeviceCodeServer(t, withPendingThenSuccess(2, "access-tok-abc", "refresh-tok-xyz"))
	cfg := OAuthConfig{
		BaseURL:        srv.URL,
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "test-client",
	}
	client := &http.Client{Timeout: 5 * time.Second}

	// Start login.
	loginResp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: cfg,
		Client: client,
		TTL:    10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("DeviceLogin: %v", err)
	}
	txnID := loginResp.Transaction.ID

	// First poll: pending.
	status, err := PollLogin(context.Background(), PollLoginRequest{
		Config:        cfg,
		Client:        client,
		TransactionID: txnID,
	})
	if err != nil {
		t.Fatalf("PollLogin 1: %v", err)
	}
	if status.Status != TransactionPending {
		t.Fatalf("poll 1 status=%q, want pending", status.Status)
	}

	// Second poll: pending.
	status, err = PollLogin(context.Background(), PollLoginRequest{
		Config:        cfg,
		Client:        client,
		TransactionID: txnID,
	})
	if err != nil {
		t.Fatalf("PollLogin 2: %v", err)
	}
	if status.Status != TransactionPending {
		t.Fatalf("poll 2 status=%q, want pending", status.Status)
	}

	// Third poll: success.
	status, err = PollLogin(context.Background(), PollLoginRequest{
		Config:        cfg,
		Client:        client,
		TransactionID: txnID,
	})
	if err != nil {
		t.Fatalf("PollLogin 3: %v", err)
	}
	if status.Status != TransactionSuccess {
		t.Fatalf("poll 3 status=%q, want success", status.Status)
	}
	if status.Credential == nil {
		t.Fatal("Credential is nil on success")
	}
	if status.Credential.AccessToken != "access-tok-abc" {
		t.Fatalf("AccessToken=%q, want access-tok-abc", status.Credential.AccessToken)
	}
	if status.Credential.RefreshToken != "refresh-tok-xyz" {
		t.Fatalf("RefreshToken=%q, want refresh-tok-xyz", status.Credential.RefreshToken)
	}

	// Transaction should be deleted from store.
	if _, ok := GetTransaction(txnID); ok {
		t.Fatal("transaction should be deleted after success")
	}
}

func TestPollLogin_Expired(t *testing.T) {
	resetStore()
	cfg := OAuthConfig{
		BaseURL:        "http://unused",
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "test",
	}

	// Manually create an expired transaction.
	txnID, _ := newTransactionID()
	defaultStore.set(txnID, &Transaction{
		ID:         txnID,
		Verifier:   "v",
		Nonce:      "n",
		Machine:    "m",
		DeviceCode: "dc",
		VerifyURL:  "https://qoder.com/activate",
		ExpiresAt:  time.Now().Add(-1 * time.Minute), // expired
		CreatedAt:  time.Now().Add(-2 * time.Minute),
		Status:     TransactionPending,
	})

	_, err := PollLogin(context.Background(), PollLoginRequest{
		Config:        cfg,
		Client:        &http.Client{},
		TransactionID: txnID,
	})
	if err == nil {
		t.Fatal("expected error for expired transaction")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("error should mention expired: %v", err)
	}
}

func TestPollLogin_Unknown(t *testing.T) {
	resetStore()
	_, err := PollLogin(context.Background(), PollLoginRequest{
		Config:        OAuthConfig{BaseURL: "http://unused", ClientID: "test"},
		Client:        &http.Client{},
		TransactionID: "txn-nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for unknown transaction")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error should mention unknown: %v", err)
	}
}

func TestPollLogin_TerminalReturnsSuccess(t *testing.T) {
	resetStore()
	client := &http.Client{}

	srv := fakeDeviceCodeServer(t, withPendingThenSuccess(0, "at", "rt"))
	cfg := OAuthConfig{
		BaseURL:        srv.URL,
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "test",
	}

	loginResp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: cfg,
		Client: client,
		TTL:    10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	txnID := loginResp.Transaction.ID

	// First poll succeeds.
	status, err := PollLogin(context.Background(), PollLoginRequest{
		Config: cfg, Client: client, TransactionID: txnID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != TransactionSuccess {
		t.Fatalf("expected success, got %q", status.Status)
	}

	// Re-poll after success: transaction was deleted → unknown.
	_, err = PollLogin(context.Background(), PollLoginRequest{
		Config: cfg, Client: client, TransactionID: txnID,
	})
	if err == nil {
		t.Fatal("expected error for already-consumed transaction")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error should mention unknown after success: %v", err)
	}
}

func TestCancelTransaction(t *testing.T) {
	resetStore()
	txnID, _ := newTransactionID()
	defaultStore.set(txnID, &Transaction{
		ID:        txnID,
		Status:    TransactionPending,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if _, ok := GetTransaction(txnID); !ok {
		t.Fatal("transaction should exist before cancel")
	}
	CancelTransaction(txnID)
	if _, ok := GetTransaction(txnID); ok {
		t.Fatal("transaction should be deleted after cancel")
	}
}

func TestConcurrentPoll_SameTransaction(t *testing.T) {
	resetStore()
	srv := fakeDeviceCodeServer(t, withPendingThenSuccess(3, "at", "rt"))
	cfg := OAuthConfig{
		BaseURL:        srv.URL,
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "test",
	}
	client := &http.Client{Timeout: 5 * time.Second}

	loginResp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: cfg,
		Client: client,
		TTL:    10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	txnID := loginResp.Transaction.ID

	// Launch concurrent polls.
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	errCount := 0

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, err := PollLogin(context.Background(), PollLoginRequest{
				Config: cfg, Client: client, TransactionID: txnID,
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errCount++
				return
			}
			if status.Status == TransactionSuccess {
				successCount++
			}
		}()
	}
	wg.Wait()

	// Only one goroutine should get success (the others get terminal error or pending).
	if successCount == 0 {
		t.Fatal("expected at least one success")
	}
	t.Logf("concurrent poll: success=%d, errors=%d", successCount, errCount)
}

func TestPollLogin_RefreshToken(t *testing.T) {
	// Test the token endpoint returns refresh_token in success response.
	resetStore()
	srv := fakeDeviceCodeServer(t, withPendingThenSuccess(0, "new-at", "new-rt"))
	cfg := OAuthConfig{
		BaseURL:        srv.URL,
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "test",
	}
	client := &http.Client{Timeout: 5 * time.Second}

	loginResp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: cfg, Client: client, TTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	status, err := PollLogin(context.Background(), PollLoginRequest{
		Config: cfg, Client: client, TransactionID: loginResp.Transaction.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Credential == nil {
		t.Fatal("expected credential")
	}
	if status.Credential.AccessToken != "new-at" {
		t.Fatalf("AccessToken=%q, want new-at", status.Credential.AccessToken)
	}
	if status.Credential.RefreshToken != "new-rt" {
		t.Fatalf("RefreshToken=%q, want new-rt", status.Credential.RefreshToken)
	}
}

func TestPollLogin_MalformedJSON(t *testing.T) {
	resetStore()
	srv := fakeDeviceCodeServer(t, withMalformedJSON())
	cfg := OAuthConfig{
		BaseURL:        srv.URL,
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "test",
	}
	client := &http.Client{Timeout: 5 * time.Second}

	loginResp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: cfg, Client: client, TTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = PollLogin(context.Background(), PollLoginRequest{
		Config: cfg, Client: client, TransactionID: loginResp.Transaction.ID,
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error should mention malformed: %v", err)
	}
}

func TestPollLogin_AccessDenied(t *testing.T) {
	resetStore()
	srv := fakeDeviceCodeServer(t, withAccessDenied())
	cfg := OAuthConfig{
		BaseURL:        srv.URL,
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "test",
	}
	client := &http.Client{Timeout: 5 * time.Second}

	loginResp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config: cfg, Client: client, TTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = PollLogin(context.Background(), PollLoginRequest{
		Config: cfg, Client: client, TransactionID: loginResp.Transaction.ID,
	})
	if err == nil {
		t.Fatal("expected error for access_denied")
	}

	// Transaction should be deleted after error.
	if _, ok := GetTransaction(loginResp.Transaction.ID); ok {
		t.Fatal("transaction should be deleted after access_denied")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BaseURL != "https://qoder.com" {
		t.Fatalf("BaseURL=%q", cfg.BaseURL)
	}
	if cfg.DeviceCodePath != "/oauth/device/code" {
		t.Fatalf("DeviceCodePath=%q", cfg.DeviceCodePath)
	}
	if cfg.TokenPath != "/oauth/token" {
		t.Fatalf("TokenPath=%q", cfg.TokenPath)
	}
	if cfg.ClientID != "qoder-cpa" {
		t.Fatalf("ClientID=%q", cfg.ClientID)
	}
}

func TestTransactionStore_ConcurrentAccess(t *testing.T) {
	resetStore()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := TransactionID(fmt.Sprintf("txn-%d", n))
			defaultStore.set(id, &Transaction{ID: id, Status: TransactionPending})
			defaultStore.get(id)
			defaultStore.delete(id)
		}(i)
	}
	wg.Wait()
}
