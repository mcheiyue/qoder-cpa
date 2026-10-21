package main

import (
	"encoding/json"
	"testing"
)

// 锁：宿主 saveAuthFile 是裸写（os.WriteFile，零合并），插件载荷只含自有键。
// 不合并会抹掉宿主写入的 priority —— 2026-10-21 现网实证：qoder 四账号
// priority 全为 None，而同日 workbuddy 八个账号 priority 完好（它已做合并）。
func TestMergeForeignAuthMetadataKeepsHostKeys(t *testing.T) {
	old := []byte(`{"access_token":"old-token","priority":5,"disabled":false,"note":"mine"}`)
	fresh := []byte(`{"access_token":"new-token","email":"18074073866"}`)

	merged, err := mergeForeignAuthMetadata(old, fresh)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("decode merged: %v", err)
	}
	if got["priority"] != float64(5) {
		t.Fatalf("priority lost after merge: %v", got["priority"])
	}
	if got["note"] != "mine" {
		t.Fatalf("note lost after merge: %v", got["note"])
	}
	// 自有键必须以新载荷为准，不能从旧文件复活已轮换的 token。
	if got["access_token"] != "new-token" {
		t.Fatalf("own key must come from fresh payload, got %v", got["access_token"])
	}
	if got["email"] != "18074073866" {
		t.Fatalf("fresh key missing: %v", got["email"])
	}
}

// 锁：旧文件不存在/为空时不得报错，必须直接返回新载荷（首次登录写回走这条路）。
func TestMergeForeignAuthMetadataEmptyOld(t *testing.T) {
	fresh := []byte(`{"access_token":"new-token"}`)
	merged, err := mergeForeignAuthMetadata(nil, fresh)
	if err != nil {
		t.Fatalf("merge with empty old: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("decode merged: %v", err)
	}
	if got["access_token"] != "new-token" {
		t.Fatalf("payload changed: %v", merged)
	}
}
