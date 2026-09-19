package qoderauth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

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
