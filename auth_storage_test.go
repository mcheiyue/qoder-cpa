package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestAuthParseABIAndStableIDs(t *testing.T) {
	first := qoderauth.Credential{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour),
		UserID: "user-a", Email: "a@example.com", Profile: qoderauth.TransportProfileCosyAPI2,
	}
	second := first
	second.UserID = "user-b"
	second.Email = "b@example.com"
	firstData, err := authData(first, "")
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := authData(second, "")
	if err != nil {
		t.Fatal(err)
	}
	if firstData.ID == secondData.ID || firstData.ID != string(qoderauth.AuthIDForUser("user-a")) {
		t.Fatalf("ids=%q %q", firstData.ID, secondData.ID)
	}

	reqRaw, _ := json.Marshal(pluginapi.AuthParseRequest{
		Provider: "qoder", FileName: "account.json", RawJSON: firstData.StorageJSON,
	})
	envelope, err := handleMethod(pluginabi.MethodAuthParse, reqRaw)
	if err != nil {
		t.Fatal(err)
	}
	parsed := decodeResult[pluginapi.AuthParseResponse](t, envelope)
	if !parsed.Handled || parsed.Auth.ID != firstData.ID || parsed.Auth.FileName != "account.json" {
		t.Fatalf("parsed=%#v", parsed)
	}
}

func TestAuthRefreshABIRotatesTokenAndPreservesProfile(t *testing.T) {
	previousCall := hostJSONCall
	previousService := defaultAuthService
	t.Cleanup(func() { hostJSONCall, defaultAuthService = previousCall, previousService })
	defaultAuthService.oauthConfig = qoderauth.OAuthConfig{BaseURL: "https://qoder.test", TokenPath: "/token", ClientID: "client"}
	hostJSONCall = func(_ string, _ any) (json.RawMessage, error) {
		return json.Marshal(pluginapi.HTTPResponse{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`),
		})
	}
	credential := qoderauth.Credential{
		AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(time.Minute),
		UserID: "user-a", Email: "a@example.com", Profile: qoderauth.TransportProfileBearerOpenAI,
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
	result := decodeResult[pluginapi.AuthRefreshResponse](t, envelope)
	refreshed, err := parseStoredCredential(result.Auth.StorageJSON)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.AccessToken != "new-access" || refreshed.RefreshToken != "new-refresh" {
		t.Fatalf("tokens were not rotated")
	}
	if refreshed.Profile != qoderauth.TransportProfileBearerOpenAI || refreshed.UserID != "user-a" {
		t.Fatalf("credential fields changed: %#v", refreshed)
	}
}

func TestAuthParseRejectsMalformedStorage(t *testing.T) {
	raw, _ := json.Marshal(pluginapi.AuthParseRequest{Provider: "qoder", RawJSON: []byte(`{"access_token":"token-only"}`)})
	envelope, err := handleMethod(pluginabi.MethodAuthParse, raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testEnvelope[pluginapi.AuthParseResponse]
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OK || decoded.Error == nil || decoded.Error.Code != "auth_error" {
		t.Fatalf("envelope=%s", envelope)
	}
}

func TestAuthProviderIdentifierAndMissingUserBoundary(t *testing.T) {
	identifierEnvelope, err := handleMethod(pluginabi.MethodAuthIdentifier, nil)
	if err != nil {
		t.Fatal(err)
	}
	identifier := decodeResult[struct {
		Identifier string `json:"identifier"`
	}](t, identifierEnvelope)
	if identifier.Identifier != qoderauth.Provider {
		t.Fatalf("identifier=%q", identifier.Identifier)
	}

	raw, _ := json.Marshal(pluginapi.AuthParseRequest{
		Provider: qoderauth.Provider,
		RawJSON:  []byte(`{"access_token":"access","transport_profile":"cosy-api2"}`),
	})
	envelope, err := handleMethod(pluginabi.MethodAuthParse, raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded testEnvelope[pluginapi.AuthParseResponse]
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OK || decoded.Error == nil || decoded.Error.Code != "auth_error" {
		t.Fatalf("envelope=%s", envelope)
	}
}
