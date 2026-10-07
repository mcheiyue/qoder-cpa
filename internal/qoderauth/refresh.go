package qoderauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	Token        string `json:"token"`
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

	resp, err := sf.do(string(AuthIDForProfile(req.Cred.UserID, req.Cred.Profile)), func() (*RefreshResponse, error) {
		return doRefresh(ctx, req)
	})
	if err != nil {
		return nil, sanitizeRefreshError(err, req.Cred)
	}
	return resp, nil
}

// sanitizeRefreshError scrubs credential material from upstream-provided
// error text (e.g. error_description echoing tokens) while preserving the
// *RefreshError type and its Unwrap chain (ErrMissingRefreshToken).
func sanitizeRefreshError(err error, cred Credential) error {
	var rerr *RefreshError
	if !errors.As(err, &rerr) {
		return err
	}
	reason := scrubSecrets(rerr.Reason, cred)
	if reason == rerr.Reason {
		return err
	}
	return &RefreshError{Reason: reason}
}

// scrubSecrets replaces credential secrets longer than 4 chars with "***".
// Short/empty values are skipped to avoid mangling unrelated text.
func scrubSecrets(s string, cred Credential) string {
	for _, secret := range []string{cred.AccessToken, cred.RefreshToken, cred.RuntimeKey} {
		if len(secret) > 4 {
			s = strings.ReplaceAll(s, secret, "***")
		}
	}
	return s
}

func doRefresh(ctx context.Context, req RefreshRequest) (*RefreshResponse, error) {
	body, err := json.Marshal(map[string]string{"refresh_token": req.Cred.RefreshToken})
	if err != nil {
		return nil, fmt.Errorf("qoderauth: encoding refresh request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Config.TokenURL,
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("qoderauth: building refresh request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, err := doHTTPRequest(ctx, req.Client, httpReq)
	if err != nil {
		return nil, err
	}

	var tokResp tokenRefreshResponse
	if err := json.Unmarshal(respBody, &tokResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}

	accessToken := tokResp.Token
	if accessToken == "" {
		accessToken = tokResp.AccessToken
	}
	if accessToken == "" {
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
			AccessToken:      accessToken,
			RefreshToken:     refreshToken,
			ExpiresAt:        now.Add(time.Duration(tokResp.ExpiresIn) * time.Second),
			UserID:           req.Cred.UserID,
			MachineID:        req.Cred.MachineID,
			Email:            req.Cred.Email,
			OrganizationID:   req.Cred.OrganizationID,
			OrganizationTags: append([]string(nil), req.Cred.OrganizationTags...),
			RuntimeInfo:      req.Cred.RuntimeInfo,
			RuntimeKey:       req.Cred.RuntimeKey,
			Profile:          req.Cred.Profile,
		},
		NextRefreshAfter: now.Add(ttl),
	}, nil
}
