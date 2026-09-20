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
		"api('/qoder-auth-url')",
		"cli-proxy-auth",
		"managementKey",
		"qoder-cpa-mgmt-key",
		"mgmt-key",
		"未检测到管理密钥",
		"登录失败",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("embedded web UI missing %q", required)
		}
	}
}
