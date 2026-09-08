// Package grouping discovers and manages layout groups that temporarily move
// as a single vessel.
package grouping

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"math/rand"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/shape"
)

func identifySequences(graph *layoutgraph.Graph, nodes []*layoutgraph.Node, guard *limits.WorkGuard) ([][]*layoutgraph.Node, error) {
	activeNodes := make(map[*layoutgraph.Node]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		activeNodes[node] = struct{}{}
	}
	var stepNodes []*layoutgraph.Node
	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if _, active := activeNodes[node]; !active || node.IsContainer() {
			continue
		}
		if node.Sequence != nil && !node.Sequence.IsActive() {
			continue
		}
		if node.FixedTopLeft != nil || !node.IsSequenceStep() {
			continue
		}
		stepNodes = append(stepNodes, node)
	}
	if len(stepNodes) <= 1 {
		return nil, nil
	}

	var sequences [][]*layoutgraph.Node
	steps := []*layoutgraph.Node{stepNodes[0]}
	for i := 1; i < len(stepNodes); i++ {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		previous, current := stepNodes[i-1], stepNodes[i]
		connected := false
		for _, edge := range previous.Edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if previous.Adjacent(edge) == current {
				connected = true
				break
			}
		}
		if connected {
			steps = append(steps, current)
		} else {
			if len(steps) > 1 {
				sequences = append(sequences, steps)
			}
			steps = []*layoutgraph.Node{current}
		}
	}
	if len(steps) > 1 {
		sequences = append(sequences, steps)
	}
	return sequences, nil
}

