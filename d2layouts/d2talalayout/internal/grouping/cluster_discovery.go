package grouping

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const maxEngineNodes = limits.MaxEngineNodes

type clusterEdgeSignature struct {
	from, to                     int
	bidirectional, undirected    int
	directed                     int
	fromArrowheads, toArrowheads map[layoutgraph.Arrowhead]struct{}
}

func (signature *clusterEdgeSignature) add(node *layoutgraph.Node, edge *layoutgraph.Edge) {
	if edge.From == node {
		signature.from++
	}
	if edge.To == node {
		signature.to++
	}
	switch {
	case edge.IsBidirectional():
		signature.bidirectional++
	case edge.IsUndirected():
		signature.undirected++
	case edge.IsDirected():
		signature.directed++
	}
	if edge.From == node {
		signature.fromArrowheads[edge.SourceArrowhead] = struct{}{}
		signature.toArrowheads[edge.TargetArrowhead] = struct{}{}
	} else {
		signature.fromArrowheads[edge.TargetArrowhead] = struct{}{}
		signature.toArrowheads[edge.SourceArrowhead] = struct{}{}
	}
}

func (signature clusterEdgeSignature) arrowTypeCount() int {
	types := 0
	if signature.directed > 0 {
		types++
	}
	if signature.bidirectional > 0 {
		types++
	}
	if signature.undirected > 0 {
		types++
	}
	return types
}

func arrowheadSetsEqual(first, second map[layoutgraph.Arrowhead]struct{}) bool {
	if len(first) != len(second) {
		return false
	}
	for arrowhead := range first {
		if _, found := second[arrowhead]; !found {
			return false
		}
	}
	return true
}

func (signature clusterEdgeSignature) matches(other clusterEdgeSignature) bool {
	return signature.from == other.from &&
		signature.to == other.to &&
		signature.bidirectional == other.bidirectional &&
		signature.undirected == other.undirected &&
		signature.arrowTypeCount() <= 1 &&
		other.arrowTypeCount() <= 1 &&
		arrowheadSetsEqual(signature.fromArrowheads, other.fromArrowheads) &&
		arrowheadSetsEqual(signature.toArrowheads, other.toArrowheads)
}

type clusterDiscoveryInfo struct {
	neighbors       []*layoutgraph.Node
	neighborSet     map[*layoutgraph.Node]struct{}
	edges           []*layoutgraph.Edge
	edgeSignature   clusterEdgeSignature
	estimatedWidth  float64
	estimatedHeight float64
	noClustering    bool
	toTableColumn   bool
}

type clusterDiscoveryIndex struct {
	infos         map[*layoutgraph.Node]*clusterDiscoveryInfo
	edgeOrder     map[*layoutgraph.Edge]int
	edgeNodes     map[*layoutgraph.Edge][]*layoutgraph.Node
	sequenceEdges map[*layoutgraph.Sequence]map[*layoutgraph.Edge]*layoutgraph.Node
}

func (index *clusterDiscoveryIndex) sequenceOriginal(
	sequence *layoutgraph.Sequence,
	edge *layoutgraph.Edge,
	guard *limits.WorkGuard,
) (*layoutgraph.Node, error) {
	byEdge, built := index.sequenceEdges[sequence]
	if !built {
		byEdge = make(map[*layoutgraph.Edge]*layoutgraph.Node, len(sequence.EdgeAbductions))
		for _, abduction := range sequence.EdgeAbductions {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if abduction == nil || abduction.Edge == nil {
				continue
			}
			switch {
			case abduction.CurrentFrom == sequence.Vessel:
				if _, exists := byEdge[abduction.Edge]; !exists {
					byEdge[abduction.Edge] = abduction.OriginallyFrom
				}
			case abduction.CurrentTo == sequence.Vessel:
				if _, exists := byEdge[abduction.Edge]; !exists {
					byEdge[abduction.Edge] = abduction.OriginallyTo
				}
			}
		}
		index.sequenceEdges[sequence] = byEdge
	}
	return byEdge[edge], nil
}

