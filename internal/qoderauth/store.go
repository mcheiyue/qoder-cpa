package qoderauth

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// transactionStore manages in-memory transactions.
type transactionStore struct {
	mu           sync.RWMutex
	transactions map[TransactionID]*Transaction
}

func newTransactionStore() *transactionStore {
	return &transactionStore{
		transactions: make(map[TransactionID]*Transaction),
	}
}

func (s *transactionStore) get(id TransactionID) (*Transaction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.transactions[id]
	return t, ok
}

func (s *transactionStore) set(id TransactionID, t *Transaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transactions[id] = t
}

func (s *transactionStore) delete(id TransactionID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.transactions, id)
}

// defaultStore is the package-level transaction store.
var defaultStore = newTransactionStore()

// newTransactionID generates a cryptographically random transaction ID.
func newTransactionID() (TransactionID, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("qoderauth: generating transaction ID: %w", err)
	}
	return TransactionID("txn-" + hexEncode(b)), nil
}

// validateVerifyHost checks that the VerifyURL uses an allowed host.
// Production requires HTTPS + explicit allowlist; tests may use loopback.
func validateVerifyHost(rawURL string, allowInsecure bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: parse error: %v", ErrInvalidHost, err)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: empty host", ErrInvalidHost)
	}
	if u.Scheme != "https" {
		if !allowInsecure || !isLoopback(host) {
			return fmt.Errorf("%w: scheme %q, need https", ErrInvalidHost, u.Scheme)
		}
	}
	if !isLoopback(host) && !strings.HasSuffix(host, "qoder.com") && !strings.HasSuffix(host, "qoder.sh") {
		return fmt.Errorf("%w: host %q not in allowlist", ErrInvalidHost, host)
	}
	return nil
}

// isLoopback reports whether host is 127.0.0.1, ::1, or localhost.
func isLoopback(host string) bool {
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// CancelTransaction removes a pending transaction without consuming it.
func CancelTransaction(txnID TransactionID) {
	defaultStore.delete(txnID)
}

// GetTransaction returns a read-only view of the transaction for testing.
func GetTransaction(txnID TransactionID) (*Transaction, bool) {
	return defaultStore.get(txnID)
}

// resetStore replaces the default store (for tests).
func resetStore() {
	defaultStore = newTransactionStore()
}
