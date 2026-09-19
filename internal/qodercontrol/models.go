package qodercontrol

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// Model describes a model in the Qoder catalog.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type modelsResponse struct {
	Data []Model `json:"data"`
}

// FetchModels retrieves the dynamic model catalog from Qoder.
// Returns an empty slice on empty response (no fake models).
// Returns ErrModelUnavailable on upstream failure.
func (c *Client) FetchModels(ctx context.Context, cred qoderauth.Credential) ([]Model, error) {
	body, err := c.doRequest(ctx, http.MethodGet, c.buildURL("/v1/models"), cred.AccessToken)
	if err != nil {
		if _, ok := err.(*UpstreamError); ok {
			return nil, ErrModelUnavailable
		}
		return nil, ErrModelUnavailable
	}
	if len(body) == 0 {
		return nil, nil
	}
	var mr modelsResponse
	if err := json.Unmarshal(body, &mr); err != nil {
		return nil, ErrModelUnavailable
	}
	return mr.Data, nil
}
