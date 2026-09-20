package qoderauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Provider is the stable provider key for Qoder credentials.
const Provider = "qoder"

// AuthID is a stable, deterministic identifier for a Qoder credential,
// formatted as "qoder-<hex>" where hex is the SHA-256 of the user ID.
type AuthID string

// TransactionID is an opaque identifier for a device login transaction.
type TransactionID string

// MachineID is a per-device identifier sent during device OAuth.
type MachineID string

// TransportProfile selects the chat transport adapter.
type TransportProfile string

const (
	TransportProfileCosyAPI2     TransportProfile = "cosy-api2"
	TransportProfileCosyAPI3     TransportProfile = "cosy-api3"
	TransportProfileBearerOpenAI TransportProfile = "bearer-openai"
)

// Credential holds the persistent state for an authenticated Qoder account.
type Credential struct {
	AccessToken      string
	RefreshToken     string
	ExpiresAt        time.Time
	UserID           string
	Email            string
	OrganizationID   string
	OrganizationTags []string
	RuntimeInfo      string
	RuntimeKey       string
	Profile          TransportProfile
}

// StorageJSON is the JSON form persisted by CPA auth store.
// Only Credential fields and Profile are serialized; verifier/nonce/device
// state are never included.
type StorageJSON struct {
	AccessToken      string           `json:"access_token"`
	RefreshToken     string           `json:"refresh_token"`
	ExpiresAt        time.Time        `json:"expires_at"`
	UserID           string           `json:"user_id"`
	Email            string           `json:"email"`
	OrganizationID   string           `json:"organization_id,omitempty"`
	OrganizationTags []string         `json:"organization_tags,omitempty"`
	RuntimeInfo      string           `json:"runtime_info,omitempty"`
	RuntimeKey       string           `json:"runtime_key,omitempty"`
	Profile          TransportProfile `json:"transport_profile"`
}

// Transaction is the in-memory state for a device login polling session.
// It is never serialized.
type Transaction struct {
	ID         TransactionID
	Verifier   string
	Nonce      string
	Machine    MachineID
	DeviceCode string
	VerifyURL  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	Status     TransactionStatus
	Credential *Credential
	ErrMessage string
	mu         sync.Mutex
}

// TransactionStatus tracks the lifecycle of a device login transaction.
type TransactionStatus string

const (
	TransactionPending TransactionStatus = "pending"
	TransactionSuccess TransactionStatus = "success"
	TransactionError   TransactionStatus = "error"
)

// Domain errors returned by this package.
var (
	ErrTransactionExpired  = errors.New("qoderauth: transaction expired")
	ErrTransactionUnknown  = errors.New("qoderauth: transaction unknown")
	ErrTransactionTerminal = errors.New("qoderauth: transaction already terminal")
	ErrInvalidHost         = errors.New("qoderauth: invalid verify URL host")
	ErrMissingRefreshToken = errors.New("qoderauth: refresh token revoked, re-login required")
	ErrUpstreamFailed      = errors.New("qoderauth: upstream OAuth failed")
	ErrMalformedResponse   = errors.New("qoderauth: malformed upstream response")
	ErrResponseTooLarge    = errors.New("qoderauth: response exceeds size limit")
)

// maxResponseBodyBytes is the upper bound for upstream response bodies.
const maxResponseBodyBytes = 1 << 20 // 1 MiB

// newAuthID computes "qoder-<hex>" from a user ID string.
func newAuthID(userID string) AuthID {
	h := sha256.Sum256([]byte(userID))
	return AuthID("qoder-" + hexEncode(h[:]))
}

// AuthIDForUser returns the stable CPA auth identifier for a Qoder user.
func AuthIDForUser(userID string) AuthID { return newAuthID(userID) }

// hexEncode returns a lowercase hex string.
func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	s := make([]byte, len(b)*2)
	for i, v := range b {
		s[i*2] = hex[v>>4]
		s[i*2+1] = hex[v&0x0f]
	}
	return string(s)
}

// newPKCEVerifier returns a cryptographically random 43-byte base64url verifier.
func newPKCEVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("qoderauth: generating PKCE verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// pkceChallenge returns the S256 code_challenge for a verifier.
func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// newNonce returns a 16-byte random nonce for CSRF protection.
func newNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("qoderauth: generating nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// newMachineID generates a deterministic per-device machine identifier.
func newMachineID() (MachineID, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("qoderauth: generating machine ID: %w", err)
	}
	return MachineID("m-" + hexEncode(b)), nil
}

// FromCredential constructs a StorageJSON from a Credential.
func FromCredential(c Credential) StorageJSON {
	return StorageJSON{
		AccessToken:      c.AccessToken,
		RefreshToken:     c.RefreshToken,
		ExpiresAt:        c.ExpiresAt,
		UserID:           c.UserID,
		Email:            c.Email,
		OrganizationID:   c.OrganizationID,
		OrganizationTags: append([]string(nil), c.OrganizationTags...),
		RuntimeInfo:      c.RuntimeInfo,
		RuntimeKey:       c.RuntimeKey,
		Profile:          c.Profile,
	}
}

// ToCredential converts StorageJSON back to a Credential.
func (s StorageJSON) ToCredential() Credential {
	return Credential{
		AccessToken:      s.AccessToken,
		RefreshToken:     s.RefreshToken,
		ExpiresAt:        s.ExpiresAt,
		UserID:           s.UserID,
		Email:            s.Email,
		OrganizationID:   s.OrganizationID,
		OrganizationTags: append([]string(nil), s.OrganizationTags...),
		RuntimeInfo:      s.RuntimeInfo,
		RuntimeKey:       s.RuntimeKey,
		Profile:          s.Profile,
	}
}

// Validate checks that a Credential has the minimum required fields.
func (c Credential) Validate() error {
	if c.AccessToken == "" {
		return errors.New("qoderauth: empty access token")
	}
	if c.UserID == "" {
		return errors.New("qoderauth: empty user ID")
	}
	return nil
}

// IsValidProfile reports whether p is a recognized transport profile.
func IsValidProfile(p TransportProfile) bool {
	switch p {
	case TransportProfileCosyAPI2, TransportProfileCosyAPI3, TransportProfileBearerOpenAI:
		return true
	default:
		return false
	}
}

// drainAndClose reads up to maxResponseBodyBytes from r and closes it.
func drainAndClose(r io.ReadCloser) {
	if r == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, r, maxResponseBodyBytes)
	_ = r.Close()
}

// doHTTPRequest executes req using client, returning body bytes.
// It enforces the response size limit and drains/closes the body.
func doHTTPRequest(ctx context.Context, client *http.Client, req *http.Request) ([]byte, error) {
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamFailed, err)
	}
	defer drainAndClose(resp.Body)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if int64(len(body)) > maxResponseBodyBytes {
		return nil, ErrResponseTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpstreamFailed, resp.StatusCode)
	}
	return body, nil
}
