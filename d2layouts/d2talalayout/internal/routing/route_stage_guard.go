package routing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphbounds"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

const (
	// Route post-processing contains several intentionally quadratic searches.
	// Bound the aggregate work independently of adapter validation because the
	// engine also has direct callers.
	maxRouteStageWorkUnits       uint64 = 50_000_000
	routeStageContextCheckStride uint64 = 1024
	// EdgeRoutingStage owns OVG construction, up to three concurrent route
	// flavors, and its stage-level discovery/rollback work. This aggregate cap
	// is the sum of those independently calibrated envelopes; unlike the nested
	// caps, it is shared across every disconnected subgraph.
	maxEdgeRoutingStageWorkUnits uint64 = maxOVGWorkUnits +
		maxRouteSearchFlavors*maxRouteSearchWorkUnits + maxRouteStageWorkUnits
)

var errRouteStageWorkLimit = errors.New("TALA route-stage work limit exceeded")

// routeWorkGuard is shared by every helper in a route post-processing stage.
// Work is unsigned and checked before addition, so even injected limits and
// hostile inputs cannot wrap the counter.
type routeWorkGuard struct {
	parallel atomic.Bool
	ctx      context.Context
	done     <-chan struct{}
	location string
	used     uint64
	limit    uint64
}

func newRouteWorkGuard(ctx context.Context, location string, limit uint64) (*routeWorkGuard, error) {
	if ctx == nil {
		return nil, fmt.Errorf("TALA %s requires a context", location)
	}
	guard := &routeWorkGuard{ctx: ctx, done: ctx.Done(), location: location, limit: limit}
	if err := guard.check(); err != nil {
		return nil, err
	}
	return guard, nil
}

func (guard *routeWorkGuard) step() error {
	return guard.add(1)
}

func (guard *routeWorkGuard) add(units uint64) error {
	// OVG construction owns this guard from one goroutine. Avoid a mutex on
	// every one of its millions of accounted operations until route-flavor
	// workers explicitly switch the aggregate to its parallel path.
	if !guard.parallel.Load() {
		if guard.used > guard.limit || units > guard.limit-guard.used {
			if err := guard.check(); err != nil {
				return err
			}
			return fmt.Errorf("%w: TALA %s work exceeds limit %d", errRouteStageWorkLimit, guard.location, guard.limit)
		}
		previous := guard.used
		guard.used += units
		if units == 0 || previous/routeStageContextCheckStride != guard.used/routeStageContextCheckStride {
			return guard.check()
		}
		return nil
	}
	for {
		previous := atomic.LoadUint64(&guard.used)
		if previous > guard.limit || units > guard.limit-previous {
			if err := guard.check(); err != nil {
				return err
			}
			return fmt.Errorf("%w: TALA %s work exceeds limit %d", errRouteStageWorkLimit, guard.location, guard.limit)
		}
		current := previous + units
		if !atomic.CompareAndSwapUint64(&guard.used, previous, current) {
			continue
		}
		if units == 0 || previous/routeStageContextCheckStride != current/routeStageContextCheckStride {
			return guard.check()
		}
		return nil
	}
}

func (guard *routeWorkGuard) enableParallel() {
	guard.parallel.Store(true)
}

type routeAggregateWorkKey struct{}

func contextWithRouteAggregateWork(ctx context.Context, guard workBudget) context.Context {
	if ctx == nil || guard == nil {
		return ctx
	}
	return context.WithValue(ctx, routeAggregateWorkKey{}, guard)
}

func routeAggregateWorkFromContext(ctx context.Context) workBudget {
	if ctx == nil {
		return nil
	}
	guard, _ := ctx.Value(routeAggregateWorkKey{}).(workBudget)
	return guard
}

func (guard *routeWorkGuard) check() error {
	if err := cachedContextErr(guard.ctx, guard.done); err != nil {
		return fmt.Errorf("%s: %w", guard.location, err)
	}
	return nil
}

func (guard *routeWorkGuard) finish() error {
	return guard.check()
}

// Step and Finish let routing share its exact aggregate budget with lower
// layout-domain kernels without exposing the guard's counters or limits.
func (guard *routeWorkGuard) Step() error   { return guard.step() }
func (guard *routeWorkGuard) Finish() error { return guard.finish() }

