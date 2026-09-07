package packing

import (
	"context"
	"math"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphbounds"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func newWorkGuard(ctx context.Context, limit int64) (*limits.WorkGuard, error) {
	guard, err := limits.NewWorkGuard(ctx, "BinPack", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	guard.SetLimit(limit)
	return guard, nil
}

func binPackScoreGuarded(nodes layoutgraph.Nodes, root *layoutgraph.Node, guard *limits.WorkGuard) (float64, error) {
	topLeft, bottomRight, err := graphbounds.FixedBoundingBox(nodes, guard)
	if err != nil {
		return 0, err
	}
	width := bottomRight.X - topLeft.X
	height := bottomRight.Y - topLeft.Y
	desiredAxisPenalty := 0.
	if root != nil {
		if root.DesiredWidth != nil && root.DesiredHeight == nil && width < *root.DesiredWidth {
			desiredAxisPenalty = *root.DesiredWidth - width
		} else if root.DesiredWidth == nil && root.DesiredHeight != nil && height < *root.DesiredHeight {
			desiredAxisPenalty = *root.DesiredHeight - height
		}
	}
	score := width*height + math.Pow(width-height, 2.0)*subgraphSquareDampener +
		math.Pow(desiredAxisPenalty, 2.0)*subgraphSquareDampener
	return score, guard.Finish()
}

func binPackContainerTopLeft(g *layoutgraph.Graph, container *layoutgraph.Node, children layoutgraph.Nodes, guard *limits.WorkGuard) (*geo.Point, error) {
	topLeft, bottomRight, err := graphbounds.FixedBoundingBox(children, guard)
	if err != nil {
		return nil, err
	}
	if container == nil {
		return topLeft, guard.Finish()
	}
	padding := g.ContainerPadding(container, false)
	inside := container.InsidePlacement(bottomRight.X-topLeft.X, bottomRight.Y-topLeft.Y, padding)
	return &inside, guard.Finish()
}

func binPackIsDescendantOf(descendant, ancestor *layoutgraph.Node, guard *limits.WorkGuard) (bool, error) {
	seen := make(map[*layoutgraph.Node]struct{})
	for {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if descendant == ancestor {
			return true, nil
		}
		if descendant == nil {
			return false, nil
		}
		if _, exists := seen[descendant]; exists {
			return false, invariant.New("BinPack found a cycle in node ancestry")
		}
		seen[descendant] = struct{}{}
		switch {
		case descendant.Container != nil:
			descendant = descendant.Container
		case descendant.Cluster != nil && descendant.Cluster.Vessel != nil:
			descendant = descendant.Cluster.Vessel
		case descendant.Sequence != nil && descendant.Sequence.Vessel != nil:
			descendant = descendant.Sequence.Vessel
		default:
			return ancestor == nil, nil
		}
	}
}

func binPackHasExternalConnection(
	g *layoutgraph.Graph,
	node, container *layoutgraph.Node,
	near, containerIsExternal bool,
	guard *limits.WorkGuard,
) (bool, error) {
	if container == nil {
		return false, nil
	}
	descendants, err := g.AllDescendantNodesWithWorkGuard(node, true, guard)
	if err != nil {
		return false, err
	}
	toVisit := make([]*layoutgraph.Node, 0, len(descendants)+1)
	toVisit = append(toVisit, node)
	for _, descendant := range descendants {
		if err := guard.Step(); err != nil {
			return false, err
		}
		toVisit = append(toVisit, descendant)
	}
	for _, current := range toVisit {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if near {
			orderedNears, err := binPackOrderedNears(current, guard)
			if err != nil {
				return false, err
			}
			for _, adjacent := range orderedNears {
				inside, err := binPackIsDescendantOf(adjacent, container, guard)
				if err != nil {
					return false, err
				}
				if !inside {
					return true, nil
				}
			}
			continue
		}
		for _, edge := range current.Edges {
			if err := guard.Step(); err != nil {
				return false, err
			}
			adjacent := current.Adjacent(edge)
			// Once routes exist, moving a descendant connected to this container
			// translates the complete route and pulls the container endpoint off its
			// fixed border. Treat that attachment like a cross-container constraint.
			if containerIsExternal && adjacent == container {
				return true, nil
			}
			inside, err := binPackIsDescendantOf(adjacent, container, guard)
			if err != nil {
				return false, err
			}
			if !inside {
				return true, nil
			}
		}
	}
	return false, guard.Finish()
}

func binPackOrderedNears(node *layoutgraph.Node, guard *limits.WorkGuard) ([]*layoutgraph.Node, error) {
	if node == nil {
		return nil, invariant.New("BinPack cannot order nears for a nil node")
	}
	if len(node.Nears) > limits.MaxEngineNodes {
		return nil, invariant.New("BinPack node near count exceeds node limit")
	}
	nears := make([]*layoutgraph.Node, 0, len(node.Nears))
	for near := range node.Nears {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if near == nil {
			return nil, invariant.New("BinPack found a nil near")
		}
		nears = append(nears, near)
	}
	// Account for the comparison sort before entering the small, bounded legacy
	// sorter, so every recursive BinPack helper consumes the same stage budget.
	for width := len(nears); width > 1; width = (width + 1) / 2 {
		for range nears {
			if err := guard.Step(); err != nil {
				return nil, err
			}
		}
	}
	layoutgraph.SortNodesByID(nears)
	return nears, guard.Finish()
}

type binPackHierarchyBox struct {
	topLeft, bottomRight *geo.Point
}

func binPackHierarchyBoxes(packed []layoutgraph.Nodes, guard *limits.WorkGuard) ([]binPackHierarchyBox, error) {
	boxes := make([]binPackHierarchyBox, 0)
	for _, nodes := range packed {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if len(nodes) == 0 {
			return nil, invariant.New("BinPack contains an empty packed subgraph")
		}
		if nodes[0].Hierarchy == nil {
			continue
		}
		topLeft, bottomRight, err := graphbounds.BoundingBox(nodes, guard)
		if err != nil {
			return nil, err
		}
		boxes = append(boxes, binPackHierarchyBox{topLeft: topLeft, bottomRight: bottomRight})
	}
	return boxes, guard.Finish()
}

func binPackPointInHierarchy(boxes []binPackHierarchyBox, point *geo.Point, guard *limits.WorkGuard) (bool, error) {
	if point == nil {
		return false, invariant.New("BinPack hierarchy check received a nil point")
	}
	for _, box := range boxes {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if box.topLeft.X <= point.X && point.X <= box.bottomRight.X &&
			box.topLeft.Y <= point.Y && point.Y <= box.bottomRight.Y {
			return true, nil
		}
	}
	return false, guard.Finish()
}

func binPackWrapChildren(root *layoutgraph.Node, guard *limits.WorkGuard) error {
	if !root.IsContainer() {
		return nil
	}
	children := layoutgraph.Nodes(root.Graph.Containers[root])
	padding := root.Graph.ContainerPadding(root, false)
	topLeft, bottomRight, err := graphbounds.FixedBoundingBox(children, guard)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := guard.Step(); err != nil {
			return err
		}
		if child.Label != nil && (child.TopLeft.X == topLeft.X || child.TopLeft.X+child.Width == bottomRight.X) &&
			child.Label.Width > child.Width {
			topLeft.X = math.Min(topLeft.X, math.Floor(child.TopLeft.X+(child.Width/2.-child.Label.Width/2.)))
			bottomRight.X = math.Max(bottomRight.X, math.Ceil(child.TopLeft.X+child.Width-(child.Width/2.-child.Label.Width/2.)))
		}
	}
	root.FitToBoundingBox(topLeft, bottomRight, padding)
	inside := root.InsidePlacement(bottomRight.X-topLeft.X, bottomRight.Y-topLeft.Y, padding)
	root.Translate(topLeft.X-inside.X, topLeft.Y-inside.Y)
	return guard.Finish()
}

