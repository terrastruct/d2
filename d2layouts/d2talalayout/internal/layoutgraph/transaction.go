package layoutgraph

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

type GraphState struct {
	// nodeGeometry is also the transaction's deduplicated node set.
	nodeGeometry     map[*Node]graphGeometrySnapshot
	originalNodes    []*Node
	originalNodesRef []*Node
	hasFixedTopLeft  bool

	clusterArrangements map[*Cluster]ClusterArrangement
	clusterDesired      map[*Cluster]ClusterArrangement
	clusterPaddings     map[*Cluster]float64
	edgeGeometry        map[*Edge]edgeSnapshot
	treeOrientations    map[*Tree]geo.Orientation

	// We don't want to be frozen due to some existing overlap, since many can be valid diagrams
	// We just care about not introducing new overlaps
	existingOverlaps      map[*Node]map[*Node]struct{}
	existingExactOverlaps map[*Node]map[*Node]struct{}
	captureTopology       bool
	captureEdgeRoutes     bool

	graph          graphSnapshot
	nodes          map[*Node]nodeSnapshot
	edges          map[*Edge]edgeSnapshot
	clusters       map[*Cluster]clusterSnapshot
	sequences      map[*Sequence]sequenceSnapshot
	trees          map[*Tree]treeSnapshot
	edgeAbductions map[*EdgeAbduction]EdgeAbduction
	herds          map[*HerdAssignment]herdSnapshot
	hierarchies    map[*Hierarchy]hierarchySnapshot
}

// graphGeometrySnapshot keeps geometry in one value for speculative commit
// checks and preserves the original position pointer for rollback.
type graphGeometrySnapshot struct {
	topLeft pointerSnapshot[geo.Point]
	width   float64
	height  float64
}

type transactionPendingDescendant struct {
	node *Node
	emit bool
}

type Transaction struct {
	Ops     []func() error
	Graph   *Graph
	options TransactionOptions
	guard   *limits.WorkGuard

	PriorGraphState *GraphState
	spareGraphState *GraphState
	// placementCostSnapshot is optional stage-owned rollback data. Ordinary
	// speculative Rollback deliberately ignores it.
	placementCostSnapshot *PlacementCostSnapshot

	// Geometry-only transactions usually move a small subset of the graph. Reuse
	// these sets across speculative candidates so validation neither scans every
	// pair nor allocates fresh bookkeeping for every Commit.
	dirtyNodes     map[*Node]struct{}
	dirtyNodeMarks []bool
	fixedOrigins   map[*Node]geo.Point

	// Final-state overlap validation uses one sweep-built candidate graph per
	// commit. The slices and pointer index are retained across speculative
	// candidates, which avoids both O(changed*nodes) closure construction and an
	// O(nodes) scan for every node being validated.
	overlapCandidates     [][]int
	overlapSweepNodes     []transactionSweepNode
	overlapActiveNodes    []transactionSweepNode
	overlapNodeIndices    map[*Node][]int
	overlapNodeIndexReady bool
	// Geometry trial clones can share an immutable rollback point. Their first
	// accepted UpdateState must not recycle that shared state as scratch.
	priorGraphStateShared bool
	overlapInvalidBoxes   []bool
	exceptionMarks        []uint32
	exceptionGeneration   uint32
	descendantStack       []transactionPendingDescendant
	descendantNodes       []*Node
	descendantSeen        map[*Node]uint32
	descendantGeneration  uint32
}

type TransactionOptions struct {
	AffectContainers      bool
	IgnoreContainerEscape bool
	// AffectEdgeRoutes records route slice and point identity for operations
	// that move already-routed edges together with nodes.
	AffectEdgeRoutes bool
}

type transactionWorkGuardKey struct{}

func contextWithTransactionWorkGuard(ctx context.Context, guard *limits.WorkGuard) context.Context {
	if ctx == nil || guard == nil {
		return ctx
	}
	return context.WithValue(ctx, transactionWorkGuardKey{}, guard)
}

func existingTransactionWorkGuard(ctx context.Context, location string) (*limits.WorkGuard, bool, error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("TALA %s requires a context", location)
	}
	if guard, ok := ctx.Value(transactionWorkGuardKey{}).(*limits.WorkGuard); ok && guard != nil {
		if err := guard.Finish(); err != nil {
			return nil, false, err
		}
		return guard, true, nil
	}
	return nil, false, nil
}

func ensureTransactionWorkGuard(ctx context.Context, location string) (context.Context, *limits.WorkGuard, error) {
	guard, exists, err := existingTransactionWorkGuard(ctx, location)
	if err != nil {
		return nil, nil, err
	}
	if exists {
		return ctx, guard, nil
	}
	guard, err = limits.NewWorkGuard(ctx, location, maxTransactionWorkUnits)
	if err != nil {
		return nil, nil, err
	}
	return context.WithValue(ctx, transactionWorkGuardKey{}, guard), guard, nil
}

func (g *Graph) newRequestTransaction(ctx context.Context, options TransactionOptions) (*Transaction, error) {
	_, guard, err := ensureTransactionWorkGuard(ctx, "Transaction")
	if err != nil {
		return nil, err
	}
	return g.newTransactionWithOptionsContext(ctx, options, guard)
}

func (g *Graph) newRequestTransactionWithGuard(ctx context.Context, guard *limits.WorkGuard, options TransactionOptions) (*Transaction, error) {
	if guard == nil {
		return nil, fmt.Errorf("TALA transaction requires a shared work guard")
	}
	return g.newTransactionWithOptionsContext(ctx, options, guard)
}

func cloneSlice[V any](values []V) []V {
	cloned := make([]V, len(values))
	copy(cloned, values)
	return cloned
}

