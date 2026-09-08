package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

	"github.com/d2lang/d2/lib/geo"
)

// Orthogonal Visibility Graph
type OVG struct {
	NodesInsideBoundingBox []*layoutgraph.Node
	Nodes                  []*OVGNode
	OccupiedPoints         map[geo.Point]*OVGNode
	Edges                  []*OVGEdge
	VerticalEdges          map[float64][]*OVGEdge
	HorizontalEdges        map[float64][]*OVGEdge
	Ports                  map[*layoutgraph.Node][]*OVGNode
	Centers                map[*layoutgraph.Node]*OVGNode

	fixedOverlapsMu    sync.Mutex
	fixedOverlapsCache []fixedOverlapsCacheEntry
	buildGuard         *ovgBuildGuard
}

type fixedOverlapsCacheEntry struct {
	graph    *layoutgraph.Graph
	nodes    layoutgraph.Nodes
	overlaps map[*layoutgraph.Node]struct{}
}

func (g *OVG) AddNodeUnchecked(node *OVGNode) {
	g.Nodes = append(g.Nodes, node)
	g.OccupiedPoints[*node.Point] = node
}

func (g *OVG) AddNode(node *OVGNode) *OVGNode {
	if occupant, exists := g.OccupiedPoints[*node.Point]; exists {
		return occupant
	} else {
		g.AddNodeUnchecked(node)
		return node
	}
}

func (g *OVG) Connect(nodeA, nodeB *OVGNode) *OVGEdge {
	if nodeA.IsNodeCenter && !nodeB.isPort() || nodeB.IsNodeCenter && !nodeA.isPort() {
		// only ports can connect to center
		return nil
	}

	edge := NewOVGEdge(nodeA, nodeB)
	g.Edges = append(g.Edges, edge)
	nodeA.addEdge(edge)
	nodeB.addEdge(edge)
	if edge.isVertical() {
		g.VerticalEdges[nodeA.X] = append(g.VerticalEdges[nodeA.X], edge)
	} else if edge.isHorizontal() {
		g.HorizontalEdges[nodeA.Y] = append(g.HorizontalEdges[nodeA.Y], edge)
	}
	// this distance doesn't change as the OVG nodes don't move around
	// so it is safe to precompute it here and just use it later on during edge routing
	edge.Distance = geo.EuclideanDistance(
		nodeA.X,
		nodeA.Y,
		nodeB.X,
		nodeB.Y,
	)
	return edge
}

func (g *OVG) removeIsolatedNodes(guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	newNodes := make([]*OVGNode, 0)

	for _, n := range g.Nodes {
		if err := guard.step(); err != nil {
			return err
		}
		if len(n.Edges) > 0 {
			newNodes = append(newNodes, n)
		}
	}

	g.Nodes = newNodes
	return guard.check()
}

func NewOVG(nodesInsideBoundingBox []*layoutgraph.Node) *OVG {
	return &OVG{
		NodesInsideBoundingBox: nodesInsideBoundingBox,
		Ports:                  map[*layoutgraph.Node][]*OVGNode{},
		Centers:                map[*layoutgraph.Node]*OVGNode{},
		OccupiedPoints:         map[geo.Point]*OVGNode{},
		VerticalEdges:          map[float64][]*OVGEdge{},
		HorizontalEdges:        map[float64][]*OVGEdge{},
	}
}

func newBuildOVG(nodesInsideBoundingBox []*layoutgraph.Node, guard *ovgBuildGuard) *OVG {
	ovg := NewOVG(nodesInsideBoundingBox)
	ovg.buildGuard = guard
	return ovg
}

func (ovg *OVG) reindexOccupiedPoints() {
	ovg.OccupiedPoints = make(map[geo.Point]*OVGNode, len(ovg.Nodes))
	for _, node := range ovg.Nodes {
		ovg.OccupiedPoints[*node.Point] = node
	}
}

func buildOVGFromGraphWithLimits(ctx context.Context, g *layoutgraph.Graph, nearbyNodes []*layoutgraph.Node, limits ovgBuildLimits) (*OVG, error) {
	guard, err := newOVGBuildGuard(ctx, limits)
	if err != nil {
		return nil, err
	}
	return buildOVGFromGraphWithGuard(g, nearbyNodes, guard)
}

func buildOVGFromGraphWithGuard(g *layoutgraph.Graph, nearbyNodes []*layoutgraph.Node, guard *ovgBuildGuard) (*OVG, error) {
	if err := guard.check(); err != nil {
		return nil, err
	}
	orthogonalNodeCapacity, err := checkedOVGSliceCapacity(len(g.Nodes), len(nearbyNodes))
	if err != nil {
		return nil, err
	}
	orthogonalNodes := make([]*layoutgraph.Node, 0, orthogonalNodeCapacity)
	orthogonalNodes = append(orthogonalNodes, nearbyNodes...)

	var hierarchyOVGs []*OVG
	seenHierarchies := make(map[*layoutgraph.Hierarchy]struct{})
	for _, n := range g.Nodes {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if n.Hierarchy == nil {
			orthogonalNodes = append(orthogonalNodes, n)
		} else if _, seen := seenHierarchies[n.Hierarchy]; !seen {
			seenHierarchies[n.Hierarchy] = struct{}{}
			hierarchyOVG, err := newOVGForHierarchy(g, n.Hierarchy, guard)
			if err != nil {
				return nil, err
			}
			hierarchyOVGs = append(hierarchyOVGs, hierarchyOVG)
		}
	}

	ovg := newBuildOVG(orthogonalNodes, guard)
	if err := ovg.addPorts(g, guard); err != nil {
		return nil, err
	}
	for _, hOVG := range hierarchyOVGs {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if err := ovg.mergePorts(hOVG, guard); err != nil {
			return nil, err
		}
	}
	if err := ovg.addNodesIntersections(g, guard); err != nil {
		return nil, err
	}
	for _, hOVG := range hierarchyOVGs {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if err := ovg.mergeNonPorts(hOVG, guard); err != nil {
			return nil, err
		}
	}

	if err := ovg.addEdgesNodes(g, guard); err != nil {
		return nil, err
	}
	if err := ovg.addTreeNodes(g, guard); err != nil {
		return nil, err
	}
	tl, br, err := guard.tightBoundingBox(layoutgraph.Nodes(ovg.NodesInsideBoundingBox))
	if err != nil {
		return nil, err
	}
	if err := ovg.addNewBoundaryLayers(g, tl, br, guard); err != nil {
		return nil, err
	}
	if err := ovg.addPortConnectionNodesAtBoundaries(g, tl, br, guard); err != nil {
		return nil, err
	}
	if err := ovg.addCornerNodes(g, tl, br, guard); err != nil {
		return nil, err
	}
	if err := ovg.addTunnels(g, guard); err != nil {
		return nil, err
	}

	// TODO if any OVG nodes are too close to container edge, split them
	if err := ovg.connectNodes(g, guard); err != nil {
		return nil, err
	}
	if err := ovg.connectPortsToCenter(guard); err != nil {
		return nil, err
	}
	if err := ovg.removeIsolatedNodes(guard); err != nil {
		return nil, err
	}
	if err := ovg.mapNodesToContainer(g, guard); err != nil {
		return nil, err
	}
	if err := ovg.flagNodesNearPorts(guard); err != nil {
		return nil, err
	}
	for i, node := range ovg.Nodes {
		if err := guard.step(); err != nil {
			return nil, err
		}
		node.Index = i
	}

	return ovg, nil
}

