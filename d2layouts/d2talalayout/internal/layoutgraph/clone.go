package layoutgraph

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

// Clone returns the layout graph projection used for an independent layout
// attempt. It deliberately copies the stable graph state, not transient
// optimizer state: derived caches, herd assignments, long-distance placement
// requirements, route diagnostics, and fixed-label bookkeeping are reset.
//
// The returned graph is published only after the complete bounded copy has
// succeeded. Every graph-owned node, cluster, and sequence is rebound to the
// returned graph; inactive cluster and sequence vessels remain detached.
func Clone(ctx context.Context, source *Graph) (*Graph, error) {
	staged := &Graph{}
	if err := cloneInto(ctx, source, staged); err != nil {
		return nil, err
	}
	return staged, nil
}

func cloneInto(ctx context.Context, source, staged *Graph) error {
	if ctx == nil {
		return fmt.Errorf("TALA CloneGraph requires a context")
	}
	if source == nil {
		return fmt.Errorf("cannot clone a nil graph")
	}
	if err := validateEngineGraph(ctx, "CloneGraph", source); err != nil {
		return err
	}
	guard, err := limits.NewWorkGuard(ctx, "CloneGraph", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}

	state := graphCloneState{
		source:            source,
		clone:             staged,
		guard:             guard,
		nodesByID:         make(map[EntityID]*Node),
		nodeRecordsByID:   make(map[EntityID]*Node),
		topLevelNodeIDs:   make(map[EntityID]struct{}),
		nodesBySource:     make(map[*Node]*Node),
		edgesBySource:     make(map[*Edge]*Edge),
		edgesByID:         make(map[EntityID]*Edge),
		clustersBySource:  make(map[*Cluster]*Cluster),
		sequencesBySource: make(map[*Sequence]*Sequence),
	}
	if err := state.copy(); err != nil {
		return err
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	return nil
}

type graphCloneState struct {
	source *Graph
	clone  *Graph
	guard  *limits.WorkGuard

	nodesByID       map[EntityID]*Node
	nodeRecordsByID map[EntityID]*Node
	// Cluster and sequence members join Graph.Nodes in the codec's top-level
	// node records; grouping vessels do not. Trees may reuse only the former.
	topLevelNodeIDs map[EntityID]struct{}
	nodesBySource   map[*Node]*Node
	nodeRecords     []*Node
	treeNodeRecords []*Node

	edgesBySource map[*Edge]*Edge
	edgesByID     map[EntityID]*Edge

	clustersBySource  map[*Cluster]*Cluster
	sequencesBySource map[*Sequence]*Sequence
}

func (state *graphCloneState) copy() error {
	inSerializedNodes := make(map[*Node]struct{})
	for _, node := range state.source.Nodes {
		if _, err := state.addNodeRecord(node); err != nil {
			return err
		}
		inSerializedNodes[node] = struct{}{}
	}
	for _, vessel := range state.source.ClusterOrder() {
		inSerializedNodes[vessel] = struct{}{}
		for _, node := range state.source.Clusters[vessel].Nodes {
			if _, alreadySerialized := inSerializedNodes[node]; alreadySerialized {
				continue
			}
			if _, err := state.addNodeRecord(node); err != nil {
				return err
			}
			inSerializedNodes[node] = struct{}{}
		}
	}
	for _, vessel := range state.source.SequenceOrder() {
		inSerializedNodes[vessel] = struct{}{}
		for _, node := range state.source.Sequences[vessel].Nodes {
			if _, alreadySerialized := inSerializedNodes[node]; alreadySerialized {
				continue
			}
			if _, err := state.addNodeRecord(node); err != nil {
				return err
			}
			inSerializedNodes[node] = struct{}{}
		}
	}

	state.clone.Edges = make([]*Edge, 0, len(state.source.Edges))
	for _, edge := range state.source.Edges {
		if err := state.guard.Step(); err != nil {
			return err
		}
		if edge == nil {
			return fmt.Errorf("cannot clone a nil edge")
		}
		if _, duplicate := state.edgesBySource[edge]; duplicate {
			return fmt.Errorf("cannot clone duplicate edge record %d", edge.ID)
		}
		if edge.ID != 0 {
			if _, duplicate := state.edgesByID[edge.ID]; duplicate {
				return fmt.Errorf("cannot clone duplicate edge ID %d", edge.ID)
			}
		}
		cloned, err := state.copyEdge(edge)
		if err != nil {
			return err
		}
		state.edgesBySource[edge] = cloned
		if edge.ID != 0 {
			state.edgesByID[edge.ID] = cloned
		}
		cloned.From.addEdge(cloned)
		if !cloned.isLoop() {
			cloned.To.addEdge(cloned)
		}
		state.clone.Edges = append(state.clone.Edges, cloned)
	}

	if err := state.copyContainers(); err != nil {
		return err
	}
	if err := state.copyClusters(); err != nil {
		return err
	}
	if err := state.copySequences(); err != nil {
		return err
	}
	if err := state.copyHubs(); err != nil {
		return err
	}
	if err := state.copyAbductions(); err != nil {
		return err
	}
	if err := state.copyTrees(); err != nil {
		return err
	}
	if err := state.copyNears(); err != nil {
		return err
	}
	if err := state.copyHierarchies(); err != nil {
		return err
	}
	if err := state.copyDirections(); err != nil {
		return err
	}

	filtered := make([]*Node, 0, len(state.nodeRecords))
	for _, sourceNode := range state.nodeRecords {
		if err := state.guard.Step(); err != nil {
			return err
		}
		node := state.nodesBySource[sourceNode]
		if node.Cluster != nil && node.Cluster.isActive() {
			continue
		}
		if node.Sequence != nil && node.Sequence.isActive() {
			continue
		}
		filtered = append(filtered, node)
	}
	state.clone.Nodes = filtered
	state.clone.CellSize = state.source.CellSize
	state.clone.IsRootHierarchy = state.source.IsRootHierarchy
	state.clone.CommonUncleSiblings = nil
	state.clone.edgeLengthCache = make(map[uint64]float64)

	state.source.costMu.RLock()
	state.clone.crossingCost = state.source.crossingCost
	state.clone.turnCost = state.source.turnCost
	state.clone.nonCenterPortCost = state.source.nonCenterPortCost
	state.source.costMu.RUnlock()
	return nil
}

func (state *graphCloneState) addNodeRecord(source *Node) (*Node, error) {
	if err := state.guard.Step(); err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("cannot clone a nil node")
	}
	if source.ID == 0 {
		return nil, fmt.Errorf("cannot clone reserved node ID 0")
	}
	if _, duplicate := state.nodesByID[source.ID]; duplicate {
		return nil, fmt.Errorf("cannot clone duplicate node ID %d", source.ID)
	}
	cloned := copyNodeRecord(source, state.clone)
	state.nodesByID[source.ID] = cloned
	state.nodeRecordsByID[source.ID] = source
	state.topLevelNodeIDs[source.ID] = struct{}{}
	state.nodesBySource[source] = cloned
	state.nodeRecords = append(state.nodeRecords, source)
	return cloned, nil
}