func (gs *GraphState) updateContext(g *Graph, guard *limits.WorkGuard) error {
	if g == nil {
		return fmt.Errorf("TALA transaction graph is nil")
	}
	if guard == nil {
		return fmt.Errorf("TALA transaction requires a work guard")
	}
	if len(g.Nodes) > maxEngineNodes {
		return fmt.Errorf("TALA transaction node count exceeds limit %d", maxEngineNodes)
	}
	if len(g.Edges) > maxEngineEdges {
		return fmt.Errorf("TALA transaction edge count exceeds limit %d", maxEngineEdges)
	}
	for _, collection := range []struct {
		name  string
		count int
	}{
		{name: "container", count: len(g.Containers)},
		{name: "cluster", count: len(g.Clusters)},
		{name: "sequence", count: len(g.Sequences)},
		{name: "tree", count: len(g.Trees)},
		{name: "node-tree", count: len(g.NodeToTree)},
		{name: "hub", count: len(g.Hubs)},
		{name: "direction", count: len(g.Directions)},
		{name: "sibling", count: len(g.CommonUncleSiblings)},
	} {
		if collection.count > maxEngineNodes+1 {
			return fmt.Errorf("TALA transaction %s map exceeds limit %d", collection.name, maxEngineNodes+1)
		}
	}
	// Cluster, sequence, and container views commonly repeat the same nodes.
	// Size the initial maps only from the graph's primary view and let Go grow
	// them if the deduped traversal discovers additional unique nodes. The
	// traversal itself, rather than an additive membership estimate, enforces
	// the supported unique-node limit.
	totalNodes := min(len(g.Nodes), maxEngineNodes)

	// Reset or create maps all at once to minimize allocation overhead
	// Using a more efficient approach to allocation
	if gs.nodeGeometry == nil || len(gs.nodeGeometry) > 2*totalNodes {
		// Create new maps when they don't exist or are significantly larger than needed
		gs.nodeGeometry = make(map[*Node]graphGeometrySnapshot, totalNodes)
	} else {
		// Clear maps without reallocating
		clear(gs.nodeGeometry)
	}
	gs.hasFixedTopLeft = false

	clusterCount := len(g.Clusters)
	if gs.clusterArrangements == nil || len(gs.clusterArrangements) > 2*clusterCount {
		gs.clusterArrangements = make(map[*Cluster]ClusterArrangement, clusterCount)
		gs.clusterDesired = make(map[*Cluster]ClusterArrangement, clusterCount)
		gs.clusterPaddings = make(map[*Cluster]float64, clusterCount)
	} else {
		clear(gs.clusterArrangements)
		clear(gs.clusterDesired)
		clear(gs.clusterPaddings)
	}

	gs.originalNodesRef = g.Nodes
	if cap(gs.originalNodes) < len(g.Nodes) {
		gs.originalNodes = make([]*Node, len(g.Nodes))
	} else {
		gs.originalNodes = gs.originalNodes[:len(g.Nodes)]
	}
	copy(gs.originalNodes, g.Nodes)

	recordNode := func(n *Node) (bool, error) {
		if n == nil {
			return false, nil
		}
		if _, exists := gs.nodeGeometry[n]; exists {
			return false, nil
		}
		if len(gs.nodeGeometry) >= maxEngineNodes {
			return false, fmt.Errorf("TALA transaction unique node snapshot exceeds limit %d", maxEngineNodes)
		}
		if err := guard.Step(); err != nil {
			return false, err
		}
		topLeft := snapshotPointer(n.TopLeft)
		gs.nodeGeometry[n] = graphGeometrySnapshot{
			topLeft: topLeft,
			width:   n.Width,
			height:  n.Height,
		}
		gs.hasFixedTopLeft = gs.hasFixedTopLeft || n.FixedTopLeft != nil
		return true, nil
	}

	recordDescendants := func(root *Node) error {
		queue := []*Node{root}
		for len(queue) > 0 {
			node := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			recorded, err := recordNode(node)
			if err != nil {
				return err
			}
			if !recorded {
				continue
			}
			for range g.Containers[node] {
				if err := guard.Step(); err != nil {
					return err
				}
			}
			queue = append(queue, g.Containers[node]...)
		}
		return nil
	}
	for _, n := range g.Nodes {
		if err := recordDescendants(n); err != nil {
			return err
		}
	}
	// Some speculative subgraphs keep descendants outside g.Nodes. Record map
	// roots and values as well so rollback does not depend on slice membership.
	for container, children := range g.Containers {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := recordDescendants(container); err != nil {
			return err
		}
		for _, child := range children {
			if err := recordDescendants(child); err != nil {
				return err
			}
		}
	}

	// Store cluster policy and nodes.
	for _, c := range g.Clusters {
		if err := guard.Step(); err != nil {
			return err
		}
		if c == nil {
			continue
		}
		gs.clusterArrangements[c] = c.Arrangement
		gs.clusterDesired[c] = c.DesiredArrangement
		gs.clusterPaddings[c] = c.Padding
		if err := recordDescendants(c.Vessel); err != nil {
			return err
		}
		if err := recordDescendants(c.Container); err != nil {
			return err
		}
		for _, n := range c.Nodes {
			if err := recordDescendants(n); err != nil {
				return err
			}
		}
	}

	// Store sequence nodes that may be temporarily removed from g.Nodes.
	for _, s := range g.Sequences {
		if err := guard.Step(); err != nil {
			return err
		}
		if s == nil {
			continue
		}
		if err := recordDescendants(s.Vessel); err != nil {
			return err
		}
		if err := recordDescendants(s.Container); err != nil {
			return err
		}
		for _, n := range s.Nodes {
			if err := recordDescendants(n); err != nil {
				return err
			}
		}
	}

	if gs.captureTopology {
		if err := gs.captureRuntimeStateContext(g, guard); err != nil {
			return err
		}
	} else {
		if err := gs.captureGeometryStateContext(g, guard); err != nil {
			return err
		}
	}
	return guard.Finish()
}

