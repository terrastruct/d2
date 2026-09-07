package placement

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

const optimizerSpatialIndexMinNodes = 96

// optimizerSpatialEntry retains graph order separately from the X ordering
// used by the interval tree. Candidate queries sort the small result set back
// into graph order before running the legacy directional overlap predicate.
type optimizerSpatialEntry struct {
	graphIndex int
	left       float64
	top        float64
	right      float64
	bottom     float64
}

type optimizerOccupancyEntry struct {
	graphIndex int
	generation uint64
}

// optimizerSpatialIndex is rebuilt once for each node considered by the sized
// optimizer. Candidate evaluation may move that node repeatedly, but all other
// boxes remain fixed; queries skip the moving node by identity. This replaces
// the candidate-by-graph scans without requiring updates for every trial point.
type optimizerSpatialIndex struct {
	graph           *layoutgraph.Graph
	nodeCount       int
	occupancyUsable bool
	spatialUsable   bool
	entries         []optimizerSpatialEntry
	maxRight        []float64
	candidates      []int

	occupied            map[geo.Point]optimizerOccupancyEntry
	occupancyGeneration uint64
}

func (optim *sizedOptimizer) rebuildSpatialIndex(guard *limits.OptimizationWorkGuard) error {
	if optim == nil || optim.g == nil {
		return fmt.Errorf("TALA %s spatial index requires an optimizer graph", guard.Location())
	}
	return optim.spatialIndex.rebuild(optim.g, guard)
}

func (optim *sizedOptimizer) indexedIsOccupied(point *geo.Point, guard *limits.OptimizationWorkGuard) (*layoutgraph.Node, bool, error) {
	return optim.spatialIndex.isOccupied(optim.g, point, guard)
}

func (optim *sizedOptimizer) indexedDoesOverlap(node *layoutgraph.Node, point *geo.Point, exceptions []*layoutgraph.Node, guard *limits.OptimizationWorkGuard) (bool, error) {
	return optim.spatialIndex.doesOverlap(node, point, exceptions, guard)
}

func (optim *sizedOptimizer) nodeMayMoveDescendants(node *layoutgraph.Node) bool {
	return node != nil && (node.IsContainer() || node.IsClusterVessel() || optim.g.Sequences[node] != nil)
}

func (optim *sizedOptimizer) indexedCanMove(node *layoutgraph.Node, point *geo.Point, guard *limits.OptimizationWorkGuard) (bool, error) {
	if node == nil || node.Graph == nil || point == nil {
		return false, fmt.Errorf("TALA %s movement check requires a node, graph, and point", guard.Location())
	}
	if nonNilEquals(node.TopLeft, point) {
		return true, nil
	}
	// Candidate scoring moves descendants together with a container, cluster, or
	// sequence vessel. Their indexed boxes would become stale after the first
	// trial point, so retain the legacy scan for those comparatively rare nodes.
	if optim.nodeMayMoveDescendants(node) {
		return optimizerCanMove(node, point, true, guard)
	}
	_, occupied, err := optim.indexedIsOccupied(point, guard)
	if err != nil || occupied {
		return !occupied && err == nil, err
	}
	overlaps, err := optim.indexedDoesOverlap(node, point, nil, guard)
	return !overlaps && err == nil, err
}

func optimizerIndexFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func optimizerIndexFiniteBox(node *layoutgraph.Node) bool {
	if node == nil || node.TopLeft == nil || node.Width < 0 || node.Height < 0 {
		return false
	}
	return optimizerIndexFinite(node.TopLeft.X) && optimizerIndexFinite(node.TopLeft.Y) &&
		optimizerIndexFinite(node.Width) && optimizerIndexFinite(node.Height) &&
		optimizerIndexFinite(node.TopLeft.X+node.Width) && optimizerIndexFinite(node.TopLeft.Y+node.Height)
}

