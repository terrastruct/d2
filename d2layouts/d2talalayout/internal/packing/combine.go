package packing

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type exactSliceSnapshot[S ~[]V, V any] struct {
	original S
	backing  []V
}

func captureExactSlice[S ~[]V, V any](values S) exactSliceSnapshot[S, V] {
	if values == nil {
		return exactSliceSnapshot[S, V]{}
	}
	full := values[:cap(values)]
	backing := make([]V, len(full))
	copy(backing, full)
	return exactSliceSnapshot[S, V]{original: values, backing: backing}
}

func (snapshot exactSliceSnapshot[S, V]) restore() S {
	if snapshot.original == nil {
		return nil
	}
	copy(snapshot.original[:cap(snapshot.original)], snapshot.backing)
	return snapshot.original
}

type combineEdgeSnapshot struct {
	value                layoutgraph.Edge
	d2ID                 pointerSnapshot[string]
	points               exactSliceSnapshot[[]*geo.Point, *geo.Point]
	pointValues          []pointerSnapshot[geo.Point]
	label                pointerSnapshot[layoutgraph.Label]
	sourceArrowheadLabel pointerSnapshot[layoutgraph.Label]
	targetArrowheadLabel pointerSnapshot[layoutgraph.Label]
	fromTableColumnIndex pointerSnapshot[int]
	toTableColumnIndex   pointerSnapshot[int]
}

func captureCombineEdge(edge *layoutgraph.Edge) combineEdgeSnapshot {
	fullPoints := edge.Points[:cap(edge.Points)]
	pointValues := make([]pointerSnapshot[geo.Point], len(fullPoints))
	for i, point := range fullPoints {
		pointValues[i] = snapshotPointer(point)
	}
	return combineEdgeSnapshot{
		value:                *edge,
		d2ID:                 snapshotPointer(edge.D2ID),
		points:               captureExactSlice(edge.Points),
		pointValues:          pointValues,
		label:                snapshotPointer(edge.Label),
		sourceArrowheadLabel: snapshotPointer(edge.SourceArrowheadLabel),
		targetArrowheadLabel: snapshotPointer(edge.TargetArrowheadLabel),
		fromTableColumnIndex: snapshotPointer(edge.FromTableColumnIndex),
		toTableColumnIndex:   snapshotPointer(edge.ToTableColumnIndex),
	}
}

func (snapshot combineEdgeSnapshot) restore(edge *layoutgraph.Edge) {
	snapshot.d2ID.restore()
	snapshot.label.restore()
	snapshot.sourceArrowheadLabel.restore()
	snapshot.targetArrowheadLabel.restore()
	snapshot.fromTableColumnIndex.restore()
	snapshot.toTableColumnIndex.restore()
	for _, point := range snapshot.pointValues {
		point.restore()
	}
	originalPoints := snapshot.points.restore()
	*edge = snapshot.value
	edge.D2ID = snapshot.d2ID.pointer
	edge.Points = originalPoints
	edge.Label = snapshot.label.pointer
	edge.SourceArrowheadLabel = snapshot.sourceArrowheadLabel.pointer
	edge.TargetArrowheadLabel = snapshot.targetArrowheadLabel.pointer
	edge.FromTableColumnIndex = snapshot.fromTableColumnIndex.pointer
	edge.ToTableColumnIndex = snapshot.toTableColumnIndex.pointer
}

