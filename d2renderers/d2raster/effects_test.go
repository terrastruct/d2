package d2raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestTransformedAndEvenOddClips(t *testing.T) {
	t.Run("composed transforms", func(t *testing.T) {
		painted := d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: 10, Height: 10}, Fill: red,
		})
		painted.Transform = d2scene.Scale(2, 2)
		painted.Clip = &d2scene.Clip{
			Path:      clipRect(0, 0, 4, 10, d2scene.NonZero),
			Transform: d2scene.Translate(2, 0),
		}
		root := d2scene.NewNode(nil)
		root.Transform = d2scene.Translate(3, 2)
		root.Children = []*d2scene.Node{painted}
		frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 30, Height: 25}, root), testOptions())
		if err != nil {
			t.Fatal(err)
		}
		assertPixel(t, frame.NRGBAAt(10, 10), color.NRGBA{R: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(5, 10), color.NRGBA{})
		assertPixel(t, frame.NRGBAAt(16, 10), color.NRGBA{})
	})

	t.Run("even odd hole and antialias", func(t *testing.T) {
		node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 20, Height: 20}, Fill: red})
		path := clipRect(2.25, 2.25, 15.5, 15.5, d2scene.EvenOdd)
		path.Commands = append(path.Commands, clipRect(7, 7, 6, 6, d2scene.EvenOdd).Commands...)
		node.Clip = &d2scene.Clip{Path: path, Transform: d2scene.Identity()}
		frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 20, Height: 20}, node), testOptions())
		if err != nil {
			t.Fatal(err)
		}
		assertPixel(t, frame.NRGBAAt(4, 4), color.NRGBA{R: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(10, 10), color.NRGBA{})
		foundAntialias := false
		for y := 0; y < 20; y++ {
			for x := 0; x < 20; x++ {
				alpha := frame.NRGBAAt(x, y).A
				foundAntialias = foundAntialias || alpha > 0 && alpha < 255
			}
		}
		if !foundAntialias {
			t.Fatal("even-odd clip has no antialiased boundary pixels")
		}
	})
}

func TestAlphaAndLuminanceMasks(t *testing.T) {
	t.Run("alpha", func(t *testing.T) {
		maskRoot := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{Width: 10, Height: 20},
			Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 128}},
		})
		node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 20, Height: 20}, Fill: red})
		node.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: maskRoot, Transform: d2scene.Identity()}
		frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 20, Height: 20}, node), testOptions())
		if err != nil {
			t.Fatal(err)
		}
		got := frame.NRGBAAt(5, 10)
		if got.R != 255 || got.A < 127 || got.A > 128 {
			t.Fatalf("alpha-mask pixel = %#v, want red at half alpha", got)
		}
		assertPixel(t, frame.NRGBAAt(15, 10), color.NRGBA{})
	})

	newLabelHoleDocument := func(maskType d2scene.MaskType) *d2scene.Document {
		white := d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
		maskRoot := d2scene.NewNode(nil)
		base := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 20, Height: 20}, Fill: white})
		hole := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 6, Width: 8, Height: 20}, Fill: black})
		hole.Opacity = .75
		maskRoot.Children = []*d2scene.Node{base, hole}
		node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 20, Height: 20}, Fill: red})
		node.Mask = &d2scene.Mask{Type: maskType, Root: maskRoot, Transform: d2scene.Identity()}
		return d2scene.NewDocument(d2scene.Box{Width: 20, Height: 20}, node)
	}

	t.Run("luminance label hole", func(t *testing.T) {
		frame, err := Render(context.Background(), newLabelHoleDocument(d2scene.MaskLuminance), testOptions())
		if err != nil {
			t.Fatal(err)
		}
		assertPixel(t, frame.NRGBAAt(3, 10), color.NRGBA{R: 255, A: 255})
		got := frame.NRGBAAt(10, 10)
		if got.R != 255 || got.A < 63 || got.A > 65 {
			t.Fatalf("75%% black-over-white luminance hole = %#v, want red at 25%% alpha", got)
		}
	})

	t.Run("alpha differs from luminance", func(t *testing.T) {
		frame, err := Render(context.Background(), newLabelHoleDocument(d2scene.MaskAlpha), testOptions())
		if err != nil {
			t.Fatal(err)
		}
		assertPixel(t, frame.NRGBAAt(10, 10), color.NRGBA{R: 255, A: 255})
	})
}

