package d2raster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestGaussianBlurPixelsAndDegenerateIdentity(t *testing.T) {
	t.Parallel()

	makeNode := func(filter d2scene.Filter) *d2scene.Node {
		node := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{X: 6, Y: 6, Width: 12, Height: 12},
			Fill: red,
		})
		if filter != nil {
			node.Filters = []d2scene.Filter{filter}
		}
		return node
	}
	document := func(node *d2scene.Node) *d2scene.Document {
		return d2scene.NewDocument(d2scene.Box{Width: 24, Height: 24}, node)
	}

	baseline, err := renderTestPNG(context.Background(), document(makeNode(nil)), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	zero := &d2scene.GaussianBlur{}
	identity, err := renderTestPNG(context.Background(), document(makeNode(zero)), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(identity, baseline) {
		t.Fatal("zero-deviation Gaussian blur changed the frame")
	}

	blurred, err := Render(context.Background(), document(makeNode(d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1})), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, blurred.NRGBAAt(12, 12), color.NRGBA{R: 255, A: 255})
	if got := blurred.NRGBAAt(5, 12); got.R != 255 || got.A == 0 || got.A == 255 {
		t.Fatalf("Gaussian fringe pixel = %#v, want translucent red", got)
	}
	assertPixel(t, blurred.NRGBAAt(2, 12), color.NRGBA{})

	horizontal, err := Render(context.Background(), document(makeNode(d2scene.GaussianBlur{SigmaX: 1})), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if horizontal.NRGBAAt(5, 12).A == 0 {
		t.Fatal("horizontal Gaussian blur did not spread horizontally")
	}
	assertPixel(t, horizontal.NRGBAAt(12, 5), color.NRGBA{})

	edgeNode := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 12, Height: 12, Y: 6}, Fill: red,
	})
	edgeNode.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1}}
	edge, err := Render(context.Background(), document(edgeNode), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := edge.NRGBAAt(0, 12), blurred.NRGBAAt(6, 12); got != want {
		t.Fatalf("viewport-edge Gaussian pixel = %#v, want translation-invariant %#v", got, want)
	}
}

func TestBoxBlurMatchesDirectConvolution(t *testing.T) {
	t.Parallel()

	sourceBounds := image.Rect(-3, 2, 8, 9)
	rgbaParent := image.NewRGBA(image.Rect(-7, -2, 13, 13))
	rgbaSource := rgbaParent.SubImage(sourceBounds).(*image.RGBA)
	alphaParent := image.NewAlpha(image.Rect(-7, -2, 13, 13))
	alphaSource := alphaParent.SubImage(sourceBounds).(*image.Alpha)
	for y := sourceBounds.Min.Y; y < sourceBounds.Max.Y; y++ {
		for x := sourceBounds.Min.X; x < sourceBounds.Max.X; x++ {
			alpha := uint8(64 + (x-sourceBounds.Min.X)*7 + (y-sourceBounds.Min.Y)*5)
			offset := rgbaSource.PixOffset(x, y)
			rgbaSource.Pix[offset] = alpha / 2
			rgbaSource.Pix[offset+1] = alpha / 3
			rgbaSource.Pix[offset+2] = alpha / 4
			rgbaSource.Pix[offset+3] = alpha
			alphaSource.Pix[alphaSource.PixOffset(x, y)] = alpha
		}
	}

	for _, test := range []struct {
		name   string
		axis   blurAxis
		bounds image.Rectangle
	}{
		{name: "horizontal padded rows", axis: blurHorizontal, bounds: image.Rect(-6, 0, 11, 11)},
		{name: "vertical padded columns", axis: blurVertical, bounds: image.Rect(-6, -1, 11, 12)},
	} {
		for _, radius := range []int{1, 2, 5} {
			t.Run(fmt.Sprintf("%s/radius=%d", test.name, radius), func(t *testing.T) {
				pass := blurPass{axis: test.axis, radius: radius, bounds: test.bounds}

				rgbaDestination := paddedRGBA(test.bounds)
				if err := boxBlurRGBA(context.Background(), rgbaDestination, rgbaSource, pass); err != nil {
					t.Fatal(err)
				}
				rgbaReference := directBlurRGBA(test.bounds, rgbaSource, pass)
				assertRawRGBAEqual(t, rgbaDestination, rgbaReference)

				fromRGBADestination := paddedAlpha(test.bounds)
				if err := boxBlurAlphaFromRGBA(context.Background(), fromRGBADestination, rgbaSource, pass); err != nil {
					t.Fatal(err)
				}
				fromRGBAReference := directBlurAlpha(test.bounds, rgbaSource.Bounds(), func(x, y int) uint8 {
					return rgbaSource.Pix[rgbaSource.PixOffset(x, y)+3]
				}, pass)
				assertRawAlphaEqual(t, fromRGBADestination, fromRGBAReference)

				alphaDestination := paddedAlpha(test.bounds)
				if err := boxBlurAlpha(context.Background(), alphaDestination, alphaSource, pass); err != nil {
					t.Fatal(err)
				}
				alphaReference := directBlurAlpha(test.bounds, alphaSource.Bounds(), func(x, y int) uint8 {
					return alphaSource.Pix[alphaSource.PixOffset(x, y)]
				}, pass)
				assertRawAlphaEqual(t, alphaDestination, alphaReference)
			})
		}
	}
}

