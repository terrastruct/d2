package d2raster

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
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

func BenchmarkCompositeCOLRv1SoftLightOpaque(b *testing.B) {
	bounds := image.Rect(0, 0, 256, 256)
	dst := image.NewRGBA(bounds)
	layer := image.NewRGBA(bounds)
	for offset := 0; offset < len(dst.Pix); offset += 4 {
		dst.Pix[offset], dst.Pix[offset+1], dst.Pix[offset+2], dst.Pix[offset+3] = 17, 91, 203, 255
		layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] = 211, 67, 29, 255
	}
	b.SetBytes(int64(len(dst.Pix) + len(layer.Pix)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := compositeCOLRv1SoftLightLayer(context.Background(), dst, layer, .73); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompositeBlendLayer(b *testing.B) {
	modes := []struct {
		name string
		mode d2scene.BlendMode
	}{
		{name: "Multiply", mode: d2scene.BlendMultiply},
		{name: "Darken", mode: d2scene.BlendDarken},
		{name: "ColorBurn", mode: d2scene.BlendColorBurn},
		{name: "Overlay", mode: d2scene.BlendOverlay},
		{name: "Lighten", mode: d2scene.BlendLighten},
	}
	inputs := []struct {
		name          string
		sourceAlpha   byte
		backdropAlpha byte
		opacity       float64
	}{
		{name: "Opaque", sourceAlpha: 0xff, backdropAlpha: 0xff, opacity: 1},
		{name: "Transparent", sourceAlpha: 0, backdropAlpha: 227, opacity: 1},
		{name: "Translucent", sourceAlpha: 193, backdropAlpha: 227, opacity: 1},
		{name: "PartialOpacity", sourceAlpha: 0xff, backdropAlpha: 0xff, opacity: .73},
	}
	for _, mode := range modes {
		for _, input := range inputs {
			for _, implementation := range []struct {
				name string
				run  func(context.Context, *image.RGBA, *image.RGBA, float64, d2scene.BlendMode) error
			}{
				{name: "Optimized", run: compositeLayer},
				{name: "Scalar", run: referenceCompositeBlendLayer},
			} {
				b.Run(mode.name+"/"+input.name+"/"+implementation.name, func(b *testing.B) {
					bounds := image.Rect(0, 0, 512, 512)
					destination := image.NewRGBA(bounds)
					layer := image.NewRGBA(bounds)
					for offset := 0; offset < len(layer.Pix); offset += 4 {
						pixel := offset / 4
						destination.Pix[offset], destination.Pix[offset+1], destination.Pix[offset+2], destination.Pix[offset+3] =
							min(byte(17+pixel%181), input.backdropAlpha), min(byte(31+pixel%149), input.backdropAlpha), min(byte(47+pixel%127), input.backdropAlpha), input.backdropAlpha
						layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] =
							min(byte(211-pixel%173), input.sourceAlpha), min(byte(179-pixel%137), input.sourceAlpha), min(byte(139-pixel%103), input.sourceAlpha), input.sourceAlpha
					}
					b.SetBytes(int64(len(destination.Pix) + len(layer.Pix)))
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						if err := implementation.run(context.Background(), destination, layer, input.opacity, mode.mode); err != nil {
							b.Fatal(err)
						}
					}
					benchmarkCompositeLayer = destination
				})
			}
		}
	}
}

func TestCompositeBlendOpaquePrefixMatchesFloatExhaustively(t *testing.T) {
	t.Parallel()
	for _, mode := range []d2scene.BlendMode{
		d2scene.BlendMultiply,
		d2scene.BlendDarken,
		d2scene.BlendColorBurn,
		d2scene.BlendOverlay,
		d2scene.BlendLighten,
	} {
		bounds := image.Rect(0, 0, 256, 256)
		got := image.NewRGBA(bounds)
		layer := image.NewRGBA(bounds)
		for source := range 256 {
			for backdrop := range 256 {
				offset := got.PixOffset(backdrop, source)
				got.Pix[offset], got.Pix[offset+1], got.Pix[offset+2], got.Pix[offset+3] = uint8(backdrop), uint8(source), uint8(backdrop^source), 255
				layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] = uint8(source), uint8(255-backdrop), uint8(source^0xa5), 255
			}
		}
		want := &image.RGBA{Pix: bytes.Clone(got.Pix), Stride: got.Stride, Rect: got.Rect}
		if err := referenceCompositeBlendLayer(context.Background(), want, layer, 1, mode); err != nil {
			t.Fatal(err)
		}
		if err := compositeLayer(context.Background(), got, layer, 1, mode); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			for index := range got.Pix {
				if got.Pix[index] != want.Pix[index] {
					t.Fatalf("mode=%d byte=%d: opaque-prefix=%d float=%d", mode, index, got.Pix[index], want.Pix[index])
				}
			}
		}
	}
}