type routeMutationSnapshot struct {
	graph      *layoutgraph.Graph
	cellSize   float64
	costs      layoutgraph.RoutingCostState
	nodes      map[*layoutgraph.Node]pointerSnapshot[geo.Point]
	nodeGraphs map[*layoutgraph.Node]*layoutgraph.Graph
	edges      map[*layoutgraph.Edge]edgeSnapshot
}

// captureRouteMutations records every route reachable through the collections
// used by post-processing. captureEdge preserves the exact slice header,
// backing array, point identities, and point values.
func captureRouteMutations(g *layoutgraph.Graph, extraEdges []*layoutgraph.Edge, guard *routeWorkGuard) (routeMutationSnapshot, error) {
	snapshot := routeMutationSnapshot{
		graph:      g,
		nodes:      make(map[*layoutgraph.Node]pointerSnapshot[geo.Point]),
		nodeGraphs: make(map[*layoutgraph.Node]*layoutgraph.Graph),
		edges:      make(map[*layoutgraph.Edge]edgeSnapshot),
	}
	if g != nil {
		snapshot.cellSize = g.CellSize
		snapshot.costs = g.RoutingCosts()
	}
	nodeQueue := make([]*layoutgraph.Node, 0)
	captureNode := func(node *layoutgraph.Node) error {
		if err := guard.step(); err != nil {
			return err
		}
		if node == nil {
			return nil
		}
		if _, seen := snapshot.nodes[node]; seen {
			return nil
		}
		snapshot.nodes[node] = snapshotPointer(node.TopLeft)
		snapshot.nodeGraphs[node] = node.Graph
		nodeQueue = append(nodeQueue, node)
		return nil
	}
	capture := func(edge *layoutgraph.Edge) error {
		// Charge every reference before de-duplication. A hostile repeated-edge
		// slice must not turn snapshot discovery into unaccounted linear work.
		if err := guard.step(); err != nil {
			return err
		}
		if edge == nil {
			return nil
		}
		if _, seen := snapshot.edges[edge]; seen {
			return nil
		}

		fullPoints := edge.Points[:cap(edge.Points)]
		pointValues := make([]pointerSnapshot[geo.Point], len(fullPoints))
		backing := make([]*geo.Point, len(fullPoints))
		for index, point := range fullPoints {
			if err := guard.step(); err != nil {
				return err
			}
			backing[index] = point
			pointValues[index] = snapshotPointer(point)
		}
		snapshot.edges[edge] = edgeSnapshot{
			value:                *edge,
			d2ID:                 snapshotPointer(edge.D2ID),
			points:               exactSliceSnapshot[[]*geo.Point, *geo.Point]{original: edge.Points, backing: backing},
			pointValues:          pointValues,
			label:                snapshotPointer(edge.Label),
			sourceArrowheadLabel: snapshotPointer(edge.SourceArrowheadLabel),
			targetArrowheadLabel: snapshotPointer(edge.TargetArrowheadLabel),
			fromTableColumnIndex: snapshotPointer(edge.FromTableColumnIndex),
			toTableColumnIndex:   snapshotPointer(edge.ToTableColumnIndex),
		}
		if err := captureNode(edge.From); err != nil {
			return err
		}
		return captureNode(edge.To)
	}
	if g != nil {
		for _, edge := range g.Edges {
			if err := capture(edge); err != nil {
				return routeMutationSnapshot{}, err
			}
		}
		for _, node := range g.Nodes {
			if err := captureNode(node); err != nil {
				return routeMutationSnapshot{}, err
			}
			if node == nil {
				continue
			}
			for _, edge := range node.Edges {
				if err := capture(edge); err != nil {
					return routeMutationSnapshot{}, err
				}
			}
		}
	}
	for _, edge := range extraEdges {
		if err := capture(edge); err != nil {
			return routeMutationSnapshot{}, err
		}
	}
	// A route stage temporarily rewrites graph references not just for graph
	// members but also for container, cluster, and sequence descendants. Walk
	// each unique descendant once so rollback covers those references and any
	// positions a route helper may move.
	for index := 0; index < len(nodeQueue); index++ {
		node := nodeQueue[index]
		nodeGraph := node.Graph
		if nodeGraph == nil {
			nodeGraph = g
		}
		if nodeGraph == nil {
			continue
		}
		if node.IsContainer() {
			for _, child := range nodeGraph.Containers[node] {
				if err := captureNode(child); err != nil {
					return routeMutationSnapshot{}, err
				}
			}
		}
		if node.IsClusterVessel() {
			if cluster := nodeGraph.Clusters[node]; cluster != nil {
				for _, child := range cluster.Nodes {
					if err := captureNode(child); err != nil {
						return routeMutationSnapshot{}, err
					}
				}
			}
		}
		if sequence := nodeGraph.Sequences[node]; sequence != nil {
			for _, child := range sequence.Nodes {
				if err := captureNode(child); err != nil {
					return routeMutationSnapshot{}, err
				}
			}
		}
	}
	return snapshot, guard.finish()
}

