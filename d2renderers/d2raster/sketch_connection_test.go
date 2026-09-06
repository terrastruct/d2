package d2raster_test

import (
	"bytes"
	"context"
	"image/color"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestRasterSmokeBuildsAndRendersSketchConnectionFrames(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Root.Fill = "#ffffff"
	diagram.Shapes = []d2target.Shape{
		{ID: "a", Type: d2target.ShapeRectangle, Pos: d2target.Point{}, Width: 30, Height: 30, Fill: "#ffffff", Stroke: "#222222", StrokeWidth: 2, Opacity: 1},
		{ID: "b", Type: d2target.ShapeRectangle, Pos: d2target.Point{X: 140}, Width: 30, Height: 30, Fill: "#ffffff", Stroke: "#222222", StrokeWidth: 2, Opacity: 1},
	}
	diagram.Connections = []d2target.Connection{{
		ID: "a-b", Src: "a", Dst: "b",
		Route:   []*geo.Point{{X: 30, Y: 15}, {X: 70, Y: -15}, {X: 100, Y: 45}, {X: 140, Y: 15}},
		IsCurve: true, Animated: true,
		SrcArrow: d2target.CircleArrowhead, DstArrow: d2target.FilledDiamondArrowhead,
		Stroke: "#222222", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{
			Label: "sketch", FontSize: 14, FontFamily: "default", LabelWidth: 42, LabelHeight: 17,
		},
		Fill: "#ffffff", LabelPosition: "INSIDE_MIDDLE_CENTER",
	}}
	pad := int64(20)
	document, err := d2scenebuild.Build(context.Background(), diagram, d2scenebuild.Options{
		Pad: &pad, Sketch: true,
		SketchBudget: d2scenebuild.SketchBudget{
			MaxOperationSets: 1_000, MaxOperations: 100_000, MaxPathCommands: 100_000,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	connection := findSketchNode(document.Root, "a-b")
	if connection == nil || connection.Children[0].ID != "a-b:geometry" || connection.Children[0].Mask == nil {
		t.Fatalf("sketch connection lost its masked geometry group: %+v", connection)
	}
	if arrow := findSketchNode(connection, "a-b:dst-arrowhead"); arrow == nil {
		t.Fatal("sketch destination arrowhead is missing")
	} else if _, ok := arrow.Primitive.(d2scene.Path); !ok {
		t.Fatalf("sketch arrowhead primitive = %T, want typed Path", arrow.Primitive)
	}

	options := d2raster.FrameOptions{
		Scale: 1, Background: color.White,
		MaxWidth: 1_000, MaxHeight: 1_000, MaxPixels: 1_000_000,
		MaxNodes: 10_000, MaxDepth: 100, MaxPathCommands: 100_000,
		MaxAnimationTracks: 100, MaxAnimationKeyframes: 1_000,
		MaxAssets: 100, MaxAssetBytes: 64 << 20, MaxDecodedAssetBytes: 64 << 20, MaxImportDepth: 32,
		MaxOffscreenBytes: 64 << 20, MaxEvenOddClipWork: 1_000_000_000,
	}
	first, err := d2raster.Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	options.Time = 500 * time.Millisecond
	second, err := d2raster.Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if first.Bounds() != second.Bounds() || first.Bounds().Empty() {
		t.Fatalf("sketch frame bounds = %v/%v", first.Bounds(), second.Bounds())
	}
	if bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("animated sketch connection frames are identical at t=0 and t=500ms")
	}
	nonWhite := 0
	for offset := 0; offset+3 < len(first.Pix); offset += 4 {
		if first.Pix[offset] < 245 || first.Pix[offset+1] < 245 || first.Pix[offset+2] < 245 {
			nonWhite++
		}
	}
	if nonWhite < 100 {
		t.Fatalf("sketch raster has only %d inked pixels", nonWhite)
	}
}

func findSketchNode(root *d2scene.Node, id string) *d2scene.Node {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return root
	}
	for _, child := range root.Children {
		if match := findSketchNode(child, id); match != nil {
			return match
		}
	}
	return nil
}
