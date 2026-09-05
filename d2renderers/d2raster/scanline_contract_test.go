package d2raster_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
)

// These tests exercise the scanline package through the narrow surface used by
// d2raster. They are contract tests, not tests of implementation details.

func TestScanlineRasterizerNonZeroWindingContract(t *testing.T) {
	t.Run("same winding remains filled and opposite winding makes a hole", func(t *testing.T) {
		rasterizer := scanline.NewRasterizer(64, 32)
		addContractRect(rasterizer, 2, 2, 30, 30, false)
		addContractRect(rasterizer, 10, 10, 22, 22, false)
		addContractRect(rasterizer, 34, 2, 62, 30, false)
		addContractRect(rasterizer, 42, 10, 54, 22, true)

		mask := drawContractAlpha(rasterizer, 64, 32)
		assertContractAlpha(t, mask, 16, 16, 255)
		assertContractAlpha(t, mask, 48, 16, 0)
		assertContractAlpha(t, mask, 38, 6, 255)
	})

	t.Run("reflection preserves fill topology", func(t *testing.T) {
		const size = 32
		render := func(reflect bool) *image.Alpha {
			rasterizer := scanline.NewRasterizer(size, size)
			points := [][2]float32{{4, 4}, {28, 4}, {28, 28}, {4, 28}}
			hole := [][2]float32{{10, 10}, {10, 22}, {22, 22}, {22, 10}}
			if reflect {
				reflectContractX(points, size)
				reflectContractX(hole, size)
			}
			addContractPolygon(rasterizer, points)
			addContractPolygon(rasterizer, hole)
			return drawContractAlpha(rasterizer, size, size)
		}

		normal := render(false)
		reflected := render(true)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				got := reflected.AlphaAt(size-1-x, y).A
				want := normal.AlphaAt(x, y).A
				if got != want {
					t.Fatalf("reflected alpha at (%d,%d) = %d, want mirror %d", size-1-x, y, got, want)
				}
			}
		}
	})
}

func TestScanlineRasterizerOverlapsAreOnePaintOperation(t *testing.T) {
	rasterizer := scanline.NewRasterizer(24, 12)
	addContractRect(rasterizer, 2, 2, 13, 10, false)
	addContractRect(rasterizer, 9, 2, 20, 10, false)

	destination := image.NewRGBA(image.Rect(0, 0, 24, 12))
	source := color.NRGBA{R: 231, G: 73, B: 19, A: 128}
	drawContractRGBA(t, rasterizer, destination, source)

	left := destination.RGBAAt(5, 5)
	overlap := destination.RGBAAt(11, 5)
	right := destination.RGBAAt(17, 5)
	if left != overlap || overlap != right {
		t.Fatalf("same-winding overlap was painted more than once: left=%#v overlap=%#v right=%#v", left, overlap, right)
	}
	if overlap.A < 127 || overlap.A > 128 {
		t.Fatalf("overlap alpha = %d, want one half-alpha paint", overlap.A)
	}
}

func TestScanlineRasterizerClipsHugeOffCanvasGeometry(t *testing.T) {
	const size = 8
	for _, test := range []struct {
		name string
		huge float32
	}{
		{name: "million", huge: 1e6},
		{name: "one hundred quintillion", huge: 1e20},
	} {
		t.Run(test.name, func(t *testing.T) {
			rasterizer := scanline.NewRasterizer(size, size)
			addContractRect(rasterizer, -test.huge, -test.huge, test.huge, test.huge, false)
			covered := drawContractAlpha(rasterizer, size, size)
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					assertContractAlpha(t, covered, x, y, 255)
				}
			}

			rasterizer.Reset(size, size)
			addContractRect(rasterizer, test.huge, test.huge, 2*test.huge, 2*test.huge, false)
			offCanvas := drawContractAlpha(rasterizer, size, size)
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					assertContractAlpha(t, offCanvas, x, y, 0)
				}
			}
		})
	}
}