func paddedRGBA(bounds image.Rectangle) *image.RGBA {
	parent := image.NewRGBA(image.Rect(bounds.Min.X-2, bounds.Min.Y-2, bounds.Max.X+3, bounds.Max.Y+3))
	return parent.SubImage(bounds).(*image.RGBA)
}

func paddedAlpha(bounds image.Rectangle) *image.Alpha {
	parent := image.NewAlpha(image.Rect(bounds.Min.X-2, bounds.Min.Y-2, bounds.Max.X+3, bounds.Max.Y+3))
	return parent.SubImage(bounds).(*image.Alpha)
}

func directBlurRGBA(bounds image.Rectangle, source *image.RGBA, pass blurPass) *image.RGBA {
	destination := image.NewRGBA(bounds)
	window := int64(pass.radius)*2 + 1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var sums [4]int64
			for delta := -pass.radius; delta <= pass.radius; delta++ {
				sampleX, sampleY := x, y
				if pass.axis == blurHorizontal {
					sampleX += delta
				} else {
					sampleY += delta
				}
				if !image.Pt(sampleX, sampleY).In(source.Bounds()) {
					continue
				}
				offset := source.PixOffset(sampleX, sampleY)
				for channel := range 4 {
					sums[channel] += int64(source.Pix[offset+channel])
				}
			}
			offset := destination.PixOffset(x, y)
			for channel := range 4 {
				destination.Pix[offset+channel] = uint8((sums[channel] + window/2) / window)
			}
		}
	}
	return destination
}

func directBlurAlpha(bounds, sourceBounds image.Rectangle, sample func(int, int) uint8, pass blurPass) *image.Alpha {
	destination := image.NewAlpha(bounds)
	window := int64(pass.radius)*2 + 1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			sum := int64(0)
			for delta := -pass.radius; delta <= pass.radius; delta++ {
				sampleX, sampleY := x, y
				if pass.axis == blurHorizontal {
					sampleX += delta
				} else {
					sampleY += delta
				}
				if image.Pt(sampleX, sampleY).In(sourceBounds) {
					sum += int64(sample(sampleX, sampleY))
				}
			}
			destination.Pix[destination.PixOffset(x, y)] = uint8((sum + window/2) / window)
		}
	}
	return destination
}

func assertRawRGBAEqual(t *testing.T, got, want *image.RGBA) {
	t.Helper()
	for y := want.Bounds().Min.Y; y < want.Bounds().Max.Y; y++ {
		for x := want.Bounds().Min.X; x < want.Bounds().Max.X; x++ {
			gotOffset, wantOffset := got.PixOffset(x, y), want.PixOffset(x, y)
			if !bytes.Equal(got.Pix[gotOffset:gotOffset+4], want.Pix[wantOffset:wantOffset+4]) {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got.Pix[gotOffset:gotOffset+4], want.Pix[wantOffset:wantOffset+4])
			}
		}
	}
}

func assertRawAlphaEqual(t *testing.T, got, want *image.Alpha) {
	t.Helper()
	for y := want.Bounds().Min.Y; y < want.Bounds().Max.Y; y++ {
		for x := want.Bounds().Min.X; x < want.Bounds().Max.X; x++ {
			if got.AlphaAt(x, y) != want.AlphaAt(x, y) {
				t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, got.AlphaAt(x, y).A, want.AlphaAt(x, y).A)
			}
		}
	}
}

func TestDropShadowPixelsAndTransparentIdentity(t *testing.T) {
	t.Parallel()

	makeNode := func(shadow d2scene.DropShadow) *d2scene.Node {
		node := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{X: 4, Y: 4, Width: 4, Height: 4},
			Fill: red,
		})
		node.Filters = []d2scene.Filter{shadow}
		return node
	}
	document := func(node *d2scene.Node) *d2scene.Document {
		return d2scene.NewDocument(d2scene.Box{Width: 16, Height: 12}, node)
	}

	shadow := d2scene.DropShadow{OffsetX: 6, Color: color.NRGBA{A: 128}}
	frame, err := Render(context.Background(), document(makeNode(shadow)), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, frame.NRGBAAt(5, 5), color.NRGBA{R: 255, A: 255})
	assertPixel(t, frame.NRGBAAt(11, 5), color.NRGBA{A: 128})
	assertPixel(t, frame.NRGBAAt(15, 5), color.NRGBA{})

	baselineNode := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 4, Y: 4, Width: 4, Height: 4}, Fill: red})
	baseline, err := renderTestPNG(context.Background(), document(baselineNode), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	transparent, err := renderTestPNG(context.Background(), document(makeNode(d2scene.DropShadow{
		OffsetX: math.MaxFloat64, SigmaX: math.MaxFloat64, SigmaY: math.MaxFloat64, Color: color.NRGBA{R: 255},
	})), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(transparent, baseline) {
		t.Fatal("transparent drop shadow changed the frame")
	}
	_, err = renderTestPNG(context.Background(), document(makeNode(d2scene.DropShadow{
		OffsetX: math.MaxFloat64, Color: color.NRGBA{A: 255},
	})), testOptions())
	if err == nil || !strings.Contains(err.Error(), "translated filter bounds exceed") {
		t.Fatalf("unrepresentable opaque shadow error = %v", err)
	}

	offscreenSource := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: -6, Y: 4, Width: 4, Height: 4}, Fill: red,
	})
	offscreenSource.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetX: 10, Color: color.NRGBA{A: 255},
	}}
	broughtIntoView, err := Render(context.Background(), document(offscreenSource), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, broughtIntoView.NRGBAAt(5, 5), color.NRGBA{A: 255})

	patternSource := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{X: -6, Y: 4, Width: 4, Height: 4},
		Fill: stripedPattern(d2scene.UserSpaceOnUse, d2scene.Box{Width: 2, Height: 1}, d2scene.Identity()),
	})
	patternSource.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetX: 10, Color: color.NRGBA{A: 255},
	}}
	patternShadow, err := Render(context.Background(), document(patternSource), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, patternShadow.NRGBAAt(5, 5), color.NRGBA{A: 255})
}

