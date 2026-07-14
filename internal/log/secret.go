package log

import (
	"context"
	"log/slog"
	"regexp"
)

// Sensitive wraps a string value that must never appear in log output.
// When logged through slog it renders as "[REDACTED]".
type Sensitive struct {
	value string
}

// NewSensitive creates a new Sensitive wrapper.
func NewSensitive(value string) Sensitive { return Sensitive{value: value} }

// LogValue implements slog.LogValuer so the actual value is never
// serialised.
func (s Sensitive) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

// redactionHandler wraps a slog.Handler and redacts any string matching
// the configured patterns from both the message and all attributes.
type redactionHandler struct {
	inner    slog.Handler
	patterns []*regexp.Regexp
}

func (h *redactionHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactionHandler) Handle(ctx context.Context, record slog.Record) error {
	// Redact the message text.
	for _, pat := range h.patterns {
		record.Message = pat.ReplaceAllString(record.Message, "[REDACTED]")
	}

	// Collect and redact attributes.
	var redactedAttrs []slog.Attr
	record.Attrs(func(a slog.Attr) bool {
		redactedAttrs = append(redactedAttrs, redactAttr(a, h.patterns))
		return true
	})

	// Build a new record so we control the attribute set.
	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	for _, a := range redactedAttrs {
		newRecord.AddAttrs(a)
	}

	return h.inner.Handle(ctx, newRecord)
}

func (h *redactionHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &redactionHandler{
		inner:    h.inner.WithAttrs(attrs),
		patterns: h.patterns,
	}
}

func (h *redactionHandler) WithGroup(name string) slog.Handler {
	return &redactionHandler{
		inner:    h.inner.WithGroup(name),
		patterns: h.patterns,
	}
}

// redactAttr recursively walks an attribute and redacts string values
// that match any of the given patterns.
func redactAttr(a slog.Attr, patterns []*regexp.Regexp) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		s := a.Value.String()
		for _, pat := range patterns {
			s = pat.ReplaceAllString(s, "[REDACTED]")
		}
		return slog.String(a.Key, s)

	case slog.KindGroup:
		groupAttrs := a.Value.Group()
		redacted := make([]any, len(groupAttrs))
		for i, ga := range groupAttrs {
			redacted[i] = redactAttr(ga, patterns)
		}
		return slog.Group(a.Key, redacted...)

	default:
		return a
	}
}

// WithSecretRedaction returns a logger that redacts any strings matching
// the given patterns from all log output (message and attributes).
func WithSecretRedaction(logger *slog.Logger, patterns []*regexp.Regexp) *slog.Logger {
	return slog.New(&redactionHandler{
		inner:    logger.Handler(),
		patterns: patterns,
	})
}