func (index *optimizerSpatialIndex) rebuild(g *layoutgraph.Graph, guard *limits.OptimizationWorkGuard) error {
	if g == nil {
		return fmt.Errorf("TALA %s spatial index requires a graph", guard.Location())
	}
	if len(g.Nodes) > limits.MaxEngineNodes {
		return fmt.Errorf("TALA %s spatial index node count exceeds limit %d", guard.Location(), limits.MaxEngineNodes)
	}

	index.graph = g
	index.nodeCount = len(g.Nodes)
	index.occupancyUsable = true
	index.spatialUsable = len(g.Nodes) >= optimizerSpatialIndexMinNodes
	index.entries = index.entries[:0]
	index.candidates = index.candidates[:0]

	index.occupancyGeneration++
	// Avoid clearing map buckets on every optimizer node while bounding stale
	// coordinate retention when accepted moves continually introduce new keys.
	maxRetained := max(64, 4*len(g.Nodes))
	if index.occupancyGeneration == 0 || len(index.occupied) > maxRetained {
		clear(index.occupied)
		index.occupancyGeneration = 1
	}
	if index.occupied == nil {
		index.occupied = make(map[geo.Point]optimizerOccupancyEntry, len(g.Nodes))
	}

	for graphIndex, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("TALA %s found a nil graph node", guard.Location())
		}
		if node.TopLeft == nil {
			continue
		}
		if !optimizerIndexFinite(node.TopLeft.X) || !optimizerIndexFinite(node.TopLeft.Y) {
			index.occupancyUsable = false
			index.spatialUsable = false
			continue
		}
		point := *node.TopLeft
		entry, found := index.occupied[point]
		if !found || entry.generation != index.occupancyGeneration {
			index.occupied[point] = optimizerOccupancyEntry{
				graphIndex: graphIndex,
				generation: index.occupancyGeneration,
			}
		}

		if !index.spatialUsable {
			continue
		}
		if !optimizerIndexFiniteBox(node) {
			// The legacy predicates have intentionally unusual behavior for NaN,
			// infinities, and negative dimensions. Retain that behavior by using
			// their full scan rather than forcing those values into an ordering.
			index.spatialUsable = false
			continue
		}
		index.entries = append(index.entries, optimizerSpatialEntry{
			graphIndex: graphIndex,
			left:       node.TopLeft.X,
			top:        node.TopLeft.Y,
			right:      node.TopLeft.X + node.Width,
			bottom:     node.TopLeft.Y + node.Height,
		})
	}

	if !index.spatialUsable || len(index.entries) == 0 {
		return guard.Finish()
	}
	if err := guard.AddSort(len(index.entries)); err != nil {
		return err
	}
	slices.SortStableFunc(index.entries, func(a, b optimizerSpatialEntry) int {
		if a.left != b.left {
			switch {
			case a.left < b.left:
				return -1
			case b.left < a.left:
				return 1
			default:
				return 0
			}
		}
		return cmp.Compare(a.graphIndex, b.graphIndex)
	})

	requiredTreeSize := 4 * len(index.entries)
	if cap(index.maxRight) < requiredTreeSize {
		index.maxRight = make([]float64, requiredTreeSize)
	} else {
		index.maxRight = index.maxRight[:requiredTreeSize]
	}
	index.buildMaxRight(1, 0, len(index.entries))
	return guard.Finish()
}

func (index *optimizerSpatialIndex) buildMaxRight(treeIndex, low, high int) float64 {
	if high-low == 1 {
		index.maxRight[treeIndex] = index.entries[low].right
		return index.maxRight[treeIndex]
	}
	middle := low + (high-low)/2
	leftMax := index.buildMaxRight(treeIndex*2, low, middle)
	rightMax := index.buildMaxRight(treeIndex*2+1, middle, high)
	index.maxRight[treeIndex] = max(leftMax, rightMax)
	return index.maxRight[treeIndex]
}

func (index *optimizerSpatialIndex) query(
	left, top, right, bottom float64,
	guard *limits.OptimizationWorkGuard,
) ([]int, error) {
	index.candidates = index.candidates[:0]
	if !index.spatialUsable || len(index.entries) == 0 {
		return index.candidates, guard.Finish()
	}
	if err := index.queryTree(1, 0, len(index.entries), left, top, right, bottom, guard); err != nil {
		return nil, err
	}
	if err := guard.AddSort(len(index.candidates)); err != nil {
		return nil, err
	}
	sort.Ints(index.candidates)
	return index.candidates, guard.Finish()
}

