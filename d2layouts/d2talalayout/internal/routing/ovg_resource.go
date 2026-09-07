package routing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const (
	// OVG construction derives a visibility graph whose size can be much larger
	// than the input graph. These limits bound the memory and CPU cost of one
	// routing attempt independently of the adapter's input limits.
	maxOVGIntersectionCandidates uint64 = 1_000_000
	maxOVGNodes                  uint64 = 200_000
	maxOVGEdges                  uint64 = 500_000
	// maxOVGWorkUnits bounds actual OVG construction and query work, including
	// index construction. Comparisons eliminated by an index or a hoisted
	// lookup do not consume CPU and must not be charged. The independent
	// candidate, node, and edge limits above still bound derived graph growth.
	maxOVGWorkUnits uint64 = 250_000_000
)

var errOVGResourceLimit = errors.New("TALA OVG resource limit exceeded")

type ovgBuildLimits struct {
	intersectionCandidates uint64
	nodes                  uint64
	edges                  uint64
	work                   uint64
}

func defaultOVGBuildLimits() ovgBuildLimits {
	return ovgBuildLimits{
		intersectionCandidates: maxOVGIntersectionCandidates,
		nodes:                  maxOVGNodes,
		edges:                  maxOVGEdges,
		work:                   maxOVGWorkUnits,
	}
}

type ovgBuildGuard struct {
	ctx  context.Context
	done <-chan struct{}
	// aggregate is shared by all OVGs and route-search flavors in one public
	// routing operation. EdgeRoutingStage also reuses this entire guard across
	// subgraphs, making its resource counters stage-wide rather than per graph.
	aggregate workBudget

	limits     ovgBuildLimits
	candidates uint64
	nodes      uint64
	edges      uint64
	work       uint64
}

func newOVGBuildGuard(ctx context.Context, limits ovgBuildLimits) (*ovgBuildGuard, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	guard := &ovgBuildGuard{ctx: ctx, done: ctx.Done(), aggregate: routeAggregateWorkFromContext(ctx), limits: limits}
	if err := guard.check(); err != nil {
		return nil, err
	}
	return guard, nil
}

func (guard *ovgBuildGuard) check() error {
	if err := cachedContextErr(guard.ctx, guard.done); err != nil {
		return fmt.Errorf("EdgeRouting: %w", err)
	}
	return nil
}

func ovgResourceOverflow(resource string) error {
	return fmt.Errorf("%w: %s arithmetic overflow", errOVGResourceLimit, resource)
}

func ovgResourceExceeded(resource string, requested, limit uint64) error {
	return fmt.Errorf("%w: %s %d exceeds limit %d", errOVGResourceLimit, resource, requested, limit)
}

func (guard *ovgBuildGuard) reserve(resource string, used *uint64, amount, limit uint64) error {
	if err := guard.check(); err != nil {
		return err
	}
	next, ok := limits.CheckedAddUint64(*used, amount)
	if !ok {
		return ovgResourceOverflow(resource)
	}
	if next > limit {
		return ovgResourceExceeded(resource, next, limit)
	}
	*used = next
	return nil
}

func (guard *ovgBuildGuard) reserveCandidates(amount uint64) error {
	return guard.reserve("intersection candidate count", &guard.candidates, amount, guard.limits.intersectionCandidates)
}

func (guard *ovgBuildGuard) reserveNodes(amount uint64) error {
	return guard.reserve("node count", &guard.nodes, amount, guard.limits.nodes)
}

func (guard *ovgBuildGuard) reserveEdges(amount uint64) error {
	return guard.reserve("edge count", &guard.edges, amount, guard.limits.edges)
}

