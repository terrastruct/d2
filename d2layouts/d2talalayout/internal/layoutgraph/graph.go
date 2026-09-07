package layoutgraph

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/typedpool"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

const (
	minCellSize           = 10.0
	turnPenaltyMultiplier = 0.125
	// Keep the exact value because center-port route scoring is output-sensitive.
	centerPortMultiplier = 0.04287499999999999
)

type Graph struct {
	// Unfortunate, but because we represent root as nil, we need to indicate root hierarchy as a field in graph
	IsRootHierarchy bool
	Nodes           []*Node
	Edges           []*Edge
	CellSize        float64

	// ovg
	crossingCost      float64
	turnCost          float64
	nonCenterPortCost float64
	costMu            sync.RWMutex

	// These are special entities within a graph:

	// -- Containers are nodes which have nodes enclosed inside them
	// ---- They can be nested
	// ---- The top level is considered a container of nil
	// ---- These are inferred
	Containers map[*Node][]*Node

	// -- Clusters are nodes which should stick together and can be reoriented and resized
	// ---- They can be oriented vertically or horizontally depending on where its connections are
	// ---- They can be resized to be all the same dimensions
	// ---- These are inferred
	Clusters map[*Node]*Cluster

	// -- Trees are nodes which are in a tree structure (i.e. any non-cyclic subgraph)
	// ---- They are oriented in one of 4 directions, depending on where its root node ends up
	// ---- These are inferred
	// ---- Trees are pruned from the graph, then after node placement, they are reattached and placed as trees
	Trees      map[*Node][]*Tree
	NodeToTree map[*Node]*Tree

	Hubs map[*Node][]*Node

	Sequences map[*Node]*Sequence

	//  -- Directions is used to lookup the direction the edges should point towards within each container
	// e.g. Bottom means the edges should primarily go from Top to Bottom
	Directions          map[*Node]geo.Orientation
	CommonUncleSiblings map[*Node]Nodes

	edgeLengthCache map[uint64]float64
}

type descendantWalkEntry struct {
	node *Node
	emit bool
}

type descendantWalkScratch struct {
	seen        map[*Node]struct{}
	stack       []descendantWalkEntry
	descendants []*Node
}

var descendantWalkPool = typedpool.New(func() *descendantWalkScratch {
	return &descendantWalkScratch{seen: make(map[*Node]struct{})}
})

func NewGraph() *Graph {
	return &Graph{
		Nodes: make([]*Node, 0),
		Edges: make([]*Edge, 0),

		Containers:      make(map[*Node][]*Node),
		Clusters:        make(map[*Node]*Cluster),
		Trees:           make(map[*Node][]*Tree),
		Hubs:            make(map[*Node][]*Node),
		Sequences:       make(map[*Node]*Sequence),
		Directions:      make(map[*Node]geo.Orientation),
		edgeLengthCache: make(map[uint64]float64),
	}
}

// Direction reports the layout direction assigned to container.
func (g *Graph) Direction(container *Node) geo.Orientation {
	if direction, has := g.Directions[container]; has {
		return direction
	}
	return geo.NONE
}

func (g *Graph) isBadStateContext(node *Node, graphState *GraphState, ignoreContainerEscape bool, guard workStepper) (bool, error) {
	bad, skipOverlapCheck, err := g.isStructurallyBadStateContext(node, graphState, ignoreContainerEscape, guard)
	if err != nil || bad {
		return bad, err
	}
	if skipOverlapCheck {
		return false, guard.Finish()
	}
	return g.hasBadOverlapStateContext(node, graphState, guard)
}

// isStructurallyBadStateContext checks the linear-time node invariants that do
// not depend on any other non-descendant graph node. Keeping these separate
// lets transactions validate every node's containment and fixed-position
// invariants while restricting pairwise overlap checks to relevant candidates.
func (g *Graph) isStructurallyBadStateContext(node *Node, graphState *GraphState, ignoreContainerEscape bool, guard workStepper) (bad, skipOverlapCheck bool, err error) {
	return g.isStructurallyBadStateWithFixedOriginContext(node, graphState, ignoreContainerEscape, nil, false, guard)
}

