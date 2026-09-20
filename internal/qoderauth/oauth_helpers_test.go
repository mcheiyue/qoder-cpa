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

	mux.HandleFunc("/api/v1/deviceToken/poll", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("poll: got method %s, want GET", r.Method)
		}
		if r.URL.Query().Get("nonce") == "" || r.URL.Query().Get("verifier") == "" {
			t.Error("poll: missing nonce or verifier")
		}
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
				"token": %q,
				"refresh_token": %q,
				"user_id": "user-test",
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
	if cfg.APIBaseURL != "https://openapi.qoder.sh" {
		t.Fatalf("APIBaseURL=%q", cfg.APIBaseURL)
	}
	if cfg.DevicePath != "/device/selectAccounts" || cfg.PollPath != "/api/v1/deviceToken/poll" {
		t.Fatalf("device paths=%q %q", cfg.DevicePath, cfg.PollPath)
	}
	if cfg.ClientID != "qoder-cpa" {
		t.Fatalf("ClientID=%q", cfg.ClientID)
	}
}
