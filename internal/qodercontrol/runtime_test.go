package qodercontrol

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

func TestFetchRuntimeFields_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cosy/runtime" {
			t.Errorf("path=%q, want /v1/cosy/runtime", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"authorization":"Bearer cosy-auth-xyz",
			"session_id":"sess-1",
			"uid":"u1",
			"token":"tok-abc"
		}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	cred := qoderauth.Credential{AccessToken: "at-123", UserID: "u1"}
	rf, err := c.FetchRuntimeFields(context.Background(), cred)
	if err != nil {
		t.Fatalf("FetchRuntimeFields: %v", err)
	}
	if rf.Authorization != "Bearer cosy-auth-xyz" {
		t.Errorf("Authorization=%q, want Bearer cosy-auth-xyz", rf.Authorization)
	}
	if rf.SessionID != "sess-1" {
		t.Errorf("SessionID=%q, want sess-1", rf.SessionID)
	}
	if rf.UID != "u1" {
		t.Errorf("UID=%q, want u1", rf.UID)
	}
}

func TestFetchRuntimeFields_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"error":"forbidden"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchRuntimeFields(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrUpstreamFailed) {
		t.Errorf("want ErrUpstreamFailed, got %v", err)
	}
}

func TestFetchRuntimeFields_MissingAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"session_id":"sess-1","uid":"u1"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchRuntimeFields(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrMalformedResponse) {
		t.Errorf("want ErrMalformedResponse for missing authorization, got %v", err)
	}
}

func TestFetchRuntimeFields_NonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, `not json`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchRuntimeFields(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrNonJSONResponse) {
		t.Errorf("want ErrNonJSONResponse, got %v", err)
	}
}

func TestRefreshRuntimeFields_Success(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"authorization":"Bearer refreshed-auth",
			"session_id":"sess-new",
			"uid":"u1",
			"token":"tok-new"
		}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	rf, err := c.RefreshRuntimeFields(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("RefreshRuntimeFields: %v", err)
	}
	if rf.Authorization != "Bearer refreshed-auth" {
		t.Errorf("Authorization=%q, want Bearer refreshed-auth", rf.Authorization)
	}
	if callCount != 1 {
		t.Errorf("callCount=%d, want 1", callCount)
	}
}
