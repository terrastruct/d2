package routing

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const (
	// Production routing declares at most three alternative flavors. Keeping a
	// separate budget for each flavor makes limit behavior independent of worker
	// scheduling while this cap bounds the aggregate operation to three budgets.
	maxRouteSearchFlavors = 3

	// maxRouteSearchWorkUnits is calibrated against representative routing
	// corpora. The larger measured per-flavor peak is 51,125,738 units. 120
	// million leaves over 2x headroom while the
	// independent OVG and graph limits bound topology and allocation.
	maxRouteSearchWorkUnits uint64 = 120_000_000

	routeSearchContextCheckStride uint64 = 1024
)

var errRouteSearchWorkLimit = errors.New("TALA route-search work limit exceeded")

// workBudget is implemented by both the standalone route-stage guard and the
// route-search guard. Shared geometric helpers can therefore account the work
// to the operation that invoked them without changing their routing behavior.
type workBudget interface {
	step() error
	add(uint64) error
	check() error
}

type routeSearchWorkGuard struct {
	ctx       context.Context
	done      <-chan struct{}
	flavor    RouteGenerationFlavor
	used      uint64
	limit     uint64
	metrics   *routeSearchTelemetry
	metricID  int // telemetry only; initialized to -1 before concurrent workers
	aggregate workBudget
	nextCheck uint64
	// The production stage envelope is the sum of independently enforced OVG,
	// flavor, and post-route envelopes. Charge a flavor's actual total when it
	// joins instead of contending on the shared counter for every inner-loop
	// operation. Injected limits retain exact per-operation accounting.
	deferAggregate   bool
	aggregateCharged uint64
}

func newRouteSearchWorkGuard(ctx context.Context, flavor RouteGenerationFlavor, limit uint64) (*routeSearchWorkGuard, error) {
	if ctx == nil {
		return nil, fmt.Errorf("TALA EdgeRouting requires a context")
	}
	guard := &routeSearchWorkGuard{
		ctx:       ctx,
		done:      ctx.Done(),
		flavor:    flavor,
		limit:     limit,
		metrics:   routeSearchTelemetryFromContext(ctx),
		metricID:  -1,
		aggregate: routeAggregateWorkFromContext(ctx),
	}
	if aggregate, ok := guard.aggregate.(*routeWorkGuard); ok {
		aggregate.enableParallel()
		guard.deferAggregate = aggregate.limit == maxEdgeRoutingStageWorkUnits && limit == maxRouteSearchWorkUnits
	}
	if err := guard.check(); err != nil {
		return nil, err
	}
	guard.nextCheck = routeSearchContextCheckStride
	return guard, nil
}

// bind switches from the caller context used during router construction to the
// worker context. The latter is canceled when a sibling flavor panics.
func (guard *routeSearchWorkGuard) bind(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("TALA EdgeRouting requires a context")
	}
	guard.ctx = ctx
	guard.done = ctx.Done()
	if err := guard.check(); err != nil {
		return err
	}
	guard.nextCheck = guard.used + routeSearchContextCheckStride
	return nil
}

func (guard *routeSearchWorkGuard) step() error {
	// Single-step calls sit in the innermost scans. Poll every iteration so a
	// synthetic context without a Done signal remains observable immediately.
	// Standard contexts use the same bounded polling stride as the bulk path.
	if guard.done == nil || guard.used >= guard.nextCheck {
		if err := guard.check(); err != nil {
			return err
		}
		guard.nextCheck = guard.used + routeSearchContextCheckStride
	}
	if guard.used >= guard.limit {
		if err := guard.check(); err != nil {
			return err
		}
		return fmt.Errorf(
			"%w: TALA EdgeRouting flavor %s work exceeds limit %d",
			errRouteSearchWorkLimit,
			guard.flavor,
			guard.limit,
		)
	}
	guard.used++
	if guard.deferAggregate || guard.aggregate == nil {
		return nil
	}
	return guard.aggregate.add(1)
}

func (guard *routeSearchWorkGuard) add(units uint64) error {
	if guard.used > guard.limit || units > guard.limit-guard.used {
		if err := guard.check(); err != nil {
			return err
		}
		return fmt.Errorf(
			"%w: TALA EdgeRouting flavor %s work exceeds limit %d",
			errRouteSearchWorkLimit,
			guard.flavor,
			guard.limit,
		)
	}
	guard.used += units
	if !guard.deferAggregate && guard.aggregate != nil {
		if err := guard.aggregate.add(units); err != nil {
			return err
		}
	}
	if guard.done == nil || units == 0 || guard.used >= guard.nextCheck {
		if err := guard.check(); err != nil {
			return err
		}
		guard.nextCheck = guard.used + routeSearchContextCheckStride
	}
	return nil
}

func (guard *routeSearchWorkGuard) check() error {
	if err := cachedContextErr(guard.ctx, guard.done); err != nil {
		return fmt.Errorf("EdgeRouting: %w", err)
	}
	return nil
}

func (guard *routeSearchWorkGuard) reserveProduct(a, b uint64) error {
	product, ok := limits.CheckedMulUint64(a, b)
	if !ok {
		if err := guard.check(); err != nil {
			return err
		}
		return fmt.Errorf("%w: route-search work arithmetic overflow", errRouteSearchWorkLimit)
	}
	return guard.add(product)
}

func (guard *routeSearchWorkGuard) reserveSum(a, b uint64) error {
	sum, ok := limits.CheckedAddUint64(a, b)
	if !ok {
		if err := guard.check(); err != nil {
			return err
		}
		return fmt.Errorf("%w: route-search work arithmetic overflow", errRouteSearchWorkLimit)
	}
	return guard.add(sum)
}

