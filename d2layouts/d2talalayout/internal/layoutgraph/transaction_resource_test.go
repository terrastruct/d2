package layoutgraph

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

func (g *Graph) newTransactionContext(ctx context.Context, guard *limits.WorkGuard) (*Transaction, error) {
	return g.newTransactionWithOptionsContext(ctx, TransactionOptions{}, guard)
}

func legacyTransactionOverlaps(g *Graph) (map[*Node]map[*Node]struct{}, map[*Node]map[*Node]struct{}) {
	near := make(map[*Node]map[*Node]struct{})
	exact := make(map[*Node]map[*Node]struct{})
	add := func(overlaps map[*Node]map[*Node]struct{}, first, second *Node) {
		if overlaps[first] == nil {
			overlaps[first] = make(map[*Node]struct{})
		}
		if overlaps[second] == nil {
			overlaps[second] = make(map[*Node]struct{})
		}
		overlaps[first][second] = struct{}{}
		overlaps[second][first] = struct{}{}
	}
	for _, first := range g.Nodes {
		if first == nil || first.TopLeft == nil {
			continue
		}
		for _, second := range g.Nodes {
			if second == nil || second.TopLeft == nil || first == second {
				continue
			}
			if first.DoesOverlapExact(second) {
				add(exact, first, second)
			}
			if first.doesOverlap(second) {
				add(near, first, second)
			}
		}
	}
	return near, exact
}

func overlapPairIDs(overlaps map[*Node]map[*Node]struct{}) map[string]struct{} {
	pairs := make(map[string]struct{})
	for first, others := range overlaps {
		for second := range others {
			low, high := first.ID, second.ID
			if low > high {
				low, high = high, low
			}
			pairs[fmt.Sprintf("%d/%d", low, high)] = struct{}{}
		}
	}
	return pairs
}

func TestGraphStateChargesLongDistanceNeighborOnce(t *testing.T) {
	graph := NewGraph()
	from := graph.AddNode(NewNode(1, 10, 10))
	to := graph.AddNode(NewNode(2, 10, 10))

	measure := func(limit int64) (int64, error) {
		guard, err := limits.NewWorkGuard(context.Background(), "GraphStateNeighborWork", limit)
		require.NoError(t, err)
		state := NewGraphStateSnapshot(GraphStateSnapshotOptions{CaptureTopology: true})
		err = state.UpdateWithWorkGuard(graph, guard)
		return guard.Used(), err
	}

	baseline, err := measure(limits.MaxEngineWorkUnits)
	require.NoError(t, err)
	from.LongDistanceNeighborRequirements = map[*Node]LongDistanceNeighborRequirements{
		to: {EdgeCount: 3, MaxWidth: 100, MaxHeight: 200},
	}
	withNeighbor, err := measure(limits.MaxEngineWorkUnits)
	require.NoError(t, err)
	require.Equal(t, int64(1), withNeighbor-baseline)

	_, err = measure(withNeighbor - 1)
	require.ErrorContains(t, err, "work exceeds limit")
	used, err := measure(withNeighbor)
	require.NoError(t, err)
	require.Equal(t, withNeighbor, used)
}

func TestTransactionSweepMatchesLegacyAllPairs(t *testing.T) {
	for seed := int64(0); seed < 250; seed++ {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			g := NewGraph()
			nodeCount := 4 + rng.Intn(29)
			for i := 0; i < nodeCount; i++ {
				node := NewNode(EntityID(i+1), float64(1+rng.Intn(600)), float64(1+rng.Intn(600)))
				if rng.Intn(8) != 0 {
					node.TopLeft = geo.NewPoint(float64(rng.Intn(12_000)-2_000), float64(rng.Intn(12_000)-2_000))
				}
				node.margin = Spacing{
					top:    float64(rng.Intn(2_001)),
					bottom: float64(rng.Intn(2_001)),
					left:   float64(rng.Intn(2_001)),
					right:  float64(rng.Intn(2_001)),
				}
				if i%5 == 0 {
					node.LoopOffsets = map[geo.Orientation]float64{
						geo.Top:   float64(501 + rng.Intn(2_000)),
						geo.Right: float64(501 + rng.Intn(2_000)),
					}
				}
				if i%7 == 0 {
					node.shapeType = tableType
				}
				g.AddNodeUnchecked(node)
			}

			// Include ordinary and deliberately asymmetric adjacency. The latter is
			// malformed production state but was accepted by the old ordered scan,
			// so the index must preserve its union-of-directions semantics.
			for i := 0; i < nodeCount*2; i++ {
				from := g.Nodes[rng.Intn(nodeCount)]
				to := g.Nodes[rng.Intn(nodeCount)]
				if from == to {
					continue
				}
				edge := NewEdge(from, to)
				edge.MinWidth = 501 + rng.Intn(3_000)
				edge.MinHeight = 501 + rng.Intn(3_000)
				g.Edges = append(g.Edges, edge)
				from.Edges = append(from.Edges, edge)
				if rng.Intn(3) != 0 {
					to.Edges = append(to.Edges, edge)
				}
			}

			wantNear, wantExact := legacyTransactionOverlaps(g)
			guard, err := limits.NewWorkGuard(context.Background(), "TransactionSweepTest", maxTransactionWorkUnits)
			require.NoError(t, err)
			gotNear, gotExact, err := buildTransactionOverlaps(g, guard)
			require.NoError(t, err)
			require.Equal(t, overlapPairIDs(wantNear), overlapPairIDs(gotNear))
			require.Equal(t, overlapPairIDs(wantExact), overlapPairIDs(gotExact))
		})
	}
}

