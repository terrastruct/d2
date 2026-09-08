package limits

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"math/rand"
)

const (
	// MaxOptimizationWorkUnits bounds one hierarchy or placement optimization.
	MaxOptimizationWorkUnits uint64 = 250_000_000

	optimizationContextCheckStride uint64 = 64
)

// ErrOptimizationResourceLimit reports exhausted or overflowing optimization
// work accounting.
var ErrOptimizationResourceLimit = errors.New("TALA optimization resource limit exceeded")

// OptimizationWorkGuard accounts one complete hierarchy or placement
// optimization. The limit is an explicit ceiling; zero permits cancellation
// checks but no charged work.
type OptimizationWorkGuard struct {
	ctx      context.Context
	location string
	used     uint64
	limit    uint64
}

// NewOptimizationWorkGuard constructs a guard and observes cancellation before
// any optimization work begins.
func NewOptimizationWorkGuard(ctx context.Context, location string, limit uint64) (*OptimizationWorkGuard, error) {
	if ctx == nil {
		return nil, fmt.Errorf("TALA %s requires a context", location)
	}
	guard := &OptimizationWorkGuard{ctx: ctx, location: location, limit: limit}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return guard, nil
}

// Step charges one optimization work unit.
func (guard *OptimizationWorkGuard) Step() error {
	return guard.Add(1)
}

// Add charges amount optimization work units.
func (guard *OptimizationWorkGuard) Add(amount uint64) error {
	if amount == 0 {
		return guard.Check()
	}
	if math.MaxUint64-guard.used < amount {
		return fmt.Errorf("%w: %s work arithmetic overflow", ErrOptimizationResourceLimit, guard.location)
	}
	previous := guard.used
	next := previous + amount
	if next > guard.limit {
		return fmt.Errorf("%w: TALA %s work exceeds limit %d", ErrOptimizationResourceLimit, guard.location, guard.limit)
	}
	guard.used = next
	if previous/optimizationContextCheckStride != next/optimizationContextCheckStride {
		return guard.Check()
	}
	return nil
}

// AddProduct multiplies two work dimensions without allowing arithmetic
// overflow.
func (guard *OptimizationWorkGuard) AddProduct(a, b uint64) error {
	if a != 0 && b > math.MaxUint64/a {
		return fmt.Errorf("%w: %s work arithmetic overflow", ErrOptimizationResourceLimit, guard.location)
	}
	return guard.Add(a * b)
}

// AddSort charges the comparison bound for sorting length values.
func (guard *OptimizationWorkGuard) AddSort(length int) error {
	if length <= 1 {
		return guard.Check()
	}
	return guard.AddProduct(uint64(length), uint64(bits.Len64(uint64(length-1))))
}

// Check observes optimization cancellation immediately.
func (guard *OptimizationWorkGuard) Check() error {
	if err := guard.ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", guard.location, err)
	}
	return nil
}

// Finish observes cancellation at an optimization boundary.
func (guard *OptimizationWorkGuard) Finish() error {
	return guard.Check()
}

// Location returns the operation name used in diagnostics.
func (guard *OptimizationWorkGuard) Location() string {
	return guard.location
}

// Used returns the accepted work units.
func (guard *OptimizationWorkGuard) Used() uint64 {
	return guard.used
}

// Shuffle is math/rand.Shuffle's Fisher-Yates implementation with a guard
// check before every random draw. Its reduction algorithm and draw order are
// intentionally identical so seeded layout output remains stable.
func Shuffle[T any](values []T, random *rand.Rand, guard *OptimizationWorkGuard) error {
	if random == nil {
		return fmt.Errorf("TALA %s shuffle requires a random generator", guard.location)
	}
	i := len(values) - 1
	for ; i > 1<<31-1-1; i-- {
		if err := guard.Step(); err != nil {
			return err
		}
		j := int(random.Int63n(int64(i + 1)))
		values[i], values[j] = values[j], values[i]
	}
	for ; i > 0; i-- {
		j, err := shuffleIndex(random, int32(i+1), guard)
		if err != nil {
			return err
		}
		values[i], values[j] = values[j], values[i]
	}
	return nil
}

func shuffleIndex(random *rand.Rand, n int32, guard *OptimizationWorkGuard) (int32, error) {
	if err := guard.Step(); err != nil {
		return 0, err
	}
	v := random.Uint32()
	product := uint64(v) * uint64(n)
	low := uint32(product)
	if low < uint32(n) {
		threshold := uint32(-n) % uint32(n)
		for low < threshold {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			v = random.Uint32()
			product = uint64(v) * uint64(n)
			low = uint32(product)
		}
	}
	return int32(product >> 32), nil
}
