package placementcost

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"runtime"
	"sync"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/typedpool"
	"github.com/d2lang/d2/lib/geo"
)

const invalidEdgeLengthState uint64 = 0

func placementEdgeLengthState(graph *layoutgraph.Graph, costs layoutgraph.RoutingCostState, options EdgeLengthOptions) uint64 {
	h := fnv.New64a()
	var b [8]byte // Buffer for encoding numbers

	// Process booleans
	var flags byte
	if options.IncludeNodeSizes {
		flags |= 1 << 0
	}
	if options.EnforceMinimumGap {
		flags |= 1 << 1
	}
	if options.PenalizeDirection {
		flags |= 1 << 2
	}
	h.Write([]byte{flags})

	// Process graph costs
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(graph.CellSize))
	h.Write(b[:])
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(costs.Turn))
	h.Write(b[:])
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(costs.NonCenterPort))
	h.Write(b[:])
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(costs.Crossing))
	h.Write(b[:])

	// Process nodes
	for _, n := range graph.Nodes {
		if n.TopLeft == nil {
			return invalidEdgeLengthState
		}
		binary.LittleEndian.PutUint64(b[:], uint64(n.ID))
		h.Write(b[:])

		binary.LittleEndian.PutUint64(b[:], math.Float64bits(n.TopLeft.X))
		h.Write(b[:])
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(n.TopLeft.Y))
		h.Write(b[:])

		binary.LittleEndian.PutUint64(b[:], math.Float64bits(n.Width))
		h.Write(b[:])
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(n.Height))
		h.Write(b[:])

		for _, e := range n.Edges {
			binary.LittleEndian.PutUint64(b[:], uint64(e.ID))
			h.Write(b[:])
		}
	}

	// Process edge abductions
	for _, ea := range options.EdgeAbductions {
		binary.LittleEndian.PutUint64(b[:], uint64(ea.Edge.ID))
		h.Write(b[:])
		if ea.OriginallyFrom != nil {
			binary.LittleEndian.PutUint64(b[:], uint64(ea.OriginallyFrom.ID))
			h.Write(b[:])
		}
		if ea.OriginallyTo != nil {
			binary.LittleEndian.PutUint64(b[:], uint64(ea.OriginallyTo.ID))
			h.Write(b[:])
		}
		if ea.CurrentTo != nil {
			binary.LittleEndian.PutUint64(b[:], uint64(ea.CurrentTo.ID))
			h.Write(b[:])
		}
		if ea.CurrentFrom != nil {
			binary.LittleEndian.PutUint64(b[:], uint64(ea.CurrentFrom.ID))
			h.Write(b[:])
		}
	}

	return h.Sum64()
}

func edgeLengthWorkerCount(nodeCount, gomaxprocs int) int {
	if nodeCount <= 0 {
		return 0
	}
	// Preserve the inexpensive sequential path for small graphs.
	if nodeCount <= 10 {
		return 1
	}
	if gomaxprocs < 1 {
		gomaxprocs = 1
	}
	return min(nodeCount, gomaxprocs)
}

// EdgeLength evaluates the graph's placement cost and may populate its
// graph-owned edge-length cache.
func EdgeLength(ctx context.Context, graph *layoutgraph.Graph, options EdgeLengthOptions) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}

	edgeLengthState := placementEdgeLengthState(graph, graph.RoutingCosts(), options)
	if edgeLengthState != invalidEdgeLengthState {
		if d, ok := graph.LookupEdgeLengthCost(edgeLengthState); ok {
			return d, nil
		}
	}

	symmetryCost := graph.CellSize
	var totalSum float64

	workerCount := edgeLengthWorkerCount(len(graph.Nodes), runtime.GOMAXPROCS(0))
	var costs []float64
	if workerCount > 1 {
		costs = make([]float64, len(graph.Nodes))
		var wg sync.WaitGroup
		var panicOnce sync.Once
		var workerPanicked bool
		var errOnce sync.Once
		var workerErr error

		for worker := range workerCount {
			wg.Go(func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						panicOnce.Do(func() {
							// A recovered value can contain credentials or invoke an
							// attacker-controlled String method. Retain only the fact
							// that this worker violated an invariant.
							workerPanicked = true
						})
					}
				}()

				for i := worker; i < len(graph.Nodes); i += workerCount {
					n := graph.Nodes[i]
					nl, err := NodeEdgeLength(ctx, n, options)
					if err != nil {
						errOnce.Do(func() { workerErr = err })
						return
					}
					if options.IncludeNodeSizes {
						columnCrossingCost, err := ColumnCrossingCost(ctx, n, options.EdgeAbductions)
						if err != nil {
							errOnce.Do(func() { workerErr = err })
							return
						}
						symmetry, err := nodeSymmetry(ctx, n, options.EdgeAbductions, true)
						if err != nil {
							errOnce.Do(func() { workerErr = err })
							return
						}
						nl += columnCrossingCost
						nl -= symmetry * symmetryCost * float64(len(n.Edges))
					}
					if err := ctx.Err(); err != nil {
						errOnce.Do(func() { workerErr = fmt.Errorf("EdgeLength: %w", err) })
						return
					}

					costs[i] = nl
				}
			})
		}

		wg.Wait()
		if workerPanicked {
			panic("TALA EdgeLength worker invariant failure")
		}
		if workerErr != nil {
			return 0, workerErr
		}
	} else {
		for _, n := range graph.Nodes {
			nl, err := NodeEdgeLength(ctx, n, options)
			if err != nil {
				return 0, err
			}
			if options.IncludeNodeSizes {
				columnCrossingCost, err := ColumnCrossingCost(ctx, n, options.EdgeAbductions)
				if err != nil {
					return 0, err
				}
				symmetry, err := nodeSymmetry(ctx, n, options.EdgeAbductions, true)
				if err != nil {
					return 0, err
				}
				nl += columnCrossingCost
				nl -= symmetry * symmetryCost * float64(len(n.Edges))
			}
			if err := ctx.Err(); err != nil {
				return 0, fmt.Errorf("EdgeLength: %w", err)
			}
			totalSum += nl
		}
	}
	if workerCount > 1 {
		for _, c := range costs {
			totalSum += c
		}
	}

	crossings, err := GraphEdgeCrossings(ctx, graph)
	if err != nil {
		return 0, err
	}
	totalSum += graph.CrossingCost() * float64(crossings)
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	if edgeLengthState != invalidEdgeLengthState {
		graph.StoreEdgeLengthCost(edgeLengthState, totalSum)
	}

	return totalSum, nil
}

