// Package graphjson serializes and reconstructs TALA layout graphs.
package graphjson

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func newWorkGuard(ctx context.Context, location string) (*limits.WorkGuard, error) {
	return limits.NewWorkGuard(ctx, location, limits.MaxEngineWorkUnits)
}

func serializePoint(point *geo.Point) *SerializedPoint {
	if point == nil {
		return nil
	}
	return &SerializedPoint{
		X: point.X,
		Y: point.Y,
	}
}

func deserializePoint(point *SerializedPoint) *geo.Point {
	if point == nil {
		return nil
	}
	return geo.NewPoint(point.X, point.Y)
}

// edgeSerializationIDs assigns a stable, serialization-only ID to every edge
// whose ID is unset. layoutgraph.Graph constructors legitimately leave layoutgraph.Edge.ID at zero, but
// serialized trees and edge abductions need an unambiguous reference when a
// graph contains more than one such edge.
type edgeSerializationIDs struct {
	byEdge map[*layoutgraph.Edge]layoutgraph.EntityID
	used   map[layoutgraph.EntityID]struct{}
	next   layoutgraph.EntityID
}

func newEdgeSerializationIDs(g *layoutgraph.Graph, guard *limits.WorkGuard) (*edgeSerializationIDs, error) {
	ids := &edgeSerializationIDs{
		byEdge: make(map[*layoutgraph.Edge]layoutgraph.EntityID),
		used:   make(map[layoutgraph.EntityID]struct{}),
		next:   -1,
	}

	reserveEdge := func(edge *layoutgraph.Edge) error {
		if err := guard.Step(); err != nil {
			return err
		}
		if edge != nil && edge.ID != 0 {
			ids.used[edge.ID] = struct{}{}
		}
		return nil
	}
	seenNodes := make(map[*layoutgraph.Node]struct{})
	reserveNode := func(node *layoutgraph.Node) error {
		if err := guard.Step(); err != nil {
			return err
		}
		if node == nil {
			return nil
		}
		if _, seen := seenNodes[node]; seen {
			return nil
		}
		seenNodes[node] = struct{}{}
		for _, edge := range node.Edges {
			if err := reserveEdge(edge); err != nil {
				return err
			}
		}
		return nil
	}

	for _, edge := range g.Edges {
		if err := reserveEdge(edge); err != nil {
			return nil, err
		}
	}
	for _, node := range g.Nodes {
		if err := reserveNode(node); err != nil {
			return nil, err
		}
	}
	for container, children := range g.Containers {
		if err := reserveNode(container); err != nil {
			return nil, err
		}
		for _, child := range children {
			if err := reserveNode(child); err != nil {
				return nil, err
			}
		}
	}
	for vessel, cluster := range g.Clusters {
		if err := reserveNode(vessel); err != nil {
			return nil, err
		}
		if cluster == nil {
			continue
		}
		if err := reserveNode(cluster.Container); err != nil {
			return nil, err
		}
		for _, node := range cluster.Nodes {
			if err := reserveNode(node); err != nil {
				return nil, err
			}
		}
		for _, abduction := range cluster.EdgeAbductions {
			if abduction != nil {
				if err := reserveEdge(abduction.Edge); err != nil {
					return nil, err
				}
			}
		}
	}
	for vessel, sequence := range g.Sequences {
		if err := reserveNode(vessel); err != nil {
			return nil, err
		}
		if sequence == nil {
			continue
		}
		if err := reserveNode(sequence.Container); err != nil {
			return nil, err
		}
		for _, node := range sequence.Nodes {
			if err := reserveNode(node); err != nil {
				return nil, err
			}
		}
		for _, abduction := range sequence.EdgeAbductions {
			if abduction != nil {
				if err := reserveEdge(abduction.Edge); err != nil {
					return nil, err
				}
			}
		}
	}
	for rootSentinel, roots := range g.Trees {
		if err := reserveNode(rootSentinel); err != nil {
			return nil, err
		}
		stack := make([]*layoutgraph.Tree, 0, len(roots))
		for _, root := range slices.Backward(roots) {
			stack = append(stack, root)
		}
		for len(stack) > 0 {
			tree := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if tree == nil {
				return nil, fmt.Errorf("cannot serialize a nil tree")
			}
			if err := reserveNode(tree.Node); err != nil {
				return nil, err
			}
			if err := reserveEdge(tree.SentinelEdge); err != nil {
				return nil, err
			}
			for _, v := range slices.Backward(tree.Children) {
				stack = append(stack, v)
			}
		}
	}

	// Allocate unset references in the same stable order used by the serialized
	// representation. In particular, do not let per-node adjacency order decide
	// IDs later used by fixture references and deterministic output.
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		ids.reference(edge)
	}
	for _, vessel := range g.ClusterOrder() {
		for _, abduction := range g.Clusters[vessel].EdgeAbductions {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if abduction != nil {
				ids.reference(abduction.Edge)
			}
		}
	}
	for _, vessel := range g.SequenceOrder() {
		for _, abduction := range g.Sequences[vessel].EdgeAbductions {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if abduction != nil {
				ids.reference(abduction.Edge)
			}
		}
	}
	for _, rootSentinel := range g.TreeOrder() {
		roots := g.Trees[rootSentinel]
		stack := make([]*layoutgraph.Tree, 0, len(roots))
		for _, root := range slices.Backward(roots) {
			stack = append(stack, root)
		}
		for len(stack) > 0 {
			tree := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if err := guard.Step(); err != nil {
				return nil, err
			}
			ids.reference(tree.SentinelEdge)
			for _, v := range slices.Backward(tree.Children) {
				stack = append(stack, v)
			}
		}
	}

	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (ids *edgeSerializationIDs) reference(edge *layoutgraph.Edge) layoutgraph.EntityID {
	if edge == nil {
		return 0
	}
	if id, ok := ids.byEdge[edge]; ok {
		return id
	}
	if edge.ID != 0 {
		ids.byEdge[edge] = edge.ID
		return edge.ID
	}
	for {
		candidate := ids.next
		ids.next--
		if candidate == 0 {
			continue
		}
		if _, used := ids.used[candidate]; used {
			continue
		}
		ids.used[candidate] = struct{}{}
		ids.byEdge[edge] = candidate
		return candidate
	}
}

