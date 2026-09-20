package qoderauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthConfig holds the Qoder Device OAuth endpoint contracts.
// Production URLs are HTTPS+allowlisted; loopback allowed only in tests.
type OAuthConfig struct {
	BaseURL    string
	APIBaseURL string
	DevicePath string
	PollPath   string
	ClientID   string
}

// DefaultConfig returns the official Qoder Device OAuth endpoints.
func DefaultConfig() OAuthConfig {
	return OAuthConfig{
		BaseURL:    "https://qoder.com",
		APIBaseURL: "https://openapi.qoder.sh",
		DevicePath: "/device/selectAccounts",
		PollPath:   "/api/v1/deviceToken/poll",
		ClientID:   "qoder-cpa",
	}
}

// DeviceLoginRequest contains the parameters needed to initiate a device login.
type DeviceLoginRequest struct {
	Config    OAuthConfig
	Client    *http.Client
	MachineID MachineID
	TTL       time.Duration // transaction lifetime; 0 defaults to 15 min
}

// DeviceLoginResponse is returned after successfully initiating device login.
type DeviceLoginResponse struct {
	VerifyURL   string
	DeviceCode  string
	Transaction *Transaction
	ExpiresAt   time.Time
}

// PollLoginRequest contains the parameters for polling a login transaction.
type PollLoginRequest struct {
	Config        OAuthConfig
	Client        *http.Client
	TransactionID TransactionID
}

// PollStatus is the result of a single poll.
type PollStatus struct {
	Status     TransactionStatus
	Credential *Credential
	Message    string
}

type tokenResponse struct {
	AccessToken  string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	ExpiresAt    string `json:"expires_at"`
	ExpiresIn    int    `json:"expires_in"`
}

type tokenPendingResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// DeviceLogin initiates the Qoder Device OAuth flow.
func DeviceLogin(ctx context.Context, req DeviceLoginRequest) (*DeviceLoginResponse, error) {
	_ = ctx
	if req.Config.BaseURL == "" {
		return nil, errors.New("qoderauth: empty base URL")
	}
	base, err := url.Parse(req.Config.BaseURL)
	if err != nil || base.Hostname() == "" || (base.Scheme != "https" && !isLoopback(base.Hostname())) {
		return nil, ErrInvalidHost
	}

	verifier, err := newPKCEVerifier()
	if err != nil {
		return nil, err
	}
	nonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	machineID := req.MachineID
	if machineID == "" {
		machineID, err = newMachineID()
		if err != nil {
			return nil, err
		}
	}

	challenge := pkceChallenge(verifier)
	ttl := req.TTL
	if ttl == 0 {
		ttl = 15 * time.Minute
	}

	verifyURL, err := url.Parse(strings.TrimRight(req.Config.BaseURL, "/") + req.Config.DevicePath)
	if err != nil {
		return nil, fmt.Errorf("qoderauth: building device URL: %w", err)
	}
	query := verifyURL.Query()
	query.Set("challenge", challenge)
	query.Set("challenge_method", "S256")
	query.Set("machine_id", string(machineID))
	query.Set("nonce", nonce)
	query.Set("directLogin", "false")
	verifyURL.RawQuery = query.Encode()

	txnID, err := newTransactionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	txn := &Transaction{
		ID:        txnID,
		Verifier:  verifier,
		Nonce:     nonce,
		Machine:   machineID,
		VerifyURL: verifyURL.String(),
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
		Status:    TransactionPending,
	}

	defaultStore.set(txnID, txn)

	return &DeviceLoginResponse{
		VerifyURL:   verifyURL.String(),
		Transaction: txn,
		ExpiresAt:   txn.ExpiresAt,
	}, nil
}

