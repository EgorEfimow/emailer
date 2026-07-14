package log

import "log/slog"

// WithRunID returns a new logger that attaches the given runID to every
// log entry under the key "run_id".
func WithRunID(logger *slog.Logger, runID string) *slog.Logger {
	return logger.With(slog.String("run_id", runID))
}