// CombineSubgraphs arranges laid-out subgraphs and returns a combined graph
// view that shares the master graph's topology.
func CombineSubgraphs(ctx context.Context, masterGraph *layoutgraph.Graph, graphs []*layoutgraph.Graph, ancestorObstacles []geo.Box) (_ *layoutgraph.Graph, err error) {
	if masterGraph == nil {
		return nil, fmt.Errorf("TALA CombineSubgraphs requires a master graph")
	}
	if err := layoutgraph.Validate(ctx, "CombineSubgraphs", masterGraph); err != nil {
		return nil, err
	}
	if len(graphs) > limits.MaxEngineNodes {
		return nil, fmt.Errorf("TALA CombineSubgraphs subgraph count %d exceeds limit %d", len(graphs), limits.MaxEngineNodes)
	}
	type topologyIdentity struct {
		containers, clusters, trees, nodeToTree uintptr
		hubs, sequences, directions, siblings   uintptr
	}
	identityOf := func(graph *layoutgraph.Graph) topologyIdentity {
		return topologyIdentity{
			containers: reflect.ValueOf(graph.Containers).Pointer(),
			clusters:   reflect.ValueOf(graph.Clusters).Pointer(),
			trees:      reflect.ValueOf(graph.Trees).Pointer(),
			nodeToTree: reflect.ValueOf(graph.NodeToTree).Pointer(),
			hubs:       reflect.ValueOf(graph.Hubs).Pointer(),
			sequences:  reflect.ValueOf(graph.Sequences).Pointer(),
			directions: reflect.ValueOf(graph.Directions).Pointer(),
			siblings:   reflect.ValueOf(graph.CommonUncleSiblings).Pointer(),
		}
	}
	// Identical map identities mean identical graph-owned topology, so that
	// expensive portion only needs one full preflight. Node-owned topology is
	// deliberately validated for every graph below as one distinct-record union.
	validatedGraphOwnedTopologies := map[topologyIdentity]struct{}{
		identityOf(masterGraph): {},
	}
	allGraphs := make([]*layoutgraph.Graph, 1, len(graphs)+1)
	allGraphs[0] = masterGraph
	for _, graph := range graphs {
		if graph == nil {
			return nil, fmt.Errorf("TALA CombineSubgraphs received a nil subgraph")
		}
		identity := identityOf(graph)
		if _, validated := validatedGraphOwnedTopologies[identity]; !validated {
			if err := layoutgraph.Validate(ctx, "CombineSubgraphs", graph); err != nil {
				return nil, err
			}
			validatedGraphOwnedTopologies[identity] = struct{}{}
		}
		allGraphs = append(allGraphs, graph)
	}
	if err := layoutgraph.ValidateSubgraphCombination(ctx, allGraphs); err != nil {
		return nil, err
	}

	guard, err := limits.NewWorkGuard(ctx, "CombineSubgraphs", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	allNodes := make([]*layoutgraph.Node, 0, len(masterGraph.Nodes))
	uniqueNodes := make(map[*layoutgraph.Node]struct{}, len(masterGraph.Nodes))
	edgeSnapshots := make(map[*layoutgraph.Edge]combineEdgeSnapshot, len(masterGraph.Edges))
	var routePointCapacity int64
	collectGraph := func(graph *layoutgraph.Graph) error {
		for _, node := range graph.Nodes {
			if err := guard.Step(); err != nil {
				return err
			}
			if node == nil {
				return fmt.Errorf("TALA CombineSubgraphs contains a nil node")
			}
			if _, captured := uniqueNodes[node]; !captured {
				uniqueNodes[node] = struct{}{}
				if len(uniqueNodes) > limits.MaxEngineNodes {
					return fmt.Errorf("TALA CombineSubgraphs unique node count exceeds limit %d", limits.MaxEngineNodes)
				}
				allNodes = append(allNodes, node)
			}
		}
		for _, edge := range graph.Edges {
			if err := guard.Step(); err != nil {
				return err
			}
			if edge == nil {
				return fmt.Errorf("TALA CombineSubgraphs contains a nil edge")
			}
			if edge.From == nil || edge.To == nil {
				return fmt.Errorf("TALA CombineSubgraphs edge %d has missing endpoints", edge.ID)
			}
			if _, captured := edgeSnapshots[edge]; !captured {
				if len(edgeSnapshots) >= limits.MaxEngineEdges {
					return fmt.Errorf("TALA CombineSubgraphs unique edge count exceeds limit %d", limits.MaxEngineEdges)
				}
				capacity := int64(cap(edge.Points))
				if capacity > layoutgraph.MaxRoutePoints-routePointCapacity {
					return fmt.Errorf("TALA CombineSubgraphs route point count exceeds limit %d", layoutgraph.MaxRoutePoints)
				}
				routePointCapacity += capacity
				for _, point := range edge.Points {
					if err := guard.Step(); err != nil {
						return err
					}
					if point == nil {
						return fmt.Errorf("TALA CombineSubgraphs edge %d contains a nil route point", edge.ID)
					}
				}
				edgeSnapshots[edge] = captureCombineEdge(edge)
			}
		}
		return nil
	}
	if err := collectGraph(masterGraph); err != nil {
		return nil, err
	}
	for _, graph := range graphs {
		if err := collectGraph(graph); err != nil {
			return nil, err
		}
	}
	nodeSnapshots, err := snapshotCombineNodePositions(ctx, allNodes)
	if err != nil {
		return nil, err
	}
	nodeGraphReferences := make(map[*layoutgraph.Node]*layoutgraph.Graph, len(nodeSnapshots))
	for _, snapshot := range nodeSnapshots {
		nodeGraphReferences[snapshot.node] = snapshot.node.Graph
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		restoreCombineNodePositions(nodeSnapshots)
		for node, graph := range nodeGraphReferences {
			node.Graph = graph
		}
		for edge, snapshot := range edgeSnapshots {
			snapshot.restore(edge)
		}
	}()

	combined := layoutgraph.NewGraph()
	combined.CopyEntitiesFrom(masterGraph)
	if len(graphs) == 0 {
		complete = true
		return combined, nil
	}

	combinedBR := geo.NewPoint(math.Inf(-1), math.Inf(-1))
	candidatePoints := []*geo.Point{}

	// first subgraph (contains all fixed nodes) is always placed at 0,0
	if firstGraph := graphs[0]; firstGraph.HasFixedNode() {
		graphs = graphs[1:]
		_, graphBR := firstGraph.BoundingBox()

		for _, n := range firstGraph.Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			combined.AddNodeUnchecked(n)
			for _, child := range masterGraph.AllDescendantNodes(n, true) {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				combined.AddNodeUnchecked(child)
			}
		}
		for _, edge := range firstGraph.Edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			combined.AddEdge(edge)
		}

		combinedBR.X = math.Max(combinedBR.X, graphBR.X)
		combinedBR.Y = math.Max(combinedBR.Y, graphBR.Y)

		// Add this graph's new candidate points (at 1,2,3 with and without subgraphPadding).
		//  ┌───┐ 1
		//  │ g │
		//  └───┘ 2
		//  3
		candidatePoints = append(candidatePoints,
			geo.NewPoint(graphBR.X, 0),
			geo.NewPoint(graphBR.X, graphBR.Y),
			geo.NewPoint(0, graphBR.Y),
			geo.NewPoint(graphBR.X+subgraphPadding, 0),
			geo.NewPoint(graphBR.X+subgraphPadding, graphBR.Y),
			geo.NewPoint(0, graphBR.Y+subgraphPadding),
		)
	}

	sortedByArea := make([]*layoutgraph.Graph, len(graphs))
	copy(sortedByArea, graphs)
	sort.Slice(sortedByArea, func(i, j int) bool {
		return sortedByArea[i].Area() > sortedByArea[j].Area()
	})
	if err := guard.Finish(); err != nil {
		return nil, err
	}

	for _, graph := range sortedByArea {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		graphTL, graphBR := graph.BoundingBox()

		minCost := math.Inf(1)
		minOverlaps := math.MaxInt
		// place the first graph at (0,0) so that combined's top left is always there
		minCandidatePoint := geo.NewPoint(0, 0)
		var minCandidatePointIndex int

		// For each candidate point, see if it fits at that point
		// If it does, calculate the area and compare to see if it's the best point to place it
		for i, candidatePoint := range candidatePoints {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			// points relative to graph top left would now be relative to the candidate point
			offsetX := candidatePoint.X - graphTL.X
			offsetY := candidatePoint.Y - graphTL.Y

			collides := false
			ancestorOverlaps := 0
			for _, node := range graph.Nodes {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				node.MoveWithChildren(offsetX, offsetY)
				for _, otherNode := range combined.Nodes {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					if node.DoesOverlap(otherNode) {
						collides = true
						break
					}
				}
				if !collides {
					for _, o := range ancestorObstacles {
						if err := guard.Step(); err != nil {
							return nil, err
						}
						if o.TopLeft.X == -layoutgraph.ContainerPadding && o.TopLeft.Y == -layoutgraph.ContainerPadding {
							continue
						}
						if node.Box.Overlaps(o) {
							ancestorOverlaps++
						}
					}
				}
				node.MoveWithChildren(-offsetX, -offsetY)
				if collides {
					break
				}
			}
			if collides {
				continue
			}
			if ancestorOverlaps > minOverlaps {
				continue
			}

			// The new combined width is only (candidate point + graph width) if that is wider than the existing width (same for height)
			newWidth := math.Max(combinedBR.X, candidatePoint.X+(graphBR.X-graphTL.X))
			newHeight := math.Max(combinedBR.Y, candidatePoint.Y+(graphBR.Y-graphTL.Y))
			// A: Area
			// D: Deviation from square (squared)
			// Heuristic: A + cD
			// Note: the base cost is area, but we add an additional cost based on how far it is from being a square.
			// we square the difference between width and height to match area's unit (length^2)
			cost := newWidth*newHeight + math.Pow(newWidth-newHeight, 2.0)*subgraphSquareDampener

			if ancestorOverlaps < minOverlaps || (ancestorOverlaps == minOverlaps && cost < minCost) {
				minOverlaps = ancestorOverlaps
				minCost = cost
				minCandidatePoint = candidatePoint
				minCandidatePointIndex = i
			}
		}

		if len(candidatePoints) > 0 {
			// Remove the candidate point used
			candidatePoints[minCandidatePointIndex] = candidatePoints[len(candidatePoints)-1]
			candidatePoints[len(candidatePoints)-1] = nil
			candidatePoints = candidatePoints[:len(candidatePoints)-1]
		}

		// Since we are placing the graph at minCandidatePoint,
		// each point relative to the graph top left now needs to be relative to minCandidatePoint
		offsetX := minCandidatePoint.X - graphTL.X
		offsetY := minCandidatePoint.Y - graphTL.Y

		for _, n := range graph.Nodes {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			n.MoveWithChildren(offsetX, offsetY)
			combined.AddNodeUnchecked(n)
			for _, child := range masterGraph.AllDescendantNodes(n, true) {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				combined.AddNodeUnchecked(child)
			}
		}
		for _, edge := range graph.Edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			for _, point := range edge.Points {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				point.X += offsetX
				point.Y += offsetY
			}
			combined.AddEdge(edge)
		}

		// update graph tl and br to be relative to combined
		graphTL.X += offsetX
		graphTL.Y += offsetY
		graphBR.X += offsetX
		graphBR.Y += offsetY

		combinedBR.X = math.Max(combinedBR.X, graphBR.X)
		combinedBR.Y = math.Max(combinedBR.Y, graphBR.Y)

		// Add this graph's new candidate points (at 1,2,3 with and without subgraphPadding).
		//  ┌───┐ 1
		//  │ g │
		//  └───┘ 2
		//  3
		candidatePoints = append(candidatePoints,
			geo.NewPoint(graphBR.X, graphTL.Y),
			geo.NewPoint(graphBR.X, graphBR.Y),
			geo.NewPoint(graphTL.X, graphBR.Y),
			geo.NewPoint(graphBR.X+subgraphPadding, graphTL.Y),
			geo.NewPoint(graphBR.X+subgraphPadding, graphBR.Y),
			geo.NewPoint(graphTL.X, graphBR.Y+subgraphPadding),
		)
	}

	if err := guard.Finish(); err != nil {
		return nil, err
	}
	complete = true
	return combined, nil
}
