package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// cnAuthService builds an authService with distinguishable intl/cn test endpoints.
func cnAuthService() authService {
	return authService{
		oauthConfig: qoderauth.OAuthConfig{
			BaseURL: "https://qoder.test", APIBaseURL: "https://qoder.test", DevicePath: "/device", PollPath: "/poll", ClientID: "test-client",
		},
		oauthConfigCN: qoderauth.OAuthConfig{
			BaseURL: "https://qoder.cn.test", APIBaseURL: "https://qoder.cn.test", DevicePath: "/device", PollPath: "/poll", ClientID: "test-client",
		},
		controlConfig: qodercontrol.Config{
			BaseURL: "https://qoder.test", AllowedHosts: []string{"qoder.test"},
		},
		controlConfigCN: qodercontrol.Config{
			BaseURL: "https://qoder.cn.test", AllowedHosts: []string{"qoder.cn.test"},
		},
	}
}

func TestAuthLoginStartCNRegionUsesCNBase(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService = cnAuthService()
	tokenCalls := 0
	hostJSONCall = fakeAuthHost(t, &tokenCalls)

	startRaw, _ := json.Marshal(rpcAuthLoginStartRequest{
		AuthLoginStartRequest: pluginapi.AuthLoginStartRequest{
			Provider: "qoder", Metadata: map[string]any{"region": "cn"},
		},
		HostCallbackID: "cb-login",
	})
	envelope, err := handleMethod(pluginabi.MethodAuthLoginStart, startRaw)
	if err != nil {
		t.Fatal(err)
	}
	start := decodeResult[pluginapi.AuthLoginStartResponse](t, envelope)
	if !strings.HasPrefix(start.URL, "https://qoder.cn.test/device?") {
		t.Fatalf("start.URL=%q, want CN device URL", start.URL)
	}
	// 宿主把 start 响应 Metadata 存入 OAuth session，poll 时原样带回（同 workbuddy realm 回填）。
	if got, _ := start.Metadata["region"].(string); got != "cn" {
		t.Fatalf("start.Metadata region=%q, want cn backfill for poll replay", got)
	}
}

func TestAuthLoginPollCNRegionAssignsCNProfileAndIsolatedID(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService = cnAuthService()
	tokenCalls := 0
	hostJSONCall = fakeAuthHost(t, &tokenCalls)

	startRaw, _ := json.Marshal(rpcAuthLoginStartRequest{
		AuthLoginStartRequest: pluginapi.AuthLoginStartRequest{
			Provider: "qoder", Metadata: map[string]any{"region": "cn"},
		},
		HostCallbackID: "cb-login",
	})
	startEnvelope, err := handleMethod(pluginabi.MethodAuthLoginStart, startRaw)
	if err != nil {
		t.Fatal(err)
	}
	start := decodeResult[pluginapi.AuthLoginStartResponse](t, startEnvelope)

	pollRaw, _ := json.Marshal(rpcAuthLoginPollRequest{
		AuthLoginPollRequest: pluginapi.AuthLoginPollRequest{
			Provider: "qoder", State: start.State, Metadata: map[string]any{"region": "cn"},
		},
		HostCallbackID: "cb-login",
	})
	if _, err := handleMethod(pluginabi.MethodAuthLoginPoll, pollRaw); err != nil {
		t.Fatal(err)
	}
	successEnvelope, err := handleMethod(pluginabi.MethodAuthLoginPoll, pollRaw)
	if err != nil {
		t.Fatal(err)
	}
	success := decodeResult[pluginapi.AuthLoginPollResponse](t, successEnvelope)
	if success.Status != pluginapi.AuthLoginStatusSuccess {
		t.Fatalf("status=%q, want success", success.Status)
	}
	cred, err := parseStoredCredential(success.Auth.StorageJSON)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Profile != qoderauth.TransportProfileCosyCN {
		t.Fatalf("Profile=%q, want %q", cred.Profile, qoderauth.TransportProfileCosyCN)
	}
	intlID := string(qoderauth.AuthIDForUser("user-1"))
	if success.Auth.ID == intlID {
		t.Fatalf("CN auth ID %q collides with intl AuthIDForUser", success.Auth.ID)
	}
	if success.Auth.ID != string(qoderauth.AuthIDForProfile("user-1", qoderauth.TransportProfileCosyCN)) {
		t.Fatalf("auth ID=%q, want AuthIDForProfile(cn) derivation", success.Auth.ID)
	}
}

func TestAuthRefreshCNProfileUsesCNTokenURL(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService = cnAuthService()

	var refreshURL string
	hostJSONCall = func(_ string, payload any) (json.RawMessage, error) {
		req := payload.(map[string]any)["request"].(pluginapi.HTTPRequest)
		refreshURL = req.URL
		return json.Marshal(pluginapi.HTTPResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`),
		})
	}
	credential := qoderauth.Credential{
		AccessToken: "old-access", RefreshToken: "old-refresh",
		UserID: "user-a", Email: "a@example.com", Profile: qoderauth.TransportProfileCosyCN,
	}
	data, err := authData(credential, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(rpcAuthRefreshRequest{
		AuthRefreshRequest: pluginapi.AuthRefreshRequest{AuthID: data.ID, AuthProvider: "qoder", StorageJSON: data.StorageJSON},
		HostCallbackID:     "cb-refresh",
	})
	envelope, err := handleMethod(pluginabi.MethodAuthRefresh, raw)
	if err != nil {
		t.Fatal(err)
	}
	decodeResult[pluginapi.AuthRefreshResponse](t, envelope)
	if !strings.HasPrefix(refreshURL, "https://qoder.cn.test/api/v1/deviceToken/refresh") {
		t.Fatalf("refreshURL=%q, want CN deviceToken/refresh endpoint", refreshURL)
	}
}
