package d2elklayout

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func BenchmarkLayoutDense(b *testing.B) {
	for _, fixture := range []struct {
		name         string
		nodes, edges int
	}{
		{name: "100_nodes_200_edges", nodes: 100, edges: 200},
		{name: "250_nodes_1000_edges", nodes: 250, edges: 1000},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			input := denseBenchmarkInput(fixture.nodes, fixture.edges)
			ctx := context.Background()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				g := compileBenchmarkGraph(b, input)
				b.StartTimer()
				if err := Layout(ctx, g, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDeleteBendsDense(b *testing.B) {
	for _, fixture := range []struct {
		name         string
		nodes, edges int
	}{
		{name: "100_nodes_200_edges", nodes: 100, edges: 200},
		{name: "250_nodes_1000_edges", nodes: 250, edges: 1000},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			input := denseBenchmarkInput(fixture.nodes, fixture.edges)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				g := compileBenchmarkGraph(b, input)
				setSyntheticRoutes(g)
				b.StartTimer()
				deleteBends(g)
			}
		})
	}
}

func BenchmarkELKGraphStatsNested(b *testing.B) {
	g := compileBenchmarkGraph(b, nestedBenchmarkInput(4, 4))
	b.Run("repeated_scans", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkIntSink = repeatedScanStats(g)
		}
	})
	b.Run("one_pass", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			stats := collectELKGraphStats(g)
			benchmarkIntSink = len(stats.degrees) + len(stats.selfLoopLabelMax)
		}
	})
}

func BenchmarkDescendantShiftNested(b *testing.B) {
	input := nestedBenchmarkInput(4, 4)
	b.Run("graph_helper", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			g := compileBenchmarkGraph(b, input)
			setSyntheticRoutes(g)
			b.StartTimer()
			applyNestedShifts(g, func(obj *d2graph.Object, dx, dy float64) {
				obj.ShiftDescendants(dx, dy)
			})
		}
	})
	b.Run("incident_index", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			g := compileBenchmarkGraph(b, input)
			setSyntheticRoutes(g)
			b.StartTimer()
			shifter := newDescendantShifter(g)
			applyNestedShifts(g, shifter.shift)
		}
	})
}

func denseBenchmarkInput(nodes, edges int) string {
	var input strings.Builder
	for i := 0; i < nodes; i++ {
		fmt.Fprintf(&input, "n%d: Node %d\n", i, i)
	}
	for i := 0; i < edges; i++ {
		src := i % nodes
		dst := (src + 1 + i/nodes) % nodes
		fmt.Fprintf(&input, "n%d -> n%d: edge %d\n", src, dst, i)
	}
	return input.String()
}

func nestedBenchmarkInput(levels, fanout int) string {
	var input strings.Builder
	var writeLevel func(prefix, indent string, level int)
	writeLevel = func(prefix, indent string, level int) {
		for i := 0; i < fanout; i++ {
			id := fmt.Sprintf("%sn%d", prefix, i)
			if level+1 < levels {
				fmt.Fprintf(&input, "%s%s: Container %s {\n", indent, id, id)
				writeLevel(id+"_", indent+"  ", level+1)
				fmt.Fprintf(&input, "%s}\n", indent)
			} else {
				fmt.Fprintf(&input, "%s%s: Leaf %s\n", indent, id, id)
			}
		}
		for i := 1; i < fanout; i++ {
			fmt.Fprintf(&input, "%s%sn%d -> %sn%d\n", indent, prefix, i-1, prefix, i)
		}
	}
	writeLevel("", "", 0)
	return input.String()
}

func applyNestedShifts(g *d2graph.Graph, shift func(*d2graph.Object, float64, float64)) {
	for _, obj := range g.Objects {
		if len(obj.ChildrenArray) == 0 {
			continue
		}
		shift(obj, 18, 0)
		shift(obj, -7, 0)
		shift(obj, 0, 13)
		shift(obj, 0, -5)
	}
}

func repeatedScanStats(g *d2graph.Graph) int {
	total := 0
	for _, obj := range g.Objects {
		incoming, outgoing := 0, 0
		for _, edge := range g.Edges {
			if edge.Src == obj {
				outgoing++
			}
			if edge.Dst == obj {
				incoming++
			}
		}
		total += incoming + outgoing
		if len(obj.ChildrenArray) == 0 {
			continue
		}
		for _, child := range obj.Children {
			for _, edge := range g.Edges {
				if edge.Src == edge.Dst && edge.Src == child && edge.Label.Value != "" {
					total += edge.LabelDimensions.Width + edge.LabelDimensions.Height
				}
			}
		}
	}
	return total
}

var benchmarkIntSink int

func compileBenchmarkGraph(tb testing.TB, input string) *d2graph.Graph {
	tb.Helper()
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	if err != nil {
		tb.Fatal(err)
	}
	for i, obj := range g.Objects {
		column := i % 25
		row := i / 25
		obj.Box = geo.NewBox(geo.NewPoint(float64(column*160), float64(row*120)), 80, 60)
		obj.Shape.Value = d2target.ShapeRectangle
	}
	return g
}

func setSyntheticRoutes(g *d2graph.Graph) {
	for i, edge := range g.Edges {
		src := edge.Src.Box
		dst := edge.Dst.Box
		startX := src.TopLeft.X + src.Width
		startY := src.TopLeft.Y + src.Height/2
		lane := float64(i%7 + 1)
		edge.Route = []*geo.Point{
			geo.NewPoint(startX, startY),
			geo.NewPoint(startX+20+lane, startY),
			geo.NewPoint(startX+20+lane, startY+15),
			geo.NewPoint(dst.TopLeft.X-20-lane, startY+15),
			geo.NewPoint(dst.TopLeft.X-20-lane, dst.TopLeft.Y+dst.Height/2),
			geo.NewPoint(dst.TopLeft.X, dst.TopLeft.Y+dst.Height/2),
		}
	}
}