// PreservePriorGraphState pins the current rollback point so a later
// UpdateState cannot recycle it as scratch. Callers may retain the returned
// state for a larger atomic operation that spans multiple transaction commits.
func (t *Transaction) PreservePriorGraphState() *GraphState {
	if t == nil {
		return nil
	}
	t.priorGraphStateShared = true
	return t.PriorGraphState
}

// CapturePlacementCosts attaches an optional stage-owned cost rollback point
// to this transaction. Ordinary Transaction.Rollback deliberately ignores it:
// speculative rejections retain memoized costs, while the enclosing stage
// decides whether to restore them.
func (t *Transaction) CapturePlacementCosts(location string) error {
	if t == nil || t.Graph == nil || t.guard == nil {
		return fmt.Errorf("TALA %s placement-cost snapshot requires an initialized transaction", location)
	}
	if t.placementCostSnapshot != nil {
		return nil
	}
	cacheEntries := t.Graph.EdgeLengthCacheEntries()
	if int64(cacheEntries) > maxEngineTopologyReferences {
		return fmt.Errorf("TALA %s edge-length cache entries exceed limit %d", location, maxEngineTopologyReferences)
	}
	for range cacheEntries {
		if err := t.guard.Step(); err != nil {
			return err
		}
	}
	costs := t.Graph.SnapshotPlacementCosts()
	if err := t.guard.Finish(); err != nil {
		return err
	}
	t.placementCostSnapshot = costs
	return nil
}

// RestorePlacementCosts restores a captured placement-cost rollback point and
// reports whether one was present.
func (t *Transaction) RestorePlacementCosts() bool {
	if t == nil || t.placementCostSnapshot == nil {
		return false
	}
	costs := t.placementCostSnapshot
	t.placementCostSnapshot = nil
	costs.Restore()
	return true
}

func transactionSweepPadding(g *Graph, guard *limits.WorkGuard) (float64, error) {
	padding := float64(TableNodeGap)
	maxMargin := 0.0
	maxLoopOffset := 0.0
	edges := make(map[*Edge]struct{}, len(g.Edges))
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if node == nil {
			return 0, fmt.Errorf("TALA transaction contains a nil node")
		}
		if node.TopLeft != nil {
			if !transactionNodeHasFiniteBox(node) {
				// Empty containers deliberately use NaN/+Inf sentinel geometry until
				// their final fit stage. The legacy all-pairs predicates never treated
				// those sentinels as overlaps, so omit them from the spatial index too.
				if node.isContainer && len(g.Containers[node]) == 0 {
					continue
				}
				return 0, invariant.Errorf(
					"transaction node %d has invalid geometry (x=%v y=%v width=%v height=%v)",
					node.ID, node.TopLeft.X, node.TopLeft.Y, node.Width, node.Height,
				)
			}
		}
		for _, margin := range []float64{node.margin.top, node.margin.right, node.margin.bottom, node.margin.left} {
			if !transactionFinite(margin) {
				return 0, invariant.Errorf("transaction node %d has non-finite margin", node.ID)
			}
		}
		maxMargin = max(maxMargin, node.margin.top, node.margin.right, node.margin.bottom, node.margin.left)
		for _, offset := range node.LoopOffsets {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if !transactionFinite(offset) {
				return 0, invariant.Errorf("transaction node %d has a non-finite loop offset", node.ID)
			}
			if offset > maxLoopOffset {
				maxLoopOffset = offset
			}
		}
		// Direct internal callers can temporarily expose an edge through only one
		// endpoint before graph.Edges is synchronized. Include those references so
		// the broad phase never drops a pair that deltaTo would inspect.
		for _, edge := range node.Edges {
			if err := guard.Step(); err != nil {
				return 0, err
			}
			if edge == nil {
				return 0, fmt.Errorf("TALA transaction node %d contains a nil edge", node.ID)
			}
			edges[edge] = struct{}{}
		}
	}
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if edge == nil {
			return 0, fmt.Errorf("TALA transaction contains a nil edge")
		}
		edges[edge] = struct{}{}
	}
	for edge := range edges {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		padding = max(padding, float64(edge.MinWidth), float64(edge.MinHeight))
	}
	// deltaTo can combine one directional margin or loop offset from each
	// node in the pair. Twice the graph maximum is therefore a proven broad-
	// phase bound for every exact predicate evaluated below.
	padding = max(padding, 2*maxMargin, float64(NodeGap)+2*maxLoopOffset)
	if !transactionFinite(padding) {
		return 0, invariant.New("transaction overlap padding is non-finite")
	}
	return padding, guard.Finish()
}

func transactionFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func transactionNodeHasFiniteBox(node *Node) bool {
	return node != nil && node.TopLeft != nil &&
		transactionFinite(node.TopLeft.X) && transactionFinite(node.TopLeft.Y) &&
		transactionFinite(node.Width) && transactionFinite(node.Height) &&
		transactionFinite(node.TopLeft.X+node.Width) && transactionFinite(node.TopLeft.Y+node.Height) &&
		node.Width >= 0 && node.Height >= 0
}

type transactionSweepNode struct {
	node  *Node
	index int
}

func buildTransactionOverlaps(g *Graph, guard *limits.WorkGuard) (map[*Node]map[*Node]struct{}, map[*Node]map[*Node]struct{}, error) {
	return buildTransactionOverlapsWithReferenceLimit(g, guard, maxTransactionOverlapReferences)
}