func (guard *ovgBuildGuard) reserveWork(amount uint64) error {
	next, ok := limits.CheckedAddUint64(guard.work, amount)
	if !ok {
		if err := guard.check(); err != nil {
			return err
		}
		return ovgResourceOverflow("work units")
	}
	if next > guard.limits.work {
		if err := guard.check(); err != nil {
			return err
		}
		return ovgResourceExceeded("work units", next, guard.limits.work)
	}
	// Synthetic contexts without Done retain per-operation Err polling. Standard
	// contexts poll at a bounded stride, avoiding a channel select in every inner
	// OVG scan while still checking before any resource-limit result.
	if guard.done == nil || amount == 0 ||
		guard.work/routeStageContextCheckStride != next/routeStageContextCheckStride {
		if err := guard.check(); err != nil {
			return err
		}
	}
	guard.work = next
	if guard.aggregate != nil {
		return guard.aggregate.add(amount)
	}
	return nil
}

func (guard *ovgBuildGuard) step() error {
	return guard.reserveWork(1)
}

func (guard *ovgBuildGuard) reserveSortWork(length int) error {
	if length < 2 {
		return guard.check()
	}
	levels := uint64(0)
	for remaining := uint64(length - 1); remaining > 0; remaining >>= 1 {
		levels++
	}
	work, ok := limits.CheckedMulUint64(uint64(length), levels)
	if !ok {
		return ovgResourceOverflow("sort work units")
	}
	return guard.reserveWork(work)
}

func checkedOVGIntersectionCount(xCount, yCount int) (uint64, error) {
	count, ok := limits.CheckedMulUint64(uint64(xCount), uint64(yCount))
	if !ok {
		return 0, ovgResourceOverflow("intersection candidate count")
	}
	return count, nil
}

func checkedUint64ToInt(value, intLimit uint64) (int, error) {
	if value > intLimit {
		return 0, ovgResourceExceeded("allocation capacity", value, intLimit)
	}
	return int(value), nil
}

func checkedOVGSliceCapacity(lengths ...int) (int, error) {
	total := uint64(0)
	for _, length := range lengths {
		next, ok := limits.CheckedAddUint64(total, uint64(length))
		if !ok {
			return 0, ovgResourceOverflow("allocation capacity")
		}
		total = next
	}
	return checkedUint64ToInt(total, maxIntAsUint64())
}

func maxIntAsUint64() uint64 {
	return uint64(^uint(0) >> 1)
}

// checkedOVGEdgeCapacity computes the exact sweep upper bound used for slice
// preallocation. Every eligible node appears in one horizontal and one vertical
// line, so the sweeps can add at most (n-h)+(n-v) edges. This avoids the old
// dense-grid h*v estimate, which could overflow and badly overallocate sparse
// visibility graphs.
func checkedOVGEdgeCapacity(existing, nodeCount, horizontalLines, verticalLines uint64, edgeLimit, intLimit uint64) (int, error) {
	if existing > edgeLimit {
		return 0, ovgResourceExceeded("edge count", existing, edgeLimit)
	}

	twiceNodes, ok := limits.CheckedMulUint64(nodeCount, 2)
	if !ok {
		return 0, ovgResourceOverflow("edge capacity")
	}
	lineCount, ok := limits.CheckedAddUint64(horizontalLines, verticalLines)
	if !ok || twiceNodes < lineCount {
		return 0, ovgResourceOverflow("edge capacity")
	}
	newEdges := twiceNodes - lineCount
	capacity, ok := limits.CheckedAddUint64(existing, newEdges)
	if !ok {
		return 0, ovgResourceOverflow("edge capacity")
	}
	if capacity > edgeLimit {
		capacity = edgeLimit
	}
	return checkedUint64ToInt(capacity, intLimit)
}

func (guard *ovgBuildGuard) addNode(ovg *OVG, node *OVGNode) (*OVGNode, error) {
	if err := guard.step(); err != nil {
		return nil, err
	}
	if occupant, exists := ovg.OccupiedPoints[*node.Point]; exists {
		return occupant, nil
	}
	if err := guard.reserveNodes(1); err != nil {
		return nil, err
	}
	ovg.AddNodeUnchecked(node)
	return node, nil
}