func TestNestedClipMaskAndMaskTransform(t *testing.T) {
	maskRoot := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{Width: 20, Height: 5},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	})
	inner := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 20, Height: 20}, Fill: red})
	inner.Mask = &d2scene.Mask{
		Type: d2scene.MaskAlpha, Root: maskRoot, Transform: d2scene.Translate(0, 5),
	}
	outer := d2scene.NewNode(nil)
	outer.Transform = d2scene.Translate(2, 3)
	outer.Children = []*d2scene.Node{inner}
	outer.Clip = &d2scene.Clip{Path: clipRect(0, 0, 10, 20, d2scene.NonZero), Transform: d2scene.Identity()}
	frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 25, Height: 25}, outer), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, frame.NRGBAAt(5, 10), color.NRGBA{R: 255, A: 255})
	assertPixel(t, frame.NRGBAAt(5, 5), color.NRGBA{})
	assertPixel(t, frame.NRGBAAt(15, 10), color.NRGBA{})
}

func TestSharedMaskRootConcurrentRendersDoNotMutate(t *testing.T) {
	white := d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
	sharedRoot := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 5, Height: 10}, Fill: white})
	sharedMask := &d2scene.Mask{Type: d2scene.MaskAlpha, Root: sharedRoot, Transform: d2scene.Identity()}
	left := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
	left.Transform = d2scene.Translate(2, 2)
	left.Mask = sharedMask
	right := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: blue})
	right.Transform = d2scene.Translate(18, 2)
	right.Mask = sharedMask
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{left, right}
	document := d2scene.NewDocument(d2scene.Box{Width: 32, Height: 14}, root)

	baseline, err := renderTestPNG(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	const renders = 12
	errorsByRender := make([]error, renders)
	var wait sync.WaitGroup
	for index := 0; index < renders; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			got, err := renderTestPNG(context.Background(), document, testOptions())
			if err == nil && !bytes.Equal(got, baseline) {
				err = errors.New("render bytes differ from baseline")
			}
			errorsByRender[index] = err
		}(index)
	}
	wait.Wait()
	for index, err := range errorsByRender {
		if err != nil {
			t.Fatalf("concurrent render %d: %v", index, err)
		}
	}
	if sharedRoot.Transform != d2scene.Identity() || sharedMask.Transform != d2scene.Identity() || left.Transform != d2scene.Translate(2, 2) || right.Transform != d2scene.Translate(18, 2) {
		t.Fatal("concurrent render mutated shared mask scene transforms")
	}
}

