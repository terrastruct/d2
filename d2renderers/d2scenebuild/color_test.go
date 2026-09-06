package d2scenebuild

import (
	"context"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestBuildCarriesTypedGradientsForRootShapesAndConnections(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.Fill = "radial-gradient(white, black)"
	diagram.Root.Stroke = "none"
	diagram.Shapes = []d2target.Shape{
		{ID: "a", Type: d2target.ShapeRectangle, Width: 10, Height: 10, Fill: "linear-gradient(to right, red, blue)", Stroke: "none", Opacity: 1},
		{ID: "b", Type: d2target.ShapeRectangle, Pos: d2target.Point{X: 20}, Width: 10, Height: 10, Fill: "#fff", Stroke: "none", Opacity: 1},
	}
	diagram.Connections = []d2target.Connection{{
		ID: "a -> b", Src: "a", Dst: "b", Route: []*geo.Point{{X: 10, Y: 5}, {X: 20, Y: 5}},
		SrcArrow: d2target.NoArrowhead, DstArrow: d2target.NoArrowhead,
		Stroke: "linear-gradient(red, blue)", StrokeWidth: 2, Opacity: 1,
	}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	root := document.Root.Children[0].Primitive.(d2scene.Rect)
	if _, ok := root.Fill.(d2scene.RadialGradient); !ok {
		t.Fatalf("root fill = %T, want RadialGradient", root.Fill)
	}
	shape := document.Root.Children[1].Children[0].Primitive.(d2scene.Rect)
	if _, ok := shape.Fill.(d2scene.LinearGradient); !ok {
		t.Fatalf("shape fill = %T, want LinearGradient", shape.Fill)
	}
	connection := document.Root.Children[3].Children[0].Primitive.(d2scene.Path)
	if connection.Stroke == nil {
		t.Fatal("connection has no stroke")
	}
	if _, ok := connection.Stroke.Paint.(d2scene.LinearGradient); !ok {
		t.Fatalf("connection stroke = %T, want LinearGradient", connection.Stroke.Paint)
	}
}

func TestGradientPaintGeometry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		start, end d2scene.Point
	}{
		{name: "default", raw: "linear-gradient(red, blue)", start: d2scene.Point{}, end: d2scene.Point{Y: 1}},
		{name: "right", raw: "linear-gradient(to right, red, blue)", start: d2scene.Point{Y: .5}, end: d2scene.Point{X: 1, Y: .5}},
		{name: "top right", raw: "linear-gradient(to top right, red, blue)", start: d2scene.Point{Y: 1}, end: d2scene.Point{X: 1}},
		{name: "angle", raw: "linear-gradient(45deg, red, blue)", start: d2scene.Point{X: .5, Y: .5}, end: d2scene.Point{X: .5 + math.Sqrt(.5)/2, Y: .5 + math.Sqrt(.5)/2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paint, err := gradientPaint(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			gradient, ok := paint.(d2scene.LinearGradient)
			if !ok {
				t.Fatalf("paint = %T, want LinearGradient", paint)
			}
			if !closePoint(gradient.Start, test.start) || !closePoint(gradient.End, test.end) {
				t.Fatalf("vector = %+v -> %+v, want %+v -> %+v", gradient.Start, gradient.End, test.start, test.end)
			}
			if gradient.Units != d2scene.ObjectBoundingBox || gradient.Transform != d2scene.Identity() || gradient.Spread != d2scene.SpreadPad {
				t.Fatalf("unexpected gradient policy: %+v", gradient)
			}
		})
	}
}

func TestGradientPaintStopsAndRadialDefaults(t *testing.T) {
	t.Parallel()

	paint, err := gradientPaint("radial-gradient(circle, rgba(255,0,0,0.5) -10%, green 25%, blue 20%, white 120%)")
	if err != nil {
		t.Fatal(err)
	}
	gradient, ok := paint.(d2scene.RadialGradient)
	if !ok {
		t.Fatalf("paint = %T, want RadialGradient", paint)
	}
	if gradient.Center != (d2scene.Point{X: .5, Y: .5}) || gradient.Focal != gradient.Center || gradient.Radius != .5 || gradient.FocalRadius != 0 {
		t.Fatalf("radial geometry = %+v", gradient)
	}
	wantOffsets := []float64{0, .25, .25, 1}
	for i, want := range wantOffsets {
		if gradient.Stops[i].Offset != want {
			t.Errorf("stop %d offset = %v, want %v", i, gradient.Stops[i].Offset, want)
		}
	}
	if gradient.Stops[0].Color != (color.NRGBA{R: 255, A: 128}) {
		t.Fatalf("first stop = %+v, want half-alpha red", gradient.Stops[0].Color)
	}

	one, err := gradientPaint("linear-gradient(red)")
	if err != nil {
		t.Fatal(err)
	}
	if got := one.(d2scene.LinearGradient).Stops[0].Offset; got != 0 {
		t.Fatalf("one-stop offset = %v, want 0", got)
	}
}

func TestGradientPaintRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"linear-gradient(to sideways, red, blue)",
		"linear-gradient(red calc(1%), blue)",
		"linear-gradient(not-a-color, blue)",
	} {
		if _, err := gradientPaint(raw); err == nil {
			t.Errorf("gradientPaint(%q) unexpectedly succeeded", raw)
		}
	}

	if _, err := gradientPaint("linear-gradient(red 20px, blue)"); err == nil || !strings.Contains(err.Error(), "position") {
		t.Fatalf("length stop error = %v, want position context", err)
	}
}

func closePoint(left, right d2scene.Point) bool {
	return math.Abs(left.X-right.X) < 1e-12 && math.Abs(left.Y-right.Y) < 1e-12
}
