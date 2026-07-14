package log

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWithRunID(t *testing.T) {
	t.Run("injects run_id into log entry", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := NewLogger(&buf, "info")
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}

		logger = WithRunID(logger, "run-abc-123")
		logger.Info("test")

		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if entry["run_id"] != "run-abc-123" {
			t.Errorf("run_id = %v, want run-abc-123", entry["run_id"])
		}
	})

	t.Run("run_id appears on all subsequent entries", func(t *testing.T) {
		var buf bytes.Buffer
		logger, err := NewLogger(&buf, "info")
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}

		logger = WithRunID(logger, "run-xyz")
		logger.Info("first")
		logger.Warn("second")

		lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		for i, line := range lines {
			var entry map[string]any
			if err := json.Unmarshal(line, &entry); err != nil {
				t.Fatalf("line %d: json.Unmarshal: %v", i, err)
			}
			if entry["run_id"] != "run-xyz" {
				t.Errorf("line %d: run_id = %v, want run-xyz", i, entry["run_id"])
			}
		}
	})

	t.Run("different run IDs produce different loggers", func(t *testing.T) {
		var buf1, buf2 bytes.Buffer
		logger1, _ := NewLogger(&buf1, "info")
		logger2, _ := NewLogger(&buf2, "info")

		logger1 = WithRunID(logger1, "run-a")
		logger2 = WithRunID(logger2, "run-b")

		logger1.Info("msg")
		logger2.Info("msg")

		var e1, e2 map[string]any
		json.Unmarshal(buf1.Bytes(), &e1)
		json.Unmarshal(buf2.Bytes(), &e2)

		if e1["run_id"] != "run-a" {
			t.Errorf("logger1 run_id = %v, want run-a", e1["run_id"])
		}
		if e2["run_id"] != "run-b" {
			t.Errorf("logger2 run_id = %v, want run-b", e2["run_id"])
		}
	})
}