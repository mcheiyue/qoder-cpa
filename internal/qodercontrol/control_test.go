package qodercontrol

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDoRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization=%q, want Bearer test-token", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept=%q, want application/json", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c, _ := NewClient(nil, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.com"},
	})
	body, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/test", "test-token")
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if !strings.Contains(string(body), `"ok":true`) {
		t.Errorf("body=%s, want ok:true", body)
	}
}

func TestDoRequest_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()

	c, _ := NewClient(nil, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.com"},
	})
	_, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/test", "tok")
	if err == nil {
		t.Fatal("want error for 401")
	}
	var upstreamErr *UpstreamError
	if !errors.As(err, &upstreamErr) {
		t.Errorf("want UpstreamError, got %T", err)
	} else if upstreamErr.StatusCode != 401 {
		t.Errorf("StatusCode=%d, want 401", upstreamErr.StatusCode)
	}
}

func TestDoRequest_BodyLimit(t *testing.T) {
	big := strings.Repeat("x", int(defaultBodyLimit+1))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, big)
	}))
	defer srv.Close()

	c, _ := NewClient(nil, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.com"},
	})
	_, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/test", "")
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("want ErrResponseTooLarge, got %v", err)
	}
}

func TestDoRequest_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c, _ := NewClient(nil, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.com"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.doRequest(ctx, http.MethodGet, srv.URL+"/test", "")
	if err == nil {
		t.Fatal("want error for cancelled context")
	}
}

func TestDoRequest_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		io.WriteString(w, "slow")
	}))
	defer srv.Close()

	c, _ := NewClient(&http.Client{Timeout: 50 * time.Millisecond}, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.com"},
	})
	_, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/test", "")
	if err == nil {
		t.Fatal("want error for timeout")
	}
}

func TestDoRequest_NonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html><body>Error</body></html>")
	}))
	defer srv.Close()

	c, _ := NewClient(nil, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.com"},
	})
	body, err := c.doRequest(context.Background(), http.MethodGet, srv.URL+"/test", "")
	if err != nil {
		t.Fatalf("doRequest should return body on 200: %v", err)
	}
	if !strings.Contains(string(body), "<html>") {
		t.Errorf("body should contain HTML")
	}
}

func TestBuildURL(t *testing.T) {
	c, _ := NewClient(nil, Config{
		BaseURL:      "https://openapi.qoder.com",
		AllowedHosts: []string{"openapi.qoder.com"},
	})
	got := c.buildURL("/v1/models")
	want := "https://openapi.qoder.com/v1/models"
	if got != want {
		t.Errorf("buildURL=%q, want %q", got, want)
	}
}

func TestBuildURL_StripsTrailingSlash(t *testing.T) {
	c, _ := NewClient(nil, Config{
		BaseURL:      "https://openapi.qoder.com/",
		AllowedHosts: []string{"openapi.qoder.com"},
	})
	got := c.buildURL("/v1/models")
	want := "https://openapi.qoder.com/v1/models"
	if got != want {
		t.Errorf("buildURL=%q, want %q", got, want)
	}
}
