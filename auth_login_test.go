package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type testEnvelope[T any] struct {
	OK     bool             `json:"ok"`
	Result json.RawMessage  `json:"result"`
	Error  *pluginabi.Error `json:"error"`
}

func decodeResult[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var envelope testEnvelope[T]
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("envelope error: %#v", envelope.Error)
	}
	var result T
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAuthLoginABIRoundTrip(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService = authService{
		oauthConfig: qoderauth.OAuthConfig{
			BaseURL: "https://qoder.test", APIBaseURL: "https://qoder.test", DevicePath: "/device", PollPath: "/poll", ClientID: "test-client",
		},
		controlConfig: qodercontrol.Config{BaseURL: "https://qoder.test", AllowedHosts: []string{"qoder.test"}},
	}
	tokenCalls := 0
	hostJSONCall = fakeAuthHost(t, &tokenCalls)

	startRaw, _ := json.Marshal(rpcAuthLoginStartRequest{
		AuthLoginStartRequest: pluginapi.AuthLoginStartRequest{Provider: "qoder"}, HostCallbackID: "cb-login",
	})
	startEnvelope, err := handleMethod(pluginabi.MethodAuthLoginStart, startRaw)
	if err != nil {
		t.Fatal(err)
	}
	start := decodeResult[pluginapi.AuthLoginStartResponse](t, startEnvelope)
	if start.Provider != "qoder" || start.State == "" || !strings.HasPrefix(start.URL, "https://qoder.test/device?") {
		t.Fatalf("start=%#v", start)
	}

	pollRaw, _ := json.Marshal(rpcAuthLoginPollRequest{
		AuthLoginPollRequest: pluginapi.AuthLoginPollRequest{Provider: "qoder", State: start.State}, HostCallbackID: "cb-login",
	})
	pendingEnvelope, _ := handleMethod(pluginabi.MethodAuthLoginPoll, pollRaw)
	pending := decodeResult[pluginapi.AuthLoginPollResponse](t, pendingEnvelope)
	if pending.Status != pluginapi.AuthLoginStatusPending {
		t.Fatalf("pending=%#v", pending)
	}
	successEnvelope, _ := handleMethod(pluginabi.MethodAuthLoginPoll, pollRaw)
	success := decodeResult[pluginapi.AuthLoginPollResponse](t, successEnvelope)
	if success.Status != pluginapi.AuthLoginStatusSuccess || success.Auth.ID == "" {
		t.Fatalf("success=%#v", success)
	}
	if success.Auth.Label != "alice@example.com" || success.Auth.Provider != "qoder" {
		t.Fatalf("auth=%#v", success.Auth)
	}
	if strings.Contains(string(success.Auth.StorageJSON), "verifier") || strings.Contains(string(success.Auth.StorageJSON), "nonce") {
		t.Fatalf("transient secret leaked: %s", success.Auth.StorageJSON)
	}
	if _, ok := success.Auth.Metadata["access_token"]; ok {
		t.Fatal("token leaked to metadata")
	}
	cred, err := parseStoredCredential(success.Auth.StorageJSON)
	if err != nil {
		t.Fatal(err)
	}
	if cred.UserID != "user-1" || cred.Profile != qoderauth.TransportProfileCosyAPI2 {
		t.Fatalf("credential=%#v", cred)
	}
	if cred.RuntimeInfo == "" || cred.RuntimeKey == "" {
		t.Fatalf("COSY runtime fields were not persisted")
	}
}

func TestAuthLoginSucceedsWhenUserinfoCannotEnrichTokenIdentity(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService = authService{
		oauthConfig: qoderauth.OAuthConfig{
			BaseURL: "https://qoder.test", APIBaseURL: "https://qoder.test", DevicePath: "/device", PollPath: "/poll", ClientID: "test-client",
		},
		controlConfig: qodercontrol.Config{BaseURL: "https://qoder.test", AllowedHosts: []string{"qoder.test"}},
	}
	tokenCalls := 0
	hostJSONCall = fakeAuthHostWithUserinfo(t, &tokenCalls, `{}`)

	startRaw, _ := json.Marshal(rpcAuthLoginStartRequest{
		AuthLoginStartRequest: pluginapi.AuthLoginStartRequest{Provider: "qoder"}, HostCallbackID: "cb-login",
	})
	startEnvelope, err := handleMethod(pluginabi.MethodAuthLoginStart, startRaw)
	if err != nil {
		t.Fatal(err)
	}
	start := decodeResult[pluginapi.AuthLoginStartResponse](t, startEnvelope)
	pollRaw, _ := json.Marshal(rpcAuthLoginPollRequest{
		AuthLoginPollRequest: pluginapi.AuthLoginPollRequest{Provider: "qoder", State: start.State}, HostCallbackID: "cb-login",
	})
	if _, err := handleMethod(pluginabi.MethodAuthLoginPoll, pollRaw); err != nil {
		t.Fatal(err)
	}
	successEnvelope, _ := handleMethod(pluginabi.MethodAuthLoginPoll, pollRaw)
	success := decodeResult[pluginapi.AuthLoginPollResponse](t, successEnvelope)
	if success.Status != pluginapi.AuthLoginStatusSuccess {
		t.Fatalf("status=%q, want success when token response already contains user_id", success.Status)
	}
	cred, err := parseStoredCredential(success.Auth.StorageJSON)
	if err != nil {
		t.Fatal(err)
	}
	if cred.UserID != "user-1" {
		t.Fatalf("UserID=%q, want token identity user-1", cred.UserID)
	}
}

func fakeAuthHost(t *testing.T, tokenCalls *int) func(string, any) (json.RawMessage, error) {
	return fakeAuthHostWithUserinfo(t, tokenCalls, `{"user_id":"user-1","email":"alice@example.com","name":"Alice"}`)
}

func fakeAuthHostWithUserinfo(t *testing.T, tokenCalls *int, userinfo string) func(string, any) (json.RawMessage, error) {
	t.Helper()
	return func(method string, payload any) (json.RawMessage, error) {
		if method != pluginabi.MethodHostHTTPDo {
			t.Fatalf("method=%q", method)
		}
		httpReq := payload.(map[string]any)["request"].(pluginapi.HTTPRequest)
		path, _ := url.Parse(httpReq.URL)
		var body string
		switch path.Path {
		case "/poll":
			*tokenCalls++
			if *tokenCalls == 1 {
				body = `{"error":"authorization_pending","error_description":"waiting"}`
			} else {
				body = `{"token":"access-1","refresh_token":"refresh-1","user_id":"user-1","expires_in":3600}`
			}
		case "/api/v1/userinfo":
			body = userinfo
		default:
			t.Fatalf("unexpected URL %s", httpReq.URL)
		}
		return json.Marshal(pluginapi.HTTPResponse{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(body)})
	}
}
