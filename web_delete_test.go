package main

import (
	"strings"
	"testing"
)

// TestWebUIAccountsDeleteButton 锁 WebUI 删除账号接线：账号行必须有删除按钮，
// 且删除走宿主自带 DELETE /v0/management/auth-files（插件 SDK 只暴露
// auth.get/list/save，没有 delete，只能复用既有 api() 直连宿主管理面），
// 删除成功后必须重新 load()，否则界面仍显示已删账号。
func TestWebUIAccountsDeleteButton(t *testing.T) {
	html := string(qoderWebUI)
	if html == "" {
		t.Fatal("qoderWebUI empty")
	}
	for _, want := range []string{
		"deleteAccount",     // 行内按钮入口
		"data-delete",       // 行内定位键（宿主文件名）
		"method:'DELETE'",   // DELETE 动词（api() 透传 options.method）
		"/auth-files?name=", // 宿主删除端点 + 定位键
		"需重新登录才能恢复",         // 二次确认必须说明不可逆
	} {
		if !strings.Contains(html, want) {
			t.Errorf("web/index.html missing %q", want)
		}
	}
}