func validateRouteStageGeometry(g *layoutgraph.Graph, extraEdges []*layoutgraph.Edge, guard *routeWorkGuard) error {
	if len(extraEdges) > int(layoutgraph.MaxTopologyReferences) {
		return fmt.Errorf("TALA %s extra edge references exceed limit %d", guard.location, layoutgraph.MaxTopologyReferences)
	}
	finite := func(value float64) bool {
		return !math.IsNaN(value) && !math.IsInf(value, 0)
	}
	seenNodes := make(map[*layoutgraph.Node]struct{})
	graphNodes := make(map[*layoutgraph.Node]struct{}, len(g.Nodes))
	validateNode := func(node *layoutgraph.Node, description string) error {
		if err := guard.step(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("TALA %s contains a nil %s", guard.location, description)
		}
		if _, ok := seenNodes[node]; ok {
			return nil
		}
		seenNodes[node] = struct{}{}
		if node.TopLeft == nil {
			return fmt.Errorf("TALA %s %s has no position", guard.location, description)
		}
		return nil
	}
	for index, node := range g.Nodes {
		if err := validateNode(node, fmt.Sprintf("graph node at index %d", index)); err != nil {
			return err
		}
		graphNodes[node] = struct{}{}
	}

	seen := make(map[*layoutgraph.Edge]struct{})
	var routePointCapacity uint64
	validateEdge := func(edge *layoutgraph.Edge, requireGraphEndpoints bool) error {
		if err := guard.step(); err != nil {
			return err
		}
		if edge == nil {
			return fmt.Errorf("TALA %s contains a nil edge", guard.location)
		}
		if _, ok := seen[edge]; ok {
			return nil
		}
		seen[edge] = struct{}{}
		if requireGraphEndpoints {
			if _, ok := graphNodes[edge.From]; !ok {
				return fmt.Errorf("graph edge %d source node does not belong to the graph", edge.IDValue())
			}
			if _, ok := graphNodes[edge.To]; !ok {
				return fmt.Errorf("graph edge %d target node does not belong to the graph", edge.IDValue())
			}
		}
		capacity := uint64(cap(edge.Points))
		if routePointCapacity > uint64(layoutgraph.MaxRoutePoints) || capacity > uint64(layoutgraph.MaxRoutePoints)-routePointCapacity {
			return fmt.Errorf("TALA %s route point capacity exceeds limit %d", guard.location, layoutgraph.MaxRoutePoints)
		}
		routePointCapacity += capacity
		if len(edge.Points) == 1 {
			return invariant.Errorf(
				"edge %d has an incomplete route: expected at least two points, got 1",
				edge.IDValue(),
			)
		}
		for _, item := range []struct {
			name  string
			label *layoutgraph.Label
		}{
			{name: "label", label: edge.Label},
			{name: "source arrowhead label", label: edge.SourceArrowheadLabel},
			{name: "target arrowhead label", label: edge.TargetArrowheadLabel},
		} {
			if item.label != nil && item.label.Position != label.Unset && !item.label.Position.IsEdgePosition() {
				return invariant.Errorf(
					"edge %d %s has an invalid edge position",
					edge.IDValue(), item.name,
				)
			}
		}
		if err := validateNode(edge.From, fmt.Sprintf("edge %d source", edge.IDValue())); err != nil {
			return err
		}
		if err := validateNode(edge.To, fmt.Sprintf("edge %d target", edge.IDValue())); err != nil {
			return err
		}
		for _, point := range edge.Points {
			if err := guard.step(); err != nil {
				return err
			}
			if point == nil {
				return fmt.Errorf("TALA %s edge %d contains a nil route point", guard.location, edge.IDValue())
			}
			if !finite(point.X) || !finite(point.Y) {
				return fmt.Errorf("TALA %s edge %d has a non-finite route point", guard.location, edge.IDValue())
			}
		}
		return nil
	}
	for _, edge := range g.Edges {
		if err := validateEdge(edge, true); err != nil {
			return err
		}
	}
	for _, node := range g.Nodes {
		for _, edge := range node.Edges {
			if err := validateEdge(edge, false); err != nil {
				return err
			}
		}
	}
	for _, edge := range extraEdges {
		if err := validateEdge(edge, false); err != nil {
			return err
		}
	}
	return guard.finish()
}

