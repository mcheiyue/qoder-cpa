package qodertransport

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// fakeAdapter records calls; optionally returns an error on StreamChat.
type fakeAdapter struct {
	calls atomic.Int64
	err   error // if non-nil, StreamChat returns this error
}

func (a *fakeAdapter) StreamChat(_ context.Context, _ StreamRequest) (StreamHandle, error) {
	a.calls.Add(1)
	if a.err != nil {
		return nil, a.err
	}
	return &noopHandle{}, nil
}

func (a *fakeAdapter) callCount() int64 { return a.calls.Load() }

// noopHandle satisfies StreamHandle without real I/O.
type noopHandle struct{}

func (h *noopHandle) ReadChunk() ([]byte, error) { return nil, context.Canceled }
func (h *noopHandle) Cancel()                    {}

// cancelHandle records whether Cancel was called, proving propagation.
type cancelHandle struct{ cancelled atomic.Bool }

func (h *cancelHandle) ReadChunk() ([]byte, error) { return nil, context.Canceled }
func (h *cancelHandle) Cancel()                    { h.cancelled.Store(true) }
func (h *cancelHandle) wasCancelled() bool         { return h.cancelled.Load() }

// cancelAdapter returns a cancelHandle so we can observe Cancel.
type cancelAdapter struct {
	calls atomic.Int64
	ctx   context.Context
}

func (a *cancelAdapter) StreamChat(ctx context.Context, _ StreamRequest) (StreamHandle, error) {
	a.calls.Add(1)
	a.ctx = ctx
	return &cancelHandle{}, nil
}

// --- Selector contract tests ---

func TestExplicitProfile_Select_routes_to_unique_adapter(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantC2  int64
		wantC3  int64
		wantB   int64
	}{
		{"cosy-api2", "cosy-api2", 1, 0, 0},
		{"cosy-api3", "cosy-api3", 0, 1, 0},
		{"bearer-openai", "bearer-openai", 0, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c2, c3, b := &fakeAdapter{}, &fakeAdapter{}, &fakeAdapter{}
			sel, err := NewSelector(c2, c3, b)
			if err != nil {
				t.Fatalf("NewSelector: %v", err)
			}
			tr, err := sel.Select(tt.profile)
			if err != nil {
				t.Fatalf("Select: %v", err)
			}
			tr.StreamChat(context.Background(), StreamRequest{})
			if c2.callCount() != tt.wantC2 {
				t.Errorf("cosy-api2 calls=%d, want %d", c2.callCount(), tt.wantC2)
			}
			if c3.callCount() != tt.wantC3 {
				t.Errorf("cosy-api3 calls=%d, want %d", c3.callCount(), tt.wantC3)
			}
			if b.callCount() != tt.wantB {
				t.Errorf("bearer calls=%d, want %d", b.callCount(), tt.wantB)
			}
		})
	}
}

func TestExplicitProfile_Select_error_no_fallback(t *testing.T) {
	c2 := &fakeAdapter{err: errors.New("upstream 500")}
	c3, b := &fakeAdapter{}, &fakeAdapter{}
	sel, err := NewSelector(c2, c3, b)
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	tr, err := sel.Select("cosy-api2")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	_, err = tr.StreamChat(context.Background(), StreamRequest{})
	if err == nil {
		t.Fatal("expected error from adapter")
	}
	if c2.callCount() != 1 {
		t.Errorf("cosy-api2 calls=%d, want 1", c2.callCount())
	}
	if c3.callCount() != 0 {
		t.Errorf("cosy-api3 calls=%d, want 0 (no fallback)", c3.callCount())
	}
	if b.callCount() != 0 {
		t.Errorf("bearer calls=%d, want 0 (no fallback)", b.callCount())
	}
}

func TestExplicitProfile_Select_cancel_propagates_to_handle(t *testing.T) {
	ca := &cancelAdapter{}
	c2, c3 := &fakeAdapter{}, &fakeAdapter{}
	sel, err := NewSelector(c2, c3, ca) // bearer-openai = 3rd param
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	tr, err := sel.Select("bearer-openai")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h, err := tr.StreamChat(ctx, StreamRequest{})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	cancel()   // context cancel
	h.Cancel() // adapter-level cancel
	select {
	case <-ca.ctx.Done():
	default:
		t.Fatal("context cancellation was not propagated to selected adapter")
	}

	ch := h.(*cancelHandle)
	if !ch.wasCancelled() {
		t.Error("Cancel was not propagated to handle")
	}
	// Only selected adapter called.
	if ca.calls.Load() != 1 {
		t.Errorf("bearer calls=%d, want 1", ca.calls.Load())
	}
	if c2.callCount() != 0 {
		t.Errorf("cosy-api2 calls=%d, want 0", c2.callCount())
	}
	if c3.callCount() != 0 {
		t.Errorf("cosy-api3 calls=%d, want 0", c3.callCount())
	}
}

func TestExplicitProfile_NewSelector_rejects_nil_adapter(t *testing.T) {
	_, err := NewSelector(nil, &fakeAdapter{}, &fakeAdapter{})
	if err == nil {
		t.Fatal("expected error for nil adapter")
	}
	var setErr *AdapterSetError
	if !errors.As(err, &setErr) {
		t.Fatalf("want AdapterSetError, got %T", err)
	}
	if setErr.Profile != ProfileCosyAPI2 {
		t.Fatalf("profile: got %q, want %q", setErr.Profile, ProfileCosyAPI2)
	}
}

func TestExplicitProfile_Select_empty_resolves_to_default(t *testing.T) {
	c2, c3, b := &fakeAdapter{}, &fakeAdapter{}, &fakeAdapter{}
	sel, err := NewSelector(c2, c3, b)
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}
	tr, err := sel.Select("") // empty → DefaultTransportProfile = cosy-api2
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	tr.StreamChat(context.Background(), StreamRequest{})
	if c2.callCount() != 1 {
		t.Errorf("cosy-api2 calls=%d, want 1 (default)", c2.callCount())
	}
	if c3.callCount() != 0 {
		t.Errorf("cosy-api3 calls=%d, want 0", c3.callCount())
	}
	if b.callCount() != 0 {
		t.Errorf("bearer calls=%d, want 0", b.callCount())
	}
}

func TestExplicitProfile_Select_unknown_returns_config_error(t *testing.T) {
	sel, err := NewSelector(&fakeAdapter{}, &fakeAdapter{}, &fakeAdapter{})
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}
	_, err = sel.Select("invalid")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	var ce *ConfigError
	if !errors.As(err, &ce) {
		t.Errorf("want ConfigError, got %T: %v", err, err)
	}
}