func BenchmarkPaintDropShadow(b *testing.B) {
	bounds := image.Rect(-257, 113, 255, 625)
	sourceParent := image.NewRGBA(image.Rect(bounds.Min.X-3, bounds.Min.Y-2, bounds.Max.X+5, bounds.Max.Y+4))
	source := sourceParent.SubImage(bounds).(*image.RGBA)
	alphaParent := image.NewAlpha(image.Rect(bounds.Min.X-5, bounds.Min.Y-4, bounds.Max.X+7, bounds.Max.Y+6))
	alpha := alphaParent.SubImage(bounds).(*image.Alpha)
	destinationParent := image.NewRGBA(image.Rect(bounds.Min.X-2, bounds.Min.Y-3, bounds.Max.X+4, bounds.Max.Y+5))
	destination := destinationParent.SubImage(bounds).(*image.RGBA)
	state := uint32(42)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			state = state*1664525 + 1013904223
			value := uint8(state>>24 | 1)
			source.Pix[source.PixOffset(x, y)+3] = value
			alpha.Pix[alpha.PixOffset(x, y)] = value
		}
	}
	shadow := color.NRGBA{R: 29, G: 97, B: 211, A: 173}
	for _, test := range []struct {
		name    string
		blurred *image.Alpha
	}{
		{name: "RGBA", blurred: nil},
		{name: "Alpha", blurred: alpha},
	} {
		for _, offset := range []struct {
			name string
			x, y float64
		}{
			{name: "Fractional", x: .375, y: -.625},
			{name: "Integer", x: 3, y: -5},
		} {
			b.Run(test.name+"/"+offset.name+"/DirectPixels", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if err := paintDropShadow(context.Background(), destination, source, test.blurred, offset.x, offset.y, shadow); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run(test.name+"/"+offset.name+"/ClosureOracle", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if err := referencePaintDropShadow(context.Background(), destination, source, test.blurred, offset.x, offset.y, shadow); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkPaintDropShadowSmall(b *testing.B) {
	for _, size := range []int{1, 4, 16} {
		bounds := image.Rect(0, 0, size, size)
		source := image.NewRGBA(bounds)
		destination := image.NewRGBA(bounds)
		for pixel := 3; pixel < len(source.Pix); pixel += 4 {
			source.Pix[pixel] = uint8(pixel*37 | 1)
		}
		shadow := color.NRGBA{R: 29, G: 97, B: 211, A: 173}
		b.Run(fmt.Sprintf("%dx%d", size, size), func(b *testing.B) {
			b.Run("DirectPixels", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if err := paintDropShadow(context.Background(), destination, source, nil, 0, 0, shadow); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("ClosureOracle", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if err := referencePaintDropShadow(context.Background(), destination, source, nil, 0, 0, shadow); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func TestShadowAlphaDirectPixelsMatchClosureOracle(t *testing.T) {
	t.Parallel()
	for _, bounds := range []image.Rectangle{
		image.Rect(-13, -9, 11, 17),
		image.Rect(5, 7, 29, 33),
	} {
		rgbaParent := image.NewRGBA(image.Rect(bounds.Min.X-3, bounds.Min.Y-2, bounds.Max.X+5, bounds.Max.Y+4))
		rgba := rgbaParent.SubImage(bounds).(*image.RGBA)
		alphaParent := image.NewAlpha(image.Rect(bounds.Min.X-5, bounds.Min.Y-4, bounds.Max.X+7, bounds.Max.Y+6))
		alpha := alphaParent.SubImage(bounds).(*image.Alpha)
		state := uint32(42)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				state = state*1664525 + 1013904223
				value := uint8(state >> 24)
				rgba.Pix[rgba.PixOffset(x, y)+3] = value
				alpha.Pix[alpha.PixOffset(x, y)] = value
			}
		}

		coordinates := []float64{
			math.Inf(-1), -math.MaxFloat64,
			float64(bounds.Min.X) - 1.25, float64(bounds.Min.X) - 1,
			float64(bounds.Min.X) - .999, float64(bounds.Min.X) - .5,
			float64(bounds.Min.X), float64(bounds.Min.X) + .125,
			float64(bounds.Min.X+bounds.Dx()/2) + .375,
			float64(bounds.Max.X) - 1, float64(bounds.Max.X) - .25,
			float64(bounds.Max.X), float64(bounds.Max.X) + .25,
			math.MaxFloat64, math.Inf(1), math.NaN(),
		}
		for _, test := range []struct {
			name    string
			blurred *image.Alpha
			sample  func(float64, float64) uint8
		}{
			{name: "RGBA", sample: func(x, y float64) uint8 { return sampleShadowAlphaRGBA(rgba, x, y) }},
			{name: "Alpha", blurred: alpha, sample: func(x, y float64) uint8 { return sampleShadowAlphaImage(alpha, x, y) }},
		} {
			t.Run(fmt.Sprintf("%v/%s", bounds, test.name), func(t *testing.T) {
				for _, y := range coordinates {
					for _, x := range coordinates {
						got := test.sample(x, y)
						want := referenceSampleShadowAlpha(rgba, test.blurred, x, y)
						if got != want {
							t.Fatalf("sample (%v,%v) = %d, want %d", x, y, got, want)
						}
					}
				}
				state := uint32(19)
				for sample := 0; sample < 20_000; sample++ {
					state = state*1664525 + 1013904223
					x := float64(bounds.Min.X-2) + float64(state)/float64(^uint32(0))*float64(bounds.Dx()+4)
					state = state*1664525 + 1013904223
					y := float64(bounds.Min.Y-2) + float64(state)/float64(^uint32(0))*float64(bounds.Dy()+4)
					got := test.sample(x, y)
					want := referenceSampleShadowAlpha(rgba, test.blurred, x, y)
					if got != want {
						t.Fatalf("random sample %d at (%v,%v) = %d, want %d", sample, x, y, got, want)
					}
				}
			})
		}
	}
}

func TestPaintDropShadowDirectPixelsMatchClosureOracle(t *testing.T) {
	t.Parallel()
	for _, bounds := range []image.Rectangle{
		image.Rect(-19, -11, 17, 23),
		image.Rect(5, 7, 41, 41),
	} {
		sourceParent := image.NewRGBA(image.Rect(bounds.Min.X-4, bounds.Min.Y-3, bounds.Max.X+6, bounds.Max.Y+5))
		source := sourceParent.SubImage(bounds).(*image.RGBA)
		alphaParent := image.NewAlpha(image.Rect(bounds.Min.X-2, bounds.Min.Y-5, bounds.Max.X+4, bounds.Max.Y+7))
		alpha := alphaParent.SubImage(bounds).(*image.Alpha)
		state := uint32(42)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				state = state*1664525 + 1013904223
				value := uint8(state >> 24)
				source.Pix[source.PixOffset(x, y)+3] = value
				alpha.Pix[alpha.PixOffset(x, y)] = value
			}
		}
		for _, offset := range []struct{ x, y float64 }{
			{x: 0, y: 0},
			{x: 3, y: -5},
			{x: -17, y: 19},
			{x: float64(bounds.Dx()), y: -float64(bounds.Dy())},
			{x: .375, y: -.625},
			{x: -3.25, y: 2.75},
			{x: float64(bounds.Dx()) + .5, y: -float64(bounds.Dy()) - .5},
			{x: math.MaxFloat64, y: -math.MaxFloat64},
			{x: math.Inf(1), y: math.NaN()},
		} {
			for _, test := range []struct {
				name    string
				blurred *image.Alpha
			}{
				{name: "RGBA"},
				{name: "Alpha", blurred: alpha},
			} {
				t.Run(fmt.Sprintf("%v/%s/%g,%g", bounds, test.name, offset.x, offset.y), func(t *testing.T) {
					parentBounds := image.Rect(bounds.Min.X-3, bounds.Min.Y-2, bounds.Max.X+5, bounds.Max.Y+4)
					gotParent := image.NewRGBA(parentBounds)
					wantParent := image.NewRGBA(parentBounds)
					for index := range gotParent.Pix {
						gotParent.Pix[index] = uint8(index*37 + 11)
					}
					copy(wantParent.Pix, gotParent.Pix)
					got := gotParent.SubImage(bounds).(*image.RGBA)
					want := wantParent.SubImage(bounds).(*image.RGBA)
					shadow := color.NRGBA{R: 29, G: 97, B: 211, A: 173}
					if err := paintDropShadow(context.Background(), got, source, test.blurred, offset.x, offset.y, shadow); err != nil {
						t.Fatal(err)
					}
					if err := referencePaintDropShadow(context.Background(), want, source, test.blurred, offset.x, offset.y, shadow); err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(gotParent.Pix, wantParent.Pix) {
						t.Fatal("direct-pixel painting differs from closure oracle or changed stride padding")
					}
				})
			}
		}
	}
}

func TestPaintDropShadowDirectPixelsCancellation(t *testing.T) {
	t.Parallel()
	bounds := image.Rect(-9, -7, 23, 25)
	source := image.NewRGBA(bounds)
	alpha := image.NewAlpha(bounds)
	for index := 3; index < len(source.Pix); index += 4 {
		source.Pix[index] = 255
	}
	for index := range alpha.Pix {
		alpha.Pix[index] = 255
	}
	for _, offset := range []struct{ x, y float64 }{{x: .375, y: -.625}, {x: 3, y: -5}} {
		for _, blurred := range []*image.Alpha{nil, alpha} {
			got := image.NewRGBA(bounds)
			want := image.NewRGBA(bounds)
			gotContext := &cancelAfterContext{after: 2}
			wantContext := &cancelAfterContext{after: 2}
			gotErr := paintDropShadow(gotContext, got, source, blurred, offset.x, offset.y, color.NRGBA{A: 255})
			wantErr := referencePaintDropShadow(wantContext, want, source, blurred, offset.x, offset.y, color.NRGBA{A: 255})
			if !errors.Is(gotErr, context.Canceled) || !errors.Is(wantErr, context.Canceled) {
				t.Fatalf("offset (%g,%g) cancellation errors = (%v, %v), want context.Canceled", offset.x, offset.y, gotErr, wantErr)
			}
			if gotContext.calls != wantContext.calls || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("offset (%g,%g) cancellation checkpoint/output differs: calls %d/%d", offset.x, offset.y, gotContext.calls, wantContext.calls)
			}
		}
	}
}

func TestPaintDropShadowIntegerFallsBackOutsideExactFloatDomain(t *testing.T) {
	t.Parallel()
	if int64(platformMaxInt()) <= int64(1)<<53 {
		t.Skip("requires 64-bit image coordinates outside float64's exact integer domain")
	}

	far := -(int64(1) << 53) - 1024
	bounds := image.Rect(int(far), 0, int(far+2), 1)
	offsetX := float64(-far)
	if translated, ok := exactIntegerTranslatedBounds(bounds, offsetX, 0); ok {
		t.Fatalf("translated bounds = %v, want float-aliasing fallback", translated)
	}

	source := image.NewRGBA(bounds)
	source.Pix[3], source.Pix[7] = 31, 223
	alpha := image.NewAlpha(bounds)
	alpha.Pix[0], alpha.Pix[1] = 47, 211
	for _, test := range []struct {
		name    string
		blurred *image.Alpha
	}{
		{name: "RGBA"},
		{name: "Alpha", blurred: alpha},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := image.NewRGBA(image.Rect(0, 0, 2, 1))
			want := image.NewRGBA(got.Bounds())
			shadow := color.NRGBA{R: 29, G: 97, B: 211, A: 173}
			if err := paintDropShadow(context.Background(), got, source, test.blurred, offsetX, 0, shadow); err != nil {
				t.Fatal(err)
			}
			if err := referencePaintDropShadow(context.Background(), want, source, test.blurred, offsetX, 0, shadow); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("direct-pixel painting differs from float-sampling oracle: got %v, want %v", got.Pix, want.Pix)
			}
		})
	}
}

func TestPaintDropShadowIntegerFallsBackForInexactDestinationCoordinates(t *testing.T) {
	t.Parallel()
	if int64(platformMaxInt()) <= int64(1)<<53 {
		t.Skip("requires 64-bit image coordinates outside float64's exact integer domain")
	}

	exactMin := -(int64(1) << 53)
	source := image.NewRGBA(image.Rect(int(exactMin), 0, int(exactMin+2), 1))
	source.Pix[3], source.Pix[7] = 31, 223
	destinationBounds := image.Rect(int(exactMin-1), 0, int(exactMin+2), 1)
	got := image.NewRGBA(destinationBounds)
	want := image.NewRGBA(destinationBounds)
	shadow := color.NRGBA{R: 29, G: 97, B: 211, A: 173}
	if err := paintDropShadow(context.Background(), got, source, nil, 0, 0, shadow); err != nil {
		t.Fatal(err)
	}
	if err := referencePaintDropShadow(context.Background(), want, source, nil, 0, 0, shadow); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatalf("direct-pixel painting differs from float-sampling oracle: got %v, want %v", got.Pix, want.Pix)
	}
}

func referencePaintDropShadow(ctx context.Context, destination, source *image.RGBA, blurred *image.Alpha, offsetX, offsetY float64, shadow color.NRGBA) error {
	for y := destination.Bounds().Min.Y; y < destination.Bounds().Max.Y; y++ {
		if (y-destination.Bounds().Min.Y)&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		for x := destination.Bounds().Min.X; x < destination.Bounds().Max.X; x++ {
			if (x-destination.Bounds().Min.X)&1023 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			alpha := referenceSampleShadowAlpha(source, blurred, float64(x)-offsetX, float64(y)-offsetY)
			if alpha == 0 {
				continue
			}
			shadowAlpha := uint8((uint32(alpha)*uint32(shadow.A) + 127) / 255)
			offset := destination.PixOffset(x, y)
			destination.Pix[offset] = uint8((uint32(shadow.R)*uint32(shadowAlpha) + 127) / 255)
			destination.Pix[offset+1] = uint8((uint32(shadow.G)*uint32(shadowAlpha) + 127) / 255)
			destination.Pix[offset+2] = uint8((uint32(shadow.B)*uint32(shadowAlpha) + 127) / 255)
			destination.Pix[offset+3] = shadowAlpha
		}
	}
	return ctx.Err()
}

func referenceSampleShadowAlpha(source *image.RGBA, blurred *image.Alpha, x, y float64) uint8 {
	var bounds image.Rectangle
	var sample func(int, int) uint8
	if blurred == nil {
		bounds = source.Bounds()
		sample = func(px, py int) uint8 {
			if px < bounds.Min.X || px >= bounds.Max.X || py < bounds.Min.Y || py >= bounds.Max.Y {
				return 0
			}
			return source.Pix[source.PixOffset(px, py)+3]
		}
	} else {
		bounds = blurred.Bounds()
		sample = func(px, py int) uint8 {
			if px < bounds.Min.X || px >= bounds.Max.X || py < bounds.Min.Y || py >= bounds.Max.Y {
				return 0
			}
			return blurred.Pix[blurred.PixOffset(px, py)]
		}
	}
	if !finite(x) || !finite(y) ||
		x < float64(bounds.Min.X)-1 || x >= float64(bounds.Max.X) ||
		y < float64(bounds.Min.Y)-1 || y >= float64(bounds.Max.Y) {
		return 0
	}
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	if fx == 0 && fy == 0 {
		return sample(x0, y0)
	}
	a00 := float64(sample(x0, y0))
	a10 := float64(sample(x0+1, y0))
	a01 := float64(sample(x0, y0+1))
	a11 := float64(sample(x0+1, y0+1))
	top := a00 + (a10-a00)*fx
	bottom := a01 + (a11-a01)*fx
	return roundedByte(top + (bottom-top)*fy)
}

func TestFiltersRespectDeclaredOrder(t *testing.T) {
	t.Parallel()

	render := func(filters ...d2scene.Filter) ([]byte, *preparedDocument) {
		t.Helper()
		node := d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{X: 5, Y: 5, Width: 6, Height: 6}, Fill: red,
		})
		node.Filters = filters
		document := d2scene.NewDocument(d2scene.Box{Width: 20, Height: 20}, node)
		prepared, err := prepare(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		png, err := renderTestPNG(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		return png, prepared
	}
	blur := d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1}
	shadow := d2scene.DropShadow{OffsetX: 1, OffsetY: 1, SigmaX: 1, SigmaY: 1, Color: color.NRGBA{A: 220}}
	blurThenShadow, first := render(blur, shadow)
	shadowThenBlur, second := render(shadow, blur)
	if len(first.root.filters) != 2 || first.root.filters[0].kind != preparedGaussianBlur || first.root.filters[1].kind != preparedDropShadow {
		t.Fatalf("prepared blur-then-shadow order = %+v", first.root.filters)
	}
	if len(second.root.filters) != 2 || second.root.filters[0].kind != preparedDropShadow || second.root.filters[1].kind != preparedGaussianBlur {
		t.Fatalf("prepared shadow-then-blur order = %+v", second.root.filters)
	}
	if bytes.Equal(blurThenShadow, shadowThenBlur) {
		t.Fatal("reversing non-commuting filters did not change rendered pixels")
	}
}

