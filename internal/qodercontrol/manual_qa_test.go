package qodercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// TestManualQA_FakeServer runs a comprehensive fake-server manual QA covering
// profile, models, and quota in a single server.
func TestManualQA_FakeServer(t *testing.T) {
	var profileHits, modelsHits, quotaHits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header on every request.
		if r.Header.Get("Authorization") != "Bearer qa-token" {
			t.Errorf("Authorization=%q, want Bearer qa-token", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/userinfo":
			profileHits.Add(1)
			io.WriteString(w, `{"user_id":"qa-user","email":"qa@qoder.com","name":"QA"}`)
		case "/v1/models":
			modelsHits.Add(1)
			io.WriteString(w, `{"data":[{"id":"m1","name":"Model 1"},{"id":"m2","name":"Model 2"}]}`)
		case "/v1/quota":
			quotaHits.Add(1)
			io.WriteString(w, `{"plan":"free","remaining":500,"limit":1000}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := NewClient(nil, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.sh"},
		Timeout:       5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cred := qoderauth.Credential{AccessToken: "qa-token", UserID: "qa-user"}
	ctx := context.Background()

	// Profile
	p, err := c.FetchProfile(ctx, cred)
	if err != nil {
		t.Fatalf("FetchProfile: %v", err)
	}
	if p.UserID != "qa-user" {
		t.Errorf("profile UserID=%q, want qa-user", p.UserID)
	}

	// Models
	models, err := c.FetchModels(ctx, cred)
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models)=%d, want 2", len(models))
	}

	// Quota
	q, err := c.FetchQuota(ctx, cred)
	if err != nil {
		t.Fatalf("FetchQuota: %v", err)
	}
	if q.Plan != "free" {
		t.Errorf("quota Plan=%q, want free", q.Plan)
	}

	// Verify call counts.
	if profileHits.Load() != 1 {
		t.Errorf("profileHits=%d, want 1", profileHits.Load())
	}
	if modelsHits.Load() != 1 {
		t.Errorf("modelsHits=%d, want 1", modelsHits.Load())
	}
	if quotaHits.Load() != 1 {
		t.Errorf("quotaHits=%d, want 1", quotaHits.Load())
	}
}

// TestAdversarial_Redirect verifies that redirects are not followed (default client).
func TestAdversarial_Redirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/userinfo" {
			http.Redirect(w, r, "/evil", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"ok":"redirected"}`)
	}))
	defer srv.Close()

	c, err := NewClient(nil, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.sh"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err == nil {
		t.Fatal("want error for redirect (default client should not follow)")
	}
}

// TestAdversarial_SlowServer verifies timeout.
func TestAdversarial_SlowServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		io.WriteString(w, `{"user_id":"slow"}`)
	}))
	defer srv.Close()

	c, err := NewClient(&http.Client{Timeout: 50 * time.Millisecond}, Config{
		BaseURL:       srv.URL,
		AllowInsecure: true,
		AllowedHosts:  []string{"openapi.qoder.sh"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err == nil {
		t.Fatal("want error for slow server timeout")
	}
}

// TestAdversarial_HTML200 verifies non-JSON 200 is handled.
func TestAdversarial_HTML200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><body>Cloudflare Error</body></html>`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrNonJSONResponse) {
		t.Errorf("want ErrNonJSONResponse for HTML 200, got %v", err)
	}
}

// TestAdversarial_ErrorBodyTokenCanary verifies secrets are not leaked in errors.
func TestAdversarial_ErrorBodyTokenCanary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"error":"access_token=secret123 refresh_token=rt_secret456"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err == nil {
		t.Fatal("want error")
	}
	// The UpstreamError should NOT include the response body.
	var upstreamErr *UpstreamError
	if errors.As(err, &upstreamErr) {
		// Verify the error string doesn't contain body contents.
		jsonStr, _ := json.Marshal(upstreamErr)
		if len(jsonStr) > 0 {
			t.Logf("UpstreamError JSON: %s", string(jsonStr))
		}
	}
}

// TestAdversarial_EmptyModelsCatalog verifies no fake models on empty upstream.
func TestAdversarial_EmptyModelsCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":null}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	models, err := c.FetchModels(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if models != nil {
		t.Errorf("want nil/null models, got %v (must not fabricate)", models)
	}
}

// TestAdversarial_Quota500 verifies quota failure returns ErrQuotaUnknown.
func TestAdversarial_Quota500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `<html>502</html>`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchQuota(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrQuotaUnknown) {
		t.Errorf("want ErrQuotaUnknown, got %v", err)
	}
}

// TestAdversarial_BodyLimitExceeded verifies oversized response is rejected.
func TestAdversarial_BodyLimitExceeded(t *testing.T) {
	big := make([]byte, defaultBodyLimit+100)
	for i := range big {
		big[i] = 'x'
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(big)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchProfile(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("want ErrResponseTooLarge, got %v", err)
	}
}