// For each tala.Node, get the ports and add them as OVG Nodes
func (ovg *OVG) addPorts(g *layoutgraph.Graph, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	// If all of a container's children have no connections, there's no need to consider them for pathing
	skipPathing := make(map[*layoutgraph.Node]struct{})
	for _, children := range g.Containers {
		if err := guard.step(); err != nil {
			return err
		}
		has := false
		for _, child := range children {
			if err := guard.step(); err != nil {
				return err
			}
			if len(child.Edges) > 0 {
				has = true
				break
			}
		}
		if !has {
			for _, child := range children {
				if err := guard.step(); err != nil {
					return err
				}
				skipPathing[child] = struct{}{}
			}
		}
	}

	// Add ports
	for _, node := range ovg.NodesInsideBoundingBox {
		if err := guard.step(); err != nil {
			return err
		}
		if _, ok := skipPathing[node]; ok {
			continue
		}
		// most shapes have 12 ports (3 on each side)
		ovg.Ports[node] = make([]*OVGNode, 0, 12)
		for i, relativePoints := range node.SnapPointPercentages() {
			if err := guard.step(); err != nil {
				return err
			}
			for _, point := range relativePoints {
				if err := guard.step(); err != nil {
					return err
				}
				port, err := guard.addPoint(ovg, geo.NewPoint(node.TopLeft.X+math.Round(node.Width*point.XPercentage), node.TopLeft.Y+math.Round(node.Height*point.YPercentage)))
				if err != nil {
					return err
				}
				ovg.Ports[node] = append(ovg.Ports[node], port)
				var direction geo.Orientation
				if i == 0 {
					direction = geo.Top
				} else if i == 1 {
					direction = geo.Left
				} else if i == 2 {
					direction = geo.Bottom
				} else if i == 3 {
					direction = geo.Right
				}
				port.addPortOwner(node, direction, false)
			}
		}

		ports := ovg.Ports[node]
		for _, index := range node.CenterPortIndices() {
			if err := guard.step(); err != nil {
				return err
			}
			ports[index].setCenterPort(node)
		}
	}
	return guard.check()
}

// For all OVG Nodes added from ports, take their x/y cartesian product to create a grid of OVG Nodes
func (ovg *OVG) addNodesIntersections(g *layoutgraph.Graph, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	xCandidates := make(map[float64]struct{})
	yCandidates := make(map[float64]struct{})

	for _, node := range ovg.Nodes {
		if err := guard.step(); err != nil {
			return err
		}
		xCandidates[node.Point.X] = struct{}{}
		yCandidates[node.Point.Y] = struct{}{}
	}

	return ovg.addIntersections(g, xCandidates, yCandidates, guard)
}

func (ovg *OVG) addIntersections(g *layoutgraph.Graph, xs, ys map[float64]struct{}, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	candidateCount, err := checkedOVGIntersectionCount(len(xs), len(ys))
	if err != nil {
		return err
	}
	// Reserve the complete Cartesian product before allocating its orderings or
	// entering the loop. This makes a limit failure deterministic and cheap.
	if err := guard.reserveCandidates(candidateCount); err != nil {
		return err
	}
	if err := guard.reserveWork(candidateCount); err != nil {
		return err
	}

	xOrder := make([]float64, 0, len(xs))
	for x := range xs {
		if err := guard.check(); err != nil {
			return err
		}
		xOrder = append(xOrder, x)
	}
	if err := guard.reserveSortWork(len(xOrder)); err != nil {
		return err
	}
	slices.Sort(xOrder)

	yOrder := make([]float64, 0, len(ys))
	for y := range ys {
		if err := guard.check(); err != nil {
			return err
		}
		yOrder = append(yOrder, y)
	}
	if err := guard.reserveSortWork(len(yOrder)); err != nil {
		return err
	}
	slices.Sort(yOrder)

	graphNodes := layoutgraph.Nodes(g.Nodes)
	fixedOverlaps, err := ovg.fixedOverlapsForBuild(g, graphNodes, guard)
	if err != nil {
		return err
	}
	portIndex, err := newOVGPortIndex(ovg.Ports, g, fixedOverlaps, guard)
	if err != nil {
		return err
	}
	proximityIndex, err := newOVGPointProximityIndex(g, xOrder, guard)
	if err != nil {
		return err
	}

	// TODO don't have cross section of port nodes from container to descendants
	// Cross section of all the port nodes
	for _, px := range xOrder {
		if err := guard.check(); err != nil {
			return err
		}
		for _, py := range yOrder {
			if err := guard.check(); err != nil {
				return err
			}
			// Can't be too close to any port or node
			tooClose, err := portIndex.tooClose(px, py, layoutgraph.MinPortClearance, guard)
			if err != nil {
				return err
			}
			if tooClose {
				continue
			}
			// Charge candidate inspection before doing any geometry, but keep the
			// candidate on the stack until it survives both rejection filters.
			if err := guard.step(); err != nil {
				return err
			}
			candidatePoint := geo.Point{X: px, Y: py}
			nearNode, err := proximityIndex.pointNear(px, py, guard)
			if err != nil {
				return err
			}
			if nearNode {
				continue
			}
			candidate := OVGNode{Point: &candidatePoint}
			unobstructed, err := candidate.hasUnobstructedLineToPorts(ovg, portIndex, 2, guard)
			if err != nil {
				return err
			}
			if unobstructed {
				n := NewOVGNode(geo.NewPoint(px, py))
				if _, err := guard.addNode(ovg, n); err != nil {
					return err
				}
			}
		}
	}
	return guard.check()
}

