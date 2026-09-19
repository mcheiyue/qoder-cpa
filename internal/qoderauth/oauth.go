package qoderauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthConfig holds the Qoder Device OAuth endpoint contracts.
// Production URLs are HTTPS+allowlisted; loopback allowed only in tests.
type OAuthConfig struct {
	BaseURL        string
	DeviceCodePath string
	TokenPath      string
	ClientID       string
}

// DefaultConfig returns the official Qoder Device OAuth endpoints.
func DefaultConfig() OAuthConfig {
	return OAuthConfig{
		BaseURL:        "https://qoder.com",
		DeviceCodePath: "/oauth/device/code",
		TokenPath:      "/oauth/token",
		ClientID:       "qoder-cpa",
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

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
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
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type tokenPendingResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// DeviceLogin initiates the Qoder Device OAuth flow.
func DeviceLogin(ctx context.Context, req DeviceLoginRequest) (*DeviceLoginResponse, error) {
	if req.Client == nil {
		return nil, errors.New("qoderauth: nil HTTP client")
	}
	if req.Config.BaseURL == "" {
		return nil, errors.New("qoderauth: empty base URL")
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

	body := url.Values{
		"client_id":             {req.Config.ClientID},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {nonce},
		"machine_id":            {string(machineID)},
	}

	tokenURL := req.Config.BaseURL + req.Config.DeviceCodePath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("qoderauth: building device code request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	respBody, err := doHTTPRequest(ctx, req.Client, httpReq)
	if err != nil {
		return nil, err
	}

	var dcResp deviceCodeResponse
	if err := json.Unmarshal(respBody, &dcResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}

	txnID, err := newTransactionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	txn := &Transaction{
		ID:         txnID,
		Verifier:   verifier,
		Nonce:      nonce,
		Machine:    machineID,
		DeviceCode: dcResp.DeviceCode,
		VerifyURL:  dcResp.VerificationURI,
		ExpiresAt:  now.Add(ttl),
		CreatedAt:  now,
		Status:     TransactionPending,
	}

	defaultStore.set(txnID, txn)

	return &DeviceLoginResponse{
		VerifyURL:   dcResp.VerificationURI,
		DeviceCode:  dcResp.DeviceCode,
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

	body := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {txn.DeviceCode},
		"client_id":   {req.Config.ClientID},
	}

	tokenURL := req.Config.BaseURL + req.Config.TokenPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("qoderauth: building token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	respBody, err := doHTTPRequest(ctx, req.Client, httpReq)
	if err != nil {
		return nil, err
	}

	var tokResp tokenResponse
	if err := json.Unmarshal(respBody, &tokResp); err == nil && tokResp.AccessToken != "" {
		cred := &Credential{
			AccessToken:  tokResp.AccessToken,
			RefreshToken: tokResp.RefreshToken,
			ExpiresAt:    time.Now().Add(time.Duration(tokResp.ExpiresIn) * time.Second),
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
