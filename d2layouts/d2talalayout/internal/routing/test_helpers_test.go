package routing

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
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

type cancelAfterErrChecks struct {
	context.Context
	remaining int
}

func (ctx *cancelAfterErrChecks) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return nil
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

func requireZeroRoutingCosts(t *testing.T, graph *layoutgraph.Graph) {
	t.Helper()
	costs := graph.RoutingCosts()
	if costs.Crossing != 0 || costs.Turn != 0 || costs.NonCenterPort != 0 {
		t.Fatalf("route cost caches were not restored: crossing=%v turn=%v port=%v", costs.Crossing, costs.Turn, costs.NonCenterPort)
	}
}

type exactTestSlice[T comparable] struct {
	header  []T
	backing []T
}

func captureExactTestSlice[T comparable](values []T) exactTestSlice[T] {
	return exactTestSlice[T]{header: values, backing: slices.Clone(values[:cap(values)])}
}

func (snapshot exactTestSlice[T]) assertRestored(t *testing.T, got []T, name string) {
	t.Helper()
	if len(got) != len(snapshot.header) || cap(got) != cap(snapshot.header) {
		t.Fatalf("%s header = len %d cap %d; want len %d cap %d", name, len(got), cap(got), len(snapshot.header), cap(snapshot.header))
	}
	if cap(got) > 0 && &got[:cap(got)][0] != &snapshot.header[:cap(snapshot.header)][0] {
		t.Fatalf("%s backing array identity changed", name)
	}
	if !slices.Equal(got[:cap(got)], snapshot.backing) {
		t.Fatalf("%s backing array contents changed", name)
	}
}
