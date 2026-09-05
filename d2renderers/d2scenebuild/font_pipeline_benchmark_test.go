package d2scenebuild

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2target"
)

var benchmarkDocument any

func BenchmarkBuildMarkdownFontPipeline(b *testing.B) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "markdown", Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: 10, Y: 20}, Width: 640, Height: 520,
		Fill: "#ffffff", Stroke: "#000000", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{
			Label:    "# Heading\n\nParagraph with **bold**, *italic*, `code`, and repeated words.\n\n> quote\n\n- item one\n- item two\n\n| A | B |\n| - | - |\n| 1 | 2 |",
			Language: "markdown", FontFamily: "default", FontSize: 16,
			LabelWidth: 600, LabelHeight: 480,
		},
	}}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		document, err := Build(ctx, diagram, Options{})
		if err != nil {
			b.Fatal(err)
		}
		benchmarkDocument = document
	}
}

func BenchmarkBuildCodeFontPipeline(b *testing.B) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "code", Type: d2target.ShapeCode,
		Pos: d2target.Point{X: 10, Y: 20}, Width: 760, Height: 240,
		Fill: "#ffffff", Stroke: "#000000", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{
			Label:    "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello from the renderer\")\n}\n",
			Language: "go", FontFamily: "mono", FontSize: 16,
			LabelWidth: 720, LabelHeight: 200,
		},
	}}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		document, err := Build(ctx, diagram, Options{})
		if err != nil {
			b.Fatal(err)
		}
		benchmarkDocument = document
	}
}
