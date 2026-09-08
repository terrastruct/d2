package packing

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type combineNodePositionSnapshot struct {
	node    *layoutgraph.Node
	topLeft pointerSnapshot[geo.Point]
}

func restoreCombineNodePositions(snapshots []combineNodePositionSnapshot) {
	for _, snapshot := range snapshots {
		snapshot.node.TopLeft = snapshot.topLeft.restore()
	}
}

// snapshotCombineNodePositions captures every position that CombineSubgraphs
// can translate while preserving each point's exact identity.
func snapshotCombineNodePositions(ctx context.Context, nodes []*layoutgraph.Node) ([]combineNodePositionSnapshot, error) {
	guard, err := limits.NewWorkGuard(ctx, "CombineSubgraphs", limits.MaxEngineWorkUnits)
	if err != nil {
		return nil, err
	}
	seen := make(map[*layoutgraph.Node]struct{})
	snapshots := make([]combineNodePositionSnapshot, 0, len(nodes))
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
			return fmt.Errorf("TALA CombineSubgraphs unique node count exceeds limit %d", limits.MaxEngineNodes)
		}
		if err := guard.Step(); err != nil {
			return err
		}
		seen[node] = struct{}{}
		snapshots = append(snapshots, combineNodePositionSnapshot{node: node, topLeft: snapshotPointer(node.TopLeft)})
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