func (g *Graph) isStructurallyBadStateWithFixedOriginContext(node *Node, graphState *GraphState, ignoreContainerEscape bool, fixedOrigin *geo.Point, fixedOriginCached bool, guard workStepper) (bad, skipOverlapCheck bool, err error) {
	if node == nil || node.TopLeft == nil {
		return true, false, invariant.New("transaction bad-state check received an incomplete node")
	}
	if err := guard.Step(); err != nil {
		return false, false, err
	}
	// No need to check if cluster node is in bad state, the vessel covers it
	// Right now only happens when checking if containers bad state, since cluster nodes are still containers
	if node.Cluster != nil {
		return false, true, nil
	}
	if !ignoreContainerEscape {
		if c := node.container(); c != nil &&
			c.TopLeft != nil && node.TopLeft != nil &&
			!c.Surrounds(node, 0) {
			return true, false, nil
		}
		if node.isContainer {
			for _, child := range g.Containers[node] {
				if err := guard.Step(); err != nil {
					return false, false, err
				}
				if child == nil || child.TopLeft == nil {
					return false, false, invariant.New("transaction container has an incomplete child")
				}
				if !node.Surrounds(child, 0) {
					return true, false, nil
				}
			}
		}
	}

	// Nothing should be past the fixed origin. Transactions cache the first
	// fixed origin for each container once per commit; standalone callers retain
	// the legacy lookup path.
	pastFixedOrigin := false
	if fixedOriginCached {
		pastFixedOrigin = fixedOrigin != nil &&
			(node.TopLeft.X < fixedOrigin.X || node.TopLeft.Y < fixedOrigin.Y)
	} else {
		pastFixedOrigin = node.isPointPastFixedOrigin(node.TopLeft.X, node.TopLeft.Y, true)
	}
	if pastFixedOrigin {
		return true, false, nil
	}

	if node.FixedTopLeft != nil && graphState != nil {
		originalPoint := geo.Point{}
		if original, exists := graphState.nodeGeometry[node]; exists && original.topLeft.pointer != nil {
			originalPoint = original.topLeft.value
		}
		if node.TopLeft.X != originalPoint.X || node.TopLeft.Y != originalPoint.Y {
			return true, false, nil
		}
	}
	return false, false, nil
}

func (g *Graph) hasBadOverlapStateContext(node *Node, graphState *GraphState, guard workStepper) (bool, error) {
	if node == nil || node.TopLeft == nil {
		return true, invariant.New("transaction overlap check received an incomplete node")
	}
	if node.Cluster != nil {
		return false, nil
	}
	var pairwiseExceptions map[*Node]map[*Node]struct{}
	if graphState != nil {
		pairwiseExceptions = graphState.existingOverlaps
	}

	var exceptions []*Node
	if node.isContainer || node.isClusterVessel || g.Sequences[node] != nil {
		var err error
		exceptions, err = g.allDescendantNodesGuarded(node, true, guard)
		if err != nil {
			return false, err
		}
	}
	// doesOverlapWithDimensionsContext already skips node identity, so only
	// descendants and ancestors need to be materialized as exceptions.
	if node.Container != nil || node.Cluster != nil || node.Sequence != nil {
		ancestors, err := g.ancestorsOfGuarded(node, guard)
		if err != nil {
			return false, err
		}
		exceptions = append(exceptions, ancestors...)
	}
	return g.doesOverlapWithDimensionsContext(node, node.TopLeft, node.Width, node.Height, exceptions, pairwiseExceptions, guard)
}

// mirrorAxes performs a reflection of the graph (both horizontal and vertical according to x and y)
// When the graph has containers, the contents of those containers are recursively reflected iff it is reachable from other nodes
// If it is reachable, reflecting maintains edge length. Otherwise, reflecting could reverse edge directions undesirably

// For example, we want to mirror the two outer nodes, but do not want to mirror the contents of the container
//
// . ┌─────────┐
// . │         │
// . │   ┌──┐  │
// . │   │  │  │
// . │   └─┬┘  │
// . │     │   │
// . │   ┌─▼┐  │
// . │   └──┘  │
// . │         │
// . └────▲────┘
// .      │
// .      │
// .  ┌───┴───┐
// .  │       │
// .  └───────┘
// Currently used by container child over container center
func (g *Graph) AddNodeUnchecked(node *Node) {
	g.Nodes = append(g.Nodes, node)
	node.Graph = g
}

func (g *Graph) AddNodeToContainer(container, node *Node) {
	g.Containers[container] = append(
		g.Containers[container],
		node,
	)
	node.Container = container
	if container != nil {
		container.isContainer = true
	}
}

func (g *Graph) AddNewNodeToContainer(container, node *Node) {
	g.AddNodeUnchecked(node)
	g.AddNodeToContainer(container, node)
}

func (g *Graph) AddNode(node *Node) *Node {
	if occupant, is := g.isOccupied(node.TopLeft); !is {
		g.AddNodeUnchecked(node)
		return node
	} else {
		return occupant
	}
}

func (g *Graph) RemoveNode(node *Node) {
	newNodes := make([]*Node, 0, len(g.Nodes)-1)

	for _, n := range g.Nodes {
		if n != node {
			newNodes = append(newNodes, n)
		}
	}

	g.Nodes = newNodes
}

func (g *Graph) AddEdge(edge *Edge) {
	if slices.Contains(g.Edges, edge) {
		return
	}
	g.Edges = append(g.Edges, edge)
}

