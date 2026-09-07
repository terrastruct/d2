package d2grid

import (
	"fmt"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/lib/geo"
)

func benchmarkGrid(n, rows int) *gridDiagram {
	gd := &gridDiagram{rows: rows, horizontalGap: 40, verticalGap: 40}
	for i := 0; i < n; i++ {
		gd.objects = append(gd.objects, &d2graph.Object{ID: fmt.Sprintf("n%d", i), Box: geo.NewBox(geo.NewPoint(0, 0), float64(50+(i*37)%200), float64(40+(i*53)%100))})
	}
	return gd
}

func BenchmarkDynamicGridSearch(b *testing.B) {
	for _, tc := range []struct{ n, rows int }{{20, 4}, {50, 8}, {100, 10}, {400, 20}} {
		b.Run(fmt.Sprintf("%d_objects_%d_rows", tc.n, tc.rows), func(b *testing.B) {
			gd := benchmarkGrid(tc.n, tc.rows)
			target := float64(gd.horizontalGap * (tc.n - tc.rows))
			for _, o := range gd.objects {
				target += o.Width
			}
			target /= float64(tc.rows)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				gd.getBestLayout(target, false)
			}
		})
	}
}
