package d2svg_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
)

func TestRenderValidatesPaddingDimensions(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	fontFamily := d2fonts.SourceSansPro
	monoFontFamily := d2fonts.SourceCodePro
	diagram.FontFamily = &fontFamily
	diagram.MonoFontFamily = &monoFontFamily
	diagram.Root.StrokeWidth = 2
	diagram.Shapes = []d2target.Shape{{
		ID:          "a",
		Type:        d2target.ShapeRectangle,
		Pos:         d2target.Point{X: 10, Y: 20},
		Width:       100,
		Height:      80,
		Fill:        "#ffffff",
		Stroke:      "#000000",
		StrokeWidth: 2,
	}}

	tl, br := diagram.BoundingBox()
	shortestSide := min(br.X-tl.X, br.Y-tl.Y)
	zeroDimensionPad := int64(-shortestSide / 2)
	maxInt := int64(^uint(0) >> 1)
	postValidationOverflowPad := (maxInt - int64(br.X-tl.X)) / 2
	doubleBorderDiagram := *diagram
	doubleBorderDiagram.Root.StrokeWidth = 0
	doubleBorderDiagram.Root.DoubleBorder = true

	for _, tc := range []struct {
		name     string
		pad      int64
		wantZero bool
	}{
		{name: "negative crop", pad: -1},
		{name: "zero dimension", pad: zeroDimensionPad, wantZero: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pad := tc.pad
			out, err := d2svg.Render(diagram, &d2svg.RenderOpts{Pad: &pad})
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if strings.Contains(string(out), `width="-`) || strings.Contains(string(out), `height="-`) {
				t.Fatalf("Render() emitted a negative SVG dimension:\n%s", out)
			}
			if tc.wantZero && !strings.Contains(string(out), `height="0"`) && !strings.Contains(string(out), `width="0"`) {
				t.Fatalf("Render() did not preserve a zero SVG dimension:\n%s", out)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		pad    int64
		target *d2target.Diagram
	}{
		{name: "negative dimension", pad: zeroDimensionPad - 1, target: diagram},
		{name: "minimum integer", pad: math.MinInt64, target: diagram},
		{name: "maximum integer", pad: math.MaxInt64, target: diagram},
		{name: "root stroke overflow", pad: postValidationOverflowPad, target: diagram},
		{name: "double border overflow", pad: postValidationOverflowPad, target: &doubleBorderDiagram},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pad := tc.pad
			out, err := d2svg.Render(tc.target, &d2svg.RenderOpts{Pad: &pad})
			if err == nil {
				t.Fatalf("Render() succeeded with padding %d:\n%s", pad, out)
			}
			want := fmt.Sprintf("padding %d produces invalid SVG dimensions", pad)
			if err.Error() != want {
				t.Fatalf("Render() error = %q, want %q", err, want)
			}
			if out != nil {
				t.Fatalf("Render() returned output on error: %q", out)
			}
		})
	}
}
