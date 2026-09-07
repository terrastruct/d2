package placement

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func candidateMovementGraph(count int) (*layoutgraph.Graph, *layoutgraph.Node) {
	graph := layoutgraph.NewGraph()
	root := layoutgraph.NewNode(1, 1000, 1000)
	root.TopLeft = geo.NewPoint(0, 0)
	graph.AddNewNodeToContainer(nil, root)
	for i := 0; i < count; i++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(i+2), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i), float64(i))
		graph.AddNewNodeToContainer(root, node)
	}
	if count > 2 {
		// Repeated membership across all three ownership views must only move each
		// node once, preserving optimizerDescendants' stable traversal order.
		root.SetClusterVessel(true)
		graph.Clusters[root] = &layoutgraph.Cluster{Vessel: root, Graph: graph, Nodes: graph.Nodes[1:3]}
		graph.Sequences[root] = &layoutgraph.Sequence{Vessel: root, Graph: graph, Nodes: graph.Nodes[1:3]}
		graph.Containers[root] = append(graph.Containers[root], graph.Nodes[1])
		graph.AddNodeToContainer(graph.Nodes[1], graph.Nodes[2])
	}
	return graph, root
}

func TestOptimizerCandidateMovementMatchesLegacy(t *testing.T) {
	for _, count := range []int{0, 1, 128} {
		legacyGraph, legacyRoot := candidateMovementGraph(count)
		candidateGraph, candidateRoot := candidateMovementGraph(count)
		legacyGuard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
		candidateGuard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
		_, err := captureOptimizerNodePositions([]*layoutgraph.Node{legacyRoot}, legacyGuard)
		if err != nil {
			t.Fatal(err)
		}
		movement, err := captureOptimizerCandidateMovement(candidateRoot, candidateGuard)
		if err != nil {
			t.Fatal(err)
		}
		for _, point := range []geo.Point{{X: 100, Y: 100}, {X: 100, Y: 100}, {X: -35.5, Y: 9.25}, {X: 0, Y: 0}} {
			if err := optimizerMoveNodeAbs(legacyRoot, point.X, point.Y, legacyGuard); err != nil {
				t.Fatal(err)
			}
			if err := movement.moveAbs(point.X, point.Y, candidateGuard); err != nil {
				t.Fatal(err)
			}
			requireSameGraphNodeGeometry(t, candidateGraph, legacyGraph)
			if got, want := candidateGuard.Used(), legacyGuard.Used(); got != want {
				t.Fatalf("%d descendants: charged %d work units, want %d", count, got, want)
			}
		}
	}
}

func TestOptimizerCandidateMovementPreservesEveryResourceBoundary(t *testing.T) {
	const count = 12
	_, root := candidateMovementGraph(count)
	setupGuard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
	movement, err := captureOptimizerCandidateMovement(root, setupGuard)
	if err != nil {
		t.Fatal(err)
	}
	fullWork := movement.descendantWork + 2*uint64(len(movement.positions)-1)
	for limit := uint64(0); limit <= fullWork; limit++ {
		legacyGraph, legacyRoot := candidateMovementGraph(count)
		candidateGraph, candidateRoot := candidateMovementGraph(count)
		original := captureExactOptimizerPositions(candidateGraph)
		setupGuard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
		movement, err := captureOptimizerCandidateMovement(candidateRoot, setupGuard)
		if err != nil {
			t.Fatal(err)
		}
		legacyGuard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", limit)
		candidateGuard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", limit)
		legacyErr := optimizerMoveNodeAbs(legacyRoot, 100, 100, legacyGuard)
		candidateErr := movement.moveAbs(100, 100, candidateGuard)
		if candidateErr != nil {
			restoreNodePositions(movement.positions)
		}
		if errors.Is(candidateErr, limits.ErrOptimizationResourceLimit) != errors.Is(legacyErr, limits.ErrOptimizationResourceLimit) {
			t.Fatalf("budget %d: candidate error %v, legacy error %v", limit, candidateErr, legacyErr)
		}
		if got, want := candidateGuard.Used(), legacyGuard.Used(); got != want {
			t.Fatalf("budget %d: charged %d work units, want %d", limit, got, want)
		}
		requireSameGraphNodeGeometry(t, candidateGraph, legacyGraph)
		if candidateErr != nil {
			requireExactOptimizerPositions(t, original)
		}
	}
}

type candidateMoveCancellation struct {
	context.Context
	remaining int
}

func (ctx *candidateMoveCancellation) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return nil
}

func TestOptimizerCandidateMovementPreservesCancellationPolling(t *testing.T) {
	for cancelAt := 1; cancelAt <= 12; cancelAt++ {
		legacyGraph, legacyRoot := candidateMovementGraph(128)
		candidateGraph, candidateRoot := candidateMovementGraph(128)
		original := captureExactOptimizerPositions(candidateGraph)
		setupGuard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
		movement, err := captureOptimizerCandidateMovement(candidateRoot, setupGuard)
		if err != nil {
			t.Fatal(err)
		}
		legacyCtx := &candidateMoveCancellation{Context: context.Background(), remaining: cancelAt}
		candidateCtx := &candidateMoveCancellation{Context: context.Background(), remaining: cancelAt}
		legacyGuard, _ := limits.NewOptimizationWorkGuard(legacyCtx, "test", limits.MaxOptimizationWorkUnits)
		candidateGuard, _ := limits.NewOptimizationWorkGuard(candidateCtx, "test", limits.MaxOptimizationWorkUnits)
		legacyErr := optimizerMoveNodeAbs(legacyRoot, 100, 100, legacyGuard)
		candidateErr := movement.moveAbs(100, 100, candidateGuard)
		if candidateErr != nil {
			restoreNodePositions(movement.positions)
		}
		if errors.Is(candidateErr, context.Canceled) != errors.Is(legacyErr, context.Canceled) {
			t.Fatalf("poll %d: candidate error %v, legacy error %v", cancelAt, candidateErr, legacyErr)
		}
		if candidateGuard.Used() != legacyGuard.Used() || candidateCtx.remaining != legacyCtx.remaining {
			t.Fatalf("poll %d: work/cancellation accounting differs", cancelAt)
		}
		requireSameGraphNodeGeometry(t, candidateGraph, legacyGraph)
		if candidateErr != nil {
			requireExactOptimizerPositions(t, original)
		}
	}
}

func BenchmarkOptimizerCandidateMovement(b *testing.B) {
	for _, cached := range []bool{false, true} {
		name := "legacy"
		if cached {
			name = "cached"
		}
		b.Run(name, func(b *testing.B) {
			_, root := candidateMovementGraph(128)
			guard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", ^uint64(0))
			movement, err := captureOptimizerCandidateMovement(root, guard)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if cached {
					err = movement.moveAbs(root.TopLeft.X+1, root.TopLeft.Y+1, guard)
				} else {
					err = optimizerMoveNodeAbs(root, root.TopLeft.X+1, root.TopLeft.Y+1, guard)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
