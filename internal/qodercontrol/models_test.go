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

func TestFetchModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%q, want /v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[
			{"id":"qoder-1","name":"Qoder 1"},
			{"id":"qoder-2","name":"Qoder 2"}
		]}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	models, err := c.FetchModels(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models)=%d, want 2", len(models))
	}
	if models[0].ID != "qoder-1" {
		t.Errorf("models[0].ID=%q, want qoder-1", models[0].ID)
	}
}

func TestFetchModels_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[]}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	models, err := c.FetchModels(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchModels empty: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("want 0 models, got %d", len(models))
	}
}

func TestFetchModels_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	models, err := c.FetchModels(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err != nil {
		t.Fatalf("FetchModels empty body: %v", err)
	}
	if models != nil {
		t.Errorf("want nil models for empty body, got %v", models)
	}
}

func TestFetchModels_UpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"server down"}`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchModels(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrModelUnavailable) {
		t.Errorf("want ErrModelUnavailable, got %v", err)
	}
}

func TestFetchModels_HTMLResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><body>Internal Error</body></html>`)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	_, err := c.FetchModels(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if !errors.Is(err, ErrModelUnavailable) {
		t.Errorf("want ErrModelUnavailable for HTML response, got %v", err)
	}
}

func TestFetchModels_NoFakeModels(t *testing.T) {
	// Verify that on failure, no static/compiled models are returned.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv)
	models, err := c.FetchModels(context.Background(), qoderauth.Credential{AccessToken: "at"})
	if err == nil {
		t.Fatal("want error for 503")
	}
	if models != nil {
		t.Errorf("want nil models on failure, got %v (must not fabricate)", models)
	}
}
