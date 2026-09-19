package qodertransport

import (
	"errors"
	"strings"
	"testing"
)

func TestExplicitProfile_Parse_cosy_api2(t *testing.T) {
	profile, err := ParseTransportProfile("cosy-api2")
	if err != nil {
		t.Fatalf("ParseTransportProfile: %v", err)
	}
	if profile != ProfileCosyAPI2 {
		t.Errorf("got %q, want %q", profile, ProfileCosyAPI2)
	}
}

func TestExplicitProfile_Parse_cosy_api3(t *testing.T) {
	profile, err := ParseTransportProfile("cosy-api3")
	if err != nil {
		t.Fatalf("ParseTransportProfile: %v", err)
	}
	if profile != ProfileCosyAPI3 {
		t.Errorf("got %q, want %q", profile, ProfileCosyAPI3)
	}
}

func TestExplicitProfile_Parse_bearer_openai(t *testing.T) {
	profile, err := ParseTransportProfile("bearer-openai")
	if err != nil {
		t.Fatalf("ParseTransportProfile: %v", err)
	}
	if profile != ProfileBearerOpenAI {
		t.Errorf("got %q, want %q", profile, ProfileBearerOpenAI)
	}
}

func TestExplicitProfile_Parse_unknown_returns_config_error(t *testing.T) {
	_, err := ParseTransportProfile("totally-unknown")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	var configErr *ConfigError
	if !errors.As(err, &configErr) {
		t.Errorf("want ConfigError, got %T", err)
	}
}

func TestExplicitProfile_Parse_unknown_error_contains_value(t *testing.T) {
	_, err := ParseTransportProfile("totally-unknown")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "totally-unknown") {
		t.Errorf("error should contain the unknown value, got: %s", err.Error())
	}
}

func TestExplicitProfile_Resolve_empty_returns_default(t *testing.T) {
	profile, err := ResolveProfile("")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if profile != DefaultTransportProfile {
		t.Errorf("got %q, want %q", profile, DefaultTransportProfile)
	}
}

func TestExplicitProfile_Resolve_valid_passes_through(t *testing.T) {
	profile, err := ResolveProfile("bearer-openai")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if profile != ProfileBearerOpenAI {
		t.Errorf("got %q, want %q", profile, ProfileBearerOpenAI)
	}
}

func TestExplicitProfile_Resolve_unknown_returns_error(t *testing.T) {
	_, err := ResolveProfile("bogus")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestExplicitProfile_Default_is_cosy_api2(t *testing.T) {
	if DefaultTransportProfile != ProfileCosyAPI2 {
		t.Errorf("DefaultTransportProfile=%q, want %q", DefaultTransportProfile, ProfileCosyAPI2)
	}
}