func (guard *ovgBuildGuard) addPoint(ovg *OVG, point *geo.Point) (*OVGNode, error) {
	if err := guard.step(); err != nil {
		return nil, err
	}
	if occupant, exists := ovg.OccupiedPoints[*point]; exists {
		return occupant, nil
	}
	if err := guard.reserveNodes(1); err != nil {
		return nil, err
	}
	node := NewOVGNode(point)
	ovg.AddNodeUnchecked(node)
	return node, nil
}

// newDerivedNode accounts for a temporary OVG node before allocating it. The
// node may later also consume a slot in one or more OVGs; those memberships are
// deliberately counted separately by addNode.
func (guard *ovgBuildGuard) newDerivedNode(point *geo.Point) (*OVGNode, error) {
	if err := guard.step(); err != nil {
		return nil, err
	}
	if err := guard.reserveNodes(1); err != nil {
		return nil, err
	}
	return NewOVGNode(point), nil
}

func (guard *ovgBuildGuard) newCandidateNode(point *geo.Point) (*OVGNode, error) {
	if err := guard.step(); err != nil {
		return nil, err
	}
	return NewOVGNode(point), nil
}

func (guard *ovgBuildGuard) addNodeUnchecked(ovg *OVG, node *OVGNode) error {
	if err := guard.step(); err != nil {
		return err
	}
	if err := guard.reserveNodes(1); err != nil {
		return err
	}
	ovg.AddNodeUnchecked(node)
	return nil
}

func (guard *ovgBuildGuard) connect(ovg *OVG, nodeA, nodeB *OVGNode) (*OVGEdge, error) {
	if err := guard.step(); err != nil {
		return nil, err
	}
	if nodeA.IsNodeCenter && !nodeB.isPort() || nodeB.IsNodeCenter && !nodeA.isPort() {
		return nil, nil
	}
	if err := guard.reserveEdges(1); err != nil {
		return nil, err
	}
	return ovg.Connect(nodeA, nodeB), nil
}

func (guard *ovgBuildGuard) pointNearGraphNode(graph *layoutgraph.Graph, point *geo.Point) (bool, error) {
	for _, node := range graph.Nodes {
		if err := guard.step(); err != nil {
			return false, err
		}
		// It is valid for visibility nodes to lie on container boundaries.
		if node.IsContainer() {
			continue
		}
		if node.IsPointNear(point) {
			return true, nil
		}
	}
	return false, nil
}

// ovgPointProximityIndex is a build-local broad phase for the Cartesian
// intersection grid. Candidate X coordinates are known in advance, so each
// coordinate keeps graph nodes whose expanded box can contain a candidate.
// Nodes stay in graph order and the exact isPointNear predicate remains the
// final test.
type ovgPointProximityIndex struct {
	byX map[float64][]*layoutgraph.Node
}

func newOVGPointProximityIndex(graph *layoutgraph.Graph, xs []float64, guard *ovgBuildGuard) (*ovgPointProximityIndex, error) {
	index := &ovgPointProximityIndex{
		byX: make(map[float64][]*layoutgraph.Node, len(xs)),
	}
	for _, x := range xs {
		if err := guard.step(); err != nil {
			return nil, err
		}
		for _, node := range graph.Nodes {
			if err := guard.step(); err != nil {
				return nil, err
			}
			if node.IsContainer() {
				continue
			}
			if node.TopLeft.X-layoutgraph.MinRouteNodeClearance <= x &&
				node.TopLeft.X+node.Width+layoutgraph.MinRouteNodeClearance >= x {
				index.byX[x] = append(index.byX[x], node)
			}
		}
	}
	return index, guard.check()
}

func (index *ovgPointProximityIndex) pointNear(x, y float64, guard *ovgBuildGuard) (bool, error) {
	if err := guard.step(); err != nil {
		return false, err
	}
	point := geo.Point{X: x, Y: y}
	for _, node := range index.byX[x] {
		if err := guard.step(); err != nil {
			return false, err
		}
		if node.IsPointNear(&point) {
			return true, nil
		}
	}
	return false, nil
}