func buildTransactionOverlapsWithReferenceLimit(g *Graph, guard *limits.WorkGuard, referenceLimit int64) (map[*Node]map[*Node]struct{}, map[*Node]map[*Node]struct{}, error) {
	padding, err := transactionSweepPadding(g, guard)
	if err != nil {
		return nil, nil, err
	}
	placed := make([]transactionSweepNode, 0, len(g.Nodes))
	for index, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, nil, err
		}
		if node.TopLeft != nil && transactionNodeHasFiniteBox(node) {
			placed = append(placed, transactionSweepNode{node: node, index: index})
		}
	}
	if err := guard.Step(); err != nil {
		return nil, nil, err
	}
	slices.SortStableFunc(placed, func(left, right transactionSweepNode) int {
		if left.node.TopLeft.X != right.node.TopLeft.X {
			switch {
			case left.node.TopLeft.X < right.node.TopLeft.X:
				return -1
			case right.node.TopLeft.X < left.node.TopLeft.X:
				return 1
			default:
				return 0
			}
		}
		if order := cmp.Compare(left.node.ID, right.node.ID); order != 0 {
			return order
		}
		return cmp.Compare(left.index, right.index)
	})

	existingOverlaps := make(map[*Node]map[*Node]struct{})
	existingExactOverlaps := make(map[*Node]map[*Node]struct{})
	var retainedReferences int64
	addPair := func(overlaps map[*Node]map[*Node]struct{}, first, second *Node) error {
		if referenceLimit < 2 || retainedReferences > referenceLimit-2 {
			return fmt.Errorf("TALA transaction overlap references exceed limit %d", referenceLimit)
		}
		if overlaps[first] == nil {
			overlaps[first] = make(map[*Node]struct{})
		}
		if overlaps[second] == nil {
			overlaps[second] = make(map[*Node]struct{})
		}
		overlaps[first][second] = struct{}{}
		overlaps[second][first] = struct{}{}
		retainedReferences += 2
		return nil
	}

	active := make([]*Node, 0)
	for _, current := range placed {
		if err := guard.Step(); err != nil {
			return nil, nil, err
		}
		kept := active[:0]
		for _, other := range active {
			if err := guard.Step(); err != nil {
				return nil, nil, err
			}
			// g.Nodes is a legacy public slice and malformed direct callers can
			// repeat the same pointer. The old all-pairs scan explicitly skipped
			// identity pairs, so the sweep must not retain a self-overlap either.
			if other == current.node {
				continue
			}
			if other.TopLeft.X+other.Width+padding <= current.node.TopLeft.X {
				continue
			}
			kept = append(kept, other)
			if other.TopLeft.Y >= current.node.TopLeft.Y+current.node.Height+padding ||
				current.node.TopLeft.Y >= other.TopLeft.Y+other.Height+padding {
				continue
			}
			if current.node.DoesOverlapExact(other) {
				if err := addPair(existingExactOverlaps, current.node, other); err != nil {
					return nil, nil, err
				}
			}
			// Preserve the old ordered-pair semantics even if a malformed direct
			// caller has asymmetric edge references.
			if current.node.doesOverlap(other) || other.doesOverlap(current.node) {
				if err := addPair(existingOverlaps, current.node, other); err != nil {
					return nil, nil, err
				}
			}
		}
		active = append(kept, current.node)
	}
	if err := guard.Finish(); err != nil {
		return nil, nil, err
	}
	return existingOverlaps, existingExactOverlaps, nil
}

