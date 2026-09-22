package qodercontrol

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

func TestFetchQuota_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" {
			t.Errorf("authorization=%q, want Bearer at", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case quotaUsagePath:
			io.WriteString(w, `{"userType":"personal_standard","isQuotaExceeded":false,"expiresAt":1770000000000,"upgradeUrl":"https://qoder.com/upgrade","userQuota":{"total":2000,"used":1000,"remaining":1000,"unit":"credits"}}`)
		case quotaPlanPath:
			io.WriteString(w, `{"user_type":"personal_standard","plan_tier_name":"Pro","is_paid_plan":true}`)
		case quotaStatusPath:
			io.WriteString(w, `{"userType":"personal_standard","userTag":"Pro","nextResetAt":1770003600000}`)
		default:
			t.Errorf("unexpected path=%q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	q, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchQuota: %v", err)
	}
	if q.PlanTier != "Pro" {
		t.Errorf("PlanTier=%q, want Pro", q.PlanTier)
	}
	if q.Remaining != 1000 {
		t.Errorf("Remaining=%v, want 1000", q.Remaining)
	}
	if q.Limit != 2000 {
		t.Errorf("Limit=%v, want 2000", q.Limit)
	}
	if q.Used != 1000 || q.Unit != "credits" || !q.PaidPlan || q.Exhausted {
		t.Fatalf("quota=%+v", q)
	}
	if q.PeriodEnd.IsZero() || q.ResetAt.IsZero() || q.SyncedAt.IsZero() {
		t.Fatalf("quota timestamps=%+v", q)
	}
}

func TestFetchQuota_PartialSuccessKeepsSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case quotaUsagePath:
			io.WriteString(w, `{"userQuota":{"total":10,"used":10,"remaining":0,"unit":"credits"},"isQuotaExceeded":true}`)
		case quotaPlanPath:
			w.WriteHeader(http.StatusServiceUnavailable)
		case quotaStatusPath:
			io.WriteString(w, `{"userTag":"Free","nextResetAt":1770000000000}`)
		}
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	q, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchQuota: %v", err)
	}
	if !q.Exhausted || q.PlanTier != "Free" || q.Error == "" {
		t.Fatalf("partial quota=%+v", q)
	}
}

func TestFetchQuota_OptionalFieldsAndExhaustion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case quotaUsagePath:
			io.WriteString(w, `{"userQuota":{"total":5,"used":5,"remaining":0}}`)
		case quotaPlanPath, quotaStatusPath:
			io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	q, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchQuota: %v", err)
	}
	if !q.Exhausted || q.Unit != "credits" || !q.ResetAt.IsZero() {
		t.Fatalf("quota=%+v", q)
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
	q, err := c.FetchQuota(context.Background(), cred)
	if !errors.Is(err, ErrQuotaUnknown) {
		t.Errorf("want ErrQuotaUnknown (not credential invalid), got %v", err)
	}
	if q == nil || q.Error != "upstream HTTP 401" {
		t.Errorf("quota error=%+v, want sanitized upstream status", q)
	}
	// The credential is not modified - this is verified by the caller, not this function.
}

func TestQuotaTimeHandlesMilliseconds(t *testing.T) {
	want := time.Unix(1770000000, 0).UTC()
	if got := quotaTime(1770000000000); !got.Equal(want) {
		t.Fatalf("quotaTime=%s, want %s", got, want)
	}
}
