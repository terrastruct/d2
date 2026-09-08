package layoutgraph

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func EnsureTransactionWorkGuard(ctx context.Context, location string) (context.Context, *limits.WorkGuard, error) {
	return ensureTransactionWorkGuard(ctx, location)
}

// ExistingTransactionWorkGuard checks and returns the request's aggregate
// transaction guard without allocating a derived context or standalone budget.
func ExistingTransactionWorkGuard(ctx context.Context, location string) (*limits.WorkGuard, bool, error) {
	return existingTransactionWorkGuard(ctx, location)
}

// GraphStateSnapshotOptions selects the topology and route identity captured
// by a rollback snapshot.
type GraphStateSnapshotOptions struct {
	CaptureTopology   bool
	CaptureEdgeRoutes bool
}

// NewGraphStateSnapshot creates an empty rollback snapshot with the requested
// capture scope.
func NewGraphStateSnapshot(options GraphStateSnapshotOptions) *GraphState {
	return &GraphState{
		captureTopology:   options.CaptureTopology,
		captureEdgeRoutes: options.CaptureEdgeRoutes,
	}
}

func (gs *GraphState) UpdateWithWorkGuard(g *Graph, guard *limits.WorkGuard) error {
	return gs.updateContext(g, guard)
}

// OwnedNodes returns every node whose state is owned by g, including graph
// nodes temporarily removed by tree or sequence preprocessing. It charges the
// same work units in the same traversal order as the former hierarchy code.
func (gs *GraphState) OwnedNodes(g *Graph, guard *limits.WorkGuard) (map[*Node]struct{}, error) {
	owned := make(map[*Node]struct{}, len(gs.nodes))
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		owned[node] = struct{}{}
	}
	for node := range gs.nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node.Graph == g {
			owned[node] = struct{}{}
		}
	}
	return owned, nil
}

func Validate(ctx context.Context, operation string, g *Graph) error {
	return validateEngineGraph(ctx, operation, g)
}

func (g *Graph) HasFixedNode() bool {
	return g.hasFixedNode()
}

func (g *Graph) AllDescendantNodes(node *Node, includeClusterNodes bool) []*Node {
	return g.allDescendantNodes(node, includeClusterNodes)
}

func (g *Graph) AbductEdges(container *Node, toGraph *Graph) []*EdgeAbduction {
	return g.abductEdges(container, toGraph)
}

func (g *Graph) RestoreEdgeAbductions(edgeAbductions []*EdgeAbduction) []*EdgeAbduction {
	return g.restoreEdgeAbductions(edgeAbductions)
}

// ReplaceEdgesUnchecked replaces graph edge ownership and rebuilds both
// endpoint incidence lists. The caller is responsible for supplying valid
// graph-owned endpoints.
func (g *Graph) ReplaceEdgesUnchecked(edges []*Edge) {
	for _, edge := range g.Edges {
		edge.From.Edges = nil
		edge.To.Edges = nil
	}
	g.Edges = edges
	for _, edge := range edges {
		edge.From.addEdge(edge)
		edge.To.addEdge(edge)
	}
}

func (g *Graph) ContainerPadding(container *Node, considerChildren bool) Spacing {
	return g.containerPadding(container, considerChildren)
}

func UniformSpacing(value float64) Spacing {
	return Spacing{top: value, bottom: value, left: value, right: value}
}

func (spacing Spacing) Left() float64 {
	return spacing.left
}

func (spacing Spacing) Right() float64 {
	return spacing.right
}

func (nodes Nodes) SetGraphReference(g *Graph) {
	nodes.setGraphReference(g)
}

func (nodes Nodes) Center() *geo.Point {
	return nodes.center()
}

func (nodes Nodes) CenterWithWorkGuard(guard *limits.WorkGuard) (*geo.Point, error) {
	if guard == nil {
		return nil, fmt.Errorf("TALA Center requires a work guard")
	}
	return nodes.centerWithWorkGuard(guard)
}

func (nodes Nodes) BoundingBox() (*geo.Point, *geo.Point) {
	return nodes.bounds()
}

func (nodes Nodes) BoundingBoxWithWorkGuard(guard *limits.WorkGuard) (*geo.Point, *geo.Point, error) {
	if guard == nil {
		return nil, nil, fmt.Errorf("TALA BoundingBox requires a work guard")
	}
	return nodes.boundsWithWorkGuard(guard)
}

func (node *Node) IsContainer() bool {
	return node != nil && node.isContainer
}

func (node *Node) IsTable() bool {
	return node.shapeType == tableType
}

func (node *Node) SetClusterVessel(value bool) {
	node.isClusterVessel = value
}

func (node *Node) Adjacent(edge *Edge) *Node {
	return node.adjacent(edge)
}

func (node *Node) Center() *geo.Point {
	return node.center()
}

func (node *Node) Translate(dx, dy float64) {
	node.translate(dx, dy)
}

func (node *Node) Transpose() {
	node.transpose()
}

func (node *Node) Mirror(x, y bool) {
	node.mirror(x, y)
}

func (node *Node) FitToBoundingBox(topLeft, bottomRight *geo.Point, padding Spacing) {
	node.fitToBoundingBox(topLeft, bottomRight, padding)
}

func (node *Node) WrapChildren() {
	node.wrapChildren()
}

func (edge *Edge) IsLoop() bool {
	return edge != nil && edge.isLoop()
}

func (edge *Edge) IsDirected() bool {
	return edge.isDirected()
}

func (edge *Edge) DirectedEndpoints() (from, to *Node, ok bool) {
	return edge.directedEndpoints()
}

func (edge *Edge) IsTargetedTo(node *Node) bool {
	return edge.isTargetedTo(node)
}

func (edge *Edge) IsBetweenTableColumns() bool {
	return edge.isBetweenTableColumns()
}

// HierarchyRankWeight returns the authored-edge multiplicity represented by a
// temporary hierarchy DAG edge. Ordinary graph edges have implicit weight 1.
func (edge *Edge) HierarchyRankWeight() int {
	if edge == nil || !edge.hierarchyRankWeightSet {
		return 1
	}
	return edge.hierarchyRankWeight
}

// SetHierarchyRankWeight records temporary hierarchy DAG metadata. It is kept
// out of graph serialization and reset by ordinary graph cloning.
func (edge *Edge) SetHierarchyRankWeight(weight int) {
	edge.hierarchyRankWeight = weight
	edge.hierarchyRankWeightSet = true
}
