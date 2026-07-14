package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// compileCheck ensures our fakes satisfy the interface.
var _ Provider = (*fakeProvider)(nil)

// fakeProvider is a minimal in-memory Provider for tests.
type fakeProvider struct {
	mu         sync.Mutex
	name       string
	responses  []Response
	callCount  int
	classifyErr error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Classify(_ context.Context, _ Request) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.callCount++
	if f.classifyErr != nil {
		return Response{}, f.classifyErr
	}
	if f.callCount <= len(f.responses) {
		return f.responses[f.callCount-1], nil
	}
	return Response{}, nil
}

// ---------------------------------------------------------------------------
// ProviderRegistry tests
// ---------------------------------------------------------------------------

func TestProviderRegistry_RegisterAndLookup(t *testing.T) {
	reg := NewProviderRegistry()

	factory := func(_ context.Context, _, _, _ string) (Provider, error) {
		return &fakeProvider{name: "test"}, nil
	}

	reg.Register("test-provider", factory)

	f := reg.Lookup("test-provider")
	if f == nil {
		t.Fatal("Lookup returned nil for registered provider")
	}

	p, err := f(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}
	if p.Name() != "test" {
		t.Errorf("provider name = %q, want %q", p.Name(), "test")
	}
}

func TestProviderRegistry_LookupUnknown(t *testing.T) {
	reg := NewProviderRegistry()

	f := reg.Lookup("nonexistent")
	if f != nil {
		t.Error("Lookup for unknown provider should return nil")
	}
}

func TestProviderRegistry_RegisterDuplicatePanics(t *testing.T) {
	reg := NewProviderRegistry()

	factory := func(_ context.Context, _, _, _ string) (Provider, error) {
		return &fakeProvider{}, nil
	}

	reg.Register("dup", factory)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()

	reg.Register("dup", factory)
}

func TestProviderRegistry_Registered(t *testing.T) {
	reg := NewProviderRegistry()

	factory := func(_ context.Context, _, _, _ string) (Provider, error) {
		return &fakeProvider{}, nil
	}

	reg.Register("a", factory)
	reg.Register("b", factory)

	names := reg.Registered()
	if len(names) != 2 {
		t.Fatalf("got %d registered names, want 2", len(names))
	}

	// Check both names are present.
	seen := make(map[string]bool)
	for _, n := range names {
		seen[n] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("registered names = %v, want both 'a' and 'b'", names)
	}
}

func TestProviderRegistry_Empty(t *testing.T) {
	reg := NewProviderRegistry()

	names := reg.Registered()
	if names == nil {
		t.Error("Registered() should return empty slice, not nil")
	}
	if len(names) != 0 {
		t.Errorf("expected empty registry, got %d names", len(names))
	}
}

func TestProviderFactory_Error(t *testing.T) {
	reg := NewProviderRegistry()

	expectedErr := errors.New("factory error")
	factory := func(_ context.Context, _, _, _ string) (Provider, error) {
		return nil, expectedErr
	}

	reg.Register("failing", factory)

	f := reg.Lookup("failing")
	if f == nil {
		t.Fatal("Lookup returned nil for registered provider")
	}

	_, err := f(context.Background(), "", "", "")
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

// ---------------------------------------------------------------------------
// Provider interface tests
// ---------------------------------------------------------------------------

func TestProvider_Classify_HappyPath(t *testing.T) {
	p := &fakeProvider{
		name: "test",
		responses: []Response{
			{
				RawResponse: `{"classifications": []}`,
			},
		},
	}

	resp, err := p.Classify(context.Background(), Request{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RawResponse != `{"classifications": []}` {
		t.Errorf("raw response = %q, want %q", resp.RawResponse, `{"classifications": []}`)
	}
}

func TestProvider_Classify_Error(t *testing.T) {
	expectedErr := errors.New("provider error")
	p := &fakeProvider{
		name:        "test",
		classifyErr: expectedErr,
	}

	_, err := p.Classify(context.Background(), Request{})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestProvider_Classify_ContextCancelled(t *testing.T) {
	p := &fakeProvider{
		name: "test",
		responses: []Response{
			{RawResponse: "ok"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Classify(ctx, Request{})
	// The fake doesn't check context, so it should succeed.
	// Real providers should check context. This is a contract test.
	if err != nil {
		t.Fatalf("fake provider should not fail on cancelled context: %v", err)
	}
}

func TestProviderName(t *testing.T) {
	p := &fakeProvider{name: "gemini"}
	if p.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gemini")
	}
}

// ---------------------------------------------------------------------------
// Contract tests for the Provider interface
// ---------------------------------------------------------------------------

// TestProviderConcurrencySafety verifies that providers can be called
// concurrently. This is a contract test that all provider implementations
// must pass.
func TestProviderConcurrencySafety(t *testing.T) {
	p := &fakeProvider{
		name: "concurrent",
		responses: []Response{
			{RawResponse: "ok1"},
			{RawResponse: "ok2"},
			{RawResponse: "ok3"},
			{RawResponse: "ok4"},
		},
	}

	// Call the provider from multiple goroutines concurrently.
	// This should not race or panic.
	const n = 4
	done := make(chan struct{}, n)

	for i := 0; i < n; i++ {
		go func() {
			_, err := p.Classify(context.Background(), Request{})
			if err != nil {
				t.Errorf("concurrent call error: %v", err)
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < n; i++ {
		<-done
	}

	if p.callCount != n {
		t.Errorf("expected %d calls, got %d", n, p.callCount)
	}
}