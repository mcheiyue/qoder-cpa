// Package qodertransport defines the explicit ChatTransport profile seam for
// Qoder adapters. Each request resolves to exactly one adapter via a
// TransportProfile; there is no automatic fallback or retry.
package qodertransport

import (
	"context"
	"fmt"
)

// TransportProfile identifies which upstream adapter handles a chat request.
type TransportProfile string

const (
	ProfileCosyAPI2     TransportProfile = "cosy-api2"
	ProfileCosyAPI3     TransportProfile = "cosy-api3"
	ProfileBearerOpenAI TransportProfile = "bearer-openai"

	// DefaultTransportProfile is the compile-time fallback when the credential
	// stores an empty profile string. It must never change dynamically.
	DefaultTransportProfile = ProfileCosyAPI2
)

// ConfigError is returned when a TransportProfile value is not one of the
// three legal profiles. Callers can extract it via errors.As.
type ConfigError struct {
	Value string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("qodertransport: unknown transport profile %q", e.Value)
}

// AdapterSetError reports a profile that has no configured adapter.
type AdapterSetError struct {
	Profile TransportProfile
}

func (e *AdapterSetError) Error() string {
	return fmt.Sprintf("qodertransport: missing adapter for profile %q", e.Profile)
}

// ParseTransportProfile converts a raw string to a validated TransportProfile.
// Empty string is not accepted here; use ResolveProfile for that.
// Unknown values return a typed ConfigError extractable via errors.As.
func ParseTransportProfile(raw string) (TransportProfile, error) {
	switch TransportProfile(raw) {
	case ProfileCosyAPI2, ProfileCosyAPI3, ProfileBearerOpenAI:
		return TransportProfile(raw), nil
	default:
		return "", &ConfigError{Value: raw}
	}
}

// ResolveProfile returns the effective TransportProfile.
// Empty raw maps to DefaultTransportProfile; unknown values return ConfigError.
func ResolveProfile(raw string) (TransportProfile, error) {
	if raw == "" {
		return DefaultTransportProfile, nil
	}
	return ParseTransportProfile(raw)
}

// StreamRequest is the minimal transport-neutral input for a chat request.
type StreamRequest struct {
	Body []byte // raw JSON chat body
	ID   string // request ID for tracing
}

// StreamHandle is a minimal opaque handle to an active stream.
// Callers read chunks and can cancel; concrete types are adapter-specific.
type StreamHandle interface {
	ReadChunk() ([]byte, error)
	Cancel()
}

// ChatTransport executes exactly one adapter's chat logic.
// Implementations must not retry or fall back to other adapters.
type ChatTransport interface {
	StreamChat(ctx context.Context, req StreamRequest) (StreamHandle, error)
}

// Selector resolves a TransportProfile to a ChatTransport adapter.
// The mapping is fixed at construction time; Select never modifies it.
type Selector struct {
	adapters [3]ChatTransport // [cosy-api2, cosy-api3, bearer-openai]
}

// NewSelector creates a Selector with the three required adapters in
// profile order: cosy-api2, cosy-api3, bearer-openai.
func NewSelector(cosy2, cosy3, bearer ChatTransport) (*Selector, error) {
	profiles := [3]TransportProfile{ProfileCosyAPI2, ProfileCosyAPI3, ProfileBearerOpenAI}
	adapters := [3]ChatTransport{cosy2, cosy3, bearer}
	for index, adapter := range adapters {
		if adapter == nil {
			return nil, &AdapterSetError{Profile: profiles[index]}
		}
	}
	return &Selector{adapters: adapters}, nil
}

// Select resolves raw to a TransportProfile and returns the single adapter.
// Empty raw uses DefaultTransportProfile. Unknown raw returns ConfigError.
func (s *Selector) Select(raw string) (ChatTransport, error) {
	p, err := ResolveProfile(raw)
	if err != nil {
		return nil, err
	}
	switch p {
	case ProfileCosyAPI2:
		return s.adapters[0], nil
	case ProfileCosyAPI3:
		return s.adapters[1], nil
	case ProfileBearerOpenAI:
		return s.adapters[2], nil
	default:
		// Defensive: ResolveProfile should never return an unknown profile.
		return nil, &ConfigError{Value: string(p)}
	}
}
