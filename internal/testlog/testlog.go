// Package testlog adapts testing-style loggers to slog.
package testlog

import (
	"log/slog"
)

// TB is the subset of testing.TB needed to write logs to a test.
type TB interface {
	Helper()
	Log(args ...any)
}

type tbWriter struct {
	tb TB
}

func (w *tbWriter) Write(p []byte) (n int, err error) {
	w.tb.Helper()
	w.tb.Log(string(p))
	return len(p), nil
}

// New returns a logger that writes each record to tb.
func New(tb TB) *slog.Logger {
	return slog.New(slog.NewTextHandler(&tbWriter{tb: tb}, nil))
}
