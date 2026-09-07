package d2raster

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func regionMaskDocument() *d2scene.Document {
	mask := d2scene.NewNode(nil)
	mask.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: -20, Y: -20, Width: 240, Height: 940}, Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}}),
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 50.13, Y: 57.71, Width: 39.47, Height: 21.39}, Fill: black}),
	}
	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 10, Y: 10, Width: 180, Height: 880}, Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 211, G: 103, B: 51, A: 173}}})
	node.Mask = &d2scene.Mask{Root: mask, Type: d2scene.MaskLuminance, Transform: d2scene.Identity()}
	return d2scene.NewDocument(d2scene.Box{Width: 200, Height: 900}, node)
}

func TestMaskRenderBoundsAdmission(t *testing.T) {
	for _, test := range []struct {
		name    string
		mutate  func(*d2scene.Document)
		cropped bool
	}{
		{name: "white backdrop", cropped: true},
		{name: "root translation", cropped: true, mutate: func(d *d2scene.Document) { d.Root.Mask.Root.Transform = d2scene.Translate(.125, .375) }},
		{name: "root opacity", mutate: func(d *d2scene.Document) { d.Root.Mask.Root.Opacity = .9 }},
		{name: "root blend", mutate: func(d *d2scene.Document) { d.Root.Mask.Root.Blend = d2scene.BlendMultiply }},
		{name: "root filter", mutate: func(d *d2scene.Document) {
			d.Root.Mask.Root.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaY: 1}}
		}},
		{name: "base opacity", mutate: func(d *d2scene.Document) { d.Root.Mask.Root.Children[0].Opacity = .9 }},
		{name: "base filter", mutate: func(d *d2scene.Document) {
			d.Root.Mask.Root.Children[0].Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaY: 1}}
		}},
		{name: "base stroke", mutate: func(d *d2scene.Document) {
			p := d.Root.Mask.Root.Children[0].Primitive.(d2scene.Rect)
			p.Stroke = &d2scene.Stroke{Paint: black, Width: 1, MiterLimit: 4}
			d.Root.Mask.Root.Children[0].Primitive = p
		}},
		{name: "base dark", mutate: func(d *d2scene.Document) {
			p := d.Root.Mask.Root.Children[0].Primitive.(d2scene.Rect)
			p.Fill = black
			d.Root.Mask.Root.Children[0].Primitive = p
		}},
		{name: "base shear", mutate: func(d *d2scene.Document) { d.Root.Mask.Root.Children[0].Transform = d2scene.Matrix{A: 1, D: 1, C: .13} }},
		{name: "base degenerate corners", mutate: func(d *d2scene.Document) {
			d.Root.Mask.Root.Children[0].Primitive = d2scene.Path{Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, Commands: []d2scene.PathCommand{d2scene.MoveTo(-20, -20), d2scene.LineTo(220, -20), d2scene.LineTo(-20, -20), d2scene.LineTo(-20, 920), d2scene.ClosePath()}}
		}},
		{name: "alpha mask", mutate: func(d *d2scene.Document) { d.Root.Mask.Type = d2scene.MaskAlpha }},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := regionMaskDocument()
			if test.mutate != nil {
				test.mutate(document)
			}
			prepared, err := prepareWithSessionBands(context.Background(), document, testOptions(), nil, 64)
			if err != nil {
				t.Fatal(err)
			}
			destination := image.Rect(10, 10, 190, 150)
			got, err := maskRenderBounds(context.Background(), prepared.root.mask, destination, true)
			if err != nil {
				t.Fatal(err)
			}
			if test.cropped && (got.Empty() || got == destination) || !test.cropped && got != destination {
				t.Fatalf("mask bounds=%v cropped=%v", got, test.cropped)
			}
		})
	}
}

func TestMaskRegionsMatchFullFrame(t *testing.T) {
	for _, effect := range []string{"plain", "opacity", "blur", "shadow", "nested", "clip", "mask", "root-transform"} {
		t.Run(effect, func(t *testing.T) {
			document := regionMaskDocument()
			hole := document.Root.Mask.Root.Children[1]
			switch effect {
			case "opacity":
				hole.Opacity = .75
			case "blur":
				hole.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 2.37, SigmaY: 5.13}}
			case "shadow":
				hole.Filters = []d2scene.Filter{d2scene.DropShadow{OffsetX: -7.13, OffsetY: 23.71, SigmaX: 2.1, SigmaY: 3.7, Color: color.NRGBA{A: 197}}}
			case "nested":
				hole.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 3.1, SigmaY: 2.7}}
				group := d2scene.NewNode(nil)
				group.Opacity = .71
				group.Children = []*d2scene.Node{hole}
				document.Root.Mask.Root.Children[1] = group
			case "clip":
				hole.Clip = &d2scene.Clip{Transform: d2scene.Identity(), Path: d2scene.Path{Commands: []d2scene.PathCommand{d2scene.MoveTo(40, 40), d2scene.LineTo(90, 40), d2scene.LineTo(70, 100), d2scene.ClosePath()}}}
			case "mask":
				hole.Mask = &d2scene.Mask{Transform: d2scene.Identity(), Type: d2scene.MaskAlpha, Root: d2scene.NewNode(d2scene.Ellipse{Center: d2scene.Point{X: 70, Y: 70}, RadiusX: 12, RadiusY: 19, Fill: red})}
			case "root-transform":
				document.Root.Mask.Root.Transform = d2scene.Matrix{A: 1.1, D: .95, E: -.17, F: .71}
			}
			for _, scale := range []float64{.5, 1, 1.25} {
				options := testOptions()
				options.Scale = scale
				options.MaxHeight = 1200
				want, err := Render(context.Background(), document, options)
				if err != nil {
					t.Fatal(err)
				}
				for _, height := range []int{1, 7, 64, 256} {
					got := image.NewNRGBA(want.Bounds())
					if err := RenderBands(context.Background(), document, options, height, func(b *image.NRGBA) error { draw.Draw(got, b.Bounds(), b, b.Bounds().Min, draw.Src); return nil }); err != nil {
						t.Fatal(err)
					}
					assertBandPixels(t, got, want)
				}
			}
		})
	}
}

func TestMaskRegionsSkipUnchangedPixelsAndCancellation(t *testing.T) {
	prepared, err := prepareWithSessionBands(context.Background(), regionMaskDocument(), testOptions(), nil, 64)
	if err != nil {
		t.Fatal(err)
	}
	bounds, err := maskRenderBounds(context.Background(), prepared.root.mask, image.Rect(10, 300, 190, 350), true)
	if err != nil || !bounds.Empty() {
		t.Fatalf("unchanged bounds=%v error=%v", bounds, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := maskRenderBounds(ctx, prepared.root.mask, image.Rect(10, 10, 190, 150), true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	bounds, err = maskRenderBounds(context.Background(), prepared.root.mask, image.Rect(-20, -20, 190, 150), true)
	if err != nil || bounds != image.Rect(-20, -20, 190, 150) {
		t.Fatalf("white backdrop edge bounds=%v error=%v", bounds, err)
	}
}
