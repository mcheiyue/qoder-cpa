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

// 锁：上游 /api/v2/user/plan 的 start_date/end_date 是试用起止，
// /api/v2/quota/usage 的 addOnQuota 是独立于主额度的加成桶。缺任一项，
// 面板上 "Pro Trial" 就只剩名字、看不到到期日与加成额度。
func TestFetchQuota_ParsesTrialWindowAndAddOnQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case quotaUsagePath:
			io.WriteString(w, `{"userQuota":{"total":300,"used":0,"remaining":300,"unit":"credits"},"addOnQuota":{"total":100,"used":0,"remaining":100,"unit":"credits"}}`)
		case quotaPlanPath:
			io.WriteString(w, `{"plan_tier_name":"Pro Trial","start_date":1792243326471,"end_date":1793452926471}`)
		case quotaStatusPath:
			io.WriteString(w, `{"name":"nick5805205645","email":"","userTag":"Pro Trial","nextResetAt":1793452926471}`)
		}
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	q, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchQuota: %v", err)
	}
	if q.Nickname != "nick5805205645" {
		t.Errorf("Nickname=%q", q.Nickname)
	}
	if q.AddOn != 100 || q.AddOnRemaining != 100 {
		t.Errorf("addOn=%v/%v, want 100/100", q.AddOn, q.AddOnRemaining)
	}
	if q.Remaining != 300 || q.Limit != 300 {
		t.Errorf("main quota must stay separate from addOn: %+v", q)
	}
	wantEnd := time.Unix(1793452926, 0).UTC()
	if !q.PeriodEnd.Equal(wantEnd) || q.PeriodStart.IsZero() {
		t.Errorf("period=%v..%v, want start set and end %v", q.PeriodStart, q.PeriodEnd, wantEnd)
	}
}

// 锁：上游可能只回 isQuotaExceeded 布尔而不带任何额度数值。此时账号是否
// 耗尽属于未知，不能拿布尔把账号判死（否则面板与选号都会永久排除它）。
func TestFetchQuota_BooleanOnlyMustNotMarkExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case quotaUsagePath:
			io.WriteString(w, `{"isQuotaExceeded":true}`)
		case quotaPlanPath:
			io.WriteString(w, `{}`)
		case quotaStatusPath:
			io.WriteString(w, `{"name":"nick1","isQuotaExceeded":true}`)
		}
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	q, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchQuota: %v", err)
	}
	if q.Exhausted {
		t.Fatalf("boolean-only quota must not be exhausted: %+v", q)
	}
}

// 锁：/api/v3/user/status 不携带任何额度数值，它的 isQuotaExceeded 只能作
// 旁证，不能单独把账号判为耗尽。
func TestFetchQuota_StatusBooleanAloneMustNotMarkExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case quotaUsagePath:
			w.WriteHeader(http.StatusInternalServerError)
		case quotaPlanPath:
			io.WriteString(w, `{}`)
		case quotaStatusPath:
			io.WriteString(w, `{"name":"nick1","isQuotaExceeded":true}`)
		}
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	q, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchQuota: %v", err)
	}
	if q.Exhausted {
		t.Fatalf("status-only boolean must not be exhausted: %+v", q)
	}
	if q.Error != "upstream HTTP 500" {
		t.Errorf("Error=%q, want sanitized upstream status", q.Error)
	}
}

func TestQuotaTimeHandlesMilliseconds(t *testing.T) {
	want := time.Unix(1770000000, 0).UTC()
	if got := quotaTime(1770000000000); !got.Equal(want) {
		t.Fatalf("quotaTime=%s, want %s", got, want)
	}
}
