package routing

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func ovgScalingGraph(count int) *layoutgraph.Graph {
	graph := layoutgraph.NewGraph()
	columns := int(math.Ceil(math.Sqrt(float64(count))))
	for i := range count {
		column, row := i%columns, i/columns
		node := graph.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i+1), float64(60+(i%3)*20), float64(40+(i%4)*10)))
		// Varied row offsets, widths and heights force partial alignments and actual
		// obstacle checks while retaining enough shared axes for a valid OVG.
		node.TopLeft = geo.NewPoint(float64(column*230+(row%3)*23), float64(row*180+(column%4)*17))
		if i > 0 {
			graph.Connect(graph.Nodes[i-1], node)
		}
		if i >= columns {
			graph.Connect(graph.Nodes[i-columns], node)
		}
	}
	graph.ComputeCellSize()
	return graph
}

func TestOVGConstructionScalingFingerprint(t *testing.T) {
	for _, count := range []int{32, 128, 256} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ovg, err := buildOVGFromGraphWithLimits(t.Context(), ovgScalingGraph(count), nil, defaultOVGBuildLimits())
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(ovg)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("nodes=%d edges=%d sha256=%x", len(ovg.Nodes), len(ovg.Edges), sha256.Sum256(data))
		})
	}
}

func BenchmarkOVGConstructionScaling(b *testing.B) {
	for _, count := range []int{32, 128, 256} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			graph := ovgScalingGraph(count)
			b.ReportAllocs()
			var ovg *OVG
			var err error
			var guard *ovgBuildGuard
			b.ResetTimer()
			for b.Loop() {
				guard, err = newOVGBuildGuard(b.Context(), defaultOVGBuildLimits())
				if err != nil {
					b.Fatal(err)
				}
				ovg, err = buildOVGFromGraphWithGuard(graph, nil, guard)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(ovg.Nodes)), "nodes/op")
			b.ReportMetric(float64(len(ovg.Edges)), "edges/op")
			b.ReportMetric(float64(guard.work), "work/op")
		})
	}
}
