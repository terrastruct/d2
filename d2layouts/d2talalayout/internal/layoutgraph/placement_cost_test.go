package layoutgraph

import (
	"testing"
)

func TestPlacementCostSnapshotRestoresCacheIdentity(t *testing.T) {
	graph := NewGraph()
	graph.StoreEdgeLengthCost(1, 10)
	graph.StoreEdgeLengthCost(2, 20)
	graph.RestoreRoutingCosts(RoutingCostState{Crossing: 30, Turn: 40, NonCenterPort: 50})
	originalCache := graph.edgeLengthCache

	snapshot := graph.SnapshotPlacementCosts()

	graph.costMu.Lock()
	graph.edgeLengthCache = map[uint64]float64{3: 300}
	graph.crossingCost = 300
	graph.turnCost = 400
	graph.nonCenterPortCost = 500
	graph.costMu.Unlock()
	snapshot.Restore()

	graph.costMu.Lock()
	originalCache[99] = 990
	graph.costMu.Unlock()
	if got, ok := graph.LookupEdgeLengthCost(99); !ok || got != 990 {
		t.Fatal("rollback did not restore the original cache map")
	}
	graph.costMu.Lock()
	delete(originalCache, 99)
	graph.costMu.Unlock()

	if costs := graph.RoutingCosts(); costs != (RoutingCostState{Crossing: 30, Turn: 40, NonCenterPort: 50}) {
		t.Fatalf("restored costs = %+v, want (30, 40, 50)", costs)
	}
	if graph.EdgeLengthCacheEntries() != 2 {
		t.Fatalf("restored cache entries = %d, want 2", graph.EdgeLengthCacheEntries())
	}
	for key, want := range map[uint64]float64{1: 10, 2: 20} {
		if got, ok := graph.LookupEdgeLengthCost(key); !ok || got != want {
			t.Fatalf("restored cache[%d] = (%v, %v), want (%v, true)", key, got, ok, want)
		}
	}
}

func TestResetPlacementCostsPreservesCacheModeAndIdentity(t *testing.T) {
	graph := NewGraph()
	originalCache := graph.edgeLengthCache
	graph.StoreEdgeLengthCost(1, 10)
	graph.RestoreRoutingCosts(RoutingCostState{Crossing: 20, Turn: 30, NonCenterPort: 40})

	graph.ResetPlacementCosts()

	graph.costMu.Lock()
	originalCache[2] = 20
	graph.costMu.Unlock()
	if got, ok := graph.LookupEdgeLengthCost(2); !ok || got != 20 {
		t.Fatal("reset replaced the enabled cache map")
	}
	if costs := graph.RoutingCosts(); costs != (RoutingCostState{}) {
		t.Fatalf("reset costs = %+v, want zeroes", costs)
	}

	graph.costMu.Lock()
	graph.edgeLengthCache = nil
	graph.costMu.Unlock()
	graph.ResetPlacementCosts()
	if graph.edgeLengthCache != nil {
		t.Fatal("reset enabled a disabled cache")
	}
}

func TestPublishDeserializedGraphReplacesPlacementCache(t *testing.T) {
	graph := NewGraph()
	graph.StoreEdgeLengthCost(1, 10)
	originalCache := graph.edgeLengthCache

	PublishDeserializedGraph(graph, NewGraph(), 2, 3, 4)

	if graph.edgeLengthCache == nil || len(graph.edgeLengthCache) != 0 {
		t.Fatal("deserialization did not install an enabled empty cache")
	}
	if originalCache[1] != 10 {
		t.Fatal("deserialization mutated the detached cache")
	}
	graph.costMu.Lock()
	originalCache[2] = 20
	graph.costMu.Unlock()
	if _, ok := graph.LookupEdgeLengthCost(2); ok {
		t.Fatal("deserialization retained the prior cache map")
	}
	if costs := graph.RoutingCosts(); costs != (RoutingCostState{Crossing: 2, Turn: 3, NonCenterPort: 4}) {
		t.Fatalf("deserialized costs = %+v, want (2, 3, 4)", costs)
	}
}