func (guard *ovgBuildGuard) tightBoundingBox(nodes layoutgraph.Nodes) (*geo.Point, *geo.Point, error) {
	if len(nodes) == 0 {
		return geo.NewPoint(math.Inf(-1), math.Inf(-1)), geo.NewPoint(math.Inf(1), math.Inf(1)), guard.check()
	}
	if nodes[0].TopLeft == nil {
		return nil, nil, guard.check()
	}

	minX := nodes[0].TopLeft.X
	minY := nodes[0].TopLeft.Y
	maxX := nodes[0].TopLeft.X + nodes[0].Width
	maxY := nodes[0].TopLeft.Y + nodes[0].Height
	for _, node := range nodes {
		if err := guard.step(); err != nil {
			return nil, nil, err
		}
		if node.TopLeft == nil {
			return nil, nil, nil
		}
		minX = math.Min(minX, node.TopLeft.X)
		minY = math.Min(minY, node.TopLeft.Y)
		maxX = math.Max(maxX, node.TopLeft.X+node.Width)
		maxY = math.Max(maxY, node.TopLeft.Y+node.Height)
	}
	return geo.NewPoint(minX, minY), geo.NewPoint(maxX, maxY), guard.check()
}

func (guard *ovgBuildGuard) ovgBoundingBox(ovg *OVG) (*geo.Point, *geo.Point, error) {
	if len(ovg.Nodes) == 0 {
		return geo.NewPoint(math.Inf(-1), math.Inf(-1)), geo.NewPoint(math.Inf(1), math.Inf(1)), guard.check()
	}
	minX := ovg.Nodes[0].X
	minY := ovg.Nodes[0].Y
	maxX := ovg.Nodes[0].X
	maxY := ovg.Nodes[0].Y
	for _, node := range ovg.Nodes {
		if err := guard.step(); err != nil {
			return nil, nil, err
		}
		minX = math.Min(minX, node.X)
		minY = math.Min(minY, node.Y)
		maxX = math.Max(maxX, node.X)
		maxY = math.Max(maxY, node.Y)
	}
	return geo.NewPoint(minX, minY), geo.NewPoint(maxX, maxY), guard.check()
}

func (guard *ovgBuildGuard) portsByOrientation(ovg *OVG, owner *layoutgraph.Node, orientation geo.Orientation) ([]*OVGNode, error) {
	ports := make([]*OVGNode, 0, len(ovg.Ports[owner]))
	seen := make(map[*OVGNode]struct{}, len(ovg.Ports[owner]))
	for _, port := range ovg.Ports[owner] {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		if port.hasPortDirection(owner, orientation) {
			ports = append(ports, port)
		}
	}
	return ports, guard.check()
}

func (guard *ovgBuildGuard) passesThroughAllowingPorts(node *layoutgraph.Node, p1, p2 *geo.Point, direction geo.Orientation, ports []*OVGNode) (bool, error) {
	if len(ports) == 0 {
		return node.PassesThrough(p1, p2), guard.check()
	}
	// A port can exempt a segment only when it points outwards in the supplied
	// direction. Most visibility nodes have no port owner (NONE), so do this
	// invariant direction check before scanning a shape's complete port list.
	allowsPort := false
	switch direction {
	case geo.Top:
		allowsPort = p1.X == p2.X && p1.Y > p2.Y
	case geo.Bottom:
		allowsPort = p1.X == p2.X && p1.Y < p2.Y
	case geo.Left:
		allowsPort = p1.Y == p2.Y && p1.X > p2.X
	case geo.Right:
		allowsPort = p1.Y == p2.Y && p1.X < p2.X
	}
	if allowsPort {
		for _, port := range ports {
			if err := guard.step(); err != nil {
				return false, err
			}
			if nonNilEquals(port.Point, p1) || nonNilEquals(port.Point, p2) {
				return false, nil
			}
		}
	}
	return node.PassesThrough(p1, p2), guard.check()
}

