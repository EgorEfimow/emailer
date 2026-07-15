//nolint:errcheck
package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"regexp"
	"testing"
)

func TestSensitive(t *testing.T) {
	t.Run("Value returns the underlying string", func(t *testing.T) {
		s := NewSensitive("super-secret-123")
		if got := s.Value(); got != "super-secret-123" {
			t.Errorf("Value() = %q, want %q", got, "super-secret-123")
		}
	})

	t.Run("LogValue returns [REDACTED]", func(t *testing.T) {
		s := NewSensitive("super-secret-123")
		val := s.LogValue()
		if val.String() != "[REDACTED]" {
			t.Errorf("LogValue = %q, want [REDACTED]", val.String())
		}
	})

	t.Run("logged via slog is redacted", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := NewLogger(&buf, "info")
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}

		logger.Info("test", "password", NewSensitive("my-password"))

		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if entry["password"] != "[REDACTED]" {
			t.Errorf("password = %v, want [REDACTED]", entry["password"])
		}
	})

	t.Run("non-sensitive attributes pass through", func(t *testing.T) {
		var buf bytes.Buffer
		logger, _ := NewLogger(&buf, "info")
		logger.Info("test", "username", "alice", "password", NewSensitive("s3cret"))

		var entry map[string]any
		json.Unmarshal(buf.Bytes(), &entry)
		if entry["username"] != "alice" {
			t.Errorf("username = %v, want alice", entry["username"])
		}
		if entry["password"] != "[REDACTED]" {
			t.Errorf("password = %v, want [REDACTED]", entry["password"])
		}
	})
}

func TestWithSecretRedaction(t *testing.T) {
	patterns := []*regexp.Regexp{regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`)}

	t.Run("redacts matching pattern in message", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := NewLogger(&buf, "info")
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}
		logger = WithSecretRedaction(logger, patterns)
		logger.Info("key is sk-abc123def456ghi789jkl")

		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if entry["msg"] != "key is [REDACTED]" {
			t.Errorf("msg = %v, want %v", entry["msg"], "key is [REDACTED]")
		}
	})

	t.Run("redacts matching pattern in string attribute", func(t *testing.T) {
		var buf bytes.Buffer
		logger, _ := NewLogger(&buf, "info")
		logger = WithSecretRedaction(logger, patterns)
		logger.Info("login", "api_key", "sk-abc123def456ghi789jkl")

		var entry map[string]any
		json.Unmarshal(buf.Bytes(), &entry)
		if entry["api_key"] != "[REDACTED]" {
			t.Errorf("api_key = %v, want [REDACTED]", entry["api_key"])
		}
	})

	t.Run("non-matching attributes pass through", func(t *testing.T) {
		var buf bytes.Buffer
		logger, _ := NewLogger(&buf, "info")
		logger = WithSecretRedaction(logger, patterns)
		logger.Info("test", "username", "bob", "count", 42)

		var entry map[string]any
		json.Unmarshal(buf.Bytes(), &entry)
		if entry["username"] != "bob" {
			t.Errorf("username = %v, want bob", entry["username"])
		}
		if entry["count"] != float64(42) {
			t.Errorf("count = %v, want 42", entry["count"])
		}
	})

	t.Run("no patterns logs normally", func(t *testing.T) {
		var buf bytes.Buffer
		logger, _ := NewLogger(&buf, "info")
		logger = WithSecretRedaction(logger, nil)
		logger.Info("hello", "key", "value")

		var entry map[string]any
		json.Unmarshal(buf.Bytes(), &entry)
		if entry["msg"] != "hello" || entry["key"] != "value" {
			t.Errorf("unexpected output: %v", entry)
		}
	})

	t.Run("multiple patterns", func(t *testing.T) {
		multi := []*regexp.Regexp{
			regexp.MustCompile(`Bearer\s+[a-zA-Z0-9]+`),
			regexp.MustCompile(`token-\d+`),
		}
		var buf bytes.Buffer
		logger, _ := NewLogger(&buf, "info")
		logger = WithSecretRedaction(logger, multi)
		logger.Info("Bearer abc123 and token-456 present")

		var entry map[string]any
		json.Unmarshal(buf.Bytes(), &entry)
		if entry["msg"] != "[REDACTED] and [REDACTED] present" {
			t.Errorf("msg = %v, want %v", entry["msg"], "[REDACTED] and [REDACTED] present")
		}
	})

	t.Run("redacts in nested groups", func(t *testing.T) {
		var buf bytes.Buffer
		logger, _ := NewLogger(&buf, "info")
		logger = WithSecretRedaction(logger, patterns)
		logger.Info("test",
			slog.Group("creds",
				slog.String("api_key", "sk-abc123def456ghi789jkl"),
				slog.String("username", "alice"),
			),
		)

		var entry map[string]any
		json.Unmarshal(buf.Bytes(), &entry)
		creds, ok := entry["creds"].(map[string]any)
		if !ok {
			t.Fatalf("expected creds to be a map, got %T (raw: %s)", entry["creds"], buf.String())
		}
		if creds["api_key"] != "[REDACTED]" {
			t.Errorf("creds.api_key = %v, want [REDACTED]", creds["api_key"])
		}
		if creds["username"] != "alice" {
			t.Errorf("creds.username = %v, want alice", creds["username"])
		}
	})
}

func TestSensitiveWithRedaction(t *testing.T) {
	// Verify that Sensitive wrapping takes precedence over pattern
	// redaction (i.e. Sensitive never reveals the value, even if the
	// pattern is loose).
	patterns := []*regexp.Regexp{regexp.MustCompile(`.*`)}
	var buf bytes.Buffer
	logger, _ := NewLogger(&buf, "info")
	logger = WithSecretRedaction(logger, patterns)
	logger.Info("test", "secret", NewSensitive("dont-reveal-me"))

	var entry map[string]any
	json.Unmarshal(buf.Bytes(), &entry)
	if entry["msg"] != "[REDACTED]" {
		t.Errorf("msg = %v, want '[REDACTED]'", entry["msg"])
	}
	if entry["secret"] != "[REDACTED]" {
		t.Errorf("secret = %v, want [REDACTED]", entry["secret"])
	}
}