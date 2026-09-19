package qodercontrol

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// Quota holds account quota/subscription information.
type Quota struct {
	Plan      string `json:"plan"`
	Remaining int64  `json:"remaining"`
	Limit     int64  `json:"limit"`
}

type quotaResponse struct {
	Plan      string `json:"plan"`
	Remaining int64  `json:"remaining"`
	Limit     int64  `json:"limit"`
}

// FetchQuota retrieves quota/subscription info from Qoder.
// Returns ErrQuotaUnknown on upstream failure; does NOT invalidate credentials.
func (c *Client) FetchQuota(ctx context.Context, cred qoderauth.Credential) (*Quota, error) {
	body, err := c.doRequest(ctx, http.MethodGet, c.buildURL("/v1/quota"), cred.AccessToken)
	if err != nil {
		if _, ok := err.(*UpstreamError); ok {
			return nil, ErrQuotaUnknown
		}
		return nil, ErrQuotaUnknown
	}
	if len(body) == 0 {
		return nil, ErrQuotaUnknown
	}
	var qr quotaResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, ErrQuotaUnknown
	}
	return &Quota{
		Plan:      qr.Plan,
		Remaining: qr.Remaining,
		Limit:     qr.Limit,
	}, nil
}
