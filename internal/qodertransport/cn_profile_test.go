package qodertransport

import "testing"

func TestParseAndResolveProfileCosyCN(t *testing.T) {
	p, err := ParseTransportProfile("cosy-cn")
	if err != nil {
		t.Fatalf("ParseTransportProfile(cosy-cn): %v", err)
	}
	if p != ProfileCosyCN {
		t.Errorf("profile=%q, want %q", p, ProfileCosyCN)
	}
	resolved, err := ResolveProfile("cosy-cn")
	if err != nil || resolved != ProfileCosyCN {
		t.Errorf("ResolveProfile(cosy-cn)=(%q, %v), want %q", resolved, err, ProfileCosyCN)
	}
	empty, err := ResolveProfile("")
	if err != nil || empty != DefaultTransportProfile {
		t.Errorf("ResolveProfile(EMPTY)=(%q, %v), want default %q unchanged", empty, err, DefaultTransportProfile)
	}
}

func TestSelectorFourthSlotForCosyCN(t *testing.T) {
	c2, c3, b, cn := &fakeAdapter{}, &fakeAdapter{}, &fakeAdapter{}, &fakeAdapter{}
	sel, err := NewSelector(c2, c3, b, cn)
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}
	if got, err := sel.Select("cosy-cn"); err != nil || got != ChatTransport(cn) {
		t.Errorf("Select(cosy-cn)=(%p, %v), want 4th adapter", got, err)
	}
	if got, err := sel.Select(""); err != nil || got != ChatTransport(c2) {
		t.Errorf("Select(EMPTY)=(%p, %v), want default cosy-api2 adapter", got, err)
	}
	if got, err := sel.Select("bearer-openai"); err != nil || got != ChatTransport(b) {
		t.Errorf("Select(bearer-openai)=(%p, %v), want bearer adapter", got, err)
	}
}

func TestNewSelectorRequiresFourthAdapter(t *testing.T) {
	if _, err := NewSelector(&fakeAdapter{}, &fakeAdapter{}, &fakeAdapter{}, nil); err == nil {
		t.Fatal("NewSelector with nil cosy-cn adapter: want AdapterSetError, got nil")
	}
}
