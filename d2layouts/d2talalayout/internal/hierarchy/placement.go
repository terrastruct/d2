package hierarchy

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

	"github.com/d2lang/d2/lib/geo"
)

const (
	crossingSpacing     = 50.0
	parentSpacing       = 300.0
	siblingDummySpacing = 50.0
	siblingSpacing      = 60.0
)

type placementNode struct {
	graphNode *layoutgraph.Node
	level     int
	// this is the level index considering the order left to right as if it was a flatten array, no container depth considered
	rank      int
	aboves    map[*placementNode]struct{}
	belows    map[*placementNode]struct{}
	container *placementNode
	children  []*placementNode
	// flags whether or not we should recurse to optimize the children's position
	// currently only for SQL Tables as we need to consider their columns as `children`
	// but we don't want to change their position
	// also, tables are simple nodes and we create children as dummy nodes so that we can optimize
	// the tables position based on the columns (as routing from column to column can change crossings)
	optimizeChildrenCrossings bool

	// a flag for intermediate nodes used to break long connections
	// a connection from level 1 to 3 is broken with a dummy node at level 2
	// (so that all `aboves`/`belows` are 1 level apart)
	// this flags marks the node at level 2 as being part of the chain
	isChainningConnection bool

	// isContainer quickly checks if a node is a container or not
	// Table nodes have children, but aren't containers
	isContainer bool

	isDummy bool
}

func newPlacementNode(level int, node *layoutgraph.Node) *placementNode {
	return &placementNode{
		graphNode:                 node,
		level:                     level,
		rank:                      0,
		aboves:                    make(map[*placementNode]struct{}),
		belows:                    make(map[*placementNode]struct{}),
		container:                 nil,
		children:                  nil,
		optimizeChildrenCrossings: true,
		isChainningConnection:     false,
		isContainer:               false,
		isDummy:                   false,
	}
}

func (pn *placementNode) degree() int {
	if pn.isDummy {
		// dummy nodes have only 2 edges
		return 2
	}
	return countHierarchyStructuralEdges(pn.graphNode)
}

func (pn *placementNode) findNonDummyNode(above bool) *placementNode {
	firstNeighbor := func(n *placementNode, isAbove bool) *placementNode {
		nodes := n.aboves
		if !isAbove {
			nodes = n.belows
		}
		for connected := range nodes {
			return connected
		}
		return nil
	}
	n := pn
	for n.isDummy && n.isChainningConnection {
		n = firstNeighbor(n, above)
	}
	return n
}

func (below *placementNode) connect(above *placementNode) {
	if above.level == below.level {
		return
	}
	if above.level > below.level {
		below, above = above, below
	}
	below.aboves[above] = struct{}{}
	above.belows[below] = struct{}{}
}

func Place(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, randGenerator *rand.Rand) (err error) {
	if err := layoutgraph.Validate(ctx, "PlaceHierarchies", g); err != nil {
		return err
	}
	if len(g.Containers[root]) == 0 {
		return nil
	}
	ctx, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "PlaceHierarchiesTransactions")
	if err != nil {
		return err
	}
	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureTopology:   true,
		CaptureEdgeRoutes: true,
	})
	if err := state.UpdateWithWorkGuard(g, guard); err != nil {
		return err
	}
	rollback := &layoutgraph.Transaction{Graph: g, PriorGraphState: state}
	complete := false
	defer func() {
		if !complete {
			rollback.Rollback()
		}
	}()

	if err := place(ctx, g, root, randGenerator); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("PlaceHierarchies: %w", err)
	}
	complete = true
	return nil
}

func place(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, randGenerator *rand.Rand) error {

	hGraph := layoutgraph.NewGraph()
	hGraph.CopyEntitiesFrom(g)
	for _, child := range g.Containers[root] {
		hGraph.AddNodeUnchecked(child)
	}

	edgeAbductions := g.AbductEdges(root, hGraph)
	edgesRestored := false
	var splitOwnership layoutgraph.NodeGraphOwnershipJournal
	defer func() {
		if !edgesRestored {
			g.RestoreEdgeAbductions(edgeAbductions)
		}
		// The split follows Nears outside hGraph.Nodes and temporarily redirects
		// their owners too. Restore every exact pre-split owner before retaining the
		// hierarchy stage's existing successful owner state for nodes in g.
		splitOwnership.Restore()
		layoutgraph.Nodes(g.Nodes).SetGraphReference(g)
	}()

	for _, child := range g.Containers[root] {
		if child.Hierarchy == nil && child.IsContainer() {
			if err := place(ctx, g, child, randGenerator); err != nil {
				return err
			}
		}
	}

	subgraphs, splitOwnership, err := hGraph.SplitSubgraphsTracked(ctx, layoutgraph.SplitOptions{IncludeNears: true}, nil)
	if err != nil {
		return err
	}
	for _, subgraph := range subgraphs {
		if subgraph.Nodes[0].Hierarchy == nil {
			continue
		}
		subgraph.ComputeCellSize()
		layoutgraph.Nodes(g.Nodes).SetGraphReference(subgraph)
		edgeAbductions = subgraph.RestoreEdgeAbductions(edgeAbductions)
		if err := placeNodesInHierarchy(ctx, subgraph, randGenerator); err != nil {
			return err
		}
	}

	g.RestoreEdgeAbductions(edgeAbductions)
	edgesRestored = true
	return nil
}

