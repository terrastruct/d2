package layoutgraph

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/log"
)

func withTestLogger(ctx context.Context, tb testlog.TB) context.Context {
	tb.Helper()
	return log.With(ctx, testlog.New(tb))
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func requireCanceledAt(t *testing.T, err error, location string) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(%v, context.Canceled) = false", err)
	}
	if !strings.Contains(err.Error(), location) {
		t.Fatalf("cancellation error = %v, want operation %q", err, location)
	}
}

type cancelAfterErrChecks struct {
	context.Context
	remaining int
}

func (ctx *cancelAfterErrChecks) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return ctx.Context.Err()
}
