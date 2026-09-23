package qodercontrol

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
)

// Model describes a model in the Qoder catalog.
type Model struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	IsReasoning    bool   `json:"is_reasoning"`
	MaxInputTokens int    `json:"max_input_tokens"`
}

func (m *Model) UnmarshalJSON(raw []byte) error {
	var wire struct {
		ID               string `json:"id"`
		Key              string `json:"key"`
		Name             string `json:"name"`
		DisplayName      string `json:"display_name"`
		DisplayNameCamel string `json:"displayName"`
		IsReasoning      bool   `json:"is_reasoning"`
		MaxInputTokens   int    `json:"max_input_tokens"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	m.ID = firstString(wire.ID, wire.Key)
	m.Name = modelDisplayName(m.ID, wire.DisplayName, wire.DisplayNameCamel, wire.Name)
	m.IsReasoning = wire.IsReasoning
	m.MaxInputTokens = wire.MaxInputTokens
	return nil
}

type modelsResponse struct {
	Data []Model `json:"data"`
}

// FetchModels retrieves the dynamic model catalog from Qoder.
// Returns an empty slice on empty response (no fake models).
// Returns ErrModelUnavailable on upstream failure.
func (c *Client) FetchModels(ctx context.Context, cred qoderauth.Credential) ([]Model, error) {
	if cred.RuntimeInfo != "" && cred.RuntimeKey != "" {
		return c.fetchSignedModels(ctx, cred)
	}
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

func (c *Client) fetchSignedModels(ctx context.Context, cred qoderauth.Credential) ([]Model, error) {
	ep := cosy.EndpointAPI2
	if cred.Profile == qoderauth.TransportProfileCosyAPI3 {
		ep = cosy.EndpointAPI3
	}
	requestID := make([]byte, 16)
	if _, err := rand.Read(requestID); err != nil {
		return nil, ErrModelUnavailable
	}
	parts, err := cosy.BuildCatalogRequestAt(ep, cosy.RuntimeFields{
		EncryptUserInfo: cred.RuntimeInfo,
		Key:             cred.RuntimeKey,
	}, "qoder-"+hex.EncodeToString(requestID), "1.1.34", time.Now())
	if err != nil {
		return nil, ErrModelUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parts.URL, nil)
	if err != nil {
		return nil, ErrModelUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", parts.Authorization)
	req.Header.Set("Cosy-Business-Product", "cli")
	req.Header.Set("Cosy-Business-Type", "agent")
	req.Header.Set("Cosy-ClientType", "5")
	req.Header.Set("Cosy-Data-Policy", "agree")
	req.Header.Set("Cosy-Date", parts.Date)
	req.Header.Set("Cosy-Key", parts.CosyKey)
	req.Header.Set("Cosy-Scene", "assistant")
	req.Header.Set("Cosy-User", cred.UserID)
	if cred.OrganizationID != "" {
		req.Header.Set("Cosy-Organization-Id", cred.OrganizationID)
	}
	if len(cred.OrganizationTags) > 0 {
		req.Header.Set("Cosy-Organization-Tags", strings.Join(cred.OrganizationTags, ","))
	}
	req.Header.Set("Cosy-Version", parts.CosyVersion)
	req.Header.Set("Login-Version", "v2")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrModelUnavailable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.config.BodyLimit+1))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 || int64(len(body)) > c.config.BodyLimit {
		return nil, ErrModelUnavailable
	}
	models, err := parseCatalog(body)
	if err != nil || len(models) == 0 {
		return nil, ErrModelUnavailable
	}
	return models, nil
}

func parseCatalog(raw []byte) ([]Model, error) {
	var value any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &value); err != nil {
		return nil, err
	}
	return collectModels(value), nil
}

func collectModels(value any) []Model {
	seen := map[string]struct{}{}
	models := make([]Model, 0)
	var visit func(any)
	visit = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, item := range typed {
				visit(item)
			}
		case map[string]any:
			id := strings.TrimSpace(stringValue(typed["key"]))
			if id == "" {
				id = strings.TrimSpace(stringValue(typed["id"]))
			}
			if id != "" {
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					models = append(models, Model{
						ID:             id,
						Name:           modelDisplayName(id, stringValue(typed["display_name"]), stringValue(typed["displayName"]), stringValue(typed["name"])),
						IsReasoning:    boolValue(typed["is_reasoning"]),
						MaxInputTokens: intValue(typed["max_input_tokens"]),
					})
				}
			}
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return models
}

func modelDisplayName(id string, values ...string) string {
	id = strings.TrimSpace(id)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !strings.EqualFold(value, id) {
			return value
		}
	}
	return id
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func firstString(values ...any) string {
	for _, value := range values {
		if s := strings.TrimSpace(stringValue(value)); s != "" {
			return s
		}
	}
	return ""
}

func boolValue(value any) bool {
	b, _ := value.(bool)
	return b
}

func intValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	}
	return 0
}