// copyAuxiliaryNodeRecord copies a node record embedded in another graph
// record, such as a grouping vessel or tree node. References elsewhere in the
// graph are resolved by ID, but two distinct full records with the same ID
// cannot be merged without silently discarding one record's state.
func (state *graphCloneState) copyAuxiliaryNodeRecord(source *Node, relation string) (*Node, bool, error) {
	if source == nil {
		return nil, false, fmt.Errorf("cannot clone %s: nil node", relation)
	}
	if source.ID == 0 {
		return nil, false, fmt.Errorf("cannot clone %s: reserved node ID 0", relation)
	}
	if declared, exists := state.nodeRecordsByID[source.ID]; exists {
		if declared != source {
			return nil, false, fmt.Errorf("cannot clone %s: distinct node record reuses ID %d", relation, source.ID)
		}
		return state.nodesByID[source.ID], false, nil
	}

	cloned := copyNodeRecord(source, state.clone)
	state.nodesByID[source.ID] = cloned
	state.nodeRecordsByID[source.ID] = source
	state.nodesBySource[source] = cloned
	return cloned, true, nil
}

func (state *graphCloneState) resolveNode(source *Node, relation string) (*Node, error) {
	if source == nil {
		return nil, nil
	}
	if err := state.guard.Step(); err != nil {
		return nil, err
	}
	cloned, ok := state.nodesByID[source.ID]
	if !ok {
		return nil, fmt.Errorf("cannot clone %s: node %d is not included in the graph", relation, source.ID)
	}
	state.nodesBySource[source] = cloned
	return cloned, nil
}