func (g *Graph) ComputeCellSize() {
	minHeight := math.Inf(1)
	minWidth := math.Inf(1)
	maxHeight := math.Inf(-1)
	maxWidth := math.Inf(-1)

	for _, node := range g.Nodes {
		minWidth = math.Min(minWidth, node.Width)
		minHeight = math.Min(minHeight, node.Height)
		maxWidth = math.Max(maxWidth, node.Width)
		maxHeight = math.Max(maxHeight, node.Height)
	}

	minLength := math.Min(minWidth, minHeight)
	maxLength := math.Max(maxWidth, maxHeight)

	if maxLength < (3.0 * minLength) {
		g.CellSize = math.Ceil(maxLength)
	} else {
		g.CellSize = math.Ceil((3.0 * minLength) / 2.0)
	}
	g.CellSize = math.Max(g.CellSize, minCellSize)
}

func (g *Graph) Disconnect(edge *Edge) {
	edge.From.removeEdge(edge)
	edge.To.removeEdge(edge)

	newEdges := make([]*Edge, 0, len(g.Edges))
	for _, oldEdge := range g.Edges {
		if oldEdge != edge {
			newEdges = append(newEdges, oldEdge)
		}
	}

	g.Edges = newEdges
}

func (g *Graph) Connect(nodeA, nodeB *Node) *Edge {
	edge := NewEdge(nodeA, nodeB)
	g.Edges = append(g.Edges, edge)
	nodeA.addEdge(edge)
	if nodeA != nodeB {
		nodeB.addEdge(edge)
	}
	return edge
}

func (g *Graph) doesOverlapWithDimensionsContext(node *Node, p *geo.Point, newWidth, newHeight float64, exceptions []*Node, pairwiseExceptions map[*Node]map[*Node]struct{}, guard workStepper) (bool, error) {
	if node == nil || p == nil {
		return true, invariant.New("overlap check received an incomplete node")
	}
	right := p.X + newWidth
	bottom := p.Y + newHeight
	var exceptionSet map[*Node]struct{}
	if len(exceptions) > 0 {
		if len(exceptions) <= 8 || len(exceptions) > maxPooledOverlapExceptions {
			// Small maps fit in the compiler's stack-allocated map group; large
			// maps must not enter the pool after a partially populated failure.
			exceptionSet = make(map[*Node]struct{}, len(exceptions))
		} else {
			scratch := overlapExceptionPool.Get()
			defer releaseOverlapExceptions(scratch)
			if scratch.nodes == nil {
				scratch.nodes = make(map[*Node]struct{}, len(exceptions))
			}
			exceptionSet = scratch.nodes
		}
		for _, exception := range exceptions {
			if err := guard.Step(); err != nil {
				return false, err
			}
			exceptionSet[exception] = struct{}{}
		}
	}
	// Every candidate comparison uses the same moved-node exception set. Resolve
	// it once instead of repeating an outer-map lookup for every graph node.
	pairwiseNodeExceptions := pairwiseExceptions[node]

	for _, otherNode := range g.Nodes {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if otherNode == nil {
			return false, invariant.New("overlap check encountered a nil graph node")
		}
		if otherNode == node {
			continue
		}
		if otherNode.TopLeft != nil {
			// This heuristic is only a trial filter; the transaction constructor uses
			// the proven graph-wide padding above as its construction bound.
			const maxSafeDelta = 500.0
			if p.X > otherNode.TopLeft.X+otherNode.Width+maxSafeDelta ||
				p.X+newWidth+maxSafeDelta < otherNode.TopLeft.X ||
				p.Y > otherNode.TopLeft.Y+otherNode.Height+maxSafeDelta ||
				p.Y+newHeight+maxSafeDelta < otherNode.TopLeft.Y {
				continue
			}
			// Distant boxes cannot overlap regardless of the exception sets. Test
			// geometry first so sparse layouts avoid two map probes per box.
			if _, excepted := pairwiseNodeExceptions[otherNode]; excepted {
				continue
			}
			if _, excepted := exceptionSet[otherNode]; excepted {
				continue
			}
			deltaValue, err := node.deltaToGuarded(otherNode, p, guard)
			if err != nil {
				return false, err
			}
			delta := float64(deltaValue)

			// Standard overlap test
			if (p.X < (otherNode.TopLeft.X + otherNode.Width + delta)) && ((right + delta) > otherNode.TopLeft.X) &&
				(p.Y < (otherNode.TopLeft.Y + otherNode.Height + delta)) && ((bottom + delta) > otherNode.TopLeft.Y) {
				return true, nil
			}
		}
	}
	return false, guard.Finish()
}

// WouldOverlapWithWorkGuard reports whether placing node at point overlaps an
// unexcepted graph node while charging the complete comparison.
func (g *Graph) WouldOverlapWithWorkGuard(
	node *Node,
	point *geo.Point,
	exceptions []*Node,
	pairwiseExceptions map[*Node]map[*Node]struct{},
	guard *limits.WorkGuard,
) (bool, error) {
	if guard == nil {
		return false, fmt.Errorf("TALA overlap check requires a work guard")
	}
	if node == nil {
		return g.doesOverlapWithDimensionsContext(
			node,
			point,
			0,
			0,
			exceptions,
			pairwiseExceptions,
			guard,
		)
	}
	return g.doesOverlapWithDimensionsContext(
		node,
		point,
		node.Width,
		node.Height,
		exceptions,
		pairwiseExceptions,
		guard,
	)
}

