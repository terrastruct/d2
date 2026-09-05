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

func TestStreamedNonZeroClipMatchesAlphaMaskRandomized(t *testing.T) {
	t.Parallel()

	parentBounds := image.Rect(0, 0, 103, 89)
	bounds := image.Rect(11, 9, 92, 78)
	state := uint64(0xa0761d6478bd642f)
	next := func() uint64 {
		state ^= state >> 12
		state ^= state << 25
		state ^= state >> 27
		return state * 0x2545f4914f6cdd1d
	}
	coordinate := func(minimum, size int) float64 {
		return float64(minimum-2) + float64(next()%uint64((size+4)*8))/8
	}

	for iteration := range 160 {
		var paths []subpath
		for range int(next()%9) + 1 {
			points := make([]d2scene.Point, int(next()%8)+2)
			for index := range points {
				points[index] = d2scene.Point{
					X: coordinate(bounds.Min.X, bounds.Dx()),
					Y: coordinate(bounds.Min.Y, bounds.Dy()),
				}
			}
			paths = append(paths, subpath{points: points})
		}
		clip := &preparedClip{subpaths: paths, fillRule: d2scene.NonZero, bounds: bounds}

		seed := image.NewRGBA(parentBounds)
		for offset := 0; offset < len(seed.Pix); offset += 4 {
			alpha := uint8(next())
			seed.Pix[offset+3] = alpha
			seed.Pix[offset+0] = uint8(next() % (uint64(alpha) + 1))
			seed.Pix[offset+1] = uint8(next() % (uint64(alpha) + 1))
			seed.Pix[offset+2] = uint8(next() % (uint64(alpha) + 1))
		}

		wantParent := image.NewRGBA(parentBounds)
		copy(wantParent.Pix, seed.Pix)
		want := wantParent.SubImage(bounds).(*image.RGBA)
		wantScratch := &rasterScratch{offscreen: offscreenBudget{limit: math.MaxInt64}}
		mask := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		rasterizer := wantScratch.reset(mask.Bounds())
		shifted := d2scene.Translate(-float64(bounds.Min.X), -float64(bounds.Min.Y))
		for _, path := range paths {
			addFillSubpath(rasterizer, path, shifted)
		}
		if err := rasterizer.WriteAlpha(context.Background(), wantScratch.workBudget(), mask); err != nil {
			t.Fatalf("iteration %d Alpha-mask rasterization: %v", iteration, err)
		}
		if err := multiplyLayerByAlpha(context.Background(), want, mask); err != nil {
			t.Fatalf("iteration %d Alpha-mask application: %v", iteration, err)
		}

		gotParent := image.NewRGBA(parentBounds)
		copy(gotParent.Pix, seed.Pix)
		got := gotParent.SubImage(bounds).(*image.RGBA)
		gotScratch := &rasterScratch{offscreen: offscreenBudget{limit: math.MaxInt64}}
		if err := applyNonZeroClip(context.Background(), got, clip, gotScratch); err != nil {
			t.Fatalf("iteration %d streamed clip: %v", iteration, err)
		}
		if !bytes.Equal(gotParent.Pix, wantParent.Pix) {
			for index := range gotParent.Pix {
				if gotParent.Pix[index] != wantParent.Pix[index] {
					t.Fatalf("iteration %d first differing parent byte %d: streamed=%d Alpha-mask=%d", iteration, index, gotParent.Pix[index], wantParent.Pix[index])
				}
			}
			t.Fatalf("iteration %d output lengths differ", iteration)
		}
	}

	for _, test := range []struct {
		name string
		clip *preparedClip
	}{
		{name: "disjoint bounds", clip: &preparedClip{fillRule: d2scene.NonZero, bounds: image.Rect(-20, -20, -10, -10)}},
		{name: "no edges", clip: &preparedClip{fillRule: d2scene.NonZero, bounds: bounds}},
	} {
		parent := image.NewRGBA(parentBounds)
		for index := range parent.Pix {
			parent.Pix[index] = 0xff
		}
		if err := applyNonZeroClip(context.Background(), parent.SubImage(bounds).(*image.RGBA), test.clip, &rasterScratch{}); err != nil {
			t.Fatal(err)
		}
		for y := parentBounds.Min.Y; y < parentBounds.Max.Y; y++ {
			for x := parentBounds.Min.X; x < parentBounds.Max.X; x++ {
				pixel := parent.RGBAAt(x, y)
				inside := image.Pt(x, y).In(bounds)
				if inside && pixel != (color.RGBA{}) || !inside && pixel != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
					t.Fatalf("%s clip pixel (%d,%d) = %#v, inside=%v", test.name, x, y, pixel, inside)
				}
			}
		}
	}
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
	clip := &preparedClip{
		fillRule: d2scene.NonZero,
		bounds:   layer.Bounds(),
		subpaths: []subpath{{points: []d2scene.Point{
			{X: 20, Y: 30}, {X: 532, Y: 30}, {X: 532, Y: 542}, {X: 20, Y: 542},
		}}},
	}
	if err := applyNonZeroClip(ctx, layer, clip, &rasterScratch{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("applyNonZeroClip() error = %v, want context.Canceled", err)
	}

	wideBounds := image.Rect(0, 0, 8_192, 2)
	wideClip := &preparedClip{
		fillRule: d2scene.NonZero,
		bounds:   wideBounds,
		subpaths: []subpath{{points: []d2scene.Point{
			{}, {X: 8_192}, {X: 8_192, Y: 2}, {Y: 2},
		}}},
	}
	cancelDuringRow := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: 5}
	if err := applyNonZeroClip(cancelDuringRow, image.NewRGBA(wideBounds), wideClip, &rasterScratch{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("wide applyNonZeroClip() error = %v, want context.Canceled", err)
	}
	if cancelDuringRow.calls < 5 {
		t.Fatalf("wide clip context checks = %d, want mid-row cancellation", cancelDuringRow.calls)
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
	evenOddClipped := fullRect(red)
	evenOddClipped.Clip = &d2scene.Clip{Path: clipRect(0, 0, 10, 10, d2scene.EvenOdd), Transform: d2scene.Identity()}

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
		{name: "RGBA layer plus streamed clip coverage and scanline scratch", node: clipped, want: 400 + scanlineScratch},
		{name: "RGBA layer plus even-odd Alpha clip and scanline scratch", node: evenOddClipped, want: 500 + scanlineScratch},
		{name: "RGBA layer plus RGBA mask and scanline scratch", node: masked, want: 800 + scanlineScratch},
		{name: "nested RGBA opacity layers plus scanline scratch", node: nested, want: 800 + scanlineScratch},
		{name: "mask with nested RGBA layer plus scanline scratch", node: nestedMask, want: 1200 + scanlineScratch},
		{name: "direct gradient streamed coverage plus scanline scratch", node: gradientNode, want: scanlineScratch},
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
	evenOddClipped := fullRect(red)
	evenOddClipped.Clip = &d2scene.Clip{Path: clipRect(0, 0, 10, 10, d2scene.EvenOdd), Transform: d2scene.Identity()}

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
	scanlineScratch, ok := scanline.RetainedBytes(10, 10, 2)
	if !ok {
		t.Fatal("scanline retained-byte calculation overflowed")
	}

	for _, test := range []struct {
		name string
		node *d2scene.Node
		peak int64
	}{
		{name: "clip effect layer allocation", node: clipped, peak: 400 + scanlineScratch},
		{name: "even-odd clip Alpha allocation", node: evenOddClipped, peak: 500 + scanlineScratch},
		{name: "mask RGBA allocation", node: masked, peak: 800 + scanlineScratch},
		{name: "nested opacity allocation", node: nestedOpacity, peak: 800 + scanlineScratch},
		{name: "nested mask-root allocation", node: nestedMask, peak: 1200 + scanlineScratch},
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
	// Streamed gradient coverage retains only the rasterizer's independent
	// width and height maxima.
	wantPeak := wantScratch
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
		t.Fatalf("whole-target aggregate = %d, %v; fixture must exceed the aggregate bound", int64(shapes)*worstPerShape, ok)
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

	t.Run("shared samples retain logical work and cancellation cadence", func(t *testing.T) {
		paths := []subpath{
			{points: make([]d2scene.Point, 3)},
			{points: make([]d2scene.Point, 5)},
			{points: make([]d2scene.Point, 1)},
		}
		var successfulEvaluations uint64
		_, err := countEvenOddPathSamples(
			context.Background(), paths,
			.125, .375, .625, .875,
			.125, .375, .625, .875,
			&successfulEvaluations,
		)
		if err != nil {
			t.Fatal(err)
		}
		const wantSuccessfulEvaluations = uint64((3 + 5) * 4 * 4)
		if successfulEvaluations != wantSuccessfulEvaluations {
			t.Fatalf("successful logical edge evaluations = %d, want %d", successfulEvaluations, wantSuccessfulEvaluations)
		}

		points := make([]d2scene.Point, 64)
		for index := range points {
			points[index] = d2scene.Point{X: float64(index), Y: float64(index & 1)}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var evaluations uint64
		_, err = countEvenOddPathSamples(
			ctx, []subpath{{points: points}},
			.125, .375, .625, .875,
			.5, .5, .5, .5,
			&evaluations,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("countEvenOddPathSamples() error = %v, want context.Canceled", err)
		}
		if evaluations != 256 {
			t.Fatalf("logical edge evaluations before cancellation = %d, want 256", evaluations)
		}
	})
}

func TestRasterizeEvenOddMaskMatchesIndependentSamples(t *testing.T) {
	state := uint64(0x9e3779b97f4a7c15)
	next := func() uint64 {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		return state
	}
	for testIndex := range 256 {
		width := 1 + int(next()%17)
		height := 1 + int(next()%15)
		minimum := image.Pt(int(next()%31)-15, int(next()%29)-14)
		bounds := image.Rectangle{Min: minimum, Max: minimum.Add(image.Pt(width, height))}
		stride := width + int(next()%8)
		pixelBytes := stride*height + int(next()%8)
		got := &image.Alpha{Pix: make([]uint8, pixelBytes), Stride: stride, Rect: bounds}
		want := &image.Alpha{Pix: make([]uint8, pixelBytes), Stride: stride, Rect: bounds}
		for index := range got.Pix {
			value := uint8(next())
			got.Pix[index] = value
			want.Pix[index] = value
		}

		origin := image.Pt(int(next()%65)-32, int(next()%61)-30)
		paths := make([]subpath, int(next()%7))
		for pathIndex := range paths {
			points := make([]d2scene.Point, int(next()%25))
			for pointIndex := range points {
				// Eighth-pixel coordinates deliberately put some crossings exactly
				// on the four horizontal and vertical sample positions.
				points[pointIndex] = d2scene.Point{
					X: float64(origin.X) + float64(int(next()%257)-128)/8,
					Y: float64(origin.Y) + float64(int(next()%241)-120)/8,
				}
			}
			paths[pathIndex] = subpath{points: points, closed: next()&1 != 0}
		}

		if err := rasterizeEvenOddMask(context.Background(), got, origin, paths); err != nil {
			t.Fatalf("case %d optimized rasterizer: %v", testIndex, err)
		}
		if err := rasterizeEvenOddMaskIndependentSamples(context.Background(), want, origin, paths); err != nil {
			t.Fatalf("case %d independent-sample rasterizer: %v", testIndex, err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			for index := range got.Pix {
				if got.Pix[index] != want.Pix[index] {
					t.Fatalf("case %d byte %d = %d, want %d (bounds %v, stride %d, origin %v)", testIndex, index, got.Pix[index], want.Pix[index], bounds, stride, origin)
				}
			}
		}
	}
}

func TestRasterizeEvenOddMaskCancellationCadenceMatchesIndependentSamples(t *testing.T) {
	points := make([]d2scene.Point, 128)
	for index := range points {
		points[index] = d2scene.Point{X: float64(index & 7), Y: float64(index) / 16}
	}
	paths := []subpath{{points: points}}
	const (
		width  = 3
		height = 3
		stride = 5
	)
	for _, cancelAt := range []int{1, 2, 3, 4, 7, 15, 31, 77, 78} {
		got := &image.Alpha{Pix: make([]uint8, stride*height+3), Stride: stride, Rect: image.Rect(-2, 4, -2+width, 4+height)}
		want := &image.Alpha{Pix: make([]uint8, len(got.Pix)), Stride: stride, Rect: got.Rect}
		for index := range got.Pix {
			got.Pix[index] = 0xa5
			want.Pix[index] = 0xa5
		}
		gotContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
		wantContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
		gotErr := rasterizeEvenOddMask(gotContext, got, image.Pt(-3, 5), paths)
		wantErr := rasterizeEvenOddMaskIndependentSamples(wantContext, want, image.Pt(-3, 5), paths)
		if errors.Is(gotErr, context.Canceled) != errors.Is(wantErr, context.Canceled) {
			t.Fatalf("cancel call %d errors differ: optimized %v, independent %v", cancelAt, gotErr, wantErr)
		}
		if gotContext.calls != wantContext.calls {
			t.Fatalf("cancel call %d Err calls = %d, want %d", cancelAt, gotContext.calls, wantContext.calls)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("cancel call %d left a different output prefix", cancelAt)
		}
	}
}

type cancelAfterErrCallsContext struct {
	context.Context
	cancelAt int
	calls    int
}

func (ctx *cancelAfterErrCallsContext) Err() error {
	ctx.calls++
	if ctx.calls >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func rasterizeEvenOddMaskIndependentSamples(ctx context.Context, mask *image.Alpha, origin image.Point, paths []subpath) error {
	const sampleCount = evenOddSamplesPerAxis * evenOddSamplesPerAxis
	var edgeEvaluations uint64
	for y := 0; y < mask.Bounds().Dy(); y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		row := y * mask.Stride
		for x := 0; x < mask.Bounds().Dx(); x++ {
			if x&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			inside := 0
			for sampleY := 0; sampleY < evenOddSamplesPerAxis; sampleY++ {
				py := float64(origin.Y+y) + (float64(sampleY)+0.5)/evenOddSamplesPerAxis
				for sampleX := 0; sampleX < evenOddSamplesPerAxis; sampleX++ {
					px := float64(origin.X+x) + (float64(sampleX)+0.5)/evenOddSamplesPerAxis
					isInside, err := pointInEvenOddPathIndependent(ctx, paths, px, py, &edgeEvaluations)
					if err != nil {
						return err
					}
					if isInside {
						inside++
					}
				}
			}
			mask.Pix[row+x] = uint8((inside*255 + sampleCount/2) / sampleCount)
		}
	}
	return ctx.Err()
}

func pointInEvenOddPathIndependent(ctx context.Context, paths []subpath, x, y float64, edgeEvaluations *uint64) (bool, error) {
	inside := false
	for _, path := range paths {
		if len(path.points) < 2 {
			continue
		}
		previous := path.points[len(path.points)-1]
		for _, current := range path.points {
			*edgeEvaluations = *edgeEvaluations + 1
			if *edgeEvaluations&255 == 0 {
				if err := ctx.Err(); err != nil {
					return false, err
				}
			}
			if (current.Y > y) != (previous.Y > y) &&
				x < (previous.X-current.X)*(y-current.Y)/(previous.Y-current.Y)+current.X {
				inside = !inside
			}
			previous = current
		}
	}
	return inside, nil
}

func BenchmarkRasterizeEvenOddMask(b *testing.B) {
	rectangle := []subpath{{points: []d2scene.Point{
		{X: 4.25, Y: 3.75},
		{X: 244.5, Y: 8.25},
		{X: 249.75, Y: 246.5},
		{X: 7.5, Y: 251.25},
	}}}
	star := make([]d2scene.Point, 128)
	for index := range star {
		radius := 30.0
		if index&1 != 0 {
			radius = 12
		}
		angle := 2 * math.Pi * float64(index) / float64(len(star))
		star[index] = d2scene.Point{
			X: 32 + radius*math.Cos(angle),
			Y: 32 + radius*math.Sin(angle),
		}
	}
	manyPaths := make([]subpath, 64)
	for index := range manyPaths {
		x := float64((index%16)*16) + 0.125
		y := float64((index/16)*16) + 0.375
		manyPaths[index].points = []d2scene.Point{
			{X: x, Y: y},
			{X: x + 11.5, Y: y + 0.25},
			{X: x + 11.75, Y: y + 11.5},
			{X: x + 0.25, Y: y + 11.75},
		}
	}
	tests := []struct {
		name   string
		bounds image.Rectangle
		paths  []subpath
	}{
		{name: "8x8_rectangle", bounds: image.Rect(0, 0, 8, 8), paths: rectangle},
		{name: "256x256_rectangle", bounds: image.Rect(0, 0, 256, 256), paths: rectangle},
		{name: "64x64_128_edge_star", bounds: image.Rect(0, 0, 64, 64), paths: []subpath{{points: star}}},
		{name: "256x64_64_paths", bounds: image.Rect(0, 0, 256, 64), paths: manyPaths},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			mask := image.NewAlpha(test.bounds)
			ctx := context.Background()
			b.ReportAllocs()
			b.SetBytes(int64(test.bounds.Dx() * test.bounds.Dy()))
			b.ResetTimer()
			for range b.N {
				if err := rasterizeEvenOddMask(ctx, mask, image.Point{}, test.paths); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
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
