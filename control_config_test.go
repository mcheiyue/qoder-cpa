package main

import (
	"strings"
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// 锁：管理面控制端点必须按凭证 profile 选基址——cosy-cn 走 CN 站
// （openapi.qoder.com.cn），其余（cosy-api2 / cosy-api3 / bearer-openai）走国际站。
// 写死国际站会让 CN 账号的 campaigns/quota/credits 打到错站。
// 实证：CN 站 /sash/api/v1/me/campaigns 对国际站 token 返回 401 TOKEN_EXPIRE
// （端点存在，仅 token 域不符）——故 CN 基址对 sash 路径成立。
func TestControlConfigForProfile(t *testing.T) {
	cn := controlConfigForProfile(qoderauth.TransportProfileCosyCN)
	if !strings.Contains(cn.BaseURL, "qoder.com.cn") {
		t.Fatalf("cosy-cn BaseURL=%q, want CN host (openapi.qoder.com.cn)", cn.BaseURL)
	}
	for _, p := range []qoderauth.TransportProfile{
		qoderauth.TransportProfileCosyAPI2,
		qoderauth.TransportProfileCosyAPI3,
		qoderauth.TransportProfileBearerOpenAI,
	} {
		cfg := controlConfigForProfile(p)
		if !strings.Contains(cfg.BaseURL, "openapi.qoder.sh") {
			t.Fatalf("profile %q BaseURL=%q, want intl host (openapi.qoder.sh)", p, cfg.BaseURL)
		}
	}
}
