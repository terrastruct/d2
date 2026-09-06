package d2scenebuild

import (
	"context"
	"fmt"
	"testing"

	"github.com/d2lang/d2/d2target"
)

func BenchmarkBuildRepeatedTextLabels(b *testing.B) {
	for _, count := range []int{1, 100} {
		b.Run(fmt.Sprintf("%dLabels", count), func(b *testing.B) {
			diagram := d2target.NewDiagram()
			for index := range count {
				diagram.Shapes = append(diagram.Shapes, d2target.Shape{
					ID: fmt.Sprintf("label-%d", index), Type: d2target.ShapeRectangle,
					Pos: d2target.Point{X: index % 10 * 180, Y: index / 10 * 80}, Width: 160, Height: 60,
					Fill: "#ffffff", Stroke: "#000000", StrokeWidth: 2, Opacity: 1,
					Text: d2target.Text{
						Label: "repeat repeated ASCII label", FontFamily: "default", FontSize: 16,
						LabelWidth: 140, LabelHeight: 24,
					},
				})
			}
			b.ReportAllocs()
			for b.Loop() {
				document, err := Build(context.Background(), diagram, Options{})
				if err != nil {
					b.Fatal(err)
				}
				benchmarkDocument = document
			}
		})
	}
}