func countEdgeCrossingsExcludingSharedNodes(ctx context.Context, edges []*layoutgraph.Edge) (int64, error) {
	var crossings int64
	for i := 0; i < len(edges)-1; i++ {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		edge1 := edges[i]
		for j := i + 1; j < len(edges); j++ {
			if err := scoringCancellationError(ctx, j-i-1); err != nil {
				return 0, err
			}
			edge2 := edges[j]

			// Skip if edges share a common node
			if edge1.From == edge2.From || edge1.From == edge2.To ||
				edge1.To == edge2.From || edge1.To == edge2.To {
				continue
			}

			// Create segments and check if they cross
			seg1 := geo.NewSegment(edge1.From.Center(), edge1.To.Center())
			seg2 := geo.NewSegment(edge2.From.Center(), edge2.To.Center())

			if layoutgraph.SegmentsCross(seg1.Start, seg1.End, seg2.Start, seg2.End) {
				crossings++
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	return crossings, nil
}

type graphEdgeCrossingScratch struct {
	levelGroup map[int]int
	groups     [][]*layoutgraph.Edge
	active     int
}

const maxPooledGraphEdgeCrossingEntries = 4096

var graphEdgeCrossingScratchPool = typedpool.New(func() *graphEdgeCrossingScratch {
	return &graphEdgeCrossingScratch{levelGroup: make(map[int]int)}
})

func putGraphEdgeCrossingScratch(scratch *graphEdgeCrossingScratch) {
	if len(scratch.levelGroup) > maxPooledGraphEdgeCrossingEntries ||
		cap(scratch.groups) > maxPooledGraphEdgeCrossingEntries {
		return
	}
	remaining := maxPooledGraphEdgeCrossingEntries - cap(scratch.groups)
	for _, edges := range scratch.groups {
		if cap(edges) > remaining {
			return
		}
		remaining -= cap(edges)
	}

	for i := 0; i < scratch.active; i++ {
		clear(scratch.groups[i][:cap(scratch.groups[i])])
		scratch.groups[i] = scratch.groups[i][:0]
	}
	clear(scratch.levelGroup)
	scratch.active = 0
	graphEdgeCrossingScratchPool.Put(scratch)
}

// GraphEdgeCrossings counts crossings between edges that do not share a node.
func GraphEdgeCrossings(ctx context.Context, graph *layoutgraph.Graph) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	if len(graph.Edges) == 0 {
		if err := ctx.Err(); err != nil {
			return 0, fmt.Errorf("EdgeLength: %w", err)
		}
		return 0, nil
	}

	scratch := graphEdgeCrossingScratchPool.Get()
	defer putGraphEdgeCrossingScratch(scratch)

	for i, edge := range graph.Edges {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		if edge.From == nil || edge.To == nil || edge.From.TopLeft == nil || edge.To.TopLeft == nil {
			continue
		}
		fromLevel := edge.From.ContainerLevel()
		toLevel := edge.To.ContainerLevel()
		if fromLevel != toLevel {
			continue
		}

		groupIndex, exists := scratch.levelGroup[fromLevel]
		if !exists {
			groupIndex = scratch.active
			if groupIndex == len(scratch.groups) {
				scratch.groups = append(scratch.groups, nil)
			} else {
				scratch.groups[groupIndex] = scratch.groups[groupIndex][:0]
			}
			scratch.active++
			scratch.levelGroup[fromLevel] = groupIndex
		}
		scratch.groups[groupIndex] = append(scratch.groups[groupIndex], edge)
	}

	var totalCrossings int64
	for _, groupIndex := range scratch.levelGroup {
		crossings, err := countEdgeCrossingsExcludingSharedNodes(ctx, scratch.groups[groupIndex])
		if err != nil {
			return 0, err
		}
		totalCrossings += crossings
	}

	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	return totalCrossings, nil
}

// ContainerAlignmentCost scores misalignment between peer containers.
func ContainerAlignmentCost(ctx context.Context, graph *layoutgraph.Graph) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	l := 0.0
	for i := 0; i < len(graph.Nodes)-1; i++ {
		if err := scoringCancellationError(ctx, i); err != nil {
			return 0, err
		}
		n1 := graph.Nodes[i]
		if !n1.IsContainer() {
			continue
		}
		for j := i + 1; j < len(graph.Nodes); j++ {
			if err := scoringCancellationError(ctx, j-i-1); err != nil {
				return 0, err
			}
			n2 := graph.Nodes[j]
			if !n2.IsContainer() {
				continue
			}
			if n1.EffectiveContainer() != n2.EffectiveContainer() {
				continue
			}
			if n1.Width == n2.Width && n1.Height == n2.Height {
				if n1.TopLeft.X != n2.TopLeft.X && n1.TopLeft.Y != n2.TopLeft.Y {
					l += graph.NonCenterPortCost()
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("EdgeLength: %w", err)
	}
	return l, nil
}
