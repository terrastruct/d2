package layoutgraph

import "maps"

// LookupEdgeLengthCost reads one entry from the graph-owned placement cache
// while holding the graph's cost read lock.
func (graph *Graph) LookupEdgeLengthCost(state uint64) (float64, bool) {
	graph.costMu.RLock()
	defer graph.costMu.RUnlock()
	cost, ok := graph.edgeLengthCache[state]
	return cost, ok
}

// StoreEdgeLengthCost writes one entry while holding the graph's cost lock. It
// is a no-op when this graph has disabled placement caching.
func (graph *Graph) StoreEdgeLengthCost(state uint64, cost float64) {
	graph.costMu.Lock()
	defer graph.costMu.Unlock()
	if graph.edgeLengthCache != nil {
		graph.edgeLengthCache[state] = cost
	}
}

// EdgeLengthCacheEntries reports the size of the graph-owned placement cache.
func (graph *Graph) EdgeLengthCacheEntries() int {
	graph.costMu.RLock()
	defer graph.costMu.RUnlock()
	return len(graph.edgeLengthCache)
}

// ResetPlacementCosts clears route-derived scoring state without replacing
// the graph's cache map or changing whether caching is enabled.
func (graph *Graph) ResetPlacementCosts() {
	graph.costMu.Lock()
	defer graph.costMu.Unlock()
	clear(graph.edgeLengthCache)
	graph.crossingCost = 0
	graph.turnCost = 0
	graph.nonCenterPortCost = 0
}

// PlacementCostSnapshot is an opaque copy of graph-owned placement scoring
// state used to roll back a rejected optimizer candidate.
type PlacementCostSnapshot struct {
	graph             *Graph
	cacheRef          map[uint64]float64
	cache             map[uint64]float64
	crossingCost      float64
	turnCost          float64
	nonCenterPortCost float64
}

// SnapshotPlacementCosts copies graph-owned scoring state. The placement
// domain charges its own work budget before requesting this bounded copy.
func (graph *Graph) SnapshotPlacementCosts() *PlacementCostSnapshot {
	graph.costMu.RLock()
	defer graph.costMu.RUnlock()
	snapshot := &PlacementCostSnapshot{
		graph:             graph,
		cacheRef:          graph.edgeLengthCache,
		crossingCost:      graph.crossingCost,
		turnCost:          graph.turnCost,
		nonCenterPortCost: graph.nonCenterPortCost,
	}
	if graph.edgeLengthCache != nil {
		snapshot.cache = maps.Clone(graph.edgeLengthCache)
	}
	return snapshot
}

// Restore reinstates the captured costs and the exact cache map owned by the
// graph when the state was captured.
func (snapshot *PlacementCostSnapshot) Restore() {
	if snapshot == nil || snapshot.graph == nil {
		return
	}
	snapshot.graph.costMu.Lock()
	defer snapshot.graph.costMu.Unlock()
	if snapshot.cacheRef == nil {
		snapshot.graph.edgeLengthCache = nil
	} else {
		clear(snapshot.cacheRef)
		maps.Copy(snapshot.cacheRef, snapshot.cache)
		snapshot.graph.edgeLengthCache = snapshot.cacheRef
	}
	snapshot.graph.crossingCost = snapshot.crossingCost
	snapshot.graph.turnCost = snapshot.turnCost
	snapshot.graph.nonCenterPortCost = snapshot.nonCenterPortCost
}
