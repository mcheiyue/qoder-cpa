package qodercontrol

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// RuntimeFields holds COSY authorization parameters.
type RuntimeFields struct {
	Authorization string `json:"authorization"`
	SessionID     string `json:"session_id"`
	UID           string `json:"uid"`
	Token         string `json:"token"`
}

type runtimeResponse struct {
	Authorization string `json:"authorization"`
	SessionID     string `json:"session_id"`
	UID           string `json:"uid"`
	Token         string `json:"token"`
}

// FetchRuntimeFields retrieves COSY runtime fields from Qoder.
// Returns ErrUpstreamFailed on non-2xx, ErrNonJSONResponse on non-JSON,
// ErrMalformedResponse on missing required fields.
func (c *Client) FetchRuntimeFields(ctx context.Context, cred qoderauth.Credential) (*RuntimeFields, error) {
	body, err := c.doRequest(ctx, http.MethodGet, c.buildURL("/v1/cosy/runtime"), cred.AccessToken)
	if err != nil {
		return nil, err
	}
	var rr runtimeResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, ErrNonJSONResponse
	}
	if rr.Authorization == "" {
		return nil, ErrMalformedResponse
	}
	return &RuntimeFields{
		Authorization: rr.Authorization,
		SessionID:     rr.SessionID,
		UID:           rr.UID,
		Token:         rr.Token,
	}, nil
}

// RefreshRuntimeFields fetches fresh runtime fields; caller uses this
// to renew COSY authorization without re-authenticating.
func (c *Client) RefreshRuntimeFields(ctx context.Context, cred qoderauth.Credential) (*RuntimeFields, error) {
	return c.FetchRuntimeFields(ctx, cred)
}
