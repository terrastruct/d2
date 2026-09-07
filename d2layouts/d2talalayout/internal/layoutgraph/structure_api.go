package layoutgraph

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

const (
	MaxTopologyReferences = maxEngineTopologyReferences
	MaxRoutePoints        = maxEngineRoutePoints
)

// Area returns a node's rectangular area.
func (node *Node) Area() float64 {
	return node.area()
}

// OwningContainer resolves the active cluster or sequence vessel before
// falling back to the node's direct container.
func (node *Node) OwningContainer() *Node {
	return node.container()
}

func (node *Node) IsSequenceStep() bool {
	return node != nil && node.shapeType == stepType
}

func (node *Node) SameShape(other *Node) bool {
	return node != nil && other != nil && node.shapeType == other.shapeType
}

func (node *Node) UnmarkClusterVessel() {
	if node != nil {
		node.isClusterVessel = false
	}
}

func (node *Node) OrderedNears() []*Node {
	return node.orderedNears()
}

func (graph *Graph) BoundingBox() (*geo.Point, *geo.Point) {
	return graph.bounds()
}

func (nodes Nodes) DistanceClusters(threshold float64) [][]*Node {
	return nodes.clusters(threshold)
}

func (nodes Nodes) DistanceClustersWithWorkGuard(threshold float64, guard *limits.WorkGuard) ([][]*Node, error) {
	if guard == nil {
		return nil, fmt.Errorf("TALA DistanceClusters requires a work guard")
	}
	return nodes.clustersWithWorkGuard(threshold, guard)
}

func (assignment *HerdAssignment) SameSidePairCount() int {
	return len(assignment.sameSidePaired)
}

func (assignment *HerdAssignment) OppositeSidePairCount() int {
	return len(assignment.oppositeSidePaired)
}

func (assignment *HerdAssignment) PairSameSide(node *Node) {
	assignment.sameSidePaired[node] = struct{}{}
}

func (assignment *HerdAssignment) PairOppositeSide(node *Node) {
	assignment.oppositeSidePaired[node] = struct{}{}
}

func (node *Node) ConnectionTo(other *Node) *Edge {
	return node.connectionTo(other)
}

func (node *Node) WalkRDFS(apply func(*Node)) {
	node.rdfsWalk(apply)
}

func (edge *Edge) Reconnect(node *Node, to bool) {
	edge.reconnect(node, to)
}

func (spacing Spacing) Top() float64 {
	return spacing.top
}

func (spacing Spacing) Bottom() float64 {
	return spacing.bottom
}

func (sequence *Sequence) IsActive() bool {
	return sequence.isActive()
}

func RestoreGraphState(graph *Graph, state *GraphState) {
	(&Transaction{Graph: graph, PriorGraphState: state}).Rollback()
}

func (graph *Graph) NewRequestTransactionWithWorkGuard(ctx context.Context, guard *limits.WorkGuard, options TransactionOptions) (*Transaction, error) {
	return graph.newRequestTransactionWithGuard(ctx, guard, options)
}

// SortNodesByID gives sibling layout domains the engine's stable node order.
func SortNodesByID(nodes []*Node) {
	sortNodesByID(nodes)
}

func (node *Node) SetContainer(value bool) {
	if node != nil {
		node.isContainer = value
	}
}

func (graph *Graph) IsTreeSentinel(node *Node) bool {
	return graph.isTreeSentinel(node)
}

func (graph *Graph) IsSequenceVessel(node *Node) bool {
	return graph.isSequence(node)
}

func (edge *Edge) IsBidirectional() bool {
	return edge.isBidirectional()
}

func (edge *Edge) IsUndirected() bool {
	return edge.isUndirected()
}

// ContainerRDFSOrder returns nested containers from innermost to outermost
// while charging the supplied request work budget.
func (graph *Graph) ContainerRDFSOrder(root *Node, guard *limits.WorkGuard) ([]*Node, error) {
	return graph.containerRDFSOrderContext(root, guard)
}

func (graph *Graph) ContainerRDFSOrderUnbounded(root *Node) []*Node {
	return graph.containerRDFSOrder(root)
}