func TestCompositeBlendOpaquePrefixMatchesScalarOnMixedRows(t *testing.T) {
	t.Parallel()
	const width, height = 2*4096 + 17, 5
	bounds := image.Rect(-13, 29, -13+width, 29+height)
	stride := width*4 + 19
	initialDestination := make([]byte, stride*height+11)
	initialLayer := make([]byte, stride*height+11)
	for index := range initialDestination {
		initialDestination[index] = byte(index*73 + 29)
		initialLayer[index] = byte(index*41 + 17)
	}
	for y := range height {
		for x := range width {
			offset := y*stride + x*4
			initialDestination[offset+3] = 255
			initialLayer[offset+3] = 255
		}
	}
	// Exercise a late source mismatch, a cancellation-boundary backdrop
	// mismatch, a row that starts partial, and a transparent tail.
	initialLayer[1*stride+63*4+3] = 173
	initialLayer[1*stride+63*4+0] = min(initialLayer[1*stride+63*4+0], byte(173))
	initialLayer[1*stride+63*4+1] = min(initialLayer[1*stride+63*4+1], byte(173))
	initialLayer[1*stride+63*4+2] = min(initialLayer[1*stride+63*4+2], byte(173))
	initialDestination[2*stride+4096*4+3] = 201
	initialDestination[2*stride+4096*4+0] = min(initialDestination[2*stride+4096*4+0], byte(201))
	initialDestination[2*stride+4096*4+1] = min(initialDestination[2*stride+4096*4+1], byte(201))
	initialDestination[2*stride+4096*4+2] = min(initialDestination[2*stride+4096*4+2], byte(201))
	initialLayer[3*stride+3] = 211
	initialLayer[3*stride+0] = min(initialLayer[3*stride+0], byte(211))
	initialLayer[3*stride+1] = min(initialLayer[3*stride+1], byte(211))
	initialLayer[3*stride+2] = min(initialLayer[3*stride+2], byte(211))
	for x := width - 97; x < width; x++ {
		offset := 4*stride + x*4
		initialLayer[offset], initialLayer[offset+1], initialLayer[offset+2], initialLayer[offset+3] = 0, 0, 0, 0
	}

	for _, mode := range []d2scene.BlendMode{
		d2scene.BlendMultiply,
		d2scene.BlendDarken,
		d2scene.BlendColorBurn,
		d2scene.BlendOverlay,
		d2scene.BlendLighten,
	} {
		got := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
		want := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
		layer := &image.RGBA{Pix: bytes.Clone(initialLayer), Stride: stride, Rect: bounds}
		if err := compositeLayer(context.Background(), got, layer, 1, mode); err != nil {
			t.Fatal(err)
		}
		if err := referenceCompositeBlendLayer(context.Background(), want, layer, 1, mode); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("mode=%d mixed-row opaque-prefix output differs, including padding/trailing bytes", mode)
		}
	}
}

