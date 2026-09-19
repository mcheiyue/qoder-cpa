package cosy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// NewTransport endpoint validation
// ---------------------------------------------------------------------------

func TestNewTransport_UnknownEndpoint_ReturnsError(t *testing.T) {
	_, err := NewTransport(Config{Endpoint: Endpoint("cosy-evil")})
	if err == nil {
		t.Fatal("expected error for unknown endpoint")
	}
	if !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("error: got %v, want ErrUnknownEndpoint", err)
	}
}

func TestNewTransport_UnknownEndpoint_FakeServerZeroCalls(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, err := NewTransport(Config{
		Endpoint:          Endpoint("cosy-evil"),
		HTTPClient:        ts.Client(),
		BaseURL:           ts.URL,
		AllowTestEndpoint: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 0 {
		t.Errorf("fake server called %d times, want 0", calls.Load())
	}
}

func TestNewTransport_API2_Succeeds(t *testing.T) {
	tr, err := NewTransport(Config{Endpoint: EndpointAPI2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("transport is nil")
	}
}

func TestNewTransport_API3_Succeeds(t *testing.T) {
	tr, err := NewTransport(Config{Endpoint: EndpointAPI3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("transport is nil")
	}
}

// ---------------------------------------------------------------------------
// Config BaseURL SSRF protection
// ---------------------------------------------------------------------------

func TestConfig_BaseURL_RequiresAllowTestEndpoint(t *testing.T) {
	_, err := NewTransport(Config{
		Endpoint:          EndpointAPI2,
		BaseURL:           "http://localhost:9999",
		AllowTestEndpoint: false,
	})
	if err == nil {
		t.Fatal("expected error: BaseURL without AllowTestEndpoint")
	}
}

func TestConfig_BaseURL_AllowTestEndpoint_AllowsLoopback(t *testing.T) {
	tr, err := NewTransport(Config{
		Endpoint:          EndpointAPI2,
		BaseURL:           "http://127.0.0.1:8080",
		AllowTestEndpoint: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("transport is nil")
	}
}

func TestConfig_BaseURL_RejectsNonLoopback(t *testing.T) {
	errs := []string{
		"http://evil.com/steal",
		"https://10.0.0.1:8080/secret",
		"http://192.168.1.1:80",
		"http://172.16.0.1:80",
		"http://0.0.0.0:80",
	}
	for _, url := range errs {
		t.Run(url, func(t *testing.T) {
			_, err := NewTransport(Config{
				Endpoint:          EndpointAPI2,
				BaseURL:           url,
				AllowTestEndpoint: true,
			})
			if err == nil {
				t.Errorf("expected error for non-loopback BaseURL %q", url)
			}
		})
	}
}

func TestConfig_BaseURL_RejectsUserinfo(t *testing.T) {
	_, err := NewTransport(Config{
		Endpoint:          EndpointAPI2,
		BaseURL:           "http://user:pass@127.0.0.1:8080",
		AllowTestEndpoint: true,
	})
	if err == nil {
		t.Fatal("expected error for BaseURL with userinfo")
	}
}

func TestConfig_BaseURL_RejectsQuery(t *testing.T) {
	_, err := NewTransport(Config{
		Endpoint:          EndpointAPI2,
		BaseURL:           "http://127.0.0.1:8080?secret=1",
		AllowTestEndpoint: true,
	})
	if err == nil {
		t.Fatal("expected error for BaseURL with query")
	}
}

func TestConfig_BaseURL_RejectsFragment(t *testing.T) {
	_, err := NewTransport(Config{
		Endpoint:          EndpointAPI2,
		BaseURL:           "http://127.0.0.1:8080#frag",
		AllowTestEndpoint: true,
	})
	if err == nil {
		t.Fatal("expected error for BaseURL with fragment")
	}
}

func TestConfig_BaseURL_RejectsHTTPScheme(t *testing.T) {
	_, err := NewTransport(Config{
		Endpoint:          EndpointAPI2,
		BaseURL:           "ftp://127.0.0.1:8080",
		AllowTestEndpoint: true,
	})
	if err == nil {
		t.Fatal("expected error for non-http(s) BaseURL")
	}
}

func TestConfig_BaseURL_AllowsHTTPS(t *testing.T) {
	tr, err := NewTransport(Config{
		Endpoint:          EndpointAPI2,
		BaseURL:           "https://localhost:9999",
		AllowTestEndpoint: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("transport is nil")
	}
}
