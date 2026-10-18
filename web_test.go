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
// 「区域」列区分国内外、倍率殿后；免费档绿标、促销角标透出。「来源账号」列已删除——
// 同一上游目录被 8 个账号各复制一份，该列只是噪音。区域列不可省：国际站与 CN 站
// 对同一模型给出不同倍率（实测 deepseek-v4.1-flash 国际 x0.00 / 国内 x0.11），
// 合并成一行会让另一站的真实值被静默丢弃。
func TestEmbeddedWebUIModelsRateColumn(t *testing.T) {
	html := string(qoderWebUI)
	for _, required := range []string{
		"<th>调用 ID</th><th>展示名</th><th>区域</th><th>倍率</th>",
		"function rateCell",
		"function regionLabel",
		"model.profile",
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
