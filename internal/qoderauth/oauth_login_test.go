package qoderauth

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestDeviceLogin_Success(t *testing.T) {
	resetStore()
	cfg := OAuthConfig{
		BaseURL:    "https://qoder.com",
		DevicePath: "/device/selectAccounts",
		ClientID:   "test-client",
	}

	resp, err := DeviceLogin(context.Background(), DeviceLoginRequest{
		Config:    cfg,
		MachineID: "m-test-machine",
		TTL:       10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("DeviceLogin: %v", err)
	}

	// Verify response fields.
	verifyURL, err := url.Parse(resp.VerifyURL)
	if err != nil || verifyURL.Path != "/device/selectAccounts" {
		t.Fatalf("VerifyURL=%q", resp.VerifyURL)
	}
	if verifyURL.Query().Get("challenge") == "" || verifyURL.Query().Get("nonce") == "" || verifyURL.Query().Get("machine_id") != "m-test-machine" {
		t.Fatalf("VerifyURL query=%s", verifyURL.RawQuery)
	}
	if verifyURL.Query().Get("directLogin") != "false" {
		t.Fatalf("VerifyURL directLogin=%q, want false so an existing Qoder session can switch accounts", verifyURL.Query().Get("directLogin"))
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
	if txn.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt is in the past")
	}

	// Verify transaction is stored.
	if _, ok := GetTransaction(txn.ID); !ok {
		t.Fatal("transaction not stored")
	}
}

func TestDeviceLogin_NilClient(t *testing.T) {
	resp, err := DeviceLogin(context.Background(), DeviceLoginRequest{Config: DefaultConfig()})
	if err != nil || resp.VerifyURL == "" {
		t.Fatalf("DeviceLogin: response=%#v error=%v", resp, err)
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
		Config: OAuthConfig{BaseURL: "http://qoder.com", DevicePath: "/device/selectAccounts"},
	})
	if err == nil {
		t.Fatal("expected error for non-HTTPS URL")
	}
}
