package log

import (
	"testing"
)

type tbWriter struct {
	tb testing.TB
}

func (w *tbWriter) Write(p []byte) (n int, err error) {
	w.tb.Helper()
	w.tb.Log(string(p))
	return len(p), nil
}