func (g *Graph) isOccupied(p *geo.Point) (*Node, bool) {
	for _, n := range g.Nodes {
		if (n.TopLeft != nil) && p != nil && nonNilEquals(n.TopLeft, p) {
			return n, true
		}
	}
	return nil, false
}

func (node *Node) isPointPastFixedOrigin(x, y float64, includeSizes bool) bool {
	fixedOrigin := node.Graph.containerFixedOrigin(node.container())
	if fixedOrigin == nil {
		return false
	}
	if !includeSizes {
		fixedOrigin.X = math.Round(fixedOrigin.X / node.Graph.CellSize)
		fixedOrigin.Y = math.Round(fixedOrigin.Y / node.Graph.CellSize)
	}
	return x < fixedOrigin.X || y < fixedOrigin.Y
}

// if this graph is a subgraph, return the container of the least nested node
// normally this is the nil container
func (nodes Nodes) subgraphContainer() *Node {
	var root *Node
	minLevel := math.MaxInt
	for _, n := range nodes {
		c := n.container()
		if c == nil {
			return nil
		}
		level := c.containerLevel()
		if level < minLevel {
			minLevel = level
			root = c
		}
	}
	return root
}

func (g *Graph) bounds() (*geo.Point, *geo.Point) {
	tl, br := g.boundingBox(true)
	if tl == nil || br == nil {
		return nil, nil
	}
	// Placement consumes a pixel-aligned graph box even when edge points are
	// fractional.
	return geo.NewPoint(math.Round(tl.X), math.Round(tl.Y)), geo.NewPoint(math.Round(br.X), math.Round(br.Y))
}

func (g *Graph) unroundedBounds() (*geo.Point, *geo.Point) {
	return g.boundingBox(false)
}

func (g *Graph) boundingBox(roundNodeDimensions bool) (*geo.Point, *geo.Point) {
	var tl, br *geo.Point
	if roundNodeDimensions {
		tl, br = Nodes(g.Nodes).fixedBounds()
	} else {
		tl, br = Nodes(g.Nodes).unroundedFixedBounds()
	}
	if tl == nil || br == nil {
		return nil, nil
	}

	minX := tl.X
	minY := tl.Y

	maxX := br.X
	maxY := br.Y

	for _, edge := range g.Edges {
		edgeTL, edgeBR := edge.boundingBoxValues()
		if !math.IsInf(edgeTL.X, 0) {
			minX = math.Min(minX, edgeTL.X)
			minY = math.Min(minY, edgeTL.Y)
			maxX = math.Max(maxX, edgeBR.X)
			maxY = math.Max(maxY, edgeBR.Y)
		}

	}

	return geo.NewPoint(minX, minY), geo.NewPoint(maxX, maxY)
}

func (g *Graph) turnCostValue() float64 {
	g.costMu.RLock()
	if g.turnCost != 0.0 {
		cost := g.turnCost
		g.costMu.RUnlock()
		return cost
	}
	g.costMu.RUnlock()

	g.costMu.Lock()
	defer g.costMu.Unlock()
	if g.turnCost != 0.0 {
		return g.turnCost
	}
	cost := turnPenaltyMultiplier * float64(len(g.Edges)) * g.maxEdgeLength()
	g.turnCost = cost

	return cost
}

func (g *Graph) nonCenterPortCostValue() float64 {
	g.costMu.RLock()
	if g.nonCenterPortCost != 0.0 {
		cost := g.nonCenterPortCost
		g.costMu.RUnlock()
		return cost
	}
	g.costMu.RUnlock()

	g.costMu.Lock()
	defer g.costMu.Unlock()
	if g.nonCenterPortCost != 0.0 {
		return g.nonCenterPortCost
	}
	if len(g.Edges) == 0 {
		return 0
	}
	cost := centerPortMultiplier * float64(len(g.Edges)) * g.maxEdgeLength()

	// For certain diagrams, the `cost` above can be smaller than the distance between ports
	// and then edge routing doesn't prefer center ports as expected
	// ┌──────────────────────────────────────────────────────┐
	// │                                   port               │
	// |                                distance              |
	// │                                                      │
	// │                           │──────────────────│       │
	// └────────┬──────────────────┬──────────────────┬───────┘
	//          │                  │                  │
	//          │                  │                  │
	// the routine below estimates another cost based on the min node size
	minSize := math.Inf(1)
	for _, n := range g.Nodes {
		if n.isContainer {
			continue
		}
		minSize = math.Min(minSize, n.Height)
		minSize = math.Min(minSize, n.Width)
	}

	if !math.IsInf(minSize, 1) {
		cost = math.Max(cost, minSize/3.)
	}
	g.nonCenterPortCost = cost

	return cost
}