func (state *graphCloneState) copyEdge(source *Edge) (*Edge, error) {
	from, err := state.resolveNode(source.From, "edge source")
	if err != nil {
		return nil, err
	}
	to, err := state.resolveNode(source.To, "edge target")
	if err != nil {
		return nil, err
	}
	if from == nil || to == nil {
		return nil, fmt.Errorf("cannot clone edge %d with a nil endpoint", source.ID)
	}

	cloned := NewEdge(from, to)
	cloned.ID = source.ID
	cloned.D2ID = cloneValue(source.D2ID)
	cloned.SourceArrowhead = source.SourceArrowhead
	cloned.TargetArrowhead = source.TargetArrowhead
	cloned.SourceArrowheadLabel = copyLabelRecord(source.SourceArrowheadLabel)
	cloned.TargetArrowheadLabel = copyLabelRecord(source.TargetArrowheadLabel)
	cloned.MinWidth = source.MinWidth
	cloned.MinHeight = source.MinHeight
	cloned.Label = copyLabelRecord(source.Label)
	cloned.LabelPercentage = source.LabelPercentage
	cloned.FromTableColumnIndex = cloneValue(source.FromTableColumnIndex)
	cloned.ToTableColumnIndex = cloneValue(source.ToTableColumnIndex)
	cloned.IsInvisible = source.IsInvisible
	cloned.Style = copyEdgeStyle(source.Style)
	for _, point := range source.Points {
		if err := state.guard.Step(); err != nil {
			return nil, err
		}
		cloned.Points = append(cloned.Points, point.Copy())
	}
	return cloned, nil
}

func (state *graphCloneState) copyContainers() error {
	state.clone.Containers = make(map[*Node][]*Node, len(state.source.Containers))
	if len(state.source.Containers) == 0 {
		return nil
	}
	order, err := state.source.containerRDFSOrderContext(nil, state.guard)
	if err != nil {
		return err
	}
	if len(state.source.Containers) != len(order)+1 {
		return fmt.Errorf("cannot clone containers: map length %d does not match hierarchy length %d", len(state.source.Containers), len(order)+1)
	}
	order = append(order, nil)
	for _, sourceContainer := range order {
		container, err := state.resolveNode(sourceContainer, "container")
		if err != nil {
			return err
		}
		children := state.source.Containers[sourceContainer]
		clonedChildren := make([]*Node, 0, len(children))
		for _, sourceChild := range children {
			if sourceChild == nil || state.nodeRecordsByID[sourceChild.ID] != sourceChild {
				return fmt.Errorf("cannot clone container child: exact node record is not available")
			}
			child, err := state.resolveNode(sourceChild, "container child")
			if err != nil {
				return err
			}
			if child == nil {
				return fmt.Errorf("cannot clone a nil container child")
			}
			child.Container = container
			clonedChildren = append(clonedChildren, child)
		}
		state.clone.Containers[container] = clonedChildren
		if container != nil {
			container.isContainer = true
		}
	}
	return nil
}

func (state *graphCloneState) copyClusters() error {
	state.clone.Clusters = make(map[*Node]*Cluster, len(state.source.Clusters))
	for _, sourceVessel := range state.source.ClusterOrder() {
		if err := state.guard.Step(); err != nil {
			return err
		}
		sourceCluster := state.source.Clusters[sourceVessel]
		if sourceCluster == nil {
			return fmt.Errorf("cannot clone nil cluster for vessel %d", sourceVessel.ID)
		}
		if sourceCluster.Vessel != sourceVessel {
			return fmt.Errorf("cannot clone cluster under vessel %d because its record vessel differs", sourceVessel.ID)
		}
		if sourceCluster.Container != nil && sourceCluster.Container.ID == sourceVessel.ID {
			return fmt.Errorf("cannot clone cluster %d because it cannot contain itself", sourceVessel.ID)
		}
		vessel, copied, err := state.copyAuxiliaryNodeRecord(sourceVessel, "cluster vessel")
		if err != nil {
			return err
		}
		if copied {
			vessel.Graph = nil
		}
		vessel.isClusterVessel = true

		container, err := state.resolveNode(sourceCluster.Container, "cluster container")
		if err != nil {
			return err
		}
		members := make([]*Node, 0, len(sourceCluster.Nodes))
		for _, sourceMember := range sourceCluster.Nodes {
			member, err := state.resolveNode(sourceMember, "cluster member")
			if err != nil {
				return err
			}
			members = append(members, member)
		}
		cluster := &Cluster{
			Vessel:             vessel,
			Nodes:              members,
			Arrangement:        sourceCluster.Arrangement,
			DesiredArrangement: sourceCluster.DesiredArrangement,
			Graph:              state.clone,
			Padding:            sourceCluster.Padding,
			FixedSize:          sourceCluster.FixedSize,
			Container:          container,
		}
		state.clone.Clusters[vessel] = cluster
		state.clustersBySource[sourceCluster] = cluster
		for _, member := range members {
			if err := state.guard.Step(); err != nil {
				return err
			}
			member.Cluster = cluster
		}
	}
	return nil
}

