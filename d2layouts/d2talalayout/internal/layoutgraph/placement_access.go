package layoutgraph

import (
	"context"

	"github.com/d2lang/d2/lib/geo"
)

func (graph *Graph) FixedNodes() []*Node { return graph.fixedNodes() }
func (nodes Nodes) FixedNodes() []*Node  { return nodes.fixedNodes() }
func (nodes Nodes) HasFixedNode() bool   { return nodes.hasFixedNode() }

func (graph *Graph) TreeEdgeMap() map[*Edge]bool { return graph.buildIsTreeEdgeMap() }
func (graph *Graph) AddIsolatedTreeEdges(edges map[*Edge]bool) {
	graph.addIsolatedTreeEdgesToMap(edges)
}

func (graph *Graph) ApplyEdgeAbductions(abductions []*EdgeAbduction) {
	graph.applyEdgeAbductions(abductions)
}

func (graph *Graph) SyncNestedGeometry() { graph.syncNested() }
func (graph *Graph) TurnCost() float64   { return graph.turnCostValue() }
func (graph *Graph) HalveTurnCost() {
	graph.costMu.Lock()
	graph.turnCost /= 2
	graph.costMu.Unlock()
}
func (graph *Graph) ResetTurnCost() { graph.resetTurnCost() }

func (graph *Graph) ContainerFixedOrigin(container *Node) *geo.Point {
	return graph.containerFixedOrigin(container)
}
func (edge *Edge) FacingTablePortValues(from, to *Node) (geo.Point, geo.Point, bool, bool, geo.Orientation) {
	ports := edge.facingTablePorts(from, to)
	return ports.from, ports.to, ports.hasFrom, ports.hasTo, ports.orientation
}
func (edge *Edge) HasTableColumn() bool                      { return edge.hasTableColumn() }
func (edge *Edge) IsAxisAligned() bool                       { return edge.isAxisAligned() }
func (edge *Edge) NumSegments() int                          { return edge.segmentCount() }
func (edge *Edge) BoundingBox() (*geo.Point, *geo.Point)     { return edge.bounds() }
func (edge *Edge) BoundingBoxValues() (geo.Point, geo.Point) { return edge.boundingBoxValues() }

func (node *Node) IsClass() bool      { return node != nil && node.shapeType == classType }
func (node *Node) Ports() []geo.Point { return node.ports() }

func (node *Node) OrthogonalDistanceTo(other *Node) (float64, float64) {
	return node.orthogonalDistanceTo(other)
}
func (node *Node) ConnectedNodes(excluded []*Node, graph *Graph) []*Node {
	return node.connectedNodes(excluded, graph)
}
func (node *Node) PointPastFixedOrigin(x, y float64, includeSizes bool) bool {
	return node.isPointPastFixedOrigin(x, y, includeSizes)
}
func (node *Node) DoesOverlapAt(other *Node, point *geo.Point) bool {
	return node.doesOverlapAt(other, point)
}
func (node *Node) DoesOverlap(other *Node) bool { return node.doesOverlap(other) }
func (node *Node) OverlapsAlongDimension(other *Node, horizontal, includeSizes bool) bool {
	return node.overlapsAlongDimension(other, horizontal, includeSizes)
}
func (node *Node) VisibilityGraphCandidate(horizontal, checkSide, includeSizes bool, other *Node, padding float64) bool {
	return node.isVisibilityGraphCandidate(horizontal, checkSide, includeSizes, other, padding)
}
func (node *Node) MoveAbsWithChildren(x, y float64) { node.moveNodeAbsWithChildren(x, y) }
func (node *Node) MoveWithChildren(dx, dy float64)  { node.moveNodeWithChildren(dx, dy) }
func (node *Node) PositionContainerChildren(withPadding bool) {
	node.positionContainerChildren(withPadding)
}
func (node *Node) NearestSharedAncestor(other *Node) *Node {
	return node.nearestSharedAncestor(other)
}
func (node *Node) ContainerLevel() int                { return node.containerLevel() }
func (node *Node) IsDescendantOf(ancestor *Node) bool { return node.isDescendantOf(ancestor) }
func (node *Node) IDValue() EntityID                  { return node.entityID() }
func (node *Node) IsAdjacentTo(other *Node, includeSizes bool) bool {
	return node.isAdjacentTo(other, includeSizes)
}
func (node *Node) PassesThrough(first, second *geo.Point) bool {
	return node.passesThrough(first, second)
}
func (node *Node) SpillsOutOf(container *Node) bool { return node.spillsOutOf(container) }
func (node *Node) DeltaTo(other *Node, at *geo.Point) int {
	return node.deltaTo(other, at)
}
func (node *Node) IsMajorityTarget() bool                   { return node.isMajorityTarget() }
func (node *Node) FitToGraph(graph *Graph, padding Spacing) { node.fitNodeToGraph(graph, padding) }

func (nodes Nodes) NumAdjacent() int { return nodes.adjacentCount() }
func (nodes Nodes) FixedBoundingBox() (*geo.Point, *geo.Point) {
	return nodes.fixedBounds()
}

func (cluster *Cluster) IsActive() bool { return cluster.isActive() }

func (sequence *Sequence) AbductedNodeByEdge(edge *Edge) *Node {
	return sequence.findAbductedNodeByEdge(edge)
}

func (tree *Tree) SentinelNode() *Node { return tree.sentinelNode() }

func (transaction *Transaction) OriginalDimensions(node *Node) (width, height float64) {
	geometry := transaction.PriorGraphState.nodeGeometry[node]
	return geometry.width, geometry.height
}

// ValidateSubgraphCombination validates node-owned topology across a set of
// aliased graph views after each distinct graph-owned map identity has passed
// Validate.
func ValidateSubgraphCombination(ctx context.Context, graphs []*Graph) error {
	return validateCombineNodeTopology(ctx, graphs)
}
