package d2raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestBlendModeOpaqueReferenceVectors(t *testing.T) {
	t.Parallel()

	backdrop := color.RGBA{R: 64, G: 192, B: 255, A: 255}
	source := color.RGBA{R: 128, G: 64, B: 255, A: 255}
	tests := []struct {
		name string
		mode d2scene.BlendMode
		want color.RGBA
	}{
		{name: "multiply", mode: d2scene.BlendMultiply, want: color.RGBA{R: 32, G: 48, B: 255, A: 255}},
		{name: "darken", mode: d2scene.BlendDarken, want: color.RGBA{R: 64, G: 64, B: 255, A: 255}},
		{name: "color burn", mode: d2scene.BlendColorBurn, want: color.RGBA{R: 0, G: 4, B: 255, A: 255}},
		{name: "overlay", mode: d2scene.BlendOverlay, want: color.RGBA{R: 64, G: 161, B: 255, A: 255}},
		{name: "lighten", mode: d2scene.BlendLighten, want: color.RGBA{R: 128, G: 192, B: 255, A: 255}},
		{name: "COLRv1 soft light", mode: preparedCOLRv1SoftLight, want: color.RGBA{R: 42, G: 150, B: 255, A: 255}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
			layer := image.NewRGBA(dst.Bounds())
			dst.SetRGBA(0, 0, backdrop)
			layer.SetRGBA(0, 0, source)
			if err := compositeLayer(context.Background(), dst, layer, 1, test.mode); err != nil {
				t.Fatal(err)
			}
			if got := dst.RGBAAt(0, 0); got != test.want {
				t.Fatalf("%s pixel = %#v, want %#v", test.name, got, test.want)
			}
		})
	}
}

func TestBlendModePartialAlphaAndTransparentInputs(t *testing.T) {
	t.Parallel()

	t.Run("partial alpha multiply", func(t *testing.T) {
		dst := rgbaPixel(color.RGBA{R: 64, A: 255})
		layer := rgbaPixel(color.RGBA{R: 64, A: 128})
		if err := compositeLayer(context.Background(), dst, layer, 1, d2scene.BlendMultiply); err != nil {
			t.Fatal(err)
		}
		if got, want := dst.RGBAAt(0, 0), (color.RGBA{R: 48, A: 255}); got != want {
			t.Fatalf("partial multiply = %#v, want %#v", got, want)
		}
	})

	t.Run("transparent backdrop preserves source", func(t *testing.T) {
		source := color.RGBA{R: 64, G: 32, B: 16, A: 128}
		dst := rgbaPixel(color.RGBA{})
		layer := rgbaPixel(source)
		if err := compositeLayer(context.Background(), dst, layer, 1, d2scene.BlendOverlay); err != nil {
			t.Fatal(err)
		}
		if got := dst.RGBAAt(0, 0); got != source {
			t.Fatalf("transparent-backdrop result = %#v, want %#v", got, source)
		}
	})

	t.Run("transparent source preserves backdrop", func(t *testing.T) {
		backdrop := color.RGBA{R: 20, G: 40, B: 60, A: 80}
		dst := rgbaPixel(backdrop)
		if err := compositeLayer(context.Background(), dst, rgbaPixel(color.RGBA{}), 1, d2scene.BlendColorBurn); err != nil {
			t.Fatal(err)
		}
		if got := dst.RGBAAt(0, 0); got != backdrop {
			t.Fatalf("transparent-source result = %#v, want %#v", got, backdrop)
		}
	})

	t.Run("group opacity scales premultiplied source", func(t *testing.T) {
		dst := rgbaPixel(color.RGBA{})
		layer := rgbaPixel(color.RGBA{R: 64, G: 32, B: 16, A: 128})
		if err := compositeLayer(context.Background(), dst, layer, .5, d2scene.BlendDarken); err != nil {
			t.Fatal(err)
		}
		if got, want := dst.RGBAAt(0, 0), (color.RGBA{R: 32, G: 16, B: 8, A: 64}); got != want {
			t.Fatalf("half-opacity result = %#v, want %#v", got, want)
		}
	})
}

func TestCOLRv1SoftLightUsesLinearLightWithPremultipliedAlpha(t *testing.T) {
	t.Parallel()

	// These premultiplied input and output bytes independently pin the W3C
	// source-over/soft-light equations after the OpenType-required inverse and
	// forward sRGB transfer functions. Group opacity is quantized consistently
	// with the rest of the renderer before participating in alpha compositing.
	dst := rgbaPixel(color.RGBA{R: 16, G: 48, B: 80, A: 128})
	layer := rgbaPixel(color.RGBA{R: 96, G: 16, B: 48, A: 192})
	if err := compositeLayer(context.Background(), dst, layer, .37, preparedCOLRv1SoftLight); err != nil {
		t.Fatal(err)
	}
	if got, want := dst.RGBAAt(0, 0), (color.RGBA{R: 43, G: 48, B: 86, A: 163}); got != want {
		t.Fatalf("linear-light partial-alpha soft light = %#v, want %#v", got, want)
	}
}

