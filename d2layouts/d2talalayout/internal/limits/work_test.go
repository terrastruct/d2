package limits

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

type errOnlyCancelContext struct {
	context.Context
	canceled bool
}

func (ctx *errOnlyCancelContext) Err() error {
	if ctx.canceled {
		return context.Canceled
	}
	return nil
}

type workGuardCancelCase struct {
	name       string
	stride     int64
	newContext func() (context.Context, func())
}

func workGuardCancelCases() []workGuardCancelCase {
	return []workGuardCancelCase{
		{
			name:   "standard context",
			stride: cancellableContextCheckStride,
			newContext: func() (context.Context, func()) {
				ctx, cancel := context.WithCancel(context.Background())
				return ctx, cancel
			},
		},
		{
			name:   "nil Done context",
			stride: contextCheckStride,
			newContext: func() (context.Context, func()) {
				ctx := &errOnlyCancelContext{Context: context.Background()}
				return ctx, func() { ctx.canceled = true }
			},
		},
	}
}

func TestWorkGuardPreservesLimitAndCancellationErrors(t *testing.T) {
	guard, err := NewWorkGuard(context.Background(), "test", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); err == nil || !strings.Contains(err.Error(), "TALA test work exceeds limit 2") {
		t.Fatalf("work-limit error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewWorkGuard(canceled, "canceled test", 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "canceled test") {
		t.Fatalf("cancellation error = %v, want operation %q", err, "canceled test")
	}
}

func TestWorkGuardRejectsInvalidOrExceededZeroLimit(t *testing.T) {
	if _, err := NewWorkGuard(context.Background(), "negative test", -1); err == nil || !strings.Contains(err.Error(), "work limit must not be negative") {
		t.Fatalf("negative-limit error = %v", err)
	}
	guard, err := NewWorkGuard(context.Background(), "zero test", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); err == nil || !strings.Contains(err.Error(), "work exceeds limit 0") {
		t.Fatalf("zero-limit error = %v", err)
	}
}

func TestWorkGuardAddObservesCrossedCancellationStride(t *testing.T) {
	for _, tt := range workGuardCancelCases() {
		for _, charge := range []struct {
			name  string
			limit int64
		}{
			{name: "within limit", limit: tt.stride + 1},
			{name: "overflow", limit: tt.stride},
		} {
			t.Run(tt.name+"/"+charge.name, func(t *testing.T) {
				ctx, cancel := tt.newContext()
				guard, err := NewWorkGuard(ctx, "crossed stride", charge.limit)
				if err != nil {
					t.Fatal(err)
				}
				if err := guard.Step(); err != nil {
					t.Fatal(err)
				}
				cancel()

				err = guard.Add(tt.stride)
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Add error = %v, want context.Canceled", err)
				}
				if !strings.Contains(err.Error(), "crossed stride") {
					t.Fatalf("Add error = %v, want operation %q", err, "crossed stride")
				}
				if used := guard.Used(); used != tt.stride+1 {
					t.Fatalf("Add used = %d, want %d", used, tt.stride+1)
				}
			})
		}
	}
}

func TestWorkGuardAddLimitPrecedesCancellationWithoutAcceptedBoundary(t *testing.T) {
	for _, contextCase := range workGuardCancelCases() {
		for _, charge := range []struct {
			name             string
			initialUnits     int64
			addUnits         int64
			initialOverLimit bool
		}{
			{name: "no accepted boundary", initialUnits: 1, addUnits: contextCase.stride - 1},
			{name: "already at limit", initialUnits: contextCase.stride - 1, addUnits: 1},
			{name: "already over limit", initialUnits: contextCase.stride, initialOverLimit: true},
			{name: "huge charge", initialUnits: 1, addUnits: math.MaxInt64},
		} {
			t.Run(contextCase.name+"/"+charge.name, func(t *testing.T) {
				ctx, cancel := contextCase.newContext()
				guard, err := NewWorkGuard(ctx, "overflow ordering", contextCase.stride-1)
				if err != nil {
					t.Fatal(err)
				}
				err = guard.Add(charge.initialUnits)
				if charge.initialOverLimit && err == nil {
					t.Fatal("initial overflowing Add succeeded")
				}
				if !charge.initialOverLimit && err != nil {
					t.Fatal(err)
				}
				cancel()

				err = guard.Add(charge.addUnits)
				if err == nil || !strings.Contains(err.Error(), "TALA overflow ordering work exceeds limit") {
					t.Fatalf("Add error = %v, want work-limit error", err)
				}
				if errors.Is(err, context.Canceled) {
					t.Fatalf("Add error = %v, want work-limit precedence", err)
				}
				if used := guard.Used(); used != contextCase.stride {
					t.Fatalf("Add used = %d, want %d", used, contextCase.stride)
				}
			})
		}
	}
}

func TestWorkGuardAddZeroPreservesBoundaryPolling(t *testing.T) {
	for _, tt := range []struct {
		name         string
		steps        int
		wantCanceled bool
	}{
		{name: "at stride", steps: 0, wantCanceled: true},
		{name: "away from stride", steps: 1, wantCanceled: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			guard, err := NewWorkGuard(ctx, "zero charge", 10_000)
			if err != nil {
				t.Fatal(err)
			}
			for range tt.steps {
				if err := guard.Step(); err != nil {
					t.Fatal(err)
				}
			}
			cancel()

			err = guard.Add(0)
			if tt.wantCanceled && !errors.Is(err, context.Canceled) {
				t.Fatalf("Add(0) error = %v, want context.Canceled", err)
			}
			if !tt.wantCanceled && err != nil {
				t.Fatalf("Add(0) error = %v, want nil", err)
			}
			if used := guard.Used(); used != int64(tt.steps) {
				t.Fatalf("Add(0) used = %d, want %d", used, tt.steps)
			}
		})
	}
}

func TestCheckedUint64Arithmetic(t *testing.T) {
	if sum, ok := CheckedAddUint64(40, 2); !ok || sum != 42 {
		t.Fatalf("CheckedAddUint64(40, 2) = %d, %t", sum, ok)
	}
	if _, ok := CheckedAddUint64(math.MaxUint64, 1); ok {
		t.Fatal("overflowing addition succeeded")
	}
	if product, ok := CheckedMulUint64(21, 2); !ok || product != 42 {
		t.Fatalf("CheckedMulUint64(21, 2) = %d, %t", product, ok)
	}
	if _, ok := CheckedMulUint64(math.MaxUint64, 2); ok {
		t.Fatal("overflowing multiplication succeeded")
	}
}