func TestEffectPreflightLimitsAndInvalidInputs(t *testing.T) {
	validMaskRoot := func() *d2scene.Node {
		return d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
	}
	invalidTests := []struct {
		name   string
		change func(*d2scene.Node)
		want   string
	}{
		{name: "nil mask root", change: func(node *d2scene.Node) {
			node.Mask = &d2scene.Mask{Transform: d2scene.Identity()}
		}, want: "invalid mask"},
		{name: "mask type", change: func(node *d2scene.Node) {
			node.Mask = &d2scene.Mask{Type: d2scene.MaskType(255), Root: validMaskRoot(), Transform: d2scene.Identity()}
		}, want: "invalid mask"},
		{name: "mask transform", change: func(node *d2scene.Node) {
			node.Mask = &d2scene.Mask{Root: validMaskRoot(), Transform: d2scene.Matrix{A: math.NaN(), D: 1}}
		}, want: "invalid mask"},
		{name: "clip transform", change: func(node *d2scene.Node) {
			node.Clip = &d2scene.Clip{Path: clipRect(0, 0, 5, 5, d2scene.NonZero), Transform: d2scene.Matrix{A: 1, D: math.Inf(1)}}
		}, want: "invalid clip transform"},
		{name: "clip fill rule", change: func(node *d2scene.Node) {
			node.Clip = &d2scene.Clip{Path: clipRect(0, 0, 5, 5, d2scene.FillRule(255)), Transform: d2scene.Identity()}
		}, want: "invalid fill rule"},
	}
	for _, test := range invalidTests {
		t.Run(test.name, func(t *testing.T) {
			document := testDocument(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
			test.change(document.Root)
			_, err := prepare(context.Background(), document, testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("prepare() error = %v, want %q", err, test.want)
			}
		})
	}

	t.Run("mask cycle", func(t *testing.T) {
		node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
		node.Mask = &d2scene.Mask{Root: node, Transform: d2scene.Identity()}
		_, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, node), testOptions())
		if err == nil || !strings.Contains(err.Error(), "node cycle") {
			t.Fatalf("prepare() error = %v, want node cycle", err)
		}
	})

	t.Run("mask counts toward node and depth limits", func(t *testing.T) {
		for _, limit := range []struct {
			name   string
			change func(*FrameOptions)
			want   string
		}{
			{name: "node", change: func(options *FrameOptions) { options.MaxNodes = 1 }, want: "node count"},
			{name: "depth", change: func(options *FrameOptions) { options.MaxDepth = 1 }, want: "node depth"},
		} {
			t.Run(limit.name, func(t *testing.T) {
				document := testDocument(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
				document.Root.Mask = &d2scene.Mask{Root: validMaskRoot(), Transform: d2scene.Identity()}
				options := testOptions()
				limit.change(&options)
				_, err := prepare(context.Background(), document, options)
				if err == nil || !strings.Contains(err.Error(), limit.want) {
					t.Fatalf("prepare() error = %v, want %q", err, limit.want)
				}
			})
		}
	})

	t.Run("clip counts toward path limit", func(t *testing.T) {
		document := testDocument(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
		document.Root.Clip = &d2scene.Clip{Path: clipRect(0, 0, 10, 10, d2scene.NonZero), Transform: d2scene.Identity()}
		options := testOptions()
		options.MaxPathCommands = 3
		_, err := prepare(context.Background(), document, options)
		if err == nil || !strings.Contains(err.Error(), "path command") {
			t.Fatalf("prepare() error = %v, want path command limit", err)
		}
	})
}

func TestSingularAndZeroAreaEffectsRenderTransparent(t *testing.T) {
	tests := []struct {
		name   string
		change func(*d2scene.Node)
	}{
		{name: "singular clip", change: func(node *d2scene.Node) {
			node.Clip = &d2scene.Clip{Path: clipRect(0, 0, 10, 10, d2scene.NonZero), Transform: d2scene.Scale(0, 0)}
		}},
		{name: "zero-area clip", change: func(node *d2scene.Node) {
			node.Clip = &d2scene.Clip{Path: d2scene.Path{FillRule: d2scene.NonZero}, Transform: d2scene.Identity()}
		}},
		{name: "singular alpha mask", change: func(node *d2scene.Node) {
			root := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
			node.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: root, Transform: d2scene.Scale(0, 0)}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
			test.change(node)
			frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, node), testOptions())
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 10; y++ {
				for x := 0; x < 10; x++ {
					if frame.NRGBAAt(x, y).A != 0 {
						t.Fatalf("pixel (%d,%d) = %#v, want transparent", x, y, frame.NRGBAAt(x, y))
					}
				}
			}
		})
	}
}

func TestOpacityLayerUsesInkBounds(t *testing.T) {
	group := d2scene.NewNode(nil)
	group.Opacity = .5
	group.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 400, Y: 500, Width: 10, Height: 8}, Fill: red}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 1000, Height: 1000}, group)
	prepared, err := prepare(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.root.bounds == image.Rect(0, 0, 1000, 1000) || prepared.root.bounds.Dx() > 14 || prepared.root.bounds.Dy() > 12 {
		t.Fatalf("opacity layer bounds = %v, want small ink-bounds layer", prepared.root.bounds)
	}
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	got := frame.NRGBAAt(405, 504)
	if got.R != 255 || got.A < 127 || got.A > 128 {
		t.Fatalf("bounds-sized opacity pixel = %#v", got)
	}
}

func TestBoundsSizedEffectLayerPreservesTransformedStroke(t *testing.T) {
	node := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{d2scene.MoveTo(0, 5), d2scene.LineTo(10, 5)},
		Stroke: &d2scene.Stroke{
			Paint: blue, Width: 2, Cap: d2scene.CapRound, Join: d2scene.JoinRound,
		},
	})
	node.Transform = d2scene.Translate(20, 10).Mul(d2scene.Scale(2, 1))
	node.Opacity = .5
	frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 50, Height: 30}, node), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	got := frame.NRGBAAt(30, 15)
	if got.B != 255 || got.A < 127 || got.A > 128 {
		t.Fatalf("transformed bounds-layer stroke = %#v, want blue at half alpha", got)
	}
	assertPixel(t, frame.NRGBAAt(10, 15), color.NRGBA{})
}

func TestEffectLoopsHonorCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	layer := image.NewRGBA(image.Rect(20, 30, 532, 542))
	mask := image.NewAlpha(image.Rect(0, 0, 512, 512))
	if err := multiplyLayerByAlpha(ctx, layer, mask); !errors.Is(err, context.Canceled) {
		t.Fatalf("multiplyLayerByAlpha() error = %v, want context.Canceled", err)
	}
}

func TestOffscreenResourcePlanIsByteExactAndInclusive(t *testing.T) {
	fullRect := func(paint d2scene.Paint) *d2scene.Node {
		return d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: paint})
	}
	white := d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}

	opacity := fullRect(red)
	opacity.Opacity = .5

	clipped := fullRect(red)
	clipped.Clip = &d2scene.Clip{Path: clipRect(0, 0, 10, 10, d2scene.NonZero), Transform: d2scene.Identity()}

	masked := fullRect(red)
	masked.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: fullRect(white), Transform: d2scene.Identity()}

	nested := d2scene.NewNode(nil)
	nested.Opacity = .5
	nestedChild := fullRect(red)
	nestedChild.Opacity = .5
	nested.Children = []*d2scene.Node{nestedChild}

	nestedMask := fullRect(red)
	nestedMaskRoot := fullRect(white)
	nestedMaskRoot.Opacity = .5
	nestedMask.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: nestedMaskRoot, Transform: d2scene.Identity()}
	gradientNode := fullRect(d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 10}, Units: d2scene.UserSpaceOnUse,
		Transform: d2scene.Identity(), Stops: gradientStops,
	})

	solid := fullRect(red)

	scanlineScratch, ok := scanline.RetainedBytes(10, 10, 2)
	if !ok {
		t.Fatal("scanline retained-byte calculation overflowed")
	}
	tests := []struct {
		name string
		node *d2scene.Node
		want int64
	}{
		{name: "direct solid scanline scratch", node: solid, want: scanlineScratch},
		{name: "RGBA opacity layer plus scanline scratch", node: opacity, want: 400 + scanlineScratch},
		{name: "RGBA layer plus Alpha clip and scanline scratch", node: clipped, want: 500 + scanlineScratch},
		{name: "RGBA layer plus RGBA mask and scanline scratch", node: masked, want: 800 + scanlineScratch},
		{name: "nested RGBA opacity layers plus scanline scratch", node: nested, want: 800 + scanlineScratch},
		{name: "mask with nested RGBA layer plus scanline scratch", node: nestedMask, want: 1200 + scanlineScratch},
		{name: "direct gradient Alpha mask plus scanline scratch", node: gradientNode, want: 100 + scanlineScratch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, test.node)
			options := testOptions()
			prepared, err := prepare(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.resources.peakOffscreenBytes != test.want {
				t.Fatalf("peak offscreen bytes = %d, want %d", prepared.resources.peakOffscreenBytes, test.want)
			}

			options.MaxOffscreenBytes = test.want - 1
			if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage") {
				t.Fatalf("below-limit prepare() error = %v, want offscreen limit", err)
			}
			options.MaxOffscreenBytes = test.want
			if _, err := Render(context.Background(), document, options); err != nil {
				t.Fatalf("inclusive-limit Render() error = %v", err)
			}
		})
	}

	t.Run("sequential siblings reuse peak budget", func(t *testing.T) {
		root := d2scene.NewNode(nil)
		left, right := fullRect(red), fullRect(blue)
		left.Opacity, right.Opacity = .5, .5
		root.Children = []*d2scene.Node{left, right}
		document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, root)
		options := testOptions()
		want := int64(400) + scanlineScratch
		options.MaxOffscreenBytes = want
		prepared, err := prepare(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		if prepared.resources.peakOffscreenBytes != want {
			t.Fatalf("sequential-sibling peak = %d, want %d", prepared.resources.peakOffscreenBytes, want)
		}
	})

	t.Run("scanline rasterizer retains bounded working storage", func(t *testing.T) {
		node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 513, Height: 1}, Fill: red})
		document := d2scene.NewDocument(d2scene.Box{Width: 513, Height: 1}, node)
		want, ok := scanline.RetainedBytes(513, 1, 2)
		if !ok {
			t.Fatal("scanline retained-byte calculation overflowed")
		}
		options := testOptions()
		options.MaxOffscreenBytes = want - 1
		if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage") {
			t.Fatalf("below-limit prepare() error = %v, want scanline storage limit", err)
		}
		options.MaxOffscreenBytes = want
		prepared, err := prepare(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		if prepared.resources.rasterizerBytes != want || prepared.resources.peakOffscreenBytes != want {
			t.Fatalf("scanline raster plan = %+v, want %d bytes", prepared.resources, want)
		}
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("inclusive scanline-raster limit: %v", err)
		}
	})
}