func TestScanlineRasterizerHonorsDestinationOriginAndStride(t *testing.T) {
	parent := image.NewRGBA(image.Rect(0, 0, 20, 16))
	background := color.RGBA{G: 91, A: 255}
	draw.Draw(parent, parent.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	sub := parent.SubImage(image.Rect(4, 5, 12, 11)).(*image.RGBA)

	rasterizer := scanline.NewRasterizer(sub.Bounds().Dx(), sub.Bounds().Dy())
	addContractRect(rasterizer, 0, 0, 8, 6, false)
	drawContractRGBA(t, rasterizer, sub, color.NRGBA{R: 233, A: 255})

	if got := parent.RGBAAt(4, 5); got != (color.RGBA{R: 233, A: 255}) {
		t.Fatalf("subimage origin pixel = %#v", got)
	}
	if got := parent.RGBAAt(11, 10); got != (color.RGBA{R: 233, A: 255}) {
		t.Fatalf("subimage far pixel = %#v", got)
	}
	if got := parent.RGBAAt(3, 5); got != background {
		t.Fatalf("pixel before subimage changed to %#v", got)
	}
	if got := parent.RGBAAt(12, 10); got != background {
		t.Fatalf("pixel after subimage changed to %#v", got)
	}
}

func TestScanlineRasterizerPartialCoverageMatchesDrawMask(t *testing.T) {
	rasterizer := scanline.NewRasterizer(1, 1)
	addContractRect(rasterizer, .5, 0, 1, 1, false)
	mask := drawContractAlpha(rasterizer, 1, 1)
	if got := mask.AlphaAt(0, 0).A; got != 128 {
		t.Fatalf("half-pixel coverage = %d, want 128", got)
	}

	background := color.RGBA{R: 21, G: 87, B: 201, A: 255}
	source := color.NRGBA{R: 239, G: 53, B: 17, A: 173}
	destination := image.NewRGBA(image.Rect(0, 0, 1, 1))
	reference := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for _, target := range []*image.RGBA{destination, reference} {
		draw.Draw(target, target.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	}
	drawContractRGBA(t, rasterizer, destination, source)
	draw.DrawMask(reference, reference.Bounds(), image.NewUniform(source), image.Point{}, mask, image.Point{}, draw.Over)
	if got, want := destination.RGBAAt(0, 0), reference.RGBAAt(0, 0); got != want {
		t.Fatalf("partial source-over = %#v, want standard draw result %#v", got, want)
	}
}

func TestScanlineRasterizerPreservesSubpixelCoverage(t *testing.T) {
	rasterizer := scanline.NewRasterizer(4, 1)
	addContractRect(rasterizer, .25, 0, 2.75, 1, false)
	mask := drawContractAlpha(rasterizer, 4, 1)

	left := mask.AlphaAt(0, 0).A
	middle := mask.AlphaAt(1, 0).A
	right := mask.AlphaAt(2, 0).A
	outside := mask.AlphaAt(3, 0).A
	if left == 0 || left == 255 || left != right {
		t.Fatalf("subpixel edge coverage = left %d, right %d; want equal partial coverage", left, right)
	}
	if middle != 255 || outside != 0 {
		t.Fatalf("subpixel interior/outside coverage = %d/%d, want 255/0", middle, outside)
	}
}

func TestScanlineRasterizerResetAndResizeContract(t *testing.T) {
	rasterizer := scanline.NewRasterizer(12, 4)
	destination := image.NewRGBA(image.Rect(0, 0, 12, 4))

	addContractRect(rasterizer, 0, 0, 4, 4, false)
	drawContractRGBA(t, rasterizer, destination, color.NRGBA{R: 255, A: 255})
	rasterizer.Reset(12, 4)
	addContractRect(rasterizer, 8, 0, 12, 4, false)
	drawContractRGBA(t, rasterizer, destination, color.NRGBA{B: 255, A: 255})

	if got := destination.RGBAAt(1, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("first paint changed after reset: %#v", got)
	}
	if got := destination.RGBAAt(5, 1); got != (color.RGBA{}) {
		t.Fatalf("reset leaked a path into the gap: %#v", got)
	}
	if got := destination.RGBAAt(10, 1); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("second paint after reset = %#v", got)
	}

	rasterizer.Reset(3, 2)
	addContractRect(rasterizer, 0, 0, 3, 2, false)
	resized := drawContractAlpha(rasterizer, 3, 2)
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			assertContractAlpha(t, resized, x, y, 255)
		}
	}
}

