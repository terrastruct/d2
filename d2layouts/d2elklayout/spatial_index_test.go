package d2elklayout

import (
	"math"
	"math/rand"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/lib/geo"
)

func TestBendCollisionIndexMatchesBruteForce(t *testing.T) {
	g := compileBenchmarkGraph(t, denseBenchmarkInput(60, 180))
	setSyntheticRoutes(g)
	index := newBendCollisionIndex(g)

	queries := make([]geo.Segment, 0, len(g.Edges)*2+100)
	for _, edge := range g.Edges {
		for i := 0; i+1 < len(edge.Route); i += 2 {
			queries = append(queries, *geo.NewSegment(edge.Route[i].Copy(), edge.Route[i+1].Copy()))
		}
	}
	random := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		x := random.Float64()*4500 - 250
		y := random.Float64()*800 - 150
		dx := random.Float64()*700 - 350
		dy := random.Float64()*200 - 100
		queries = append(queries, *geo.NewSegment(geo.NewPoint(x, y), geo.NewPoint(x+dx, y+dy)))
	}
	// Exercise the fallback path for a query spanning more than the grid's
	// bounded number of cells.
	queries = append(queries, *geo.NewSegment(geo.NewPoint(-100_000, 20), geo.NewPoint(100_000, 20)))

	assertBendCollisionQueriesMatch(t, g, index, queries)

	// The production algorithm mutates routes after accepting a simplification.
	// Verify removals and insertions keep the broad phase exact.
	changed := g.Edges[len(g.Edges)/2]
	changed.Route = []*geo.Point{
		geo.NewPoint(-25_000, -50),
		geo.NewPoint(25_000, -50),
		geo.NewPoint(25_000, 300),
		geo.NewPoint(80, 300),
	}
	index.updateEdge(changed)
	assertBendCollisionQueriesMatch(t, g, index, queries)
}

func TestSpatialIndexFallsBackOutsideWasm32IntRange(t *testing.T) {
	index := newSpatialIndex[int](1)
	coordinate := float64(1<<31) * index.cellSize
	if cells, ok := index.coveredCells(spatialBounds{
		minX: coordinate,
		maxX: coordinate,
	}); ok || cells != nil {
		t.Fatalf("positive out-of-range coordinate used grid cells: %#v", cells)
	}
	if cells, ok := index.coveredCells(spatialBounds{
		minX: -coordinate,
		maxX: -coordinate,
	}); ok || cells != nil {
		t.Fatalf("negative out-of-range coordinate used grid cells: %#v", cells)
	}
}

func assertBendCollisionQueriesMatch(t *testing.T, g *d2graph.Graph, index *bendCollisionIndex, queries []geo.Segment) {
	t.Helper()
	for queryIndex, query := range queries {
		sourceEdge := g.Edges[queryIndex%len(g.Edges)]
		gotObjects := index.countObjectIntersects(sourceEdge.Src, sourceEdge.Dst, query)
		wantObjects := bruteObjectIntersects(g, sourceEdge.Src, sourceEdge.Dst, query)
		if gotObjects != wantObjects {
			t.Fatalf("query %d object intersections = %d, want %d", queryIndex, gotObjects, wantObjects)
		}
		gotCrossings, gotOverlaps, gotClose, gotTouching := index.countEdgeIntersects(sourceEdge, query)
		wantCrossings, wantOverlaps, wantClose, wantTouching := bruteEdgeIntersects(g, sourceEdge, query)
		if gotCrossings != wantCrossings || gotOverlaps != wantOverlaps || gotClose != wantClose || gotTouching != wantTouching {
			t.Fatalf(
				"query %d edge intersections = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
				queryIndex, gotCrossings, gotOverlaps, gotClose, gotTouching,
				wantCrossings, wantOverlaps, wantClose, wantTouching,
			)
		}
	}
}

func bruteObjectIntersects(g *d2graph.Graph, src, dst *d2graph.Object, segment geo.Segment) int {
	count := 0
	for _, obj := range g.Objects {
		if obj == src || obj == dst {
			continue
		}
		if obj.Intersects(segment, float64(edge_node_spacing)-1) {
			count++
		}
	}
	return count
}

func bruteEdgeIntersects(g *d2graph.Graph, sourceEdge *d2graph.Edge, segment geo.Segment) (int, int, int, int) {
	isHorizontal := math.Ceil(segment.Start.Y) == math.Ceil(segment.End.Y)
	crossingsCount := 0
	overlapsCount := 0
	closeOverlapsCount := 0
	touchingCount := 0
	for _, edge := range g.Edges {
		if edge == sourceEdge {
			continue
		}
		for i := 0; i+1 < len(edge.Route); i++ {
			otherSegment := *geo.NewSegment(edge.Route[i], edge.Route[i+1])
			otherIsHorizontal := math.Ceil(otherSegment.Start.Y) == math.Ceil(otherSegment.End.Y)
			if isHorizontal == otherIsHorizontal {
				if segment.Overlaps(otherSegment, !isHorizontal, 0.) {
					if isHorizontal {
						if math.Abs(segment.Start.Y-otherSegment.Start.Y) < float64(edge_node_spacing)/2. {
							overlapsCount++
							if math.Abs(segment.Start.Y-otherSegment.Start.Y) < float64(edge_node_spacing)/4. {
								closeOverlapsCount++
								if math.Abs(segment.Start.Y-otherSegment.Start.Y) < 1. {
									touchingCount++
								}
							}
						}
					} else if math.Abs(segment.Start.X-otherSegment.Start.X) < float64(edge_node_spacing)/2. {
						overlapsCount++
						if math.Abs(segment.Start.X-otherSegment.Start.X) < float64(edge_node_spacing)/4. {
							closeOverlapsCount++
							if math.Abs(segment.Start.Y-otherSegment.Start.Y) < 1. {
								touchingCount++
							}
						}
					}
				}
			} else if segment.Intersects(otherSegment) {
				crossingsCount++
			}
		}
	}
	return crossingsCount, overlapsCount, closeOverlapsCount, touchingCount
}
