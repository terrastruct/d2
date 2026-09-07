package loops

import (
	"math"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

const sharedPortLoopGap = 30.0

// ComputeOffsets refreshes loop extents for every graph node.
func ComputeOffsets(g *layoutgraph.Graph) {
	for _, node := range g.Nodes {
		UpdateOffsets(node)
	}
}

// UpdateOffsets refreshes the space reserved around node for self-edge routes.
func UpdateOffsets(node *layoutgraph.Node) {
	// need to set node TopLeft because the node position is required to get the ports
	// which are required for routing
	tlIsNil := node.TopLeft == nil
	if tlIsNil {
		node.TopLeft = geo.NewPoint(0, 0)
	}
	computeOffsets(node)
	if tlIsNil {
		node.TopLeft = nil
	}
}

// computeLoopOffsets precomputes loops and their contribution to node bounds
// for later delta and bounding-box calculations.
func computeOffsets(node *layoutgraph.Node) {
	node.LoopOffsets = make(map[geo.Orientation]float64)

	tl, br := node.BoundingBox(nil)
	loopTL, loopBR := node.BoundingBox(nil)
	for _, edge := range routeLoops(node) {
		if !edge.IsLoop() {
			continue
		}
		eTL, eBR := edge.BoundingBoxValues()
		loopTL.X = math.Min(loopTL.X, eTL.X)
		loopTL.Y = math.Min(loopTL.Y, eTL.Y)
		loopBR.X = math.Max(loopBR.X, eBR.X)
		loopBR.Y = math.Max(loopBR.Y, eBR.Y)
		// need to reset because if these are set based on `0, 0`, graph bounding box breaks during placement
		// as nodes move around during placement and these edge points would stay at `0, 0`
		// the graph would reach a bad state
		edge.Points = nil
	}

	if nonNilEquals(tl, loopTL) && nonNilEquals(br, loopBR) {
		return
	}

	node.LoopOffsets[geo.Left] = math.Round(math.Abs(tl.X - loopTL.X))
	node.LoopOffsets[geo.Right] = math.Round(math.Abs(br.X - loopBR.X))
	node.LoopOffsets[geo.Top] = math.Round(math.Abs(tl.Y - loopTL.Y))
	node.LoopOffsets[geo.Bottom] = math.Round(math.Abs(br.Y - loopBR.Y))

	node.LoopOffsets[geo.TopLeft] = math.Max(node.LoopOffsets[geo.Top], node.LoopOffsets[geo.Left])
	node.LoopOffsets[geo.TopRight] = math.Max(node.LoopOffsets[geo.Top], node.LoopOffsets[geo.Right])
	node.LoopOffsets[geo.BottomLeft] = math.Max(node.LoopOffsets[geo.Bottom], node.LoopOffsets[geo.Left])
	node.LoopOffsets[geo.BottomRight] = math.Max(node.LoopOffsets[geo.Bottom], node.LoopOffsets[geo.Right])
}

// loopPorts works as sort of context for a given ports pair while routing loops
type loopPorts struct {
	verticalOffset     float64
	horizontalOffset   float64
	hasSourceArrowhead *bool
	hasTargetArrowhead *bool
	sourceArrowhead    layoutgraph.Arrowhead
	targetArrowhead    layoutgraph.Arrowhead
	from               *geo.Point
	fromDirection      geo.Orientation
	to                 *geo.Point
	toDirection        geo.Orientation
}

func newLoopPorts(from, to *geo.Point, fromDirection, toDirection geo.Orientation) *loopPorts {
	return &loopPorts{
		verticalOffset:     0.,
		horizontalOffset:   0.,
		hasSourceArrowhead: nil,
		hasTargetArrowhead: nil,
		from:               from,
		fromDirection:      fromDirection,
		to:                 to,
		toDirection:        toDirection,
	}
}

// Route computes every self-edge route for node.
func Route(n *layoutgraph.Node) []*layoutgraph.Edge {
	return routeLoops(n)
}

// routeLoops routes loops in a rule based approach and sets the proper label position
// edges are routed based on their arrowheads and label size
// there are 4 possible positions for loops, one at each corner
func routeLoops(n *layoutgraph.Node) []*layoutgraph.Edge {
	availablePorts := makeLoopPorts(n)

	var routedEdges []*layoutgraph.Edge
	for _, e := range edgesInOrder(n.Edges) {
		if !e.IsLoop() {
			continue
		}

		routedEdges = append(routedEdges, e)
		portsPair := findLoopPortsForEdge(e, availablePorts)
		e.Points = routeLoop(portsPair)
		if e.Label != nil {
			e.Label.Position = label.OutsideTopCenter
		}
		portsPair.verticalOffset, portsPair.horizontalOffset = computeEdgeOffset(e)
		portsPair.hasSourceArrowhead = new(e.HasSourceArrow())
		portsPair.hasTargetArrowhead = new(e.HasTargetArrow())
		portsPair.sourceArrowhead = e.SourceArrowhead
		portsPair.targetArrowhead = e.TargetArrowhead
	}
	return routedEdges
}

// edgesInOrder sorts edges based on their arrowheads and label sizes
// sorting by arrowhead is required so that, if a given `diamond` node has all edge types (->, --, <->)
// the arrowheads will match the ports as expected (see `self_edges_all_shapes` test case)
// then, sorts by label so that the smallest ones will use the routes closes to the shape, avoiding empty space
func edgesInOrder(edges []*layoutgraph.Edge) []*layoutgraph.Edge {
	sortedEdges := make([]*layoutgraph.Edge, len(edges))
	copy(sortedEdges, edges)
	arrowheadScore := func(e *layoutgraph.Edge) int {
		// ->
		if e.HasSourceArrow() != e.HasTargetArrow() {
			return 0
		} else if e.HasSourceArrow() {
			return 1
		} else {
			return 2
		}
	}
	slices.SortStableFunc(sortedEdges, func(a, b *layoutgraph.Edge) int {
		// 1st ->
		// 2nd <->
		// 3rd --
		if a.HasSourceArrow() != b.HasSourceArrow() || a.HasTargetArrow() != b.HasTargetArrow() {
			aScore := arrowheadScore(a)
			bScore := arrowheadScore(b)
			switch {
			case aScore < bScore:
				return -1
			case bScore < aScore:
				return 1
			default:
				return 0
			}
		}

		// if arrowheads are equal, sort by label area
		aArea := 0.
		bArea := 0.
		if a.Label != nil {
			aArea = a.Label.Width * a.Label.Height
		}
		if b.Label != nil {
			bArea = b.Label.Width * b.Label.Height
		}
		switch {
		case aArea < bArea:
			return -1
		case bArea < aArea:
			return 1
		default:
			return 0
		}
	})
	return sortedEdges
}

// findLoopPortsForEdge finds the ports that are using the same arrowheads in `availablePorts`
func findLoopPortsForEdge(e *layoutgraph.Edge, availablePorts []*loopPorts) *loopPorts {
	// first find a matching set if it exists
	for _, ports := range availablePorts {
		if ports.hasSourceArrowhead != nil && ports.hasTargetArrowhead != nil {
			if ports.sourceArrowhead == e.SourceArrowhead && ports.targetArrowhead == e.TargetArrowhead {
				return ports
			}
		}
	}
	// look for an open pair
	for _, ports := range availablePorts {
		if ports.hasSourceArrowhead == nil && ports.hasTargetArrowhead == nil {
			return ports
		}
	}
	// find a pair that have arrows at the same end
	for _, ports := range availablePorts {
		if ports.hasSourceArrowhead != nil && ports.hasTargetArrowhead != nil &&
			*ports.hasSourceArrowhead == e.HasSourceArrow() && *ports.hasTargetArrowhead == e.HasTargetArrow() {
			return ports
		}
	}
	return nil
}

// makeLoopPorts makes the 4 possible port pairs, one at each corner, using the closest ports for each combination
// closest ports because this avoids crossings with other routes
// the ports order is related to edgesInOrder and the `shape: diamond` condition (see test case)
// Four pairs are required because the declaration orientation makes source-only
// and target-only arrows distinct categories in addition to -- and <->.
func makeLoopPorts(n *layoutgraph.Node) []*loopPorts {
	ports := n.Ports()
	topIndices := n.PortIndices(geo.Top)
	rightIndices := n.PortIndices(geo.Right)
	bottomIndices := n.PortIndices(geo.Bottom)
	leftIndices := n.PortIndices(geo.Left)

	trTop, trRight := closestPorts(ports, topIndices, rightIndices)
	blBottom, blLeft := closestPorts(ports, bottomIndices, leftIndices)
	ltLeft, ltTop := closestPorts(ports, leftIndices, topIndices)
	brRight, brBottom := closestPorts(ports, rightIndices, bottomIndices)

	return []*loopPorts{
		newLoopPorts(ltLeft, ltTop, geo.Left, geo.Top),
		newLoopPorts(trTop, trRight, geo.Top, geo.Right),
		newLoopPorts(blBottom, blLeft, geo.Bottom, geo.Left),
		newLoopPorts(brRight, brBottom, geo.Right, geo.Bottom),
	}
}

func closestPorts(ports []geo.Point, sideA, sideB []int) (*geo.Point, *geo.Point) {
	var closestA, closestB geo.Point
	minDistance := math.Inf(1)

	for _, indexA := range sideA {
		for _, indexB := range sideB {
			pa := ports[indexA]
			pb := ports[indexB]
			d := geo.EuclideanDistance(pa.X, pa.Y, pb.X, pb.Y)
			if d < minDistance {
				minDistance = d
				closestA = pa
				closestB = pb
			}
		}
	}

	return &closestA, &closestB
}

// routeLoop creates the route shown below with ports `*`, bend points and intersection (`#`)
// . #───────────#
// . │           │
// . │      ┌────*───┐
// . │      │        │
// . #──────*        │
// .        │  node  │
// .        │        │
// .        └────────┘
func routeLoop(ports *loopPorts) []*geo.Point {
	p1 := makeBendPoint(ports.from, ports.fromDirection, ports.verticalOffset, ports.horizontalOffset)
	p2 := makeBendPoint(ports.to, ports.toDirection, ports.verticalOffset, ports.horizontalOffset)
	intersection := makeIntersectionPoint(p1, p2, ports.fromDirection)

	return []*geo.Point{
		ports.from,
		p1,
		intersection,
		p2,
		ports.to,
	}
}

func makeBendPoint(port *geo.Point, direction geo.Orientation, verticalOffset, horizontalOffset float64) *geo.Point {
	switch direction {
	case geo.Top:
		return geo.NewPoint(port.X, port.Y-verticalOffset-sharedPortLoopGap)
	case geo.Bottom:
		return geo.NewPoint(port.X, port.Y+verticalOffset+sharedPortLoopGap)
	case geo.Left:
		return geo.NewPoint(port.X-horizontalOffset-sharedPortLoopGap, port.Y)
	case geo.Right:
		return geo.NewPoint(port.X+horizontalOffset+sharedPortLoopGap, port.Y)
	}
	return nil
}

func makeIntersectionPoint(from, to *geo.Point, fromDirection geo.Orientation) *geo.Point {
	if !fromDirection.IsVertical() {
		return geo.NewPoint(from.X, to.Y)
	}
	return geo.NewPoint(to.X, from.Y)
}

// computeEdgeOffset computes the offset to place the next loop considering the current `edge` and its label
func computeEdgeOffset(edge *layoutgraph.Edge) (float64, float64) {
	var verticalOffset float64
	var horizontalOffset float64
	points := edge.Points
	if edge.Points[0].X == edge.Points[1].X {
		verticalOffset = math.Abs(points[0].Y - points[1].Y)
		horizontalOffset = math.Abs(points[len(points)-1].X - points[len(points)-2].X)
	} else {
		verticalOffset = math.Abs(points[len(points)-1].Y - points[len(points)-2].Y)
		horizontalOffset = math.Abs(points[0].X - points[1].X)
	}

	if edge.Label != nil {
		verticalOffset += edge.Label.Height
		horizontalOffset += edge.Label.Width
	}
	return verticalOffset, horizontalOffset
}
