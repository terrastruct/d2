package d2raster

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"testing"
)

func TestExpandedBlurKernelsMatchGeneralOracle(t *testing.T) {
	t.Parallel()
	state := uint32(0x9e3779b9)
	nextByte := func() byte {
		state = state*1664525 + 1013904223
		return byte(state >> 24)
	}
	for _, size := range []image.Point{{X: 1, Y: 1}, {X: 2, Y: 7}, {X: 17, Y: 9}, {X: 63, Y: 41}} {
		for _, radius := range []int{1, 2, 5, 19} {
			for _, axis := range []blurAxis{blurHorizontal, blurVertical} {
				name := fmt.Sprintf("%dx%d/radius_%d/axis_%d", size.X, size.Y, radius, axis)
				t.Run(name, func(t *testing.T) {
					sourceBounds := image.Rect(-11, 7, -11+size.X, 7+size.Y)
					xRadius, yRadius := 0, 0
					if axis == blurHorizontal {
						xRadius = radius
					} else {
						yRadius = radius
					}
					destinationBounds, err := expandFilterBounds(sourceBounds, xRadius, yRadius)
					if err != nil {
						t.Fatal(err)
					}
					pass := blurPass{axis: axis, radius: radius, bounds: destinationBounds}

					rgbaParentBounds := image.Rect(sourceBounds.Min.X-3, sourceBounds.Min.Y-2, sourceBounds.Max.X+5, sourceBounds.Max.Y+4)
					rgbaSource := image.NewRGBA(rgbaParentBounds).SubImage(sourceBounds).(*image.RGBA)
					for y := sourceBounds.Min.Y; y < sourceBounds.Max.Y; y++ {
						for x := sourceBounds.Min.X; x < sourceBounds.Max.X; x++ {
							offset := rgbaSource.PixOffset(x, y)
							for channel := range 4 {
								rgbaSource.Pix[offset+channel] = nextByte()
							}
						}
					}
					rgbaFast := paddedRGBA(destinationBounds)
					rgbaOracle := paddedRGBA(destinationBounds)
					if err := boxBlurRGBA(context.Background(), rgbaFast, rgbaSource, pass); err != nil {
						t.Fatal(err)
					}
					if err := boxBlurRGBAGeneral(context.Background(), rgbaOracle, rgbaSource, pass); err != nil {
						t.Fatal(err)
					}
					assertVisibleRowsEqual(t, rgbaFast.Pix, rgbaFast.Stride, rgbaOracle.Pix, rgbaOracle.Stride, destinationBounds.Dx()*4, destinationBounds.Dy())

					alphaSource := image.NewAlpha(rgbaParentBounds).SubImage(sourceBounds).(*image.Alpha)
					for y := sourceBounds.Min.Y; y < sourceBounds.Max.Y; y++ {
						for x := sourceBounds.Min.X; x < sourceBounds.Max.X; x++ {
							alphaSource.Pix[alphaSource.PixOffset(x, y)] = nextByte()
						}
					}
					alphaFast := paddedAlpha(destinationBounds)
					alphaOracle := paddedAlpha(destinationBounds)
					if err := boxBlurAlpha(context.Background(), alphaFast, alphaSource, pass); err != nil {
						t.Fatal(err)
					}
					if err := boxBlurAlphaPixelsGeneral(context.Background(), alphaOracle, alphaSource.Pix, alphaSource.Stride, alphaSource.Bounds(), 1, 0, pass); err != nil {
						t.Fatal(err)
					}
					assertVisibleRowsEqual(t, alphaFast.Pix, alphaFast.Stride, alphaOracle.Pix, alphaOracle.Stride, destinationBounds.Dx(), destinationBounds.Dy())

					alphaFromRGBAFast := paddedAlpha(destinationBounds)
					alphaFromRGBAOracle := paddedAlpha(destinationBounds)
					if err := boxBlurAlphaFromRGBA(context.Background(), alphaFromRGBAFast, rgbaSource, pass); err != nil {
						t.Fatal(err)
					}
					if err := boxBlurAlphaPixelsGeneral(context.Background(), alphaFromRGBAOracle, rgbaSource.Pix, rgbaSource.Stride, rgbaSource.Bounds(), 4, 3, pass); err != nil {
						t.Fatal(err)
					}
					assertVisibleRowsEqual(t, alphaFromRGBAFast.Pix, alphaFromRGBAFast.Stride, alphaFromRGBAOracle.Pix, alphaFromRGBAOracle.Stride, destinationBounds.Dx(), destinationBounds.Dy())
				})
			}
		}
	}
}

func TestExpandedBlurKernelsObserveCancellation(t *testing.T) {
	t.Parallel()
	sourceBounds := image.Rect(-7, 11, 505, 523)
	for _, axis := range []blurAxis{blurHorizontal, blurVertical} {
		xRadius, yRadius := 0, 0
		if axis == blurHorizontal {
			xRadius = 5
		} else {
			yRadius = 5
		}
		destinationBounds, err := expandFilterBounds(sourceBounds, xRadius, yRadius)
		if err != nil {
			t.Fatal(err)
		}
		pass := blurPass{axis: axis, radius: 5, bounds: destinationBounds}
		t.Run(fmt.Sprintf("RGBA/axis_%d", axis), func(t *testing.T) {
			ctx := &cancelAfterContext{after: 2}
			err := boxBlurRGBA(ctx, image.NewRGBA(destinationBounds), image.NewRGBA(sourceBounds), pass)
			if err != context.Canceled {
				t.Fatalf("cancellation error = %v, want context.Canceled", err)
			}
		})
		t.Run(fmt.Sprintf("Alpha/axis_%d", axis), func(t *testing.T) {
			ctx := &cancelAfterContext{after: 2}
			err := boxBlurAlpha(ctx, image.NewAlpha(destinationBounds), image.NewAlpha(sourceBounds), pass)
			if err != context.Canceled {
				t.Fatalf("cancellation error = %v, want context.Canceled", err)
			}
		})
	}
}

func assertVisibleRowsEqual(t *testing.T, got []byte, gotStride int, want []byte, wantStride int, width, height int) {
	t.Helper()
	for row := range height {
		if !bytes.Equal(got[row*gotStride:row*gotStride+width], want[row*wantStride:row*wantStride+width]) {
			t.Fatalf("row %d differs from the general blur kernel", row)
		}
	}
}

func BenchmarkBoxBlurExpanded(b *testing.B) {
	for _, size := range []int{64, 256, 1024} {
		for _, radius := range []int{2, 19} {
			for _, axis := range []blurAxis{blurHorizontal, blurVertical} {
				b.Run(fmt.Sprintf("%dx%d/radius_%d/axis_%d", size, size, radius, axis), func(b *testing.B) {
					source := image.NewRGBA(image.Rect(7, -13, 7+size, -13+size))
					for index := range source.Pix {
						source.Pix[index] = byte(index*37 + 11)
					}
					xRadius, yRadius := 0, 0
					if axis == blurHorizontal {
						xRadius = radius
					} else {
						yRadius = radius
					}
					bounds, err := expandFilterBounds(source.Bounds(), xRadius, yRadius)
					if err != nil {
						b.Fatal(err)
					}
					destination := image.NewRGBA(bounds)
					pass := blurPass{axis: axis, radius: radius, bounds: bounds}
					ctx := context.Background()
					b.ReportAllocs()
					b.SetBytes(int64(bounds.Dx() * bounds.Dy() * 4))
					b.ResetTimer()
					for range b.N {
						if err := boxBlurRGBA(ctx, destination, source, pass); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}
