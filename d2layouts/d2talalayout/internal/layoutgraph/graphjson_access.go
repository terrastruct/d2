package layoutgraph

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// These limits are exported for sibling graph codecs. They are part of the
// engine's resource-safety contract, not configurable layout policy.
const (
	MaxSerializationTopologyReferences = maxEngineTopologyReferences
	MaxSerializationRoutePoints        = maxEngineRoutePoints
	MaxSerializationPreflightWork      = maxEnginePreflightWork
)

// ValidateForSerialization performs the layout graph's complete bounded
// topology preflight before graphjson starts encoding references.
func ValidateForSerialization(ctx context.Context, graph *Graph) error {
	return validateEngineGraph(ctx, "SerializeGraph", graph)
}

// ContainerOrderForSerialization returns the stable reverse-DFS fixture
// order while charging the caller's shared serialization work budget.
func ContainerOrderForSerialization(graph *Graph, guard *limits.WorkGuard) ([]*Node, error) {
	return graph.containerRDFSOrderContext(nil, guard)
}

// NodeIDForSerialization represents a nil root with the fixture ID zero.
func NodeIDForSerialization(node *Node) EntityID {
	return node.entityID()
}

func OrderedNearsForSerialization(node *Node) []*Node {
	return node.orderedNears()
}

func CostsForSerialization(graph *Graph) (crossing, turn, nonCenterPort float64) {
	graph.costMu.RLock()
	defer graph.costMu.RUnlock()
	return graph.crossingCost, graph.turnCost, graph.nonCenterPortCost
}

func AddDecodedEdge(node *Node, edge *Edge) {
	node.addEdge(edge)
}

func MarkDecodedContainer(node *Node) {
	node.isContainer = true
}

func MarkDecodedClusterVessel(node *Node) {
	node.isClusterVessel = true
}

func IsActiveClusterForSerialization(cluster *Cluster) bool {
	return cluster.isActive()
}

func IsActiveSequenceForSerialization(sequence *Sequence) bool {
	return sequence.isActive()
}

func NewDecodedHierarchy(levels map[*Node]int) *Hierarchy {
	return &Hierarchy{level: levels}
}

func HierarchyLevelsForSerialization(hierarchy *Hierarchy) map[*Node]int {
	return hierarchy.level
}

// BuildNodeToTreeForDeserialization derives the auxiliary tree index under
// the decoder's shared work budget.
func BuildNodeToTreeForDeserialization(trees map[*Node][]*Tree, guard *limits.WorkGuard) (map[*Node]*Tree, error) {
	graph := &Graph{Trees: trees}
	if err := graph.buildNodeToTreeGuarded(guard); err != nil {
		return nil, err
	}
	return graph.NodeToTree, nil
}

// PublishDeserializedGraph atomically replaces graph-owned state while
// retaining the destination graph's mutex identity.
func PublishDeserializedGraph(destination, staged *Graph, crossing, turn, nonCenterPort float64) {
	destination.IsRootHierarchy = staged.IsRootHierarchy
	destination.Nodes = staged.Nodes
	destination.Edges = staged.Edges
	destination.CellSize = staged.CellSize

	destination.costMu.Lock()
	destination.crossingCost = crossing
	destination.turnCost = turn
	destination.nonCenterPortCost = nonCenterPort
	destination.edgeLengthCache = make(map[uint64]float64)
	destination.costMu.Unlock()

	destination.Containers = staged.Containers
	destination.Clusters = staged.Clusters
	destination.Trees = staged.Trees
	destination.NodeToTree = staged.NodeToTree
	destination.Hubs = staged.Hubs
	destination.Sequences = staged.Sequences
	destination.Directions = staged.Directions
	destination.CommonUncleSiblings = staged.CommonUncleSiblings
}