func (state *graphCloneState) copySequences() error {
	state.clone.Sequences = make(map[*Node]*Sequence, len(state.source.Sequences))
	for _, sourceVessel := range state.source.SequenceOrder() {
		if err := state.guard.Step(); err != nil {
			return err
		}
		sourceSequence := state.source.Sequences[sourceVessel]
		if sourceSequence == nil {
			return fmt.Errorf("cannot clone nil sequence for vessel %d", sourceVessel.ID)
		}
		if sourceSequence.Vessel != sourceVessel {
			return fmt.Errorf("cannot clone sequence under vessel %d because its record vessel differs", sourceVessel.ID)
		}
		if sourceSequence.Container != nil && sourceSequence.Container.ID == sourceVessel.ID {
			return fmt.Errorf("cannot clone sequence %d because it cannot contain itself", sourceVessel.ID)
		}
		if len(sourceSequence.Nodes) < 2 {
			return fmt.Errorf("cannot clone sequence %d with %d steps; want at least 2", sourceVessel.ID, len(sourceSequence.Nodes))
		}
		vessel, copied, err := state.copyAuxiliaryNodeRecord(sourceVessel, "sequence vessel")
		if err != nil {
			return err
		}
		if copied {
			vessel.Graph = nil
		}

		container, err := state.resolveNode(sourceSequence.Container, "sequence container")
		if err != nil {
			return err
		}
		members := make([]*Node, 0, len(sourceSequence.Nodes))
		for _, sourceMember := range sourceSequence.Nodes {
			member, err := state.resolveNode(sourceMember, "sequence member")
			if err != nil {
				return err
			}
			members = append(members, member)
		}
		sequence := &Sequence{
			Vessel:    vessel,
			Nodes:     members,
			Graph:     state.clone,
			Container: container,
		}
		state.clone.Sequences[vessel] = sequence
		state.sequencesBySource[sourceSequence] = sequence
		for _, member := range members {
			if err := state.guard.Step(); err != nil {
				return err
			}
			member.Sequence = sequence
		}
	}
	return nil
}

func (state *graphCloneState) copyHubs() error {
	state.clone.Hubs = make(map[*Node][]*Node, len(state.source.Hubs))
	for _, sourceHub := range state.source.HubOrder() {
		hub, err := state.resolveNode(sourceHub, "hub")
		if err != nil {
			return err
		}
		spokes := make([]*Node, 0, len(state.source.Hubs[sourceHub]))
		for _, sourceSpoke := range state.source.Hubs[sourceHub] {
			if sourceSpoke == nil || state.nodeRecordsByID[sourceSpoke.ID] != sourceSpoke {
				return fmt.Errorf("cannot clone hub spoke: exact node record is not included in the graph")
			}
			spoke, err := state.resolveNode(sourceSpoke, "hub spoke")
			if err != nil {
				return err
			}
			spokes = append(spokes, spoke)
		}
		state.clone.Hubs[hub] = spokes
	}
	return nil
}

func (state *graphCloneState) copyAbductions() error {
	copyFor := func(sourceAbductions []*EdgeAbduction) ([]*EdgeAbduction, error) {
		cloned := make([]*EdgeAbduction, 0, len(sourceAbductions))
		for _, sourceAbduction := range sourceAbductions {
			if err := state.guard.Step(); err != nil {
				return nil, err
			}
			edge := state.edgesBySource[sourceAbduction.Edge]
			if edge == nil && sourceAbduction.Edge.ID != 0 {
				edge = state.edgesByID[sourceAbduction.Edge.ID]
			}
			if edge == nil {
				return nil, fmt.Errorf("cannot clone edge abduction for edge %d before the edge is copied", sourceAbduction.Edge.ID)
			}
			originallyFrom, err := state.resolveNode(sourceAbduction.OriginallyFrom, "abduction original source")
			if err != nil {
				return nil, err
			}
			originallyTo, err := state.resolveNode(sourceAbduction.OriginallyTo, "abduction original target")
			if err != nil {
				return nil, err
			}
			currentFrom, err := state.resolveNode(sourceAbduction.CurrentFrom, "abduction current source")
			if err != nil {
				return nil, err
			}
			currentTo, err := state.resolveNode(sourceAbduction.CurrentTo, "abduction current target")
			if err != nil {
				return nil, err
			}
			cloned = append(cloned, &EdgeAbduction{
				Edge:           edge,
				OriginallyFrom: originallyFrom,
				OriginallyTo:   originallyTo,
				CurrentFrom:    currentFrom,
				CurrentTo:      currentTo,
			})
		}
		return cloned, nil
	}

	for _, sourceVessel := range state.source.ClusterOrder() {
		sourceCluster := state.source.Clusters[sourceVessel]
		cloned, err := copyFor(sourceCluster.EdgeAbductions)
		if err != nil {
			return err
		}
		state.clustersBySource[sourceCluster].EdgeAbductions = cloned
	}
	for _, sourceVessel := range state.source.SequenceOrder() {
		sourceSequence := state.source.Sequences[sourceVessel]
		cloned, err := copyFor(sourceSequence.EdgeAbductions)
		if err != nil {
			return err
		}
		state.sequencesBySource[sourceSequence].EdgeAbductions = cloned
	}
	return nil
}