// Serialize validates graph bounds before walking any nested
// topology, then observes ctx throughout the bounded serialization. The
// preflight is deliberately inside this lowest-level entry point so cloning
// and fixture encoding cannot accidentally bypass it.
func Serialize(ctx context.Context, g *layoutgraph.Graph) (*SerializedGraph, error) {
	if ctx == nil {
		return nil, fmt.Errorf("TALA SerializeGraph requires a context")
	}
	if g == nil {
		return nil, fmt.Errorf("cannot serialize a nil graph")
	}
	if err := layoutgraph.ValidateForSerialization(ctx, g); err != nil {
		return nil, err
	}
	guard, err := newWorkGuard(ctx, "SerializeGraph")
	if err != nil {
		return nil, err
	}

	edgeIDs, err := newEdgeSerializationIDs(g, guard)
	if err != nil {
		return nil, err
	}
	inSerializedNodes := make(map[*layoutgraph.Node]bool)
	serializedNodes := []SerializedNode{}
	for _, node := range g.Nodes {
		serialized, err := serializeNode(node, guard)
		if err != nil {
			return nil, err
		}
		serializedNodes = append(serializedNodes, serialized)
		inSerializedNodes[node] = true
	}
	for _, vessel := range g.ClusterOrder() {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if _, ok := inSerializedNodes[vessel]; !ok {
			inSerializedNodes[vessel] = true
		}
		for _, n := range g.Clusters[vessel].Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if _, ok := inSerializedNodes[n]; !ok {
				serialized, err := serializeNode(n, guard)
				if err != nil {
					return nil, err
				}
				serializedNodes = append(serializedNodes, serialized)
				inSerializedNodes[n] = true
			}
		}
	}
	for _, vessel := range g.SequenceOrder() {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if _, ok := inSerializedNodes[vessel]; !ok {
			inSerializedNodes[vessel] = true
		}
		for _, n := range g.Sequences[vessel].Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if _, ok := inSerializedNodes[n]; !ok {
				serialized, err := serializeNode(n, guard)
				if err != nil {
					return nil, err
				}
				serializedNodes = append(serializedNodes, serialized)
				inSerializedNodes[n] = true
			}
		}
	}

	serializedEdges := []SerializedEdge{}
	for _, edge := range g.Edges {
		serialized, err := serializeEdge(edge, edgeIDs, guard)
		if err != nil {
			return nil, err
		}
		serializedEdges = append(serializedEdges, serialized)
	}

	// serialize in a fixed order based on node id
	containers := map[layoutgraph.EntityID][]layoutgraph.EntityID{}
	if len(g.Containers) > 0 {
		ordered, err := layoutgraph.ContainerOrderForSerialization(g, guard)
		if err != nil {
			return nil, err
		}
		if len(g.Containers) != len(ordered)+1 {
			return nil, fmt.Errorf("g.Containers length (%d) does not match rdfs ordered container length (%d)", len(g.Containers), len(ordered)+1)
		}
		for _, containerNode := range append(ordered, nil) {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			newvals := []layoutgraph.EntityID{}
			for _, node := range g.Containers[containerNode] {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				if !inSerializedNodes[node] {
					return nil, fmt.Errorf("error while serializing: node %v is missing from g.Nodes but is a child of %v in g.Containers", node.DebugID(), containerNode.DebugID())
				}
				newvals = append(newvals, layoutgraph.NodeIDForSerialization(node))
			}
			containers[layoutgraph.NodeIDForSerialization(containerNode)] = newvals
		}
	}

	clusters := map[layoutgraph.EntityID]SerializedCluster{}
	clusterVessels := map[layoutgraph.EntityID]SerializedNode{}
	for _, vessel := range g.ClusterOrder() {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		clusterNodes := []layoutgraph.EntityID{}
		cluster := g.Clusters[vessel]
		for _, node := range cluster.Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if !inSerializedNodes[node] {
				return nil, fmt.Errorf("error while serializing: node %v is missing from g.Nodes but is in cluster %v in g.Clusters", node.DebugID(), vessel.DebugID())
			}
			clusterNodes = append(clusterNodes, layoutgraph.NodeIDForSerialization(node))
		}

		abductions := []SerializedEdgeAbduction{}
		for _, abduction := range cluster.EdgeAbductions {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			abductions = append(abductions, SerializedEdgeAbduction{
				Edge:               edgeIDs.reference(abduction.Edge),
				OriginallyFromNode: layoutgraph.NodeIDForSerialization(abduction.OriginallyFrom),
				OriginallyToNode:   layoutgraph.NodeIDForSerialization(abduction.OriginallyTo),
				CurrentFromNode:    layoutgraph.NodeIDForSerialization(abduction.CurrentFrom),
				CurrentToNode:      layoutgraph.NodeIDForSerialization(abduction.CurrentTo),
			})
		}

		newval := SerializedCluster{
			Nodes:              clusterNodes,
			Arrangement:        string(cluster.Arrangement),
			DesiredArrangement: string(cluster.DesiredArrangement),
			Container:          layoutgraph.NodeIDForSerialization(cluster.Container),
			EdgeAbductions:     abductions,
			Padding:            cluster.Padding,
			FixedSize:          cluster.FixedSize,
		}

		clusters[layoutgraph.NodeIDForSerialization(vessel)] = newval
		serializedVessel, err := serializeNode(vessel, guard)
		if err != nil {
			return nil, err
		}
		clusterVessels[layoutgraph.NodeIDForSerialization(vessel)] = serializedVessel
	}

	sequences := map[layoutgraph.EntityID]SerializedSequence{}
	sequenceVessels := map[layoutgraph.EntityID]SerializedNode{}
	for _, vessel := range g.SequenceOrder() {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		sequenceNodes := []layoutgraph.EntityID{}
		seq := g.Sequences[vessel]
		for _, node := range seq.Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if !inSerializedNodes[node] {
				return nil, fmt.Errorf("error while serializing: node %v is missing from g.Nodes but is in sequence %v in g.Sequences", layoutgraph.NodeIDForSerialization(node), layoutgraph.NodeIDForSerialization(vessel))
			}
			sequenceNodes = append(sequenceNodes, layoutgraph.NodeIDForSerialization(node))
		}

		abductions := []SerializedEdgeAbduction{}
		for _, abduction := range seq.EdgeAbductions {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			abductions = append(abductions, SerializedEdgeAbduction{
				Edge:               edgeIDs.reference(abduction.Edge),
				OriginallyFromNode: layoutgraph.NodeIDForSerialization(abduction.OriginallyFrom),
				OriginallyToNode:   layoutgraph.NodeIDForSerialization(abduction.OriginallyTo),
				CurrentFromNode:    layoutgraph.NodeIDForSerialization(abduction.CurrentFrom),
				CurrentToNode:      layoutgraph.NodeIDForSerialization(abduction.CurrentTo),
			})
		}

		newval := SerializedSequence{
			Nodes:          sequenceNodes,
			Container:      layoutgraph.NodeIDForSerialization(seq.Container),
			EdgeAbductions: abductions,
		}

		sequences[layoutgraph.NodeIDForSerialization(vessel)] = newval
		serializedVessel, err := serializeNode(vessel, guard)
		if err != nil {
			return nil, err
		}
		sequenceVessels[layoutgraph.NodeIDForSerialization(vessel)] = serializedVessel
	}

	hubs := map[layoutgraph.EntityID][]layoutgraph.EntityID{}
	if len(g.Hubs) > 0 {
		for _, n := range g.HubOrder() {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			newvals := []layoutgraph.EntityID{}
			for _, node := range g.Hubs[n] {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				if !inSerializedNodes[node] {
					return nil, fmt.Errorf("error while serializing: node %v is missing from g.Nodes but is a spoke of %v in g.Hubs", node.DebugID(), n.DebugID())
				}
				newvals = append(newvals, layoutgraph.NodeIDForSerialization(node))
			}
			hubs[layoutgraph.NodeIDForSerialization(n)] = newvals
		}
	}

	trees := map[layoutgraph.EntityID][]SerializedTree{}
	for _, rootSentinel := range g.TreeOrder() {
		for _, root := range g.Trees[rootSentinel] {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			serialized, err := serializeTree(root, edgeIDs, guard)
			if err != nil {
				return nil, err
			}
			rootID := layoutgraph.NodeIDForSerialization(rootSentinel)
			trees[rootID] = append(trees[rootID], serialized)
		}
	}

	seenHierarchies := make(map[*layoutgraph.Hierarchy]struct{})
	var hierarchies []SerializedHierarchy
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node.Hierarchy == nil {
			continue
		} else if _, seen := seenHierarchies[node.Hierarchy]; seen {
			continue
		}
		seenHierarchies[node.Hierarchy] = struct{}{}

		nodeToLevel := make(map[layoutgraph.EntityID]int)
		for node, level := range layoutgraph.HierarchyLevelsForSerialization(node.Hierarchy) {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			nodeToLevel[node.ID] = level
		}
		hierarchies = append(hierarchies, SerializedHierarchy{
			Levels: nodeToLevel,
		})
	}

	directions := map[layoutgraph.EntityID]string{}
	for container, dir := range g.Directions {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		directions[layoutgraph.NodeIDForSerialization(container)] = serializeDirection(dir)
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	crossingCost, turnCost, nonCenterPortCost := layoutgraph.CostsForSerialization(g)

	return &SerializedGraph{
		Nodes:             serializedNodes,
		Edges:             serializedEdges,
		CellSize:          g.CellSize,
		CrossingCost:      crossingCost,
		TurnCost:          turnCost,
		NonCenterPortCost: nonCenterPortCost,
		IsRootHierarchy:   g.IsRootHierarchy,
		Containers:        containers,
		Hubs:              hubs,
		Clusters:          clusters,
		ClusterVessels:    clusterVessels,
		Sequences:         sequences,
		SequenceVessels:   sequenceVessels,
		Trees:             trees,
		Hierarchies:       hierarchies,
		Directions:        directions,
	}, nil
}

