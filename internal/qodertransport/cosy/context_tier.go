package cosy

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// F3b contextTier (offline fixture stage, PLAN "contextTier 预研收口" 2026-10-03).
// Mirrors 9router open-sse/shared/qoder/contextTier.js semantics against our
// live capture shape (map form, Cosy-Version 1.1.34) with array form tolerated
// defensively. Pure functions only — nothing in BuildChatBody calls these yet;
// wiring happens only after real-line qualification of the escalated fields.

// ContextTier is one advertised context-window tier of a model_config.
type ContextTier struct {
	Name       string
	TokenCount int
	IsDefault  bool
}

// contextTierHeadroom matches 9router QODER_CONTEXT_TIER_HEADROOM: the prompt
// estimate is padded 15% before comparing against the current limit.
const contextTierHeadroom = 0.15

// ParseContextTiers extracts context_config tiers from a raw model_config map.
// Live shape is a map keyed by tier name ({"200K":{"token_count":200000,...}});
// the array shape from 9router fixtures is tolerated defensively. Returns tiers
// sorted ascending by TokenCount; empty when context_config is absent/unusable.
func ParseContextTiers(raw map[string]any) []ContextTier {
	if raw == nil {
		return nil
	}
	var out []ContextTier
	switch cc := raw["context_config"].(type) {
	case map[string]any:
		for name, v := range cc {
			tc := 0
			isDefault := false
			if m, ok := v.(map[string]any); ok {
				tc = intFromAny(m["token_count"])
				if tc == 0 {
					tc = intFromAny(m["tokenCount"])
				}
				isDefault = boolFromAny(m["is_default"]) || boolFromAny(m["isDefault"])
			}
			if tc == 0 {
				tc = parseTierTokenCount(name)
			}
			if tc <= 0 {
				continue
			}
			out = append(out, ContextTier{Name: name, TokenCount: tc, IsDefault: isDefault})
		}
	case []any:
		for _, e := range cc {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			tc := intFromAny(m["tokenCount"])
			if tc == 0 {
				tc = intFromAny(m["token_count"])
			}
			if tc == 0 {
				tc = intFromAny(m["max_input_tokens"])
			}
			if tc <= 0 {
				continue
			}
			name := ""
			if s, ok := m["name"].(string); ok {
				name = s
			}
			if name == "" {
				name = tierNameFromCount(tc)
			}
			isDefault := boolFromAny(m["isDefault"]) || boolFromAny(m["is_default"]) || boolFromAny(m["default"])
			out = append(out, ContextTier{Name: name, TokenCount: tc, IsDefault: isDefault})
		}
	default:
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TokenCount < out[j].TokenCount })
	return out
}

// EstimatePromptTokens approximates prompt size with CJK weighting: each CJK
// character counts ~1 token, everything else ~4 chars/token (the plain len/4
// rule underestimates Chinese up to 4x — exactly when tier decisions matter).
func EstimatePromptTokens(text string) int {
	var cjk, total int
	for _, r := range text {
		total++
		if isCJKRune(r) {
			cjk++
		}
	}
	return int(math.Ceil(float64(cjk) + float64(total-cjk)/4))
}

// ResolveContextTier picks the smallest advertised tier that fits the padded
// estimate while exceeding the model's current limit. Returns nil when the
// current limit already fits (wire must stay byte-identical) or when there are
// no tiers — matching 9router's auto mode; forced modes (max/default/named)
// are deliberately not implemented until an operational need shows up.
func ResolveContextTier(raw map[string]any, estimatedTokens int) *ContextTier {
	tiers := ParseContextTiers(raw)
	if len(tiers) == 0 {
		return nil
	}
	defaultTier := tiers[0]
	for i := range tiers {
		if tiers[i].IsDefault {
			defaultTier = tiers[i]
			break
		}
	}
	currentLimit := intFromAny(raw["max_input_tokens"])
	if currentLimit <= 0 {
		currentLimit = defaultTier.TokenCount
	}
	need := int(math.Ceil(float64(estimatedTokens) * (1 + contextTierHeadroom)))
	if need <= currentLimit {
		return nil
	}
	var fit *ContextTier
	for i := range tiers {
		if tiers[i].TokenCount >= need && tiers[i].TokenCount > currentLimit {
			fit = &tiers[i]
			break
		}
	}
	if fit == nil {
		fit = &tiers[len(tiers)-1] // auto:largest fallback
	}
	if fit.TokenCount <= currentLimit {
		return nil
	}
	return fit
}

// parseTierTokenCount parses "200K"/"1M"/"131072" → integer token count, 0 when unparseable.
func parseTierTokenCount(v string) int {
	s := strings.ToUpper(strings.TrimSpace(v))
	if s == "" {
		return 0
	}
	mult := 1
	switch {
	case strings.HasSuffix(s, "K"):
		mult = 1000
		s = strings.TrimSuffix(s, "K")
	case strings.HasSuffix(s, "M"):
		mult = 1000000
		s = strings.TrimSuffix(s, "M")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return n * mult
}

func tierNameFromCount(tc int) string {
	if tc >= 1000000 && tc%1000000 == 0 {
		return strconv.Itoa(tc/1000000) + "M"
	}
	if tc >= 1000 && tc%1000 == 0 {
		return strconv.Itoa(tc/1000) + "K"
	}
	return strconv.Itoa(tc)
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		return parseTierTokenCount(n)
	}
	return 0
}

func boolFromAny(v any) bool {
	b, _ := v.(bool)
	return b
}

func isCJKRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x11ff) || // Hangul Jamo
		(r >= 0x2e80 && r <= 0x9fff) || // CJK radicals + unified ideographs
		(r >= 0xac00 && r <= 0xd7af) || // Hangul syllables
		(r >= 0xf900 && r <= 0xfaff) || // CJK compatibility ideographs
		(r >= 0xff00 && r <= 0xffef) // fullwidth/halfwidth forms
}