func binPackMoveNodeWithChildren(node *layoutgraph.Node, dx, dy float64, guard *limits.WorkGuard) error {
	if err := guard.Step(); err != nil {
		return err
	}
	if dx == 0 && dy == 0 {
		return nil
	}
	if node == nil || node.TopLeft == nil || node.Graph == nil {
		return invariant.New("BinPack cannot move an unplaced node or a node without a graph")
	}
	node.Translate(dx, dy)
	descendants, err := node.Graph.AllDescendantNodesWithWorkGuard(node, true, guard)
	if err != nil {
		return err
	}
	for _, child := range descendants {
		if err := guard.Step(); err != nil {
			return err
		}
		if child == nil || child.TopLeft == nil {
			return invariant.New("BinPack cannot move an unplaced descendant")
		}
		child.Translate(dx, dy)
	}
	return guard.Finish()
}

func binPackPositionContainerChildren(node *layoutgraph.Node, guard *limits.WorkGuard) error {
	if err := guard.Step(); err != nil {
		return err
	}
	if node == nil || !node.IsContainer() {
		return nil
	}
	if node.Graph == nil {
		return invariant.New("BinPack container has no graph")
	}
	children := layoutgraph.Nodes(node.Graph.Containers[node])
	topLeft, bottomRight, err := graphbounds.FixedBoundingBox(children, guard)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := guard.Step(); err != nil {
			return err
		}
		if child == nil || child.TopLeft == nil {
			return invariant.New("BinPack container contains an unplaced child")
		}
		if child.Label != nil && (child.TopLeft.X == topLeft.X || child.TopLeft.X+child.Width == bottomRight.X) &&
			child.Label.Width > child.Width {
			topLeft.X = math.Min(topLeft.X, math.Floor(child.TopLeft.X+(child.Width/2.-child.Label.Width/2.)))
			bottomRight.X = math.Max(bottomRight.X, math.Ceil(child.TopLeft.X+child.Width-(child.Width/2.-child.Label.Width/2.)))
		}
	}
	inside := node.InsidePlacement(bottomRight.X-topLeft.X, bottomRight.Y-topLeft.Y, layoutgraph.Spacing{})
	dx := inside.X - topLeft.X
	dy := inside.Y - topLeft.Y
	for _, child := range children {
		if err := binPackMoveNodeWithChildren(child, dx, dy, guard); err != nil {
			return err
		}
	}
	return guard.Finish()
}

