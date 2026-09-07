package d2isometricimg

import (
	"bytes"
	"context"
	"image"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2target"
)

func TestNativeVectorSolidCapClosesOnlyOpaqueSourceMargin(t *testing.T) {
	n := solidTestNode(d2target.ShapeCylinder)
	n.StrokeWidth = 4
	n.Metadata.Original.FillPattern = "lines"
	build := func(ctx context.Context) nativeSolidPaint {
		b := &meshBuilder{ctx: ctx, scale: .01}
		paint := b.solidPaint(n, d2target.ShapeOval, n.Fill)
		if b.err != nil {
			t.Fatal(b.err)
		}
		return paint
	}
	ctx := nativeVectorContext(context.Background())
	paint, raster := build(ctx), build(context.Background())
	for _, pair := range [][2]*Material{{paint.cap, raster.cap}, {paint.wall, raster.wall}} {
		if !bytes.Equal(pair[0].Texture.(*image.RGBA).Pix, pair[1].Texture.(*image.RGBA).Pix) {
			t.Fatal("vector cap fill changed native raster pixels")
		}
	}
	if paint.wall.Vector.capBackground != nil {
		t.Fatal("cap fill changed the unwrapped wall")
	}
	original := nativeVectorForTexture(ctx, paint.cap.Texture.(*image.RGBA))
	if original.capBackground != nil {
		t.Fatal("cap fill mutated the retained texture source")
	}
	fragment, err := nativeSurfaceSVG(ctx, paint.cap.Vector, "cap")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fragment, "<image") || !strings.Contains(fragment, "<pattern") {
		t.Fatal("cap fill flattened the source pattern")
	}
	// The native SVG importer intentionally does not admit pattern servers.
	// Inspect that artwork above, then sample the same plain source cap below.
	n.Metadata.Original.FillPattern = ""
	paint = build(ctx)
	original = nativeVectorForTexture(ctx, paint.cap.Texture.(*image.RGBA))
	fragment, err = nativeSurfaceSVG(ctx, paint.cap.Vector, "cap")
	if err != nil {
		t.Fatal(err)
	}
	before, err := nativeSurfaceSVG(ctx, original, "before")
	if err != nil {
		t.Fatal(err)
	}
	// This point lies just inside the physical ellipse, but outside the fill
	// inset produced by the source's centered-stroke viewport. The outer mesh
	// clip still controls the cap silhouette in the final SVG.
	size := image.Rect(0, 0, 512, 512)
	if p := rasterVectorFragment(t, before, size).NRGBAAt(2, 256); p.A != 0 {
		t.Fatalf("fixture no longer has an inset fill margin: %v", p)
	}
	if p := rasterVectorFragment(t, fragment, size).NRGBAAt(2, 256); p != nativePaint(n.Fill, "transparent") {
		t.Fatalf("opaque physical cap has a transparent or mistinted margin: %v", p)
	}
	for _, fill := range []string{"transparent", "rgba(10, 30, 50, 0.5)"} {
		if got := nativeVectorSolidCap(original, nativePaint(fill, "transparent")); got != original {
			t.Fatalf("cap fill made authored translucent paint opaque: %s", fill)
		}
	}
}
