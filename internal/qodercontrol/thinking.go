package qodercontrol

import (
	"encoding/json"
	"strings"
)

// thinking_config 是上游目录里每个模型的思考档位声明，官方客户端据此决定
// 该模型「默认是否思考」以及「接受哪些 effort 档位」：
//
//	{"disabled":{...},
//	 "enabled":{"is_default":true,
//	            "efforts":{"low":{},"medium":{"is_default":true},"xhigh":{}}}}
//
// 上游网关**不会拒绝**模型不支持的档位，而是静默忽略并回落到模型默认档
// （Orchids-2api d47089e4 实测：客户端向 qfmodel 请求 "high" 既无效果也无报错）。
// 因此必须在出站前把客户端档位映射到该模型自己的档位表上。
type thinkingMode struct {
	IsDefault *bool                      `json:"is_default"`
	Efforts   map[string]json.RawMessage `json:"efforts"`
}

type thinkingConfig struct {
	Enabled  *thinkingMode `json:"enabled"`
	Disabled *thinkingMode `json:"disabled"`
}

// effortRank 给出网关 effort 词表的等级序。
var effortRank = map[string]int{"none": 0, "minimal": 1, "low": 2, "medium": 3, "high": 4, "xhigh": 5, "max": 6}

// effortOrder 是 effortRank 的键按等级升序排列。
var effortOrder = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

// thinkingOff 是客户端用来关闭思考的写法；网关自己的值是 "none"。
var thinkingOff = map[string]bool{
	"none": true, "off": true, "disabled": true, "disable": true,
	"false": true, "0": true, "no": true,
}

// Thinking 是 Model 的思考档位声明（从目录 thinking_config 解析）。
// 零值表示该模型目录里没有声明，此时出站档位原样透传。
type Thinking struct {
	// HasConfig 为 true 表示上游给了可读的 thinking_config 块。
	HasConfig bool
	// DefaultOn 表示调用方没给档位时该模型是否思考。
	DefaultOn bool
	// Efforts 是该模型接受的档位，按等级升序；空表示只有开/关两态。
	Efforts []string
	// DefaultEffort 是该模型的默认档位（目录里 is_default 标记的那个）。
	DefaultEffort string
}

func parseThinking(raw any) Thinking {
	node, ok := raw.(map[string]any)
	if !ok || node == nil {
		return Thinking{}
	}
	enabled, hasEnabled := node["enabled"].(map[string]any)
	disabled, hasDisabled := node["disabled"].(map[string]any)
	if !hasEnabled && !hasDisabled {
		return Thinking{}
	}
	out := Thinking{HasConfig: true}
	decided := false
	if hasEnabled {
		if isDefault, ok := enabled["is_default"].(bool); ok {
			out.DefaultOn, decided = isDefault, true
		}
		if efforts, ok := enabled["efforts"].(map[string]any); ok {
			for _, effort := range effortOrder {
				rawTier, present := efforts[effort]
				if !present {
					continue
				}
				out.Efforts = append(out.Efforts, effort)
				if tier, ok := rawTier.(map[string]any); ok {
					if isDefault, ok := tier["is_default"].(bool); ok && isDefault {
						out.DefaultEffort = effort
					}
				}
			}
		}
	}
	if !decided && hasDisabled {
		// enabled 未标 is_default 时，disabled.is_default 才是权威。
		if isDefault, ok := disabled["is_default"].(bool); ok {
			out.DefaultOn, decided = isDefault, true
		}
	}
	if !decided {
		// 两侧都没标：有 enabled 即视为默认开（上游实际形态如此）。
		out.DefaultOn = hasEnabled
	}
	if out.DefaultEffort == "" && len(out.Efforts) > 0 {
		// 上游没标默认档时取中位档（与 Orchids-2api 同口径）。
		out.DefaultEffort = out.Efforts[len(out.Efforts)/2]
	}
	return out
}

// NormalizeEffort 把客户端请求的档位映射到该模型接受的档位。
//
//   - 任一「关闭」写法都变成 "none"；
//   - 模型没有 thinking_config 时原样透传（不猜）；
//   - 只有开/关两态的模型不发档位（返回 ""，思考仍开着）；
//   - 支持的档位直接通过；不支持的映射到最近的档位，距离相同则偏向模型默认档；
//   - 不认识的词映射到默认档。
func (t Thinking) NormalizeEffort(requested string) string {
	effort := strings.ToLower(strings.TrimSpace(requested))
	if effort == "" {
		return ""
	}
	if thinkingOff[effort] {
		return "none"
	}
	if !t.HasConfig {
		return effort
	}
	if len(t.Efforts) == 0 {
		return ""
	}
	want, known := effortRank[effort]
	if !known {
		return t.DefaultEffort
	}
	best := ""
	for _, candidate := range t.Efforts {
		if candidate == effort {
			return effort
		}
		if best == "" || closerEffort(candidate, best, want, t.DefaultEffort) {
			best = candidate
		}
	}
	return best
}

func closerEffort(candidate, best string, want int, defaultEffort string) bool {
	distance := func(effort string) int { return abs(effortRank[effort] - want) }
	if distance(candidate) != distance(best) {
		return distance(candidate) < distance(best)
	}
	pull := effortRank[defaultEffort]
	return abs(effortRank[candidate]-pull) < abs(effortRank[best]-pull)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
