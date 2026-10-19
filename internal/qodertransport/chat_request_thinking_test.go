package qodertransport

import (
	"testing"

	"github.com/mcheiyue/qoder-cpa/internal/qodercontrol"
)

// 锁：出站 reasoning_effort 必须是该模型目录声明的档位之一。
// 上游对不支持的档位静默忽略（不报错、不生效），所以若把客户端的 "high"
// 原样发给只支持 low/medium 的模型，用户会「设了没效果」且无从排查。
func TestToCosyRequest_EffortClampedToModelTiers(t *testing.T) {
	thinking := qodercontrol.Thinking{
		HasConfig: true, DefaultOn: true,
		Efforts: []string{"low", "medium"}, DefaultEffort: "medium",
	}
	raw := []byte(`{"model":"qoder/r1","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, ModelResolver(func(string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: true, MaxInputTokens: 131072, Thinking: thinking}
	}))
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters=nil, want effort/thinking set")
	}
	if got := c.Parameters.ReasoningEffort; got != "medium" {
		t.Errorf("ReasoningEffort=%q, want medium (clamped from high)", got)
	}
	if c.Parameters.EnableThinking == nil || !*c.Parameters.EnableThinking {
		t.Error("EnableThinking must stay true when a tier was requested")
	}
}

// 锁：只有开/关两态的模型不得收到任何档位字符串（只发 EnableThinking）。
func TestToCosyRequest_OnOffModelSendsNoEffort(t *testing.T) {
	thinking := qodercontrol.Thinking{HasConfig: true, DefaultOn: true}
	raw := []byte(`{"model":"qoder/r1","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, ModelResolver(func(string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: true, Thinking: thinking}
	}))
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters=nil, want EnableThinking set")
	}
	if got := c.Parameters.ReasoningEffort; got != "" {
		t.Errorf("ReasoningEffort=%q, want empty for on/off-only model", got)
	}
	if c.Parameters.EnableThinking == nil || !*c.Parameters.EnableThinking {
		t.Error("EnableThinking must be true")
	}
}

// 锁：模型目录没有 thinking_config 时保持改动前行为——档位原样透传。
func TestToCosyRequest_NoThinkingConfigPassesEffortThrough(t *testing.T) {
	raw := []byte(`{"model":"qoder/r1","reasoning_effort":"xhigh","messages":[{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, ModelResolver(func(string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: true}
	}))
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil || c.Parameters.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort=%v, want xhigh pass-through", c.Parameters)
	}
}

// 锁：客户端显式关闭思考（reasoning_effort=none）时，即使模型默认思考也必须关掉，
// 且不得发出任何档位值。
func TestToCosyRequest_NoneDisablesThinkingOnDefaultOnModel(t *testing.T) {
	thinking := qodercontrol.Thinking{
		HasConfig: true, DefaultOn: true,
		Efforts: []string{"low", "high"}, DefaultEffort: "high",
	}
	raw := []byte(`{"model":"qoder/r1","reasoning_effort":"none","messages":[{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, ModelResolver(func(string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: true, Thinking: thinking}
	}))
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil {
		t.Fatal("Parameters=nil, want EnableThinking=false")
	}
	if c.Parameters.EnableThinking == nil || *c.Parameters.EnableThinking {
		t.Error("EnableThinking must be false when client asked for none")
	}
	if got := c.Parameters.ReasoningEffort; got != "" {
		t.Errorf("ReasoningEffort=%q, want empty when thinking off", got)
	}
}

// 锁：模型默认思考（enabled.is_default）而客户端未给档位时，
// 必须发 EnableThinking=true——目录的 is_reasoning 可能与之矛盾。
func TestToCosyRequest_DefaultOnWithoutEffortEnablesThinking(t *testing.T) {
	thinking := qodercontrol.Thinking{
		HasConfig: true, DefaultOn: true,
		Efforts: []string{"low", "high"}, DefaultEffort: "high",
	}
	raw := []byte(`{"model":"qoder/r1","messages":[{"role":"user","content":"hi"}]}`)
	p, err := parseChatPayload(raw, ModelResolver(func(string) ResolvedModel {
		return ResolvedModel{InternalID: "r1", IsReasoning: false, Thinking: thinking}
	}))
	if err != nil {
		t.Fatal(err)
	}
	c, err := toCosyRequest(p, StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Parameters == nil || c.Parameters.EnableThinking == nil || !*c.Parameters.EnableThinking {
		t.Errorf("EnableThinking must be true when thinking_config says default-on: %+v", c.Parameters)
	}
	if got := c.Parameters.ReasoningEffort; got != "" {
		t.Errorf("ReasoningEffort=%q, want empty when client gave no tier", got)
	}
}
