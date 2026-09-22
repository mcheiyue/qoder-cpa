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
	authID := strings.TrimSpace(req.AuthID)
	if authID == "" {
		authID = string(qoderauth.AuthIDForUser(cred.UserID))
	}
	mapping := make(map[string]string, len(upstream))
	seenInternal := make(map[string]struct{}, len(upstream))
	models := make([]pluginapi.ModelInfo, 0, len(upstream))
	for _, model := range upstream {
		internalID := strings.TrimSpace(strings.TrimPrefix(model.ID, qoderauth.Provider+"/"))
		if internalID == "" {
			continue
		}
		if _, exists := seenInternal[internalID]; exists {
			continue
		}
		seenInternal[internalID] = struct{}{}
		name := displayNameForModel(internalID, model.Name)
		publicID := uniquePublicModelID(name, internalID, mapping)
		if publicID == "" {
			continue
		}
		mapping[publicID] = internalID
		models = append(models, pluginapi.ModelInfo{
			ID: publicID, Object: "model", OwnedBy: qoderauth.Provider,
			Name: internalID, DisplayName: name,
			SupportedGenerationMethods: []string{"chat-completions"},
		})
	}
	defaultModelRegistry.store(authID, mapping)
	return pluginapi.ModelResponse{Provider: qoderauth.Provider, Models: models}, nil
}

func publicModelID(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return qoderauth.Provider + "/" + name
}

func uniquePublicModelID(name, internalID string, mapping map[string]string) string {
	preferred := publicModelID(name)
	if preferred == "" {
		return ""
	}
	if existing, ok := mapping[preferred]; !ok || existing == internalID {
		return preferred
	}
	fallback := publicModelID(internalID)
	if existing, ok := mapping[fallback]; !ok || existing == internalID {
		return fallback
	}
	for suffix := 2; ; suffix++ {
		candidate := publicModelID(internalID + "-" + fmt.Sprint(suffix))
		if _, exists := mapping[candidate]; !exists {
			return candidate
		}
	}
}

func displayNameForModel(id, upstreamName string) string {
	if name := strings.TrimSpace(upstreamName); name != "" {
		return name
	}
	return strings.TrimSpace(id)
}
