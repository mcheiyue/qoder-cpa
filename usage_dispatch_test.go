package main

import (
	"encoding/json"
	"testing"
)

// H1③: registration 必须声明 usage_plugin 能力（宿主 usage 推送通道）。
func TestRegistrationDeclaresUsagePlugin(t *testing.T) {
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
	if reg.Capabilities["usage_plugin"] != true {
		t.Fatalf("usage_plugin = %v, want true", reg.Capabilities["usage_plugin"])
	}
}

// H1③: usage.handle 必须有分发（workbuddy 同构占位 not_implemented，不是 unknown_method）。
// 执行出口的 credit/tokens 上报已由 F6 aggregateUsage payload 覆盖（宿主自动解析），此处锁宿主→插件推送通道。
func TestUsageHandleDispatched(t *testing.T) {
	resp, err := handleMethod("usage.handle", []byte(`{"model":"qfmodel"}`))
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("bad envelope %s: %v", resp, err)
	}
	if env.OK {
		return // 真消费实现也接受
	}
	if env.Error == nil || env.Error.Code == "unknown_method" {
		t.Fatalf("usage.handle must be dispatched, got: %s", resp)
	}
}