func (index *optimizerSpatialIndex) queryTree(
	treeIndex, low, high int,
	left, top, right, bottom float64,
	guard *limits.OptimizationWorkGuard,
) error {
	if err := guard.Step(); err != nil {
		return err
	}
	if low >= high || index.maxRight[treeIndex] < left || index.entries[low].left > right {
		return nil
	}
	if high-low == 1 {
		entry := index.entries[low]
		if entry.left <= right && entry.right >= left && entry.top <= bottom && entry.bottom >= top {
			index.candidates = append(index.candidates, entry.graphIndex)
		}
		return nil
	}

	middle := low + (high-low)/2
	if err := index.queryTree(treeIndex*2, low, middle, left, top, right, bottom, guard); err != nil {
		return err
	}
	return index.queryTree(treeIndex*2+1, middle, high, left, top, right, bottom, guard)
}

func (index *optimizerSpatialIndex) isOccupied(
	g *layoutgraph.Graph,
	point *geo.Point,
	guard *limits.OptimizationWorkGuard,
) (*layoutgraph.Node, bool, error) {
	if index.graph != g || index.nodeCount != len(g.Nodes) || !index.occupancyUsable ||
		!optimizerIndexFinite(point.X) || !optimizerIndexFinite(point.Y) {
		return optimizerIsOccupied(g, point, guard)
	}
	if err := guard.Step(); err != nil {
		return nil, false, err
	}
	entry, found := index.occupied[*point]
	if !found || entry.generation != index.occupancyGeneration {
		return nil, false, guard.Finish()
	}
	if entry.graphIndex < 0 || entry.graphIndex >= len(g.Nodes) {
		return nil, false, fmt.Errorf("TALA %s spatial occupancy index is stale", guard.Location())
	}
	return g.Nodes[entry.graphIndex], true, guard.Finish()
}

func (index *optimizerSpatialIndex) doesOverlap(
	node *layoutgraph.Node,
	point *geo.Point,
	exceptions []*layoutgraph.Node,
	guard *limits.OptimizationWorkGuard,
) (bool, error) {
	if node == nil || node.Graph == nil || point == nil {
		return false, fmt.Errorf("TALA %s overlap check requires a node, graph, and point", guard.Location())
	}
	if len(node.Graph.Nodes) > limits.MaxEngineNodes || len(exceptions) > limits.MaxEngineNodes {
		return false, fmt.Errorf("TALA %s overlap inputs exceed node limit %d", guard.Location(), limits.MaxEngineNodes)
	}
	if index.graph != node.Graph || index.nodeCount != len(node.Graph.Nodes) || !index.spatialUsable ||
		!optimizerIndexFinite(point.X) || !optimizerIndexFinite(point.Y) ||
		!optimizerIndexFinite(node.Width) || !optimizerIndexFinite(node.Height) ||
		node.Width < 0 || node.Height < 0 ||
		!optimizerIndexFinite(point.X+node.Width) || !optimizerIndexFinite(point.Y+node.Height) {
		return optimizerDoesOverlap(node, point, exceptions, guard)
	}

	const maxSafeDelta = 500.0
	candidates, err := index.query(
		point.X-maxSafeDelta,
		point.Y-maxSafeDelta,
		point.X+node.Width+maxSafeDelta,
		point.Y+node.Height+maxSafeDelta,
		guard,
	)
	if err != nil {
		return false, err
	}
	right := point.X + node.Width
	bottom := point.Y + node.Height
	for _, graphIndex := range candidates {
		if err := guard.Step(); err != nil {
			return false, err
		}
		otherNode := node.Graph.Nodes[graphIndex]
		if otherNode == node {
			continue
		}
		excluded := false
		for _, exception := range exceptions {
			if err := guard.Step(); err != nil {
				return false, err
			}
			if exception == otherNode {
				excluded = true
				break
			}
		}
		if excluded || otherNode.TopLeft == nil {
			continue
		}
		// The interval query is deliberately only a broad phase. Retain the
		// historical directional spacing calculation and strict comparisons.
		if err := guard.Add(uint64(len(node.Edges))); err != nil {
			return false, err
		}
		delta := float64(node.DeltaTo(otherNode, point))
		if point.X < otherNode.TopLeft.X+otherNode.Width+delta && right+delta > otherNode.TopLeft.X &&
			point.Y < otherNode.TopLeft.Y+otherNode.Height+delta && bottom+delta > otherNode.TopLeft.Y {
			return true, nil
		}
	}
	return false, guard.Finish()
}