func TestTransactionSweepMatchesLegacyWithDuplicateTopLevelReference(t *testing.T) {
	g := NewGraph()
	first := NewNode(1, 100, 100)
	first.TopLeft = geo.NewPoint(0, 0)
	second := NewNode(2, 100, 100)
	second.TopLeft = geo.NewPoint(50, 50)
	third := NewNode(3, 25, 25)
	third.TopLeft = geo.NewPoint(1_000, 1_000)
	g.AddNodeUnchecked(first)
	g.AddNodeUnchecked(second)
	g.AddNodeUnchecked(first)
	g.AddNodeUnchecked(third)

	wantNear, wantExact := legacyTransactionOverlaps(g)
	guard, err := limits.NewWorkGuard(context.Background(), "TransactionDuplicateReferenceTest", maxTransactionWorkUnits)
	require.NoError(t, err)
	gotNear, gotExact, err := buildTransactionOverlaps(g, guard)
	require.NoError(t, err)
	require.Equal(t, overlapPairIDs(wantNear), overlapPairIDs(gotNear))
	require.Equal(t, overlapPairIDs(wantExact), overlapPairIDs(gotExact))
	require.NotContains(t, gotNear[first], first)
	require.NotContains(t, gotExact[first], first)
}

func TestTransactionSweepReferenceLimitIsAggregate(t *testing.T) {
	g := NewGraph()
	for i := 0; i < 5; i++ {
		node := NewNode(EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(0, 0)
		g.AddNodeUnchecked(node)
	}

	// Ten unordered pairs are retained twice in each of the exact and near
	// maps: 10 * 2 * 2 = 40 directed references.
	guard, err := limits.NewWorkGuard(context.Background(), "TransactionReferenceTest", 1_000)
	require.NoError(t, err)
	_, _, err = buildTransactionOverlapsWithReferenceLimit(g, guard, 39)
	require.ErrorContains(t, err, "overlap references exceed limit 39")

	guard, err = limits.NewWorkGuard(context.Background(), "TransactionReferenceTest", 1_000)
	require.NoError(t, err)
	near, exact, err := buildTransactionOverlapsWithReferenceLimit(g, guard, 40)
	require.NoError(t, err)
	require.Len(t, overlapPairIDs(near), 10)
	require.Len(t, overlapPairIDs(exact), 10)
}

func TestTransactionSupportsSparseMaximumNodeCount(t *testing.T) {
	g := NewGraph()
	for i := 0; i < maxEngineNodes; i++ {
		node := NewNode(EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i*1_000), float64((i%17)*1_000))
		g.AddNodeUnchecked(node)
	}

	txn, err := g.newTransactionContext(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, txn)
	require.Empty(t, txn.PriorGraphState.existingOverlaps)
	require.Empty(t, txn.PriorGraphState.existingExactOverlaps)
}

func TestTransactionDeduplicatesClusterAndSequenceMembership(t *testing.T) {
	g := NewGraph()
	for i := 0; i < 4_000; i++ {
		node := NewNode(EntityID(i+1), 10, 10)
		node.TopLeft = geo.NewPoint(float64(i*1_000), 0)
		g.AddNodeUnchecked(node)
	}
	g.Clusters[g.Nodes[0]] = &Cluster{Vessel: g.Nodes[0], Nodes: g.Nodes, Graph: g}
	g.Sequences[g.Nodes[1]] = &Sequence{Vessel: g.Nodes[1], Nodes: g.Nodes, Graph: g}

	txn, err := g.newTransactionContext(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, txn.PriorGraphState.nodeGeometry, 4_000)
}

func TestTransactionConstructorPreservesCancellationIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewGraph().newTransactionContext(ctx, nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestTransactionUpdateStateLimitRollsBackAcceptedMutation(t *testing.T) {
	g := NewGraph()
	node := NewNode(1, 10, 10)
	originalTopLeft := geo.NewPoint(1, 2)
	node.TopLeft = originalTopLeft
	g.AddNodeUnchecked(node)

	txn, err := g.newTransactionContext(context.Background(), nil)
	require.NoError(t, err)
	txn.AddOp(func() error {
		node.TopLeft = geo.NewPoint(100, 200)
		return nil
	})
	require.NoError(t, txn.Commit(context.Background()))
	txn.Clear()

	lowLimit, err := limits.NewWorkGuard(context.Background(), "TransactionUpdateStateTest", 1)
	require.NoError(t, err)
	require.NoError(t, lowLimit.Step())
	txn.guard = lowLimit
	require.ErrorContains(t, txn.UpdateState(), "work exceeds limit 1")
	require.Same(t, originalTopLeft, node.TopLeft)
	require.Equal(t, geo.Point{X: 1, Y: 2}, *node.TopLeft)
}