func serializeLabel(l *layoutgraph.Label) *SerializedLabel {
	if l == nil {
		return nil
	}
	return &SerializedLabel{
		Text:     l.Text,
		Position: l.Position.String(),
		Width:    l.Width,
		Height:   l.Height,
	}
}

func serializeIcon(i *layoutgraph.Icon) *SerializedIcon {
	if i == nil {
		return nil
	}
	return &SerializedIcon{
		Position: i.Position.String(),
	}
}

func deserializeLabel(l *SerializedLabel, guard *limits.WorkGuard) (*layoutgraph.Label, error) {
	if l == nil {
		return nil, nil
	}
	if err := guard.Step(); err != nil {
		return nil, err
	}
	return &layoutgraph.Label{
		Text:     l.Text,
		Position: label.FromString(l.Position),
		Width:    l.Width,
		Height:   l.Height,
	}, nil
}

func deserializeIcon(i *SerializedIcon, guard *limits.WorkGuard) (*layoutgraph.Icon, error) {
	if i == nil {
		return nil, nil
	}
	if err := guard.Step(); err != nil {
		return nil, err
	}
	return &layoutgraph.Icon{
		Position: label.FromString(i.Position),
	}, nil
}

func serializeNode(node *layoutgraph.Node, guard *limits.WorkGuard) (SerializedNode, error) {
	nears := []layoutgraph.EntityID{}
	for _, n := range layoutgraph.OrderedNearsForSerialization(node) {
		if err := guard.Step(); err != nil {
			return SerializedNode{}, err
		}
		nears = append(nears, layoutgraph.NodeIDForSerialization(n))
	}

	var d2ID *string
	if node.D2ID != nil {
		d2ID = new(*node.D2ID)
	}
	var fontSize *int
	if node.FontSize != nil {
		fontSize = new(*node.FontSize)
	}

	var desiredWidth, desiredHeight *float64
	if node.DesiredWidth != nil {
		desiredWidth = new(*node.DesiredWidth)
	}
	if node.DesiredHeight != nil {
		desiredHeight = new(*node.DesiredHeight)
	}

	return SerializedNode{
		ID:             node.ID,
		IsInvisible:    node.IsInvisible,
		D2ID:           d2ID,
		Width:          node.Width,
		Height:         node.Height,
		TopLeft:        serializePoint(node.TopLeft),
		FixedTopLeft:   serializePoint(node.FixedTopLeft),
		DesiredWidth:   desiredWidth,
		DesiredHeight:  desiredHeight,
		Nears:          nears,
		FontSize:       fontSize,
		ShapeType:      node.ShapeType(),
		ForceHierarchy: node.ForceHierarchy,
		Label:          serializeLabel(node.Label),
		Icon:           serializeIcon(node.Icon),
		NumColumns:     node.NumColumns(),
		Is3D:           node.Is3D,
		IsMultiple:     node.IsMultiple,
	}, nil
}

