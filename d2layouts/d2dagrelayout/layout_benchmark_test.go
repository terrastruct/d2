package d2dagrelayout

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/textmeasure"
)

var benchmarkGraphSink *d2graph.Graph

func benchmarkNestedSource(levels, fanout int) string {
	var out strings.Builder
	var writeLevel func(prefix, indent string, level int)
	writeLevel = func(prefix, indent string, level int) {
		for i := 0; i < fanout; i++ {
			id := fmt.Sprintf("%sn%d", prefix, i)
			if level+1 < levels {
				fmt.Fprintf(&out, "%s%s: Container %s {\n", indent, id, id)
				writeLevel(id+"_", indent+"  ", level+1)
				fmt.Fprintf(&out, "%s}\n", indent)
			} else {
				fmt.Fprintf(&out, "%s%s: Leaf %s\n", indent, id, id)
			}
		}
		for i := 1; i < fanout; i++ {
			fmt.Fprintf(&out, "%s%sn%d -> %sn%d\n", indent, prefix, i-1, prefix, i)
		}
	}
	writeLevel("", "", 0)
	return out.String()
}

func benchmarkParallelSource(edges int) string {
	var out strings.Builder
	out.WriteString("a\nb\n")
	for i := 0; i < edges; i++ {
		fmt.Fprintf(&out, "a -> b: edge %d\n", i)
	}
	return out.String()
}

func benchmarkCrossRankSource(nodes int) string {
	var out strings.Builder
	for i := 0; i < nodes; i++ {
		fmt.Fprintf(&out, "n%d: Node %d { label.near: outside-right-center }\n", i, i)
		if i > 0 {
			fmt.Fprintf(&out, "n%d -> n%d\n", i-1, i)
		}
	}
	return out.String()
}

func prepareBenchmarkGraph(b *testing.B, input string, ruler *textmeasure.Ruler) *d2graph.Graph {
	b.Helper()
	g, _, err := d2compiler.Compile("index.d2", strings.NewReader(input), nil)
	if err != nil {
		b.Fatal(err)
	}
	if err := g.ApplyTheme(d2themescatalog.NeutralDefault.ID); err != nil {
		b.Fatal(err)
	}
	if err := g.SetDimensions(nil, ruler, nil, nil); err != nil {
		b.Fatal(err)
	}
	return g
}

func benchmarkDagreLayout(b *testing.B, input string) {
	b.Helper()
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		g := prepareBenchmarkGraph(b, input, ruler)
		b.StartTimer()
		if err := DefaultLayout(context.Background(), g); err != nil {
			b.Fatal(err)
		}
		benchmarkGraphSink = g
	}
}

func BenchmarkDagreAdapterPathologies(b *testing.B) {
	b.Run("nested-4x4", func(b *testing.B) {
		benchmarkDagreLayout(b, benchmarkNestedSource(4, 4))
	})
	b.Run("parallel-edges-512", func(b *testing.B) {
		benchmarkDagreLayout(b, benchmarkParallelSource(512))
	})
	b.Run("cross-rank-100", func(b *testing.B) {
		benchmarkDagreLayout(b, benchmarkCrossRankSource(100))
	})
}

func BenchmarkAdjustCrossRankSpacing(b *testing.B) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		b.Fatal(err)
	}
	for _, nodes := range []int{64, 128, 512} {
		b.Run(fmt.Sprintf("chain-%d", nodes), func(b *testing.B) {
			input := benchmarkCrossRankSource(nodes)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				g := prepareBenchmarkGraph(b, input, ruler)
				for index, obj := range g.Objects {
					obj.TopLeft = geo.NewPoint(float64(index*120), 0)
				}
				for _, edge := range g.Edges {
					edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
				}
				b.StartTimer()
				adjustCrossRankSpacing(g, MIN_RANK_SEP, true)
				benchmarkGraphSink = g
			}
		})
	}
}