// For each edge in tala.Edges, add nodes on the edge perimeter (bounding box) and half-way through the bounding box
func (ovg *OVG) addEdgesNodes(g *layoutgraph.Graph, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	graphNodes := layoutgraph.Nodes(g.Nodes)
	fixedOverlaps, err := ovg.fixedOverlapsForBuild(g, graphNodes, guard)
	if err != nil {
		return err
	}
	portIndex, err := newOVGPortIndex(ovg.Ports, g, fixedOverlaps, guard)
	if err != nil {
		return err
	}
	for _, edge := range g.Edges {
		if err := guard.step(); err != nil {
			return err
		}
		if edge.IsLoop() {
			continue
		}
		if edge.From.Hierarchy != nil && edge.To.Hierarchy != nil && edge.To.Hierarchy == edge.From.Hierarchy {
			// if the edge is between two nodes in the same hierarchy, it was already handled
			// by the hierarchical OVG routine
			// though, there can be edges between nodes in two different hierarchies
			// in that case, these points must be created
			continue
		}
		fillPoints, err := ovg.perimeterPoints(edge.From, edge.To, guard)
		if err != nil {
			return err
		}
		halfwayPoints, err := ovg.halfwayPoints(edge.From, edge.To, guard)
		if err != nil {
			return err
		}
		fillPoints = append(fillPoints, halfwayPoints...)
		for _, fillPoint := range fillPoints {
			if err := guard.step(); err != nil {
				return err
			}
			if err := guard.step(); err != nil {
				return err
			}
			nearNode, err := guard.pointNearGraphNode(g, fillPoint)
			if err != nil {
				return err
			}
			if nearNode {
				continue
			}
			candidate := OVGNode{Point: fillPoint}
			unobstructed, err := candidate.hasUnobstructedLineToPorts(ovg, portIndex, 1, guard)
			if err != nil {
				return err
			}
			if unobstructed {
				fillPointNode := NewOVGNode(fillPoint)
				if _, err := guard.addNode(ovg, fillPointNode); err != nil {
					return err
				}
			}
		}
	}
	return guard.check()
}

func (ovg *OVG) perimeterPoints(node, otherNode *layoutgraph.Node, guard *ovgBuildGuard) ([]*geo.Point, error) {
	fillPoints := make([]*geo.Point, 0)
	// Perimeter points
	top := math.Min(node.TopLeft.Y, otherNode.TopLeft.Y) - overshootAmount
	bottom := math.Max(node.TopLeft.Y+node.Height, otherNode.TopLeft.Y+otherNode.Height) + overshootAmount
	left := math.Min(node.TopLeft.X, otherNode.TopLeft.X) - overshootAmount
	right := math.Max(node.TopLeft.X+node.Width, otherNode.TopLeft.X+otherNode.Width) + overshootAmount

	for _, graphNode := range []*layoutgraph.Node{node, otherNode} {
		if err := guard.step(); err != nil {
			return nil, err
		}
		topPorts, err := guard.portsByOrientation(ovg, graphNode, geo.Top)
		if err != nil {
			return nil, err
		}
		for _, portNode := range topPorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(portNode.Point.X, top))
		}
		bottomPorts, err := guard.portsByOrientation(ovg, graphNode, geo.Bottom)
		if err != nil {
			return nil, err
		}
		for _, portNode := range bottomPorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(portNode.Point.X, bottom))
		}
		leftPorts, err := guard.portsByOrientation(ovg, graphNode, geo.Left)
		if err != nil {
			return nil, err
		}
		for _, portNode := range leftPorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(left, portNode.Point.Y))
		}
		rightPorts, err := guard.portsByOrientation(ovg, graphNode, geo.Right)
		if err != nil {
			return nil, err
		}
		for _, portNode := range rightPorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(right, portNode.Point.Y))
		}
	}

	fillPoints = append(fillPoints, geo.NewPoint(left, top))
	fillPoints = append(fillPoints, geo.NewPoint(right, top))
	fillPoints = append(fillPoints, geo.NewPoint(left, bottom))
	fillPoints = append(fillPoints, geo.NewPoint(right, bottom))
	return fillPoints, guard.check()
}

// add points halfway between node and otherNode
func (ovg *OVG) halfwayPoints(node, otherNode *layoutgraph.Node, guard *ovgBuildGuard) ([]*geo.Point, error) {
	fillPoints := make([]*geo.Point, 0)
	orientation := node.Orientation(otherNode)
	if orientation == geo.NONE {
		return fillPoints, guard.check()
	}
	// Halfway points
	// Fill from node to otherNode, which means if node is to the left of otherNode, we are filling rightwards, from node
	fillTop := false
	fillRight := false
	fillBottom := false
	fillLeft := false
	switch orientation {
	case geo.Top:
		fillBottom = true
	case geo.TopLeft:
		fillBottom = true
		fillRight = true
	case geo.TopRight:
		fillBottom = true
		fillLeft = true
	case geo.Bottom:
		fillTop = true
	case geo.BottomLeft:
		fillTop = true
		fillRight = true
	case geo.BottomRight:
		fillTop = true
		fillLeft = true
	case geo.Left:
		fillRight = true
	case geo.Right:
		fillLeft = true
	}

	nodeTL := node.TopLeft.Copy()
	nodeBR := geo.NewPoint(math.Round(nodeTL.X+node.Width), math.Round(nodeTL.Y+node.Height))

	otherNodeTL := otherNode.TopLeft.Copy()
	otherNodeBR := geo.NewPoint(math.Round(otherNodeTL.X+otherNode.Width), math.Round(otherNodeTL.Y+otherNode.Height))

	// all cluster nodes should use the same halfway points based on the whole cluster
	if node.Cluster != nil {
		var err error
		nodeTL, nodeBR, err = guard.tightBoundingBox(layoutgraph.Nodes(node.Cluster.Nodes))
		if err != nil {
			return nil, err
		}
	}
	if otherNode.Cluster != nil {
		var err error
		otherNodeTL, otherNodeBR, err = guard.tightBoundingBox(layoutgraph.Nodes(otherNode.Cluster.Nodes))
		if err != nil {
			return nil, err
		}
	}

	nodePorts := ovg.Ports[node]
	otherNodePorts := ovg.Ports[otherNode]
	if node.Sequence != nil {
		var err error
		nodeTL, nodeBR, err = guard.tightBoundingBox(layoutgraph.Nodes(node.Sequence.Nodes))
		if err != nil {
			return nil, err
		}
		for _, n := range node.Sequence.Nodes {
			if err := guard.step(); err != nil {
				return nil, err
			}
			if n != node {
				nodePorts = append(nodePorts, ovg.Ports[n]...)
			}
		}
	}
	if otherNode.Sequence != nil {
		var err error
		otherNodeTL, otherNodeBR, err = guard.tightBoundingBox(layoutgraph.Nodes(otherNode.Sequence.Nodes))
		if err != nil {
			return nil, err
		}
		for _, n := range otherNode.Sequence.Nodes {
			if err := guard.step(); err != nil {
				return nil, err
			}
			if n != otherNode {
				otherNodePorts = append(otherNodePorts, ovg.Ports[n]...)
			}
		}
	}

	if fillTop {
		y := (nodeTL.Y + otherNodeBR.Y) / 2
		for _, portNode := range nodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(portNode.Point.X, y))
		}
		for _, portNode := range otherNodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(portNode.Point.X, y))
		}
	}
	if fillBottom {
		y := (otherNodeTL.Y + nodeBR.Y) / 2
		for _, portNode := range nodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(portNode.Point.X, y))
		}
		for _, portNode := range otherNodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(portNode.Point.X, y))
		}
	}
	if fillLeft {
		x := (nodeTL.X + otherNodeBR.X) / 2
		for _, portNode := range nodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(x, portNode.Point.Y))
		}
		for _, portNode := range otherNodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(x, portNode.Point.Y))
		}
	}
	if fillRight {
		x := (otherNodeTL.X + nodeBR.X) / 2
		for _, portNode := range nodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(x, portNode.Point.Y))
		}
		for _, portNode := range otherNodePorts {
			if err := guard.step(); err != nil {
				return nil, err
			}
			fillPoints = append(fillPoints, geo.NewPoint(x, portNode.Point.Y))
		}
	}

	// These are the corner points for the halfway fill points
	if fillRight && fillBottom {
		x := (otherNodeTL.X + nodeBR.X) / 2
		y := (otherNodeTL.Y + nodeBR.Y) / 2
		fillPoints = append(fillPoints, geo.NewPoint(x, y))
	}

	if fillRight && fillTop {
		x := (otherNodeTL.X + nodeBR.X) / 2
		y := (nodeTL.Y + otherNodeBR.Y) / 2
		fillPoints = append(fillPoints, geo.NewPoint(x, y))
	}

	if fillLeft && fillTop {
		x := (nodeTL.X + otherNodeBR.X) / 2
		y := (nodeTL.Y + otherNodeBR.Y) / 2
		fillPoints = append(fillPoints, geo.NewPoint(x, y))
	}

	if fillLeft && fillBottom {
		x := (nodeTL.X + otherNodeBR.X) / 2
		y := (otherNodeTL.Y + nodeBR.Y) / 2
		fillPoints = append(fillPoints, geo.NewPoint(x, y))
	}
	return fillPoints, guard.check()
}