func placeNodesInHierarchy(ctx context.Context, g *layoutgraph.Graph, rand *rand.Rand) (err error) {
	// A hierarchy may already be attached by an earlier stage. Never let derived
	// state override an absolute constraint on a member, including a nested
	// container member.
	if hierarchy := g.Nodes[0].Hierarchy; hierarchy != nil {
		for node := range hierarchy.Levels() {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("PlaceHierarchies: %w", err)
			}
			if node.FixedTopLeft != nil {
				return nil
			}
		}
	}
	ctx, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "PlaceHierarchiesTransactions")
	if err != nil {
		return err
	}
	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureEdgeRoutes: true,
	})
	if err := state.UpdateWithWorkGuard(g, guard); err != nil {
		return err
	}
	rollback := &layoutgraph.Transaction{Graph: g, PriorGraphState: state}
	complete := false
	defer func() {
		if !complete {
			rollback.Rollback()
		}
	}()

	placementNodes := createPlacementNodes(g, g.Nodes, rand)
	connectPlacementNodes(g, placementNodes)
	byLevel := groupPlacementNodesByLevel(placementNodes)
	initializeRanks(byLevel)
	breakLongConnections(placementNodes, byLevel)
	minimizeHierarchyCrossings(byLevel)
	if err := globalSifting(byLevel); err != nil {
		return err
	}
	isHorizontal := IsHorizontal(g.Nodes)
	if isHorizontal {
		transpose(g)
	}
	placeNodesByLevel(g, byLevel, isHorizontal)
	for level := 0; level < len(byLevel); level++ {
		byLevel[level] = removeTableColumnNodes(byLevel[level])
		computeLevelRanks(byLevel[level])
	}
	if err := alignHierarchy(ctx, g, byLevel); err != nil {
		return err
	}
	if isHorizontal {
		transpose(g)
	}
	switch g.Nodes[0].ContainerDirection() {
	case geo.Left:
		mirrorX(g)
	case geo.Top:
		mirrorY(g)
	}
	complete = true
	return nil
}

// create placement nodes recursively and set the container whenever required
func createPlacementNodes(g *layoutgraph.Graph, nodes []*layoutgraph.Node, rand *rand.Rand) []*placementNode {
	var placementNodes []*placementNode
	for _, node := range nodes {
		pn := newPlacementNode(node.HierarchyLevel(), node)
		placementNodes = append(placementNodes, pn)
		if node.IsTable() {
			pn.children = make([]*placementNode, node.NumColumns())
			pn.optimizeChildrenCrossings = false
			pn.isContainer = false
			for i := 0; i < node.NumColumns(); i++ {
				pn.children[i] = newPlacementNode(pn.level, nil)
				pn.children[i].container = pn
				pn.children[i].isDummy = true
			}
		} else {
			pn.optimizeChildrenCrossings = true
			pn.children = make([]*placementNode, 0, len(g.Containers[node]))
			for _, child := range createPlacementNodes(g, g.Containers[node], rand) {
				if child.container == nil {
					child.container = pn
					pn.children = append(pn.children, child)
				}
			}
			pn.isContainer = len(pn.children) > 0
		}
	}
	if rand != nil {
		rand.Shuffle(len(placementNodes), func(i, j int) {
			placementNodes[i], placementNodes[j] = placementNodes[j], placementNodes[i]
		})
	}
	return placementNodes
}

