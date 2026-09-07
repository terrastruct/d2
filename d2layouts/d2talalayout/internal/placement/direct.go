package placement

import (
	"cmp"
	"context"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

type directionCounts struct {
	left, right, top, bottom int
}

func (counts *directionCounts) add(other directionCounts) {
	counts.left += other.left
	counts.right += other.right
	counts.top += other.top
	counts.bottom += other.bottom
}

func edgeDirectionCounts(edge *layoutgraph.Edge) directionCounts {
	var counts directionCounts
	from, to, directed := edge.DirectedEndpoints()
	if !directed {
		return counts
	}
	switch from.Orientation(to).GetOpposite() {
	case geo.Left:
		counts.left++
	case geo.TopLeft:
		counts.top++
		counts.left++
	case geo.BottomLeft:
		counts.bottom++
		counts.left++
	case geo.Right:
		counts.right++
	case geo.TopRight:
		counts.top++
		counts.right++
	case geo.BottomRight:
		counts.bottom++
		counts.right++
	case geo.Top:
		counts.top++
	case geo.Bottom:
		counts.bottom++
	}
	return counts
}

type directionTransforms struct{ mirrorX, mirrorY bool }

type directionCount struct {
	direction geo.Orientation
	count     int
}

func compareDirectionCounts(a, b directionCount, preferred geo.Orientation) int {
	if order := cmp.Compare(b.count, a.count); order != 0 {
		return order
	}
	aPreferred := a.direction == preferred
	bPreferred := b.direction == preferred
	switch {
	case aPreferred && !bPreferred:
		return -1
	case !aPreferred && bPreferred:
		return 1
	default:
		return 0
	}
}

func (counts directionCounts) transformsTo(direction geo.Orientation) directionTransforms {
	values := []directionCount{
		{geo.Right, counts.right},
		{geo.Bottom, counts.bottom},
		{geo.Left, counts.left},
		{geo.Top, counts.top},
	}
	slices.SortStableFunc(values, func(a, b directionCount) int {
		return compareDirectionCounts(a, b, direction)
	})
	primary, secondary := values[0], values[1]
	if secondary.direction == primary.direction.GetOpposite() {
		secondary = values[2]
	}
	if direction == geo.NONE {
		if primary.direction.IsHorizontal() {
			direction = geo.Right
		} else {
			direction = geo.Bottom
		}
	}
	xDirection, yDirection := geo.Right, geo.Bottom
	if direction.IsHorizontal() {
		xDirection = direction
	} else {
		yDirection = direction
	}
	var transforms directionTransforms
	if primary.direction.IsHorizontal() {
		transforms.mirrorX = primary.direction != xDirection
	} else {
		transforms.mirrorY = primary.direction != yDirection
	}
	if secondary.count > values[3].count {
		if secondary.direction.IsHorizontal() {
			transforms.mirrorX = secondary.direction != xDirection
		} else {
			transforms.mirrorY = secondary.direction != yDirection
		}
	}
	return transforms
}

func containerEdgeDirections(graph *layoutgraph.Graph, container *layoutgraph.Node) directionCounts {
	counted := make(map[*layoutgraph.Edge]struct{})
	var counts directionCounts
	for _, node := range graph.Containers[container] {
		if node.Graph != graph {
			continue
		}
		for _, edge := range node.Edges {
			if _, seen := counted[edge]; seen {
				continue
			}
			counted[edge] = struct{}{}
			counts.add(edgeDirectionCounts(edge))
		}
	}
	return counts
}

func hasFixedDescendant(node *layoutgraph.Node) bool {
	if node.FixedTopLeft != nil {
		return true
	}
	if node.IsContainer() {
		for _, child := range node.Graph.Containers[node] {
			if hasFixedDescendant(child) {
				return true
			}
		}
	}
	if node.IsClusterVessel() {
		for _, child := range node.Graph.Clusters[node].Nodes {
			if hasFixedDescendant(child) {
				return true
			}
		}
	} else if sequence := node.Graph.Sequences[node]; sequence != nil {
		for _, child := range sequence.Nodes {
			if hasFixedDescendant(child) {
				return true
			}
		}
	}
	return false
}

func mirrorAxes(ctx context.Context, graph *layoutgraph.Graph, mirrorX, mirrorY bool) error {
	guard, err := limits.NewWorkGuard(ctx, "MirrorAxesReachability", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	reachableContainers := make(map[*layoutgraph.Node]struct{})
	visited := make(map[*layoutgraph.Node]struct{})
	for _, node := range graph.Nodes {
		if _, seen := visited[node]; seen {
			continue
		}
		reachable, err := node.AllReachableNodesContext(true, true, false, nil, guard)
		if err != nil {
			return err
		}
		for _, current := range reachable {
			visited[current] = struct{}{}
			reachableContainers[current.Container] = struct{}{}
		}
	}
	if err := guard.Finish(); err != nil {
		return err
	}

	originalPositions, err := snapshotNodePositionsContext(ctx, "MirrorAxes", graph.Nodes)
	if err != nil {
		return err
	}
	originalTreeOrientations := make(map[*layoutgraph.Tree]geo.Orientation)
	for _, snapshot := range originalPositions {
		if tree := graph.NodeToTree[snapshot.node]; tree != nil {
			originalTreeOrientations[tree] = tree.Orientation
		}
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		restoreNodePositions(originalPositions)
		for tree, orientation := range originalTreeOrientations {
			tree.Orientation = orientation
		}
	}()

	mirrored := make(map[*layoutgraph.Node]struct{})
	var mutationErr error
	mirrorNode := func(node *layoutgraph.Node) {
		if mutationErr != nil {
			return
		}
		if err := guard.Step(); err != nil {
			mutationErr = err
			return
		}
		if _, seen := mirrored[node]; seen {
			return
		}
		if _, reachable := reachableContainers[node.EffectiveContainer()]; reachable {
			node.Mirror(mirrorX, mirrorY)
			mirrored[node] = struct{}{}
			if err := guard.Finish(); err != nil {
				mutationErr = err
				return
			}
			if tree := graph.NodeToTree[node]; tree != nil &&
				(mirrorX && tree.Orientation.IsHorizontal() || mirrorY && tree.Orientation.IsVertical()) {
				tree.Orientation = tree.Orientation.GetOpposite()
				if err := guard.Finish(); err != nil {
					mutationErr = err
					return
				}
			}
		}
		node.PositionContainerChildren(true)
		if err := guard.Finish(); err != nil {
			mutationErr = err
		}
	}
	for _, node := range graph.Nodes {
		if mutationErr != nil {
			break
		}
		if _, seen := mirrored[node]; !seen {
			node.WalkRDFS(mirrorNode)
		}
	}
	if mutationErr != nil {
		return mutationErr
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}

// direct mirrors a placed subgraph toward its dominant edge direction when
// doing so preserves or improves the placement cost.
type directOptions struct {
	checkEdgeLength bool
}

func direct(ctx context.Context, graph *layoutgraph.Graph, nodes layoutgraph.Nodes, container *layoutgraph.Node, options directOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(nodes) == 0 || nodes[0].Hierarchy != nil {
		return nil
	}
	for _, node := range nodes {
		if hasFixedDescendant(node) {
			return nil
		}
		if sequence := graph.Sequences[node]; sequence != nil && len(sequence.EdgeAbductions) > 0 {
			return nil
		}
		if node.Sequence != nil && len(node.Sequence.EdgeAbductions) > 0 {
			return nil
		}
	}
	transforms := containerEdgeDirections(graph, container).transformsTo(graph.Direction(container))
	if !transforms.mirrorX && !transforms.mirrorY {
		return nil
	}
	if !options.checkEdgeLength {
		return mirrorAxes(ctx, graph, transforms.mirrorX, transforms.mirrorY)
	}
	currentLength, err := placementcost.EdgeLength(ctx, graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		return err
	}
	transaction, err := graph.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
	if err != nil {
		return err
	}
	transaction.AddOp(func() error { return mirrorAxes(ctx, graph, transforms.mirrorX, transforms.mirrorY) })
	if err := transaction.Commit(ctx); err != nil {
		if layoutgraph.IsCandidateRejection(err) {
			return nil
		}
		return err
	}
	newLength, err := placementcost.EdgeLength(ctx, graph, placementcost.EdgeLengthOptions{EdgeAbductions: nil, IncludeNodeSizes: true, EnforceMinimumGap: false, PenalizeDirection: true})
	if err != nil {
		transaction.Rollback()
		return err
	}
	if geo.PrecisionCompare(newLength, currentLength, geo.PRECISION) > 0 {
		transaction.Rollback()
	}
	return nil
}
