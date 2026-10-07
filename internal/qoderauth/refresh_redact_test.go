package qoderauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// H1④: Refresh 顶层出口必须对上游原文做 token 脱敏，且保住 RefreshError 类型与
// ErrMissingRefreshToken Unwrap 链（refresh_error_test 的既有断言不许破坏）。
func TestRefreshUpstreamErrorRedactsTokens(t *testing.T) {
	const (
		refreshToken = "rt-super-secret-000111222333"
		accessToken  = "at-also-secret-444555666777"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 上游用 200 + 错误体回 invalid_grant（doHTTPRequest 非 2xx 会直接丢 body）。
		// error_description 回显凭据 —— 这就是脱敏要挡的泄漏面。
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token ` + refreshToken + ` rejected, access ` + accessToken + ` expired"}`))
	}))
	defer srv.Close()

	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "cid"},
		Client: srv.Client(),
		Cred:   Credential{RefreshToken: refreshToken, AccessToken: accessToken, UserID: "u1"},
	})
	if err == nil {
		t.Fatal("expected refresh error")
	}

	msg := err.Error()
	if strings.Contains(msg, refreshToken) {
		t.Fatalf("refresh token leaked in error: %s", msg)
	}
	if strings.Contains(msg, accessToken) {
		t.Fatalf("access token leaked in error: %s", msg)
	}
	if !strings.Contains(msg, "***") {
		t.Fatalf("expected *** redaction marker, got: %s", msg)
	}

	// 类型链不许破坏：调用方依赖 errors.As/Is 分类。
	var rerr *RefreshError
	if !errors.As(err, &rerr) {
		t.Fatalf("expected *RefreshError, got %T (%s)", err, msg)
	}
	if !errors.Is(err, ErrMissingRefreshToken) {
		t.Fatalf("expected ErrMissingRefreshToken wrap, got: %s", msg)
	}
	if rerr.Reason == "" || strings.Contains(rerr.Reason, refreshToken) {
		t.Fatalf("Reason must be redacted too, got: %q", rerr.Reason)
	}
}

// H1④: 凭据不含敏感值时错误原样透出，脱敏不许误伤（短串/空串不替换）。
func TestRefreshRedactNoOpWithoutSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token revoked by upstream"}`))
	}))
	defer srv.Close()

	_, err := Refresh(context.Background(), RefreshRequest{
		Config: RefreshConfig{TokenURL: srv.URL, ClientID: "cid"},
		Client: srv.Client(),
		Cred:   Credential{RefreshToken: "rt-abcdef0123456789", UserID: "u1"},
	})
	if err == nil {
		t.Fatal("expected refresh error")
	}
	if !strings.Contains(err.Error(), "token revoked by upstream") {
		t.Fatalf("unrelated upstream text must survive, got: %s", err)
	}
}
