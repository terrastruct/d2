package limits

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"testing"
)

func TestOptimizationWorkGuardRejectsArithmeticOverflow(t *testing.T) {
	guard, err := NewOptimizationWorkGuard(context.Background(), "test", math.MaxUint64)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Add(math.MaxUint64); err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); !errors.Is(err, ErrOptimizationResourceLimit) {
		t.Fatalf("Step error = %v; want optimization resource limit", err)
	}

	guard, err = NewOptimizationWorkGuard(context.Background(), "test", math.MaxUint64)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.AddProduct(math.MaxUint64, 2); !errors.Is(err, ErrOptimizationResourceLimit) {
		t.Fatalf("AddProduct error = %v; want optimization resource limit", err)
	}
}

func TestOptimizationWorkGuardTreatsZeroAsZeroWork(t *testing.T) {
	guard, err := NewOptimizationWorkGuard(context.Background(), "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Step(); !errors.Is(err, ErrOptimizationResourceLimit) {
		t.Fatalf("Step error = %v; want optimization resource limit", err)
	}
}

func TestOptimizationShuffleMatchesMathRand(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3, 10, 127, 1024} {
		want := make([]int, count)
		got := make([]int, count)
		for i := range want {
			want[i] = i
			got[i] = i
		}
		wantRand := rand.New(rand.NewSource(991))
		gotRand := rand.New(rand.NewSource(991))
		wantRand.Shuffle(len(want), func(i, j int) { want[i], want[j] = want[j], want[i] })
		guard, err := NewOptimizationWorkGuard(context.Background(), "test", MaxOptimizationWorkUnits)
		if err != nil {
			t.Fatal(err)
		}
		if err := Shuffle(got, gotRand, guard); err != nil {
			t.Fatal(err)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("count %d shuffle[%d] = %d; want %d", count, i, got[i], want[i])
			}
		}
		if gotRand.Int63() != wantRand.Int63() {
			t.Fatalf("count %d random state diverged after shuffle", count)
		}
	}
}

func TestOptimizationShuffleRetriesRejectedDraw(t *testing.T) {
	n := int32(1_431_655_766)
	threshold := uint32(-n) % uint32(n)

	var seed int64
	var want int32
	var draws uint64
	for ; seed < 100; seed++ {
		random := rand.New(rand.NewSource(seed))
		draws = 0
		for {
			draws++
			product := uint64(random.Uint32()) * uint64(n)
			if uint32(product) >= threshold {
				want = int32(product >> 32)
				break
			}
		}
		if draws > 1 {
			break
		}
	}
	if draws <= 1 {
		t.Fatal("failed to find a deterministic rejected shuffle draw")
	}

	guard, err := NewOptimizationWorkGuard(context.Background(), "test", MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := shuffleIndex(rand.New(rand.NewSource(seed)), n, guard)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("shuffle index = %d; want %d", got, want)
	}
	if guard.Used() != draws {
		t.Fatalf("shuffle draws charged = %d; want %d", guard.Used(), draws)
	}
}