func (state *graphCloneState) copyTrees() error {
	state.clone.Trees = make(map[*Node][]*Tree, len(state.source.Trees))
	state.clone.NodeToTree = make(map[*Node]*Tree)
	treeOwnersByNodeID := make(map[EntityID]struct{})

	var copyTree func(source *Tree, parent *Tree) (*Tree, error)
	copyTree = func(source *Tree, parent *Tree) (*Tree, error) {
		if err := state.guard.Step(); err != nil {
			return nil, err
		}
		if source.SentinelEdge == nil {
			return nil, fmt.Errorf("cannot clone a tree with a nil sentinel edge")
		}
		if source.Node == nil {
			return nil, fmt.Errorf("cannot clone a tree with a nil node")
		}
		if _, exists := treeOwnersByNodeID[source.Node.ID]; exists {
			return nil, fmt.Errorf("cannot clone tree node %d with more than one tree owner", source.Node.ID)
		}
		treeOwnersByNodeID[source.Node.ID] = struct{}{}
		if declared := state.nodeRecordsByID[source.Node.ID]; declared == source.Node {
			if _, topLevel := state.topLevelNodeIDs[source.Node.ID]; !topLevel {
				return nil, fmt.Errorf("cannot clone tree node %d because it repeats a non-top-level node record", source.Node.ID)
			}
		}

		node, copied, err := state.copyAuxiliaryNodeRecord(source.Node, "tree node")
		if err != nil {
			return nil, err
		}
		if copied {
			state.treeNodeRecords = append(state.treeNodeRecords, source.Node)
		}

		sentinel := state.edgesBySource[source.SentinelEdge]
		if sentinel == nil && source.SentinelEdge.ID != 0 && state.edgesByID[source.SentinelEdge.ID] != nil {
			return nil, fmt.Errorf("cannot clone tree sentinel edge: distinct edge record reuses ID %d", source.SentinelEdge.ID)
		}
		if sentinel == nil {
			sentinel, err = state.copyEdge(source.SentinelEdge)
			if err != nil {
				return nil, err
			}
			state.edgesBySource[source.SentinelEdge] = sentinel
			if source.SentinelEdge.ID != 0 {
				state.edgesByID[source.SentinelEdge.ID] = sentinel
			}
		}
		cloned := &Tree{
			Node:         node,
			Parent:       parent,
			SentinelEdge: sentinel,
			Orientation:  source.Orientation,
		}
		state.clone.NodeToTree[node] = cloned
		for _, sourceChild := range source.Children {
			child, err := copyTree(sourceChild, cloned)
			if err != nil {
				return nil, err
			}
			cloned.Children = append(cloned.Children, child)
		}
		return cloned, nil
	}

	for _, sourceSentinel := range state.source.TreeOrder() {
		sentinel, err := state.resolveNode(sourceSentinel, "tree root sentinel")
		if err != nil {
			return err
		}
		var roots []*Tree
		for _, sourceRoot := range state.source.Trees[sourceSentinel] {
			root, err := copyTree(sourceRoot, nil)
			if err != nil {
				return err
			}
			roots = append(roots, root)
		}
		if len(roots) > 0 {
			state.clone.Trees[sentinel] = roots
		}
	}
	return nil
}

