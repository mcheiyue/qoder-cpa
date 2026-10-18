package main

import (
	"encoding/json"
	"testing"
)

// 锁：registration 不得声明 usage_plugin。
//
// 宿主 internal/pluginhost/adapters_usage_translation.go 的 RegisterUsagePlugins()
// 对 activeRecords() 里每个声明了 UsagePlugin 的插件**全局**注册（不按 provider 过滤），
// 于是 CPA 里任何 provider 的每条 usage 记录都会给本插件发一次 usage.handle RPC；
// 而本插件对该方法只返回 not_implemented（见 usage_dispatch_test.go），
// 结果就是每笔请求都多一次失败 RPC + 一行 debug 日志，纯空转。
//
// credits/tokens 上报走 executor 的 F6 aggregateUsage payload（宿主自动解析），
// 与 usage.handle 无关 —— 所以删掉这条声明不会丢失任何用量数据。
func TestRegistrationDoesNotDeclareUsagePlugin(t *testing.T) {
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
	if _, exists := reg.Capabilities["usage_plugin"]; exists {
		t.Fatalf("usage_plugin must not be declared (handler only returns not_implemented): %v", reg.Capabilities["usage_plugin"])
	}
}

// 锁：usage.handle 仍必须被分发（返回 not_implemented 而非 unknown_method）。
//
// 保留这条是因为宿主升级后若改为「按 provider 过滤 + 支持消费」，我们需要的是
// 补齐实现，而不是让方法变成 unknown_method（那会被当作协议错误）。
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
		return
	}
	if env.Error == nil || env.Error.Code == "unknown_method" {
		t.Fatalf("usage.handle must be dispatched, got: %s", resp)
	}
}
