package qoderauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RefreshConfig holds the endpoint for token refresh.
type RefreshConfig struct {
	// TokenURL is the token endpoint URL for refresh grants.
	TokenURL string
	// ClientID is the OAuth client ID.
	ClientID string
}

// RefreshRequest contains the parameters for refreshing a credential.
type RefreshRequest struct {
	Config RefreshConfig
	Client *http.Client
	Cred   Credential
	TTL    time.Duration // next refresh interval; 0 defaults to access token lifetime
}

// RefreshResponse is the result of a successful token refresh.
type RefreshResponse struct {
	Credential       Credential
	NextRefreshAfter time.Time
}

// tokenRefreshResponse is the upstream JSON for a token refresh.
type tokenRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// RefreshError indicates the refresh failed and re-login is needed.
type RefreshError struct {
	Reason string
}

func (e *RefreshError) Error() string {
	return fmt.Sprintf("qoderauth: refresh failed: %s", e.Reason)
}

func (e *RefreshError) Unwrap() error {
	return ErrMissingRefreshToken
}

// refreshSingleflight prevents concurrent refreshes for the same auth ID.
type refreshSingleflight struct {
	mu       sync.Mutex
	inflight map[string]*refreshResult
}

type refreshResult struct {
	done chan struct{}
	resp *RefreshResponse
	err  error
}

var sf = &refreshSingleflight{inflight: make(map[string]*refreshResult)}

func (s *refreshSingleflight) do(authID string, fn func() (*RefreshResponse, error)) (*RefreshResponse, error) {
	s.mu.Lock()
	if r, ok := s.inflight[authID]; ok {
		s.mu.Unlock()
		<-r.done
		return r.resp, r.err
	}
	r := &refreshResult{done: make(chan struct{})}
	s.inflight[authID] = r
	s.mu.Unlock()

	defer func() {
		close(r.done)
		s.mu.Lock()
		delete(s.inflight, authID)
		s.mu.Unlock()
	}()

	resp, err := fn()
	r.resp = resp
	r.err = err
	return resp, err
}

// Refresh performs a token refresh using the stored refresh token.
// It implements refresh token rotation: if the upstream returns a new
// refresh token, it replaces the old one; if not, the old token is kept.
// If the refresh token is revoked or missing, it returns an error that
// wraps ErrMissingRefreshToken.
func Refresh(ctx context.Context, req RefreshRequest) (*RefreshResponse, error) {
	if req.Client == nil {
		return nil, errors.New("qoderauth: nil HTTP client")
	}
	if req.Config.TokenURL == "" {
		return nil, errors.New("qoderauth: empty token URL")
	}
	if req.Cred.RefreshToken == "" {
		return nil, &RefreshError{Reason: "no refresh token"}
	}
	if req.Config.ClientID == "" {
		return nil, errors.New("qoderauth: empty client ID")
	}

	return sf.do(string(newAuthID(req.Cred.UserID)), func() (*RefreshResponse, error) {
		return doRefresh(ctx, req)
	})
}

func doRefresh(ctx context.Context, req RefreshRequest) (*RefreshResponse, error) {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {req.Cred.RefreshToken},
		"client_id":     {req.Config.ClientID},
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Config.TokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("qoderauth: building refresh request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	respBody, err := doHTTPRequest(ctx, req.Client, httpReq)
	if err != nil {
		return nil, err
	}

	var tokResp tokenRefreshResponse
	if err := json.Unmarshal(respBody, &tokResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}

	if tokResp.AccessToken == "" {
		// Try error response.
		var errResp tokenPendingResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			switch errResp.Error {
			case "invalid_grant", "expired_token":
				return nil, &RefreshError{Reason: errResp.ErrorDescription}
			}
		}
		return nil, &RefreshError{Reason: "no access token in refresh response"}
	}

	// Refresh token rotation: use new token if present, else keep old.
	refreshToken := req.Cred.RefreshToken
	if tokResp.RefreshToken != "" {
		refreshToken = tokResp.RefreshToken
	}

	now := time.Now()
	ttl := req.TTL
	if ttl == 0 {
		ttl = time.Duration(tokResp.ExpiresIn) * time.Second
	}

	return &RefreshResponse{
		Credential: Credential{
			AccessToken:  tokResp.AccessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    now.Add(time.Duration(tokResp.ExpiresIn) * time.Second),
			UserID:       req.Cred.UserID,
			Email:        req.Cred.Email,
			Profile:      req.Cred.Profile,
		},
		NextRefreshAfter: now.Add(ttl),
	}, nil
}
