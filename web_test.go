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

// TestEmbeddedWebUIModelsRateColumn 锁模型目录契约：调用 ID 在前、展示名次之、
// 全球/国内倍率分列（与 workbuddy 面板同款），免费档绿标、促销角标透出。
// 「来源账号」列已删除——同一上游目录被 8 个账号各复制一份，该列只是噪音。
// 分列而非分行：现网 25 个模型跨区域倍率差异为 0，拆成两行只是重复信息。
func TestEmbeddedWebUIModelsRateColumn(t *testing.T) {
	html := string(qoderWebUI)
	for _, required := range []string{
		"<th>调用 ID</th><th>展示名</th><th>全球</th><th>国内</th>",
		"function rateCell",
		"function mergeModels",
		"row.profile === 'cosy-cn'",
		"免费",
		"model.is_free",
		"model.badge",
	} {
		if !strings.Contains(html, required) {
			t.Errorf("embedded web UI missing %q", required)
		}
	}
	if strings.Contains(html, "来源账号") {
		t.Error("模型目录的「来源账号」列应已删除")
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