// SequenceDefiningEdges returns the IDs of edges consumed when step nodes are
// replaced by sequence vessels.
func SequenceDefiningEdges(ctx context.Context, graph *layoutgraph.Graph) (map[layoutgraph.EntityID]struct{}, error) {
	if err := layoutgraph.Validate(ctx, "GetSequenceDefiningEdges", graph); err != nil {
		return nil, err
	}
	guard, err := limits.NewWorkGuard(ctx, "GetSequenceDefiningEdges", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	containerOrder, err := graph.ContainerRDFSOrder(nil, guard)
	if err != nil {
		return nil, err
	}
	containerOrder = append(containerOrder, nil)
	sequenceEdges := make(map[layoutgraph.EntityID]struct{})
	for _, container := range containerOrder {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		sequences, err := identifySequences(graph, graph.Containers[container], guard)
		if err != nil {
			return nil, err
		}
		for _, steps := range sequences {
			for i := 1; i < len(steps); i++ {
				var connection *layoutgraph.Edge
				for _, edge := range steps[i-1].Edges {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					if (edge.From == steps[i-1] && edge.To == steps[i]) ||
						(edge.To == steps[i-1] && edge.From == steps[i]) {
						connection = edge
						break
					}
				}
				if connection == nil {
					return nil, fmt.Errorf("TALA sequence steps %d and %d have no defining edge", steps[i-1].ID, steps[i].ID)
				}
				sequenceEdges[connection.ID] = struct{}{}
			}
		}
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return sequenceEdges, nil
}

func isValidRememberedSequence(
	graph *layoutgraph.Graph,
	vessel *layoutgraph.Node,
	sequence *layoutgraph.Sequence,
	activeNodes map[*layoutgraph.Node]struct{},
	guard *limits.WorkGuard,
) (bool, error) {
	if vessel == nil || sequence == nil || sequence.IsActive() || sequence.Vessel != vessel || sequence.Graph != graph || len(sequence.Nodes) < 2 {
		return false, nil
	}
	if sequence.Container != nil {
		if _, active := activeNodes[sequence.Container]; !active {
			return false, nil
		}
	}
	children, hasContainer := graph.Containers[sequence.Container]
	if !hasContainer {
		return false, nil
	}
	childIndex := make(map[*layoutgraph.Node]int, len(children))
	duplicateChild := make(map[*layoutgraph.Node]struct{})
	for i, child := range children {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if _, seen := childIndex[child]; seen {
			duplicateChild[child] = struct{}{}
			continue
		}
		childIndex[child] = i
	}
	seen := make(map[*layoutgraph.Node]struct{}, len(sequence.Nodes))
	previousIndex := -1
	for _, node := range sequence.Nodes {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if node == nil {
			return false, nil
		}
		if _, duplicate := seen[node]; duplicate {
			return false, nil
		}
		seen[node] = struct{}{}
		if _, active := activeNodes[node]; !active || node.Graph != graph || !node.IsSequenceStep() || node.FixedTopLeft != nil {
			return false, nil
		}
		if node.Sequence != sequence || node.Container != sequence.Container {
			return false, nil
		}
		index, inContainer := childIndex[node]
		if _, duplicate := duplicateChild[node]; !inContainer || duplicate {
			return false, nil
		}
		if previousIndex >= 0 && index != previousIndex+1 {
			return false, nil
		}
		previousIndex = index
	}
	return true, nil
}

func clearRememberedSequenceMembership(sequence *layoutgraph.Sequence, guard *limits.WorkGuard) error {
	if sequence == nil {
		return nil
	}
	for _, node := range sequence.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		if node != nil && node.Sequence == sequence {
			node.Sequence = nil
		}
	}
	return nil
}

// AddSequences replaces each discovered step run with a temporary vessel.
func AddSequences(ctx context.Context, graph *layoutgraph.Graph, random *rand.Rand) (err error) {
	if err := layoutgraph.Validate(ctx, "AddSequences", graph); err != nil {
		return err
	}
	guard, err := limits.NewWorkGuard(ctx, "AddSequences", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureTopology:   true,
		CaptureEdgeRoutes: true,
	})
	if err := state.UpdateWithWorkGuard(graph, guard); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			layoutgraph.RestoreGraphState(graph, state)
		}
	}()

	remembered := make(map[*layoutgraph.Node][][]*layoutgraph.Node)
	rememberedIDByFirstStep := make(map[*layoutgraph.Node]layoutgraph.EntityID)
	reservedNodeIDs := make(map[layoutgraph.EntityID]struct{})
	activeNodes := make(map[*layoutgraph.Node]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return err
		}
		activeNodes[node] = struct{}{}
		reservedNodeIDs[node.ID] = struct{}{}
	}
	for vessel, cluster := range graph.Clusters {
		if err := guard.Step(); err != nil {
			return err
		}
		if vessel != nil {
			reservedNodeIDs[vessel.ID] = struct{}{}
		}
		if cluster != nil {
			for _, node := range cluster.Nodes {
				if err := guard.Step(); err != nil {
					return err
				}
				if node != nil {
					reservedNodeIDs[node.ID] = struct{}{}
				}
			}
		}
	}
	for vessel, sequence := range graph.Sequences {
		if err := guard.Step(); err != nil {
			return err
		}
		if vessel != nil {
			reservedNodeIDs[vessel.ID] = struct{}{}
		}
		if sequence != nil {
			for _, node := range sequence.Nodes {
				if err := guard.Step(); err != nil {
					return err
				}
				if node != nil {
					reservedNodeIDs[node.ID] = struct{}{}
				}
			}
		}
	}
	for _, vessel := range graph.SequenceOrder() {
		if err := guard.Step(); err != nil {
			return err
		}
		sequence := graph.Sequences[vessel]
		valid, err := isValidRememberedSequence(graph, vessel, sequence, activeNodes, guard)
		if err != nil {
			return err
		}
		if valid {
			remembered[sequence.Container] = append(remembered[sequence.Container], sequence.Nodes)
			rememberedIDByFirstStep[sequence.Nodes[0]] = vessel.ID
		} else if err := clearRememberedSequenceMembership(sequence, guard); err != nil {
			return err
		}
	}
	graph.Sequences = map[*layoutgraph.Node]*layoutgraph.Sequence{}

	containerOrder, err := graph.ContainerRDFSOrder(nil, guard)
	if err != nil {
		return err
	}
	for _, container := range append(containerOrder, nil) {
		if err := guard.Step(); err != nil {
			return err
		}
		groups, err := identifySequences(graph, graph.Containers[container], guard)
		if err != nil {
			return err
		}
		groups = append(groups, remembered[container]...)
		containerIndex := make(map[*layoutgraph.Node]int, len(graph.Containers[container]))
		for i, node := range graph.Containers[container] {
			if err := guard.Step(); err != nil {
				return err
			}
			containerIndex[node] = i
		}
		slices.SortStableFunc(groups, func(a, b []*layoutgraph.Node) int {
			return cmp.Compare(containerIndex[a[0]], containerIndex[b[0]])
		})

		generatedIDs := make([]layoutgraph.EntityID, len(groups))
		slotByID := make(map[layoutgraph.EntityID]int, len(groups))
		for i := range groups {
			if err := guard.Step(); err != nil {
				return err
			}
			generatedIDs[i] = random.Int63()
			slotByID[generatedIDs[i]] = i
		}
		orderedGroups := make([][]*layoutgraph.Node, len(groups))
		usedGroups := make([]bool, len(groups))
		for i, steps := range groups {
			if err := guard.Step(); err != nil {
				return err
			}
			if rememberedID, ok := rememberedIDByFirstStep[steps[0]]; ok {
				if slot, found := slotByID[rememberedID]; found && orderedGroups[slot] == nil {
					orderedGroups[slot] = steps
					usedGroups[i] = true
				}
			}
		}
		nextGroup := 0
		for i := range orderedGroups {
			if err := guard.Step(); err != nil {
				return err
			}
			if orderedGroups[i] != nil {
				continue
			}
			for nextGroup < len(groups) && usedGroups[nextGroup] {
				if err := guard.Step(); err != nil {
					return err
				}
				nextGroup++
			}
			orderedGroups[i] = groups[nextGroup]
			usedGroups[nextGroup] = true
		}

		for i, steps := range orderedGroups {
			if err := guard.Step(); err != nil {
				return err
			}
			id := generatedIDs[i]
			if rememberedID, ok := rememberedIDByFirstStep[steps[0]]; ok {
				id = rememberedID
			} else {
				id = nextAvailableNodeID(graph, id, reservedNodeIDs)
				reservedNodeIDs[id] = struct{}{}
			}
			addSequence(graph, buildSequence(steps, graph, container, id))
		}
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}