func (ovg *OVG) addTreeNodes(g *layoutgraph.Graph, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	portArrowheads := make(map[*OVGNode]map[layoutgraph.Arrowhead]struct{})
	// If there are trees in this subgraph, we need to add ovg nodes for the specific tree edge routes
	for _, node := range g.Nodes {
		if err := guard.step(); err != nil {
			return err
		}
		if tree, has := g.NodeToTree[node]; has {
			treePath, err := treeEdgePathForBuild(tree, ovg.Ports, portArrowheads, guard)
			if err != nil {
				return err
			}

			if _, err := guard.addPoint(ovg, treePath.SourceMidpoint); err != nil {
				return err
			}
			if _, err := guard.addPoint(ovg, treePath.TargetMidpoint); err != nil {
				return err
			}
			// if the shape centers are misaligned, treePath may use a new parent-aligned port so we add it here
			if isSentinelEdgeSource(tree) {
				port, err := guard.addNode(ovg, treePath.SourcePortNode)
				if err != nil {
					return err
				}
				if !port.isPortOf(node) {
					ovg.Ports[node] = append(ovg.Ports[node], port)
					port.addPortOwner(node, treePath.SourceOrientationToTarget.GetOpposite(), false)
				}

				if _, has := portArrowheads[treePath.TargetPortNode]; !has {
					portArrowheads[treePath.TargetPortNode] = make(map[layoutgraph.Arrowhead]struct{})
				}
				portArrowheads[treePath.TargetPortNode][tree.SentinelEdge.TargetArrowhead] = struct{}{}
			} else {
				port, err := guard.addNode(ovg, treePath.TargetPortNode)
				if err != nil {
					return err
				}
				if !port.isPortOf(node) {
					ovg.Ports[node] = append(ovg.Ports[node], port)
					port.addPortOwner(node, treePath.SourceOrientationToTarget, false)
				}

				if _, has := portArrowheads[treePath.SourcePortNode]; !has {
					portArrowheads[treePath.SourcePortNode] = make(map[layoutgraph.Arrowhead]struct{})
				}
				portArrowheads[treePath.SourcePortNode][tree.SentinelEdge.SourceArrowhead] = struct{}{}
			}
		}
	}
	return guard.check()
}

