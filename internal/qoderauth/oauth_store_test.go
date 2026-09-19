package qoderauth

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

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
