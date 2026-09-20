package qodercontrol

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

func TestFetchProfile_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/userinfo" {
			t.Errorf("path=%q, want /api/v1/userinfo", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer at-123" {
			t.Errorf("Authorization=%q, want Bearer at-123", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"user_id":"u1","email":"test@qoder.com","name":"Test User"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	cred := qoderauth.Credential{AccessToken: "at-123", UserID: "u1"}
	p, err := c.FetchProfile(context.Background(), cred)
	if err != nil {
		t.Fatalf("FetchProfile: %v", err)
	}
	if p.UserID != "u1" {
		t.Errorf("UserID=%q, want u1", p.UserID)
	}
	if p.Email != "test@qoder.com" {
		t.Errorf("Email=%q, want test@qoder.com", p.Email)
	}
	if p.Name != "Test User" {
		t.Errorf("Name=%q, want Test User", p.Name)
	}
}

func TestFetchProfile_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "bad"})
	if !errors.Is(err, ErrUpstreamFailed) {
		t.Errorf("want ErrUpstreamFailed, got %v", err)
	}
}

func TestFetchProfile_NonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><body>Error page with token canary=abc123</body></html>`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrNonJSONResponse) {
		t.Errorf("want ErrNonJSONResponse, got %v", err)
	}
}

func TestFetchProfile_MissingUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"email":"test@qoder.com"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrMalformedResponse) {
		t.Errorf("want ErrMalformedResponse for missing user_id, got %v", err)
	}
}

func TestFetchProfile_NoSecretInError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"secret=rt-super-secret-token"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err == nil {
		t.Fatal("want error")
	}
	errStr := err.Error()
	if strings.Contains(errStr, "rt-super-secret-token") {
		t.Errorf("error contains secret token: %s", errStr)
	}
}

func TestFetchProfileValidated_UIDMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"user_id":"u1","email":"a@b.com"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	cred := qoderauth.Credential{AccessToken: "at", UserID: "u1"}
	p, err := c.FetchProfileValidated(context.Background(), cred)
	if err != nil {
		t.Fatalf("FetchProfileValidated: %v", err)
	}
	if p.UserID != "u1" {
		t.Errorf("UserID=%q, want u1", p.UserID)
	}
}

func TestFetchProfileValidated_UIDMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"user_id":"u-other","email":"a@b.com"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	cred := qoderauth.Credential{AccessToken: "at", UserID: "u1"}
	_, err := c.FetchProfileValidated(context.Background(), cred)
	if err == nil {
		t.Fatal("want error for UID mismatch")
	}
	if !IsProfileUIDMismatch(err) {
		t.Errorf("want ProfileUIDMismatch, got %v", err)
	}
}

func newTestClient(t *testing.T, srv *httptest.Server) (*Client, error) {
	t.Helper()
	return NewClient(nil, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.sh"},
	})
}