func TestOffscreenReservationExhaustionRollsBackNestedState(t *testing.T) {
	fullRect := func(paint d2scene.Paint) *d2scene.Node {
		return d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: paint})
	}
	white := d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}

	clipped := fullRect(red)
	clipped.Clip = &d2scene.Clip{Path: clipRect(0, 0, 10, 10, d2scene.NonZero), Transform: d2scene.Identity()}

	masked := fullRect(red)
	masked.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: fullRect(white), Transform: d2scene.Identity()}

	nestedOpacity := d2scene.NewNode(nil)
	nestedOpacity.Opacity = .5
	nestedOpacityChild := fullRect(red)
	nestedOpacityChild.Opacity = .5
	nestedOpacity.Children = []*d2scene.Node{nestedOpacityChild}

	nestedMask := fullRect(red)
	nestedMaskRoot := fullRect(white)
	nestedMaskRoot.Opacity = .5
	nestedMask.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: nestedMaskRoot, Transform: d2scene.Identity()}
	gradientNode := fullRect(d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 10}, Units: d2scene.UserSpaceOnUse,
		Transform: d2scene.Identity(), Stops: gradientStops,
	})
	scanlineScratch, ok := scanline.RetainedBytes(10, 10, 2)
	if !ok {
		t.Fatal("scanline retained-byte calculation overflowed")
	}

	for _, test := range []struct {
		name string
		node *d2scene.Node
		peak int64
	}{
		{name: "clip Alpha allocation", node: clipped, peak: 500 + scanlineScratch},
		{name: "mask RGBA allocation", node: masked, peak: 800 + scanlineScratch},
		{name: "nested opacity allocation", node: nestedOpacity, peak: 800 + scanlineScratch},
		{name: "nested mask-root allocation", node: nestedMask, peak: 1200 + scanlineScratch},
		{name: "gradient Alpha allocation", node: gradientNode, peak: 100 + scanlineScratch},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, test.node)
			prepared, err := prepare(context.Background(), document, testOptions())
			if err != nil {
				t.Fatal(err)
			}
			scratch := &rasterScratch{offscreen: offscreenBudget{limit: test.peak - 1}}
			canvas := image.NewRGBA(image.Rect(0, 0, 10, 10))
			rasterizerBytes, err := scratch.offscreen.reserveBytes(prepared.resources.rasterizerBytes, "scanline rasterizer working storage")
			if err != nil {
				t.Fatal(err)
			}
			err = renderNode(context.Background(), canvas, prepared.root, scratch)
			if err == nil || !strings.Contains(err.Error(), "exceeding limit") {
				t.Fatalf("renderNode() error = %v, want offscreen exhaustion", err)
			}
			if scratch.offscreen.live != rasterizerBytes {
				t.Fatalf("live bytes after failed render = %d, want retained rasterizer %d", scratch.offscreen.live, rasterizerBytes)
			}
			scratch.offscreen.release(rasterizerBytes)
			if scratch.offscreen.live != 0 {
				t.Fatalf("live bytes after releasing rasterizer = %d, want 0", scratch.offscreen.live)
			}

			// Reusing the same tracker after raising its limit proves every
			// successful ancestor reservation was rolled back on the error path.
			scratch.offscreen.limit = test.peak
			rasterizerBytes, err = scratch.offscreen.reserveBytes(prepared.resources.rasterizerBytes, "scanline rasterizer working storage")
			if err != nil {
				t.Fatal(err)
			}
			if err := renderNode(context.Background(), canvas, prepared.root, scratch); err != nil {
				t.Fatalf("render after rollback: %v", err)
			}
			if scratch.offscreen.live != rasterizerBytes || scratch.offscreen.peak != test.peak {
				t.Fatalf("tracker after successful retry = %+v, want live %d peak %d", scratch.offscreen, rasterizerBytes, test.peak)
			}
			scratch.offscreen.release(rasterizerBytes)
			if scratch.offscreen.live != 0 {
				t.Fatalf("tracker live bytes after retry release = %d, want 0", scratch.offscreen.live)
			}
		})
	}
}