func (index *clusterDiscoveryIndex) refreshNeighbors(
	g *layoutgraph.Graph,
	node *layoutgraph.Node,
	guard *limits.WorkGuard,
) error {
	info := index.infos[node]
	if info == nil {
		return fmt.Errorf("TALA AddClusters cannot find node discovery index")
	}
	neighbors := make([]*layoutgraph.Node, 0, len(node.Edges))
	neighborSet := make(map[*layoutgraph.Node]struct{}, len(node.Edges))
	for _, edge := range node.Edges {
		if err := guard.Step(); err != nil {
			return err
		}
		adjacent := node.Adjacent(edge)
		if sequence := g.Sequences[adjacent]; sequence != nil {
			var err error
			adjacent, err = index.sequenceOriginal(sequence, edge, guard)
			if err != nil {
				return err
			}
		}
		if _, exists := neighborSet[adjacent]; !exists {
			neighborSet[adjacent] = struct{}{}
			neighbors = append(neighbors, adjacent)
		}
	}
	info.neighbors = neighbors
	info.neighborSet = neighborSet
	return guard.Finish()
}

func (index *clusterDiscoveryIndex) refreshAfterClusterAbduction(
	g *layoutgraph.Graph,
	cluster *layoutgraph.Cluster,
	edges []*layoutgraph.Edge,
	guard *limits.WorkGuard,
) error {
	affected := make([]*layoutgraph.Node, 0, len(edges))
	seen := make(map[*layoutgraph.Node]struct{}, len(edges))
	addAffected := func(node *layoutgraph.Node) error {
		if err := guard.Step(); err != nil {
			return err
		}
		if node == nil || node == cluster.Vessel || index.infos[node] == nil {
			return nil
		}
		if _, exists := seen[node]; exists {
			return nil
		}
		seen[node] = struct{}{}
		affected = append(affected, node)
		return nil
	}
	// clusterIncidentEdges returns graph-edge order. Preserve it and the original
	// candidate-node discovery order. Using the reverse adjacency index also
	// preserves legacy behavior for tolerated asymmetric node.Edges inventories.
	for _, edge := range edges {
		for _, node := range index.edgeNodes[edge] {
			if err := addAffected(node); err != nil {
				return err
			}
		}
	}
	for _, node := range affected {
		if err := index.refreshNeighbors(g, node, guard); err != nil {
			return err
		}
	}
	return guard.Finish()
}