func TestFilterParametersUseComposedDeviceTransform(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 1, Y: 1, Width: 2, Height: 2}, Fill: red,
	})
	node.Transform = d2scene.Scale(2, 3)
	node.Filters = []d2scene.Filter{
		&d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1},
		&d2scene.DropShadow{OffsetX: 2, OffsetY: 2, Color: color.NRGBA{A: 255}},
	}
	prepared, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 20, Height: 20}, node), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.root.filters) != 2 {
		t.Fatalf("prepared filters = %d, want 2", len(prepared.root.filters))
	}
	blur, shadow := prepared.root.filters[0], prepared.root.filters[1]
	if blur.kind != preparedGaussianBlur || len(blur.passes) != 6 {
		t.Fatalf("scaled Gaussian preparation = %+v", blur)
	}
	horizontal, vertical := 0, 0
	for _, pass := range blur.passes {
		if pass.axis == blurHorizontal {
			horizontal += pass.radius
		} else {
			vertical += pass.radius
		}
	}
	if horizontal != 6 || vertical != 9 {
		t.Fatalf("scaled Gaussian support = (%d,%d), want (6,9)", horizontal, vertical)
	}
	if shadow.kind != preparedDropShadow || shadow.offsetX != 4 || shadow.offsetY != 6 {
		t.Fatalf("scaled drop-shadow offset = %+v, want (4,6)", shadow)
	}
}