func (state *graphCloneState) copyNears() error {
	for _, source := range append(state.nodeRecords, state.treeNodeRecords...) {
		node := state.nodesBySource[source]
		for _, sourceNear := range source.orderedNears() {
			if err := state.guard.Step(); err != nil {
				return err
			}
			near, err := state.resolveNode(sourceNear, "near relation")
			if err != nil {
				return err
			}
			node.AddNear(near)
		}
	}
	return nil
}

func (state *graphCloneState) copyHierarchies() error {
	seen := make(map[*Hierarchy]struct{})
	for _, sourceNode := range state.source.Nodes {
		if err := state.guard.Step(); err != nil {
			return err
		}
		sourceHierarchy := sourceNode.Hierarchy
		if sourceHierarchy == nil {
			continue
		}
		if _, ok := seen[sourceHierarchy]; ok {
			continue
		}
		seen[sourceHierarchy] = struct{}{}
		levels := make(map[*Node]int, len(sourceHierarchy.level))
		clonedHierarchy := &Hierarchy{level: levels}
		for sourceMember, level := range sourceHierarchy.level {
			member, err := state.resolveNode(sourceMember, "hierarchy member")
			if err != nil {
				return err
			}
			levels[member] = level
			member.Hierarchy = clonedHierarchy
		}
	}
	return nil
}

func (state *graphCloneState) copyDirections() error {
	state.clone.Directions = make(map[*Node]geo.Orientation, len(state.source.Directions))
	for sourceContainer, sourceDirection := range state.source.Directions {
		container, err := state.resolveNode(sourceContainer, "direction container")
		if err != nil {
			return err
		}
		state.clone.Directions[container] = canonicalDirection(sourceDirection)
	}
	return nil
}

func copyNodeRecord(source *Node, owner *Graph) *Node {
	cloned := NewNode(source.ID, source.Width, source.Height)
	cloned.D2ID = cloneValue(source.D2ID)
	cloned.TopLeft = cloneValue(source.TopLeft)
	cloned.FixedTopLeft = cloneValue(source.FixedTopLeft)
	cloned.Graph = owner
	cloned.FontSize = cloneValue(source.FontSize)
	cloned.DesiredWidth = cloneValue(source.DesiredWidth)
	cloned.DesiredHeight = cloneValue(source.DesiredHeight)
	cloned.SetShape(source.ShapeType())
	cloned.SetNumColumns(source.NumColumns())
	cloned.ForceHierarchy = source.ForceHierarchy
	cloned.Label = copyLabelRecord(source.Label)
	cloned.Icon = copyIconRecord(source.Icon)
	cloned.Is3D = source.Is3D
	cloned.IsMultiple = source.IsMultiple
	cloned.IsInvisible = source.IsInvisible
	return cloned
}

func copyLabelRecord(source *Label) *Label {
	if source == nil {
		return nil
	}
	return &Label{
		Text:     source.Text,
		Position: label.FromString(source.Position.String()),
		Width:    source.Width,
		Height:   source.Height,
	}
}

func copyIconRecord(source *Icon) *Icon {
	if source == nil {
		return nil
	}
	return &Icon{Position: label.FromString(source.Position.String())}
}

func copyEdgeStyle(source EdgeStyle) EdgeStyle {
	return EdgeStyle{
		Opacity:       cloneValue(source.Opacity),
		Stroke:        cloneValue(source.Stroke),
		Fill:          cloneValue(source.Fill),
		FillPattern:   cloneValue(source.FillPattern),
		StrokeWidth:   cloneValue(source.StrokeWidth),
		StrokeDash:    cloneValue(source.StrokeDash),
		BorderRadius:  cloneValue(source.BorderRadius),
		Shadow:        cloneValue(source.Shadow),
		ThreeDee:      cloneValue(source.ThreeDee),
		Multiple:      cloneValue(source.Multiple),
		Font:          cloneValue(source.Font),
		FontSize:      cloneValue(source.FontSize),
		FontColor:     cloneValue(source.FontColor),
		Animated:      cloneValue(source.Animated),
		Bold:          cloneValue(source.Bold),
		Italic:        cloneValue(source.Italic),
		Underline:     cloneValue(source.Underline),
		Filled:        cloneValue(source.Filled),
		DoubleBorder:  cloneValue(source.DoubleBorder),
		TextTransform: cloneValue(source.TextTransform),
	}
}

func cloneValue[T any](source *T) *T {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func canonicalDirection(direction geo.Orientation) geo.Orientation {
	switch direction {
	case geo.Top, geo.Bottom, geo.Left, geo.Right:
		return direction
	default:
		return geo.NONE
	}
}
