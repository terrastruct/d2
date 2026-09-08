package graphjson

import (
	"context"
	"fmt"
	"reflect"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

const (
	maxEngineNodes              = limits.MaxEngineNodes
	maxEngineEdges              = limits.MaxEngineEdges
	maxEngineTopologyReferences = layoutgraph.MaxSerializationTopologyReferences
	maxEngineRoutePoints        = layoutgraph.MaxSerializationRoutePoints
	maxEngineTopologyDepth      = layoutgraph.MaxTopologyDepth
	maxEnginePreflightWork      = layoutgraph.MaxSerializationPreflightWork
)

// validateSerializedGraphForDeserialization bounds and validates the value
// topology before reconstruction allocates runtime records or mutates any
// caller-visible object. This lower-level guard protects direct internal graph
// reconstruction from malformed fixtures or developer-tool input.
func validateSerializedGraphForDeserialization(ctx context.Context, graph *SerializedGraph) error {
	guard, err := newWorkGuard(ctx, "DeserializeGraph preflight")
	if err != nil {
		return err
	}
	guard.SetLimit(maxEnginePreflightWork)
	// Reject hostile top-level slices before their lengths are used as map
	// capacity hints below. Aggregate records from trees and auxiliary vessels
	// remain bounded by addNode and addEdge during the iterative walk.
	if len(graph.Nodes) > maxEngineNodes {
		return fmt.Errorf("TALA DeserializeGraph node records exceed limit %d", maxEngineNodes)
	}
	if len(graph.Edges) > maxEngineEdges {
		return fmt.Errorf("TALA DeserializeGraph edge records exceed limit %d", maxEngineEdges)
	}

	var nodeRecords, edgeRecords, references, routePoints int64
	addBounded := func(total *int64, count, limit int64, kind string) error {
		if count < 0 || count > limit-*total {
			return fmt.Errorf("TALA DeserializeGraph %s exceed limit %d", kind, limit)
		}
		*total += count
		return guard.Check()
	}
	addReferences := func(count int, kind string) error {
		return addBounded(&references, int64(count), maxEngineTopologyReferences, kind+" references")
	}
	addRoutePoints := func(count int) error {
		return addBounded(&routePoints, int64(count), maxEngineRoutePoints, "route points")
	}

	nodesByID := make(map[layoutgraph.EntityID]SerializedNode)
	topLevelNodeIDs := make(map[layoutgraph.EntityID]struct{}, len(graph.Nodes))
	addNode := func(node *SerializedNode, location string) error {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := addBounded(&nodeRecords, 1, maxEngineNodes, "node records"); err != nil {
			return err
		}
		if node.ID == 0 {
			return fmt.Errorf("TALA DeserializeGraph %s uses reserved node ID 0", location)
		}
		if previous, exists := nodesByID[node.ID]; exists {
			if !reflect.DeepEqual(previous, *node) {
				return fmt.Errorf("TALA DeserializeGraph node ID %d has inconsistent repeated records", node.ID)
			}
		} else {
			nodesByID[node.ID] = *node
		}
		return addReferences(len(node.Nears), location)
	}

	edgesBySerializedID := make(map[layoutgraph.EntityID]SerializedEdge)
	effectiveEdgeIDs := make(map[layoutgraph.EntityID]layoutgraph.EntityID)
	topLevelEdgeIDs := make(map[layoutgraph.EntityID]struct{}, len(graph.Edges))
	addEdge := func(edge *SerializedEdge, location string) error {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := addBounded(&edgeRecords, 1, maxEngineEdges, "edge records"); err != nil {
			return err
		}
		if edge.ID == 0 {
			return fmt.Errorf("TALA DeserializeGraph %s uses reserved serialized edge ID 0", location)
		}
		effectiveID := edge.ID
		if edge.OriginalID != nil {
			if edge.ID >= 0 || *edge.OriginalID != 0 {
				return fmt.Errorf(
					"TALA DeserializeGraph %s has invalid original edge ID %d for serialized ID %d",
					location,
					*edge.OriginalID,
					edge.ID,
				)
			}
			effectiveID = *edge.OriginalID
		}
		if effectiveID != 0 {
			if previousSerializedID, exists := effectiveEdgeIDs[effectiveID]; exists && previousSerializedID != edge.ID {
				return fmt.Errorf(
					"TALA DeserializeGraph %s effective edge ID %d is also represented by serialized ID %d",
					location,
					effectiveID,
					previousSerializedID,
				)
			}
			effectiveEdgeIDs[effectiveID] = edge.ID
		}
		if previous, exists := edgesBySerializedID[edge.ID]; exists {
			if !equalSerializedEdges(previous, *edge) {
				return fmt.Errorf("TALA DeserializeGraph edge ID %d has inconsistent repeated records", edge.ID)
			}
		} else {
			edgesBySerializedID[edge.ID] = *edge
		}
		return addRoutePoints(len(edge.Points))
	}

	if err := addReferences(len(graph.Containers), "container map"); err != nil {
		return err
	}
	if err := addReferences(len(graph.Hubs), "hub map"); err != nil {
		return err
	}
	if err := addReferences(len(graph.Clusters), "cluster map"); err != nil {
		return err
	}
	if err := addReferences(len(graph.Sequences), "sequence map"); err != nil {
		return err
	}
	if err := addReferences(len(graph.ClusterVessels), "cluster vessel map"); err != nil {
		return err
	}
	if err := addReferences(len(graph.SequenceVessels), "sequence vessel map"); err != nil {
		return err
	}
	if err := addReferences(len(graph.Trees), "tree map"); err != nil {
		return err
	}
	if err := addReferences(len(graph.Hierarchies), "hierarchy list"); err != nil {
		return err
	}
	if err := addReferences(len(graph.Directions), "direction map"); err != nil {
		return err
	}

	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		if _, duplicate := topLevelNodeIDs[node.ID]; duplicate {
			return fmt.Errorf("TALA DeserializeGraph has duplicate node ID %d in top-level records", node.ID)
		}
		topLevelNodeIDs[node.ID] = struct{}{}
		if err := addNode(node, fmt.Sprintf("node %d", i)); err != nil {
			return err
		}
	}
	for i := range graph.Edges {
		edge := &graph.Edges[i]
		if _, duplicate := topLevelEdgeIDs[edge.ID]; duplicate {
			return fmt.Errorf("TALA DeserializeGraph has duplicate edge ID %d in top-level records", edge.ID)
		}
		topLevelEdgeIDs[edge.ID] = struct{}{}
		if err := addEdge(edge, fmt.Sprintf("edge %d", i)); err != nil {
			return err
		}
	}
	for id, children := range graph.Containers {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := addReferences(len(children), fmt.Sprintf("container %d", id)); err != nil {
			return err
		}
	}
	for id, spokes := range graph.Hubs {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := addReferences(len(spokes), fmt.Sprintf("hub %d", id)); err != nil {
			return err
		}
	}
	for id, node := range graph.ClusterVessels {
		if err := addNode(&node, fmt.Sprintf("cluster vessel %d", id)); err != nil {
			return err
		}
	}
	for id, node := range graph.SequenceVessels {
		if err := addNode(&node, fmt.Sprintf("sequence vessel %d", id)); err != nil {
			return err
		}
	}
	for id, cluster := range graph.Clusters {
		if err := guard.Step(); err != nil {
			return err
		}
		if cluster.Container == id {
			return fmt.Errorf("TALA DeserializeGraph cluster %d cannot contain itself", id)
		}
		if err := addReferences(len(cluster.Nodes)+len(cluster.EdgeAbductions), fmt.Sprintf("cluster %d", id)); err != nil {
			return err
		}
	}
	for id, sequence := range graph.Sequences {
		if err := guard.Step(); err != nil {
			return err
		}
		if sequence.Container == id {
			return fmt.Errorf("TALA DeserializeGraph sequence %d cannot contain itself", id)
		}
		if err := addReferences(len(sequence.Nodes)+len(sequence.EdgeAbductions), fmt.Sprintf("sequence %d", id)); err != nil {
			return err
		}
	}

	treeOwners := make(map[layoutgraph.EntityID]string)
	type treeFrame struct {
		tree     *SerializedTree
		depth    int
		location string
	}
	for rootID, roots := range graph.Trees {
		if err := addReferences(len(roots), fmt.Sprintf("tree root %d", rootID)); err != nil {
			return err
		}
		stack := make([]treeFrame, 0, len(roots))
		for i := len(roots) - 1; i >= 0; i-- {
			stack = append(stack, treeFrame{tree: &roots[i], depth: 1, location: fmt.Sprintf("tree %d[%d]", rootID, i)})
		}
		for len(stack) > 0 {
			frame := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if err := guard.Step(); err != nil {
				return err
			}
			if frame.depth > maxEngineTopologyDepth {
				return fmt.Errorf("TALA DeserializeGraph tree depth exceeds limit %d", maxEngineTopologyDepth)
			}
			if previous, exists := treeOwners[frame.tree.Node.ID]; exists {
				return fmt.Errorf(
					"TALA DeserializeGraph tree node %d has more than one tree owner (%s and %s)",
					frame.tree.Node.ID,
					previous,
					frame.location,
				)
			}
			if _, serializedBeforeTrees := nodesByID[frame.tree.Node.ID]; serializedBeforeTrees {
				if _, topLevel := topLevelNodeIDs[frame.tree.Node.ID]; !topLevel {
					return fmt.Errorf(
						"TALA DeserializeGraph tree node %d repeats a non-top-level node record",
						frame.tree.Node.ID,
					)
				}
			}
			treeOwners[frame.tree.Node.ID] = frame.location
			if err := addNode(&frame.tree.Node, frame.location+" node"); err != nil {
				return err
			}
			if err := addEdge(&frame.tree.SentinelEdge, frame.location+" sentinel edge"); err != nil {
				return err
			}
			if err := addReferences(len(frame.tree.Children), frame.location+" children"); err != nil {
				return err
			}
			for i := len(frame.tree.Children) - 1; i >= 0; i-- {
				stack = append(stack, treeFrame{
					tree:     &frame.tree.Children[i],
					depth:    frame.depth + 1,
					location: fmt.Sprintf("%s child %d", frame.location, i),
				})
			}
		}
	}
	hierarchyOwners := make(map[layoutgraph.EntityID]int)
	for i, hierarchy := range graph.Hierarchies {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := addReferences(len(hierarchy.Levels), fmt.Sprintf("hierarchy %d", i)); err != nil {
			return err
		}
		for nodeID := range hierarchy.Levels {
			if previous, exists := hierarchyOwners[nodeID]; exists {
				return fmt.Errorf("TALA DeserializeGraph hierarchy node %d appears in both hierarchy %d and hierarchy %d", nodeID, previous, i)
			}
			hierarchyOwners[nodeID] = i
		}
	}
	return guard.Finish()
}

func equalSerializedEdges(left, right SerializedEdge) bool {
	if !equalSerializedStyles(left.Style, right.Style) {
		return false
	}
	// Compare Style explicitly so every serialized scalar field remains part of the
	// equality check without coupling the remaining edge record to its
	// implementation.
	left.Style = SerializedEdge{}.Style
	right.Style = SerializedEdge{}.Style
	return reflect.DeepEqual(left, right)
}

func equalSerializedStyles(left, right SerializedStyle) bool {
	leftValue := reflect.ValueOf(left)
	rightValue := reflect.ValueOf(right)
	scalarType := reflect.TypeFor[*SerializedScalar]()
	for i := 0; i < leftValue.NumField(); i++ {
		if leftValue.Type().Field(i).Type != scalarType {
			return false
		}
		leftScalar, _ := leftValue.Field(i).Interface().(*SerializedScalar)
		rightScalar, _ := rightValue.Field(i).Interface().(*SerializedScalar)
		if leftScalar == nil || rightScalar == nil {
			if leftScalar != rightScalar {
				return false
			}
			continue
		}
		if leftScalar.Value != rightScalar.Value {
			return false
		}
	}
	return true
}