func TestFilterOrderPrecedesClipMaskOpacity(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4}, Fill: red,
	})
	node.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetX: 4, Color: color.NRGBA{A: 255},
	}}
	node.Clip = &d2scene.Clip{Path: clipRect(0, 0, 9, 12, d2scene.NonZero), Transform: d2scene.Identity()}
	maskRoot := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{Width: 12, Height: 12},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	})
	maskRoot.Opacity = .5
	node.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: maskRoot, Transform: d2scene.Identity()}
	node.Opacity = .5

	frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 12, Height: 12}, node), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	shadow := frame.NRGBAAt(8, 4)
	if shadow.R != 0 || shadow.G != 0 || shadow.B != 0 || shadow.A < 63 || shadow.A > 64 {
		t.Fatalf("filtered shadow after mask and opacity = %#v, want black alpha 63/64", shadow)
	}
	assertPixel(t, frame.NRGBAAt(9, 4), color.NRGBA{})
}

func TestAnimatedDropShadowTargetIndexAndConcurrentImmutability(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 2, Y: 4, Width: 4, Height: 4}, Fill: red,
	})
	node.Filters = []d2scene.Filter{
		d2scene.DropShadow{OffsetX: 1, Color: color.NRGBA{G: 255, A: 255}},
		d2scene.GaussianBlur{},
		d2scene.DropShadow{OffsetX: 4, Color: color.NRGBA{A: 64}},
	}
	track := animationTrack(
		d2scene.AnimateDropShadow,
		d2scene.ShadowValue(d2scene.DropShadow{OffsetX: 4, Color: color.NRGBA{A: 64}}),
		d2scene.ShadowValue(d2scene.DropShadow{OffsetX: 8, Color: color.NRGBA{B: 255, A: 192}}),
	)
	track.TargetIndex = 2
	node.Animations = []d2scene.Track{track}
	document := d2scene.NewDocument(d2scene.Box{Width: 20, Height: 12}, node)
	original := fmt.Sprintf("%#v", document)

	options := testOptions()
	options.Time = 500 * time.Millisecond
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.root.filters) != 2 {
		t.Fatalf("prepared active filters = %d, want two drop shadows and omitted identity blur", len(prepared.root.filters))
	}
	first, second := prepared.root.filters[0], prepared.root.filters[1]
	if first.kind != preparedDropShadow || first.offsetX != 1 || first.shadowColor != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("static target zero changed: %+v", first)
	}
	if second.kind != preparedDropShadow || second.offsetX != 6 || second.shadowColor != (color.NRGBA{B: 128, A: 128}) {
		t.Fatalf("animated target two = %+v, want midpoint shadow", second)
	}

	times := []time.Duration{0, 250 * time.Millisecond, 500 * time.Millisecond, 750 * time.Millisecond, time.Second}
	want := make([][]byte, len(times))
	for index, timestamp := range times {
		frameOptions := testOptions()
		frameOptions.Time = timestamp
		want[index], err = renderTestPNG(context.Background(), document, frameOptions)
		if err != nil {
			t.Fatal(err)
		}
	}
	var wait sync.WaitGroup
	errs := make(chan error, len(times)*4)
	for range 4 {
		for index, timestamp := range times {
			wait.Add(1)
			go func(index int, timestamp time.Duration) {
				defer wait.Done()
				frameOptions := testOptions()
				frameOptions.Time = timestamp
				got, err := renderTestPNG(context.Background(), document, frameOptions)
				if err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(got, want[index]) {
					errs <- errors.New("concurrent animated drop-shadow frame changed")
				}
			}(index, timestamp)
		}
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if fmt.Sprintf("%#v", document) != original {
		t.Fatal("animated drop-shadow rendering mutated the document")
	}
}

