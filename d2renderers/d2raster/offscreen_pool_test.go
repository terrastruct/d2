package d2raster

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestOffscreenImageCacheReusesAndClearsExactStorage(t *testing.T) {
	t.Parallel()

	budget := offscreenBudget{limit: 128}
	first, reservation, err := budget.newRGBA(image.Rect(3, 5, 7, 9), "test RGBA")
	if err != nil {
		t.Fatal(err)
	}
	for index := range first.Pix {
		first.Pix[index] = uint8(index + 1)
	}
	firstPixel := &first.Pix[0]
	budget.recycleRGBA(first, reservation)
	if budget.live != 0 || budget.cachedBytes != 64 || budget.cache.rgba == nil {
		t.Fatalf("cached first image: live=%d cached=%d entry=%v", budget.live, budget.cachedBytes, budget.cache.rgba != nil)
	}

	secondBounds := image.Rect(-4, -2, 0, 2)
	second, reservation, err := budget.newRGBA(secondBounds, "reused RGBA")
	if err != nil {
		t.Fatal(err)
	}
	if &second.Pix[0] != firstPixel {
		t.Fatal("equal-sized RGBA allocation did not reuse its backing storage")
	}
	if second.Bounds() != secondBounds || second.Stride != 16 {
		t.Fatalf("reused RGBA geometry = bounds %v stride %d", second.Bounds(), second.Stride)
	}
	for index, value := range second.Pix {
		if value != 0 {
			t.Fatalf("reused RGBA byte %d = %d, want zero", index, value)
		}
	}
	if budget.live != 64 || budget.cachedBytes != 0 || budget.cache.rgba != nil || budget.peak != 64 {
		t.Fatalf("active reused image: live=%d cached=%d entry=%v peak=%d", budget.live, budget.cachedBytes, budget.cache.rgba != nil, budget.peak)
	}
	alpha, alphaReservation, err := budget.newAlpha(image.Rect(0, 0, 8, 8), "same-sized Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if &alpha.Pix[0] == firstPixel {
		t.Fatal("Alpha allocation reused RGBA-formatted backing storage")
	}
	budget.recycleAlpha(alpha, alphaReservation)
	budget.recycleRGBA(second, reservation)
	if budget.live != 0 || budget.cachedBytes != 64 || budget.cache.rgba != second || budget.cache.alpha != nil {
		t.Fatalf("most-recent cache: live=%d cached=%d rgba=%v alpha=%v", budget.live, budget.cachedBytes, budget.cache.rgba != nil, budget.cache.alpha != nil)
	}
}

func TestOffscreenImageCacheRemainsInsideBudget(t *testing.T) {
	t.Parallel()

	budget := offscreenBudget{limit: 64}
	buffer, reservation, err := budget.newRGBA(image.Rect(0, 0, 4, 4), "cached RGBA")
	if err != nil {
		t.Fatal(err)
	}
	budget.recycleRGBA(buffer, reservation)
	if budget.cachedBytes != 64 {
		t.Fatalf("cached bytes = %d, want 64", budget.cachedBytes)
	}

	// A reservation of another kind cannot reuse the RGBA bytes. It must first
	// release the cache's strong reference so the retained working set remains
	// within the exact same limit as the preflight live-storage plan.
	reservation, err = budget.reserveBytes(64, "non-image storage")
	if err != nil {
		t.Fatal(err)
	}
	if budget.live != 64 || budget.cachedBytes != 0 || budget.cache.rgba != nil {
		t.Fatalf("post-eviction budget: live=%d cached=%d entry=%v", budget.live, budget.cachedBytes, budget.cache.rgba != nil)
	}
	budget.release(reservation)

	if _, _, err := budget.newAlpha(image.Rect(0, 0, 9, 8), "oversized Alpha"); err == nil {
		t.Fatal("oversized cache-backed allocation unexpectedly succeeded")
	}
	if budget.live != 0 || budget.cachedBytes != 0 {
		t.Fatalf("failed allocation changed budget: live=%d cached=%d", budget.live, budget.cachedBytes)
	}
}

func TestOffscreenImageCacheDoesNotAccumulateOneUseSizes(t *testing.T) {
	t.Parallel()

	budget := offscreenBudget{limit: 1 << 30}
	var last *image.RGBA
	for index := range 32 {
		bounds := image.Rect(0, 0, 64+index, 48+(index*17)%31)
		buffer, reservation, err := budget.newRGBA(bounds, "varied one-use test")
		if err != nil {
			t.Fatal(err)
		}
		if budget.cachedBytes != 0 || budget.cache.rgba != nil {
			t.Fatalf("allocation %d retained a mismatched cache entry", index)
		}
		if index != 0 && buffer == last {
			t.Fatalf("allocation %d reused mismatched image metadata", index)
		}
		budget.recycleRGBA(buffer, reservation)
		last = buffer
		if budget.live != 0 || budget.cachedBytes != reservation || budget.cache.rgba != buffer {
			t.Fatalf("recycle %d: live=%d cached=%d entry=%v", index, budget.live, budget.cachedBytes, budget.cache.rgba == buffer)
		}
		if budget.cachedBytes > budget.peak {
			t.Fatalf("recycle %d retained %d bytes above peak %d", index, budget.cachedBytes, budget.peak)
		}
	}
}

func BenchmarkRenderSequentialEffectLayers(b *testing.B) {
	root := d2scene.NewNode(nil)
	for index := range 32 {
		node := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{Width: 128, Height: 128},
			Fill: red,
		})
		node.Transform = d2scene.Translate(float64((index%4)*32), float64((index/4)*16))
		node.Opacity = .75
		root.Children = append(root.Children, node)
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 256, Height: 256}, root)
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

func BenchmarkRenderSequentialFilters(b *testing.B) {
	root := d2scene.NewNode(nil)
	shadow := color.NRGBA{R: 24, G: 32, B: 48, A: 180}
	for index := range 8 {
		node := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{Width: 64, Height: 64},
			Fill: red,
		})
		node.Transform = d2scene.Translate(float64((index%4)*40+12), float64((index/4)*96+12))
		node.Filters = []d2scene.Filter{
			d2scene.GaussianBlur{SigmaX: 2, SigmaY: 2},
			d2scene.DropShadow{OffsetX: 3, OffsetY: 3, SigmaX: 2, SigmaY: 2, Color: shadow},
		}
		root.Children = append(root.Children, node)
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 256, Height: 256}, root)
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

func BenchmarkOffscreenImageCacheVariedOneUseSizes(b *testing.B) {
	bounds := make([]image.Rectangle, 32)
	for index := range bounds {
		bounds[index] = image.Rect(0, 0, 64+index, 48+(index*17)%31)
	}
	b.Run("Cached", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			budget := offscreenBudget{limit: 1 << 30, peak: 1 << 30}
			for _, bounds := range bounds {
				buffer, reservation, err := budget.newRGBA(bounds, "varied one-use benchmark")
				if err != nil {
					b.Fatal(err)
				}
				benchmarkOffscreenImage = buffer
				budget.recycleRGBA(buffer, reservation)
			}
		}
	})
	b.Run("Uncached", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			budget := offscreenBudget{limit: 1 << 30}
			for _, bounds := range bounds {
				bytes, err := pixelStorageBytes(bounds, 4)
				if err != nil {
					b.Fatal(err)
				}
				reservation, err := budget.reserveBytes(bytes, "varied one-use benchmark")
				if err != nil {
					b.Fatal(err)
				}
				benchmarkOffscreenImage = image.NewRGBA(bounds)
				budget.release(reservation)
			}
		}
	})
}

var benchmarkOffscreenImage *image.RGBA