func buildSequence(steps []*layoutgraph.Node, graph *layoutgraph.Graph, container *layoutgraph.Node, id layoutgraph.EntityID) *layoutgraph.Sequence {
	maxHeight := 0.0
	for _, node := range steps {
		if node.Width <= shape.STEP_WEDGE_WIDTH {
			node.Width = 2 * shape.STEP_WEDGE_WIDTH
		}
		maxHeight = math.Max(maxHeight, node.Height)
	}
	for _, node := range steps {
		node.Height = maxHeight
	}
	for i := 1; i < len(steps); i++ {
		if edge := steps[i-1].ConnectionTo(steps[i]); edge != nil {
			graph.Disconnect(edge)
		}
	}
	sequence := &layoutgraph.Sequence{
		Vessel: layoutgraph.NewNode(id, 0, 0), Nodes: steps,
		Graph: graph, Container: container,
	}
	sequence.SyncGeometry()
	abductSequenceEdges(sequence)
	sequence.PlaceVessel()
	return sequence
}

func addSequence(graph *layoutgraph.Graph, sequence *layoutgraph.Sequence) {
	graph.AddNewNodeToContainer(sequence.Container, sequence.Vessel)
	for _, node := range sequence.Nodes {
		node.Sequence = sequence
	}
	var updatedContainerNodes []*layoutgraph.Node
	for _, child := range graph.Containers[sequence.Container] {
		if child.Sequence != sequence {
			updatedContainerNodes = append(updatedContainerNodes, child)
		}
	}
	graph.Containers[sequence.Container] = updatedContainerNodes
	for _, node := range sequence.Nodes {
		graph.RemoveNode(node)
		node.Container = nil
	}
	graph.Sequences[sequence.Vessel] = sequence
}

func abductSequenceEdges(sequence *layoutgraph.Sequence) {
	isStep := map[*layoutgraph.Node]struct{}{}
	for _, node := range sequence.Nodes {
		isStep[node] = struct{}{}
	}
	var abductions []*layoutgraph.EdgeAbduction
	for _, edge := range sequence.Graph.Edges {
		_, fromIsStep := isStep[edge.From]
		_, toIsStep := isStep[edge.To]
		if fromIsStep && toIsStep {
			continue
		}
		if fromIsStep {
			abductions = append(abductions, &layoutgraph.EdgeAbduction{
				Edge: edge, OriginallyFrom: edge.From, CurrentFrom: sequence.Vessel, CurrentTo: edge.To,
			})
			edge.Reconnect(sequence.Vessel, false)
		}
		if toIsStep {
			abductions = append(abductions, &layoutgraph.EdgeAbduction{
				Edge: edge, OriginallyTo: edge.To, CurrentTo: sequence.Vessel, CurrentFrom: edge.From,
			})
			edge.Reconnect(sequence.Vessel, true)
		}
	}
	sequence.EdgeAbductions = abductions
}

func nextAvailableNodeID(graph *layoutgraph.Graph, candidate layoutgraph.EntityID, unavailable map[layoutgraph.EntityID]struct{}) layoutgraph.EntityID {
	for {
		if _, found := unavailable[candidate]; !found && !hasNodeID(graph, candidate) {
			return candidate
		}
		if candidate == math.MaxInt64 {
			candidate = 0
		} else {
			candidate++
		}
	}
}

func hasNodeID(graph *layoutgraph.Graph, id layoutgraph.EntityID) bool {
	hasID := func(node *layoutgraph.Node) bool { return node != nil && node.ID == id }
	if slices.ContainsFunc(graph.Nodes, hasID) {
		return true
	}
	for vessel, cluster := range graph.Clusters {
		if hasID(vessel) || cluster != nil && slices.ContainsFunc(cluster.Nodes, hasID) {
			return true
		}
	}
	for vessel, sequence := range graph.Sequences {
		if hasID(vessel) || sequence != nil && slices.ContainsFunc(sequence.Nodes, hasID) {
			return true
		}
	}
	var treeHasID func(*layoutgraph.Tree) bool
	treeHasID = func(tree *layoutgraph.Tree) bool {
		return tree != nil && (hasID(tree.Node) || slices.ContainsFunc(tree.Children, treeHasID))
	}
	for sentinel, roots := range graph.Trees {
		if hasID(sentinel) || slices.ContainsFunc(roots, treeHasID) {
			return true
		}
	}
	return false
}
