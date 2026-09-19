package qodercontrol

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BaseURL != "https://openapi.qoder.com" {
		t.Errorf("BaseURL=%q, want https://openapi.qoder.com", cfg.BaseURL)
	}
	if cfg.BodyLimit != defaultBodyLimit {
		t.Errorf("BodyLimit=%d, want %d", cfg.BodyLimit, defaultBodyLimit)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout=%v, want 30s", cfg.Timeout)
	}
}

func TestNewClient_NilHTTPClient(t *testing.T) {
	c, err := NewClient(nil, Config{
		BaseURL:      "https://openapi.qoder.com",
		AllowedHosts: []string{"openapi.qoder.com"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.httpClient == nil {
		t.Fatal("httpClient is nil after NewClient with nil input")
	}
}

func TestNewClient_EmptyBaseURL(t *testing.T) {
	c, err := NewClient(nil, Config{AllowedHosts: []string{"openapi.qoder.com"}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.config.BaseURL != "https://openapi.qoder.com" {
		t.Errorf("BaseURL=%q, want default", c.config.BaseURL)
	}
}

func TestValidateBaseURL_HTTPS(t *testing.T) {
	cfg := Config{
		BaseURL:      "https://openapi.qoder.com",
		AllowedHosts: []string{"openapi.qoder.com"},
	}
	if err := validateBaseURL(cfg); err != nil {
		t.Errorf("validateBaseURL: %v", err)
	}
}

func TestValidateBaseURL_RejectsHTTP(t *testing.T) {
	cfg := Config{
		BaseURL:      "http://openapi.qoder.com",
		AllowedHosts: []string{"openapi.qoder.com"},
	}
	if err := validateBaseURL(cfg); !errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("want ErrHostNotAllowed, got %v", err)
	}
}

func TestValidateBaseURL_Allowlist(t *testing.T) {
	cfg := Config{
		BaseURL:      "https://evil.example.com",
		AllowedHosts: []string{"openapi.qoder.com"},
	}
	if err := validateBaseURL(cfg); !errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("want ErrHostNotAllowed for non-allowlisted host, got %v", err)
	}
}

func TestValidateBaseURL_SubdomainMatch(t *testing.T) {
	cfg := Config{
		BaseURL:      "https://sub.openapi.qoder.com",
		AllowedHosts: []string{"openapi.qoder.com"},
	}
	if err := validateBaseURL(cfg); err != nil {
		t.Errorf("subdomain should match parent allowlist: %v", err)
	}
}

func TestValidateBaseURL_LoopbackInsecure(t *testing.T) {
	cfg := Config{
		BaseURL:       "http://127.0.0.1:9090",
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.com"},
	}
	if err := validateBaseURL(cfg); err != nil {
		t.Errorf("loopback HTTP should be allowed with AllowInsecure: %v", err)
	}
}

func TestValidateBaseURL_LoopbackRejectedInProd(t *testing.T) {
	cfg := Config{
		BaseURL:      "http://127.0.0.1:9090",
		AllowedHosts: []string{"openapi.qoder.com"},
	}
	if err := validateBaseURL(cfg); !errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("want ErrHostNotAllowed for loopback without AllowInsecure, got %v", err)
	}
}

func TestValidateBaseURL_EmptyHost(t *testing.T) {
	cfg := Config{BaseURL: "https://"}
	if err := validateBaseURL(cfg); !errors.Is(err, ErrInvalidEndpoint) {
		t.Errorf("want ErrInvalidEndpoint for empty host, got %v", err)
	}
}

func TestValidateBaseURL_InvalidURL(t *testing.T) {
	cfg := Config{BaseURL: "://bad"}
	if err := validateBaseURL(cfg); err == nil {
		t.Error("want error for invalid URL")
	}
}

func TestHostInAllowlist(t *testing.T) {
	tests := []struct {
		host    string
		allowed []string
		want    bool
	}{
		{"openapi.qoder.com", []string{"openapi.qoder.com"}, true},
		{"sub.openapi.qoder.com", []string{"openapi.qoder.com"}, true},
		{"evil.com", []string{"openapi.qoder.com"}, false},
		{"openapi.qoder.com.evil.com", []string{"openapi.qoder.com"}, false},
		{"", []string{"openapi.qoder.com"}, false},
	}
	for _, tc := range tests {
		if got := hostInAllowlist(tc.host, tc.allowed); got != tc.want {
			t.Errorf("hostInAllowlist(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"localhost", true},
		{"example.com", false},
		{"10.0.0.1", false},
	}
	for _, tc := range tests {
		if got := isLoopback(tc.host); got != tc.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}
