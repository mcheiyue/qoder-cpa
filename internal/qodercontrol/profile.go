package qodercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// Profile holds user identity from the Qoder control plane.
type Profile struct {
	UserID           string   `json:"user_id"`
	Email            string   `json:"email"`
	Name             string   `json:"name"`
	OrganizationID   string   `json:"organization_id"`
	OrganizationTags []string `json:"organization_tags"`
}

type profileResponse struct {
	UserID           string   `json:"user_id"`
	Email            string   `json:"email"`
	Name             string   `json:"name"`
	OrganizationID   string   `json:"organization_id"`
	OrganizationTags []string `json:"organization_tags"`
}

// FetchProfile queries the Qoder user identity endpoint.
// Returns ErrUpstreamFailed on non-2xx; ErrNonJSONResponse on non-JSON;
// ErrMalformedResponse on decode failure.
func (c *Client) FetchProfile(ctx context.Context, cred qoderauth.Credential) (*Profile, error) {
	body, err := c.doRequest(ctx, http.MethodGet, c.buildURL("/api/v1/userinfo"), cred.AccessToken)
	if err != nil {
		return nil, err
	}
	var pr profileResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, ErrNonJSONResponse
	}
	if pr.UserID == "" {
		return nil, ErrMalformedResponse
	}
	return &Profile{
		UserID:           pr.UserID,
		Email:            pr.Email,
		Name:             pr.Name,
		OrganizationID:   pr.OrganizationID,
		OrganizationTags: append([]string(nil), pr.OrganizationTags...),
	}, nil
}

// ProfileUIDMismatch is returned when the upstream profile user_id
// does not match the credential user_id.
type ProfileUIDMismatch struct {
	Expected string
	Got      string
}

func (e *ProfileUIDMismatch) Error() string {
	return fmt.Sprintf("qodercontrol: profile UID mismatch: expected %s, got %s", e.Expected, e.Got)
}

func (e *ProfileUIDMismatch) Unwrap() error { return ErrMalformedResponse }

// FetchProfileValidated queries profile and validates the UID matches the credential.
func (c *Client) FetchProfileValidated(ctx context.Context, cred qoderauth.Credential) (*Profile, error) {
	p, err := c.FetchProfile(ctx, cred)
	if err != nil {
		return nil, err
	}
	if cred.UserID != "" && p.UserID != cred.UserID {
		return nil, &ProfileUIDMismatch{Expected: cred.UserID, Got: p.UserID}
	}
	return p, nil
}

// IsProfileUIDMismatch checks if an error is a ProfileUIDMismatch.
func IsProfileUIDMismatch(err error) bool {
	var mismatch *ProfileUIDMismatch
	return errors.As(err, &mismatch)
}
