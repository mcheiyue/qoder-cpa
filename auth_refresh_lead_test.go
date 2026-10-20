package main

import (
	"testing"
	"time"

	"github.com/mcheiyue/qoder-cpa/internal/qoderauth"
)

// 锁：NextRefreshAfter 必须是「过期前 5 分钟」而不是过期时刻。
// 宿主 auto_refresh_loop 在 NextRefreshAfter 到点时才发 refresh 请求；
// 若等于过期时刻，一次失败就等于 token 已死、账号立刻不可用。
// 提前量给了重试余量。ProviderRefreshLead("qoder") 返回 nil（宿主只注册了
// codex/claude/antigravity/kimi/xai/devin/meta），所以只能由插件自己提前。
func TestAuthDataRefreshesBeforeExpiry(t *testing.T) {
	expiry := time.Now().Add(2 * time.Hour)
	cred := qoderauth.Credential{
		AccessToken: "a", RefreshToken: "r", UserID: "u1",
		ExpiresAt: expiry, Profile: qoderauth.TransportProfileCosyAPI2,
	}
	data, err := authData(cred, "")
	if err != nil {
		t.Fatalf("authData: %v", err)
	}
	want := expiry.Add(-refreshLead)
	if d := data.NextRefreshAfter.Sub(want); d > time.Second || d < -time.Second {
		t.Errorf("NextRefreshAfter=%v, want ~%v (expiry-5m)", data.NextRefreshAfter, want)
	}
	if !data.NextRefreshAfter.Before(expiry) {
		t.Error("NextRefreshAfter must be strictly before expiry, else a failed refresh leaves a dead token")
	}
}

// 锁：已过期的凭证不得算出「过去很久」的刷新时刻（会让宿主立刻重刷并反复失败）。
// 提前量使 NextRefreshAfter 落在过去时，必须夹到现在，避免负等待。
func TestAuthDataRefreshTimeNeverInPast(t *testing.T) {
	cred := qoderauth.Credential{
		AccessToken: "a", RefreshToken: "r", UserID: "u1",
		ExpiresAt: time.Now().Add(-time.Hour), Profile: qoderauth.TransportProfileCosyAPI2,
	}
	data, err := authData(cred, "")
	if err != nil {
		t.Fatalf("authData: %v", err)
	}
	if data.NextRefreshAfter.Before(time.Now().Add(-time.Minute)) {
		t.Errorf("NextRefreshAfter=%v is far in the past; must clamp to now", data.NextRefreshAfter)
	}
}
