package qoderauth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

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