func TestDropShadowAnimationRejectsInvalidTargetIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		index  int
		filter d2scene.Filter
		want   string
	}{
		{name: "outside", index: 1, filter: d2scene.DropShadow{}, want: "outside 1 filters"},
		{name: "wrong kind", filter: d2scene.GaussianBlur{SigmaX: 1}, want: "does not identify a drop shadow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 4}, Fill: red})
			node.Filters = []d2scene.Filter{test.filter}
			track := animationTrack(
				d2scene.AnimateDropShadow,
				d2scene.ShadowValue(d2scene.DropShadow{}),
				d2scene.ShadowValue(d2scene.DropShadow{}),
			)
			track.TargetIndex = test.index
			node.Animations = []d2scene.Track{track}
			_, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, node), testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Render() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDropShadowAnimationDoesNotHideInvalidStaticFilter(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 4}, Fill: red})
	node.Filters = []d2scene.Filter{d2scene.DropShadow{SigmaX: -1}}
	node.Animations = []d2scene.Track{animationTrack(
		d2scene.AnimateDropShadow,
		d2scene.ShadowValue(d2scene.DropShadow{}),
		d2scene.ShadowValue(d2scene.DropShadow{}),
	)}
	_, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, node), testOptions())
	if err == nil || !strings.Contains(err.Error(), "invalid Gaussian deviation") {
		t.Fatalf("Render() error = %v, want invalid static filter rejection", err)
	}
}

func TestFilterPreflightValidationAndStructuralLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter d2scene.Filter
		want   string
	}{
		{name: "negative sigma", filter: d2scene.GaussianBlur{SigmaX: -1}, want: "invalid Gaussian deviation"},
		{name: "NaN sigma", filter: d2scene.GaussianBlur{SigmaY: math.NaN()}, want: "invalid Gaussian deviation"},
		{name: "infinite offset", filter: d2scene.DropShadow{OffsetX: math.Inf(1), Color: color.NRGBA{A: 255}}, want: "invalid drop-shadow offset"},
		{name: "nil Gaussian pointer", filter: (*d2scene.GaussianBlur)(nil), want: "nil Gaussian blur"},
		{name: "nil shadow pointer", filter: (*d2scene.DropShadow)(nil), want: "nil drop shadow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := d2scene.NewNode(nil)
			node.ID = "filtered"
			node.Filters = []d2scene.Filter{test.filter}
			_, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, node), testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("prepare() error = %v, want substring %q", err, test.want)
			}
		})
	}

	tooWide := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	tooWide.ID = "wide"
	tooWide.Transform = d2scene.Scale(float64(maxBlurSupportRadius), 1)
	tooWide.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 2}}
	_, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, tooWide), testOptions())
	if err == nil || !strings.Contains(err.Error(), "three-sigma support exceeds") {
		t.Fatalf("large transformed deviation error = %v", err)
	}

	tooMany := d2scene.NewNode(nil)
	tooMany.Filters = make([]d2scene.Filter, 2)
	for index := range tooMany.Filters {
		tooMany.Filters[index] = d2scene.GaussianBlur{}
	}
	options := testOptions()
	options.MaxNodes = 1
	_, err = prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, tooMany), options)
	if err == nil || !strings.Contains(err.Error(), "filter count to exceed structural limit 1") {
		t.Fatalf("filter structural-limit error = %v", err)
	}

	tooDistant := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	tooDistant.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetX: 20_000_000, Color: color.NRGBA{A: 255},
	}}
	_, err = prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, tooDistant), testOptions())
	if err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage") {
		t.Fatalf("distant-shadow resource error = %v", err)
	}

	empty := d2scene.NewNode(nil)
	empty.Filters = []d2scene.Filter{
		d2scene.GaussianBlur{SigmaX: math.MaxFloat64, SigmaY: math.MaxFloat64},
		d2scene.DropShadow{OffsetX: 2, SigmaX: 1, SigmaY: 1, Color: color.NRGBA{A: 255}},
	}
	prepared, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, empty), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.root.filters) != 0 || !prepared.root.bounds.Empty() || prepared.resources.peakOffscreenBytes != 0 {
		t.Fatalf("empty filtered node retained work: filters=%d bounds=%v resources=%+v", len(prepared.root.filters), prepared.root.bounds, prepared.resources)
	}
}