func (g *Graph) crossingCostValue() float64 {
	g.costMu.RLock()
	if g.crossingCost != 0.0 {
		cost := g.crossingCost
		g.costMu.RUnlock()
		return cost
	}
	g.costMu.RUnlock()

	g.costMu.Lock()
	defer g.costMu.Unlock()
	if g.crossingCost != 0.0 {
		return g.crossingCost
	}
	// c * (m * l_max)

	cost := CrossingCostWeight * float64(len(g.Edges)) * g.maxEdgeLength()
	g.crossingCost = cost

	return cost
}

func (g *Graph) resetTurnCost() {
	g.costMu.Lock()
	g.turnCost = 0
	g.costMu.Unlock()
}

func (g *Graph) maxEdgeLength() float64 {
	hasLength := false
	maxLength := float64(ConnectedNodeGap)
	for _, n := range g.Nodes {
		if n.TopLeft == nil {
			continue
		}
		for _, e := range n.Edges {
			adj := n.adjacent(e)
			if adj.TopLeft == nil {
				continue
			}
			hasLength = true
			distance := n.distanceTo(adj, true)
			maxLength = math.Max(maxLength, distance)
		}
	}
	if !hasLength {
		return 0.0
	}
	return maxLength
}

// SplitOptions controls which relationships connect nodes while splitting a
// graph into subgraphs.
type SplitOptions struct {
	IncludeContainers bool
	IncludeNears      bool
	TraverseTrees     bool
}

// SplitSubgraphs partitions the graph into sets of mutually reachable nodes.
func (g *Graph) SplitSubgraphs(ctx context.Context, options SplitOptions) ([]*Graph, error) {
	return g.splitSubgraphsWithOwnership(ctx, options, nil, nil)
}

type nodeGraphOwnershipJournal map[*Node]*Graph

func (journal nodeGraphOwnershipJournal) restore() {
	for node, graph := range journal {
		node.Graph = graph
	}
}

// splitSubgraphsWithOwnership optionally returns every Node.Graph value changed
// by the split. Reachability includes the bounded Near closure, so a caller can
// restore even Near-only nodes that are not members of the source Graph.Nodes.
func (g *Graph) splitSubgraphsWithOwnership(
	ctx context.Context,
	options SplitOptions,
	ownership *nodeGraphOwnershipJournal,
	sharedGuard workStepper,
) ([]*Graph, error) {
	guard := sharedGuard
	if guard == nil {
		if err := validateEngineGraph(ctx, "SplitSubgraphs", g); err != nil {
			return nil, err
		}
		var err error
		guard, err = limits.NewWorkGuard(ctx, "SplitSubgraphs", maxEngineWorkUnits)
		if err != nil {
			return nil, err
		}
	} else if err := guard.Finish(); err != nil {
		return nil, err
	}
	originalGraphReferences := make(map[*Node]*Graph, len(g.Nodes))
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node != nil {
			originalGraphReferences[node] = node.Graph
		}
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		for node, graph := range originalGraphReferences {
			node.Graph = graph
		}
	}()
	graphs := make([]*Graph, 0)
	if len(g.Nodes) == 0 {
		complete = true
		return graphs, nil
	}

	added := map[*Node]struct{}{}
	nodeToSubgraph := make(map[*Node]*Graph, len(g.Nodes))
	addReachable := func(startingNode *Node, subgraph *Graph) error {
		reachableNodes, err := startingNode.allReachableNodesGuarded(
			options.IncludeContainers,
			options.IncludeNears,
			options.TraverseTrees,
			nil,
			guard,
		)
		if err != nil {
			return err
		}
		for _, node := range reachableNodes {
			if err := guard.Step(); err != nil {
				return err
			}
			if _, was := added[node]; was {
				continue
			}
			if _, captured := originalGraphReferences[node]; !captured {
				if len(originalGraphReferences) >= maxEngineNodes {
					return fmt.Errorf("TALA SplitSubgraphs unique node ownership exceeds limit %d", maxEngineNodes)
				}
				originalGraphReferences[node] = node.Graph
			}
			subgraph.AddNodeUnchecked(node)
			added[node] = struct{}{}
			nodeToSubgraph[node] = subgraph
		}
		return nil
	}

	fixedNodes := make([]*Node, 0)
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node.FixedTopLeft != nil {
			fixedNodes = append(fixedNodes, node)
		}
	}
	if len(fixedNodes) > 0 {
		// create first subgraph with all fixed nodes
		// it will be placed at 0,0 in CombineSubgraphs so fixed locations are accurate
		subgraph := NewGraph()
		subgraph.CopyEntitiesFrom(g)

		for _, startingNode := range fixedNodes {
			if err := addReachable(startingNode, subgraph); err != nil {
				return nil, err
			}
		}

		graphs = append(graphs, subgraph)
	}

	for _, startingNode := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if _, was := added[startingNode]; was {
			continue
		}

		subgraph := NewGraph()
		subgraph.CopyEntitiesFrom(g)
		if err := addReachable(startingNode, subgraph); err != nil {
			return nil, err
		}

		graphs = append(graphs, subgraph)
	}

	// Each original edge is considered once. This preserves the original graph's
	// edge order without the quadratic duplicate scan in AddEdge. In the unusual
	// case where a traversal deliberately omits one endpoint into another
	// subgraph, retain the edge in both subgraphs as the old endpoint scan did.
	seenEdges := make(map[*Graph]map[*Edge]struct{}, len(graphs))
	appendEdge := func(graph *Graph, edge *Edge) {
		graphEdges := seenEdges[graph]
		if graphEdges == nil {
			graphEdges = make(map[*Edge]struct{})
			seenEdges[graph] = graphEdges
		}
		if _, seen := graphEdges[edge]; seen {
			return
		}
		graphEdges[edge] = struct{}{}
		graph.Edges = append(graph.Edges, edge)
	}
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		fromGraph := nodeToSubgraph[edge.From]
		toGraph := nodeToSubgraph[edge.To]
		if fromGraph != nil {
			appendEdge(fromGraph, edge)
		}
		if toGraph != nil && toGraph != fromGraph {
			appendEdge(toGraph, edge)
		}
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}

	if ownership != nil {
		*ownership = originalGraphReferences
	}
	complete = true
	return graphs, nil
}

