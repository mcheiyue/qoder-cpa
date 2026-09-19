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

func TestFetchQuota_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/quota" {
			t.Errorf("path=%q, want /v1/quota", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"plan":"pro","remaining":1000,"limit":2000}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	q, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchQuota: %v", err)
	}
	if q.Plan != "pro" {
		t.Errorf("Plan=%q, want pro", q.Plan)
	}
	if q.Remaining != 1000 {
		t.Errorf("Remaining=%d, want 1000", q.Remaining)
	}
	if q.Limit != 2000 {
		t.Errorf("Limit=%d, want 2000", q.Limit)
	}
}

func TestFetchQuota_UpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"internal"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrQuotaUnknown) {
		t.Errorf("want ErrQuotaUnknown, got %v", err)
	}
}

func TestFetchQuota_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `<html>502 Bad Gateway</html>`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrQuotaUnknown) {
		t.Errorf("want ErrQuotaUnknown for 500, got %v", err)
	}
}

func TestFetchQuota_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrQuotaUnknown) {
		t.Errorf("want ErrQuotaUnknown for empty body, got %v", err)
	}
}

func TestFetchQuota_FailureDoesNotInvalidateCred(t *testing.T) {
	// Verify that quota failure returns ErrQuotaUnknown, not a credential error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"token expired"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	cred := qoderauth.Credential{AccessToken: "at-valid", UserID: "u1"}
	_, err := c.FetchQuota(context.Background(), cred)
	if !errors.Is(err, ErrQuotaUnknown) {
		t.Errorf("want ErrQuotaUnknown (not credential invalid), got %v", err)
	}
	// The credential is not modified - this is verified by the caller, not this function.
}
