// Package llm provides the provider abstraction, request/response types,
// prompt builder, and response parser for LLM-based email classification.
//
// The composite key (account_label, uid) is propagated through every
// request and response to ensure idempotent deduplication.
package llm

import (
	"context"
	"time"

	"github.com/egorefimow/emailer/internal/mail"
)

// ---------------------------------------------------------------------------
// InputMessage
// ---------------------------------------------------------------------------

// InputMessage represents a single email sent to the LLM for classification.
type InputMessage struct {
	Key     mail.MessageKey
	Subject string
	From    string
	Date    time.Time
	Body    string
	IsRead  bool
}

// ---------------------------------------------------------------------------
// Request
// ---------------------------------------------------------------------------

// Request is the complete classification request sent to an LLM provider.
type Request struct {
	// Model is the provider-specific model identifier (e.g. "gemini-2.0-flash").
	Model string

	// SystemPrompt is the system-level instruction for the LLM.
	SystemPrompt string

	// ClassificationPrompt is the task-specific prompt template. If empty,
	// the built-in default template is used.
	ClassificationPrompt string

	// Labels is the list of valid classification labels. If empty, defaults
	// are used: Useful, ToDelete, Ads, Unknown.
	Labels []string

	// Messages is the list of emails to classify.
	Messages []InputMessage
}

// ---------------------------------------------------------------------------
// TokenUsage
// ---------------------------------------------------------------------------

// TokenUsage tracks token consumption for a single LLM request.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ---------------------------------------------------------------------------
// Response
// ---------------------------------------------------------------------------

// Response is the result of a single LLM classification request.
type Response struct {
	// Classifications contains one entry per input message, in the same
	// order as the input. The Key field links each classification back to
	// the original message via the composite key (account_label, uid).
	Classifications []mail.Classification

	// TokenUsage reports token consumption for this request.
	TokenUsage TokenUsage

	// RawResponse is the unparsed LLM output, useful for debugging and
	// repair attempts.
	RawResponse string
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

// Provider is the interface that all LLM backends must implement.
//
// Each provider is instantiated via its constructor function and registered
// with the ProviderRegistry. The Classify method must be safe for concurrent
// use when the provider's MaxConcurrent setting permits it.
type Provider interface {
	// Name returns the provider name (e.g. "gemini", "ollama", "openrouter").
	// This is used as the registry key.
	Name() string

	// Classify sends a classification request to the LLM backend and returns
	// the parsed response together with token usage information.
	//
	// The implementation must:
	//   - Respect the context for cancellation and timeouts.
	//   - Apply retry logic with jittered exponential backoff.
	//   - Return the raw response text in Response.RawResponse for repair.
	Classify(ctx context.Context, req Request) (Response, error)
}

// ---------------------------------------------------------------------------
// ProviderFactory
// ---------------------------------------------------------------------------

// ProviderFactory creates a new Provider instance from configuration.
// Factories are registered with the ProviderRegistry and invoked when
// building the pipeline.
type ProviderFactory func(ctx context.Context, apiKey, endpoint, model string) (Provider, error)

// ---------------------------------------------------------------------------
// ProviderRegistry
// ---------------------------------------------------------------------------

// ProviderRegistry maps provider names to their factories. Providers are
// registered before use, typically in an init-like setup or explicitly in
// the wire-up code in cmd/.
type ProviderRegistry struct {
	factories map[string]ProviderFactory
}

// NewProviderRegistry creates an empty provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		factories: make(map[string]ProviderFactory),
	}
}

// Register adds a provider factory under the given name. Panics if the name
// is already registered (programming error — provider names must be unique).
func (r *ProviderRegistry) Register(name string, factory ProviderFactory) {
	if _, ok := r.factories[name]; ok {
		panic("llm: provider " + name + " is already registered")
	}
	r.factories[name] = factory
}

// Lookup returns the factory for the given provider name. Returns nil if
// the provider is not registered.
func (r *ProviderRegistry) Lookup(name string) ProviderFactory {
	return r.factories[name]
}

// Registered returns the list of registered provider names.
func (r *ProviderRegistry) Registered() []string {
	names := make([]string, 0, len(r.factories))
	for n := range r.factories {
		names = append(names, n)
	}
	return names
}