// addNewBoundaryLayers adds new nodes at the graph boundaries
// For example, take the graph boundary below
// .	┌──*───────────────*──────┐
// .	│                         │
// .	│                         │
// .	│                         │
// .	*                         │
// .	│                         *
// .	│                         │
// .	│                         │
// .	│                         *
// .	│                         │
// .	*                         │
// .	│                         *
// .	│                         │
// .	└─────────────*───────────┘
// Results in (just the first layer, the routine will add some more)
// .	┌────*───────────────*────────┐
// .	│ ┌──*───────────────*──────┐ │
// .	│ │                         │ │
// .	│ │                         │ │
// .	│ │                         │ │
// .	* *                         │ │
// .	│ │                         * *
// .	│ │                         │ │
// .	│ │                         │ │
// .	│ │                         * *
// .	│ │                         │ │
// .	* *                         │ │
// .	│ │                         * *
// .	│ │                         │ │
// .	│ └─────────────*───────────┘ │
// .	└───────────────*─────────────┘
func (ovg *OVG) addNewBoundaryLayers(g *layoutgraph.Graph, tl, br *geo.Point, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	// Don't count containers for children obstruction
	// Pad boundaries with points to allow edges to reach around
	for i := 0.0; i < extraInterestingPointLayers; i++ {
		if err := guard.step(); err != nil {
			return err
		}
		for _, node := range ovg.Nodes {
			if err := guard.step(); err != nil {
				return err
			}
			var point *geo.Point
			if node.Point.Y == tl.Y {
				point = geo.NewPoint(node.Point.X, node.Point.Y-(ovgPadding*(i+1)))
			}
			if node.Point.Y == br.Y {
				point = geo.NewPoint(node.Point.X, node.Point.Y+(ovgPadding*(i+1)))
			}
			if node.Point.X == tl.X {
				point = geo.NewPoint(node.Point.X-(ovgPadding*(i+1)), node.Point.Y)
			}
			if node.Point.X == br.X {
				point = geo.NewPoint(node.Point.X+(ovgPadding*(i+1)), node.Point.Y)
			}

			if point != nil {
				nearNode, err := guard.pointNearGraphNode(g, point)
				if err != nil {
					return err
				}
				if !nearNode {
					if _, err := guard.addPoint(ovg, point); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// For each given OVG node, try to add a new node at the graph boundaries that make a direct connection to this node
func (ovg *OVG) addPortConnectionNodesAtBoundaries(g *layoutgraph.Graph, tl, br *geo.Point, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	// Don't count containers for children obstruction
	// Pad boundaries with points to allow edges to reach around
	for i := 0.0; i < extraInterestingPointLayers; i++ {
		if err := guard.step(); err != nil {
			return err
		}
		for node, ports := range ovg.Ports {
			if err := guard.step(); err != nil {
				return err
			}
			added := false
			seenPorts := make(map[*OVGNode]struct{}, len(ports))
			for _, portNode := range ports {
				if err := guard.step(); err != nil {
					return err
				}
				if _, ok := seenPorts[portNode]; ok {
					continue
				}
				seenPorts[portNode] = struct{}{}
				portDirections, _ := portNode.portDirectionsFor(node)
				var directionErr error
				portDirections.any(func(portDirection geo.Orientation) bool {
					if err := guard.step(); err != nil {
						directionErr = err
						return true
					}
					topPoint := geo.NewPoint(portNode.Point.X, tl.Y-(ovgPadding*(i+1)))
					bottomPoint := geo.NewPoint(portNode.Point.X, br.Y+(ovgPadding*(i+1)))
					leftPoint := geo.NewPoint(tl.X-(ovgPadding*(i+1)), portNode.Point.Y)
					rightPoint := geo.NewPoint(br.X+(ovgPadding*(i+1)), portNode.Point.Y)

					for _, boundaryPoint := range []*geo.Point{topPoint, bottomPoint, leftPoint, rightPoint} {
						if err := guard.step(); err != nil {
							directionErr = err
							return true
						}
						passesThroughARectangle := false
						for _, rectangle := range ovg.NodesInsideBoundingBox {
							if err := guard.step(); err != nil {
								directionErr = err
								return true
							}
							passesThrough, err := guard.passesThroughAllowingPorts(rectangle, portNode.Point, boundaryPoint, portDirection, ovg.Ports[rectangle])
							if err != nil {
								directionErr = err
								return true
							}
							if passesThrough {
								// We make exception for container
								isRectangleNodeContainer := false
								if rectangle.IsContainer() {
									descendant, err := guard.isDescendantOf(node, rectangle)
									if err != nil {
										directionErr = err
										return true
									}
									if descendant {
										isRectangleNodeContainer = true
									}
								}
								if !isRectangleNodeContainer {
									passesThroughARectangle = true
									break
								}
							}
						}
						if passesThroughARectangle {
							continue
						}
						nearNode, err := guard.pointNearGraphNode(g, boundaryPoint)
						if err != nil {
							directionErr = err
							return true
						}
						if !nearNode {
							if _, err := guard.addPoint(ovg, boundaryPoint); err != nil {
								directionErr = err
								return true
							}
							added = true
						}
					}
					return false
				})
				if directionErr != nil {
					return directionErr
				}
			}
			if !added {
				if err := ovg.createPortsConnectionsToBoundaries(node, ports, tl, br, guard); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (ovg *OVG) addCornerNodes(g *layoutgraph.Graph, tl, br *geo.Point, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	// Fill in the corners of boundary points
	for i := 0.0; i < extraInterestingPointLayers; i++ {
		if err := guard.step(); err != nil {
			return err
		}
		points := []*geo.Point{
			{X: tl.X - (ovgPadding * (i + 1)), Y: tl.Y - (ovgPadding * (i + 1))},
			{X: br.X + (ovgPadding * (i + 1)), Y: tl.Y - (ovgPadding * (i + 1))},
			{X: br.X + (ovgPadding * (i + 1)), Y: br.Y + (ovgPadding * (i + 1))},
			{X: tl.X - (ovgPadding * (i + 1)), Y: br.Y + (ovgPadding * (i + 1))},
		}

		for _, point := range points {
			nearNode, err := guard.pointNearGraphNode(g, point)
			if err != nil {
				return err
			}
			if !nearNode {
				if _, err := guard.addPoint(ovg, point); err != nil {
					return err
				}
			}
		}
	}
	return guard.check()
}

func (ovg *OVG) addTunnels(g *layoutgraph.Graph, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	tunnels, err := buildTunnels(g, guard)
	if err != nil {
		return err
	}
	for _, tunnel := range tunnels {
		if err := guard.step(); err != nil {
			return err
		}
		for _, entry := range []*TunnelEntry{tunnel.EntryA, tunnel.EntryB} {
			if err := guard.step(); err != nil {
				return err
			}
			candidate := entry.OVGNode
			canonical, err := guard.addNode(ovg, candidate)
			if err != nil {
				return err
			}
			if canonical != candidate {
				for owner, metadata := range candidate.portOwners() {
					if err := guard.step(); err != nil {
						return err
					}
					canonical.addPortMetadata(owner, metadata)
				}
				entry.OVGNode = canonical
			}
			entry.OVGNode.IsTunnel = true
			ovg.Ports[entry.Node] = append(ovg.Ports[entry.Node], entry.OVGNode)
		}

		if _, err := guard.connect(ovg, tunnel.EntryA.OVGNode, tunnel.EntryB.OVGNode); err != nil {
			return err
		}
	}
	return guard.check()
}

func (ovg *OVG) connectPortsToCenter(guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	for _, node := range ovg.NodesInsideBoundingBox {
		if err := guard.step(); err != nil {
			return err
		}
		centerNode, err := guard.newCandidateNode(node.Center())
		if err != nil {
			return err
		}
		ovg.Centers[node] = centerNode
		// We don't care that it's the center of an invisible node, since that's not special
		if !node.IsInvisible {
			centerNode.IsNodeCenter = true
		}
		if err := guard.addNodeUnchecked(ovg, centerNode); err != nil {
			return err
		}
		for _, portNode := range ovg.Ports[node] {
			if _, err := guard.connect(ovg, centerNode, portNode); err != nil {
				return err
			}
		}
	}
	return guard.check()
}

func (ovg *OVG) mapNodesToContainer(g *layoutgraph.Graph, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	dfsContainerOrder, err := guard.containerRDFSOrder(g, nil)
	if err != nil {
		return err
	}
	for _, ovgNode := range ovg.Nodes {
		if err := guard.step(); err != nil {
			return err
		}
		for _, container := range dfsContainerOrder {
			if err := guard.step(); err != nil {
				return err
			}
			if container.ContainsPointOnBox(ovgNode.Point) {
				ovgNode.Container = container
				break
			}
		}
	}
	return nil
}

// connectNodes connects all OVG nodes to the nearest node on the left, top, right and bottom if the edge
// doesn't go through any graph node.
//
// | This routine works as:
// | 1. Collects all nodes in each row (same Y coordinate) and column (same X coordinate)
// | 2. For each of these X/Y coordinates, sort the nodes from smallest to largest using the opposite coordinate
// | 	- Nodes on the same column (X coordinate) are sorted by the Y coordinate so they go from top to bottom
// | 3. Connect nodes in sequence (node[i] with node[i+1]) as these are the nearest ones in the orthogonal graph
// | 	- Only connect if the edge doesn't go through any other node in the Graph
//
// Based on Sweep Line Algorithm
// https://en.wikipedia.org/wiki/Sweep_line_algorithm
func (ovg *OVG) connectNodes(g *layoutgraph.Graph, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	if uint64(len(ovg.Edges)) > guard.edges {
		if err := guard.reserveEdges(uint64(len(ovg.Edges)) - guard.edges); err != nil {
			return err
		}
	}
	// Create edges
	horizontal := map[float64][]*OVGNode{}
	vertical := map[float64][]*OVGNode{}
	eligibleNodeCount := uint64(0)
	for _, node := range ovg.Nodes {
		if err := guard.step(); err != nil {
			return err
		}
		// Tunnels can only connect to each other (and to their node centers)
		if node.IsTunnel {
			continue
		}

		// sequence steps should only connect from certain sides
		restricted, err := guard.isRestrictedSequencePort(node)
		if err != nil {
			return err
		}
		if restricted {
			continue
		}

		eligibleNodeCount++
		horizontal[node.Y] = append(horizontal[node.Y], node)
		vertical[node.X] = append(vertical[node.X], node)
	}

	sortLines := func(lines map[float64][]*OVGNode) ([]float64, error) {
		var coords []float64
		for coord := range lines {
			if err := guard.step(); err != nil {
				return nil, err
			}
			coords = append(coords, coord)
		}
		if err := guard.reserveSortWork(len(coords)); err != nil {
			return nil, err
		}
		slices.Sort(coords)

		return coords, nil
	}

	// Each eligible node appears in one horizontal and one vertical sweep.
	// The two sweeps therefore add at most (n-h) + (n-v) edges.
	// *---*---*---*---*
	// |   |   |   |   |
	// *---*---*---*---*
	// |   |   |   |   |
	// *---*---*---*---*
	// |   |   |   |   |
	// *---*---*---*---*
	h := uint64(len(horizontal))
	v := uint64(len(vertical))
	edgeCapacity, err := checkedOVGEdgeCapacity(uint64(len(ovg.Edges)), eligibleNodeCount, h, v, guard.limits.edges, maxIntAsUint64())
	if err != nil {
		return err
	}
	// we need to copy existing edges (e.g., we already connect the nodes when adding tunnels)
	edges := make([]*OVGEdge, len(ovg.Edges), edgeCapacity)
	copy(edges, ovg.Edges)
	ovg.Edges = edges
	boundingNodes := layoutgraph.Nodes(ovg.NodesInsideBoundingBox)
	fixedOverlaps, err := ovg.fixedOverlapsForBuild(g, boundingNodes, guard)
	if err != nil {
		return err
	}
	horizontalLines, err := sortLines(horizontal)
	if err != nil {
		return err
	}
	for _, y := range horizontalLines {
		if err := guard.step(); err != nil {
			return err
		}
		if err := ovg.connectNodesOnSameLine(horizontal[y], y, true, fixedOverlaps, guard); err != nil {
			return err
		}
	}

	verticalLines, err := sortLines(vertical)
	if err != nil {
		return err
	}
	for _, x := range verticalLines {
		if err := guard.step(); err != nil {
			return err
		}
		if err := ovg.connectNodesOnSameLine(vertical[x], x, false, fixedOverlaps, guard); err != nil {
			return err
		}
	}
	return nil
}

// connectNodesOnSameLine connects nodes on the same horizontal/vertical line.
// The nodes MUST be on the same line, so they are sorted and the connection is just connecting to the next
// node in the list.
// See connectNodes for more details around the algorithm
func (ovg *OVG) connectNodesOnSameLine(nodes []*OVGNode, linePosition float64, isHorizontal bool, fixedOverlaps map[*layoutgraph.Node]struct{}, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}

	var intersectCandidates []*layoutgraph.Node
	for _, node := range ovg.NodesInsideBoundingBox {
		if err := guard.step(); err != nil {
			return err
		}
		if _, in := fixedOverlaps[node]; in {
			continue
		}
		if isHorizontal && node.TopLeft.Y <= linePosition && node.TopLeft.Y+node.Height >= linePosition {
			intersectCandidates = append(intersectCandidates, node)
		} else if !isHorizontal && node.TopLeft.X <= linePosition && node.TopLeft.X+node.Width >= linePosition {
			intersectCandidates = append(intersectCandidates, node)
		}
	}
	if err := guard.reserveSortWork(len(intersectCandidates)); err != nil {
		return err
	}

	sort.Slice(intersectCandidates, func(i, j int) bool {
		if isHorizontal {
			return intersectCandidates[i].TopLeft.X < intersectCandidates[j].TopLeft.X
		}
		return intersectCandidates[i].TopLeft.Y < intersectCandidates[j].TopLeft.Y
	})

	if err := guard.reserveSortWork(len(nodes)); err != nil {
		return err
	}
	sort.Slice(nodes, func(i, j int) bool {
		if isHorizontal {
			return nodes[i].X < nodes[j].X
		}
		return nodes[i].Y < nodes[j].Y
	})

	startIntersectionSearchAt := 0
	for i := 0; i < len(nodes)-1; i++ {
		if err := guard.step(); err != nil {
			return err
		}
		node := nodes[i]
		closest := nodes[i+1]
		misdirected, err := ovg.isMisdirectedPortPair(node, closest, isHorizontal, guard)
		if err != nil {
			return err
		}
		if misdirected {
			continue
		}

		shouldConnect := true
		for j := startIntersectionSearchAt; j < len(intersectCandidates); j++ {
			if err := guard.step(); err != nil {
				return err
			}
			rectangle := intersectCandidates[j]
			if (isHorizontal && rectangle.TopLeft.X > closest.X) || (!isHorizontal && rectangle.TopLeft.Y > closest.Y) {
				// here, rectangle is after closest to the right or to the bottom
				// since intersectCandidates is sorted, no other nodes will intersect, so we can skip the test
				break
			}

			passesThrough := func(from, to *OVGNode) (bool, error) {
				passes := true
				var directionErr error
				from.portDirectionsForObstacle(rectangle).any(func(direction geo.Orientation) bool {
					passesThrough, err := guard.passesThroughAllowingPorts(rectangle, from.Point, to.Point, direction, ovg.Ports[rectangle])
					if err != nil {
						directionErr = err
						return true
					}
					if !passesThrough {
						passes = false
						return true
					}
					return false
				})
				return passes, directionErr
			}
			c1, err := passesThrough(node, closest)
			if err != nil {
				return err
			}
			c2, err := passesThrough(closest, node)
			if err != nil {
				return err
			}
			if c1 && c2 {
				if rectangle.Sequence != nil {
					// sequences will have top and bottom ports overlapped by previous sequence nodes, but this overlap shouldn't prevent a connection with that port
					if !isHorizontal {
						if rectangle.TopLeft.Y == closest.Y || rectangle.TopLeft.Y+rectangle.Height == node.Y {
							continue
						}
					}
				}
				// Passing through a container is okay ...
				isContainerException := false
				if rectangle.IsContainer() {
					// ... if at least one of the nodes is within the container
					if rectangle.ContainsPointOnBox(node.Point) || rectangle.ContainsPointOnBox(closest.Point) {
						isContainerException = true
					}
				}

				if !isContainerException {
					shouldConnect = false
					break
				}
			} else {
				// this node didn't intersect with the current OVG, so we don't need to
				// check any other nodes prior to it for the next OVG nodes
				// (the loop is on top of sorted nodes)
				startIntersectionSearchAt = j
			}
		}

		if shouldConnect {
			if _, err := guard.connect(ovg, node, closest); err != nil {
				return err
			}
		}
	}
	return nil
}

// Note: assumes first node is left(IsHorizontal=true) or above(IsHorizontal=false) second
func (ovg *OVG) isMisdirectedPortPair(first, second *OVGNode, isHorizontal bool, guard *ovgBuildGuard) (bool, error) {
	if err := guard.check(); err != nil {
		return false, err
	}
	firstOwners := first.portOwners()
	secondOwners := second.portOwners()
	if len(firstOwners) == 0 || len(secondOwners) == 0 {
		return false, guard.check()
	}

	// A shared point can represent ports of several touching nodes. Reject the
	// connection only if every possible owner pair is misdirected; one valid
	// interpretation is enough to make the OVG segment usable.
	for firstOwner, firstMetadata := range firstOwners {
		if err := guard.step(); err != nil {
			return false, err
		}
		for secondOwner, secondMetadata := range secondOwners {
			if err := guard.step(); err != nil {
				return false, err
			}
			if firstOwner == secondOwner {
				return false, nil
			}

			// Since first is left/above second, at least one pair of roles must
			// face each other (not the other way around).
			correctDirections := firstMetadata.directions.has(geo.Bottom) && secondMetadata.directions.has(geo.Top)
			if isHorizontal {
				correctDirections = firstMetadata.directions.has(geo.Right) && secondMetadata.directions.has(geo.Left)
			}
			// ports must face each other unless it is a descendant and an ancestor
			firstBelowSecond, err := guard.isDescendantOf(firstOwner, secondOwner)
			if err != nil {
				return false, err
			}
			secondBelowFirst, err := guard.isDescendantOf(secondOwner, firstOwner)
			if err != nil {
				return false, err
			}
			if correctDirections || firstBelowSecond || secondBelowFirst {
				return false, nil
			}
		}
	}
	return true, guard.check()
}

// createPortsConnectionsToBoundaries will create paths for isolated graph nodes.
// If the ports of a given node don't have a clear path to the graph boundaries we might not be able to trace a route to this node.
// This function tries to fix this case by adding a padded port node and nodes at the graph boundaries that they can connect to.
//
// Take the example below where `n` ports don't have a clear path to the boudaries (only 1 shown here).
// .	┌─────────────────────────────────────────┐
// .	│  ┌──────┐       ┌──────┐      ┌──────┐  │
// .	│  │      │       │      │      │      │  │
// .	│  │      │       │      │      │      │  │
// .	│  │      │       │      │      │      │  │
// .	│  └──────┘       └──────┘      └──────┘  │
// .	│                                         │
// .	│                                         │
// .	│  ┌──────┐       ┌─*──*─┐      ┌──────┐  │
// .	│  │      │       *      *      │      │  │
// .	│  │      │       │   n  │      │      │  │
// .	│  │      │       *      *      │      │  │
// .	│  └──────┘       └─*──*─┘      └──────┘  │
// .	│                                         │
// .	│                                         │
// .	│  ┌──────┐       ┌──────┐      ┌──────┐  │
// .	│  │      │       │      │      │      │  │
// .	│  │      │       │      │      │      │  │
// .	│  │      │       │      │      │      │  │
// .	│  └──────┘       └──────┘      └──────┘  │
// .	└─────────────────────────────────────────┘
//
// In this case, route search won't be able to trace a route to `n`.
// To relax it a bit, we pad `n` ports and create some nodes at the graph boundaries that might turn into an OVG edge later on
// if there no other graph nodes in between them.
// Here is the expected result (only one boundary layer shown)
// . 	┌──────────────*────────────*─────────────┐
// . 	│  ┌──────┐       ┌──────┐      ┌──────┐  │
// . 	│  │      │       │      │      │      │  │
// . 	│  │      │       │      │      │      │  │
// . 	│  │      │       │      │      │      │  │
// . 	│  └──────┘       └──────┘      └──────┘  │
// . 	│                                         │
// . 	*                   *  *                  *
// . 	│  ┌──────┐       ┌─*──*─┐      ┌──────┐  │
// . 	│  │      │    *  *      *  *   │      │  │
// . 	│  │      │       │   n  │      │      │  │
// . 	│  │      │    *  *      *  *   │      │  │
// . 	│  └──────┘       └─*──*─┘      └──────┘  │
// . 	*                   *  *                  *
// . 	│                                         │
// . 	│  ┌──────┐       ┌──────┐      ┌──────┐  │
// . 	│  │      │       │      │      │      │  │
// . 	│  │      │       │      │      │      │  │
// . 	│  │      │       │      │      │      │  │
// . 	│  └──────┘       └──────┘      └──────┘  │
// . 	└──────────────*────────────*─────────────┘
func (ovg *OVG) createPortsConnectionsToBoundaries(owner *layoutgraph.Node, ports []*OVGNode, tl, br *geo.Point, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	seenPorts := make(map[*OVGNode]struct{}, len(ports))
	for _, port := range ports {
		if err := guard.step(); err != nil {
			return err
		}
		if _, ok := seenPorts[port]; ok {
			continue
		}
		seenPorts[port] = struct{}{}
		portDirections, _ := port.portDirectionsFor(owner)
		var directionErr error
		portDirections.any(func(portDirection geo.Orientation) bool {
			if err := guard.step(); err != nil {
				directionErr = err
				return true
			}
			var newPoint *geo.Point
			switch portDirection {
			case geo.Top:
				newPoint = geo.NewPoint(port.X, port.Y-layoutgraph.MinPortClearance)
			case geo.Bottom:
				newPoint = geo.NewPoint(port.X, port.Y+layoutgraph.MinPortClearance)
			case geo.Left:
				newPoint = geo.NewPoint(port.X-layoutgraph.MinPortClearance, port.Y)
			case geo.Right:
				newPoint = geo.NewPoint(port.X+layoutgraph.MinPortClearance, port.Y)
			default:
				return false
			}
			newNode, err := guard.addPoint(ovg, newPoint)
			directionErr = err
			if directionErr != nil {
				return true
			}

			// add nodes at the OVG boundary to connect with this new node
			for i := 1; i < extraInterestingPointLayers+1; i++ {
				if err := guard.step(); err != nil {
					directionErr = err
					return true
				}
				switch portDirection {
				case geo.Top, geo.Bottom:
					// left boundary
					if _, err := guard.addPoint(ovg, geo.NewPoint(tl.X-(ovgPadding*float64(i)), newNode.Y)); err != nil {
						directionErr = err
						return true
					}
					// right boundary
					if _, err := guard.addPoint(ovg, geo.NewPoint(br.X+(ovgPadding*float64(i)), newNode.Y)); err != nil {
						directionErr = err
						return true
					}
				case geo.Left, geo.Right:
					// top boundary
					if _, err := guard.addPoint(ovg, geo.NewPoint(newNode.X, tl.Y-(ovgPadding*float64(i)))); err != nil {
						directionErr = err
						return true
					}
					// bottom boundary
					if _, err := guard.addPoint(ovg, geo.NewPoint(newNode.X, br.Y+(ovgPadding*float64(i)))); err != nil {
						directionErr = err
						return true
					}
				}
			}
			return false
		})
		if directionErr != nil {
			return directionErr
		}
	}
	return guard.check()
}

// flagNodesNearPorts finds OVG Nodes that are lined up and too close to ports as they should be penalized during edge routing
// The algorithm works as:
// - for each port
// -   for each lined up adjacent
// -      if adj is not too close: continue
// -      else: flag adj as too close and follow its edges as other nodes can be too close
func (ovg *OVG) flagNodesNearPorts(guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	for owner, ports := range ovg.Ports {
		if err := guard.step(); err != nil {
			return err
		}
		for _, port := range ports {
			if err := guard.step(); err != nil {
				return err
			}
			// 5 is just a rough estimate for the port + 4 adjacent nodes
			seen := make(map[*OVGNode]struct{}, 5)
			seen[port] = struct{}{}
			queue := make([]*OVGNode, 0, 5)
			for _, e := range port.Edges {
				if err := guard.step(); err != nil {
					return err
				}
				adj := port.adjacent(e)
				if adj.X != port.X && adj.Y != port.Y {
					continue
				}
				queue = append(queue, adj)
			}
			for len(queue) > 0 {
				if err := guard.step(); err != nil {
					return err
				}
				curr := queue[0]
				queue = queue[1:]
				if _, exists := seen[curr]; exists {
					continue
				}
				seen[curr] = struct{}{}

				// only check non-ports, or ports of other nodes
				// but, if adj is a port of the current graph node, also follow its adjacents
				if !curr.isPortOf(owner) {
					if math.Abs(curr.Y-port.Y) >= nodeProximityThreshold {
						// the current node is not too close to the port, so no need to investigate the adjacents
						continue
					}
					if math.Abs(curr.X-port.X) >= nodeProximityThreshold {
						// the current node is not too close to the port, so no need to investigate the adjacents
						continue
					}

					if curr.IsNearPort == nil {
						curr.IsNearPort = make(map[*layoutgraph.Node]struct{})
					}
					// `curr` is near the port
					curr.IsNearPort[owner] = struct{}{}
				}

				// follow other nodes, as they be near too
				for _, e := range curr.Edges {
					if err := guard.step(); err != nil {
						return err
					}
					adj := curr.adjacent(e)
					if adj.X != port.X && adj.Y != port.Y {
						continue
					}
					queue = append(queue, adj)
				}
			}
		}
	}
	return guard.check()
}

type serializedOVGNode struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type serializedOVGEdge struct {
	From serializedOVGNode `json:"from"`
	To   serializedOVGNode `json:"to"`
}

type serializedOVG struct {
	Nodes []serializedOVGNode `json:"nodes"`
	Edges []serializedOVGEdge `json:"edges"`
}

var _ json.Marshaler = &OVG{}
var _ json.Unmarshaler = &OVG{}

func (g *OVG) MarshalJSON() ([]byte, error) {
	serialized := serializedOVG{}

	nodeOrder := func(n1, n2 *OVGNode) bool {
		if n1.X == n2.X {
			return n1.Y < n2.Y
		}
		return n1.X < n2.X
	}

	sortedNodes := make([]*OVGNode, len(g.Nodes))
	copy(sortedNodes, g.Nodes)
	sort.Slice(sortedNodes, func(i, j int) bool {
		return nodeOrder(sortedNodes[i], sortedNodes[j])
	})
	for _, node := range sortedNodes {
		serialized.Nodes = append(serialized.Nodes, serializedOVGNode{
			X: node.X,
			Y: node.Y,
		})
	}

	sortedEdges := make([]*OVGEdge, len(g.Edges))
	copy(sortedEdges, g.Edges)
	sort.Slice(sortedEdges, func(i, j int) bool {
		if sortedEdges[i].From == sortedEdges[j].From {
			return nodeOrder(sortedEdges[i].To, sortedEdges[j].To)
		}
		return nodeOrder(sortedEdges[i].From, sortedEdges[j].From)
	})
	for _, edge := range sortedEdges {
		serialized.Edges = append(serialized.Edges, serializedOVGEdge{
			From: serializedOVGNode{
				X: edge.From.X,
				Y: edge.From.Y,
			},
			To: serializedOVGNode{
				X: edge.To.X,
				Y: edge.To.Y,
			},
		})
	}
	return json.Marshal(serialized)
}

func (ovg *OVG) UnmarshalJSON(content []byte) error {
	var serialized serializedOVG
	if err := json.Unmarshal(content, &serialized); err != nil {
		return err
	}

	*ovg = *NewOVG(nil)
	positionToNode := make(map[geo.Point]*OVGNode)
	for _, node := range serialized.Nodes {
		n := NewOVGNode(geo.NewPoint(node.X, node.Y))
		ovg.AddNodeUnchecked(n)
		positionToNode[*n.Point] = n
	}

	for _, edge := range serialized.Edges {
		from := *geo.NewPoint(edge.From.X, edge.From.Y)
		to := *geo.NewPoint(edge.To.X, edge.To.Y)

		fromNode := positionToNode[from]
		toNode := positionToNode[to]
		if fromNode == nil || toNode == nil {
			return fmt.Errorf("OVG edge references a missing endpoint: %v -> %v", from, to)
		}
		ovg.Connect(fromNode, toNode)
	}

	return nil
}

func (ovg *OVG) mergePorts(other *OVG, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	for n, ports := range other.Ports {
		if err := guard.reserveWork(uint64(len(ports)) + 1); err != nil {
			return err
		}
		canonicalPorts := make([]*OVGNode, len(ports))
		for i, port := range ports {
			canonical, err := guard.addNode(ovg, port)
			if err != nil {
				return err
			}
			if canonical != port {
				for owner, metadata := range port.portOwners() {
					if err := guard.step(); err != nil {
						return err
					}
					canonical.addPortMetadata(owner, metadata)
				}
			}
			canonicalPorts[i] = canonical
		}
		ovg.Ports[n] = canonicalPorts
	}
	if err := guard.reserveWork(uint64(len(other.NodesInsideBoundingBox))); err != nil {
		return err
	}
	ovg.NodesInsideBoundingBox = append(ovg.NodesInsideBoundingBox, other.NodesInsideBoundingBox...)
	return guard.check()
}

func (ovg *OVG) mergeNonPorts(other *OVG, guard *ovgBuildGuard) error {
	if err := guard.check(); err != nil {
		return err
	}
	for _, n := range other.Nodes {
		if err := guard.step(); err != nil {
			return err
		}
		if !n.isPort() {
			if _, err := guard.addNode(ovg, n); err != nil {
				return err
			}
		}
	}
	return guard.check()
}
