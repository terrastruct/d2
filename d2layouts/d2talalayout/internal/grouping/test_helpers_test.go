package grouping

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type cancelWhenStackContains struct {
	context.Context
	function string
}

func (ctx *cancelWhenStackContains) Err() error {
	var callers [32]uintptr
	count := runtime.Callers(2, callers[:])
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ctx.function) {
			return context.Canceled
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
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

func requireCanceledAt(t *testing.T, err error, location string) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(%v, context.Canceled) = false", err)
	}
	if !strings.Contains(err.Error(), location) {
		t.Fatalf("cancellation error = %v, want operation %q", err, location)
	}
}
