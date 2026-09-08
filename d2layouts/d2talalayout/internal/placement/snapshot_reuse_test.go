package placement

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

// Keep the pre-optimization traversal as an oracle for exact accounting and
// alias preservation, including repeated ownership membership and cancellation.
func referenceOptimizerNodePositions(nodes []*layoutgraph.Node, guard *limits.OptimizationWorkGuard) ([]nodePositionSnapshot, error) {
	if len(nodes) <= 2 {
		leaves := true
		for _, node := range nodes {
			if node == nil {
				return nil, fmt.Errorf("TALA %s found a nil node", guard.Location())
			}
			if node.Graph == nil || node.IsContainer() || node.IsClusterVessel() || node.Graph.Sequences[node] != nil {
				leaves = false
				break
			}
		}
		if leaves {
			snapshots := make([]nodePositionSnapshot, 0, len(nodes))
			for _, node := range nodes {
				duplicate := false
				for _, snapshot := range snapshots {
					if snapshot.node == node {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
				if err := guard.Step(); err != nil {
					return nil, err
				}
				snapshots = append(snapshots, nodePositionSnapshot{node: node, topLeft: snapshotPointer(node.TopLeft)})
			}
			return snapshots, nil
		}
	}
	seen := make(map[*layoutgraph.Node]struct{})
	snapshots := make([]nodePositionSnapshot, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			return nil, fmt.Errorf("TALA %s found a nil node", guard.Location())
		}
		all := []*layoutgraph.Node{node}
		descendants, err := optimizerDescendants(node, guard)
		if err != nil {
			return nil, err
		}
		all = append(all, descendants...)
		for _, current := range all {
			if _, exists := seen[current]; exists {
				continue
			}
			if len(seen) >= limits.MaxEngineNodes {
				return nil, fmt.Errorf("TALA %s position snapshot exceeds node limit %d", guard.Location(), limits.MaxEngineNodes)
			}
			if err := guard.Step(); err != nil {
				return nil, err
			}
			seen[current] = struct{}{}
			snapshots = append(snapshots, nodePositionSnapshot{node: current, topLeft: snapshotPointer(current.TopLeft)})
		}
	}
	return snapshots, nil
}

func TestOptimizerPositionSnapshotSingleRootParity(t *testing.T) {
	for _, count := range []int{0, 1, 12, 128} {
		_, root := candidateMovementGraph(count)
		for cancelAt := 1; cancelAt <= 15; cancelAt++ {
			for _, limit := range []uint64{0, 1, 2, 4, 12, 40, 128, limits.MaxOptimizationWorkUnits} {
				ac := &candidateMoveCancellation{Context: context.Background(), remaining: cancelAt}
				bc := &candidateMoveCancellation{Context: context.Background(), remaining: cancelAt}
				ag, _ := limits.NewOptimizationWorkGuard(ac, "test", limit)
				bg, _ := limits.NewOptimizationWorkGuard(bc, "test", limit)
				got, gotErr := captureOptimizerNodePositions([]*layoutgraph.Node{root}, ag)
				want, wantErr := referenceOptimizerNodePositions([]*layoutgraph.Node{root}, bg)
				if !reflect.DeepEqual(got, want) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || ag.Used() != bg.Used() || ac.remaining != bc.remaining {
					t.Fatalf("children=%d cancel=%d limit=%d: results differ: got=%v want=%v work=%d/%d polls=%d/%d", count, cancelAt, limit, gotErr, wantErr, ag.Used(), bg.Used(), ac.remaining, bc.remaining)
				}
			}
		}
	}
}

func TestOptimizerMutationScratchRecapturesAndReleases(t *testing.T) {
	var scratch optimizerMutationSnapshot
	for iteration, count := range []int{128, 1, 12} {
		graph, _ := candidateMovementGraph(count)
		graph.StoreEdgeLengthCost(42, float64(iteration))
		guard, _ := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
		snapshot, err := captureOptimizerMutationStateInto(graph, guard, &scratch)
		if err != nil {
			t.Fatal(err)
		}
		positions := captureExactOptimizerPositions(graph)
		for _, node := range graph.Nodes {
			node.TopLeft.X += 17
			node.Width += 29
		}
		graph.StoreEdgeLengthCost(42, -10)
		snapshot.restore()
		requireExactOptimizerPositions(t, positions)
		if cost, ok := graph.LookupEdgeLengthCost(42); !ok || cost != float64(iteration) {
			t.Fatal("cost snapshot was stale")
		}
		for _, node := range snapshot.nodes {
			if node.node.Width != node.width {
				t.Fatal("width snapshot was stale")
			}
		}
		scratch.release()
		if scratch.costSnapshot != nil || len(scratch.seenNodes) != 0 || len(scratch.seenClusters) != 0 || len(scratch.seenHerds) != 0 {
			t.Fatal("released scratch retained a graph")
		}
		for _, node := range scratch.nodes[:cap(scratch.nodes)] {
			if node.node != nil || node.topLeft.pointer != nil {
				t.Fatal("released node buffer retained a graph")
			}
		}
		for _, cluster := range scratch.clusters[:cap(scratch.clusters)] {
			if cluster.cluster != nil {
				t.Fatal("released cluster buffer retained a graph")
			}
		}
		for _, herd := range scratch.herds[:cap(scratch.herds)] {
			if herd.herd != nil {
				t.Fatal("released herd buffer retained a graph")
			}
		}
	}
}

func BenchmarkOptimizerSnapshotStorage(b *testing.B) {
	for _, count := range []int{1, 128} {
		graph, root := candidateMovementGraph(count)
		for _, reusable := range []bool{false, true} {
			b.Run(fmt.Sprintf("mutation/n=%d/reuse=%t", count, reusable), func(b *testing.B) {
				var scratch optimizerMutationSnapshot
				b.ReportAllocs()
				for b.Loop() {
					guard, _ := limits.NewOptimizationWorkGuard(context.Background(), "bench", limits.MaxOptimizationWorkUnits)
					var err error
					if reusable {
						_, err = captureOptimizerMutationStateInto(graph, guard, &scratch)
						scratch.release()
					} else {
						_, err = captureOptimizerMutationState(graph, guard)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run(fmt.Sprintf("position/n=%d/compact=%t", count, reusable), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					guard, _ := limits.NewOptimizationWorkGuard(context.Background(), "bench", limits.MaxOptimizationWorkUnits)
					var err error
					if reusable {
						_, err = captureOptimizerNodePositions([]*layoutgraph.Node{root}, guard)
					} else {
						_, err = referenceOptimizerNodePositions([]*layoutgraph.Node{root}, guard)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func referenceOptimizerPositionsSwapped(nodeA, nodeB *layoutgraph.Node, guard *limits.OptimizationWorkGuard, fn func() error) error {
	snapshots, err := captureOptimizerNodePositions([]*layoutgraph.Node{nodeA, nodeB}, guard)
	if err != nil {
		return err
	}
	defer restoreNodePositions(snapshots)
	if err := optimizerSwapPositions(nodeA, nodeB, guard); err != nil {
		return err
	}
	return fn()
}

func TestOptimizerSpeculativeSwapSnapshotParity(t *testing.T) {
	for _, count := range []int{1, 12, 128} {
		graph, root := candidateMovementGraph(count)
		other := graph.Nodes[len(graph.Nodes)-1]
		original := captureExactOptimizerPositions(graph)
		for cancelAt := 1; cancelAt <= 30; cancelAt++ {
			for _, limit := range []uint64{0, 1, 2, 10, 128, 512, 1024, limits.MaxOptimizationWorkUnits} {
				ac := &candidateMoveCancellation{Context: context.Background(), remaining: cancelAt}
				bc := &candidateMoveCancellation{Context: context.Background(), remaining: cancelAt}
				ag, _ := limits.NewOptimizationWorkGuard(ac, "test", limit)
				bg, _ := limits.NewOptimizationWorkGuard(bc, "test", limit)
				var got, want []nodePositionSnapshot
				gotErr := withOptimizerPositionsSwapped(root, other, ag, func() error {
					got, _ = referenceOptimizerNodePositions([]*layoutgraph.Node{root}, mustOptimizerGuard(t))
					return nil
				})
				requireExactOptimizerPositions(t, original)
				wantErr := referenceOptimizerPositionsSwapped(root, other, bg, func() error {
					want, _ = referenceOptimizerNodePositions([]*layoutgraph.Node{root}, mustOptimizerGuard(t))
					return nil
				})
				requireExactOptimizerPositions(t, original)
				if !reflect.DeepEqual(got, want) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) || ag.Used() != bg.Used() || ac.remaining != bc.remaining {
					t.Fatalf("children=%d cancel=%d limit=%d: swap results differ (%v/%v), work=%d/%d polls=%d/%d", count, cancelAt, limit, gotErr, wantErr, ag.Used(), bg.Used(), ac.remaining, bc.remaining)
				}
			}
		}
	}
}

func mustOptimizerGuard(t *testing.T) *limits.OptimizationWorkGuard {
	t.Helper()
	guard, err := limits.NewOptimizationWorkGuard(context.Background(), "test", limits.MaxOptimizationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func BenchmarkOptimizerSpeculativeSwapSnapshot(b *testing.B) {
	for _, count := range []int{1, 128} {
		graph, root := candidateMovementGraph(count)
		other := graph.Nodes[len(graph.Nodes)-1]
		for _, reuse := range []bool{false, true} {
			b.Run(fmt.Sprintf("n=%d/reuse=%t", count, reuse), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					guard, _ := limits.NewOptimizationWorkGuard(context.Background(), "bench", limits.MaxOptimizationWorkUnits)
					var err error
					if reuse {
						err = withOptimizerPositionsSwapped(root, other, guard, func() error { return nil })
					} else {
						err = referenceOptimizerPositionsSwapped(root, other, guard, func() error { return nil })
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func TestOptimizerSpeculativeSwapRestoresCallbackFailure(t *testing.T) {
	for _, panicCallback := range []bool{false, true} {
		graph, root := candidateMovementGraph(12)
		original := captureExactOptimizerPositions(graph)
		callbackReached := false
		func() {
			defer func() {
				if r := recover(); r != nil && (!panicCallback || r != "callback failure") {
					t.Fatalf("unexpected panic: %v", r)
				}
			}()
			err := withOptimizerPositionsSwapped(root, graph.Nodes[1], mustOptimizerGuard(t), func() error {
				callbackReached = true
				root.TopLeft.X += 100
				if panicCallback {
					panic("callback failure")
				}
				return fmt.Errorf("callback failure")
			})
			if err == nil {
				t.Fatal("callback error was discarded")
			}
		}()
		if !callbackReached {
			t.Fatal("callback not reached")
		}
		requireExactOptimizerPositions(t, original)
	}
}