func (g *Graph) area() float64 {
	if len(g.Nodes) == 0 {
		return 0
	}
	topLeft, bottomRight := g.unroundedBounds()
	return math.Abs(topLeft.X-bottomRight.X) * math.Abs(topLeft.Y-bottomRight.Y)
}

// 2D bin packing with rectangles
// Algorithm:
// 1. keep a list of candidate points, which are 3 non-upper-left corners of every placed subgraph. Initially, just (0,0)
// 2. calculate how much empty space remains after potential placement
// 3. the min empty space candidate point gets the subgraph placed there
// Start from largest to smallest subgraphs
func (g *Graph) containerRDFSOrder(root *Node) []*Node {
	var order []*Node

	if root != nil && !root.isContainer {
		return order
	}

	for _, child := range slices.Backward(g.Containers[root]) {

		if child.isContainer {
			order = append(order, g.containerRDFSOrder(child)...)
			order = append(order, child)
			continue
		}

		if child.isClusterVessel {
			cluster := g.Clusters[child]
			for _, cNode := range slices.Backward(cluster.Nodes) {

				if cNode.isContainer {
					order = append(order, g.containerRDFSOrder(cNode)...)
					order = append(order, cNode)
				}
			}
			continue
		}
	}

	return order
}

func (g *Graph) containerRDFSOrderContext(root *Node, guard workStepper) ([]*Node, error) {
	var order []*Node
	if root != nil && !root.isContainer {
		return order, nil
	}

	for _, child := range slices.Backward(g.Containers[root]) {
		if err := guard.Step(); err != nil {
			return nil, err
		}

		if child.isContainer {
			descendants, err := g.containerRDFSOrderContext(child, guard)
			if err != nil {
				return nil, err
			}
			order = append(order, descendants...)
			order = append(order, child)
			continue
		}
		if child.isClusterVessel {
			cluster := g.Clusters[child]
			for _, clusterNode := range slices.Backward(cluster.Nodes) {
				if err := guard.Step(); err != nil {
					return nil, err
				}

				if clusterNode.isContainer {
					descendants, err := g.containerRDFSOrderContext(clusterNode, guard)
					if err != nil {
						return nil, err
					}
					order = append(order, descendants...)
					order = append(order, clusterNode)
				}
			}
		}
	}
	return order, nil
}

func (g *Graph) syncNested() {
	for _, n := range g.Nodes {
		if n.isContainer {
			n.positionContainerChildren(true)
		}
		if n.isClusterVessel {
			c := g.Clusters[n]
			c.SyncGeometry()
			// Since cluster nodes can themselves be containers, we can't merely move all children nodes, we must
			// also care to give container children padding
			for _, cn := range c.Nodes {
				if cn.isContainer {
					padding := g.containerPadding(cn, true)
					for _, child := range g.Containers[cn] {
						child.moveNodeWithChildren(padding.left, padding.top)
					}
				}
			}
		}
		if seq, is := g.Sequences[n]; is {
			seq.SyncGeometry()
		}
	}
}

// includeClusterNodes should be included when the purpose is to move all nodes
// But if the purpose is to find nodes to perform operations on, the cluster nodes should not be included (vessels will be included)
// Does not include current node
func (g *Graph) allDescendantNodes(node *Node, includeClusterNodes bool) []*Node {
	guard := unboundedWork
	descendants, err := g.allDescendantNodesGuarded(node, includeClusterNodes, guard)
	if err != nil {
		panic(err)
	}
	return descendants
}

