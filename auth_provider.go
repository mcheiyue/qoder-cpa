package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type authService struct {
	oauthConfig     qoderauth.OAuthConfig
	oauthConfigCN   qoderauth.OAuthConfig
	controlConfig   qodercontrol.Config
	controlConfigCN qodercontrol.Config
}

var defaultAuthService = authService{
	oauthConfig: qoderauth.DefaultConfig(), oauthConfigCN: qoderauth.DefaultConfigCN(),
	controlConfig: qodercontrol.DefaultConfig(), controlConfigCN: qodercontrol.DefaultConfigCN(),
}

// regionIsCN reports whether login metadata selects the CN region.
func regionIsCN(md map[string]any) bool {
	v, _ := md["region"].(string)
	return strings.EqualFold(v, "cn")
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
	id := string(qoderauth.AuthIDForProfile(cred.UserID, cred.Profile))
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
	cfg := s.oauthConfig
	region := ""
	if regionIsCN(req.Metadata) {
		cfg = s.oauthConfigCN
		region = "cn"
	}
	login, err := qoderauth.DeviceLogin(ctx, qoderauth.DeviceLoginRequest{Config: cfg, Client: client})
	if err != nil {
		return pluginapi.AuthLoginStartResponse{}, err
	}
	resp := pluginapi.AuthLoginStartResponse{
		Provider: qoderauth.Provider, URL: login.VerifyURL,
		State: string(login.Transaction.ID), ExpiresAt: login.ExpiresAt,
	}
	// 回填 region：宿主把它存入 OAuth session，poll 时原样带回，保证轮询与 start 同域。
	if region != "" {
		resp.Metadata = map[string]any{"region": region}
	}
	return resp, nil
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
	oauthCfg, controlCfg := s.oauthConfig, s.controlConfig
	if regionIsCN(req.Metadata) {
		oauthCfg, controlCfg = s.oauthConfigCN, s.controlConfigCN
	}
	status, err := qoderauth.PollLogin(ctx, qoderauth.PollLoginRequest{
		Config: oauthCfg, Client: client, TransactionID: qoderauth.TransactionID(req.State),
	})
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	if status.Status == qoderauth.TransactionPending {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending, Message: status.Message}, nil
	}
	control, err := qodercontrol.NewClient(client, controlCfg)
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	profile, err := control.FetchProfile(ctx, *status.Credential)
	if err != nil && status.Credential.UserID == "" {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	if profile != nil {
		status.Credential.UserID = profile.UserID
		status.Credential.Email = profile.Email
		status.Credential.OrganizationID = profile.OrganizationID
		status.Credential.OrganizationTags = append([]string(nil), profile.OrganizationTags...)
	}
	if regionIsCN(req.Metadata) {
		status.Credential.Profile = qoderauth.TransportProfileCosyCN
	} else {
		status.Credential.Profile = qoderauth.TransportProfileCosyAPI2
	}
	runtime, err := cosy.DeriveRuntimeFields(rand.Reader, cosy.RuntimeFieldInput{
		UID: status.Credential.UserID, OrganizationID: status.Credential.OrganizationID,
		OrganizationTags: status.Credential.OrganizationTags, DataPolicyAgreed: true,
	})
	if err != nil {
		return pluginapi.AuthLoginPollResponse{}, err
	}
	status.Credential.RuntimeInfo = runtime.EncryptUserInfo
	status.Credential.RuntimeKey = runtime.Key
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
	refreshCfg := s.oauthConfig
	if cred.Profile == qoderauth.TransportProfileCosyCN {
		refreshCfg = s.oauthConfigCN
	}
	refreshed, err := qoderauth.Refresh(ctx, qoderauth.RefreshRequest{
		Config: qoderauth.RefreshConfig{TokenURL: strings.TrimRight(refreshCfg.APIBaseURL, "/") + "/api/v1/deviceToken/refresh", ClientID: refreshCfg.ClientID},
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
