package routing

import (
	"fmt"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func BenchmarkOVGSparsePortOwners(b *testing.B) {
	for _, count := range []int{16, 128, 1024} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			graph := layoutgraph.NewGraph()
			ovg := NewOVG(nil)
			for i := range count {
				n := graph.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i+1), 40, 40))
				n.TopLeft = geo.NewPoint(float64(i*100), float64(i*100))
				ovg.Ports[n] = []*OVGNode{NewOVGNode(geo.NewPoint(n.TopLeft.X+40, n.TopLeft.Y+20))}
			}
			guard, err := newOVGBuildGuard(b.Context(), defaultOVGBuildLimits())
			if err != nil {
				b.Fatal(err)
			}
			index, err := newOVGPortIndex(ovg.Ports, graph, nil, guard)
			if err != nil {
				b.Fatal(err)
			}
			point := NewOVGNode(geo.NewPoint(120, 20))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				guard.work = 0
				visible, err := point.hasUnobstructedLineToPorts(ovg, index, 2, guard)
				if err != nil || visible {
					b.Fatalf("visible=%v err=%v", visible, err)
				}
			}
			b.ReportMetric(float64(guard.work), "work/op")
		})
	}
}
