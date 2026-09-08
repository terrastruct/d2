package layoutgraph

import "github.com/d2lang/d2/lib/geo"

// RoutingCostState is the cached routing-cost state that an atomic routing
// stage must restore on failure.
type RoutingCostState struct {
	Crossing      float64
	Turn          float64
	NonCenterPort float64
}

func (graph *Graph) RoutingCosts() RoutingCostState {
	graph.costMu.RLock()
	defer graph.costMu.RUnlock()
	return RoutingCostState{
		Crossing:      graph.crossingCost,
		Turn:          graph.turnCost,
		NonCenterPort: graph.nonCenterPortCost,
	}
}

func (graph *Graph) RestoreRoutingCosts(state RoutingCostState) {
	graph.costMu.Lock()
	defer graph.costMu.Unlock()
	graph.crossingCost = state.Crossing
	graph.turnCost = state.Turn
	graph.nonCenterPortCost = state.NonCenterPort
}

func (segment *EdgeSegment) Owner() *Edge { return segment.edge }

func (graph *Graph) CrossingCost() float64      { return graph.crossingCostValue() }
func (graph *Graph) NonCenterPortCost() float64 { return graph.nonCenterPortCostValue() }

func (edge *Edge) IDValue() EntityID                           { return edge.entityID() }
func (edge *Edge) IsDuplicateOf(other *Edge) bool              { return edge.isDuplicateOf(other) }
func (edge *Edge) HasDuplicateIn(edges []*Edge) bool           { return edge.hasDuplicateIn(edges) }
func (edge *Edge) MatchingArrowheads(other *Edge) bool         { return edge.hasMatchingArrowheads(other) }
func (edge *Edge) OwnArrowheadsMatch() bool                    { return edge.ownArrowheadsMatch() }
func (edge *Edge) IsStraight() bool                            { return edge.isStraight() }
func (edge *Edge) HasOverlappingEnd() bool                     { return edge.hasOverlappingEnd() }
func (edge *Edge) EuclideanDistance() float64                  { return edge.euclideanDistance() }
func (edge *Edge) IsTreeEdge() bool                            { return edge.isTreeEdge() }
func (edges Edges) PortEdges(node *Node) map[geo.Point][]*Edge { return edges.portEdges(node) }

func (node *Node) ContainsPoint(point *geo.Point, padding float64) bool {
	return node.containsPoint(point, padding)
}

func (node *Node) PortsByOrientation(orientation geo.Orientation) []geo.Point {
	return node.portsByOrientation(orientation)
}

func (node *Node) CenterPorts() []geo.Point                 { return node.centerPorts() }
func (node *Node) ContainsPointOnBox(point *geo.Point) bool { return node.isPointOnNode(point) }
func (node *Node) IsPointNear(point *geo.Point) bool        { return node.isPointNear(point) }
func (node *Node) IsWithinBounds(topLeft, bottomRight *geo.Point) bool {
	return node.isWithinBounds(topLeft, bottomRight)
}
func (node *Node) PortOrientation(point *geo.Point) geo.Orientation {
	return node.pointToPortOrientation(point)
}
func (node *Node) MirroredPorts() map[geo.Point]geo.Point { return node.mirroredPorts() }
func (node *Node) OverlappingPorts(other *Node) map[geo.Point]bool {
	return node.overlappingPorts(other)
}
func (node *Node) DistanceTo(other *Node, includeSizes bool) float64 {
	return node.distanceTo(other, includeSizes)
}
func (nodes Nodes) IntersectsNode(edge *Edge) bool { return nodes.intersectsNode(edge) }