func (guard *ovgBuildGuard) isDescendantOf(descendant, ancestor *layoutgraph.Node) (bool, error) {
	for current := descendant; ; {
		if err := guard.step(); err != nil {
			return false, err
		}
		if ancestor == current {
			return true, nil
		}
		if current == nil {
			return false, nil
		}
		switch {
		case current.Container != nil:
			current = current.Container
		case current.Cluster != nil:
			current = current.Cluster.Vessel
		case current.Sequence != nil:
			current = current.Sequence.Vessel
		default:
			return ancestor == nil, nil
		}
	}
}

func (guard *ovgBuildGuard) hasFixedAncestor(node *layoutgraph.Node) (bool, error) {
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

func (guard *ovgBuildGuard) sameNodes(a, b layoutgraph.Nodes) (bool, error) {
	if len(a) != len(b) {
		return false, guard.check()
	}
	for i := range a {
		if err := guard.step(); err != nil {
			return false, err
		}
		if a[i] != b[i] {
			return false, nil
		}
	}
	return true, guard.check()
}

func (guard *ovgBuildGuard) isRestrictedSequencePort(node *OVGNode) (bool, error) {
	owners := node.portOwners()
	if len(owners) == 0 {
		return false, guard.check()
	}
	for owner, metadata := range owners {
		if err := guard.step(); err != nil {
			return false, err
		}
		if owner.Sequence == nil {
			return false, nil
		}
		if metadata.directions.any(func(direction geo.Orientation) bool {
			switch direction {
			case geo.Left:
				return owner.Sequence.First() == owner
			case geo.Right:
				return owner.Sequence.Last() == owner
			default:
				return true
			}
		}) {
			return false, nil
		}
	}
	return true, guard.check()
}

func (guard *ovgBuildGuard) containerRDFSOrder(graph *layoutgraph.Graph, root *layoutgraph.Node) ([]*layoutgraph.Node, error) {
	var order []*layoutgraph.Node
	if root != nil && !root.IsContainer() {
		return order, guard.check()
	}
	children := graph.Containers[root]
	for _, child := range slices.Backward(children) {
		if err := guard.step(); err != nil {
			return nil, err
		}

		if child.IsContainer() {
			descendants, err := guard.containerRDFSOrder(graph, child)
			if err != nil {
				return nil, err
			}
			order = append(order, descendants...)
			order = append(order, child)
			continue
		}
		if child.IsClusterVessel() {
			cluster := graph.Clusters[child]
			for _, clusterNode := range slices.Backward(cluster.Nodes) {
				if err := guard.step(); err != nil {
					return nil, err
				}

				if !clusterNode.IsContainer() {
					continue
				}
				descendants, err := guard.containerRDFSOrder(graph, clusterNode)
				if err != nil {
					return nil, err
				}
				order = append(order, descendants...)
				order = append(order, clusterNode)
			}
		}
	}
	return order, guard.check()
}

// reserveHierarchyTransform accounts for a complete, non-interruptible
// coordinate transform before it mutates graph or OVG state. Preflighting the
// whole transform lets the existing inverse defers remain infallible.
func (guard *ovgBuildGuard) reserveHierarchyTransform(ovg *OVG, graphNodeCount int) error {
	if err := guard.reserveWork(uint64(graphNodeCount)); err != nil {
		return err
	}
	for _, node := range ovg.Nodes {
		if err := guard.step(); err != nil { // preflight scan
			return err
		}
		if err := guard.reserveWork(2); err != nil { // point transform and reindex
			return err
		}
		for range node.portOwners() {
			if err := guard.step(); err != nil { // preflight scan
				return err
			}
			if err := guard.reserveWork(1); err != nil { // direction transform
				return err
			}
		}
	}
	return guard.check()
}

func (ovg *OVG) fixedOverlapsForBuild(graph *layoutgraph.Graph, nodes layoutgraph.Nodes, guard *ovgBuildGuard) (map[*layoutgraph.Node]struct{}, error) {
	ovg.fixedOverlapsMu.Lock()
	defer ovg.fixedOverlapsMu.Unlock()

	for _, entry := range ovg.fixedOverlapsCache {
		if err := guard.step(); err != nil {
			return nil, err
		}
		same, err := guard.sameNodes(entry.nodes, nodes)
		if err != nil {
			return nil, err
		}
		if entry.graph == graph && same {
			return entry.overlaps, nil
		}
	}

	fixedNodes := make([]*layoutgraph.Node, 0, len(nodes)/4)
	for _, node := range nodes {
		if err := guard.step(); err != nil {
			return nil, err
		}
		fixed, err := guard.hasFixedAncestor(node)
		if err != nil {
			return nil, err
		}
		if fixed {
			fixedNodes = append(fixedNodes, node)
		}
	}

	overlaps := make(map[*layoutgraph.Node]struct{})
	for _, node := range fixedNodes {
		if _, found := overlaps[node]; found {
			continue
		}
		for _, other := range nodes {
			if err := guard.step(); err != nil {
				return nil, err
			}
			nodeBelowOther, err := guard.isDescendantOf(node, other)
			if err != nil {
				return nil, err
			}
			otherBelowNode, err := guard.isDescendantOf(other, node)
			if err != nil {
				return nil, err
			}
			if node == other || nodeBelowOther || otherBelowNode {
				continue
			}
			if node.Overlaps(other.Box) {
				overlaps[node] = struct{}{}
				fixed, err := guard.hasFixedAncestor(other)
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

	ovg.fixedOverlapsCache = append(ovg.fixedOverlapsCache, fixedOverlapsCacheEntry{
		graph:    graph,
		nodes:    append(layoutgraph.Nodes(nil), nodes...),
		overlaps: overlaps,
	})
	return overlaps, nil
}

type ovgPortIndex struct {
	xByY               map[float64][]float64
	yByX               map[float64][]float64
	owners             []*layoutgraph.Node
	ownersByX          map[float64][]int
	ownersByY          map[float64][]int
	verticalBlockers   map[float64][]*layoutgraph.Node
	horizontalBlockers map[float64][]*layoutgraph.Node
}

func newOVGPortIndex(ports map[*layoutgraph.Node][]*OVGNode, graph *layoutgraph.Graph, fixedOverlaps map[*layoutgraph.Node]struct{}, guard *ovgBuildGuard) (*ovgPortIndex, error) {
	index := &ovgPortIndex{
		xByY:               make(map[float64][]float64),
		yByX:               make(map[float64][]float64),
		owners:             make([]*layoutgraph.Node, 0, len(ports)),
		ownersByX:          make(map[float64][]int),
		ownersByY:          make(map[float64][]int),
		verticalBlockers:   make(map[float64][]*layoutgraph.Node),
		horizontalBlockers: make(map[float64][]*layoutgraph.Node),
	}
	for owner, nodePorts := range ports {
		if err := guard.step(); err != nil {
			return nil, err
		}
		ownerIndex := len(index.owners)
		index.owners = append(index.owners, owner)
		for _, port := range nodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			index.xByY[port.Y] = append(index.xByY[port.Y], port.X)
			index.yByX[port.X] = append(index.yByX[port.X], port.Y)
			index.ownersByX[port.X] = append(index.ownersByX[port.X], ownerIndex)
			index.ownersByY[port.Y] = append(index.ownersByY[port.Y], ownerIndex)
		}
	}
	for _, values := range index.xByY {
		if err := guard.reserveSortWork(len(values)); err != nil {
			return nil, err
		}
		sort.Float64s(values)
	}
	for _, values := range index.yByX {
		if err := guard.reserveSortWork(len(values)); err != nil {
			return nil, err
		}
		sort.Float64s(values)
	}

	// A port-to-candidate segment is axis aligned. Precompute the graph-order
	// blockers that can intersect each port axis, then retain passesThrough as
	// the exact segment predicate at query time.
	if graph != nil {
		for x := range index.ownersByX {
			if err := guard.step(); err != nil {
				return nil, err
			}
			for _, node := range graph.Nodes {
				if err := guard.step(); err != nil {
					return nil, err
				}
				if node == nil || node.TopLeft == nil || node.IsContainer() {
					continue
				}
				if _, ignored := fixedOverlaps[node]; ignored {
					continue
				}
				left := math.Min(node.TopLeft.X, node.TopLeft.X+node.Width)
				right := math.Max(node.TopLeft.X, node.TopLeft.X+node.Width)
				if left <= x && x <= right {
					index.verticalBlockers[x] = append(index.verticalBlockers[x], node)
				}
			}
		}
		for y := range index.ownersByY {
			if err := guard.step(); err != nil {
				return nil, err
			}
			for _, node := range graph.Nodes {
				if err := guard.step(); err != nil {
					return nil, err
				}
				if node == nil || node.TopLeft == nil || node.IsContainer() {
					continue
				}
				if _, ignored := fixedOverlaps[node]; ignored {
					continue
				}
				top := math.Min(node.TopLeft.Y, node.TopLeft.Y+node.Height)
				bottom := math.Max(node.TopLeft.Y, node.TopLeft.Y+node.Height)
				if top <= y && y <= bottom {
					index.horizontalBlockers[y] = append(index.horizontalBlockers[y], node)
				}
			}
		}
	}
	return index, nil
}

// Owner indexes are appended in ascending order during index construction.
// Merge the two axis lists so a query visits only aligned owners, once each,
// in the same order as a full owners scan. Duplicate ports and owners aligned
// on both axes do not count as distinct unobstructed graph nodes.
func (index *ovgPortIndex) alignedOwners(x, y float64) ovgAlignedOwners {
	return ovgAlignedOwners{byX: index.ownersByX[x], byY: index.ownersByY[y]}
}

type ovgAlignedOwners struct {
	byX, byY []int
}

func (owners *ovgAlignedOwners) next(guard *ovgBuildGuard) (int, bool, error) {
	if err := guard.step(); err != nil {
		return 0, false, err
	}
	if len(owners.byX) == 0 && len(owners.byY) == 0 {
		return 0, false, nil
	}
	var owner int
	if len(owners.byY) == 0 || len(owners.byX) > 0 && owners.byX[0] < owners.byY[0] {
		owner = owners.byX[0]
	} else {
		owner = owners.byY[0]
	}
	for len(owners.byX) > 0 && owners.byX[0] == owner {
		if err := guard.step(); err != nil {
			return 0, false, err
		}
		owners.byX = owners.byX[1:]
	}
	for len(owners.byY) > 0 && owners.byY[0] == owner {
		if err := guard.step(); err != nil {
			return 0, false, err
		}
		owners.byY = owners.byY[1:]
	}
	return owner, true, nil
}

func (guard *ovgBuildGuard) hasCoordinateWithin(sortedValues []float64, coordinate, distance float64) (bool, error) {
	low, high := 0, len(sortedValues)
	threshold := coordinate - distance
	for low < high {
		if err := guard.step(); err != nil {
			return false, err
		}
		mid := int(uint(low+high) >> 1)
		if sortedValues[mid] < threshold {
			low = mid + 1
		} else {
			high = mid
		}
	}
	return low < len(sortedValues) && sortedValues[low] <= coordinate+distance, guard.check()
}

func (index *ovgPortIndex) tooClose(x, y, distance float64, guard *ovgBuildGuard) (bool, error) {
	tooClose, err := guard.hasCoordinateWithin(index.xByY[y], x, distance)
	if err != nil || tooClose {
		return tooClose, err
	}
	return guard.hasCoordinateWithin(index.yByX[x], y, distance)
}