func TestCompositeBlendOpaquePrefixMatchesScalarCancellation(t *testing.T) {
	const width, height = 2*4096 + 17, 2
	bounds := image.Rect(7, -5, 7+width, -5+height)
	stride := width*4 + 13
	initialDestination := make([]byte, stride*height+7)
	initialLayer := make([]byte, stride*height+7)
	for index := range initialDestination {
		initialDestination[index] = byte(index*23 + 101)
		initialLayer[index] = byte(index*47 + 53)
	}
	for y := range height {
		for x := range width {
			offset := y*stride + x*4
			initialDestination[offset+3] = 255
			initialLayer[offset+3] = 255
		}
	}
	// The optimized prefix must hand this exact cancellation boundary to the
	// scalar remainder without adding or losing an Err call.
	initialLayer[4096*4+3] = 173
	for channel := range 3 {
		initialLayer[4096*4+channel] = min(initialLayer[4096*4+channel], byte(173))
	}

	for _, mode := range []d2scene.BlendMode{
		d2scene.BlendMultiply,
		d2scene.BlendDarken,
		d2scene.BlendColorBurn,
		d2scene.BlendOverlay,
		d2scene.BlendLighten,
	} {
		for cancelAt := 1; cancelAt <= 9; cancelAt++ {
			got := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
			want := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
			layer := &image.RGBA{Pix: bytes.Clone(initialLayer), Stride: stride, Rect: bounds}
			gotContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
			wantContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
			gotErr := compositeLayer(gotContext, got, layer, 1, mode)
			wantErr := referenceCompositeBlendLayer(wantContext, want, layer, 1, mode)
			if errors.Is(gotErr, context.Canceled) != errors.Is(wantErr, context.Canceled) {
				t.Fatalf("mode=%d cancelAt=%d errors differ: optimized=%v scalar=%v", mode, cancelAt, gotErr, wantErr)
			}
			if gotContext.calls != wantContext.calls {
				t.Fatalf("mode=%d cancelAt=%d Err calls=%d, want %d", mode, cancelAt, gotContext.calls, wantContext.calls)
			}
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("mode=%d cancelAt=%d output prefix differs", mode, cancelAt)
			}
		}
	}
}

func TestCompositeBlendTransparentPrefixMatchesScalarCancellation(t *testing.T) {
	const width, height = 2*4096 + 17, 2
	bounds := image.Rect(-17, 23, -17+width, 23+height)
	stride := width*4 + 15
	initialDestination := make([]byte, stride*height+9)
	initialLayer := make([]byte, stride*height+9)
	for index := range initialDestination {
		initialDestination[index] = byte(index*31 + 79)
		initialLayer[index] = byte(index*43 + 11)
	}
	for y := range height {
		for x := range width {
			offset := y*stride + x*4
			initialDestination[offset+3] = 227
			for channel := range 3 {
				initialDestination[offset+channel] = min(initialDestination[offset+channel], byte(227))
				if y != 0 {
					initialLayer[offset+channel] = 0
				}
			}
			if y == 0 {
				initialLayer[offset+3] = 255
			} else {
				initialLayer[offset+3] = 0
			}
		}
	}
	// The opaque first row selects the optimized compositor. On its second row,
	// end the transparent prefix exactly where the next cancellation check
	// belongs.
	partial := stride + 4096*4
	initialLayer[partial], initialLayer[partial+1], initialLayer[partial+2], initialLayer[partial+3] = 71, 29, 113, 173

	for _, mode := range []d2scene.BlendMode{
		d2scene.BlendMultiply,
		d2scene.BlendDarken,
		d2scene.BlendColorBurn,
		d2scene.BlendOverlay,
		d2scene.BlendLighten,
	} {
		for cancelAt := 1; cancelAt <= 9; cancelAt++ {
			got := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
			want := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
			layer := &image.RGBA{Pix: bytes.Clone(initialLayer), Stride: stride, Rect: bounds}
			gotContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
			wantContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
			gotErr := compositeLayer(gotContext, got, layer, 1, mode)
			wantErr := referenceCompositeBlendLayer(wantContext, want, layer, 1, mode)
			if errors.Is(gotErr, context.Canceled) != errors.Is(wantErr, context.Canceled) {
				t.Fatalf("mode=%d cancelAt=%d errors differ: optimized=%v scalar=%v", mode, cancelAt, gotErr, wantErr)
			}
			if gotContext.calls != wantContext.calls {
				t.Fatalf("mode=%d cancelAt=%d Err calls=%d, want %d", mode, cancelAt, gotContext.calls, wantContext.calls)
			}
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("mode=%d cancelAt=%d transparent-prefix output differs", mode, cancelAt)
			}
		}
	}
}