func (g *Graph) allDescendantNodesGuarded(node *Node, includeClusterNodes bool, guard workStepper) ([]*Node, error) {
	scratch := descendantWalkPool.Get()
	clear(scratch.seen)
	scratch.stack = scratch.stack[:0]
	scratch.descendants = scratch.descendants[:0]
	defer func() {
		clear(scratch.seen)
		clear(scratch.stack[:cap(scratch.stack)])
		clear(scratch.descendants)
		scratch.stack = scratch.stack[:0]
		scratch.descendants = scratch.descendants[:0]
		descendantWalkPool.Put(scratch)
	}()
	if node != nil {
		scratch.seen[node] = struct{}{}
	}
	pushChildren := func(parent *Node) error {
		// Push in reverse traversal order so the iterative walk retains the old
		// container-before-cluster-before-sequence preorder.
		if sequence := g.Sequences[parent]; sequence != nil {
			for _, v := range slices.Backward(sequence.Nodes) {
				if err := guard.Step(); err != nil {
					return err
				}
				scratch.stack = append(scratch.stack, descendantWalkEntry{node: v, emit: includeClusterNodes})
			}
		}
		if parent != nil && parent.isClusterVessel {
			if cluster := g.Clusters[parent]; cluster != nil {
				for _, v := range slices.Backward(cluster.Nodes) {
					if err := guard.Step(); err != nil {
						return err
					}
					scratch.stack = append(scratch.stack, descendantWalkEntry{node: v, emit: includeClusterNodes})
				}
			}
		}
		if parent == nil || parent.isContainer {
			children := g.Containers[parent]
			for _, child := range slices.Backward(children) {
				if err := guard.Step(); err != nil {
					return err
				}
				scratch.stack = append(scratch.stack, descendantWalkEntry{node: child, emit: true})
			}
		}
		return nil
	}
	if err := pushChildren(node); err != nil {
		return nil, err
	}

	for len(scratch.stack) > 0 {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		last := len(scratch.stack) - 1
		current := scratch.stack[last]
		scratch.stack = scratch.stack[:last]
		if current.node == nil {
			continue
		}
		if _, exists := scratch.seen[current.node]; exists {
			continue
		}
		scratch.seen[current.node] = struct{}{}
		if current.emit {
			scratch.descendants = append(scratch.descendants, current.node)
		}
		if err := pushChildren(current.node); err != nil {
			return nil, err
		}
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return append([]*Node(nil), scratch.descendants...), nil
}

func (g *Graph) ancestorsOfGuarded(node *Node, guard workStepper) (nodes []*Node, err error) {
	seen := make(map[*Node]struct{})
	for current := node; current != nil; {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if _, exists := seen[current]; exists {
			return nil, invariant.New("cycle in transaction ancestry")
		}
		seen[current] = struct{}{}
		if len(seen) > maxEngineNodes {
			return nil, invariant.New("transaction ancestry exceeds node limit")
		}
		if current != node {
			// don't include node itself
			nodes = append(nodes, current)
		}
		if current.Container != nil {
			current = current.Container
		} else if current.Cluster != nil {
			current = current.Cluster.Vessel
		} else if current.Sequence != nil {
			current = current.Sequence.Vessel
		} else {
			break
		}
	}
	return nodes, guard.Finish()
}

func (g *Graph) isTreeSentinel(n *Node) bool {
	_, is := g.Trees[n]
	return is
}

func (g *Graph) isSequence(n *Node) bool {
	_, is := g.Sequences[n]
	return is
}

func (g *Graph) CopyEntitiesFrom(other *Graph) {
	g.Containers = other.Containers
	g.Clusters = other.Clusters
	g.Trees = other.Trees
	g.NodeToTree = other.NodeToTree
	g.Sequences = other.Sequences
	g.Hubs = other.Hubs
	g.Directions = other.Directions
	g.CommonUncleSiblings = other.CommonUncleSiblings
	// TODO maybe not
	g.IsRootHierarchy = other.IsRootHierarchy
}

func (nodes Nodes) fixedNodes() []*Node {
	var fixed []*Node
	for _, n := range nodes {
		if n.FixedTopLeft != nil {
			fixed = append(fixed, n)
		}
	}
	return fixed
}

func (g *Graph) fixedNodes() []*Node {
	return Nodes(g.Nodes).fixedNodes()
}

func (nodes Nodes) hasFixedNode() bool {
	for _, n := range nodes {
		if n.FixedTopLeft != nil {
			return true
		}
	}
	return false
}

func (g *Graph) hasFixedNode() bool {
	return Nodes(g.Nodes).hasFixedNode()
}

// If container A has child B at (80,30) with a FixedTopLeft of (70,20) (ignoring padding)
// then the FixedOrigin point of the nodes [B,C] should be at (10,10), despite their bounding box TL being at (80,30).
// This way when A positions itself around its children, it has the space for B's FixedTopLeft to be correct.
// | Note: This assumes the relative positions between fixed nodes are correct:
// |  if C also has a FixedTopLeft, then it must be (70,50), otherwise B and C's positions are inconsistent.
//
// .  ┌(10,10)───────────────┐
// .  │ A                    │
// .  │          ┌(80,30)┐   │
// .  │          │   B   ├─┐ │
// .  │          └───────┘ │ │
// .  │          ┌(80,60)┐ │ │
// .  │          │   C   ├─┘ │
// .  │          └───────┘   │
// .  └──────────────────────┘
func (nodes Nodes) fixedOrigin() *geo.Point {
	container := nodes.subgraphContainer()
	for _, child := range nodes {
		if child.container() != container {
			continue
		}
		// the origin is the same for all fixed nodes in the container (see note above)
		if p := child.fixedOrigin(); p != nil {
			return p
		}
	}
	return nil
}

func (g *Graph) containerFixedOrigin(container *Node) *geo.Point {
	for _, n := range g.Nodes {
		if n.container() != container {
			continue
		}
		if p := n.fixedOrigin(); p != nil {
			return p
		}
	}
	return nil
}

func (node *Node) fixedOrigin() *geo.Point {
	if node.TopLeft != nil && node.FixedTopLeft != nil {
		return geo.NewPoint(
			node.TopLeft.X-node.FixedTopLeft.X,
			node.TopLeft.Y-node.FixedTopLeft.Y,
		)
	}
	return nil
}

// return the amount of padding the container should have on each side. (more padding if there are icons)
// Note: we need the graph reference since container may be the nil container
// considerChildren should be false if the output is used together with the nodes bounding box, becuase it already accounts for icons
func (g *Graph) containerPadding(container *Node, considerChildren bool) Spacing {
	padding := float64(ContainerPadding)
	spacing := Spacing{top: padding, bottom: padding, left: padding, right: padding}
	if container == nil {
		return spacing
	}
	if container.shapeType == circleType {
		// Circles just need padding so the corner of child doesn't touch corner of circle
		// Optically, there will be a lot of padding on the sides even given a small value
		padding /= 4.
	}

	if container.Icon != nil && container.shapeType != imageType {
		// we don't know if the icon will be placed at an inside position, but leave enough padding so it has that option
		// Note: we don't know what the exact icon size will be until the final layout size so we use max
		padding = float64(MaxIconSize + 2*label.PADDING)
	}

	if container.Label != nil && !container.Label.Position.IsOutside() {
		// First ensure total container width/height can accommodate label
		labelWidth := container.Label.Width + 2*label.PADDING
		labelHeight := container.Label.Height + 2*label.PADDING

		switch container.Label.Position {
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			spacing.top = math.Max(spacing.top, labelHeight)
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			spacing.bottom = math.Max(spacing.bottom, labelHeight)
		case label.InsideMiddleLeft:
			spacing.left = math.Max(spacing.left, labelWidth)
		case label.InsideMiddleRight:
			spacing.right = math.Max(spacing.right, labelWidth)
		}

		// Ensure total width/height can accommodate label
		minContainerWidth := labelWidth + spacing.left + spacing.right
		if container.Width < minContainerWidth {
			extraPadding := math.Ceil((minContainerWidth - container.Width) / 2)
			spacing.left = math.Max(spacing.left, extraPadding)
			spacing.right = math.Max(spacing.right, extraPadding)
		}

		minContainerHeight := labelHeight + spacing.top + spacing.bottom
		if container.Height < minContainerHeight {
			extraPadding := math.Ceil((minContainerHeight - container.Height) / 2)
			spacing.top = math.Max(spacing.top, extraPadding)
			spacing.bottom = math.Max(spacing.bottom, extraPadding)
		}
	}

	if considerChildren {
		var hasChildWithIcon bool
		var childrenMargin Spacing
		for _, child := range g.Containers[container] {
			if !hasChildWithIcon && child.Icon != nil && child.shapeType != imageType && !child.Icon.PositionFixed() {
				hasChildWithIcon = true
			}
			childrenMargin.left = math.Max(childrenMargin.left, child.margin.left)
			childrenMargin.right = math.Max(childrenMargin.right, child.margin.right)
			childrenMargin.top = math.Max(childrenMargin.top, child.margin.top)
			childrenMargin.bottom = math.Max(childrenMargin.bottom, child.margin.bottom)
		}
		spacing.left = math.Max(spacing.left, container.padding.left+childrenMargin.left)
		spacing.right = math.Max(spacing.right, container.padding.right+childrenMargin.right)
		spacing.top = math.Max(spacing.top, container.padding.top+childrenMargin.top)
		spacing.bottom = math.Max(spacing.bottom, container.padding.bottom+childrenMargin.bottom)

		if hasChildWithIcon {
			// we don't know if the icon will be placed at an inside position, but leave enough padding so it has that option
			// Note: we don't know what the exact icon size will be until the final layout size so we use max
			padding = float64(MaxIconSize + 2*label.PADDING)
		}
	}

	if container.shapeType == circleType {
		spacing.top /= 4.
		spacing.left /= 4.
		spacing.bottom /= 4.
		spacing.right /= 4.
	}

	spacing.left = math.Max(spacing.left, math.Max(container.padding.left, padding))
	spacing.right = math.Max(spacing.right, math.Max(container.padding.right, padding))
	spacing.top = math.Max(spacing.top, math.Max(container.padding.top, padding))
	spacing.bottom = math.Max(spacing.bottom, math.Max(container.padding.bottom, padding))

	return spacing
}