func connectPlacementNodes(g *layoutgraph.Graph, nodes []*placementNode) {
	nodeToPlacementNode := map[*layoutgraph.Node]*placementNode{}
	queue := make([]*placementNode, 0, len(nodes))
	queue = append(queue, nodes...)
	for len(queue) > 0 {
		pn := queue[0]
		queue = queue[1:]
		nodeToPlacementNode[pn.graphNode] = pn
		queue = append(queue, pn.children...)
	}

	for _, edge := range g.Edges {
		from := nodeToPlacementNode[edge.From]
		to := nodeToPlacementNode[edge.To]
		to.connect(from)

		if edge.IsBetweenTableColumns() {
			// connect the rows
			from = from.children[*edge.FromTableColumnIndex]
			to = to.children[*edge.ToTableColumnIndex]
			to.connect(from)
		}
	}
}

// breakLongConnections creates chains of dummy nodes when two nodes are connected but their level distance is greater than 1
// . ┌──────────┐           ┌──────────┐
// . │          │           │          │
// . └────┬─────┘           └────┬─┬───┘
// .      │                      │ │
// . ┌────▼─────┐                │ │
// . │          │                │ │
// . │          ◄────────────────┘ │
// . │          │              #########
// . │          │              # dummy #
// . │          │              #########
// . └─────┬────┘                  │
// .       │                       │
// .  ┌────▼─────┐         ┌───────▼──┐
// .  │          │         │          │
// .  └──────────┘         └──────────┘
func breakLongConnections(nodes []*placementNode, byLevel map[int][]*placementNode) []*placementNode {
	var dummies []*placementNode
	for _, pn := range nodes {
		belows := make([]*placementNode, 0, len(pn.belows))
		for below := range pn.belows {
			if below.level-pn.level > 1 {
				belows = append(belows, below)
			}
		}
		slices.SortStableFunc(belows, func(a, b *placementNode) int {
			if order := cmp.Compare(a.rank, b.rank); order != 0 {
				return order
			}
			return cmp.Compare(a.level, b.level)
		})
		for _, below := range belows {
			delete(pn.belows, below)
			delete(below.aboves, pn)

			above := pn
			for level := pn.level + 1; level < below.level; level++ {
				dummyID := -layoutgraph.EntityID(len(dummies) + 1)
				dummy := newPlacementNode(level, layoutgraph.NewNode(dummyID, 1, 1))
				dummy.isChainningConnection = true
				dummy.isDummy = true
				dummy.rank = len(byLevel[level])
				dummy.connect(above)
				dummies = append(dummies, dummy)
				byLevel[level] = append(byLevel[level], dummy)
				above = dummy
			}
			below.connect(above)
		}
		dummies = append(dummies, breakLongConnections(pn.children, byLevel)...)
	}
	return dummies
}

func groupPlacementNodesByLevel(nodes []*placementNode) map[int][]*placementNode {
	byLevel := map[int][]*placementNode{}
	for _, pn := range nodes {
		byLevel[pn.level] = append(byLevel[pn.level], pn)
	}
	return byLevel
}

// removeTableColumnNodes removes nodes where `graphNode == nil` and restores edge references
func removeTableColumnNodes(nodes []*placementNode) []*placementNode {
	newNodes := make([]*placementNode, 0, len(nodes))
	for _, node := range nodes {
		if node.isDummy {
			if node.graphNode == nil {
				// table column
				continue
			} else if node.isChainningConnection {
				above := node.findNonDummyNode(true)
				below := node.findNonDummyNode(false)
				if above.graphNode == nil || below.graphNode == nil {
					// dummy node chaining a long connection between two table columns
					continue
				}
			}
		}
		newNodes = append(newNodes, node)
		node.children = removeTableColumnNodes(node.children)
	}
	return newNodes
}

func placeNodesByLevel(g *layoutgraph.Graph, byLevel map[int][]*placementNode, isHorizontal bool) {
	yOffset := 0.0
	for i := 0; i < len(byLevel); i++ {
		var nodes []*layoutgraph.Node
		levelHeight := 0.0
		xOffset := 0.0
		for _, pn := range byLevel[i] {
			nodes = append(nodes, pn.graphNode)
			pn.graphNode.TopLeft = geo.NewPoint(xOffset, yOffset)
			if !pn.isDummy {
				var container *layoutgraph.Node
				if pn.container != nil {
					container = pn.container.graphNode
				}
				padding := g.ContainerPadding(container, true)
				placeDescendants(g, pn, pn.graphNode.InsidePlacement(1, 1, padding))
				levelHeight = math.Max(levelHeight, pn.graphNode.Height)
				xOffset += math.Ceil(pn.graphNode.Width) + siblingSpacing
			} else {
				xOffset += siblingSpacing
			}
		}
		distanceToNextLevel := minimumDistanceToNextLevel(byLevel[i], isHorizontal)
		yOffset += math.Ceil(levelHeight + distanceToNextLevel + 2*layoutgraph.MinPortClearance)

		// move to center
		center := layoutgraph.Nodes(nodes).Center()
		center.Y = math.Ceil(center.Y)
		for _, node := range nodes {
			yDiff := math.Ceil(center.Y - node.Center().Y)
			node.Translate(0, yDiff)
		}
	}
}

