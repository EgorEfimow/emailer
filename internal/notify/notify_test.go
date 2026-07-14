package notify

import (
	"context"
	"testing"
)

func TestNewChannelRegistry(t *testing.T) {
	r := NewChannelRegistry()
	if r == nil {
		t.Fatal("NewChannelRegistry() returned nil")
	}
	if len(r.Registered()) != 0 {
		t.Errorf("new registry has %d channels, want 0", len(r.Registered()))
	}
}

func TestChannelRegistry_RegisterAndLookup(t *testing.T) {
	r := NewChannelRegistry()

	factory := func(ctx context.Context, cfg any) (Channel, error) {
		return &testChannel{name: "test"}, nil
	}

	r.Register("test", factory)

	f := r.Lookup("test")
	if f == nil {
		t.Fatal("Lookup('test') returned nil")
	}

	ch, err := f(context.Background(), nil)
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}
	if ch.Name() != "test" {
		t.Errorf("channel.Name() = %q, want %q", ch.Name(), "test")
	}
}

func TestChannelRegistry_LookupUnknown(t *testing.T) {
	r := NewChannelRegistry()
	f := r.Lookup("nonexistent")
	if f != nil {
		t.Error("Lookup('nonexistent') should return nil")
	}
}

func TestChannelRegistry_DuplicatePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register() should panic on duplicate")
		}
	}()

	r := NewChannelRegistry()
	factory := func(ctx context.Context, cfg any) (Channel, error) {
		return &testChannel{name: "dup"}, nil
	}

	r.Register("dup", factory)
	r.Register("dup", factory)
}

func TestChannelRegistry_Registered(t *testing.T) {
	r := NewChannelRegistry()

	r.Register("a", func(ctx context.Context, cfg any) (Channel, error) {
		return &testChannel{name: "a"}, nil
	})
	r.Register("b", func(ctx context.Context, cfg any) (Channel, error) {
		return &testChannel{name: "b"}, nil
	})

	names := r.Registered()
	if len(names) != 2 {
		t.Errorf("got %d registered channels, want 2", len(names))
	}

	seen := make(map[string]bool)
	for _, n := range names {
		seen[n] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("registered names = %v, want [a b]", names)
	}
}

// testChannel is a minimal Channel implementation for testing.
type testChannel struct {
	name string
}

func (c *testChannel) Name() string {
	return c.name
}

func (c *testChannel) Send(_ context.Context, _ string, _ SendOptions) error {
	return nil
}