func TestScanlineRasterizerAlphaAndRGBACompositingContract(t *testing.T) {
	const size = 3
	rasterizer := scanline.NewRasterizer(size, size)
	addContractRect(rasterizer, 0, 0, size, size, false)

	background := color.RGBA{R: 21, G: 87, B: 201, A: 255}
	source := color.NRGBA{R: 239, G: 53, B: 17, A: 128}
	destination := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(destination, destination.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	drawContractRGBA(t, rasterizer, destination, source)

	reference := image.NewRGBA(image.Rect(0, 0, 1, 1))
	draw.Draw(reference, reference.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	draw.Draw(reference, reference.Bounds(), image.NewUniform(source), image.Point{}, draw.Over)
	if got, want := destination.RGBAAt(1, 1), reference.RGBAAt(0, 0); got != want {
		t.Fatalf("RGBA source-over = %#v, want standard draw result %#v", got, want)
	}

	rasterizer.Reset(size, size)
	addContractRect(rasterizer, 0, 0, size, size, false)
	alphaDestination := image.NewAlpha(image.Rect(0, 0, size, size))
	draw.Draw(alphaDestination, alphaDestination.Bounds(), image.NewUniform(color.Alpha{A: 64}), image.Point{}, draw.Src)
	writeContractAlpha(t, rasterizer, alphaDestination)
	if got := alphaDestination.AlphaAt(1, 1); got.A != 255 {
		t.Fatalf("opaque Alpha source-over = %#v, want 255", got)
	}
}

func TestScanlineRasterizerResetRenderingIsDeterministic(t *testing.T) {
	const size = 32
	rasterizer := scanline.NewRasterizer(size, size)
	var baseline []byte
	for iteration := 0; iteration < 20; iteration++ {
		if iteration > 0 {
			rasterizer.Reset(size, size)
		}
		addContractRect(rasterizer, 1.25, 1.5, 18.75, 15.25, false)
		addContractRect(rasterizer, 10.5, 7.25, 28.25, 22.75, false)
		rasterizer.MoveTo(4, 27)
		rasterizer.CubeTo(5, 15, 27, 15, 28, 27)
		rasterizer.LineTo(4, 27)
		rasterizer.ClosePath()

		destination := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.Draw(destination, destination.Bounds(), image.NewUniform(color.RGBA{R: 7, G: 23, B: 41, A: 255}), image.Point{}, draw.Src)
		drawContractRGBA(t, rasterizer, destination, color.NRGBA{R: 211, G: 79, B: 31, A: 173})
		if iteration == 0 {
			baseline = append([]byte(nil), destination.Pix...)
			continue
		}
		if !bytes.Equal(destination.Pix, baseline) {
			t.Fatalf("render %d after Reset produced different RGBA bytes", iteration)
		}
	}
}

func addContractRect(rasterizer *scanline.Rasterizer, x0, y0, x1, y1 float32, reverse bool) {
	points := [][2]float32{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	if reverse {
		for left, right := 0, len(points)-1; left < right; left, right = left+1, right-1 {
			points[left], points[right] = points[right], points[left]
		}
	}
	addContractPolygon(rasterizer, points)
}

func addContractPolygon(rasterizer *scanline.Rasterizer, points [][2]float32) {
	if len(points) == 0 {
		return
	}
	rasterizer.MoveTo(points[0][0], points[0][1])
	for _, point := range points[1:] {
		rasterizer.LineTo(point[0], point[1])
	}
	rasterizer.ClosePath()
}

func reflectContractX(points [][2]float32, width float32) {
	for index := range points {
		points[index][0] = width - points[index][0]
	}
}

func drawContractAlpha(rasterizer *scanline.Rasterizer, width, height int) *image.Alpha {
	destination := image.NewAlpha(image.Rect(0, 0, width, height))
	budget := scanline.NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, destination); err != nil {
		panic(err)
	}
	return destination
}

func drawContractRGBA(t *testing.T, rasterizer *scanline.Rasterizer, destination *image.RGBA, source color.NRGBA) {
	t.Helper()
	budget := scanline.NewWorkBudget(math.MaxInt64)
	if err := rasterizer.DrawRGBA(context.Background(), &budget, destination, source); err != nil {
		t.Fatal(err)
	}
}

func writeContractAlpha(t *testing.T, rasterizer *scanline.Rasterizer, destination *image.Alpha) {
	t.Helper()
	budget := scanline.NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, destination); err != nil {
		t.Fatal(err)
	}
}

func assertContractAlpha(t *testing.T, image *image.Alpha, x, y int, want uint8) {
	t.Helper()
	if got := image.AlphaAt(x, y).A; got != want {
		t.Fatalf("alpha at (%d,%d) = %d, want %d", x, y, got, want)
	}
}