func serializeEdge(edge *layoutgraph.Edge, edgeIDs *edgeSerializationIDs, guard *limits.WorkGuard) (SerializedEdge, error) {
	points := []*SerializedPoint{}
	for _, point := range edge.Points {
		if err := guard.Step(); err != nil {
			return SerializedEdge{}, err
		}
		points = append(points, serializePoint(point))
	}

	var fromIndex, toIndex *int
	if edge.FromTableColumnIndex != nil {
		fromIndex = new(*edge.FromTableColumnIndex)
	}
	if edge.ToTableColumnIndex != nil {
		toIndex = new(*edge.ToTableColumnIndex)
	}

	var d2ID *string
	if edge.D2ID != nil {
		d2ID = new(*edge.D2ID)
	}
	serializedID := edgeIDs.reference(edge)
	var originalID *layoutgraph.EntityID
	if serializedID != edge.ID {
		originalID = new(edge.ID)
	}

	return SerializedEdge{
		Points:               points,
		ID:                   serializedID,
		OriginalID:           originalID,
		IsInvisible:          edge.IsInvisible,
		Style:                serializeStyle(edge.Style),
		D2ID:                 d2ID,
		FromNode:             layoutgraph.NodeIDForSerialization(edge.From),
		ToNode:               layoutgraph.NodeIDForSerialization(edge.To),
		SourceArrowhead:      string(edge.SourceArrowhead),
		TargetArrowhead:      string(edge.TargetArrowhead),
		SourceArrowheadLabel: serializeLabel(edge.SourceArrowheadLabel),
		TargetArrowheadLabel: serializeLabel(edge.TargetArrowheadLabel),
		MinWidth:             edge.MinWidth,
		MinHeight:            edge.MinHeight,
		LabelPercentage:      edge.LabelPercentage,
		Label:                serializeLabel(edge.Label),
		FromTableColumnIndex: fromIndex,
		ToTableColumnIndex:   toIndex,
	}, nil
}

func serializeTree(t *layoutgraph.Tree, edgeIDs *edgeSerializationIDs, guard *limits.WorkGuard) (SerializedTree, error) {
	if t == nil {
		return SerializedTree{}, fmt.Errorf("cannot serialize a nil tree")
	}

	// Use explicit frames instead of recursive calls. validateEngineGraph has
	// already rejected cycles and excessive depth, while the guard bounds every
	// child occurrence (including repeated shared subtrees).
	type frame struct {
		tree  *layoutgraph.Tree
		out   *SerializedTree
		child int
	}
	result := SerializedTree{Children: make([]SerializedTree, len(t.Children))}
	stack := []frame{{tree: t, out: &result}}
	for len(stack) > 0 {
		current := &stack[len(stack)-1]
		if current.child < len(current.tree.Children) {
			child := current.tree.Children[current.child]
			childOut := &current.out.Children[current.child]
			current.child++
			if child == nil {
				return SerializedTree{}, fmt.Errorf("cannot serialize a nil tree child")
			}
			if err := guard.Step(); err != nil {
				return SerializedTree{}, err
			}
			childOut.Children = make([]SerializedTree, len(child.Children))
			stack = append(stack, frame{tree: child, out: childOut})
			continue
		}

		node, err := serializeNode(current.tree.Node, guard)
		if err != nil {
			return SerializedTree{}, err
		}
		if current.tree.SentinelEdge == nil {
			return SerializedTree{}, fmt.Errorf("cannot serialize a tree with a nil sentinel edge")
		}
		edge, err := serializeEdge(current.tree.SentinelEdge, edgeIDs, guard)
		if err != nil {
			return SerializedTree{}, err
		}
		current.out.Node = node
		current.out.SentinelEdge = edge
		current.out.Orientation = int(current.tree.Orientation)
		stack = stack[:len(stack)-1]
	}
	return result, nil
}

func deserializeTreeNode(g *layoutgraph.Graph, serializedTree *SerializedTree, parent *layoutgraph.Tree, nodesByID map[layoutgraph.EntityID]*layoutgraph.Node, edgesByID map[layoutgraph.EntityID]*layoutgraph.Edge, guard *limits.WorkGuard) (*layoutgraph.Tree, error) {
	if err := guard.Step(); err != nil {
		return nil, err
	}
	node, exists := nodesByID[serializedTree.Node.ID]
	if !exists {
		var err error
		node, err = deserializeNode(g, serializedTree.Node, guard)
		if err != nil {
			return nil, err
		}
		for _, nearID := range serializedTree.Node.Nears {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			near, ok := nodesByID[nearID]
			if !ok {
				return nil, fmt.Errorf("could not find tree node near of ID: %v", nearID)
			}
			node.AddNear(near)
		}
		nodesByID[node.ID] = node
	}

	edge, exists := edgesByID[serializedTree.SentinelEdge.ID]
	if !exists {
		var err error
		edge, err = deserializeEdge(serializedTree.SentinelEdge, nodesByID, guard)
		if err != nil {
			return nil, err
		}
		edgesByID[serializedTree.SentinelEdge.ID] = edge
	}

	tree := &layoutgraph.Tree{
		Node:         node,
		SentinelEdge: edge,
		Parent:       parent,
		Orientation:  geo.Orientation(serializedTree.Orientation),
	}

	return tree, nil
}

