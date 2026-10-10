package main

import (
	"strings"
	"testing"
)

func TestEmbeddedWebUISendsManagementKey(t *testing.T) {
	html := string(qoderWebUI)
	for _, required := range []string{
		"cliproxyapi-management-key",
		"Authorization",
		"X-Management-Key",
		"api('/qoder-auth-url'",
		"cli-proxy-auth",
		"managementKey",
		"qoder-cpa-mgmt-key",
		"mgmt-key",
		"未检测到管理密钥",
		"登录失败",
		"id=\"refresh-campaigns\"",
		"api('/qoder/accounts/campaigns/refresh'",
		"api('/qoder/accounts/campaigns/claim'",
		"api('/qoder/accounts/activity/fetch'",
		"id=\"g1-credits\"",
		"id=\"g1-seat\"",
		"id=\"g1-claim\"",
		"action_type === 'CLAIM_BENEFIT'",
		"id=\"login-region\"",
		"region=cn",
		"国际站",
		"国内站",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("embedded web UI missing %q", required)
		}
	}
}

// TestEmbeddedWebUIPriorityEditor 锁优先级编辑接线：9 列表头、行内
// 输入+保存、写路径 PATCH /auth-files/fields（name=priority）。
// 串行单行保存，不并发写 auth 目录。
func TestEmbeddedWebUIPriorityEditor(t *testing.T) {
	html := string(qoderWebUI)
	for _, required := range []string{
		"优先级",
		"auth-files/fields",
		"savePriority",
		"method:'PATCH'",
		"数字越大越优先",
		"colspan=\"9\"",
	} {
		if !strings.Contains(html, required) {
			t.Errorf("embedded web UI missing %q", required)
		}
	}
}
