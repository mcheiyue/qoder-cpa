package cosy

import (
	"encoding/json"
	"strings"
	"testing"
)

// F3b contextTier offline fixture (pre-research 2026-10-03, PLAN "contextTier 预研收口").
// Locks tier parsing (map form = our live capture, array form = 9router fixture),
// CJK-aware estimation, and the auto resolve decision — including the default-state
// identity lock: a short prompt must resolve to nil, i.e. zero wire change.
// Zero wiring: nothing in BuildChatBody calls these yet (real-line qualification pending).

// Real capture shape: f3b_catalog_test.go raw Temp\qoder-f3b-catalog-1790954592.json
const f3bCaptureModelConfig = `{
  "key": "qfmodel",
  "max_input_tokens": 180000,
  "context_config": {
    "1M": {"token_count": 1000000},
    "200K": {"token_count": 200000, "is_default": true},
    "400K": {"token_count": 400000}
  }
}`

// 9router fixture shape (tests/unit/qoder-context-tier.test.js:26).
const f3bArrayModelConfig = `{
  "key": "qmodel_38max",
  "max_input_tokens": 180000,
  "context_config": [
    {"name": "200K", "tokenCount": 200000, "isDefault": true},
    {"name": "400K", "tokenCount": 400000, "isDefault": false},
    {"name": "1M", "tokenCount": 1000000, "isDefault": false}
  ]
}`

func f3bDecodeRaw(t *testing.T, s string) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return raw
}

func TestParseContextTiersMapForm(t *testing.T) {
	tiers := ParseContextTiers(f3bDecodeRaw(t, f3bCaptureModelConfig))
	if len(tiers) != 3 {
		t.Fatalf("tiers = %d, want 3", len(tiers))
	}
	want := []int{200000, 400000, 1000000} // ascending
	for i, w := range want {
		if tiers[i].TokenCount != w {
			t.Fatalf("tiers[%d].TokenCount = %d, want %d", i, tiers[i].TokenCount, w)
		}
	}
	if !tiers[0].IsDefault || tiers[1].IsDefault || tiers[2].IsDefault {
		t.Fatalf("default flags = %v %v %v, want true false false", tiers[0].IsDefault, tiers[1].IsDefault, tiers[2].IsDefault)
	}
	if tiers[0].Name != "200K" {
		t.Fatalf("tiers[0].Name = %q, want 200K", tiers[0].Name)
	}
}

func TestParseContextTiersArrayForm(t *testing.T) {
	tiers := ParseContextTiers(f3bDecodeRaw(t, f3bArrayModelConfig))
	if len(tiers) != 3 {
		t.Fatalf("tiers = %d, want 3", len(tiers))
	}
	if tiers[0].TokenCount != 200000 || !tiers[0].IsDefault || tiers[0].Name != "200K" {
		t.Fatalf("tiers[0] = %+v, want 200K/200000/default", tiers[0])
	}
	if tiers[2].TokenCount != 1000000 {
		t.Fatalf("tiers[2].TokenCount = %d, want 1000000", tiers[2].TokenCount)
	}
}

func TestParseContextTiersAbsent(t *testing.T) {
	if tiers := ParseContextTiers(map[string]any{"max_input_tokens": 131072}); len(tiers) != 0 {
		t.Fatalf("absent context_config → %d tiers, want 0", len(tiers))
	}
	if tiers := ParseContextTiers(nil); len(tiers) != 0 {
		t.Fatalf("nil config → %d tiers, want 0", len(tiers))
	}
}

func TestEstimatePromptTokensCJK(t *testing.T) {
	ascii := EstimatePromptTokens(`{"messages":[{"role":"user","content":"` + strings.Repeat("a", 4000) + `"}]}`)
	cjk := EstimatePromptTokens(`{"messages":[{"role":"user","content":"` + strings.Repeat("中", 4000) + `"}]}`)
	if ascii >= 1500 {
		t.Fatalf("ascii estimate = %d, want < 1500 (~4 chars/token)", ascii)
	}
	if cjk <= 4000 {
		t.Fatalf("cjk estimate = %d, want > 4000 (CJK ~1 token/char)", cjk)
	}
	if cjk <= 3*ascii {
		t.Fatalf("cjk %d not ≫ ascii %d — CJK weighting missing", cjk, ascii)
	}
}

func TestResolveContextTierNilWhenFits(t *testing.T) {
	// Default-state identity: a prompt that fits must resolve to nil so the
	// wire stays byte-identical to current production (F0b guardian).
	if tier := ResolveContextTier(f3bDecodeRaw(t, f3bCaptureModelConfig), 100000); tier != nil {
		t.Fatalf("short prompt → %+v, want nil (wire identity)", tier)
	}
}

func TestResolveContextTierEscalates(t *testing.T) {
	raw := f3bDecodeRaw(t, f3bCaptureModelConfig)
	// est 160000 × 1.15 = 184000 > currentLimit 180000 → smallest fitting tier = 200K.
	tier := ResolveContextTier(raw, 160000)
	if tier == nil || tier.TokenCount != 200000 {
		t.Fatalf("est 160000 → %+v, want 200000", tier)
	}
	// est 300000 × 1.15 = 345000 → 400K (fits, and > 180000).
	tier = ResolveContextTier(raw, 300000)
	if tier == nil || tier.TokenCount != 400000 {
		t.Fatalf("est 300000 → %+v, want 400000", tier)
	}
}

func TestResolveContextTierLargestFallback(t *testing.T) {
	// est 900000 × 1.15 = 1035000 > 1M → fall back to largest tier (still > currentLimit).
	tier := ResolveContextTier(f3bDecodeRaw(t, f3bCaptureModelConfig), 900000)
	if tier == nil || tier.TokenCount != 1000000 {
		t.Fatalf("est 900000 → %+v, want 1000000", tier)
	}
}

func TestResolveContextTierNoTiers(t *testing.T) {
	raw := map[string]any{"max_input_tokens": 131072}
	if tier := ResolveContextTier(raw, 900000); tier != nil {
		t.Fatalf("no tiers → %+v, want nil", tier)
	}
}
