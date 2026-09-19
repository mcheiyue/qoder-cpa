package qodercontrol

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBodyLimit int64 = 1 << 20 // 1 MiB

// Config holds endpoint configuration for the Qoder control plane.
type Config struct {
	BaseURL       string        // default "https://openapi.qoder.com"
	AllowInsecure bool          // test-only: allow HTTP loopback
	AllowedHosts  []string      // production allowlist
	BodyLimit     int64         // max response body; default 1 MiB
	Timeout       time.Duration // HTTP timeout; default 30s
}

// DefaultConfig returns the official Qoder HTTPS endpoints.
func DefaultConfig() Config {
	return Config{
		BaseURL:      "https://openapi.qoder.com",
		AllowedHosts: []string{"openapi.qoder.com"},
		BodyLimit:    defaultBodyLimit,
		Timeout:      30 * time.Second,
	}
}

// Client queries the Qoder control plane (profile, runtime, models, quota).
type Client struct {
	httpClient *http.Client
	config     Config
}

// NewClient validates the config and returns a Client.
func NewClient(httpClient *http.Client, cfg Config) (*Client, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	if cfg.BodyLimit <= 0 {
		cfg.BodyLimit = defaultBodyLimit
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultConfig().BaseURL
	}
	if err := validateBaseURL(cfg); err != nil {
		return nil, err
	}
	return &Client{httpClient: httpClient, config: cfg}, nil
}

// validateBaseURL checks scheme, allowlist, and loopback rules.
func validateBaseURL(cfg Config) error {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return ErrInvalidEndpoint
	}
	host := u.Hostname()
	if host == "" {
		return ErrInvalidEndpoint
	}
	if u.Scheme != "https" {
		if !cfg.AllowInsecure || !isLoopback(host) {
			return ErrHostNotAllowed
		}
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return ErrInvalidEndpoint
	}
	if !isLoopback(host) && !hostInAllowlist(host, cfg.AllowedHosts) {
		return ErrHostNotAllowed
	}
	return nil
}

// hostInAllowlist checks exact match or parent-domain match.
func hostInAllowlist(host string, allowed []string) bool {
	for _, h := range allowed {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// isLoopback reports whether host is a loopback address.
func isLoopback(host string) bool {
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// doRequest executes an HTTP request with Bearer auth and body limit.
func (c *Client) doRequest(ctx context.Context, method, endpoint, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, ErrInvalidEndpoint
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUpstreamFailed
	}
	defer func() {
		_, _ = io.CopyN(io.Discard, resp.Body, c.config.BodyLimit+1)
		resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.config.BodyLimit+1))
	if err != nil {
		return nil, ErrMalformedResponse
	}
	if int64(len(body)) > c.config.BodyLimit {
		return nil, ErrResponseTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, &UpstreamError{StatusCode: resp.StatusCode, Endpoint: endpoint}
	}
	return body, nil
}

// buildURL constructs the full URL from the base and path.
func (c *Client) buildURL(path string) string {
	return strings.TrimRight(c.config.BaseURL, "/") + path
}