// PollLogin polls the Qoder token endpoint for the device login result.
func PollLogin(ctx context.Context, req PollLoginRequest) (*PollStatus, error) {
	txn, ok := defaultStore.get(req.TransactionID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTransactionUnknown, req.TransactionID)
	}

	txn.mu.Lock()
	defer txn.mu.Unlock()

	if txn.Status == TransactionSuccess || txn.Status == TransactionError {
		if txn.Status == TransactionSuccess && txn.Credential != nil {
			return &PollStatus{Status: TransactionSuccess, Credential: txn.Credential}, nil
		}
		return nil, fmt.Errorf("%w: %s", ErrTransactionTerminal, txn.Status)
	}

	if time.Now().After(txn.ExpiresAt) {
		txn.Status = TransactionError
		txn.ErrMessage = "transaction expired"
		defaultStore.delete(req.TransactionID)
		return nil, ErrTransactionExpired
	}

	pollURL, err := url.Parse(strings.TrimRight(req.Config.APIBaseURL, "/") + req.Config.PollPath)
	if err != nil {
		return nil, fmt.Errorf("qoderauth: building poll URL: %w", err)
	}
	query := pollURL.Query()
	query.Set("nonce", txn.Nonce)
	query.Set("verifier", txn.Verifier)
	query.Set("challenge_method", "S256")
	pollURL.RawQuery = query.Encode()
	respBody, statusCode, err := doPollRequest(ctx, req.Client, pollURL.String())
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound || statusCode == http.StatusAccepted {
		return &PollStatus{Status: TransactionPending}, nil
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpstreamFailed, statusCode)
	}

	var tokResp tokenResponse
	if err := json.Unmarshal(respBody, &tokResp); err == nil && tokResp.AccessToken != "" {
		expiresAt := time.Now().Add(time.Duration(tokResp.ExpiresIn) * time.Second)
		if tokResp.ExpiresAt != "" {
			if parsed, parseErr := time.Parse(time.RFC3339, tokResp.ExpiresAt); parseErr == nil {
				expiresAt = parsed
			}
		}
		cred := &Credential{
			AccessToken:  tokResp.AccessToken,
			RefreshToken: tokResp.RefreshToken,
			ExpiresAt:    expiresAt,
			UserID:       tokResp.UserID,
		}
		txn.Status = TransactionSuccess
		txn.Credential = cred
		defaultStore.delete(req.TransactionID)
		return &PollStatus{Status: TransactionSuccess, Credential: cred}, nil
	}

	var pendResp tokenPendingResponse
	if err := json.Unmarshal(respBody, &pendResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}

	switch pendResp.Error {
	case "authorization_pending":
		return &PollStatus{Status: TransactionPending, Message: pendResp.ErrorDescription}, nil
	case "slow_down":
		return &PollStatus{Status: TransactionPending, Message: pendResp.ErrorDescription}, nil
	case "expired_token":
		txn.Status = TransactionError
		txn.ErrMessage = pendResp.ErrorDescription
		defaultStore.delete(req.TransactionID)
		return nil, fmt.Errorf("%w: %s", ErrUpstreamFailed, pendResp.ErrorDescription)
	case "access_denied":
		txn.Status = TransactionError
		txn.ErrMessage = pendResp.ErrorDescription
		defaultStore.delete(req.TransactionID)
		return nil, fmt.Errorf("%w: %s", ErrUpstreamFailed, pendResp.ErrorDescription)
	default:
		txn.Status = TransactionError
		txn.ErrMessage = pendResp.Error
		defaultStore.delete(req.TransactionID)
		return nil, fmt.Errorf("%w: %s", ErrUpstreamFailed, pendResp.Error)
	}
}

func doPollRequest(ctx context.Context, client *http.Client, endpoint string) ([]byte, int, error) {
	if client == nil {
		return nil, 0, errors.New("qoderauth: nil HTTP client")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("qoderauth: building poll request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrUpstreamFailed, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if len(body) > maxResponseBodyBytes {
		return nil, resp.StatusCode, ErrResponseTooLarge
	}
	return body, resp.StatusCode, nil
}