// routeStageGraphBoundingBox preserves Graph.BoundingBox geometry while
// charging the stage aggregate for the existing outside-label extremity scans
// and every edge-route scan. The node helper is shared with BinPack and has
// randomized parity coverage against the legacy bounding-box implementation.
func routeStageGraphBoundingBox(g *layoutgraph.Graph, guard *routeWorkGuard) (*geo.Point, *geo.Point, error) {
	if g == nil || guard == nil {
		return nil, nil, fmt.Errorf("TALA EdgeRouting bounding box requires a graph and work guard")
	}
	topLeft, bottomRight, err := graphbounds.FixedBoundingBox(layoutgraph.Nodes(g.Nodes), guard)
	if err != nil {
		return nil, nil, err
	}
	if topLeft == nil || bottomRight == nil {
		return nil, nil, invariant.New("EdgeRouting bounding box contains an unplaced node")
	}
	minX, minY := topLeft.X, topLeft.Y
	maxX, maxY := bottomRight.X, bottomRight.Y
	for _, edge := range g.Edges {
		if err := guard.step(); err != nil {
			return nil, nil, err
		}
		// Edge bounding-box calculation scans the route once for geometry. D2's positioned
		// edge-label helper scans it twice, and each arrowhead-label helper scans
		// it three times. Charge those exact/conservative kernels before calling
		// the legacy geometry implementation.
		if err := guard.add(uint64(len(edge.Points))); err != nil {
			return nil, nil, err
		}
		chargeRouteScans := func(scans int) error {
			for range scans {
				if err := guard.add(uint64(len(edge.Points))); err != nil {
					return err
				}
			}
			return nil
		}
		if len(edge.Points) != 0 && edge.Label != nil && edge.Label.Position != label.Unset {
			if err := chargeRouteScans(2); err != nil {
				return nil, nil, err
			}
		}
		if len(edge.Points) != 0 && edge.SourceArrowheadLabel != nil {
			if err := chargeRouteScans(3); err != nil {
				return nil, nil, err
			}
		}
		if len(edge.Points) != 0 && edge.TargetArrowheadLabel != nil {
			if err := chargeRouteScans(3); err != nil {
				return nil, nil, err
			}
		}
		edgeTopLeft, edgeBottomRight := edge.BoundingBoxValues()
		if !math.IsInf(edgeTopLeft.X, 0) {
			minX = math.Min(minX, edgeTopLeft.X)
			minY = math.Min(minY, edgeTopLeft.Y)
			maxX = math.Max(maxX, edgeBottomRight.X)
			maxY = math.Max(maxY, edgeBottomRight.Y)
		}
	}
	if err := guard.finish(); err != nil {
		return nil, nil, err
	}
	return geo.NewPoint(math.Round(minX), math.Round(minY)), geo.NewPoint(math.Round(maxX), math.Round(maxY)), nil
}