func (guard *routeSearchWorkGuard) reserveSort(length int) error {
	if length < 2 {
		return guard.step()
	}
	levels := uint64(0)
	for remaining := uint64(length - 1); remaining > 0; remaining >>= 1 {
		levels++
	}
	return guard.reserveProduct(uint64(length), levels)
}

func (guard *routeSearchWorkGuard) finish() error {
	if guard.metrics != nil {
		guard.metrics.record(guard, guard.used)
	}
	if guard.deferAggregate && guard.used != guard.aggregateCharged {
		pending := guard.used - guard.aggregateCharged
		if err := guard.aggregate.add(pending); err != nil {
			return err
		}
		guard.aggregateCharged = guard.used
	}
	return guard.check()
}

// routeSearchTelemetry is installed by calibration tests through context. It
// is concurrency-safe because flavor workers run in parallel.
type routeSearchTelemetry struct {
	mu      sync.Mutex
	samples []uint64
}

// SearchTelemetry collects per-flavor work samples for routing diagnostics
// and calibration. It is safe to share across concurrent route workers.
type SearchTelemetry struct {
	state routeSearchTelemetry
}

// MaxSearchWorkUnits is the calibrated per-flavor route-search budget.
const MaxSearchWorkUnits = maxRouteSearchWorkUnits

// WithSearchTelemetry attaches route-search diagnostics to a routing context.
func WithSearchTelemetry(ctx context.Context, telemetry *SearchTelemetry) context.Context {
	if telemetry == nil {
		return ctx
	}
	return withRouteSearchTelemetry(ctx, &telemetry.state)
}

// WorkSamples returns a stable copy of the recorded per-flavor work totals.
func (telemetry *SearchTelemetry) WorkSamples() []uint64 {
	if telemetry == nil {
		return nil
	}
	return telemetry.state.snapshot()
}

type routeSearchTelemetryKey struct{}

func withRouteSearchTelemetry(ctx context.Context, telemetry *routeSearchTelemetry) context.Context {
	return context.WithValue(ctx, routeSearchTelemetryKey{}, telemetry)
}

func routeSearchTelemetryFromContext(ctx context.Context) *routeSearchTelemetry {
	telemetry, _ := ctx.Value(routeSearchTelemetryKey{}).(*routeSearchTelemetry)
	return telemetry
}

func (telemetry *routeSearchTelemetry) record(guard *routeSearchWorkGuard, work uint64) {
	telemetry.mu.Lock()
	if guard.metricID < 0 {
		guard.metricID = len(telemetry.samples)
		telemetry.samples = append(telemetry.samples, work)
	} else {
		telemetry.samples[guard.metricID] = work
	}
	telemetry.mu.Unlock()
}

func (telemetry *routeSearchTelemetry) snapshot() []uint64 {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	return append([]uint64{}, telemetry.samples...)
}

func routeSearchIsDescendantOf(guard workBudget, maybeDescendant, maybeAncestor *layoutgraph.Node) (bool, error) {
	for {
		if err := guard.step(); err != nil {
			return false, err
		}
		if maybeAncestor == maybeDescendant {
			return true, nil
		}
		if maybeDescendant == nil {
			return false, nil
		}
		switch {
		case maybeDescendant.Container != nil:
			maybeDescendant = maybeDescendant.Container
		case maybeDescendant.Cluster != nil:
			maybeDescendant = maybeDescendant.Cluster.Vessel
		case maybeDescendant.Sequence != nil:
			maybeDescendant = maybeDescendant.Sequence.Vessel
		default:
			return maybeAncestor == nil, nil
		}
	}
}

func routeSearchHasFixedAncestor(guard workBudget, node *layoutgraph.Node) (bool, error) {
	for current := node; current != nil; current = current.OwningContainer() {
		if err := guard.step(); err != nil {
			return false, err
		}
		if current.FixedTopLeft != nil {
			return true, nil
		}
	}
	return false, nil
}

// fixedOverlapsForRoute mirrors fixedOverlapsFor while accounting hierarchy
// and pair scans. Each flavor derives its own immutable result so budget use is
// independent of worker scheduling and shared-cache ownership.
func fixedOverlapsForRoute(nodes layoutgraph.Nodes, guard *routeSearchWorkGuard) (map[*layoutgraph.Node]struct{}, error) {
	fixedNodes := make([]*layoutgraph.Node, 0, len(nodes)/4)
	for _, node := range nodes {
		if err := guard.step(); err != nil {
			return nil, err
		}
		fixed, err := routeSearchHasFixedAncestor(guard, node)
		if err != nil {
			return nil, err
		}
		if fixed {
			fixedNodes = append(fixedNodes, node)
		}
	}

	overlaps := make(map[*layoutgraph.Node]struct{})
	for _, node := range fixedNodes {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if _, found := overlaps[node]; found {
			continue
		}
		for _, other := range nodes {
			if err := guard.step(); err != nil {
				return nil, err
			}
			nodeBelowOther, err := routeSearchIsDescendantOf(guard, node, other)
			if err != nil {
				return nil, err
			}
			otherBelowNode, err := routeSearchIsDescendantOf(guard, other, node)
			if err != nil {
				return nil, err
			}
			if node == other || nodeBelowOther || otherBelowNode {
				continue
			}
			if node.Overlaps(other.Box) {
				overlaps[node] = struct{}{}
				fixed, err := routeSearchHasFixedAncestor(guard, other)
				if err != nil {
					return nil, err
				}
				if fixed {
					overlaps[other] = struct{}{}
				}
				break
			}
		}
	}

	return overlaps, guard.check()
}