func deserializeTree(g *layoutgraph.Graph, serializedRoot *SerializedTree, nodesByID map[layoutgraph.EntityID]*layoutgraph.Node, edgesByID map[layoutgraph.EntityID]*layoutgraph.Edge, guard *limits.WorkGuard) (*layoutgraph.Tree, error) {
	root, err := deserializeTreeNode(g, serializedRoot, nil, nodesByID, edgesByID, guard)
	if err != nil {
		return nil, err
	}
	type frame struct {
		serialized *SerializedTree
		tree       *layoutgraph.Tree
		nextChild  int
	}
	stack := []frame{{serialized: serializedRoot, tree: root}}
	for len(stack) > 0 {
		current := &stack[len(stack)-1]
		if current.nextChild >= len(current.serialized.Children) {
			stack = stack[:len(stack)-1]
			continue
		}
		serializedChild := &current.serialized.Children[current.nextChild]
		current.nextChild++
		child, err := deserializeTreeNode(g, serializedChild, current.tree, nodesByID, edgesByID, guard)
		if err != nil {
			return nil, err
		}
		current.tree.Children = append(current.tree.Children, child)
		stack = append(stack, frame{serialized: serializedChild, tree: child})
	}
	return root, nil
}

// Unmarshal decodes fixture JSON into out and isolates unexpected internal
// invariant panics behind a stable error.
func Unmarshal(ctx context.Context, data []byte, out *layoutgraph.Graph) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("deserialize graph failed due to an internal invariant")
		}
	}()

	var sg *SerializedGraph
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&sg); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err = decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("graph JSON contains trailing data")
		}
		return err
	}

	return Deserialize(ctx, sg, out)
}

func deserializeEdge(edge SerializedEdge, nodesByID map[layoutgraph.EntityID]*layoutgraph.Node, guard *limits.WorkGuard) (*layoutgraph.Edge, error) {
	if err := guard.Step(); err != nil {
		return nil, err
	}
	if nodesByID[edge.FromNode] == nil {
		return nil, fmt.Errorf("error while deserializing: edge %v has from node %v which is missing from g.Nodes", edge.ID, edge.FromNode)
	}
	if nodesByID[edge.ToNode] == nil {
		return nil, fmt.Errorf("error while deserializing: edge %v has to node %v which is missing from g.Nodes", edge.ID, edge.ToNode)
	}
	newedge := layoutgraph.NewEdge(nodesByID[edge.FromNode], nodesByID[edge.ToNode])
	newedge.ID = edge.ID
	if edge.OriginalID != nil {
		newedge.ID = *edge.OriginalID
	}
	if edge.D2ID != nil {
		newedge.D2ID = new(*edge.D2ID)
	}
	newedge.SourceArrowhead = layoutgraph.Arrowhead(edge.SourceArrowhead)
	newedge.TargetArrowhead = layoutgraph.Arrowhead(edge.TargetArrowhead)
	var err error
	newedge.SourceArrowheadLabel, err = deserializeLabel(edge.SourceArrowheadLabel, guard)
	if err != nil {
		return nil, err
	}
	newedge.TargetArrowheadLabel, err = deserializeLabel(edge.TargetArrowheadLabel, guard)
	if err != nil {
		return nil, err
	}
	newedge.MinWidth = edge.MinWidth
	newedge.MinHeight = edge.MinHeight
	newedge.LabelPercentage = edge.LabelPercentage
	if edge.FromTableColumnIndex != nil {
		newedge.FromTableColumnIndex = new(*edge.FromTableColumnIndex)
	}
	if edge.ToTableColumnIndex != nil {
		newedge.ToTableColumnIndex = new(*edge.ToTableColumnIndex)
	}
	for _, point := range edge.Points {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		newedge.Points = append(newedge.Points, deserializePoint(point))
	}
	newedge.Label, err = deserializeLabel(edge.Label, guard)
	if err != nil {
		return nil, err
	}
	newedge.IsInvisible = edge.IsInvisible
	newedge.Style = deserializeStyle(edge.Style)

	return newedge, nil
}

func serializeStyle(style layoutgraph.EdgeStyle) SerializedStyle {
	cloneScalar := func(scalar *layoutgraph.StyleScalar) *SerializedScalar {
		if scalar == nil {
			return nil
		}
		return &SerializedScalar{Value: scalar.Value}
	}
	return SerializedStyle{
		Opacity:       cloneScalar(style.Opacity),
		Stroke:        cloneScalar(style.Stroke),
		Fill:          cloneScalar(style.Fill),
		FillPattern:   cloneScalar(style.FillPattern),
		StrokeWidth:   cloneScalar(style.StrokeWidth),
		StrokeDash:    cloneScalar(style.StrokeDash),
		BorderRadius:  cloneScalar(style.BorderRadius),
		Shadow:        cloneScalar(style.Shadow),
		ThreeDee:      cloneScalar(style.ThreeDee),
		Multiple:      cloneScalar(style.Multiple),
		Font:          cloneScalar(style.Font),
		FontSize:      cloneScalar(style.FontSize),
		FontColor:     cloneScalar(style.FontColor),
		Animated:      cloneScalar(style.Animated),
		Bold:          cloneScalar(style.Bold),
		Italic:        cloneScalar(style.Italic),
		Underline:     cloneScalar(style.Underline),
		Filled:        cloneScalar(style.Filled),
		DoubleBorder:  cloneScalar(style.DoubleBorder),
		TextTransform: cloneScalar(style.TextTransform),
	}
}

func deserializeStyle(style SerializedStyle) layoutgraph.EdgeStyle {
	cloneScalar := func(scalar *SerializedScalar) *layoutgraph.StyleScalar {
		if scalar == nil {
			return nil
		}
		return &layoutgraph.StyleScalar{Value: scalar.Value}
	}
	return layoutgraph.EdgeStyle{
		Opacity:       cloneScalar(style.Opacity),
		Stroke:        cloneScalar(style.Stroke),
		Fill:          cloneScalar(style.Fill),
		FillPattern:   cloneScalar(style.FillPattern),
		StrokeWidth:   cloneScalar(style.StrokeWidth),
		StrokeDash:    cloneScalar(style.StrokeDash),
		BorderRadius:  cloneScalar(style.BorderRadius),
		Shadow:        cloneScalar(style.Shadow),
		ThreeDee:      cloneScalar(style.ThreeDee),
		Multiple:      cloneScalar(style.Multiple),
		Font:          cloneScalar(style.Font),
		FontSize:      cloneScalar(style.FontSize),
		FontColor:     cloneScalar(style.FontColor),
		Animated:      cloneScalar(style.Animated),
		Bold:          cloneScalar(style.Bold),
		Italic:        cloneScalar(style.Italic),
		Underline:     cloneScalar(style.Underline),
		Filled:        cloneScalar(style.Filled),
		DoubleBorder:  cloneScalar(style.DoubleBorder),
		TextTransform: cloneScalar(style.TextTransform),
	}
}

