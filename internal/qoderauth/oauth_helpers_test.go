package qoderauth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDeviceCodeServer returns an httptest server that handles device code + token endpoints.
func fakeDeviceCodeServer(t *testing.T, opts ...func(*fakeConfig)) *httptest.Server {
	t.Helper()
	cfg := &fakeConfig{
		deviceCodeDelay: 0,
		tokenHandler:    defaultTokenHandler,
	}
	for _, o := range opts {
		o(cfg)
	}

	mux := http.NewServeMux()
	var callCount atomic.Int32

	mux.HandleFunc("/oauth/device/code", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("device code: got method %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("device code: parse form: %v", err)
		}
		// Verify PKCE challenge is sent.
		if r.FormValue("code_challenge") == "" {
			t.Error("device code: missing code_challenge")
		}
		if r.FormValue("code_challenge_method") != "S256" {
			t.Errorf("device code: code_challenge_method=%q, want S256", r.FormValue("code_challenge_method"))
		}
		if r.FormValue("nonce") == "" {
			t.Error("device code: missing nonce")
		}
		if r.FormValue("machine_id") == "" {
			t.Error("device code: missing machine_id")
		}
		if r.FormValue("client_id") == "" {
			t.Error("device code: missing client_id")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"device_code": "dc-test-123",
			"user_code": "ABCD-1234",
			"verification_uri": "https://qoder.com/activate",
			"expires_in": 900,
			"interval": 5
		}`)
	})

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		cfg.tokenHandler(w, r, cfg)
	})

	mux.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		cfg.tokenHandler(w, r, cfg)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
	})
	return srv
}

type fakeConfig struct {
	deviceCodeDelay time.Duration
	tokenHandler    func(http.ResponseWriter, *http.Request, *fakeConfig)
	pendingCount    int
	pendingSeen     int
}

func defaultTokenHandler(w http.ResponseWriter, r *http.Request, cfg *fakeConfig) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"error":"authorization_pending","error_description":"waiting for user"}`)
}

// withPendingThenSuccess returns a token handler that returns pending N times then success.
func withPendingThenSuccess(n int, accessToken, refreshToken string) func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.pendingCount = n
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			if fc.pendingSeen < fc.pendingCount {
				fc.pendingSeen++
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"error":"authorization_pending","error_description":"waiting"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
				"access_token": %q,
				"refresh_token": %q,
				"token_type": "bearer",
				"expires_in": 3600,
				"scope": "openid"
			}`, accessToken, refreshToken)
		}
	}
}

// withExpiredToken returns a token handler that returns expired_token error.
func withExpiredToken() func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"error":"expired_token","error_description":"code expired"}`)
		}
	}
}

// withAccessDenied returns a token handler that returns access_denied.
func withAccessDenied() func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"error":"access_denied","error_description":"user denied"}`)
		}
	}
}

// withMalformedJSON returns a token handler that returns malformed JSON.
func withMalformedJSON() func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{not json`)
		}
	}
}

// withOversizedBody returns a token handler that returns >1MiB body.
func withOversizedBody() func(*fakeConfig) {
	return func(cfg *fakeConfig) {
		cfg.tokenHandler = func(w http.ResponseWriter, r *http.Request, fc *fakeConfig) {
			w.Header().Set("Content-Type", "application/json")
			// Write exactly maxResponseBodyBytes + 1.
			big := strings.Repeat("x", maxResponseBodyBytes+1)
			fmt.Fprint(w, big)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BaseURL != "https://qoder.com" {
		t.Fatalf("BaseURL=%q", cfg.BaseURL)
	}
	if cfg.DeviceCodePath != "/oauth/device/code" {
		t.Fatalf("DeviceCodePath=%q", cfg.DeviceCodePath)
	}
	if cfg.TokenPath != "/oauth/token" {
		t.Fatalf("TokenPath=%q", cfg.TokenPath)
	}
	if cfg.ClientID != "qoder-cpa" {
		t.Fatalf("ClientID=%q", cfg.ClientID)
	}
}
