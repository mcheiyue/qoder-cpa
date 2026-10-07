package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
)

// H1②: registration 必须声明 quota_provider 能力（宿主原生配额面板入口）。
func TestRegistrationDeclaresQuotaProvider(t *testing.T) {
	raw, err := json.Marshal(registration())
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatal(err)
	}
	if reg.Capabilities["quota_provider"] != true {
		t.Fatalf("quota_provider = %v, want true", reg.Capabilities["quota_provider"])
	}
}

type quotaEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func callQuota(t *testing.T, method string, payload any) quotaEnvelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := handleMethod(method, raw)
	if err != nil {
		t.Fatal(err)
	}
	var env quotaEnvelope
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("bad envelope %s: %v", resp, err)
	}
	return env
}

func quotaStorage(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(qoderauth.StorageJSON{AccessToken: "at-test-000111", UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// H1②: quota.identifier 返回 provider 标识。
func TestQuotaIdentifierReturnsQoder(t *testing.T) {
	env := callQuota(t, "quota.identifier", nil)
	if !env.OK {
		t.Fatalf("identifier error: %s", env.Error)
	}
	var result struct {
		Identifier string `json:"identifier"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Identifier != "qoder" {
		t.Fatalf("identifier=%q, want qoder", result.Identifier)
	}
}

// H1②: quota.describe 声明支持面——单 provider、不支持 reset。
func TestQuotaDescribeProviders(t *testing.T) {
	env := callQuota(t, "quota.describe", nil)
	if !env.OK {
		t.Fatalf("describe error: %s", env.Error)
	}
	var result struct {
		SupportedProviders []string `json:"supported_providers"`
		DisplayName        string   `json:"display_name"`
		SupportsReset      bool     `json:"supports_reset"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.SupportedProviders) != 1 || result.SupportedProviders[0] != "qoder" {
		t.Fatalf("supported_providers=%v, want [qoder]", result.SupportedProviders)
	}
	if result.DisplayName == "" {
		t.Fatal("display_name empty")
	}
	if result.SupportsReset {
		t.Fatal("supports_reset should be false")
	}
}

// H1②: quota.reset 不支持——success=false + 明确文案。
func TestQuotaResetNotSupported(t *testing.T) {
	env := callQuota(t, "quota.reset", nil)
	if !env.OK {
		t.Fatalf("reset error: %s", env.Error)
	}
	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatal("success should be false")
	}
	if !strings.Contains(result.Message, "does not support quota reset") {
		t.Fatalf("message=%q", result.Message)
	}
}

// H1②: quota.fetch 凭据损坏 → auth_error（error envelope）。
func TestQuotaFetchInvalidStorage(t *testing.T) {
	env := callQuota(t, "quota.fetch", map[string]any{
		"provider":     "qoder",
		"storage_json": []byte("garbage"),
	})
	if env.OK || env.Error == nil || env.Error.Code != "auth_error" {
		t.Fatalf("env=%+v, want auth_error", env)
	}
}

// H1②: quota.fetch provider 不匹配 → unsupported_provider。
func TestQuotaFetchUnsupportedProvider(t *testing.T) {
	env := callQuota(t, "quota.fetch", map[string]any{
		"provider": "openai",
	})
	if env.OK || env.Error == nil || env.Error.Code != "unsupported_provider" {
		t.Fatalf("env=%+v, want unsupported_provider", env)
	}
}

// H1②: 上游查询 seam（quota_fetchFn），成功时 Summary 指标 + scheduler quota 快照。
func stubQuotaFetch(t *testing.T, fn func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error)) {
	t.Helper()
	orig := quotaFetchFn
	quotaFetchFn = fn
	t.Cleanup(func() { quotaFetchFn = orig })
}

// H1②: 成功路径——Summary 带 remain 指标，且 noteQuotaSnapshot 登记 scheduler 快照。
func TestQuotaFetchSuccessSnapshotsScheduler(t *testing.T) {
	s := testSchedulerState(t)
	stubQuotaFetch(t, func(_ context.Context, cred qoderauth.Credential) (*qodercontrol.Quota, error) {
		if cred.AccessToken == "" {
			t.Fatal("empty access token passed to fetch")
		}
		return &qodercontrol.Quota{Remaining: 42, Limit: 100, Used: 58, Unit: "次", Exhausted: false}, nil
	})
	env := callQuota(t, "quota.fetch", map[string]any{
		"provider":     "qoder",
		"auth_id":      "auth-1",
		"storage_json": quotaStorage(t),
	})
	if !env.OK {
		t.Fatalf("fetch error: %s", env.Error)
	}
	var result struct {
		Summary []struct {
			Key   string  `json:"key"`
			Value float64 `json:"value"`
			Unit  string  `json:"unit"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatal(err)
	}
	remain := -1.0
	for _, m := range result.Summary {
		if m.Key == "remain" {
			remain = m.Value
		}
	}
	if remain != 42 {
		t.Fatalf("remain metric=%v, want 42 (summary=%+v)", remain, result.Summary)
	}
	snap, ok := s.getQuota("auth-1")
	if !ok || snap.remaining != 42 || snap.exhausted {
		t.Fatalf("scheduler quota snapshot=%+v ok=%v, want remaining=42", snap, ok)
	}
}

// H1②: 上游失败走 Summary error 指标（ok envelope，面板可显示），且不透传上游原文（无 token 泄漏）。
func TestQuotaFetchUpstreamErrorMasked(t *testing.T) {
	stubQuotaFetch(t, func(context.Context, qoderauth.Credential) (*qodercontrol.Quota, error) {
		return nil, errors.New("upstream 401 Bearer secret-tok-xyz")
	})
	env := callQuota(t, "quota.fetch", map[string]any{
		"provider":     "qoder",
		"auth_id":      "auth-2",
		"storage_json": quotaStorage(t),
	})
	if !env.OK {
		t.Fatalf("fetch upstream error must stay ok envelope, got: %s", env.Error)
	}
	var result struct {
		Summary []struct {
			Key  string `json:"key"`
			Unit string `json:"unit"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Summary) == 0 || result.Summary[0].Key != "error" {
		t.Fatalf("summary=%+v, want first metric key=error", result.Summary)
	}
	if strings.Contains(result.Summary[0].Unit, "secret-tok-xyz") {
		t.Fatalf("upstream text leaked into unit: %q", result.Summary[0].Unit)
	}
}
