package d2isometricimg

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/scenepaint"
	"github.com/d2lang/d2/d2target"
)

// Exercise the compiled gradient fixture rather than only its paint parser.
// Give each role the only chromatic paint in the scene, so another correctly
// rendered gradient cannot conceal a fallback in that role.
func TestNativeAuthoredGradientPaintRoles(t *testing.T) {
	const gradient = "linear-gradient(to right, #ff0000, #0000ff)"
	cases := []struct {
		name string
		set  func(*d2target.Diagram)
	}{
		{"background", func(d *d2target.Diagram) { d.Root.Fill = gradient }},
		{"shape fill", func(d *d2target.Diagram) { d.Shapes[0].Fill = gradient }},
		{"cylinder fill", func(d *d2target.Diagram) { d.Shapes[0].Type = d2target.ShapeCylinder; d.Shapes[0].Fill = gradient }},
		{"markdown font", func(d *d2target.Diagram) {
			d.Shapes[0].Type = d2target.ShapeText
			d.Shapes[0].Language = "markdown"
			d.Shapes[0].Label = "**gradient**"
			d.Shapes[0].Color = gradient
		}},
		{"markdown label fill", func(d *d2target.Diagram) {
			d.Shapes[0].Type = d2target.ShapeText
			d.Shapes[0].Language = "markdown"
			d.Shapes[0].Label = "**gradient**"
			d.Shapes[0].LabelFill = gradient
		}},
		{"shape rim", func(d *d2target.Diagram) { d.Shapes[0].Stroke = gradient; d.Shapes[0].StrokeWidth = 5 }},
		{"shape font", func(d *d2target.Diagram) { d.Shapes[0].Color = gradient }},
		{"shape label fill", func(d *d2target.Diagram) { d.Shapes[0].LabelFill = gradient }},
		{"connection stroke", func(d *d2target.Diagram) { d.Connections[0].Stroke = gradient; d.Connections[0].StrokeWidth = 5 }},
		{"connection font", func(d *d2target.Diagram) { d.Connections[0].Color = gradient }},
		{"connection caption fill", func(d *d2target.Diagram) { d.Connections[0].Fill = gradient }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := sourcePanelFixture(t, "txtar/gradient/dagre/board.exp.json")
			d.Root.Fill = "white"
			for i := range d.Shapes {
				s := &d.Shapes[i]
				s.Fill = "#eeeeee"
				s.Stroke = "#333333"
				s.Color = "#333333"
				s.LabelFill = "transparent"
			}
			for i := range d.Connections {
				c := &d.Connections[i]
				c.Fill = "transparent"
				c.Stroke = "#333333"
				c.Color = "#333333"
				c.LabelFill = "transparent"
			}
			tc.set(d)
			svg, err := Render(context.Background(), d, &Options{Format: SVG, Width: 720, Height: 500})
			if err != nil {
				t.Fatal(err)
			}
			for _, stop := range []string{`stop-color="#ff0000"`, `stop-color="#0000ff"`} {
				if !strings.Contains(string(svg), stop) {
					t.Fatalf("SVG lost %s paint (%s)", tc.name, stop)
				}
			}
			if bytes.Contains(svg, []byte("data:image")) {
				t.Fatal("authored gradient was rasterized inside SVG")
			}
			pngBytes, err := Render(context.Background(), d, &Options{Format: PNG, Width: 720, Height: 500})
			if err != nil {
				t.Fatal(err)
			}
			frame, err := png.Decode(bytes.NewReader(pngBytes))
			if err != nil {
				t.Fatal(err)
			}
			red, blue := gradientChromaticPixels(frame)
			if red < 4 || blue < 4 {
				t.Fatalf("PNG lost %s gradient: red=%d blue=%d", tc.name, red, blue)
			}
		})
	}
}

func gradientChromaticPixels(frame image.Image) (red, blue int) {
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			c := color.NRGBAModel.Convert(frame.At(x, y)).(color.NRGBA)
			if int(c.R) > int(c.B)+40 && int(c.R) > int(c.G)+40 {
				red++
			}
			if int(c.B) > int(c.R)+40 && int(c.B) > int(c.G)+40 {
				blue++
			}
		}
	}
	return
}

func TestNativeGradientBackgroundAnimationAndAlpha(t *testing.T) {
	d := captureDiagram()
	d.Root.Fill = "linear-gradient(to right, rgba(255,0,0,0.5), rgba(0,0,255,0.5))"
	d.Shapes[0].Animated = true
	scene, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	native, err := newNativeScene(context.Background(), scene, 400, 300)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []float64{0, .5} {
		frame, err := native.Frame(context.Background(), at, true)
		if err != nil {
			t.Fatal(err)
		}
		left := frame.RGBAAt(0, 0)
		right := frame.RGBAAt(frame.Bounds().Max.X-1, 0)
		if left.R < 240 || left.B > 140 || right.B < 240 || right.R > 140 || left.A != 255 || right.A != 255 {
			t.Fatalf("frame %v lost composited gradient: left=%v right=%v", at, left, right)
		}
	}
}

func TestNativeContainerGradientRetainsStopsAndOpacity(t *testing.T) {
	d := sourcePanelFixture(t, "regression/cylinder_grid_label/elk/board.exp.json")
	const gradient = "linear-gradient(to right, #ff0000, #0000ff)"
	for i := range d.Shapes {
		if d.Shapes[i].ID == "container title is hidden" {
			d.Shapes[i].Fill = gradient
		}
	}
	data, err := Render(context.Background(), d, &Options{Format: SVG, Width: 600, Height: 400})
	if err != nil {
		t.Fatal(err)
	}
	for _, stop := range []string{`stop-color="#ff0000"`, `stop-color="#0000ff"`} {
		if !bytes.Contains(data, []byte(stop)) {
			t.Fatalf("container lost authored stop %s", stop)
		}
	}
	raw, err := nativeGradientOpacity("linear-gradient(to right, #ff000080 25%, #0000ffff 100%)", .22)
	if err != nil {
		t.Fatal(err)
	}
	before, err := scenepaint.Gradient("linear-gradient(to right, #ff000080 25%, #0000ffff 100%)")
	if err != nil {
		t.Fatal(err)
	}
	after, err := scenepaint.Gradient(raw)
	if err != nil {
		t.Fatal(err)
	}
	a, b := before.(d2scene.LinearGradient), after.(d2scene.LinearGradient)
	if a.Start != b.Start || a.End != b.End || a.Stops[0].Offset != b.Stops[0].Offset || b.Stops[0].Color.A != 28 || b.Stops[1].Color.A != 56 {
		t.Fatalf("container wash changed gradient: %+v", b)
	}
}
