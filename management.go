package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type managementService struct {
	mu       sync.Mutex
	hostCall func(string, any) (json.RawMessage, error)
}

var defaultManagementService = &managementService{hostCall: callHostJSON}

type managementHandler struct {
	kind string
}

type managementAccountsResponse struct {
	Accounts []managementAccount `json:"accounts"`
}

type managementAccount struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	Label     string `json:"label,omitempty"`
	Email     string `json:"email,omitempty"`
	Profile   string `json:"transport_profile,omitempty"`
	Status    string `json:"status,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"`
}

type profileUpdateRequest struct {
	AuthIndex string                     `json:"auth_index"`
	Profile   qoderauth.TransportProfile `json:"transport_profile"`
}

func managementRegister() map[string]any {
	return map[string]any{
		"routes": []map[string]string{
			{"method": http.MethodGet, "path": "/qoder/accounts"},
			{"method": http.MethodPost, "path": "/qoder/accounts/profile"},
		},
		"resources": []map[string]string{{"path": "/index.html", "menu": "Qoder"}},
	}
}

func (h managementHandler) HandleManagement(ctx context.Context, request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	switch h.kind {
	case "accounts":
		return defaultManagementService.accounts(ctx)
	case "profile":
		return defaultManagementService.updateProfile(ctx, request.Body)
	case "web":
		return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{
			"Content-Type":  {"text/html; charset=utf-8"},
			"Cache-Control": {"no-store"},
		}, Body: qoderWebUI}, nil
	default:
		return pluginapi.ManagementResponse{StatusCode: http.StatusNotFound}, nil
	}
}

func (s *managementService) accounts(_ context.Context) (pluginapi.ManagementResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := s.hostCall(pluginabi.MethodHostAuthList, nil)
	if err != nil {
		return pluginapi.ManagementResponse{}, err
	}
	var result struct {
		Files []pluginapi.HostAuthFileEntry `json:"files"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return pluginapi.ManagementResponse{}, fmt.Errorf("decode auth list: %w", err)
	}
	accounts := make([]managementAccount, 0, len(result.Files))
	for _, file := range result.Files {
		if !strings.EqualFold(file.Provider, qoderauth.Provider) && !strings.EqualFold(file.Type, qoderauth.Provider) {
			continue
		}
		profile := qoderauth.TransportProfileCosyAPI2
		if file.AuthIndex != "" {
			if rawAuth, getErr := s.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: file.AuthIndex}); getErr == nil {
				var auth pluginapi.HostAuthGetResponse
				var storage qoderauth.StorageJSON
				if json.Unmarshal(rawAuth, &auth) == nil && json.Unmarshal(auth.JSON, &storage) == nil && qoderauth.IsValidProfile(storage.Profile) {
					profile = storage.Profile
				}
			}
		}
		accounts = append(accounts, managementAccount{
			AuthIndex: file.AuthIndex, Name: file.Name, Label: file.Label,
			Email: file.Email, Profile: string(profile), Status: file.Status, Disabled: file.Disabled,
		})
	}
	return jsonManagementResponse(http.StatusOK, managementAccountsResponse{Accounts: accounts})
}

func (s *managementService) updateProfile(_ context.Context, raw []byte) (pluginapi.ManagementResponse, error) {
	var request profileUpdateRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return jsonManagementError(http.StatusBadRequest, "invalid JSON body"), nil
	}
	request.AuthIndex = strings.TrimSpace(request.AuthIndex)
	request.Profile = qoderauth.TransportProfile(strings.TrimSpace(string(request.Profile)))
	if request.AuthIndex == "" || !qoderauth.IsValidProfile(request.Profile) {
		return jsonManagementError(http.StatusBadRequest, "auth_index and a valid transport_profile are required"), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rawAuth, err := s.hostCall(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: request.AuthIndex})
	if err != nil {
		return jsonManagementError(http.StatusNotFound, "account not found"), nil
	}
	var auth pluginapi.HostAuthGetResponse
	if err := json.Unmarshal(rawAuth, &auth); err != nil {
		return pluginapi.ManagementResponse{}, fmt.Errorf("decode auth: %w", err)
	}
	var storage qoderauth.StorageJSON
	if err := json.Unmarshal(auth.JSON, &storage); err != nil {
		return jsonManagementError(http.StatusBadRequest, "account is not a qoder credential"), nil
	}
	if storage.UserID == "" || storage.AccessToken == "" {
		return jsonManagementError(http.StatusBadRequest, "account is not a qoder credential"), nil
	}
	storage.Profile = request.Profile
	encoded, err := json.Marshal(storage)
	if err != nil {
		return pluginapi.ManagementResponse{}, fmt.Errorf("encode auth: %w", err)
	}
	name := auth.Name
	if name == "" {
		return jsonManagementError(http.StatusBadRequest, "account has no writable name"), nil
	}
	if _, err := s.hostCall(pluginabi.MethodHostAuthSave, pluginapi.HostAuthSaveRequest{Name: name, JSON: encoded}); err != nil {
		return jsonManagementError(http.StatusBadGateway, "account update failed"), nil
	}
	return jsonManagementResponse(http.StatusOK, map[string]string{
		"auth_index": request.AuthIndex, "transport_profile": string(request.Profile),
	})
}

func jsonManagementResponse(status int, value any) (pluginapi.ManagementResponse, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return pluginapi.ManagementResponse{}, err
	}
	return pluginapi.ManagementResponse{StatusCode: status, Headers: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
}

func jsonManagementError(status int, message string) pluginapi.ManagementResponse {
	response, _ := jsonManagementResponse(status, map[string]string{"error": message})
	return response
}
