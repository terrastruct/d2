// Package limits provides the engine's topology ceilings and reusable work
// accounting primitives. It deliberately has no dependency on graph, layout,
// or routing types so every layout domain can consume the same limits without
// introducing package cycles.
package limits

import (
	"context"
	"fmt"
)

const (
	// MaxEngineNodes is the largest node count the engine processes in one attempt.
	MaxEngineNodes = 10_000
	// MaxEngineEdges is the largest edge count the engine processes in one attempt.
	MaxEngineEdges = 50_000
	// MaxEngineRoutePoints bounds aggregate route storage throughout the engine.
	MaxEngineRoutePoints = 1_000_000
	// MaxEngineTreeDepth bounds recursive topology throughout the engine.
	MaxEngineTreeDepth = 256
	// MaxGraphSize is the maximum supported width or height at a pipeline stage
	// boundary.
	MaxGraphSize = 30_000

	contextCheckStride            = 64
	cancellableContextCheckStride = 1024
	workUnitsPerEntity            = 1_024

	// MaxEngineWorkUnits is the default aggregate work budget for a graph.
	MaxEngineWorkUnits = int64(MaxEngineNodes+MaxEngineEdges) * workUnitsPerEntity

	// MaxTransactionWorkUnits allows a complete layout transaction to compare
	// many nearby placements without resetting the ordinary engine budget for
	// every candidate.
	MaxTransactionWorkUnits int64 = 1_000_000_000

	// MaxTransactionOverlapReferences bounds retained existing-overlap
	// references. Exceptions are retained in both directions because hot
	// placement checks key by the moved node.
	MaxTransactionOverlapReferences int64 = 4_000_000

	// MaxBinPackWorkUnits spans the complete recursive bin-packing operation,
	// including speculative transactions and route scans.
	MaxBinPackWorkUnits int64 = 1_000_000_000

	// MaxPlaceTreesWorkUnits is calibrated for the superlinear candidate and
	// obstacle work performed by tree placement.
	MaxPlaceTreesWorkUnits int64 = 68_000_000

	// MaxLabelPlacementWorkUnits bounds the quadratic overlap and candidate
	// search performed while positioning labels and icons.
	MaxLabelPlacementWorkUnits int64 = 50_000_000
)

// WorkGuard accounts a complete operation's signed work units and polls its
// context at a fixed stride. The limit is an explicit non-negative ceiling;
// zero permits cancellation checks but no charged work.
type WorkGuard struct {
	ctx      context.Context
	done     <-chan struct{}
	location string
	used     int64
	limit    int64
}

// NewWorkGuard constructs a work guard and observes cancellation before any
// operation work begins.
func NewWorkGuard(ctx context.Context, location string, limit int64) (*WorkGuard, error) {
	if ctx == nil {
		return nil, fmt.Errorf("TALA %s requires a context", location)
	}
	if limit < 0 {
		return nil, fmt.Errorf("TALA %s work limit must not be negative", location)
	}
	guard := &WorkGuard{
		ctx:      ctx,
		done:     ctx.Done(),
		location: location,
		limit:    limit,
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return guard, nil
}

// Step charges one work unit.
func (guard *WorkGuard) Step() error {
	guard.used++
	if guard.used > guard.limit {
		return fmt.Errorf(
			"TALA %s work exceeds limit %d",
			guard.location,
			guard.limit,
		)
	}
	return guard.checkAtStride()
}

// Add records multiple work units as one accounting operation. It enforces
// Step's work limit and polls cancellation when the accepted charge reaches or
// crosses a Step polling boundary. On a positive-limit overflow, Used reports
// the first rejected unit even when a crossed boundary observes cancellation.
func (guard *WorkGuard) Add(units int64) error {
	if units < 0 {
		return fmt.Errorf("TALA %s work charge must not be negative", guard.location)
	}
	if units > guard.limit || guard.used > guard.limit-units {
		previous := guard.used
		guard.used = guard.limit + 1
		stride := guard.pollingStride()
		if previous <= guard.limit && previous/stride != guard.limit/stride {
			if err := guard.Check(); err != nil {
				return err
			}
		}
		return fmt.Errorf("TALA %s work exceeds limit %d", guard.location, guard.limit)
	}
	previous := guard.used
	guard.used += units
	return guard.checkAfterAdd(previous)
}

func (guard *WorkGuard) checkAfterAdd(previous int64) error {
	stride := guard.pollingStride()
	// Add(0) polls an exact boundary; positive charges also poll when their
	// interval crosses one.
	if guard.used%stride != 0 && previous/stride == guard.used/stride {
		return nil
	}
	return guard.Check()
}

func (guard *WorkGuard) checkAtStride() error {
	stride := guard.pollingStride()
	if guard.used%stride != 0 {
		return nil
	}
	return guard.Check()
}

func (guard *WorkGuard) pollingStride() int64 {
	if guard.done == nil {
		// Contexts without Done need the shorter stride because cancellation is
		// observable only by polling Err.
		return contextCheckStride
	}
	return cancellableContextCheckStride
}

// Check observes cancellation immediately. Step and Add use a bounded stride
// in hot loops, while callers use Check at explicit cancellation boundaries.
func (guard *WorkGuard) Check() error {
	if err := cachedContextErr(guard.ctx, guard.done); err != nil {
		return fmt.Errorf("%s: %w", guard.location, err)
	}
	return nil
}

// Finish observes cancellation regardless of the current work count.
func (guard *WorkGuard) Finish() error {
	return guard.Check()
}

// SetLimit changes the ceiling without resetting already consumed work.
func (guard *WorkGuard) SetLimit(limit int64) {
	guard.limit = limit
}

// Used returns the number of charged or first-rejected work units.
func (guard *WorkGuard) Used() int64 {
	return guard.used
}

// cachedContextErr avoids repeatedly traversing context value wrappers in hot
// guarded loops. Standard cancellable contexts close Done before Err becomes
// observable. Context implementations without a Done channel retain per-call
// Err polling, which also preserves synthetic cancellation contexts.
func cachedContextErr(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return ctx.Err()
	}
	select {
	case <-done:
		return ctx.Err()
	default:
		return nil
	}
}
