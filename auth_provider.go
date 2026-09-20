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

type authService struct {
	oauthConfig   qoderauth.OAuthConfig
	controlConfig qodercontrol.Config
}

var defaultAuthService = authService{
	oauthConfig: qoderauth.DefaultConfig(), controlConfig: qodercontrol.DefaultConfig(),
}

type rpcAuthLoginStartRequest struct {
	pluginapi.AuthLoginStartRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type rpcAuthLoginPollRequest struct {
	pluginapi.AuthLoginPollRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type rpcAuthRefreshRequest struct {
	pluginapi.AuthRefreshRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

func authData(cred qoderauth.Credential, fileName string) (pluginapi.AuthData, error) {
	if err := cred.Validate(); err != nil {
		return pluginapi.AuthData{}, err
	}
	if !qoderauth.IsValidProfile(cred.Profile) {
		return pluginapi.AuthData{}, fmt.Errorf("invalid transport profile %q", cred.Profile)
	}
	storage, err := json.Marshal(qoderauth.FromCredential(cred))
	if err != nil {
		return pluginapi.AuthData{}, fmt.Errorf("encode auth storage: %w", err)
	}
	id := string(qoderauth.AuthIDForUser(cred.UserID))
	label := cred.Email
	if label == "" {
		label = id
	}
	if fileName == "" {
		fileName = id + ".json"
	}
	return pluginapi.AuthData{
		Provider: qoderauth.Provider, ID: id, FileName: fileName, Label: label,
		StorageJSON: storage, Attributes: map[string]string{"transport_profile": string(cred.Profile)},
		NextRefreshAfter: cred.ExpiresAt,
	}, nil
}

func parseStoredCredential(raw []byte) (qoderauth.Credential, error) {
	var storage qoderauth.StorageJSON
	if err := json.Unmarshal(raw, &storage); err != nil {
		return qoderauth.Credential{}, fmt.Errorf("decode qoder auth: %w", err)
	}
	cred := storage.ToCredential()
	if err := cred.Validate(); err != nil {
		return qoderauth.Credential{}, err
	}
	if !qoderauth.IsValidProfile(cred.Profile) {
		return qoderauth.Credential{}, fmt.Errorf("invalid transport profile %q", cred.Profile)
	}
	return cred, nil
}

func (s authService) parse(raw []byte) (pluginapi.AuthParseResponse, error) {
	var req pluginapi.AuthParseRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return pluginapi.AuthParseResponse{}, err
	}
	if req.Provider != "" && !strings.EqualFold(req.Provider, qoderauth.Provider) {
		return pluginapi.AuthParseResponse{}, nil
	}
	cred, err := parseStoredCredential(req.RawJSON)
	if err != nil {
		return pluginapi.AuthParseResponse{}, err
	}
	data, err := authData(cred, req.FileName)
	return pluginapi.AuthParseResponse{Handled: err == nil, Auth: data}, err
}

func (s authService) start(ctx context.Context, raw []byte) (pluginapi.AuthLoginStartResponse, error) {
	var req rpcAuthLoginStartRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return pluginapi.AuthLoginStartResponse{}, err
	}
	client, err := newHostHTTPClient(req.HostCallbackID)
	if err != nil {
		return pluginapi.AuthLoginStartResponse{}, err
	}
	login, err := qoderauth.DeviceLogin(ctx, qoderauth.DeviceLoginRequest{Config: s.oauthConfig, Client: client})
	if err != nil {
		return pluginapi.AuthLoginStartResponse{}, err
	}
	return pluginapi.AuthLoginStartResponse{
		Provider: qoderauth.Provider, URL: login.VerifyURL,
		State: string(login.Transaction.ID), ExpiresAt: login.ExpiresAt,
	}, nil
}

func (s authService) poll(ctx context.Context, raw []byte) (pluginapi.AuthLoginPollResponse, error) {
	var req rpcAuthLoginPollRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	client, err := newHostHTTPClient(req.HostCallbackID)
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	status, err := qoderauth.PollLogin(ctx, qoderauth.PollLoginRequest{
		Config: s.oauthConfig, Client: client, TransactionID: qoderauth.TransactionID(req.State),
	})
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	if status.Status == qoderauth.TransactionPending {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending, Message: status.Message}, nil
	}
	control, err := qodercontrol.NewClient(client, s.controlConfig)
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	profile, err := control.FetchProfile(ctx, *status.Credential)
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	status.Credential.UserID = profile.UserID
	status.Credential.Email = profile.Email
	status.Credential.Profile = qoderauth.TransportProfileCosyAPI2
	data, err := authData(*status.Credential, "")
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusSuccess, Auth: data}, nil
}

func (s authService) refresh(ctx context.Context, raw []byte) (pluginapi.AuthRefreshResponse, error) {
	var req rpcAuthRefreshRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return pluginapi.AuthRefreshResponse{}, err
	}
	cred, err := parseStoredCredential(req.StorageJSON)
	if err != nil {
		return pluginapi.AuthRefreshResponse{}, err
	}
	client, err := newHostHTTPClient(req.HostCallbackID)
	if err != nil {
		return pluginapi.AuthRefreshResponse{}, err
	}
	refreshed, err := qoderauth.Refresh(ctx, qoderauth.RefreshRequest{
		Config: qoderauth.RefreshConfig{TokenURL: s.oauthConfig.BaseURL + s.oauthConfig.TokenPath, ClientID: s.oauthConfig.ClientID},
		Client: client, Cred: cred,
	})
	if err != nil {
		return pluginapi.AuthRefreshResponse{}, err
	}
	data, err := authData(refreshed.Credential, "")
	if err != nil {
		return pluginapi.AuthRefreshResponse{}, err
	}
	return pluginapi.AuthRefreshResponse{Auth: data, NextRefreshAfter: refreshed.NextRefreshAfter}, nil
}
