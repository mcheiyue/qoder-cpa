package qoderauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRefresh_NoRefreshToken(t *testing.T) {
	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: "http://unused", ClientID: "test"},
		Client: &http.Client{},
		Cred:   Credential{UserID: "u1"},
	})
	if err == nil {
		t.Fatal("expected error for missing refresh token")
	}
	var rerr *RefreshError
	if !strings.Contains(err.Error(), "no refresh token") {
		t.Fatalf("error should mention no refresh token: %v", err)
	}
	// Verify it wraps ErrMissingRefreshToken.
	if !errors.As(err, &rerr) {
		t.Fatalf("expected RefreshError, got %T", err)
	}
}

func TestRefresh_NilClient(t *testing.T) {
	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: "http://unused", ClientID: "test"},
		Cred:   Credential{RefreshToken: "rt", UserID: "u1"},
	})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestRefresh_EmptyTokenURL(t *testing.T) {
	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: "", ClientID: "test"},
		Client: &http.Client{},
		Cred:   Credential{RefreshToken: "rt", UserID: "u1"},
	})
	if err == nil {
		t.Fatal("expected error for empty token URL")
	}
}

func TestRefresh_EmptyClientID(t *testing.T) {
	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: "http://unused", ClientID: ""},
		Client: &http.Client{},
		Cred:   Credential{RefreshToken: "rt", UserID: "u1"},
	})
	if err == nil {
		t.Fatal("expected error for empty client ID")
	}
}

func TestRefresh_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"token revoked"}`)
	}))
	defer srv.Close()

	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "test"},
		Client: &http.Client{Timeout: 5 * time.Second},
		Cred: Credential{
			AccessToken:  "at",
			RefreshToken: "revoked-rt",
			UserID:       "u1",
		},
	})
	if err == nil {
		t.Fatal("expected error for upstream invalid_grant")
	}
}

func TestRefresh_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{not json`)
	}))
	defer srv.Close()

	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "test"},
		Client: &http.Client{Timeout: 5 * time.Second},
		Cred: Credential{
			AccessToken:  "at",
			RefreshToken: "rt",
			UserID:       "u1",
		},
	})
	if err == nil {
		t.Fatal("expected error for malformed response")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error should mention malformed: %v", err)
	}
}

func TestRefresh_TTLFromExpiresIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"access_token": "new-at",
			"token_type": "bearer",
			"expires_in": 7200
		}`)
	}))
	defer srv.Close()

	resp, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "test"},
		Client: &http.Client{Timeout: 5 * time.Second},
		Cred: Credential{
			AccessToken:  "old-at",
			RefreshToken: "rt",
			UserID:       "u1",
		},
		// TTL=0 → defaults to ExpiresIn.
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Now().Add(7200 * time.Second)
	if resp.NextRefreshAfter.Sub(expected) > 2*time.Second || expected.Sub(resp.NextRefreshAfter) > 2*time.Second {
		t.Fatalf("NextRefreshAfter=%v, expected ~%v", resp.NextRefreshAfter, expected)
	}
}

func TestRefresh_TTLExplicit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"access_token": "new-at",
			"token_type": "bearer",
			"expires_in": 7200
		}`)
	}))
	defer srv.Close()

	resp, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "test"},
		Client: &http.Client{Timeout: 5 * time.Second},
		Cred: Credential{
			AccessToken:  "old-at",
			RefreshToken: "rt",
			UserID:       "u1",
		},
		TTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Now().Add(30 * time.Minute)
	if resp.NextRefreshAfter.Sub(expected) > 2*time.Second || expected.Sub(resp.NextRefreshAfter) > 2*time.Second {
		t.Fatalf("NextRefreshAfter=%v, expected ~%v", resp.NextRefreshAfter, expected)
	}
}

func TestRefresh_ExpiredUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"error":"expired_token","error_description":"token expired"}`)
	}))
	defer srv.Close()

	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "test"},
		Client: &http.Client{Timeout: 5 * time.Second},
		Cred: Credential{
			AccessToken:  "at",
			RefreshToken: "expired-rt",
			UserID:       "u1",
		},
	})
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestRefreshError_Unwrap(t *testing.T) {
	err := &RefreshError{Reason: "test reason"}
	if !strings.Contains(err.Error(), "test reason") {
		t.Fatalf("Error()=%q", err.Error())
	}
	if !errors.Is(err, ErrMissingRefreshToken) {
		t.Fatal("RefreshError should wrap ErrMissingRefreshToken")
	}
}
