package scanline

import (
	"context"
	"image"
	"image/color"
	"testing"
)

func TestReferenceOriginPreservesFullTargetRounding(t *testing.T) {
	const width, height = 24, 30_020
	full := NewRasterizer(width, height)
	full.MoveTo(3, float32(30_000.002))
	full.LineTo(20, float32(30_000.002))
	full.LineTo(20, float32(30_010.002))
	full.LineTo(3, float32(30_010.002))
	full.ClosePath()
	want := image.NewRGBA(image.Rect(0, 0, width, height))
	work := NewWorkBudget(1 << 30)
	if err := full.DrawRGBA(context.Background(), &work, want, color.NRGBA{R: 255, A: 255}); err != nil {
		t.Fatal(err)
	}

	for _, top := range []int{29_995, 30_000, 30_007} {
		band := NewRasterizer(width, 7)
		band.SetOriginOffset(0, -float64(top))
		band.MoveTo64(3, 30_000.002)
		band.LineTo64(20, 30_000.002)
		band.LineTo64(20, 30_010.002)
		band.LineTo64(3, 30_010.002)
		band.ClosePath()
		got := image.NewRGBA(image.Rect(0, top, width, top+7))
		work := NewWorkBudget(1 << 30)
		if err := band.DrawRGBA(context.Background(), &work, got, color.NRGBA{R: 255, A: 255}); err != nil {
			t.Fatal(err)
		}
		for y := top; y < top+7; y++ {
			for x := range width {
				if got.RGBAAt(x, y) != want.RGBAAt(x, y) {
					t.Fatalf("band %d pixel (%d,%d) = %v, want %v", top, x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
				}
			}
		}
	}
}

func TestReferenceOriginCurveAndReset(t *testing.T) {
	full := NewRasterizer(50, 30_050)
	full.MoveTo(3, float32(30_000.002))
	full.CubeTo(4, float32(30_015.129), 31, float32(30_028.321), 45, float32(30_000.417))
	full.ClosePath()
	band := NewRasterizer(50, 17)
	band.SetOriginOffset(0, -30_000)
	band.MoveTo64(3, 30_000.002)
	band.CubeTo64(4, 30_015.129, 31, 30_028.321, 45, 30_000.417)
	band.ClosePath()
	if len(band.edges) != len(full.edges) {
		t.Fatalf("translated curve edges = %d, want %d", len(band.edges), len(full.edges))
	}
	for index, original := range full.edges {
		original.from.y -= 30_000
		original.to.y -= 30_000
		if band.edges[index] != original {
			t.Fatalf("translated curve edge %d = %+v, want %+v", index, band.edges[index], original)
		}
	}
	band.Reset(50, 50)
	band.MoveTo64(3, 4)
	if band.current != (point{x: 3, y: 4}) {
		t.Fatalf("Reset retained origin offset: %+v", band.current)
	}
	band.SetOriginOffset(100, 200)
	band.MoveTo(3, 4)
	if band.current != (point{x: 3, y: 4}) {
		t.Fatalf("float32 API used reference translation: %+v", band.current)
	}
}
