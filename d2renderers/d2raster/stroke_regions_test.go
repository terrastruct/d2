package d2raster

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestStrokeRegionsMatchCompleteFrame(t *testing.T) {
	state := uint32(0x2b317e99)
	next := func() float64 { state = 1664525*state + 1013904223; return float64(state>>8) / float64(1<<24) }
	for iteration := 0; iteration < 36; iteration++ {
		group := d2scene.NewNode(nil)
		group.Opacity = .73
		for part := 0; part < 3; part++ {
			commands := []d2scene.PathCommand{d2scene.MoveTo(next()*230-15, next()*980-40)}
			for edge := 0; edge < 4; edge++ {
				commands = append(commands, d2scene.LineTo(next()*230-15, next()*980-40))
			}
			if iteration%2 == 0 {
				commands = append(commands, d2scene.ClosePath())
			}
			stroke := &d2scene.Stroke{Paint: d2scene.SolidPaint{Color: color.NRGBA{R: byte(iteration * 7), G: 31, B: byte(part * 83), A: 177}}, Width: 1 + next()*11, Cap: d2scene.LineCap(iteration % 3), Join: d2scene.LineJoin((iteration / 3) % 3), MiterLimit: 1 + next()*8}
			if iteration%4 == 0 {
				stroke.Dashes = []float64{11.37, 5.13, 3.71}
				stroke.DashOffset = 7.19
			}
			node := d2scene.NewNode(d2scene.Path{Commands: commands, Stroke: stroke})
			node.Transform = d2scene.Matrix{A: .93, B: .071, C: -.031, D: 1.013, E: 3.127, F: -2.571}
			group.Children = append(group.Children, node)
		}
		document := d2scene.NewDocument(d2scene.Box{Width: 210, Height: 940}, group)
		options := testOptions()
		if iteration%2 == 0 {
			options.Background = color.White
		}
		want, err := Render(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		for _, height := range []int{7, 37, 256} {
			got := image.NewNRGBA(want.Bounds())
			if err := RenderBands(context.Background(), document, options, height, func(b *image.NRGBA) error { draw.Draw(got, b.Bounds(), b, b.Bounds().Min, draw.Src); return nil }); err != nil {
				t.Fatalf("iteration%d height%d: %v", iteration, height, err)
			}
			assertBandPixels(t, got, want)
		}
	}
}

func TestStrokeRegionsBoundLayerStorageAndPreflight(t *testing.T) {
	root := d2scene.NewNode(d2scene.Path{Commands: []d2scene.PathCommand{d2scene.MoveTo(20.13, 20.71), d2scene.LineTo(1800.13, 20.71), d2scene.LineTo(1800.13, 900.71), d2scene.LineTo(20.13, 900.71)}, Stroke: &d2scene.Stroke{Paint: red, Width: 3.13, Cap: d2scene.CapRound, Join: d2scene.JoinRound, MiterLimit: 4}})
	root.Opacity = .73
	document := d2scene.NewDocument(d2scene.Box{Width: 1900, Height: 940}, root)
	options := testOptions()
	options.MaxWidth = 1900
	options.MaxPixels = 2_000_000
	prepared, err := prepareWithSessionBands(context.Background(), document, options, nil, 64)
	if err != nil {
		t.Fatal(err)
	}
	bounds, err := effectRenderBounds(context.Background(), prepared.root, image.Rect(0, 400, 1900, 464))
	if err != nil {
		t.Fatal(err)
	}
	if bounds.Dx() > 10 || bounds.Dy() != 64 {
		t.Fatalf("middle of vertical stroke bounds=%v", bounds)
	}
	full, err := prepareWithSession(context.Background(), document, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.resources.peakOffscreenBytes*8 >= full.resources.peakOffscreenBytes {
		t.Fatalf("band scratch%d not substantially below full%d", prepared.resources.peakOffscreenBytes, full.resources.peakOffscreenBytes)
	}
	options.MaxOffscreenBytes = prepared.resources.peakOffscreenBytes
	calls := 0
	if err := RenderBands(context.Background(), document, options, 64, func(*image.NRGBA) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("no bands rendered")
	}
	options.MaxOffscreenBytes--
	calls = 0
	if err := RenderBands(context.Background(), document, options, 64, func(*image.NRGBA) error { calls++; return nil }); err == nil || !strings.Contains(err.Error(), "offscreen") {
		t.Fatalf("one-byte-short scratch error=%v", err)
	}
	if calls != 0 {
		t.Fatal("resource rejection produced a partial frame")
	}
}