func TestFilterResourcePlanLimitsAndNodeLocalLayers(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 45, Y: 45, Width: 10, Height: 10}, Fill: red,
	})
	node.Filters = []d2scene.Filter{
		d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1},
		d2scene.DropShadow{OffsetX: 3, OffsetY: 2, SigmaX: 1, SigmaY: 1, Color: color.NRGBA{A: 180}},
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 100, Height: 100}, node)
	options := testOptions()
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	filterPeak, finalBytes, err := planFilterResources(prepared.root.filters, prepared.root.contentBounds)
	if err != nil {
		t.Fatal(err)
	}
	if finalBytes == 0 || filterPeak == 0 {
		t.Fatalf("filter resource plan = peak %d final %d", filterPeak, finalBytes)
	}
	want, ok := checkedAdd(filterPeak, prepared.resources.rasterizerBytes)
	if !ok || prepared.resources.peakOffscreenBytes != want {
		t.Fatalf("planned peak = %d, want filter %d + rasterizer %d = %d", prepared.resources.peakOffscreenBytes, filterPeak, prepared.resources.rasterizerBytes, want)
	}
	if prepared.resources.peakOffscreenBytes >= 100*100*4 {
		t.Fatalf("small filtered node planned %d bytes, unexpectedly at least one full-document RGBA layer", prepared.resources.peakOffscreenBytes)
	}

	options.MaxOffscreenBytes = want - 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage") {
		t.Fatalf("below-limit prepare() error = %v", err)
	}
	options.MaxOffscreenBytes = want
	if _, err := Render(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive filter limit Render() error = %v", err)
	}

	scratch := &rasterScratch{offscreen: offscreenBudget{limit: want - 1}}
	rasterizerBytes, err := scratch.offscreen.reserveBytes(prepared.resources.rasterizerBytes, "test rasterizer")
	if err != nil {
		t.Fatal(err)
	}
	err = renderNode(context.Background(), image.NewRGBA(image.Rect(0, 0, 100, 100)), prepared.root, scratch)
	if err == nil || !strings.Contains(err.Error(), "exceeding limit") {
		t.Fatalf("runtime below-limit error = %v", err)
	}
	if scratch.offscreen.live != rasterizerBytes {
		t.Fatalf("failed filter render retained %d bytes, want rasterizer-only %d", scratch.offscreen.live, rasterizerBytes)
	}
	scratch.offscreen.release(rasterizerBytes)

	scratch = &rasterScratch{offscreen: offscreenBudget{limit: want}}
	rasterizerBytes, err = scratch.offscreen.reserveBytes(prepared.resources.rasterizerBytes, "test rasterizer")
	if err != nil {
		t.Fatal(err)
	}
	if err := renderNode(context.Background(), image.NewRGBA(image.Rect(0, 0, 100, 100)), prepared.root, scratch); err != nil {
		t.Fatalf("runtime exact-limit render: %v", err)
	}
	if scratch.offscreen.live != rasterizerBytes || scratch.offscreen.peak != want {
		t.Fatalf("runtime accounting live=%d peak=%d, want live=%d peak=%d", scratch.offscreen.live, scratch.offscreen.peak, rasterizerBytes, want)
	}
	scratch.offscreen.release(rasterizerBytes)
}

func TestGaussianBlurCancellationReleasesLayers(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 128, Height: 128}, Fill: red,
	})
	node.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 4, SigmaY: 4}}
	prepared, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 128, Height: 128}, node), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.root.filters) != 1 {
		t.Fatalf("prepared filters = %d, want 1", len(prepared.root.filters))
	}
	filterPeak, _, err := planFilterResources(prepared.root.filters, prepared.root.contentBounds)
	if err != nil {
		t.Fatal(err)
	}
	scratch := &rasterScratch{offscreen: offscreenBudget{limit: filterPeak}}
	input, err := reserveRGBA(scratch, prepared.root.contentBounds, "test filter input")
	if err != nil {
		t.Fatal(err)
	}
	for offset := 3; offset < len(input.image.Pix); offset += 4 {
		input.image.Pix[offset] = 255
	}
	err = applyPreparedFilter(&cancelAfterContext{after: 2}, &input, prepared.root.filters[0], scratch)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Gaussian error = %v, want context.Canceled", err)
	}
	if scratch.offscreen.live != 0 {
		t.Fatalf("canceled Gaussian retained %d offscreen bytes", scratch.offscreen.live)
	}

	input, err = reserveRGBA(scratch, prepared.root.contentBounds, "retry filter input")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyPreparedFilter(context.Background(), &input, prepared.root.filters[0], scratch); err != nil {
		t.Fatalf("retry Gaussian filter: %v", err)
	}
	if scratch.offscreen.live != input.reservation {
		t.Fatalf("retry live bytes = %d, want output reservation %d", scratch.offscreen.live, input.reservation)
	}
	input.release()
	if scratch.offscreen.live != 0 {
		t.Fatalf("released retry retained %d bytes", scratch.offscreen.live)
	}
}
