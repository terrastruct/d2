// Package placement positions nodes and refines their geometry before routing.
package placement

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

const maxCompactionCandidateCount = limits.MaxGraphSize + 3

type pointerSnapshot[T any] struct {
	pointer *T
	value   T
}

func snapshotPointer[T any](pointer *T) pointerSnapshot[T] {
	if pointer == nil {
		return pointerSnapshot[T]{}
	}
	return pointerSnapshot[T]{pointer: pointer, value: *pointer}
}

func (snapshot pointerSnapshot[T]) restore() *T {
	if snapshot.pointer == nil {
		return nil
	}
	*snapshot.pointer = snapshot.value
	return snapshot.pointer
}

type nodePositionSnapshot struct {
	node    *layoutgraph.Node
	topLeft pointerSnapshot[geo.Point]
}

func restoreNodePositions(snapshots []nodePositionSnapshot) {
	for _, snapshot := range snapshots {
		snapshot.node.TopLeft = snapshot.topLeft.restore()
	}
}

func snapshotNodePositionsContext(ctx context.Context, location string, nodes []*layoutgraph.Node) ([]nodePositionSnapshot, error) {
	guard, err := limits.NewWorkGuard(ctx, location, limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	seen := make(map[*layoutgraph.Node]struct{})
	snapshots := make([]nodePositionSnapshot, 0, len(nodes))
	type pendingNode struct {
		node  *layoutgraph.Node
		graph *layoutgraph.Graph
	}
	queue := make([]pendingNode, 0, len(nodes))
	enqueue := func(node *layoutgraph.Node, graph *layoutgraph.Graph) error {
		if node == nil {
			return nil
		}
		if _, ok := seen[node]; ok {
			return nil
		}
		if len(seen) >= limits.MaxEngineNodes {
			return fmt.Errorf("TALA %s unique node count exceeds limit %d", location, limits.MaxEngineNodes)
		}
		if err := guard.Step(); err != nil {
			return err
		}
		seen[node] = struct{}{}
		snapshots = append(snapshots, nodePositionSnapshot{node: node, topLeft: snapshotPointer(node.TopLeft)})
		queue = append(queue, pendingNode{node: node, graph: graph})
		return nil
	}
	for _, node := range nodes {
		if node != nil {
			if err := enqueue(node, node.Graph); err != nil {
				return nil, err
			}
		}
	}
	for index := 0; index < len(queue); index++ {
		current := queue[index]
		if current.graph == nil {
			continue
		}
		node := current.node
		if node.IsContainer() {
			for _, child := range current.graph.Containers[node] {
				if err := enqueue(child, current.graph); err != nil {
					return nil, err
				}
			}
		}
		if node.IsClusterVessel() {
			if cluster := current.graph.Clusters[node]; cluster != nil {
				for _, child := range cluster.Nodes {
					if err := enqueue(child, current.graph); err != nil {
						return nil, err
					}
				}
			}
		}
		if sequence := current.graph.Sequences[node]; sequence != nil {
			for _, child := range sequence.Nodes {
				if err := enqueue(child, current.graph); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := guard.Finish(); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func nonNilEquals(first, second *geo.Point) bool {
	return first.X == second.X && first.Y == second.Y
}

func roundToPreviousCellSize(value, cellSize float64) float64 {
	return math.Floor(value/cellSize) * cellSize
}

func roundToNearestCellSize(value, cellSize float64) float64 {
	return math.Round(value/cellSize) * cellSize
}

func numPointsWithinManhattanDistance(distance float64) int {
	return int(math.Ceil(math.Pow(distance, 2) + math.Pow(distance+1, 2)))
}