// recursively place all descendants of a given container
func placeDescendants(g *layoutgraph.Graph, root *placementNode, tl geo.Point) {
	if !root.isContainer {
		return
	}
	padding := g.ContainerPadding(root.graphNode, true)
	x := tl.X
	for _, child := range root.children {
		child.graphNode.TopLeft = geo.NewPoint(x, tl.Y)
		placeDescendants(g, child, child.graphNode.InsidePlacement(1, 1, padding))
		x += math.Ceil(child.graphNode.Width) + siblingSpacing
	}
	childrenTL, childrenBR := layoutgraph.Nodes(g.Containers[root.graphNode]).BoundingBox()
	root.graphNode.FitToBoundingBox(childrenTL, childrenBR, padding)
}

// minimumDistanceToNextLevel computes the distance between two levels, accounting for crossings between them.
// and the label sizes
func minimumDistanceToNextLevel(levelNodes []*placementNode, isHorizontal bool) float64 {
	var scratch crossingScratch
	segments := scratch.crossLevelSegments(levelNodes, false, true)
	crossings := countCrossings(segments)
	crossingsDistance := float64(crossings) * crossingSpacing

	maxLabelSize := 0.
	for _, node := range levelNodes {
		if node.isDummy {
			continue
		}
		for _, e := range node.graphNode.Edges {
			adj := node.graphNode.Adjacent(e)
			from := node.graphNode
			to := adj
			if to.HierarchyLevel() < from.HierarchyLevel() {
				from, to = to, from
			}
			if to.HierarchyLevel()-from.HierarchyLevel() != 1 {
				continue
			}
			if e.Label != nil {
				if isHorizontal {
					maxLabelSize = math.Max(e.Label.Width, maxLabelSize)
				} else {
					maxLabelSize = math.Max(e.Label.Height, maxLabelSize)
				}
			}
		}
	}
	distance := math.Max(float64(crossingsDistance), maxLabelSize+crossingSpacing)
	// ensure there'll be some space if there are no crossings
	distance = math.Max(crossingSpacing, distance)
	// crossings distance is a kind of overshot (as routes might be shared), so clipping here to avoid large gaps between levels
	distance = math.Min(distance, parentSpacing)
	return math.Round(distance)
}

func isSource(n *layoutgraph.Node) bool {
	for _, e := range n.Edges {
		if !isHierarchyStructuralEdge(e) {
			continue
		}
		if e.HasSourceArrow() == e.HasTargetArrow() || (e.HasSourceArrow() && e.From == n) || (e.HasTargetArrow() && e.To == n) {
			return false
		}
	}
	return true
}

func isSink(n *layoutgraph.Node) bool {
	for _, e := range n.Edges {
		if !isHierarchyStructuralEdge(e) {
			continue
		}
		if e.HasSourceArrow() == e.HasTargetArrow() || (e.HasTargetArrow() && e.From == n) || (e.HasSourceArrow() && e.To == n) {
			return false
		}
	}
	return true
}

func iterContainersBFS(g *layoutgraph.Graph, apply func(n *layoutgraph.Node)) {
	queue := make([]*layoutgraph.Node, 0, len(g.Nodes))
	queue = append(queue, g.Nodes...)
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if curr.IsContainer() {
			queue = append(queue, g.Containers[curr]...)
		}
		apply(curr)
	}
}

func transpose(g *layoutgraph.Graph) {
	iterContainersBFS(g, func(n *layoutgraph.Node) {
		n.Transpose()
	})
}

func mirrorX(g *layoutgraph.Graph) {
	iterContainersBFS(g, func(n *layoutgraph.Node) {
		n.Mirror(true, false)
	})
}

func mirrorY(g *layoutgraph.Graph) {
	iterContainersBFS(g, func(n *layoutgraph.Node) {
		n.Mirror(false, true)
	})
}

func (pn *placementNode) DebugID() string {
	return fmt.Sprintf("%s[%d, %d]", pn.graphNode.DebugID(), pn.level, pn.rank)
}
