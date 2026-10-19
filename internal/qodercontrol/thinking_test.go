package qodercontrol

import "testing"

// 锁：上游网关对模型不支持的 effort 档位是「静默忽略并回落默认档」，不报错。
// 因此客户端请求 high 而模型只有 low/medium 时，必须夹到最近档；
// 若原样透传，用户会看到「设了档位但没效果」且没有任何错误提示。
func TestNormalizeEffort_MapsOntoModelTiers(t *testing.T) {
	// 目录实测形态（qoder 现网 /algo/api/v2/model/list）：
	// enabled.efforts = {xhigh, high(is_default), low, max, medium}
	full := parseThinking(map[string]any{
		"enabled": map[string]any{
			"is_default": true,
			"efforts": map[string]any{
				"low":    map[string]any{},
				"medium": map[string]any{},
				"high":   map[string]any{"is_default": true},
				"xhigh":  map[string]any{},
				"max":    map[string]any{},
			},
		},
	})
	if !full.HasConfig || !full.DefaultOn {
		t.Fatalf("HasConfig=%v DefaultOn=%v, want true/true", full.HasConfig, full.DefaultOn)
	}
	if full.DefaultEffort != "high" {
		t.Errorf("DefaultEffort=%q, want high", full.DefaultEffort)
	}

	cases := map[string]string{
		"low":     "low",  // 支持档位原样通过
		"high":    "high", // 支持档位原样通过
		"minimal": "low",  // 不支持 → 最近档
		"none":    "none", // 关闭写法归一
		"off":     "none", // 关闭写法归一
		"bogus":   "high", // 不认识的词 → 默认档
	}
	for in, want := range cases {
		if got := full.NormalizeEffort(in); got != want {
			t.Errorf("NormalizeEffort(%q)=%q, want %q", in, got, want)
		}
	}
}

// 锁：只有开/关两态的模型（efforts 为空）不能收到任何档位值——
// 发档位会被上游当成无意义字段，且可能干扰其 on/off 判定。
func TestNormalizeEffort_OnOffOnlyModelSendsNoTier(t *testing.T) {
	onOff := parseThinking(map[string]any{
		"enabled": map[string]any{"is_default": true},
	})
	if !onOff.HasConfig {
		t.Fatal("HasConfig=false, want true")
	}
	if len(onOff.Efforts) != 0 {
		t.Fatalf("Efforts=%v, want empty", onOff.Efforts)
	}
	for _, in := range []string{"low", "high", "max"} {
		if got := onOff.NormalizeEffort(in); got != "" {
			t.Errorf("NormalizeEffort(%q)=%q, want empty (on/off-only model)", in, got)
		}
	}
	if got := onOff.NormalizeEffort("none"); got != "none" {
		t.Errorf(`NormalizeEffort("none")=%q, want "none" (off must survive)`, got)
	}
}

// 锁：模型目录没有 thinking_config 时不得猜测——原样透传客户端档位，
// 保持改动前的行为（否则会给不认识档位体系的模型凭空夹取）。
func TestNormalizeEffort_NoConfigPassesThrough(t *testing.T) {
	var none Thinking
	if none.HasConfig {
		t.Fatal("zero Thinking has HasConfig=true")
	}
	for _, in := range []string{"low", "high", "xhigh", "max"} {
		if got := none.NormalizeEffort(in); got != in {
			t.Errorf("NormalizeEffort(%q)=%q, want pass-through", in, got)
		}
	}
	if got := none.NormalizeEffort(""); got != "" {
		t.Errorf("empty effort: got %q, want empty", got)
	}
}

// 锁：目录里 is_reasoning 与 thinking_config 可能矛盾（enabled 默认开但
// is_reasoning=false）。默认档必须由 enabled.is_default 决定，不能看 is_reasoning。
func TestParseThinking_DefaultFromIsDefaultFlags(t *testing.T) {
	on := parseThinking(map[string]any{
		"enabled":  map[string]any{"is_default": true},
		"disabled": map[string]any{"is_default": false},
	})
	if !on.DefaultOn {
		t.Error("enabled.is_default=true → DefaultOn must be true")
	}
	off := parseThinking(map[string]any{
		"enabled":  map[string]any{"is_default": false},
		"disabled": map[string]any{"is_default": true},
	})
	if off.DefaultOn {
		t.Error("disabled.is_default=true → DefaultOn must be false")
	}
	// 两个都没标 is_default：有 enabled 视为默认开（与上游实际形态一致）。
	bare := parseThinking(map[string]any{"enabled": map[string]any{}})
	if !bare.DefaultOn {
		t.Error("enabled present without flags → DefaultOn must be true")
	}
}