func TestCompositeLayerOverOpaquePartialPrefixMatchesScalarExhaustively(t *testing.T) {
	t.Parallel()
	for _, opacityByte := range []int{1, 73, 127, 186, 254} {
		bounds := image.Rect(0, 0, 256, 256)
		got := image.NewRGBA(bounds)
		layer := image.NewRGBA(bounds)
		for source := range 256 {
			for destination := range 256 {
				offset := got.PixOffset(destination, source)
				got.Pix[offset], got.Pix[offset+1], got.Pix[offset+2], got.Pix[offset+3] =
					byte(destination), byte(source), byte(destination^source), 255
				layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] =
					byte(source), byte(255-destination), byte(source^0xa5), 255
			}
		}
		want := &image.RGBA{Pix: bytes.Clone(got.Pix), Stride: got.Stride, Rect: got.Rect}
		opacity := float64(opacityByte) / 255
		if err := referenceCompositeLayerOver(context.Background(), want, layer, opacity); err != nil {
			t.Fatal(err)
		}
		if err := compositeLayer(context.Background(), got, layer, opacity, d2scene.BlendNormal); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			for index := range got.Pix {
				if got.Pix[index] != want.Pix[index] {
					t.Fatalf("opacity=%d byte=%d: opaque-prefix=%d scalar=%d", opacityByte, index, got.Pix[index], want.Pix[index])
				}
			}
		}
	}
}

func TestCompositeLayerOverOpaquePartialPrefixMatchesScalarOnMixedRows(t *testing.T) {
	t.Parallel()
	const width, height = 2*4096 + 19, 4
	bounds := image.Rect(-11, 23, -11+width, 23+height)
	stride := width*4 + 17
	initialDestination := make([]byte, stride*height+13)
	initialLayer := make([]byte, stride*height+13)
	for index := range initialDestination {
		initialDestination[index] = byte(index*73 + 29)
		initialLayer[index] = byte(index*41 + 17)
	}
	for y := range height {
		for x := range width {
			offset := y*stride + x*4
			initialDestination[offset+3] = 255
			initialLayer[offset+3] = 255
		}
	}
	// Cover a short opaque prefix, a mismatch exactly on a cancellation
	// boundary, and a row whose scalar remainder starts immediately.
	initialLayer[1*stride+63*4+3] = 173
	initialDestination[2*stride+4096*4+3] = 201
	initialLayer[3*stride+3] = 211
	for _, pixel := range []struct {
		bytes []byte
		base  int
		alpha byte
	}{
		{bytes: initialLayer, base: 1*stride + 63*4, alpha: 173},
		{bytes: initialDestination, base: 2*stride + 4096*4, alpha: 201},
		{bytes: initialLayer, base: 3 * stride, alpha: 211},
	} {
		for channel := range 3 {
			pixel.bytes[pixel.base+channel] = min(pixel.bytes[pixel.base+channel], pixel.alpha)
		}
	}

	got := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
	want := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
	layer := &image.RGBA{Pix: bytes.Clone(initialLayer), Stride: stride, Rect: bounds}
	const opacity = 0.73
	if err := compositeLayer(context.Background(), got, layer, opacity, d2scene.BlendNormal); err != nil {
		t.Fatal(err)
	}
	if err := referenceCompositeLayerOver(context.Background(), want, layer, opacity); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("mixed-row partial-opacity opaque-prefix output differs, including padding/trailing bytes")
	}
}

func TestCompositeLayerOverOpaquePartialPrefixMatchesScalarCancellation(t *testing.T) {
	const width, height = 2*4096 + 17, 2
	bounds := image.Rect(7, -5, 7+width, -5+height)
	stride := width*4 + 13
	initialDestination := make([]byte, stride*height+7)
	initialLayer := make([]byte, stride*height+7)
	for index := range initialDestination {
		initialDestination[index] = byte(index*23 + 101)
		initialLayer[index] = byte(index*47 + 53)
	}
	for y := range height {
		for x := range width {
			offset := y*stride + x*4
			initialDestination[offset+3] = 255
			initialLayer[offset+3] = 255
		}
	}
	initialLayer[4096*4+3] = 173
	for channel := range 3 {
		initialLayer[4096*4+channel] = min(initialLayer[4096*4+channel], byte(173))
	}

	for cancelAt := 1; cancelAt <= 9; cancelAt++ {
		got := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
		want := &image.RGBA{Pix: bytes.Clone(initialDestination), Stride: stride, Rect: bounds}
		layer := &image.RGBA{Pix: bytes.Clone(initialLayer), Stride: stride, Rect: bounds}
		gotContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
		wantContext := &cancelAfterErrCallsContext{Context: context.Background(), cancelAt: cancelAt}
		gotErr := compositeLayer(gotContext, got, layer, .73, d2scene.BlendNormal)
		wantErr := referenceCompositeLayerOver(wantContext, want, layer, .73)
		if errors.Is(gotErr, context.Canceled) != errors.Is(wantErr, context.Canceled) {
			t.Fatalf("cancelAt=%d errors differ: optimized=%v scalar=%v", cancelAt, gotErr, wantErr)
		}
		if gotContext.calls != wantContext.calls {
			t.Fatalf("cancelAt=%d Err calls=%d, want %d", cancelAt, gotContext.calls, wantContext.calls)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("cancelAt=%d output prefix differs", cancelAt)
		}
	}
}