func TestCOLRv1SoftLightCompositeIsolatedFromParent(t *testing.T) {
	t.Parallel()

	dst := rgbaPixel(color.RGBA{B: 255, A: 255})
	transparentBackdrop := preparedSolidPixelNode(color.NRGBA{}, d2scene.BlendNormal)
	source := preparedSolidPixelNode(color.NRGBA{R: 255, A: 128}, preparedCOLRv1SoftLight)
	composite := &preparedNode{
		opacity: 1, blend: d2scene.BlendNormal, isolated: true,
		children: []*preparedNode{transparentBackdrop, source},
		bounds:   image.Rect(0, 0, 1, 1), contentBounds: image.Rect(0, 0, 1, 1),
	}
	scratch := &rasterScratch{offscreen: offscreenBudget{limit: 1024}}
	if err := renderNode(context.Background(), dst, composite, scratch); err != nil {
		t.Fatal(err)
	}
	if got, want := dst.RGBAAt(0, 0), (color.RGBA{R: 128, B: 127, A: 255}); got != want {
		t.Fatalf("isolated soft-light result over parent = %#v, want %#v", got, want)
	}
}

func preparedSolidPixelNode(value color.NRGBA, blend d2scene.BlendMode) *preparedNode {
	bounds := image.Rect(0, 0, 1, 1)
	primitive := &preparedPrimitive{
		subpaths: []subpath{{
			points: []d2scene.Point{{}, {X: 1}, {X: 1, Y: 1}, {Y: 1}},
			closed: true,
		}},
		transform: d2scene.Identity(),
		fill:      &preparedPaint{kind: preparedSolidPaint, solid: value},
		fillRule:  d2scene.NonZero,
		bounds:    bounds,
	}
	return &preparedNode{opacity: 1, blend: blend, primitive: primitive, bounds: bounds, contentBounds: bounds}
}

func TestBlendSceneUsesIsolatedGroup(t *testing.T) {
	t.Parallel()

	background := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{Width: 10, Height: 10},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	})
	blended := d2scene.NewNode(nil)
	blended.Blend = d2scene.BlendMultiply
	blended.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{Width: 10, Height: 10},
			Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, A: 128}},
		}),
		d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{Width: 10, Height: 10},
			Fill: d2scene.SolidPaint{Color: color.NRGBA{G: 255, A: 128}},
		}),
	}
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{background, blended}
	document := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, root)
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	got := frame.NRGBAAt(5, 5)
	if got.R < 126 || got.R > 128 || got.G < 190 || got.G > 192 || got.B < 62 || got.B > 64 || got.A != 255 {
		t.Fatalf("isolated multiply group = %#v, want approximately {127 191 63 255}", got)
	}
	prepared, err := prepare(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.root.children[1].blend != d2scene.BlendMultiply || prepared.root.children[1].bounds.Empty() {
		t.Fatalf("prepared blend node = %+v", prepared.root.children[1])
	}
}

func TestBlendPremultipliedInvariantAndConcurrentDeterminism(t *testing.T) {
	t.Parallel()

	modes := []d2scene.BlendMode{
		d2scene.BlendMultiply, d2scene.BlendDarken, d2scene.BlendColorBurn,
		d2scene.BlendOverlay, d2scene.BlendLighten, preparedCOLRv1SoftLight,
	}
	values := []uint8{0, 31, 127, 255}
	for _, mode := range modes {
		for _, sourceAlpha := range values {
			for _, backdropAlpha := range values {
				for _, sourceValue := range values {
					for _, backdropValue := range values {
						sourceValue = min(sourceValue, sourceAlpha)
						backdropValue = min(backdropValue, backdropAlpha)
						dst := rgbaPixel(color.RGBA{R: backdropValue, G: backdropValue, B: backdropValue, A: backdropAlpha})
						layer := rgbaPixel(color.RGBA{R: sourceValue, G: sourceValue, B: sourceValue, A: sourceAlpha})
						if err := compositeLayer(context.Background(), dst, layer, .37, mode); err != nil {
							t.Fatal(err)
						}
						pixel := dst.RGBAAt(0, 0)
						if pixel.R > pixel.A || pixel.G > pixel.A || pixel.B > pixel.A {
							t.Fatalf("mode %d produced non-premultiplied pixel %#v", mode, pixel)
						}
					}
				}
			}
		}
	}

	node := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{Width: 32, Height: 24},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 240, G: 80, B: 30, A: 180}},
	})
	node.Blend = d2scene.BlendOverlay
	document := d2scene.NewDocument(d2scene.Box{Width: 32, Height: 24}, node)
	baseline, err := renderTestPNG(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	const renders = 12
	errs := make([]error, renders)
	var wait sync.WaitGroup
	for index := range renders {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			got, err := renderTestPNG(context.Background(), document, testOptions())
			if err == nil && !bytes.Equal(got, baseline) {
				err = errors.New("blend render differs from baseline")
			}
			errs[index] = err
		}(index)
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("concurrent blend render %d: %v", index, err)
		}
	}
}

func TestBlendCancellationAndInvalidMode(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := compositeLayer(ctx, image.NewRGBA(image.Rect(0, 0, 512, 512)), image.NewRGBA(image.Rect(0, 0, 512, 512)), 1, d2scene.BlendMultiply); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled blend error = %v, want context.Canceled", err)
	}
	if supportedBlendMode(preparedCOLRv1SoftLight) {
		t.Fatal("internal COLRv1 soft-light mode leaked into the public scene blend set")
	}
	if err := compositeLayer(context.Background(), rgbaPixel(color.RGBA{}), rgbaPixel(color.RGBA{}), 1, d2scene.BlendMode(254)); err == nil {
		t.Fatal("invalid blend mode unexpectedly succeeded")
	}
}

func rgbaPixel(value color.RGBA) *image.RGBA {
	image := image.NewRGBA(image.Rect(0, 0, 1, 1))
	image.SetRGBA(0, 0, value)
	return image
}
