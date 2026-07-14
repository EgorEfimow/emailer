// Package notify provides the notification channel abstraction used by the
// emailer orchestrator to deliver digests. Implementations are registered
// via a channel registry and selected by name.
package notify

import (
	"context"
)

// ---------------------------------------------------------------------------
// SendOptions
// ---------------------------------------------------------------------------

// SendOptions carries optional parameters for a notification send operation.
type SendOptions struct {
	// Filename hint for the rendered digest (e.g. "digest-2026-07-14.md").
	// Used when sending as a document attachment.
	Filename string

	// Caption is a short summary sent alongside the document, if the channel
	// supports it. Must be 1024 characters or fewer.
	Caption string
}

// ---------------------------------------------------------------------------
// Channel
// ---------------------------------------------------------------------------

// Channel is the interface that all notification backends must implement.
//
// Implementations must:
//   - Respect the context for cancellation and timeouts.
//   - Apply retry logic with jittered exponential backoff for transient errors.
//   - Cap payload size per channel limits (e.g. 45 MB for Telegram).
//   - Be safe for concurrent use.
type Channel interface {
	// Name returns the channel name (e.g. "telegram", "slack").
	// This is used as the registry key.
	Name() string

	// Send delivers a digest payload to the notification channel.
	//
	// The payload is the rendered digest string (typically Markdown).
	// SendOptions carries optional metadata such as filename and caption.
	Send(ctx context.Context, payload string, opts SendOptions) error
}

// ---------------------------------------------------------------------------
// ChannelRegistry
// ---------------------------------------------------------------------------

// ChannelFactory creates a new Channel instance from configuration.
// Factories are registered with the ChannelRegistry.
type ChannelFactory func(ctx context.Context, config any) (Channel, error)

// ChannelRegistry maps channel names to their factories.
type ChannelRegistry struct {
	factories map[string]ChannelFactory
}

// NewChannelRegistry creates an empty channel registry.
func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		factories: make(map[string]ChannelFactory),
	}
}

// Register adds a channel factory under the given name. Panics if the name
// is already registered.
func (r *ChannelRegistry) Register(name string, factory ChannelFactory) {
	if _, ok := r.factories[name]; ok {
		panic("notify: channel " + name + " is already registered")
	}
	r.factories[name] = factory
}

// Lookup returns the factory for the given channel name. Returns nil if
// the channel is not registered.
func (r *ChannelRegistry) Lookup(name string) ChannelFactory {
	return r.factories[name]
}

// Registered returns the list of registered channel names.
func (r *ChannelRegistry) Registered() []string {
	names := make([]string, 0, len(r.factories))
	for n := range r.factories {
		names = append(names, n)
	}
	return names
}