func deserializeNode(g *layoutgraph.Graph, node SerializedNode, guard *limits.WorkGuard) (*layoutgraph.Node, error) {
	if err := guard.Step(); err != nil {
		return nil, err
	}
	for range node.Nears {
		if err := guard.Step(); err != nil {
			return nil, err
		}
	}
	n := layoutgraph.NewNode(node.ID, node.Width, node.Height)

	if node.D2ID != nil {
		n.D2ID = new(*node.D2ID)
	}
	n.TopLeft = deserializePoint(node.TopLeft)
	n.FixedTopLeft = deserializePoint(node.FixedTopLeft)
	n.Graph = g
	if node.FontSize != nil {
		n.FontSize = new(*node.FontSize)
	}
	if node.DesiredWidth != nil {
		n.DesiredWidth = new(*node.DesiredWidth)
	}
	if node.DesiredHeight != nil {
		n.DesiredHeight = new(*node.DesiredHeight)
	}
	n.SetShape(node.ShapeType)
	n.SetNumColumns(node.NumColumns)
	var err error
	n.Label, err = deserializeLabel(node.Label, guard)
	if err != nil {
		return nil, err
	}
	n.Icon, err = deserializeIcon(node.Icon, guard)
	if err != nil {
		return nil, err
	}
	n.Is3D = node.Is3D
	n.IsMultiple = node.IsMultiple
	n.ForceHierarchy = node.ForceHierarchy
	n.IsInvisible = node.IsInvisible
	return n, nil
}

// Deserialize reconstructs graph while observing ctx throughout
// its topology walks. It publishes the reconstructed graph atomically after
// every fallible operation and the final context check have succeeded.
func Deserialize(ctx context.Context, graph *SerializedGraph, out *layoutgraph.Graph) error {
	if graph == nil {
		return fmt.Errorf("cannot deserialize a nil graph")
	}
	if out == nil {
		return fmt.Errorf("cannot deserialize into a nil graph")
	}
	if err := validateSerializedGraphForDeserialization(ctx, graph); err != nil {
		return err
	}
	staged := &layoutgraph.Graph{}
	if err := deserializeGraphContext(ctx, graph, staged, out); err != nil {
		return err
	}
	layoutgraph.PublishDeserializedGraph(out, staged, graph.CrossingCost, graph.TurnCost, graph.NonCenterPortCost)
	return nil
}

