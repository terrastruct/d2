package d2dagrelayout

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/textmeasure"
	"github.com/d2lang/util-go/go2"
)

func TestDeduplicateRoutePoints(t *testing.T) {
	t.Parallel()
	points := []*geo.Point{
		geo.NewPoint(1, 2),
		geo.NewPoint(1, 2),
		geo.NewPoint(3, 4),
		geo.NewPoint(3, 4),
		geo.NewPoint(1, 2),
	}
	got := deduplicateRoutePoints(points)
	want := []*geo.Point{geo.NewPoint(1, 2), geo.NewPoint(3, 4), geo.NewPoint(1, 2)}
	if len(got) != len(want) {
		t.Fatalf("deduplicated route length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].Equals(want[i]) {
			t.Fatalf("deduplicated route[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestModernSelfLoopRoutesRemainFiniteAndConnected(t *testing.T) {
	t.Parallel()
	g, _, err := d2compiler.Compile("index.d2", strings.NewReader("x -> x\nx -> x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.ApplyTheme(d2themescatalog.NeutralDefault.ID); err != nil {
		t.Fatal(err)
	}
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetDimensions(nil, ruler, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := DefaultLayout(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	for _, edge := range g.Edges {
		if len(edge.Route) < 2 {
			t.Fatalf("%s route has %d points", edge.AbsID(), len(edge.Route))
		}
		for i, point := range edge.Route {
			if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
				t.Fatalf("%s route[%d] is not finite: %v", edge.AbsID(), i, point)
			}
		}
		if !pointOnBoxBorder(edge.Route[0], edge.Src.Box) {
			t.Fatalf("%s route does not start on its source: %v, box %v", edge.AbsID(), edge.Route[0], edge.Src.Box)
		}
		if !pointOnBoxBorder(edge.Route[len(edge.Route)-1], edge.Dst.Box) {
			t.Fatalf("%s route does not end on its destination: %v, box %v", edge.AbsID(), edge.Route[len(edge.Route)-1], edge.Dst.Box)
		}
	}
}

func TestCompoundParallelCycleRemainsFinite(t *testing.T) {
	t.Parallel()
	g, _, err := d2compiler.Compile("index.d2", strings.NewReader("a.b -> c -> a.b <- c"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.ApplyTheme(d2themescatalog.NeutralDefault.ID); err != nil {
		t.Fatal(err)
	}
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetDimensions(nil, ruler, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := DefaultLayout(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if got, want := len(g.Edges), 3; got != want {
		t.Fatalf("edges = %d, want %d", got, want)
	}
	for _, obj := range g.Objects {
		for name, value := range map[string]float64{
			"x": obj.TopLeft.X, "y": obj.TopLeft.Y, "width": obj.Width, "height": obj.Height,
		} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("%s %s is not finite: %v", obj.AbsID(), name, value)
			}
		}
	}
	for _, edge := range g.Edges {
		if len(edge.Route) < 2 {
			t.Fatalf("%s route has %d points", edge.AbsID(), len(edge.Route))
		}
		for i, point := range edge.Route {
			if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
				t.Fatalf("%s route[%d] is not finite: %v", edge.AbsID(), i, point)
			}
		}
		if !pointOnBoxBorder(edge.Route[0], edge.Src.Box) {
			t.Fatalf("%s route does not start on its source: %v, box %v", edge.AbsID(), edge.Route[0], edge.Src.Box)
		}
		if !pointOnBoxBorder(edge.Route[len(edge.Route)-1], edge.Dst.Box) {
			t.Fatalf("%s route does not end on its destination: %v, box %v", edge.AbsID(), edge.Route[len(edge.Route)-1], edge.Dst.Box)
		}
	}
}

func pointOnBoxBorder(point *geo.Point, box *geo.Box) bool {
	const epsilon = 1
	left, right := box.TopLeft.X, box.TopLeft.X+box.Width
	top, bottom := box.TopLeft.Y, box.TopLeft.Y+box.Height
	near := func(a, b float64) bool { return math.Abs(a-b) <= epsilon }
	return ((near(point.X, left) || near(point.X, right)) && point.Y >= top-epsilon && point.Y <= bottom+epsilon) ||
		((near(point.Y, top) || near(point.Y, bottom)) && point.X >= left-epsilon && point.X <= right+epsilon)
}

func TestContainerTopologyMatchesLegacyTraversal(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"chains": `
x: {
  a -> b -> c
  d -> e
}
x -> outside
outside -> x
`,
		"nested": `
x: {
  a: {
    a1 -> a2
  }
  b: {
    b1 -> b2
  }
  c
  a -> b
  b -> c
}
y -> x.a.a1
x.b.b2 -> z
`,
		"cycles-and-parallel": `
x: {
  a -> b
  b -> a
  a -> b
  c -> c
  d
}
x -> y
y -> x
`,
		"container-endpoints": `
x: {
  a: {
    a1 -> a2
  }
  b
  a -> x
  x -> b
}
x -> z
z -> x
`,
	}

	for name, input := range tests {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g, _, err := d2compiler.Compile("index.d2", strings.NewReader(input), nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, container := range g.Objects {
				if len(container.ChildrenArray) == 0 {
					continue
				}
				gotHead, gotTail := getLongestEdgeChainEndpoints(g, container)
				wantHead := legacyLongestEdgeChainHead(g, container)
				wantTail := legacyLongestEdgeChainTail(g, container)
				if gotHead != wantHead || gotTail != wantTail {
					t.Fatalf("%s: endpoints got (%s, %s), want (%s, %s)", container.AbsID(), gotHead.AbsID(), gotTail.AbsID(), wantHead.AbsID(), wantTail.AbsID())
				}
			}
		})
	}
}

func TestCrossRankIndexPreservesIncidentEdgeOrder(t *testing.T) {
	t.Parallel()
	g, _, err := d2compiler.Compile("index.d2", strings.NewReader(`
a: {
  b: {
    c -> d
  }
  e
}
f -> a.b.c
a.e -> f
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	index := newCrossRankIndex(g)
	objects := append([]*d2graph.Object{g.Root}, g.Objects...)
	for _, obj := range objects {
		var wantEdges []*d2graph.Edge
		for _, edge := range g.Edges {
			if edge.Src == obj || edge.Dst == obj {
				wantEdges = append(wantEdges, edge)
			}
		}
		gotEdges := index.incident[obj]
		if len(gotEdges) != len(wantEdges) {
			t.Fatalf("incident edge count for %s = %d, want %d", obj.AbsID(), len(gotEdges), len(wantEdges))
		}
		for edgeIndex := range wantEdges {
			if gotEdges[edgeIndex] != wantEdges[edgeIndex] {
				t.Fatalf("incident edge %d for %s is out of order", edgeIndex, obj.AbsID())
			}
		}
	}
}

func legacyLongestEdgeChainHead(g *d2graph.Graph, container *d2graph.Object) *d2graph.Object {
	rank := make(map[*d2graph.Object]int)
	chainLength := make(map[*d2graph.Object]int)

	for _, obj := range container.ChildrenArray {
		isHead := true
		for _, edge := range g.Edges {
			if inContainer(edge.Src, container) != nil && inContainer(edge.Dst, obj) != nil {
				isHead = false
				break
			}
		}
		if !isHead {
			continue
		}
		rank[obj] = 1
		chainLength[obj] = 1
		queue := []*d2graph.Object{obj}
		visited := make(map[*d2graph.Object]struct{})
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			if _, ok := visited[curr]; ok {
				continue
			}
			visited[curr] = struct{}{}
			for _, edge := range g.Edges {
				child := inContainer(edge.Dst, container)
				if child == curr {
					continue
				}
				if child != nil && inContainer(edge.Src, curr) != nil {
					if rank[curr]+1 > rank[child] {
						rank[child] = rank[curr] + 1
						chainLength[obj] = go2.Max(chainLength[obj], rank[child])
					}
					queue = append(queue, child)
				}
			}
		}
	}
	max := int(math.MinInt32)
	for _, obj := range container.ChildrenArray {
		max = go2.Max(max, chainLength[obj])
	}
	var heads []*d2graph.Object
	for _, obj := range container.ChildrenArray {
		if rank[obj] == 1 && chainLength[obj] == max {
			heads = append(heads, obj)
		}
	}
	if len(heads) > 0 {
		return heads[len(heads)/2]
	}
	return container.ChildrenArray[0]
}

func legacyLongestEdgeChainTail(g *d2graph.Graph, container *d2graph.Object) *d2graph.Object {
	rank := make(map[*d2graph.Object]int)
	for _, obj := range container.ChildrenArray {
		isHead := true
		for _, edge := range g.Edges {
			if inContainer(edge.Src, container) != nil && inContainer(edge.Dst, obj) != nil {
				isHead = false
				break
			}
		}
		if !isHead {
			continue
		}
		rank[obj] = 1
		queue := []*d2graph.Object{obj}
		visited := make(map[*d2graph.Object]struct{})
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			if _, ok := visited[curr]; ok {
				continue
			}
			visited[curr] = struct{}{}
			for _, edge := range g.Edges {
				child := inContainer(edge.Dst, container)
				if child == curr {
					continue
				}
				if child != nil && inContainer(edge.Src, curr) != nil {
					rank[child] = go2.Max(rank[child], rank[curr]+1)
					queue = append(queue, child)
				}
			}
		}
	}
	max := int(math.MinInt32)
	for _, obj := range container.ChildrenArray {
		max = go2.Max(max, rank[obj])
	}
	var tails []*d2graph.Object
	for _, obj := range container.ChildrenArray {
		if rank[obj] == max {
			tails = append(tails, obj)
		}
	}
	return tails[len(tails)/2]
}