// newTransactionWithOptionsContext constructs a transaction using a request-scoped
// work guard so production stages can share one aggregate budget across all
// speculative candidates.
func (g *Graph) newTransactionWithOptionsContext(ctx context.Context, options TransactionOptions, guard *limits.WorkGuard) (*Transaction, error) {
	if guard == nil {
		var err error
		_, guard, err = ensureTransactionWorkGuard(ctx, "Transaction")
		if err != nil {
			return nil, err
		}
	} else if ctx == nil {
		return nil, fmt.Errorf("TALA transaction requires a context")
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	graphState := &GraphState{captureEdgeRoutes: options.AffectEdgeRoutes}
	if err := graphState.updateContext(g, guard); err != nil {
		return nil, err
	}

	existingOverlaps, existingExactOverlaps, err := buildTransactionOverlaps(g, guard)
	if err != nil {
		return nil, err
	}

	graphState.existingOverlaps = existingOverlaps
	graphState.existingExactOverlaps = existingExactOverlaps
	txn := &Transaction{
		Ops:     make([]func() error, 0),
		Graph:   g,
		options: options,
		guard:   guard,

		PriorGraphState: graphState,
	}

	return txn, guard.Finish()
}

func (t *Transaction) UpdateState() error {
	// Existing-overlap exceptions intentionally remain anchored to transaction
	// construction. Reusing a transaction updates the rollback point, not which
	// preexisting collisions are allowed. Snapshot refresh remains cancellable
	// and budgeted.
	previous := t.PriorGraphState
	updated := t.spareGraphState
	if updated == nil {
		updated = &GraphState{}
	}
	updated.captureTopology = previous.captureTopology
	updated.captureEdgeRoutes = previous.captureEdgeRoutes
	if err := updated.updateContext(t.Graph, t.guard); err != nil {
		// updateContext only mutates the spare snapshot. The prior rollback point
		// remains exact, so a failed refresh can reject the accepted candidate.
		t.Rollback()
		return err
	}
	updated.existingOverlaps = previous.existingOverlaps
	updated.existingExactOverlaps = previous.existingExactOverlaps
	t.PriorGraphState = updated
	if t.priorGraphStateShared {
		t.spareGraphState = nil
		t.priorGraphStateShared = false
	} else {
		t.spareGraphState = previous
	}
	return nil
}

func (t *Transaction) AddOp(fn func() error) {
	t.Ops = append(t.Ops, fn)
}

func (t *Transaction) repositionContainers(ctx context.Context) error {
	smallestToLargest := make([]*Node, 0, len(t.Graph.Containers))
	for container := range t.Graph.Containers {
		if err := t.guard.Step(); err != nil {
			return err
		}
		if container == nil || container.TopLeft == nil {
			continue
		}
		smallestToLargest = append(smallestToLargest, container)
	}
	sort.Slice(smallestToLargest, func(i, j int) bool {
		if smallestToLargest[i].area() == smallestToLargest[j].area() {
			return smallestToLargest[i].ID < smallestToLargest[j].ID
		}
		return smallestToLargest[i].area() < smallestToLargest[j].area()
	})

	for _, container := range smallestToLargest {
		if err := ctx.Err(); err != nil {
			return err
		}
		container.wrapChildren()
		if err := t.guard.Step(); err != nil {
			return err
		}
	}
	return t.guard.Finish()
}

func (gs *GraphState) hasOriginalNodeOrder(g *Graph) bool {
	if gs == nil || g == nil || len(gs.originalNodes) != len(g.Nodes) {
		return false
	}
	for i, node := range g.Nodes {
		if gs.originalNodes[i] != node {
			return false
		}
	}
	return true
}

func (gs *GraphState) geometryChanged(node *Node) bool {
	if gs == nil || node == nil {
		return true
	}
	original, ok := gs.nodeGeometry[node]
	if !ok {
		return true
	}
	if node.TopLeft == nil {
		return original.topLeft.pointer != nil ||
			node.Width != original.width || node.Height != original.height
	}
	return original.topLeft.pointer == nil ||
		node.TopLeft.X != original.topLeft.value.X || node.TopLeft.Y != original.topLeft.value.Y ||
		node.Width != original.width || node.Height != original.height
}

func transactionBoxesMayInteract(first, second *Node) bool {
	if first == nil || second == nil || first == second || first.TopLeft == nil || second.TopLeft == nil {
		return false
	}
	// This is the same symmetric broad phase used by
	// doesOverlapWithDimensionsContext. NaN values deliberately survive the
	// comparisons and are sent to the normal bad-state path.
	const maxSafeDelta = 500.0
	return !(first.TopLeft.X > second.TopLeft.X+second.Width+maxSafeDelta ||
		first.TopLeft.X+first.Width+maxSafeDelta < second.TopLeft.X ||
		first.TopLeft.Y > second.TopLeft.Y+second.Height+maxSafeDelta ||
		first.TopLeft.Y+first.Height+maxSafeDelta < second.TopLeft.Y)
}

func (t *Transaction) collectFixedOrigins(ctx context.Context) error {
	if t.fixedOrigins == nil {
		t.fixedOrigins = make(map[*Node]geo.Point)
	} else {
		clear(t.fixedOrigins)
	}
	if !t.PriorGraphState.hasFixedTopLeft {
		return t.guard.Finish()
	}
	for _, node := range t.Graph.Nodes {
		if err := t.guard.Step(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if node == nil || node.TopLeft == nil || node.FixedTopLeft == nil {
			continue
		}
		container := node.container()
		if _, found := t.fixedOrigins[container]; found {
			continue
		}
		t.fixedOrigins[container] = geo.Point{
			X: node.TopLeft.X - node.FixedTopLeft.X,
			Y: node.TopLeft.Y - node.FixedTopLeft.Y,
		}
	}
	return t.guard.Finish()
}

// collectOverlapValidationNodes records geometry changes for the legacy
// non-exact-to-exact regression check. Ordered overlap validation itself uses a
// complete post-state sweep, so it no longer needs a changed-node closure.
func (t *Transaction) collectOverlapValidationNodes(ctx context.Context) (useDirtyValidation bool, err error) {
	if !t.PriorGraphState.hasOriginalNodeOrder(t.Graph) {
		return false, nil
	}
	if t.dirtyNodes == nil {
		t.dirtyNodes = make(map[*Node]struct{})
	} else {
		clear(t.dirtyNodes)
	}
	if cap(t.dirtyNodeMarks) < len(t.Graph.Nodes) {
		t.dirtyNodeMarks = make([]bool, len(t.Graph.Nodes))
	} else {
		t.dirtyNodeMarks = t.dirtyNodeMarks[:len(t.Graph.Nodes)]
		clear(t.dirtyNodeMarks)
	}

	for index, node := range t.Graph.Nodes {
		if err := t.guard.Step(); err != nil {
			return false, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if t.PriorGraphState.geometryChanged(node) {
			t.dirtyNodes[node] = struct{}{}
			t.dirtyNodeMarks[index] = true
		}
	}
	return true, t.guard.Finish()
}

// buildPostStateOverlapCandidates builds the exact 500-unit broad phase used
// by doesOverlapWithDimensionsContext. Sparse geometry transactions retain only
// pairs incident to a changed box; full geometry scans retain every pair.
// Candidate lists are sorted by g.Nodes index so directional deltaTo and
// first-rejection order remain unchanged.
func (t *Transaction) buildPostStateOverlapCandidates(ctx context.Context, dirtyOnly bool) error {
	nodeCount := len(t.Graph.Nodes)
	if nodeCount > maxEngineNodes {
		return fmt.Errorf("TALA transaction node count exceeds limit %d", maxEngineNodes)
	}
	if cap(t.overlapCandidates) < nodeCount {
		t.overlapCandidates = make([][]int, nodeCount)
	} else {
		t.overlapCandidates = t.overlapCandidates[:nodeCount]
	}
	for index := range t.overlapCandidates {
		t.overlapCandidates[index] = t.overlapCandidates[index][:0]
	}
	if cap(t.overlapInvalidBoxes) < nodeCount {
		t.overlapInvalidBoxes = make([]bool, nodeCount)
	} else {
		t.overlapInvalidBoxes = t.overlapInvalidBoxes[:nodeCount]
		clear(t.overlapInvalidBoxes)
	}
	if cap(t.exceptionMarks) < nodeCount {
		t.exceptionMarks = make([]uint32, nodeCount)
	} else {
		t.exceptionMarks = t.exceptionMarks[:nodeCount]
	}

	rebuildNodeIndex := !dirtyOnly || !t.overlapNodeIndexReady
	if t.overlapNodeIndices == nil {
		t.overlapNodeIndices = make(map[*Node][]int, nodeCount)
		rebuildNodeIndex = true
	} else if rebuildNodeIndex {
		for node, indices := range t.overlapNodeIndices {
			t.overlapNodeIndices[node] = indices[:0]
		}
	}
	t.overlapSweepNodes = t.overlapSweepNodes[:0]
	hasInvalidBox := false
	dirtyCount := 0
	for index, node := range t.Graph.Nodes {
		if err := t.guard.Step(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if node == nil {
			return invariant.New("transaction overlap check encountered a nil graph node")
		}
		if rebuildNodeIndex {
			t.overlapNodeIndices[node] = append(t.overlapNodeIndices[node], index)
		}
		if dirtyOnly && t.dirtyNodeMarks[index] {
			dirtyCount++
		}
		if node.TopLeft == nil {
			continue
		}
		if !transactionNodeHasFiniteBox(node) {
			t.overlapInvalidBoxes[index] = true
			hasInvalidBox = true
			continue
		}
		t.overlapSweepNodes = append(t.overlapSweepNodes, transactionSweepNode{node: node, index: index})
	}
	t.overlapNodeIndexReady = dirtyOnly

	retainedReferences := int64(0)
	addPair := func(first, second int) error {
		if retainedReferences > maxTransactionOverlapReferences-2 {
			return fmt.Errorf("TALA transaction overlap references exceed limit %d", maxTransactionOverlapReferences)
		}
		t.overlapCandidates[first] = append(t.overlapCandidates[first], second)
		t.overlapCandidates[second] = append(t.overlapCandidates[second], first)
		retainedReferences += 2
		return nil
	}

	// A sparse geometry trial is cheapest as changed-nodes x graph. When many
	// boxes move, the sweep avoids scanning pairs that are far apart.
	useDirtyScan := dirtyOnly && dirtyCount*4 < nodeCount
	if useDirtyScan {
		for first, firstNode := range t.Graph.Nodes {
			if !t.dirtyNodeMarks[first] || firstNode.TopLeft == nil {
				continue
			}
			for second, secondNode := range t.Graph.Nodes {
				if err := t.guard.Step(); err != nil {
					return err
				}
				if second == first || firstNode == secondNode || secondNode.TopLeft == nil {
					continue
				}
				if t.dirtyNodeMarks[second] && second < first {
					continue
				}
				if !transactionBoxesMayInteract(firstNode, secondNode) {
					continue
				}
				if err := addPair(first, second); err != nil {
					return err
				}
			}
		}
	} else {
		slices.SortStableFunc(t.overlapSweepNodes, func(left, right transactionSweepNode) int {
			if left.node.TopLeft.X != right.node.TopLeft.X {
				switch {
				case left.node.TopLeft.X < right.node.TopLeft.X:
					return -1
				case right.node.TopLeft.X < left.node.TopLeft.X:
					return 1
				default:
					return 0
				}
			}
			if order := cmp.Compare(left.node.ID, right.node.ID); order != 0 {
				return order
			}
			return cmp.Compare(left.index, right.index)
		})
		t.overlapActiveNodes = t.overlapActiveNodes[:0]
		for _, current := range t.overlapSweepNodes {
			if err := t.guard.Step(); err != nil {
				return err
			}
			kept := t.overlapActiveNodes[:0]
			for _, other := range t.overlapActiveNodes {
				if err := t.guard.Step(); err != nil {
					return err
				}
				// Equality remains active because the legacy broad phase only rejects
				// a pair when there is a strict gap greater than 500.
				if other.node.TopLeft.X+other.node.Width+500 < current.node.TopLeft.X {
					continue
				}
				kept = append(kept, other)
				if other.node == current.node || !transactionBoxesMayInteract(other.node, current.node) {
					continue
				}
				if dirtyOnly {
					if !t.dirtyNodeMarks[other.index] && !t.dirtyNodeMarks[current.index] {
						continue
					}
				}
				if err := addPair(other.index, current.index); err != nil {
					return err
				}
			}
			t.overlapActiveNodes = append(kept, current)
		}

		// Non-finite or negative boxes deliberately follow legacy comparisons,
		// whose NaN behavior cannot be represented by a numeric sweep ordering.
		if hasInvalidBox {
			for first := 0; first < nodeCount; first++ {
				for second := first + 1; second < nodeCount; second++ {
					if !t.overlapInvalidBoxes[first] && !t.overlapInvalidBoxes[second] {
						continue
					}
					if dirtyOnly {
						if !t.dirtyNodeMarks[first] && !t.dirtyNodeMarks[second] {
							continue
						}
					}
					if err := t.guard.Step(); err != nil {
						return err
					}
					firstNode, secondNode := t.Graph.Nodes[first], t.Graph.Nodes[second]
					if firstNode == secondNode || firstNode.TopLeft == nil || secondNode.TopLeft == nil ||
						!transactionBoxesMayInteract(firstNode, secondNode) {
						continue
					}
					if err := addPair(first, second); err != nil {
						return err
					}
				}
			}
		}
	}

	for _, candidates := range t.overlapCandidates {
		if err := t.guard.Step(); err != nil {
			return err
		}
		sort.Ints(candidates)
	}
	return t.guard.Finish()
}

func (t *Transaction) markOverlapException(node *Node, generation uint32) {
	for _, index := range t.overlapNodeIndices[node] {
		t.exceptionMarks[index] = generation
	}
}

// markDescendantOverlapExceptions performs the same stable descendant walk as
// allDescendantNodesGuarded, but retains its stack, result slice, and seen
// set across the thousands of speculative transaction checks in a layout.
func (t *Transaction) markDescendantOverlapExceptions(node *Node, includeClusterNodes bool, generation uint32) error {
	t.descendantGeneration++
	if t.descendantGeneration == 0 {
		clear(t.descendantSeen)
		t.descendantGeneration = 1
	}
	seenGeneration := t.descendantGeneration
	if t.descendantSeen == nil {
		t.descendantSeen = make(map[*Node]uint32)
	}
	t.descendantStack = t.descendantStack[:0]
	t.descendantNodes = t.descendantNodes[:0]
	if node != nil {
		t.descendantSeen[node] = seenGeneration
	}

	pushChildren := func(parent *Node) error {
		if sequence := t.Graph.Sequences[parent]; sequence != nil {
			for i := len(sequence.Nodes) - 1; i >= 0; i-- {
				if err := t.guard.Step(); err != nil {
					return err
				}
				t.descendantStack = append(t.descendantStack, transactionPendingDescendant{
					node: sequence.Nodes[i], emit: includeClusterNodes,
				})
			}
		}
		if parent != nil && parent.isClusterVessel {
			if cluster := t.Graph.Clusters[parent]; cluster != nil {
				for i := len(cluster.Nodes) - 1; i >= 0; i-- {
					if err := t.guard.Step(); err != nil {
						return err
					}
					t.descendantStack = append(t.descendantStack, transactionPendingDescendant{
						node: cluster.Nodes[i], emit: includeClusterNodes,
					})
				}
			}
		}
		if parent == nil || parent.isContainer {
			children := t.Graph.Containers[parent]
			for i := len(children) - 1; i >= 0; i-- {
				if err := t.guard.Step(); err != nil {
					return err
				}
				t.descendantStack = append(t.descendantStack, transactionPendingDescendant{
					node: children[i], emit: true,
				})
			}
		}
		return nil
	}
	if err := pushChildren(node); err != nil {
		return err
	}
	for len(t.descendantStack) > 0 {
		if err := t.guard.Step(); err != nil {
			return err
		}
		last := len(t.descendantStack) - 1
		current := t.descendantStack[last]
		t.descendantStack = t.descendantStack[:last]
		if current.node == nil || t.descendantSeen[current.node] == seenGeneration {
			continue
		}
		t.descendantSeen[current.node] = seenGeneration
		if current.emit {
			t.descendantNodes = append(t.descendantNodes, current.node)
		}
		if err := pushChildren(current.node); err != nil {
			return err
		}
	}
	for _, descendant := range t.descendantNodes {
		if err := t.guard.Step(); err != nil {
			return err
		}
		t.markOverlapException(descendant, generation)
	}
	return t.guard.Finish()
}

func (t *Transaction) hasBadPostStateOverlap(nodeIndex int) (bool, error) {
	node := t.Graph.Nodes[nodeIndex]
	if node == nil || node.TopLeft == nil {
		return node == nil, nil
	}
	if node.Cluster != nil {
		return false, nil
	}

	t.exceptionGeneration++
	if t.exceptionGeneration == 0 {
		clear(t.exceptionMarks)
		t.exceptionGeneration = 1
	}
	generation := t.exceptionGeneration
	if node.isContainer || node.isClusterVessel || t.Graph.Sequences[node] != nil {
		if err := t.markDescendantOverlapExceptions(node, true, generation); err != nil {
			return false, err
		}
	}
	if node.Container != nil || node.Cluster != nil || node.Sequence != nil {
		ancestors, err := t.Graph.ancestorsOfGuarded(node, t.guard)
		if err != nil {
			return false, err
		}
		for _, ancestor := range ancestors {
			if err := t.guard.Step(); err != nil {
				return false, err
			}
			t.markOverlapException(ancestor, generation)
		}
	}

	pairwiseExceptions := t.PriorGraphState.existingOverlaps[node]
	right := node.TopLeft.X + node.Width
	bottom := node.TopLeft.Y + node.Height
	for _, otherIndex := range t.overlapCandidates[nodeIndex] {
		if err := t.guard.Step(); err != nil {
			return false, err
		}
		other := t.Graph.Nodes[otherIndex]
		if other == nil {
			return false, invariant.New("transaction overlap check encountered a nil graph node")
		}
		if _, excepted := pairwiseExceptions[other]; excepted || t.exceptionMarks[otherIndex] == generation {
			continue
		}
		if other.TopLeft == nil || !transactionBoxesMayInteract(node, other) {
			continue
		}
		deltaValue, err := node.deltaToGuarded(other, node.TopLeft, t.guard)
		if err != nil {
			return false, err
		}
		delta := float64(deltaValue)
		if node.TopLeft.X < other.TopLeft.X+other.Width+delta && right+delta > other.TopLeft.X &&
			node.TopLeft.Y < other.TopLeft.Y+other.Height+delta && bottom+delta > other.TopLeft.Y {
			return true, nil
		}
	}
	return false, t.guard.Finish()
}

func (t *Transaction) Commit(ctx context.Context) (err error) {
	// A rejected trial is atomic: callers never observe a half-applied graph.
	// Keep Rollback idempotent because existing call sites also invoke it after
	// an error and after successful candidate evaluation.
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Rollback()
			panic(recovered)
		}
		if err != nil {
			t.Rollback()
		}
	}()

	if ctx == nil {
		return fmt.Errorf("TALA transaction commit requires a context")
	}
	if t.guard == nil {
		return fmt.Errorf("TALA transaction commit requires a work guard")
	}
	if err := t.guard.Finish(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, op := range t.Ops {
		if err := t.guard.Step(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := op(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Syncing clusters will reposition cluster nodes, which may shrink containers
		// But repositioning containers can also expand clusters, since cluster nodes can be containers
		// TODO by this logic I think we need to run reposition containers again after syncCluster, but can't find a scenario where it matter
		if t.options.AffectContainers {
			if err := t.repositionContainers(ctx); err != nil {
				return err
			}
			for container := range t.Graph.Containers {
				if err := t.guard.Step(); err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if container == nil || container.TopLeft == nil {
					continue
				}
				bad, badErr := t.Graph.isBadStateContext(container, t.PriorGraphState, t.options.IgnoreContainerEscape, t.guard)
				if badErr != nil {
					return badErr
				}
				if bad {
					return ErrInvalidCandidate
				}
			}
		}
		t.Graph.SyncClusters()
		t.Graph.SyncSequences()
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	useDirtyValidation, err := t.collectOverlapValidationNodes(ctx)
	if err != nil {
		return err
	}
	if err := t.collectFixedOrigins(ctx); err != nil {
		return err
	}
	// Containment and fixed-position invariants remain graph-wide. Pairwise
	// overlap is validated afterward from one stable post-state sweep.
	for _, n := range t.Graph.Nodes {
		if err := t.guard.Step(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if n == nil {
			return invariant.New("transaction bad-state check received a nil node")
		}
		if n.TopLeft == nil {
			continue
		}
		fixedOrigin, hasFixedOrigin := t.fixedOrigins[n.container()]
		var fixedOriginPointer *geo.Point
		if hasFixedOrigin {
			fixedOriginPointer = &fixedOrigin
		}
		bad, _, badErr := t.Graph.isStructurallyBadStateWithFixedOriginContext(
			n,
			t.PriorGraphState,
			t.options.IgnoreContainerEscape,
			fixedOriginPointer,
			true,
			t.guard,
		)
		if badErr != nil {
			return badErr
		}
		if bad {
			return ErrInvalidCandidate
		}
	}
	if err := t.buildPostStateOverlapCandidates(ctx, useDirtyValidation); err != nil {
		return err
	}
	for nodeIndex, n := range t.Graph.Nodes {
		if len(t.overlapCandidates[nodeIndex]) == 0 {
			continue
		}
		if err := t.guard.Step(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if n.TopLeft == nil {
			continue
		}
		bad, badErr := t.hasBadPostStateOverlap(nodeIndex)
		if badErr != nil {
			return badErr
		}
		if bad {
			return ErrInvalidCandidate
		}
	}

	// If a non-exact overlap became an exact overlap, that's bad
	for n1, overlaps := range t.PriorGraphState.existingOverlaps {
		if err := t.guard.Step(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		for n2 := range overlaps {
			if useDirtyValidation {
				_, firstChanged := t.dirtyNodes[n1]
				_, secondChanged := t.dirtyNodes[n2]
				if !firstChanged && !secondChanged {
					continue
				}
			}
			if err := t.guard.Step(); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			wasExact := false
			if _, is := t.PriorGraphState.existingExactOverlaps[n1]; is {
				if _, alsoIs := t.PriorGraphState.existingExactOverlaps[n1][n2]; alsoIs {
					wasExact = true
				}
			}
			if !wasExact {
				if n1.DoesOverlapExact(n2) {
					return ErrInvalidCandidate
				}
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return t.guard.Finish()
}

// Clear only needs to be called if transaction will be reused
func (t *Transaction) Clear() {
	t.Ops = make([]func() error, 0)
}

func (t *Transaction) Rollback() {
	state := t.PriorGraphState
	if state.captureTopology {
		state.graph.restore(t.Graph)

		for cluster, snapshot := range state.clusters {
			*cluster = snapshot.value
			cluster.Nodes = snapshot.nodes.restore()
			cluster.EdgeAbductions = snapshot.edgeAbductions.restore()
		}
		for sequence, snapshot := range state.sequences {
			*sequence = snapshot.value
			sequence.Nodes = snapshot.nodes.restore()
			sequence.EdgeAbductions = snapshot.edgeAbductions.restore()
		}
		for tree, snapshot := range state.trees {
			*tree = snapshot.value
			tree.Children = snapshot.children.restore()
		}
		for abduction, snapshot := range state.edgeAbductions {
			*abduction = snapshot
		}
		for herd, snapshot := range state.herds {
			*herd = snapshot.value
			herd.oppositeSidePaired = restoreMap(snapshot.value.oppositeSidePaired, snapshot.oppositeSidePaired)
			herd.sameSidePaired = restoreMap(snapshot.value.sameSidePaired, snapshot.sameSidePaired)
		}
		for hierarchy, snapshot := range state.hierarchies {
			*hierarchy = snapshot.value
			hierarchy.level = restoreMap(snapshot.value.level, snapshot.level)
		}
		for edge, snapshot := range state.edges {
			snapshot.restore(edge)
		}
		for node, snapshot := range state.nodes {
			snapshot.restore(node)
		}
		return
	}

	rollbackNode := func(node *Node) {
		geometry := state.nodeGeometry[node]
		node.TopLeft = geometry.topLeft.restore()
		node.Width = geometry.width
		node.Height = geometry.height
	}
	for node := range state.nodeGeometry {
		rollbackNode(node)
	}
	for cluster, arrangement := range state.clusterArrangements {
		cluster.Arrangement = arrangement
		cluster.DesiredArrangement = state.clusterDesired[cluster]
		cluster.Padding = state.clusterPaddings[cluster]
	}
	for edge, snapshot := range state.edgeGeometry {
		snapshot.restore(edge)
	}
	for tree, orientation := range state.treeOrientations {
		tree.Orientation = orientation
	}
	copy(state.originalNodesRef, state.originalNodes)
	t.Graph.Nodes = state.originalNodesRef
}

// CloneGeometryContext creates a nested speculative transaction that may only
// move geometry. GraphState snapshots are immutable for such trials, so sharing
// the rollback point avoids cloning every node map and overlap-exception set.
// UpdateState detaches on the first accepted candidate before recycling state.
func (t *Transaction) CloneGeometryContext() (*Transaction, error) {
	if t == nil || t.PriorGraphState == nil || t.guard == nil {
		return nil, fmt.Errorf("TALA geometry transaction clone requires an initialized transaction")
	}
	if t.PriorGraphState.captureTopology {
		return nil, fmt.Errorf("TALA topology transaction requires an independent clone")
	}
	if err := t.guard.Finish(); err != nil {
		return nil, err
	}
	// The source and clone now have equal read-only ownership of this rollback
	// point. Force either side's next UpdateState to allocate its own scratch
	// state instead of recycling and mutating the shared snapshot.
	t.priorGraphStateShared = true
	return &Transaction{
		Ops:                   cloneSlice(t.Ops),
		Graph:                 t.Graph,
		options:               t.options,
		guard:                 t.guard,
		PriorGraphState:       t.PriorGraphState,
		priorGraphStateShared: true,
	}, nil
}