func BenchmarkCompositeLayerOverOpaquePartialPrefix(b *testing.B) {
	for _, input := range []struct {
		name         string
		partialFirst bool
	}{
		{name: "OpaquePrefix"},
		{name: "ScalarFallback", partialFirst: true},
	} {
		for _, implementation := range []struct {
			name string
			run  func(context.Context, *image.RGBA, *image.RGBA, float64) error
		}{
			{name: "Optimized", run: func(ctx context.Context, destination, layer *image.RGBA, opacity float64) error {
				return compositeLayer(ctx, destination, layer, opacity, d2scene.BlendNormal)
			}},
			{name: "Scalar", run: compositeLayerOver},
		} {
			b.Run(input.name+"/"+implementation.name, func(b *testing.B) {
				bounds := image.Rect(0, 0, 512, 512)
				destination := image.NewRGBA(bounds)
				layer := image.NewRGBA(bounds)
				for offset := 0; offset < len(layer.Pix); offset += 4 {
					pixel := offset / 4
					destination.Pix[offset], destination.Pix[offset+1], destination.Pix[offset+2], destination.Pix[offset+3] =
						byte(17+pixel%181), byte(31+pixel%149), byte(47+pixel%127), 255
					layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] =
						byte(211-pixel%173), byte(179-pixel%137), byte(139-pixel%103), 255
				}
				if input.partialFirst {
					layer.Pix[0], layer.Pix[1], layer.Pix[2], layer.Pix[3] = 71, 29, 113, 173
				}
				b.SetBytes(int64(len(destination.Pix) + len(layer.Pix)))
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if err := implementation.run(context.Background(), destination, layer, .73); err != nil {
						b.Fatal(err)
					}
				}
				benchmarkCompositeLayer = destination
			})
		}
	}
}

func TestCompositeLayerOverOpaqueFullOpacityMatchesReferenceArithmeticExhaustively(t *testing.T) {
	t.Parallel()

	bounds := image.Rect(0, 0, 256, 256)
	destination := image.NewRGBA(bounds)
	layer := image.NewRGBA(bounds)
	for source := 0; source <= 0xff; source++ {
		for backdrop := 0; backdrop <= 0xff; backdrop++ {
			offset := layer.PixOffset(backdrop, source)
			layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] = uint8(source), uint8(255-source), uint8(source^0x55), 0xff
			destination.Pix[offset], destination.Pix[offset+1], destination.Pix[offset+2], destination.Pix[offset+3] = uint8(backdrop), uint8(255-backdrop), uint8(backdrop^0xaa), uint8(source^backdrop)
		}
	}
	want := image.NewRGBA(bounds)
	copy(want.Pix, destination.Pix)
	if err := referenceCompositeLayerOverFullOpacity(context.Background(), want, layer); err != nil {
		t.Fatal(err)
	}
	if err := compositeLayerOver(context.Background(), destination, layer, 1); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(destination.Pix, want.Pix) {
		t.Fatal("opaque full-opacity compositing differs from reference integer arithmetic")
	}
}

func TestCompositeLayerOverUniformPrefixesMatchPerPixel(t *testing.T) {
	t.Parallel()

	bounds := image.Rect(0, 0, 128, 124)
	destination := image.NewRGBA(bounds)
	layer := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		baseAlpha := uint8(0)
		if y >= 62 {
			baseAlpha = 255
		}
		mismatch := 1 + y%62
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			offset := layer.PixOffset(x, y)
			alpha := baseAlpha
			if x%64 == mismatch {
				alpha = 173
			}
			destination.Pix[offset], destination.Pix[offset+1], destination.Pix[offset+2], destination.Pix[offset+3] = uint8(x), uint8(y), uint8(x^y), 239
			layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] = min(uint8(149), alpha), min(uint8(83), alpha), min(uint8(31), alpha), alpha
		}
	}
	want := image.NewRGBA(bounds)
	copy(want.Pix, destination.Pix)
	if err := referenceCompositeLayerOverFullOpacity(context.Background(), want, layer); err != nil {
		t.Fatal(err)
	}
	if err := compositeLayerOver(context.Background(), destination, layer, 1); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(destination.Pix, want.Pix) {
		t.Fatal("uniform-prefix compositing differs from the per-pixel path")
	}
}

