package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/draw"
	"image/gif"
	"image/png"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func framingDiagram(x int) *d2target.Diagram {
	d := d2target.NewDiagram()
	s := d2target.BaseShape()
	s.ID, s.Type, s.Width, s.Height = "anchor", d2target.ShapeRectangle, 120, 90
	s.Pos = d2target.Point{X: x, Y: 0}
	s.Fill, s.Stroke = "#d32626", "#990c0c"
	d.Shapes = []d2target.Shape{*s}
	return d
}

func TestContentFramingFitsFinalGeometryAndKeepsFixedAPI(t *testing.T) {
	d := framingDiagram(0)
	o := Options{Width: 480, Height: 300, Render: d2isometric.RenderOpts{}}
	before, _ := json.Marshal(d)
	fixed, err := Render(context.Background(), d, &o)
	if err != nil {
		t.Fatal(err)
	}
	f, err := png.DecodeConfig(bytes.NewReader(fixed))
	if err != nil || f.Width != 480 || f.Height != 300 {
		t.Fatalf("fixed canvas: %+v %v", f, err)
	}
	o.FitContent = true
	s, err := openCapture(context.Background(), d, mustNormalize(t, o))
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if s.opts.Width >= 480 && s.opts.Height >= 300 {
		t.Fatal("content framing retained unused canvas")
	}
	if s.opts.Width < 64 || s.opts.Height < 64 || s.opts.Width > 480 || s.opts.Height > 300 {
		t.Fatal("framing exceeded dimensions")
	}
	c := cameraAtResolution(s.scene.raster.camera, s.opts.Width, s.opts.Height)
	for _, triangle := range s.scene.triangles {
		for _, v := range triangle.V {
			p := c.project(v.Position)
			if p.x < 0 || p.y < 0 || p.x > float64(c.width) || p.y > float64(c.height) {
				t.Fatal("framing clipped final geometry")
			}
		}
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("framing changed compiled layout")
	}
}

func mustNormalize(t *testing.T, o Options) Options {
	t.Helper()
	out, err := normalize(&o)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTimelineUsesOneFrameForBoardsWithDifferentExtents(t *testing.T) {
	first, second := framingDiagram(0), framingDiagram(0)
	extra := framingDiagram(750).Shapes[0]
	extra.ID, extra.Fill, extra.Stroke = "new-service", "#245ed4", "#143975"
	second.Shapes = append(second.Shapes, extra)
	boards := []*d2target.Diagram{first, second}
	o := Options{Format: GIF, Width: 400, Height: 250, FitContent: true, Render: d2isometric.RenderOpts{}}
	encoded, err := RenderTimeline(context.Background(), boards, &o, 400*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 2 || g.Delay[0] != 40 || g.Delay[1] != 40 {
		t.Fatal("board order/duration changed")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
	var anchor image.Rectangle
	for i, frame := range g.Image {
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Src)
		red := image.Rectangle{}
		for y := 0; y < canvas.Bounds().Dy(); y++ {
			for x := 0; x < canvas.Bounds().Dx(); x++ {
				p := canvas.RGBAAt(x, y)
				if int(p.R) > int(p.G)*2 && int(p.R) > int(p.B)*2 && p.R > 70 {
					red = red.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if red.Empty() {
			t.Fatal("stationary anchor missing")
		}
		if i == 0 {
			anchor = red
		} else if red != anchor {
			t.Fatalf("stationary component jumped/resized: %v -> %v", anchor, red)
		}
	}
}