func clusterIsDescendantOfGuarded(descendant, ancestor *layoutgraph.Node, guard *limits.WorkGuard) (bool, error) {
	seen := make(map[*layoutgraph.Node]struct{})
	for {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if ancestor == descendant {
			return true, nil
		}
		if descendant == nil {
			return false, nil
		}
		if _, exists := seen[descendant]; exists {
			return false, fmt.Errorf("TALA AddClusters found a cycle in node ancestry")
		}
		if len(seen) >= maxEngineNodes {
			return false, fmt.Errorf("TALA AddClusters ancestry exceeds node limit")
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

func clusterHasLeakyEdgeGuarded(g *layoutgraph.Graph, node *layoutgraph.Node, guard *limits.WorkGuard) (bool, error) {
	if !node.IsContainer() {
		return false, nil
	}
	descendants, err := g.AllDescendantNodesWithWorkGuard(node, true, guard)
	if err != nil {
		return false, err
	}
	for _, descendant := range descendants {
		if err := guard.Step(); err != nil {
			return false, err
		}
		for _, edge := range descendant.Edges {
			if err := guard.Step(); err != nil {
				return false, err
			}
			adjacent := descendant.Adjacent(edge)
			inside, err := clusterIsDescendantOfGuarded(adjacent, node, guard)
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

func buildClusterDiscoveryIndex(
	g *layoutgraph.Graph,
	containerOrder []*layoutgraph.Node,
	guard *limits.WorkGuard,
) (*clusterDiscoveryIndex, error) {
	orderedNodes := make([]*layoutgraph.Node, 0, len(g.Nodes))
	seenNodes := make(map[*layoutgraph.Node]struct{}, len(g.Nodes))
	addNode := func(node *layoutgraph.Node) error {
		if err := guard.Step(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("TALA AddClusters contains a nil node")
		}
		if _, exists := seenNodes[node]; exists {
			return nil
		}
		if len(seenNodes) >= maxEngineNodes {
			return fmt.Errorf("TALA AddClusters unique node count exceeds limit %d", maxEngineNodes)
		}
		seenNodes[node] = struct{}{}
		orderedNodes = append(orderedNodes, node)
		return nil
	}
	for _, container := range containerOrder {
		if err := addNode(container); err != nil {
			return nil, err
		}
		for _, child := range g.Containers[container] {
			if err := addNode(child); err != nil {
				return nil, err
			}
		}
	}
	for _, child := range g.Containers[nil] {
		if err := addNode(child); err != nil {
			return nil, err
		}
	}

	index := &clusterDiscoveryIndex{
		infos:         make(map[*layoutgraph.Node]*clusterDiscoveryInfo, len(orderedNodes)),
		edgeOrder:     make(map[*layoutgraph.Edge]int, len(g.Edges)),
		edgeNodes:     make(map[*layoutgraph.Edge][]*layoutgraph.Node, len(g.Edges)),
		sequenceEdges: make(map[*layoutgraph.Sequence]map[*layoutgraph.Edge]*layoutgraph.Node),
	}
	for _, node := range orderedNodes {
		index.infos[node] = &clusterDiscoveryInfo{
			neighborSet: make(map[*layoutgraph.Node]struct{}),
			edgeSignature: clusterEdgeSignature{
				fromArrowheads: make(map[layoutgraph.Arrowhead]struct{}),
				toArrowheads:   make(map[layoutgraph.Arrowhead]struct{}),
			},
			estimatedWidth:  node.Width,
			estimatedHeight: node.Height,
		}
	}

	for _, node := range orderedNodes {
		info := index.infos[node]
		hasLoop := false
		for _, edge := range node.Edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			index.edgeNodes[edge] = append(index.edgeNodes[edge], node)
			if edge.IsLoop() {
				hasLoop = true
			}
			if edge.FromTableColumnIndex != nil || edge.ToTableColumnIndex != nil {
				info.toTableColumn = true
			}
		}
		if err := index.refreshNeighbors(g, node, guard); err != nil {
			return nil, err
		}
		leaky, err := clusterHasLeakyEdgeGuarded(g, node, guard)
		if err != nil {
			return nil, err
		}
		info.noClustering = g.IsTreeSentinel(node) ||
			node.IsClusterVessel() ||
			g.IsSequenceVessel(node) ||
			node.IsTable() ||
			node.Hierarchy != nil ||
			node.FixedTopLeft != nil ||
			leaky ||
			hasLoop
	}

	for edgeIndex, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if _, exists := index.edgeOrder[edge]; !exists {
			index.edgeOrder[edge] = edgeIndex
		}
		if info := index.infos[edge.From]; info != nil {
			info.edges = append(info.edges, edge)
			info.edgeSignature.add(edge.From, edge)
		}
		if edge.To != edge.From {
			if info := index.infos[edge.To]; info != nil {
				info.edges = append(info.edges, edge)
				info.edgeSignature.add(edge.To, edge)
			}
		}
	}

	return index, guard.Finish()
}

func clusterIncidentEdges(
	cluster *layoutgraph.Cluster,
	infos map[*layoutgraph.Node]*clusterDiscoveryInfo,
	edgeOrder map[*layoutgraph.Edge]int,
	guard *limits.WorkGuard,
) ([]*layoutgraph.Edge, error) {
	unique := make(map[*layoutgraph.Edge]struct{})
	for _, node := range cluster.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		info := infos[node]
		if info == nil {
			return nil, fmt.Errorf("TALA AddClusters cannot find cluster member index")
		}
		for _, edge := range info.edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			unique[edge] = struct{}{}
		}
	}
	edges := make([]*layoutgraph.Edge, 0, len(unique))
	for edge := range unique {
		edges = append(edges, edge)
	}
	for width := 1; width < len(edges); width *= 2 {
		for range edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
		}
		if width > len(edges)/2 {
			break
		}
	}
	slices.SortFunc(edges, func(a, b *layoutgraph.Edge) int {
		return cmp.Compare(edgeOrder[a], edgeOrder[b])
	})
	return edges, guard.Finish()
}
