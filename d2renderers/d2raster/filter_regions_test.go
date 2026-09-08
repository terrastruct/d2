package d2raster

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math/rand/v2"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestRegionalFiltersMatchCompleteLayers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	random := rand.New(rand.NewPCG(71, 912))
	for caseIndex := range 48 {
		input := image.Rect(-7, -5, 19, 26)
		source := image.NewRGBA(input)
		for y := input.Min.Y; y < input.Max.Y; y++ {
			for x := input.Min.X; x < input.Max.X; x++ {
				alpha := uint8(random.IntN(256))
				source.SetRGBA(x, y, color.RGBA{R: uint8(random.IntN(int(alpha) + 1)), G: uint8(random.IntN(int(alpha) + 1)), B: uint8(random.IntN(int(alpha) + 1)), A: alpha})
			}
		}
		var normalized []normalizedFilter
		for range 1 + caseIndex%4 {
			filter := normalizedFilter{sigmaX: float64(random.IntN(5)) / 2, sigmaY: float64(random.IntN(5)) / 2}
			if random.IntN(2) == 0 {
				filter.kind = preparedDropShadow
				filter.offsetX = float64(random.IntN(73)-36) / 2
				filter.offsetY = float64(random.IntN(73)-36) / 2
				filter.shadowColor = color.NRGBA{R: 117, G: 41, B: 203, A: 197}
			}
			normalized = append(normalized, filter)
		}
		filters, bounds, err := (&preflight{ctx: ctx}).prepareFilters("test", normalized, d2scene.Identity(), input)
		if err != nil {
			t.Fatal(err)
		}
		if len(filters) == 0 {
			continue
		}
		fullScratch := &rasterScratch{offscreen: offscreenBudget{limit: 1 << 30}}
		complete := renderFilterRegionForTest(t, source, filters, input, fullScratch)
		for y := bounds.Min.Y - 2; y < bounds.Max.Y+2; y += 7 {
			region := image.Rect(bounds.Min.X+caseIndex%11, y, bounds.Max.X-caseIndex%13, y+3).Intersect(bounds)
			cropped, required, err := regionalFilters(filters, input, region)
			if err != nil {
				t.Fatal(err)
			}
			scratch := &rasterScratch{offscreen: offscreenBudget{limit: 1 << 30}}
			got := renderFilterRegionForTest(t, source, cropped, required, scratch)
			for y := region.Min.Y; y < region.Max.Y; y++ {
				for x := region.Min.X; x < region.Max.X; x++ {
					if got.image.RGBAAt(x, y) != complete.image.RGBAAt(x, y) {
						t.Fatalf("case %d, region %v, pixel (%d,%d) = %v, want %v; filters %+v", caseIndex, region, x, y, got.image.RGBAAt(x, y), complete.image.RGBAAt(x, y), filters)
					}
				}
			}
			peak, finalBytes, err := planFilterResources(cropped, required)
			if err != nil {
				t.Fatal(err)
			}
			if scratch.offscreen.peak != peak || scratch.offscreen.live != finalBytes {
				t.Fatalf("case %d: filter storage live/peak = %d/%d, want %d/%d", caseIndex, scratch.offscreen.live, scratch.offscreen.peak, finalBytes, peak)
			}
			got.release()
		}
		complete.release()
	}
}

func renderFilterRegionForTest(t *testing.T, source *image.RGBA, filters []preparedFilter, input image.Rectangle, scratch *rasterScratch) ownedRGBA {
	t.Helper()
	current, err := reserveRGBA(scratch, input, "test filter input")
	if err != nil {
		t.Fatal(err)
	}
	draw.Draw(current.image, input, source, input.Min, draw.Src)
	for _, filter := range filters {
		if err := applyPreparedFilter(context.Background(), &current, filter, scratch); err != nil {
			t.Fatal(err)
		}
	}
	return current
}

func TestRegionalFilterBandsNestedEffects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, fillRule := range []d2scene.FillRule{d2scene.NonZero, d2scene.EvenOdd} {
		t.Run(fmt.Sprint(fillRule), func(t *testing.T) {
			child := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 11.25, Y: -9.5, Width: 45.75, Height: 114}, Fill: red})
			child.Filters = []d2scene.Filter{
				d2scene.DropShadow{OffsetX: -7.5, OffsetY: 11.25, SigmaX: 1.5, SigmaY: 2, Color: color.NRGBA{G: 199, A: 177}},
				d2scene.GaussianBlur{SigmaX: 1, SigmaY: .5},
			}
			child.Opacity = .7
			other := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 32.5, Y: 28.5, Width: 32, Height: 62}, Fill: d2scene.SolidPaint{Color: color.NRGBA{B: 255, A: 151}}})
			root := d2scene.NewNode(nil)
			root.Children = []*d2scene.Node{child, other}
			root.Filters = []d2scene.Filter{
				d2scene.GaussianBlur{SigmaX: .5, SigmaY: 1.5},
				d2scene.DropShadow{OffsetX: 4.25, OffsetY: -6.5, SigmaX: 1, SigmaY: 1, Color: color.NRGBA{A: 215}},
			}
			root.Clip = &d2scene.Clip{Path: clipRect(4.5, 3.5, 70, 92, fillRule), Transform: d2scene.Identity()}
			mask := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 2, Y: 7, Width: 71, Height: 80}, Fill: red})
			mask.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1}}
			root.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: mask, Transform: d2scene.Identity()}
			root.Opacity = .8
			document := d2scene.NewDocument(d2scene.Box{Width: 80, Height: 100}, root)
			want, err := Render(ctx, document, testOptions())
			if err != nil {
				t.Fatal(err)
			}
			for _, height := range []int{1, 7, 31} {
				got := image.NewNRGBA(want.Bounds())
				err := RenderBands(ctx, document, testOptions(), height, func(band *image.NRGBA) error {
					draw.Draw(got, band.Bounds(), band, band.Bounds().Min, draw.Src)
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got.Pix, want.Pix) {
					for y := range want.Bounds().Dy() {
						for x := range want.Bounds().Dx() {
							if got.NRGBAAt(x, y) != want.NRGBAAt(x, y) {
								t.Fatalf("band height %d, pixel (%d,%d) = %v, want %v", height, x, y, got.NRGBAAt(x, y), want.NRGBAAt(x, y))
							}
						}
					}
				}
			}
		})
	}
}

func TestRegionalFilterStorageDoesNotGrowWithImageHeight(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var previousPeak int64
	for _, height := range []int{1_000, 100_000, 1_000_000} {
		input := image.Rect(0, 0, 500, height)
		filters, _, err := (&preflight{ctx: ctx}).prepareFilters("test", []normalizedFilter{
			{kind: preparedGaussianBlur, sigmaX: 2, sigmaY: 3},
			{kind: preparedDropShadow, sigmaX: 3, sigmaY: 4, offsetX: -3.5, offsetY: 8.25, shadowColor: color.NRGBA{A: 200}},
		}, d2scene.Identity(), input)
		if err != nil {
			t.Fatal(err)
		}
		cropped, required, err := regionalFilters(filters, input, image.Rect(0, height/2, 500, height/2+32))
		if err != nil {
			t.Fatal(err)
		}
		peak, _, err := planFilterResources(cropped, required)
		if err != nil {
			t.Fatal(err)
		}
		if peak > 500*256*4 {
			t.Fatalf("height %d: regional peak %d exceeds bounded filter storage", height, peak)
		}
		if previousPeak != 0 && previousPeak != peak {
			t.Fatalf("height %d: regional peak grew from %d to %d", height, previousPeak, peak)
		}
		previousPeak = peak
	}
}
