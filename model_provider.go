package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type rpcAuthModelRequest struct {
	pluginapi.AuthModelRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

func staticModels() pluginapi.ModelResponse {
	return pluginapi.ModelResponse{Provider: qoderauth.Provider, Models: []pluginapi.ModelInfo{}}
}

func (s authService) models(ctx context.Context, raw []byte) (pluginapi.ModelResponse, error) {
	var req rpcAuthModelRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return pluginapi.ModelResponse{}, err
	}
	if req.AuthProvider != "" && !strings.EqualFold(req.AuthProvider, qoderauth.Provider) {
		return pluginapi.ModelResponse{}, fmt.Errorf("unsupported auth provider %q", req.AuthProvider)
	}
	cred, err := parseStoredCredential(req.StorageJSON)
	if err != nil {
		return pluginapi.ModelResponse{}, err
	}
	client, err := newHostHTTPClient(req.HostCallbackID)
	if err != nil {
		return pluginapi.ModelResponse{}, err
	}
	control, err := qodercontrol.NewClient(client, s.controlConfig)
	if err != nil {
		return pluginapi.ModelResponse{}, err
	}
	upstream, err := control.FetchModels(ctx, cred)
	if err != nil {
		return pluginapi.ModelResponse{}, err
	}
	seen := make(map[string]struct{}, len(upstream))
	models := make([]pluginapi.ModelInfo, 0, len(upstream))
	for _, model := range upstream {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if !strings.HasPrefix(id, qoderauth.Provider+"/") {
			id = qoderauth.Provider + "/" + id
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = strings.TrimPrefix(id, qoderauth.Provider+"/")
		}
		models = append(models, pluginapi.ModelInfo{
			ID: id, Object: "model", OwnedBy: qoderauth.Provider,
			Name: strings.TrimPrefix(id, qoderauth.Provider+"/"), DisplayName: name,
			SupportedGenerationMethods: []string{"chat-completions"},
		})
	}
	return pluginapi.ModelResponse{Provider: qoderauth.Provider, Models: models}, nil
}