type binPackClusterGeometryWork struct {
	guard *limits.WorkGuard
}

func (work binPackClusterGeometryWork) Step() error   { return work.guard.Step() }
func (work binPackClusterGeometryWork) Finish() error { return work.guard.Finish() }

func (work binPackClusterGeometryWork) MoveNodeWithChildren(node *layoutgraph.Node, dx, dy float64) error {
	return binPackMoveNodeWithChildren(node, dx, dy, work.guard)
}

func (work binPackClusterGeometryWork) PositionContainerChildren(node *layoutgraph.Node) error {
	return binPackPositionContainerChildren(node, work.guard)
}

var _ layoutgraph.ClusterGeometryWork = binPackClusterGeometryWork{}

type binPackRDFSFrame struct {
	node  *layoutgraph.Node
	phase uint8
	index int
}

func binPackReverseDFSWalk(root *layoutgraph.Node, apply func(*layoutgraph.Node) error, guard *limits.WorkGuard) error {
	if root == nil {
		return invariant.New("BinPack reverse traversal contains a nil node")
	}
	stack := []binPackRDFSFrame{{node: root}}
	active := map[*layoutgraph.Node]struct{}{root: {}}
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		frame := &stack[len(stack)-1]
		graph := frame.node.Graph
		if graph == nil {
			return invariant.New("BinPack reverse traversal found a node without a graph")
		}
		var children layoutgraph.Nodes
		switch frame.phase {
		case 0:
			if frame.node.IsContainer() {
				children = graph.Containers[frame.node]
			}
		case 1:
			if frame.node.IsClusterVessel() {
				cluster := graph.Clusters[frame.node]
				if cluster == nil {
					return invariant.New("BinPack reverse traversal found a cluster vessel without a cluster")
				}
				children = cluster.Nodes
			} else if sequence := graph.Sequences[frame.node]; sequence != nil {
				children = sequence.Nodes
			}
		default:
			if err := apply(frame.node); err != nil {
				return err
			}
			delete(active, frame.node)
			stack = stack[:len(stack)-1]
			continue
		}
		if frame.index >= len(children) {
			frame.phase++
			frame.index = 0
			continue
		}
		child := children[frame.index]
		frame.index++
		if child == nil {
			return invariant.New("BinPack reverse traversal contains a nil child")
		}
		if _, exists := active[child]; exists {
			return invariant.New("BinPack reverse traversal found a topology cycle")
		}
		active[child] = struct{}{}
		stack = append(stack, binPackRDFSFrame{node: child})
	}
	return guard.Finish()
}

