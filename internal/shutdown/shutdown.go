// Package shutdown provides helpers for graceful shutdown: signal-based context
// cancellation and draining in-flight work.
package shutdown

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// DefaultSignals are the signals that trigger cancellation when no custom
// signals are provided to ContextWithSignal.
var DefaultSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// ContextWithSignal returns a derived context that is cancelled when one of the
// specified signals is received. If no signals are provided, SIGINT and SIGTERM
// are used by default.
//
// The returned stop function should be called to release the signal
// registration and avoid leaking resources. It is safe to call multiple times.
func ContextWithSignal(parent context.Context, signals ...os.Signal) (context.Context, func()) {
	if len(signals) == 0 {
		signals = DefaultSignals
	}
	return signal.NotifyContext(parent, signals...)
}

// WaitForDrain waits for the provided WaitGroup to complete or the context to
// be cancelled, whichever comes first. If timeout is > 0, it sets an upper
// bound on how long to wait by deriving a context with the timeout.
//
// Returns nil if the WaitGroup completed normally.
// Returns ctx.Err() if the context was cancelled or the timeout expired.
// Returns ErrNilWaitGroup if wg is nil.
func WaitForDrain(ctx context.Context, timeout time.Duration, wg *sync.WaitGroup) error {
	if wg == nil {
		return errors.New("shutdown.WaitForDrain: nil WaitGroup")
	}

	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}