func BenchmarkCompositeLayerOver(b *testing.B) {
	tests := []struct {
		name        string
		sourceAlpha uint8
		opacity     float64
		pattern     string
	}{
		{name: "OpaqueFullOpacity", sourceAlpha: 255, opacity: 1},
		{name: "TransparentFullOpacity", sourceAlpha: 0, opacity: 1},
		{name: "TranslucentFullOpacity", sourceAlpha: 173, opacity: 1},
		{name: "OpaquePartialOpacity", sourceAlpha: 255, opacity: .73},
		{name: "AlternatingFullOpacity", opacity: 1, pattern: "alternating"},
		{name: "OpaqueEdgesWithCenterHole", opacity: 1, pattern: "center-hole"},
		{name: "LatePartialEveryBlock", opacity: 1, pattern: "late-partial"},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			bounds := image.Rect(0, 0, 512, 512)
			destination := image.NewRGBA(bounds)
			layer := image.NewRGBA(bounds)
			for offset := 0; offset < len(layer.Pix); offset += 4 {
				alpha := test.sourceAlpha
				pixel := offset / 4
				switch test.pattern {
				case "alternating":
					alpha = 255
					if pixel&1 != 0 {
						alpha = 173
					}
				case "center-hole":
					alpha = 255
					if pixel%bounds.Dx() == bounds.Dx()/2 {
						alpha = 173
					}
				case "late-partial":
					alpha = 255
					if pixel%64 == 62 {
						alpha = 173
					}
				}
				destination.Pix[offset], destination.Pix[offset+1], destination.Pix[offset+2], destination.Pix[offset+3] = 17, 91, 203, 239
				layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] = min(211, alpha), min(67, alpha), min(29, alpha), alpha
			}
			b.SetBytes(int64(len(destination.Pix) + len(layer.Pix)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := compositeLayerOver(context.Background(), destination, layer, test.opacity); err != nil {
					b.Fatal(err)
				}
			}
			benchmarkCompositeLayer = destination
		})
	}
}

func BenchmarkCompositeLayerOverLatePartialProbe(b *testing.B) {
	bounds := image.Rect(0, 0, 512, 512)
	destination := image.NewRGBA(bounds)
	layer := image.NewRGBA(bounds)
	for offset := 0; offset < len(layer.Pix); offset += 4 {
		alpha := uint8(255)
		if offset/4%64 == 62 {
			alpha = 173
		}
		destination.Pix[offset], destination.Pix[offset+1], destination.Pix[offset+2], destination.Pix[offset+3] = 17, 91, 203, 239
		layer.Pix[offset], layer.Pix[offset+1], layer.Pix[offset+2], layer.Pix[offset+3] = min(211, alpha), min(67, alpha), min(29, alpha), alpha
	}
	b.SetBytes(int64(len(destination.Pix) + len(layer.Pix)))
	b.Run("BlockProbe", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if err := compositeLayerOver(context.Background(), destination, layer, 1); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("PerPixel", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			if err := referenceCompositeLayerOverFullOpacity(context.Background(), destination, layer); err != nil {
				b.Fatal(err)
			}
		}
	})
	benchmarkCompositeLayer = destination
}

