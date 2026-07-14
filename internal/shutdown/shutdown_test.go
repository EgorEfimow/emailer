package shutdown

import (
	"context"
	"errors"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestContextWithSignal_cancelsOnCustomSignal(t *testing.T) {
	ctx, stop := ContextWithSignal(context.Background(), syscall.SIGUSR1)
	defer stop()

	// Send the signal to the current process.
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatalf("failed to send SIGUSR1: %v", err)
	}

	select {
	case <-ctx.Done():
		// signal.NotifyContext sets the cause to the signal itself.
	case <-time.After(5 * time.Second):
		t.Fatal("context was not cancelled within 5 seconds after signal")
	}
}

func TestContextWithSignal_stopCancelsContext(t *testing.T) {
	ctx, stop := ContextWithSignal(context.Background(), syscall.SIGUSR1)

	// The stop function from signal.NotifyContext both un-registers the signal
	// handler and cancels the context.
	stop()

	select {
	case <-ctx.Done():
		// ok — stop cancels the context
	case <-time.After(5 * time.Second):
		t.Fatal("context was not cancelled after stop()")
	}
}

func TestContextWithSignal_stopIsIdempotent(t *testing.T) {
	_, stop := ContextWithSignal(context.Background(), syscall.SIGUSR1)

	// Calling stop multiple times must not panic.
	stop()
	stop()
	stop()
}

func TestContextWithSignal_defaultSignals(t *testing.T) {
	// Verify that DefaultSignals does not contain uncatchable signals.
	for _, s := range DefaultSignals {
		if s == syscall.SIGKILL || s == syscall.SIGSTOP {
			t.Errorf("DefaultSignals contains uncatchable signal %v", s)
		}
	}
}

func TestContextWithSignal_noSignalsProvided(t *testing.T) {
	// When no custom signals are provided, DefaultSignals should be used.
	// We verify this by ensuring the function does not panic and returns
	// a non-nil context and stop function.
	ctx, stop := ContextWithSignal(context.Background())
	defer stop()

	if ctx == nil {
		t.Fatal("ContextWithSignal returned nil context")
	}
}

func TestContextWithSignal_parentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx, stop := ContextWithSignal(parent, syscall.SIGUSR1)
	defer stop()

	cancel()

	select {
	case <-ctx.Done():
		if cause := context.Cause(ctx); cause != context.Canceled {
			t.Errorf("expected Canceled cause from parent, got %v", cause)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("context was not cancelled after parent cancellation")
	}
}

func TestWaitForDrain_returnsNilWhenWorkCompletes(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Simulate short work.
	}()

	ctx := context.Background()
	err := WaitForDrain(ctx, 5*time.Second, &wg)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestWaitForDrain_returnsErrorOnCancel(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1) // Never finished.

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancel.

	err := WaitForDrain(ctx, time.Hour, &wg)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestWaitForDrain_returnsErrorOnTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1) // Never finished.

	err := WaitForDrain(context.Background(), time.Millisecond, &wg)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestWaitForDrain_nilWaitGroup(t *testing.T) {
	err := WaitForDrain(context.Background(), time.Second, nil)
	if err == nil {
		t.Fatal("expected error for nil WaitGroup, got nil")
	}
}

func TestWaitForDrain_zeroTimeoutWaitsIndefinitely(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		done <- WaitForDrain(ctx, 0, &wg)
	}()

	// Give a moment to ensure the goroutine blocks on the WaitGroup.
	time.Sleep(50 * time.Millisecond)

	// Complete the work.
	wg.Done()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForDrain did not return after wg.Done()")
	}

	cancel()
}