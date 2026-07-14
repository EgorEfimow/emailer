package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
		err   bool
	}{
		{input: "debug", want: slog.LevelDebug},
		{input: "DEBUG", want: slog.LevelDebug},
		{input: "Debug", want: slog.LevelDebug},
		{input: "info", want: slog.LevelInfo},
		{input: "INFO", want: slog.LevelInfo},
		{input: "warn", want: slog.LevelWarn},
		{input: "WARN", want: slog.LevelWarn},
		{input: "warning", want: slog.LevelWarn},
		{input: "WARNING", want: slog.LevelWarn},
		{input: "error", want: slog.LevelError},
		{input: "ERROR", want: slog.LevelError},
		{input: "fatal", err: true},
		{input: "", err: true},
		{input: "   ", err: true}, // TrimSpace not called; whitespace is not a valid level
		{input: "trace", err: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseLevel(tt.input)
			if tt.err {
				if err == nil {
					t.Errorf("ParseLevel(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseLevel(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	t.Run("valid level", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := NewLogger(&buf, "info")
		if err != nil {
			t.Fatalf("NewLogger unexpected error: %v", err)
		}
		if logger == nil {
			t.Fatal("NewLogger returned nil")
		}

		logger.Info("hello")

		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if entry["level"] != "INFO" {
			t.Errorf("level = %v, want INFO", entry["level"])
		}
		if entry["msg"] != "hello" {
			t.Errorf("msg = %v, want hello", entry["msg"])
		}
	})

	t.Run("invalid level", func(t *testing.T) {
		var buf bytes.Buffer
		_, err := NewLogger(&buf, "fatal")
		if err == nil {
			t.Fatal("expected error for invalid level")
		}
	})

	t.Run("level filtering", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := NewLogger(&buf, "warn")
		if err != nil {
			t.Fatalf("NewLogger unexpected error: %v", err)
		}

		logger.Debug("should be dropped")
		logger.Info("should be dropped")
		logger.Warn("should appear")
		logger.Error("should also appear")

		lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
		if len(lines) != 2 {
			t.Errorf("expected 2 log lines, got %d: %s", len(lines), buf.String())
		}
	})

	t.Run("with options", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := NewLogger(&buf, "debug", WithAddSource(true))
		if err != nil {
			t.Fatalf("NewLogger unexpected error: %v", err)
		}
		logger.Debug("source check")

		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if entry["source"] == nil {
			t.Error("expected source field when WithAddSource(true) is set")
		}
	})
}

func TestNewLoggerJSONOutput(t *testing.T) {
	// Verify that output is valid JSON and contains the expected fields.
	var buf bytes.Buffer
	logger, err := NewLogger(&buf, "info")
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	logger.Info("test message", "key", "value")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}

	for _, k := range []string{"time", "level", "msg"} {
		if _, ok := entry[k]; !ok {
			t.Errorf("entry missing required key %q", k)
		}
	}
	if entry["key"] != "value" {
		t.Errorf("extra key not propagated: key=%v", entry["key"])
	}
}