func (snapshot routeMutationSnapshot) restore() {
	if snapshot.graph != nil {
		snapshot.graph.CellSize = snapshot.cellSize
		snapshot.graph.RestoreRoutingCosts(snapshot.costs)
	}
	for node, topLeft := range snapshot.nodes {
		node.TopLeft = topLeft.restore()
		node.Graph = snapshot.nodeGraphs[node]
	}
	for edge, state := range snapshot.edges {
		state.restore(edge)
	}
}

// runAtomicRouteStage provides one error boundary for each whole stage. A
// panic is deliberately rethrown after restoration so existing invariant
// failure semantics remain unchanged.
func runAtomicRouteStage(
	ctx context.Context,
	location string,
	g *layoutgraph.Graph,
	extraEdges []*layoutgraph.Edge,
	workLimit uint64,
	fn func(*routeWorkGuard) error,
) (err error) {
	if err := layoutgraph.Validate(ctx, location, g); err != nil {
		return err
	}
	return runAtomicRouteStageAfterPreflight(ctx, location, g, extraEdges, workLimit, fn)
}

func runAtomicRouteStageAfterPreflight(
	ctx context.Context,
	location string,
	g *layoutgraph.Graph,
	extraEdges []*layoutgraph.Edge,
	workLimit uint64,
	fn func(*routeWorkGuard) error,
) (err error) {
	guard, err := newRouteWorkGuard(ctx, location, workLimit)
	if err != nil {
		return err
	}
	return runAtomicRouteStageWithGuard(g, extraEdges, guard, fn)
}

// runAtomicRouteStageWithGuard lets a whole pipeline stage use the same work
// budget for its own discovery kernels and the exact mutation boundary. The
// caller is responsible for the graph topology preflight.
func runAtomicRouteStageWithGuard(
	g *layoutgraph.Graph,
	extraEdges []*layoutgraph.Edge,
	guard *routeWorkGuard,
	fn func(*routeWorkGuard) error,
) (err error) {
	if err := validateRouteStageGeometry(g, extraEdges, guard); err != nil {
		return err
	}
	return runAtomicRouteStageWithValidatedGeometry(g, extraEdges, guard, fn)
}

// runAtomicRouteStageWithValidatedGeometry avoids charging the same geometry
// inventory twice when a caller must validate it before choosing an early
// success path. The topology preflight remains the caller's responsibility.
func runAtomicRouteStageWithValidatedGeometry(
	g *layoutgraph.Graph,
	extraEdges []*layoutgraph.Edge,
	guard *routeWorkGuard,
	fn func(*routeWorkGuard) error,
) (err error) {
	snapshot, err := captureRouteMutations(g, extraEdges, guard)
	if err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot.restore()
			panic(recovered)
		}
		if err != nil {
			snapshot.restore()
		}
	}()

	if err := fn(guard); err != nil {
		return err
	}
	return guard.finish()
}

// stableSortRouteValues is a cancellable bottom-up stable merge sort. Stable
// ordering is important to layout determinism, including for values that
// compare equal.
func stableSortRouteValues[T any](values []T, less func(T, T) bool, guard *routeWorkGuard) error {
	if len(values) < 2 {
		return guard.step()
	}
	temporary := make([]T, len(values))
	for width := 1; width < len(values); {
		for start := 0; start < len(values); start += 2 * width {
			middle := min(start+width, len(values))
			end := min(start+2*width, len(values))
			left, right, output := start, middle, start
			for left < middle && right < end {
				if err := guard.step(); err != nil {
					return err
				}
				// Prefer the left value when equivalent to preserve stability.
				if less(values[right], values[left]) {
					temporary[output] = values[right]
					right++
				} else {
					temporary[output] = values[left]
					left++
				}
				output++
			}
			for left < middle {
				if err := guard.step(); err != nil {
					return err
				}
				temporary[output] = values[left]
				left++
				output++
			}
			for right < end {
				if err := guard.step(); err != nil {
					return err
				}
				temporary[output] = values[right]
				right++
				output++
			}
		}
		if err := guard.add(uint64(len(values))); err != nil {
			return err
		}
		copy(values, temporary)
		if width > len(values)/2 {
			break
		}
		width *= 2
	}
	return nil
}
