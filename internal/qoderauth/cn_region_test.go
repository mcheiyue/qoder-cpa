package qoderauth

import "testing"

func TestDefaultConfigCNEndpoints(t *testing.T) {
	cfg := DefaultConfigCN()
	if cfg.BaseURL != "https://qoder.com.cn" {
		t.Errorf("BaseURL=%q, want https://qoder.com.cn", cfg.BaseURL)
	}
	if cfg.APIBaseURL != "https://openapi.qoder.com.cn" {
		t.Errorf("APIBaseURL=%q, want https://openapi.qoder.com.cn", cfg.APIBaseURL)
	}
	if cfg.DevicePath != "/device/selectAccounts" {
		t.Errorf("DevicePath=%q, want /device/selectAccounts", cfg.DevicePath)
	}
	if cfg.PollPath != "/api/v1/deviceToken/poll" {
		t.Errorf("PollPath=%q, want /api/v1/deviceToken/poll", cfg.PollPath)
	}
	if cfg.ClientID != "qoder-cpa" {
		t.Errorf("ClientID=%q, want qoder-cpa", cfg.ClientID)
	}
}

func TestIsValidProfileAcceptsCN(t *testing.T) {
	if !IsValidProfile(TransportProfileCosyCN) {
		t.Error("IsValidProfile(cosy-cn)=false, want true")
	}
	if IsValidProfile(TransportProfile("nope")) {
		t.Error("IsValidProfile(nope)=true, want false")
	}
}

func TestAuthIDForProfileIntlZeroDrift(t *testing.T) {
	for _, profile := range []TransportProfile{TransportProfileCosyAPI2, TransportProfileCosyAPI3, TransportProfileBearerOpenAI} {
		if got := AuthIDForProfile("user-1", profile); got != AuthIDForUser("user-1") {
			t.Errorf("AuthIDForProfile(user-1, %s)=%s, want intl drift-free %s", profile, got, AuthIDForUser("user-1"))
		}
	}
}

func TestAuthIDForProfileCNIsolated(t *testing.T) {
	intl := AuthIDForUser("user-1")
	cn := AuthIDForProfile("user-1", TransportProfileCosyCN)
	if cn == intl {
		t.Fatalf("cn AuthID %s collides with intl AuthID for same user ID", cn)
	}
	if cn != AuthIDForProfile("user-1", TransportProfileCosyCN) {
		t.Error("cn AuthID not stable across calls")
	}
	if cn == AuthIDForProfile("user-2", TransportProfileCosyCN) {
		t.Error("cn AuthID collides across different user IDs")
	}
}