func binPackSyncClusters(graph *layoutgraph.Graph, guard *limits.WorkGuard) error {
	if graph == nil {
		return invariant.New("BinPack cannot synchronize a nil graph")
	}
	if len(graph.Clusters) == 0 {
		return guard.Finish()
	}
	work := binPackClusterGeometryWork{guard: guard}
	for _, root := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := binPackReverseDFSWalk(root, func(node *layoutgraph.Node) error {
			if !node.IsClusterVessel() {
				return nil
			}
			return graph.Clusters[node].SyncGeometryWithWork(work)
		}, guard); err != nil {
			return err
		}
	}
	return guard.Finish()
}

func binPackSyncSequences(graph *layoutgraph.Graph, guard *limits.WorkGuard) error {
	if graph == nil {
		return invariant.New("BinPack cannot synchronize a nil graph")
	}
	if len(graph.Sequences) == 0 {
		return guard.Finish()
	}
	for _, root := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := binPackReverseDFSWalk(root, func(node *layoutgraph.Node) error {
			sequence := graph.Sequences[node]
			if sequence == nil {
				return nil
			}
			return sequence.SyncGeometryWithWork(guard)
		}, guard); err != nil {
			return err
		}
	}
	return guard.Finish()
}

func binPackSmallestDeltas(subgraphs []layoutgraph.Nodes, guard *limits.WorkGuard) (float64, float64, error) {
	x, y := math.Inf(1), math.Inf(1)
	for _, subgraph := range subgraphs {
		if err := guard.Step(); err != nil {
			return 0, 0, err
		}
		topLeft, bottomRight, err := graphbounds.BoundingBox(subgraph, guard)
		if err != nil {
			return 0, 0, err
		}
		x = math.Min(x, bottomRight.X-topLeft.X)
		y = math.Min(y, bottomRight.Y-topLeft.Y)
	}
	if err := guard.Finish(); err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// allEdgesHaveCompleteRoutesGuarded enforces the all-or-none route state that
// BinPack relies on. Once routes exist, connected components move as rigid
// bodies and routed container geometry becomes a packing constraint. Treating a
// partially routed graph as wholly unrouted would skip both protections while
// leaving the complete routes in place.
//
// The graph-owned edge list and every affected node-owned edge list must also
// agree exactly. Otherwise a complete route omitted from either inventory could
// evade route-aware packing and be detached or cut by a move or container
// shrink. Self-loops occur once in both inventories.
func allEdgesHaveCompleteRoutesGuarded(graph *layoutgraph.Graph, root *layoutgraph.Node, guard *limits.WorkGuard) (bool, []*layoutgraph.Edge, error) {
	if graph == nil {
		return false, nil, invariant.New("BinPack received a nil graph")
	}
	var descendants []*layoutgraph.Node
	if root == nil {
		descendants = graph.Nodes
	} else {
		var err error
		descendants, err = graph.AllDescendantNodesWithWorkGuard(root, true, guard)
		if err != nil {
			return false, nil, err
		}
	}
	affectedNodes := make([]*layoutgraph.Node, 0, len(descendants)+1)
	if root != nil {
		affectedNodes = append(affectedNodes, root)
	}
	affectedNodes = append(affectedNodes, descendants...)
	affectedNodeSet := make(map[*layoutgraph.Node]struct{}, len(affectedNodes))
	for _, node := range affectedNodes {
		if err := guard.Step(); err != nil {
			return false, nil, err
		}
		if node == nil || node.Graph != graph {
			return false, nil, invariant.New("BinPack affected-node inventory is malformed")
		}
		if _, duplicate := affectedNodeSet[node]; duplicate {
			return false, nil, invariant.Errorf("BinPack affected-node inventory repeats node %d", node.ID)
		}
		affectedNodeSet[node] = struct{}{}
	}

	expectedNodeEdges := make(map[*layoutgraph.Node]map[*layoutgraph.Edge]int, len(affectedNodes))
	expectedNodeEdgeTotals := make(map[*layoutgraph.Node]int, len(affectedNodes))
	for _, node := range affectedNodes {
		expectedNodeEdges[node] = make(map[*layoutgraph.Edge]int)
	}
	affectedGraphEdges := make(map[*layoutgraph.Edge]struct{})
	routedEdges := 0
	var incidentEdges []*layoutgraph.Edge
	for edgeIndex, edge := range graph.Edges {
		if err := guard.Step(); err != nil {
			return false, nil, err
		}
		if edge == nil {
			return false, nil, invariant.Errorf("BinPack found a nil edge at index %d", edgeIndex)
		}
		if edge.From == nil || edge.To == nil {
			return false, nil, invariant.Errorf("BinPack edge %d has a nil endpoint", edge.ID)
		}
		if edge.From.Graph != graph || edge.To.Graph != graph {
			return false, nil, invariant.Errorf("BinPack edge %d has an endpoint owned by another graph", edge.ID)
		}
		edgeAffectsRoot := false
		if _, affected := affectedNodeSet[edge.From]; affected {
			edgeAffectsRoot = true
			expectedNodeEdges[edge.From][edge]++
			expectedNodeEdgeTotals[edge.From]++
		}
		if edge.To != edge.From {
			if _, affected := affectedNodeSet[edge.To]; affected {
				edgeAffectsRoot = true
				expectedNodeEdges[edge.To][edge]++
				expectedNodeEdgeTotals[edge.To]++
			}
		}
		if edgeAffectsRoot {
			if _, duplicate := affectedGraphEdges[edge]; duplicate {
				return false, nil, invariant.Errorf("BinPack graph edge inventory repeats affected edge %d", edge.ID)
			}
			affectedGraphEdges[edge] = struct{}{}
		}
		if len(edge.Points) == 0 {
			continue
		}
		if len(edge.Points) < 2 {
			return false, nil, invariant.Errorf(
				"BinPack edge %d has an incomplete route with %d point(s)",
				edge.ID, len(edge.Points),
			)
		}
		for pointIndex, point := range edge.Points {
			if err := guard.Step(); err != nil {
				return false, nil, err
			}
			if point == nil {
				return false, nil, invariant.Errorf(
					"BinPack edge %d has a nil route point at index %d",
					edge.ID, pointIndex,
				)
			}
		}
		routedEdges++
		if root != nil && (edge.From == root || edge.To == root) {
			incidentEdges = append(incidentEdges, edge)
		}
	}
	if routedEdges != 0 && routedEdges != len(graph.Edges) {
		return false, nil, invariant.Errorf(
			"BinPack graph is partially routed: %d of %d edges have routes",
			routedEdges, len(graph.Edges),
		)
	}

	for _, node := range affectedNodes {
		actualNodeEdges := make(map[*layoutgraph.Edge]int, len(node.Edges))
		for edgeIndex, edge := range node.Edges {
			if err := guard.Step(); err != nil {
				return false, nil, err
			}
			if edge == nil {
				return false, nil, invariant.Errorf(
					"BinPack node %d edge inventory contains a nil edge at index %d",
					node.ID, edgeIndex,
				)
			}
			if edge.From != node && edge.To != node {
				return false, nil, invariant.Errorf(
					"BinPack node %d edge inventory contains non-incident edge %d",
					node.ID, edge.ID,
				)
			}
			actualNodeEdges[edge]++
			expectedOccurrences := expectedNodeEdges[node][edge]
			if actualNodeEdges[edge] > expectedOccurrences {
				return false, nil, invariant.Errorf(
					"BinPack node %d edge inventory has %d occurrence(s) of edge %d; graph requires %d",
					node.ID, actualNodeEdges[edge], edge.ID, expectedOccurrences,
				)
			}
		}
		if len(node.Edges) != expectedNodeEdgeTotals[node] {
			return false, nil, invariant.Errorf(
				"BinPack node %d edge inventory has %d occurrence(s); graph requires %d",
				node.ID, len(node.Edges), expectedNodeEdgeTotals[node],
			)
		}
	}
	if routedEdges == 0 {
		return false, nil, guard.Finish()
	}
	return true, incidentEdges, guard.Finish()
}