func referenceCompositeBlendLayer(ctx context.Context, destination, layer *image.RGBA, opacity float64, mode d2scene.BlendMode) error {
	bounds := layer.Bounds().Intersect(destination.Bounds())
	if bounds.Empty() || opacity == 0 {
		return nil
	}
	opacityByte := math.Round(opacity * 255)
	opacityScale := opacityByte / 255
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		destinationOffset := destination.PixOffset(bounds.Min.X, y)
		sourceOffset := layer.PixOffset(bounds.Min.X, y)
		for x := 0; x < bounds.Dx(); x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			si := sourceOffset + x*4
			di := destinationOffset + x*4
			sourceAlphaStored := float64(layer.Pix[si+3]) / 255
			sourceAlpha := sourceAlphaStored * opacityScale
			if sourceAlpha == 0 {
				continue
			}
			backdropAlpha := float64(destination.Pix[di+3]) / 255
			outputAlpha := sourceAlpha + backdropAlpha*(1-sourceAlpha)
			outputAlphaByte := roundedByte(outputAlpha * 255)
			for channel := range 3 {
				sourcePremultiplied := float64(layer.Pix[si+channel]) / 255 * opacityScale
				backdropPremultiplied := float64(destination.Pix[di+channel]) / 255
				sourceColor := float64(layer.Pix[si+channel]) / float64(layer.Pix[si+3])
				backdropColor := 0.0
				if destination.Pix[di+3] != 0 {
					backdropColor = float64(destination.Pix[di+channel]) / float64(destination.Pix[di+3])
				}
				mixed := blendComponent(mode, backdropColor, sourceColor)
				outputPremultiplied := sourcePremultiplied*(1-backdropAlpha) +
					sourceAlpha*backdropAlpha*mixed +
					backdropPremultiplied*(1-sourceAlpha)
				value := roundedByte(outputPremultiplied * 255)
				if value > outputAlphaByte {
					value = outputAlphaByte
				}
				destination.Pix[di+channel] = value
			}
			destination.Pix[di+3] = outputAlphaByte
		}
	}
	return ctx.Err()
}

func referenceCompositeLayerOverFullOpacity(ctx context.Context, destination, layer *image.RGBA) error {
	bounds := layer.Bounds().Intersect(destination.Bounds())
	mul255 := func(left, right uint32) uint32 { return (left*right + 127) / 255 }
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		destinationOffset := destination.PixOffset(bounds.Min.X, y)
		sourceOffset := layer.PixOffset(bounds.Min.X, y)
		for x := 0; x < bounds.Dx(); x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			si := sourceOffset + x*4
			di := destinationOffset + x*4
			sourceAlpha := uint32(layer.Pix[si+3])
			if sourceAlpha == 0 {
				continue
			}
			if sourceAlpha == 0xff {
				destination.Pix[di], destination.Pix[di+1], destination.Pix[di+2], destination.Pix[di+3] = layer.Pix[si], layer.Pix[si+1], layer.Pix[si+2], 0xff
				continue
			}
			inverseAlpha := 255 - sourceAlpha
			for channel := 0; channel < 3; channel++ {
				value := uint32(layer.Pix[si+channel]) + mul255(uint32(destination.Pix[di+channel]), inverseAlpha)
				if value > 255 {
					value = 255
				}
				destination.Pix[di+channel] = uint8(value)
			}
			alpha := sourceAlpha + mul255(uint32(destination.Pix[di+3]), inverseAlpha)
			if alpha > 255 {
				alpha = 255
			}
			destination.Pix[di+3] = uint8(alpha)
		}
	}
	return ctx.Err()
}

func referenceCompositeLayerOver(ctx context.Context, destination, layer *image.RGBA, opacity float64) error {
	bounds := layer.Bounds().Intersect(destination.Bounds())
	if bounds.Empty() || opacity == 0 {
		return nil
	}
	opacityByte := uint32(math.Round(opacity * 255))
	mul255 := func(left, right uint32) uint32 { return (left*right + 127) / 255 }
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		destinationOffset := destination.PixOffset(bounds.Min.X, y)
		sourceOffset := layer.PixOffset(bounds.Min.X, y)
		for x := 0; x < bounds.Dx(); x++ {
			if x&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			si := sourceOffset + x*4
			di := destinationOffset + x*4
			sourceAlpha := mul255(uint32(layer.Pix[si+3]), opacityByte)
			if sourceAlpha == 0 {
				continue
			}
			inverseAlpha := 255 - sourceAlpha
			for channel := range 3 {
				source := mul255(uint32(layer.Pix[si+channel]), opacityByte)
				value := source + mul255(uint32(destination.Pix[di+channel]), inverseAlpha)
				if value > 255 {
					value = 255
				}
				destination.Pix[di+channel] = uint8(value)
			}
			alpha := sourceAlpha + mul255(uint32(destination.Pix[di+3]), inverseAlpha)
			if alpha > 255 {
				alpha = 255
			}
			destination.Pix[di+3] = uint8(alpha)
		}
	}
	return ctx.Err()
}

var benchmarkCompositeLayer *image.RGBA

func rgbaPixel(value color.RGBA) *image.RGBA {
	image := image.NewRGBA(image.Rect(0, 0, 1, 1))
	image.SetRGBA(0, 0, value)
	return image
}