func TestScanlineResourcePlanRetainsIndependentDimensions(t *testing.T) {
	gradient := d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1000}, Units: d2scene.UserSpaceOnUse,
		Transform: d2scene.Identity(), Stops: gradientStops,
	}
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1000, Height: 1}, Fill: gradient}),
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1000}, Fill: gradient}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 1000, Height: 1000}, root)
	wantScratch, ok := scanline.RetainedBytes(1000, 1000, 2)
	if !ok {
		t.Fatal("scanline retained-byte calculation overflowed")
	}
	// Each gradient uses a clipped 1000x2 or 2x1000 Alpha mask. They are
	// sequential, while the rasterizer retains both dimension maxima.
	wantPeak := wantScratch + 2000
	options := testOptions()
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.resources.rasterizerWidth != 1000 || prepared.resources.rasterizerHeight != 1000 || prepared.resources.rasterizerEdges != 2 {
		t.Fatalf("rasterizer maxima = %+v, want width=1000 height=1000 edges=2", prepared.resources)
	}
	if prepared.resources.rasterizerBytes != wantScratch || prepared.resources.peakOffscreenBytes != wantPeak {
		t.Fatalf("resource plan = %+v, want scratch=%d peak=%d", prepared.resources, wantScratch, wantPeak)
	}
	options.MaxOffscreenBytes = wantPeak - 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage") {
		t.Fatalf("below-limit prepare() error = %v, want offscreen limit", err)
	}
	options.MaxOffscreenBytes = wantPeak
	if _, err := Render(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive retained-dimensions limit: %v", err)
	}
}

func TestScanlineResourcePlanChargesGeneratedEdges(t *testing.T) {
	node := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{d2scene.MoveTo(2, 10), d2scene.LineTo(22, 10)},
		Stroke: &d2scene.Stroke{
			Paint: red, Width: 6, Cap: d2scene.CapRound, Join: d2scene.JoinRound,
		},
	})
	document := d2scene.NewDocument(d2scene.Box{Width: 24, Height: 20}, node)
	options := testOptions()
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.resources.rasterizerWidth != 24 || prepared.resources.rasterizerHeight != 20 || prepared.resources.rasterizerEdges != 18 {
		t.Fatalf("round-cap rasterizer plan = %+v, want width=24 height=20 edges=18", prepared.resources)
	}
	if prepared.resources.rasterizerEdges <= len(prepared.root.primitive.strokeRuns[0].points) {
		t.Fatalf("generated edges = %d, want more than %d centerline points", prepared.resources.rasterizerEdges, len(prepared.root.primitive.strokeRuns[0].points))
	}
	want, ok := scanline.RetainedBytes(24, 20, 18)
	if !ok {
		t.Fatal("scanline retained-byte calculation overflowed")
	}
	if prepared.resources.rasterizerBytes != want || prepared.resources.peakOffscreenBytes != want {
		t.Fatalf("round-cap resource plan = %+v, want %d bytes", prepared.resources, want)
	}
	options.MaxOffscreenBytes = want - 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage") {
		t.Fatalf("below-limit prepare() error = %v, want offscreen limit", err)
	}
	options.MaxOffscreenBytes = want
	if _, err := Render(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive generated-edge limit: %v", err)
	}

	edgeBudget, ok := scanline.RetainedBytes(0, 0, 7)
	if !ok {
		t.Fatal("edge-budget calculation overflowed")
	}
	options.MaxOffscreenBytes = edgeBudget
	if _, err := prepare(context.Background(), document, options); !errors.Is(err, scanline.ErrEdgeLimit) {
		t.Fatalf("bounded edge-plan error = %v, want ErrEdgeLimit", err)
	}
}

func TestScanlineWorkPlanIsAggregateAndInclusive(t *testing.T) {
	oneRoot := d2scene.NewNode(nil)
	oneRoot.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red}),
	}
	options := testOptions()
	one, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, oneRoot), options)
	if err != nil {
		t.Fatal(err)
	}
	perFill := one.resources.scanlineWork
	if perFill <= 0 {
		t.Fatalf("single-fill scanline work = %d, want positive", perFill)
	}

	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red}),
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, root)
	want := 2 * perFill
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.resources.scanlineWork != want {
		t.Fatalf("scanline work = %d, want %d", prepared.resources.scanlineWork, want)
	}
	options.MaxScanlineWork = want - 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "scanline work") {
		t.Fatalf("below-limit prepare() error = %v, want scanline work limit", err)
	}
	options.MaxScanlineWork = want
	if _, err := Render(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive scanline work limit: %v", err)
	}
}

