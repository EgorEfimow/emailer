// Package log provides structured JSON logging with level parsing,
// run-id injection, and secret redaction.
//
// It wraps slog with a functional-options constructor and helpers
// that compose into the slog middleware chain.
package log

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Option configures the logger behaviour.
type Option func(*options)

type options struct {
	level     slog.Leveler
	addSource bool
}

// WithLevel overrides the minimum logging level.
func WithLevel(level slog.Level) Option {
	return func(o *options) {
		o.level = level
	}
}

// WithAddSource adds source-file information to every log entry.
func WithAddSource(add bool) Option {
	return func(o *options) {
		o.addSource = add
	}
}

// ParseLevel converts a level string to a slog.Level.
// Valid values: debug, info, warn/warning, error.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("log: unknown level %q: valid values are debug, info, warn, error", s)
	}
}

// NewLogger creates a structured JSON logger writing to w.
// level is a string parsed by ParseLevel; returns an error when
// the level is unrecognised.
func NewLogger(w io.Writer, level string, opts ...Option) (*slog.Logger, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	var cfg options
	cfg.level = lvl
	for _, opt := range opts {
		opt(&cfg)
	}

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     cfg.level,
		AddSource: cfg.addSource,
	})

	return slog.New(handler), nil
}