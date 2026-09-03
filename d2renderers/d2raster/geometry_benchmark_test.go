package d2raster

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

var benchmarkGeometryDestination any

func BenchmarkAddStrokeRun(b *testing.B) {
	points := make([]d2scene.Point, 1_025)
	for index := range points {
		points[index] = d2scene.Point{X: 32 + float64(index%64)*14.5, Y: 32 + float64(index/64)*55 + float64(index%2)*21.25}
	}
	run := strokeRun{points: points}
	transform := d2scene.Matrix{A: 0.91, B: 0.13, C: -0.07, D: 0.88, E: 17.25, F: 9.75}
	for _, test := range []struct {
		name string
		join d2scene.LineJoin
	}{{name: "Round", join: d2scene.JoinRound}, {name: "Miter", join: d2scene.JoinMiter}} {
		b.Run(test.name, func(b *testing.B) {
			stroke := &preparedStroke{width: 5.5, cap: d2scene.CapRound, join: test.join, miterLimit: 4}
			rasterizer := scanline.NewRasterizer(1_024, 1_024)
			rasterizer.ReserveEdges(100_000)
			if err := addStrokeRun(context.Background(), rasterizer, run, transform, stroke); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				rasterizer.Reset(1_024, 1_024)
				if err := addStrokeRun(context.Background(), rasterizer, run, transform, stroke); err != nil {
					b.Fatal(err)
				}
			}
			benchmarkGeometryDestination = rasterizer
		})
	}
}

func BenchmarkMakeStrokeRuns(b *testing.B) {
	manySegments := make([]d2scene.Point, 1_025)
	for index := range manySegments {
		manySegments[index] = d2scene.Point{X: float64(index) * 3.25, Y: float64(index%7) * 1.75}
	}
	for _, test := range []struct {
		name   string
		path   subpath
		dashes []float64
	}{
		{name: "TwoPointRuns", path: subpath{points: []d2scene.Point{{}, {X: 8_192}}}, dashes: []float64{5, 5}},
		{name: "CrossOneVertex", path: subpath{points: manySegments}, dashes: []float64{12, 7, 3, 5}},
		{name: "CrossManyVertices", path: subpath{points: manySegments}, dashes: []float64{100, 20}},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				runs, err := makeStrokeRuns(context.Background(), test.path, test.dashes, 2.5, func() error { return nil })
				if err != nil {
					b.Fatal(err)
				}
				benchmarkGeometryDestination = runs
			}
		})
	}
}