func TestScanlineWorkPlanUsesGeometryMetricsForSmallPrimitives(t *testing.T) {
	const (
		canvasSize = 4_096
		shapes     = 128
	)
	root := d2scene.NewNode(nil)
	for index := range shapes {
		x := float64((index % 16) * 20)
		y := float64((index / 16) * 20)
		root.Children = append(root.Children, d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{X: x, Y: y, Width: 10, Height: 10},
			Fill: red,
		}))
	}
	options := testOptions()
	options.MaxWidth = canvasSize
	options.MaxHeight = canvasSize
	options.MaxPixels = canvasSize * canvasSize
	prepared, err := prepare(context.Background(), d2scene.NewDocument(
		d2scene.Box{Width: canvasSize, Height: canvasSize}, root,
	), options)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.resources.scanlineWork >= 4_000_000_000 {
		t.Fatalf("geometry-aware aggregate work = %d, want below the CLI 4B ceiling", prepared.resources.scanlineWork)
	}
	worstPerShape, ok := scanline.WorkBound(canvasSize, canvasSize, 2)
	if !ok || int64(shapes)*worstPerShape <= 4_000_000_000 {
		t.Fatalf("whole-target aggregate = %d, %v; fixture must exercise the former false rejection", int64(shapes)*worstPerShape, ok)
	}
}

func TestEvenOddClipWorkLimitAndEdgeCancellation(t *testing.T) {
	newClippedNode := func() *d2scene.Node {
		node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red})
		node.Clip = &d2scene.Clip{Path: clipRect(0, 0, 10, 10, d2scene.EvenOdd), Transform: d2scene.Identity()}
		return node
	}

	t.Run("work is aggregate and inclusive", func(t *testing.T) {
		root := d2scene.NewNode(nil)
		root.Children = []*d2scene.Node{newClippedNode(), newClippedNode()}
		document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, root)
		options := testOptions()
		prepared, err := prepare(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		const perClip = int64(10 * 10 * 4 * 4 * 4)
		if prepared.resources.evenOddClipWork != 2*perClip {
			t.Fatalf("even-odd work = %d, want %d", prepared.resources.evenOddClipWork, 2*perClip)
		}
		options.MaxEvenOddClipWork = 2*perClip - 1
		if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "even-odd clip work") {
			t.Fatalf("below-limit prepare() error = %v, want even-odd work limit", err)
		}
		options.MaxEvenOddClipWork = 2 * perClip
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("inclusive-limit Render() error = %v", err)
		}
	})

	t.Run("cancellation inside one large edge scan", func(t *testing.T) {
		points := make([]d2scene.Point, 1024)
		for index := range points {
			points[index] = d2scene.Point{X: float64(index), Y: float64(index & 1)}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var evaluations uint64
		_, err := pointInEvenOddPath(ctx, []subpath{{points: points}}, 0, 0, &evaluations)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pointInEvenOddPath() error = %v, want context.Canceled", err)
		}
		if evaluations != 256 {
			t.Fatalf("edge evaluations before cancellation = %d, want 256", evaluations)
		}
	})
}

func BenchmarkRenderBoundsSizedEffects(b *testing.B) {
	white := d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
	maskRoot := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 96, Height: 64}, Fill: white})
	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 96, Height: 64}, Fill: red})
	node.Transform = d2scene.Translate(440, 468)
	node.Opacity = .75
	node.Clip = &d2scene.Clip{Path: clipRect(4, 4, 88, 56, d2scene.NonZero), Transform: d2scene.Identity()}
	node.Mask = &d2scene.Mask{Type: d2scene.MaskLuminance, Root: maskRoot, Transform: d2scene.Identity()}
	document := d2scene.NewDocument(d2scene.Box{Width: 1000, Height: 1000}, node)
	options := testOptions()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		frame, err := Render(ctx, document, options)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkFrame = frame
	}
}

func clipRect(x, y, width, height float64, rule d2scene.FillRule) d2scene.Path {
	return d2scene.Path{FillRule: rule, Commands: []d2scene.PathCommand{
		d2scene.MoveTo(x, y),
		d2scene.LineTo(x+width, y),
		d2scene.LineTo(x+width, y+height),
		d2scene.LineTo(x, y+height),
		d2scene.ClosePath(),
	}}
}