func deserializeGraphContext(ctx context.Context, graph *SerializedGraph, destination, owner *layoutgraph.Graph) error {
	guard, err := newWorkGuard(ctx, "DeserializeGraph")
	if err != nil {
		return err
	}

	nodes := []*layoutgraph.Node{}
	nodesByID := map[layoutgraph.EntityID]*layoutgraph.Node{}

	for _, node := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if _, duplicate := nodesByID[node.ID]; duplicate {
			return fmt.Errorf("error while deserializing: duplicate node ID %v", node.ID)
		}
		newNode, err := deserializeNode(owner, node, guard)
		if err != nil {
			return err
		}
		nodesByID[node.ID] = newNode
		nodes = append(nodes, newNode)
	}

	edges := []*layoutgraph.Edge{}
	edgesByID := map[layoutgraph.EntityID]*layoutgraph.Edge{}

	for _, edge := range graph.Edges {
		if err := guard.Step(); err != nil {
			return err
		}
		if _, duplicate := edgesByID[edge.ID]; duplicate {
			return fmt.Errorf("error while deserializing: duplicate edge ID %v", edge.ID)
		}
		newedge, err := deserializeEdge(edge, nodesByID, guard)
		if err != nil {
			return err
		}
		layoutgraph.AddDecodedEdge(newedge.From, newedge)
		if !newedge.IsLoop() {
			layoutgraph.AddDecodedEdge(newedge.To, newedge)
		}
		edgesByID[edge.ID] = newedge
		edges = append(edges, newedge)
	}

	// deserialize in a fixed order
	containerOrder := make([]layoutgraph.EntityID, 0, len(graph.Containers))
	for id := range graph.Containers {
		if err := guard.Step(); err != nil {
			return err
		}
		containerOrder = append(containerOrder, id)
	}
	slices.Sort(containerOrder)

	containers := map[*layoutgraph.Node][]*layoutgraph.Node{}
	for _, containerID := range containerOrder {
		if err := guard.Step(); err != nil {
			return err
		}
		container, hasContainer := nodesByID[containerID]
		if containerID != 0 && !hasContainer {
			return fmt.Errorf("error while deserializing: missing container node %v", containerID)
		}

		containerNodes := []*layoutgraph.Node{}
		for _, nID := range graph.Containers[containerID] {
			if err := guard.Step(); err != nil {
				return err
			}
			n, has := nodesByID[nID]
			if !has {
				return fmt.Errorf("error while deserializing: container %v has node %v which is missing from g.Nodes", containerID, nID)
			}
			containerNodes = append(containerNodes, n)
			n.Container = container
		}

		containers[container] = containerNodes
		if container != nil {
			layoutgraph.MarkDecodedContainer(container)
		}
	}

	hubOrder := make([]layoutgraph.EntityID, 0, len(graph.Hubs))
	for id := range graph.Hubs {
		if err := guard.Step(); err != nil {
			return err
		}
		hubOrder = append(hubOrder, id)
	}
	slices.Sort(hubOrder)

	clusterOrder := make([]layoutgraph.EntityID, 0, len(graph.Clusters))
	for id := range graph.Clusters {
		if err := guard.Step(); err != nil {
			return err
		}
		clusterOrder = append(clusterOrder, id)
	}
	slices.Sort(clusterOrder)

	clusters := map[*layoutgraph.Node]*layoutgraph.Cluster{}
	for _, id := range clusterOrder {
		if err := guard.Step(); err != nil {
			return err
		}
		var vessel *layoutgraph.Node
		if n, in := nodesByID[id]; in {
			vessel = n
			layoutgraph.MarkDecodedClusterVessel(vessel)
		} else if cv, in := graph.ClusterVessels[id]; in {
			if cv.ID != id {
				return fmt.Errorf("error while deserializing: cluster vessel key %v does not match node ID %v", id, cv.ID)
			}
			var err error
			vessel, err = deserializeNode(owner, cv, guard)
			if err != nil {
				return err
			}
			layoutgraph.MarkDecodedClusterVessel(vessel)
			vessel.Graph = nil
			nodesByID[id] = vessel
		}
		if vessel == nil {
			return fmt.Errorf("error while deserializing: cluster %v has no node or serialized vessel", id)
		}
		serializedCluster := graph.Clusters[id]
		if serializedCluster.Container != 0 {
			if _, ok := nodesByID[serializedCluster.Container]; !ok {
				return fmt.Errorf("error while deserializing: cluster %v references missing container %v", id, serializedCluster.Container)
			}
		}

		clusterNodes := []*layoutgraph.Node{}
		for _, nID := range serializedCluster.Nodes {
			if err := guard.Step(); err != nil {
				return err
			}
			n, has := nodesByID[nID]
			if !has {
				return fmt.Errorf("error while deserializing: cluster %v has node %v which is missing from g.Nodes", id, nID)
			}
			clusterNodes = append(clusterNodes, n)
		}

		cluster := &layoutgraph.Cluster{
			Vessel:             vessel,
			Arrangement:        layoutgraph.ClusterArrangement(serializedCluster.Arrangement),
			DesiredArrangement: layoutgraph.ClusterArrangement(serializedCluster.DesiredArrangement),
			Graph:              owner,
			Container:          nodesByID[serializedCluster.Container],
			Nodes:              clusterNodes,
			Padding:            serializedCluster.Padding,
			FixedSize:          serializedCluster.FixedSize,
		}
		clusters[vessel] = cluster
		for _, n := range clusterNodes {
			if err := guard.Step(); err != nil {
				return err
			}
			n.Cluster = cluster
		}
	}

	sequenceOrder := make([]layoutgraph.EntityID, 0, len(graph.Sequences))
	for vessel := range graph.Sequences {
		if err := guard.Step(); err != nil {
			return err
		}
		sequenceOrder = append(sequenceOrder, vessel)
	}
	slices.Sort(sequenceOrder)

	sequences := map[*layoutgraph.Node]*layoutgraph.Sequence{}
	for _, id := range sequenceOrder {
		if err := guard.Step(); err != nil {
			return err
		}
		var vessel *layoutgraph.Node
		if n, in := nodesByID[id]; in {
			vessel = n
		} else if sv, in := graph.SequenceVessels[id]; in {
			if sv.ID != id {
				return fmt.Errorf("error while deserializing: sequence vessel key %v does not match node ID %v", id, sv.ID)
			}
			var err error
			vessel, err = deserializeNode(owner, sv, guard)
			if err != nil {
				return err
			}
			vessel.Graph = nil
			nodesByID[id] = vessel
		}
		if vessel == nil {
			return fmt.Errorf("error while deserializing: sequence %v has no node or serialized vessel", id)
		}
		seq := graph.Sequences[id]
		if len(seq.Nodes) < 2 {
			return fmt.Errorf("error while deserializing: sequence %v has %d steps; want at least 2", id, len(seq.Nodes))
		}
		if seq.Container != 0 {
			if _, ok := nodesByID[seq.Container]; !ok {
				return fmt.Errorf("error while deserializing: sequence %v references missing container %v", id, seq.Container)
			}
		}

		sequenceNodes := []*layoutgraph.Node{}
		for _, nID := range seq.Nodes {
			if err := guard.Step(); err != nil {
				return err
			}
			n, has := nodesByID[nID]
			if !has {
				return fmt.Errorf("error while deserializing: sequence %v has node %v which is missing from g.Nodes", id, nID)
			}
			sequenceNodes = append(sequenceNodes, n)
		}

		sequence := &layoutgraph.Sequence{
			Vessel:    vessel,
			Graph:     owner,
			Container: nodesByID[seq.Container],
			Nodes:     sequenceNodes,
		}
		sequences[vessel] = sequence
		for _, n := range sequenceNodes {
			if err := guard.Step(); err != nil {
				return err
			}
			n.Sequence = sequence
		}
	}

	hubs := map[*layoutgraph.Node][]*layoutgraph.Node{}
	for _, id := range hubOrder {
		if err := guard.Step(); err != nil {
			return err
		}
		hub, ok := nodesByID[id]
		if !ok {
			return fmt.Errorf("error while deserializing: hub %v is missing from g.Nodes", id)
		}

		spokes := []*layoutgraph.Node{}
		for _, nID := range graph.Hubs[id] {
			if err := guard.Step(); err != nil {
				return err
			}
			n, has := nodesByID[nID]
			if !has {
				return fmt.Errorf("error while deserializing: hub %v has node %v which is missing from g.Nodes", id, nID)
			}
			spokes = append(spokes, n)
		}

		hubs[hub] = spokes
	}

	for _, id := range clusterOrder {
		if err := guard.Step(); err != nil {
			return err
		}
		edgeAbductions := []*layoutgraph.EdgeAbduction{}
		for _, val := range graph.Clusters[id].EdgeAbductions {
			if err := guard.Step(); err != nil {
				return err
			}
			edge, has := edgesByID[val.Edge]
			if !has {
				return fmt.Errorf("couldn't find edge abduction edge %v for cluster %v", val.Edge, id)
			}
			originallyFrom, has := nodesByID[val.OriginallyFromNode]
			if val.OriginallyFromNode != 0 && !has {
				return fmt.Errorf("couldn't find edge abduction OriginallyFrom %v for cluster %v", val.OriginallyFromNode, id)
			}
			originallyTo, has := nodesByID[val.OriginallyToNode]
			if val.OriginallyToNode != 0 && !has {
				return fmt.Errorf("couldn't find edge abduction OriginallyTo %v for cluster %v", val.OriginallyToNode, id)
			}
			currentFrom, has := nodesByID[val.CurrentFromNode]
			if val.CurrentFromNode != 0 && !has {
				return fmt.Errorf("couldn't find edge abduction CurrentFrom %v for cluster %v", val.CurrentFromNode, id)
			}
			currentTo, has := nodesByID[val.CurrentToNode]
			if val.CurrentToNode != 0 && !has {
				return fmt.Errorf("couldn't find edge abduction CurrentTo %v for cluster %v", val.CurrentToNode, id)
			}

			edgeAbductions = append(edgeAbductions, &layoutgraph.EdgeAbduction{
				Edge:           edge,
				OriginallyFrom: originallyFrom,
				OriginallyTo:   originallyTo,
				CurrentFrom:    currentFrom,
				CurrentTo:      currentTo,
			})
		}
		vessel := nodesByID[id]
		clusters[vessel].EdgeAbductions = edgeAbductions
	}

	for _, id := range sequenceOrder {
		if err := guard.Step(); err != nil {
			return err
		}
		edgeAbductions := []*layoutgraph.EdgeAbduction{}
		for _, val := range graph.Sequences[id].EdgeAbductions {
			if err := guard.Step(); err != nil {
				return err
			}
			edge, has := edgesByID[val.Edge]
			if !has {
				return fmt.Errorf("couldn't find edge abduction edge %v for sequence %v", val.Edge, id)
			}
			originallyFrom, has := nodesByID[val.OriginallyFromNode]
			if val.OriginallyFromNode != 0 && !has {
				return fmt.Errorf("couldn't find edge abduction OriginallyFrom %v for sequence %v", val.OriginallyFromNode, id)
			}
			originallyTo, has := nodesByID[val.OriginallyToNode]
			if val.OriginallyToNode != 0 && !has {
				return fmt.Errorf("couldn't find edge abduction OriginallyTo %v for sequence %v", val.OriginallyToNode, id)
			}
			currentFrom, has := nodesByID[val.CurrentFromNode]
			if val.CurrentFromNode != 0 && !has {
				return fmt.Errorf("couldn't find edge abduction CurrentFrom %v for sequence %v", val.CurrentFromNode, id)
			}
			currentTo, has := nodesByID[val.CurrentToNode]
			if val.CurrentToNode != 0 && !has {
				return fmt.Errorf("couldn't find edge abduction CurrentTo %v for sequence %v", val.CurrentToNode, id)
			}

			edgeAbductions = append(edgeAbductions, &layoutgraph.EdgeAbduction{
				Edge:           edge,
				OriginallyFrom: originallyFrom,
				OriginallyTo:   originallyTo,
				CurrentFrom:    currentFrom,
				CurrentTo:      currentTo,
			})
		}

		vessel := nodesByID[id]
		sequences[vessel].EdgeAbductions = edgeAbductions
	}

	trees := make(map[*layoutgraph.Node][]*layoutgraph.Tree)
	for rootSentinelID, serializedTrees := range graph.Trees {
		if err := guard.Step(); err != nil {
			return err
		}
		rootSentinel, ok := nodesByID[rootSentinelID]
		if rootSentinelID != 0 && !ok {
			return fmt.Errorf("error while deserializing: tree references missing root sentinel %v", rootSentinelID)
		}
		for treeIndex := range serializedTrees {
			t, err := deserializeTree(owner, &serializedTrees[treeIndex], nodesByID, edgesByID, guard)
			if err != nil {
				return err
			}
			trees[rootSentinel] = append(trees[rootSentinel], t)
		}
	}

	for _, node := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		for _, nearID := range node.Nears {
			if err := guard.Step(); err != nil {
				return err
			}
			near, ok := nodesByID[nearID]
			if !ok {
				return fmt.Errorf("could not find node %v near node %v", node.ID, nearID)
			}
			nodesByID[node.ID].AddNear(near)
		}
	}

	for _, serialized := range graph.Hierarchies {
		if err := guard.Step(); err != nil {
			return err
		}
		levels := make(map[*layoutgraph.Node]int)
		hierarchy := layoutgraph.NewDecodedHierarchy(levels)
		for nodeID, level := range serialized.Levels {
			if err := guard.Step(); err != nil {
				return err
			}
			node, ok := nodesByID[nodeID]
			if !ok {
				return fmt.Errorf("error while deserializing: hierarchy references missing node %v", nodeID)
			}
			levels[node] = level
			node.Hierarchy = hierarchy
		}

	}

	directions := map[*layoutgraph.Node]geo.Orientation{}
	for nodeID, dirString := range graph.Directions {
		if err := guard.Step(); err != nil {
			return err
		}
		container, ok := nodesByID[nodeID]
		if nodeID != 0 && !ok {
			return fmt.Errorf("error while deserializing: direction references missing container %v", nodeID)
		}
		directions[container] = deserializeDirection(dirString)
	}

	filtered := make([]*layoutgraph.Node, 0, len(nodes))
	for _, n := range nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if layoutgraph.IsActiveClusterForSerialization(n.Cluster) {
			continue
		}
		if layoutgraph.IsActiveSequenceForSerialization(n.Sequence) {
			continue
		}
		filtered = append(filtered, n)
	}
	// Derive every fallible auxiliary index before publishing any state to out.
	// This keeps cancellation and malformed input atomic for callers that
	// deserialize over an existing graph.
	nodeToTree, err := layoutgraph.BuildNodeToTreeForDeserialization(trees, guard)
	if err != nil {
		return err
	}
	if err := guard.Finish(); err != nil {
		return err
	}

	destination.Nodes = filtered
	destination.Edges = edges
	destination.CellSize = graph.CellSize
	destination.IsRootHierarchy = graph.IsRootHierarchy
	destination.Containers = containers
	destination.Hubs = hubs
	destination.Clusters = clusters
	destination.Sequences = sequences
	destination.Trees = trees
	destination.NodeToTree = nodeToTree
	destination.CommonUncleSiblings = nil
	destination.Directions = directions
	return nil
}

// Marshal encodes a graph in the internal fixture representation.
func Marshal(ctx context.Context, g *layoutgraph.Graph) ([]byte, error) {
	sg, err := Serialize(ctx, g)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sg)
}
