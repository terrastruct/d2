package labeling

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func findSharedSegments(edges []*layoutgraph.Edge) []*geo.Segment {
	segments, _ := findSharedSegmentsChecked(edges, nil)
	return segments
}

func isClusterPathShared(e *layoutgraph.Edge) bool {
	shared, _ := isClusterPathSharedChecked(e, nil)
	return shared
}

func TestLabelPercentageSearchRangeIsMirrorInvariant(t *testing.T) {
	from := layoutgraph.NewNode(1, 10, 10)
	from.Cluster = &layoutgraph.Cluster{}
	to := layoutgraph.NewNode(2, 10, 10)

	tests := []struct {
		name string
		p1   *geo.Point
		p2   *geo.Point
		p3   *geo.Point
		want float64
	}{
		{name: "right", p1: geo.NewPoint(0, 0), p2: geo.NewPoint(100, 0), p3: geo.NewPoint(100, 100), want: 0.45},
		{name: "left", p1: geo.NewPoint(0, 0), p2: geo.NewPoint(-100, 0), p3: geo.NewPoint(-100, 100), want: 0.45},
		{name: "down", p1: geo.NewPoint(0, 0), p2: geo.NewPoint(0, 100), p3: geo.NewPoint(100, 100), want: 0.475},
		{name: "up", p1: geo.NewPoint(0, 0), p2: geo.NewPoint(0, -100), p3: geo.NewPoint(100, -100), want: 0.475},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := layoutgraph.NewEdge(from, to)
			e.Label = &layoutgraph.Label{Width: 20, Height: 10}
			e.Points = []*geo.Point{tt.p1, tt.p2, tt.p3}
			r := labelPercentageSearchRange(e, e.Length())
			if math.Abs(r.start) > 1e-12 || math.Abs(r.end-tt.want) > 1e-12 {
				t.Fatalf("range = {%v, %v}, want {0, %v}", r.start, r.end, tt.want)
			}
		})
	}
}

func TestIsClusterPathSharedRequiresOverlappingIntervals(t *testing.T) {
	tests := []struct {
		name       string
		first      []*geo.Point
		second     []*geo.Point
		wantShared bool
	}{
		{
			name:   "vertical disjoint",
			first:  []*geo.Point{geo.NewPoint(-10, 0), geo.NewPoint(0, 0), geo.NewPoint(0, 10)},
			second: []*geo.Point{geo.NewPoint(10, 30), geo.NewPoint(0, 30), geo.NewPoint(0, 20)},
		},
		{
			name:       "vertical touching",
			first:      []*geo.Point{geo.NewPoint(-10, 0), geo.NewPoint(0, 0), geo.NewPoint(0, 10)},
			second:     []*geo.Point{geo.NewPoint(10, 20), geo.NewPoint(0, 20), geo.NewPoint(0, 10)},
			wantShared: true,
		},
		{
			name:   "horizontal disjoint",
			first:  []*geo.Point{geo.NewPoint(0, -10), geo.NewPoint(0, 0), geo.NewPoint(10, 0)},
			second: []*geo.Point{geo.NewPoint(30, 10), geo.NewPoint(30, 0), geo.NewPoint(20, 0)},
		},
		{
			name:       "horizontal touching",
			first:      []*geo.Point{geo.NewPoint(0, -10), geo.NewPoint(0, 0), geo.NewPoint(10, 0)},
			second:     []*geo.Point{geo.NewPoint(20, 10), geo.NewPoint(20, 0), geo.NewPoint(10, 0)},
			wantShared: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adjacent := layoutgraph.NewNode(1, 10, 10)
			firstNode := layoutgraph.NewNode(2, 10, 10)
			secondNode := layoutgraph.NewNode(3, 10, 10)
			cluster := &layoutgraph.Cluster{Nodes: []*layoutgraph.Node{firstNode, secondNode}}
			firstNode.Cluster = cluster
			secondNode.Cluster = cluster

			firstEdge := layoutgraph.NewEdge(firstNode, adjacent)
			firstEdge.Points = tt.first
			secondEdge := layoutgraph.NewEdge(secondNode, adjacent)
			secondEdge.Points = tt.second
			firstNode.Edges = []*layoutgraph.Edge{firstEdge}
			secondNode.Edges = []*layoutgraph.Edge{secondEdge}

			if got := isClusterPathShared(firstEdge); got != tt.wantShared {
				t.Fatalf("isClusterPathShared() = %v, want %v", got, tt.wantShared)
			}
		})
	}
}

func TestFindSharedSegments(t *testing.T) {
	// edges:
	// - e1: n1 -- n2
	// - e2: n1 -- n3
	// - e3: n3 -- n2
	// - e4: n2 -- n3
	// shared segments are highlighted aby `#`
	//                 ┌──────┐
	//                 │  n3  │
	//     ┌───e3──####┤      │
	//     │       #   └──────┘
	//     │    e2 #
	// ┌───┴───┐   #
	// │       ├e4─#
	// │  n2   ├e1─#
	// └───────┘   #
	//             #
	//             #  ┌───────┐
	//             #  │       │
	//             #──┤  n1   │
	//             └──┤       │
	//                └───────┘

	// no need for nodes or graph here
	e1 := layoutgraph.NewEdge(nil, nil)
	e1.Points = []*geo.Point{
		geo.NewPoint(300, 300),
		geo.NewPoint(200, 300),
		geo.NewPoint(200, 200),
		geo.NewPoint(100, 200),
	}
	e2 := layoutgraph.NewEdge(nil, nil)
	e2.Points = []*geo.Point{
		geo.NewPoint(300, 350),
		geo.NewPoint(200, 350),
		geo.NewPoint(200, 100),
		geo.NewPoint(300, 100),
	}
	e3 := layoutgraph.NewEdge(nil, nil)
	e3.Points = []*geo.Point{
		geo.NewPoint(300, 100),
		geo.NewPoint(50, 100),
		geo.NewPoint(50, 150),
	}
	e4 := layoutgraph.NewEdge(nil, nil)
	e4.Points = []*geo.Point{
		geo.NewPoint(300, 100),
		geo.NewPoint(200, 100),
		geo.NewPoint(200, 180),
		geo.NewPoint(100, 180),
	}

	edges := []*layoutgraph.Edge{e1, e2, e3, e4}
	segments := findSharedSegments(edges)
	assert.Equal(t, 3, len(segments))
	// e2 and e4 share 100..180; e2 alone occupies the exclusive 180..200
	// corridor, then e1 and e2 share 200..300. The gap must remain available
	// for a label that should belong unambiguously to e2.
	assert.Equal(t, 200., segments[0].Start.X)
	assert.Equal(t, 100., segments[0].Start.Y)
	assert.Equal(t, 200., segments[0].End.X)
	assert.Equal(t, 180., segments[0].End.Y)
	assert.Equal(t, 200., segments[1].Start.X)
	assert.Equal(t, 200., segments[1].Start.Y)
	assert.Equal(t, 200., segments[1].End.X)
	assert.Equal(t, 300., segments[1].End.Y)
	// Horizontal segment shared between e2, e3 and e4.
	assert.Equal(t, 200., segments[2].Start.X)
	assert.Equal(t, 100., segments[2].Start.Y)
	assert.Equal(t, 300., segments[2].End.X)
	assert.Equal(t, 100., segments[2].End.Y)
}
