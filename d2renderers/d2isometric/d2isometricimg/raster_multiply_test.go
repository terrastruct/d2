package d2isometricimg

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"testing"
)

func multiplyGroupQuad(depth float64, fill color.NRGBA) []Triangle {
	ts := rasterTestQuad(depth, &Material{Color: fill, Unlit: true, Multiply: true})
	for i := range ts {
		ts[i].NoDepthWrite = true
	}
	return ts
}

func TestNativeRasterMultiplyGroupsAndRaisedGeometry(t *testing.T) {
	ctx := context.Background()
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	groups := append(multiplyGroupQuad(0, color.NRGBA{R: 255, G: 128, B: 128, A: 255}), multiplyGroupQuad(.1, color.NRGBA{R: 128, G: 255, B: 128, A: 255})...)
	raster, err := NewRaster(ctx, 96, 96, groups, white)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := raster.Frame(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Red and green region tints both survive the overlap; ordinary over would
	// replace the first tint and leave green at 255.
	if got := frame.RGBAAt(44, 48); got != (color.RGBA{R: 128, G: 128, B: 64, A: 255}) {
		t.Fatalf("overlapping group tint %v", got)
	}
	actorColor := color.NRGBA{R: 17, G: 42, B: 218, A: 255}
	actor := rasterTestQuad(.5, &Material{Color: actorColor, Unlit: true})
	// Drawing surfaces that do not write depth after the actor must still test
	// its depth. They must not tint its raised, opaque front face.
	combined := append(append([]Triangle{}, groups...), actor...)
	withActor, err := NewRaster(ctx, 96, 96, combined, white)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := withActor.Frame(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := visible.RGBAAt(44, 48); got != (color.RGBA{R: 17, G: 42, B: 218, A: 255}) {
		t.Fatalf("group tinted raised actor: %v", got)
	}
	// Dynamic actors also keep their own color and must not alter cached groups.
	dynamic, err := raster.Frame(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if got := dynamic.RGBAAt(44, 48); got != (color.RGBA{R: 17, G: 42, B: 218, A: 255}) {
		t.Fatalf("dynamic actor tinted: %v", got)
	}
	again, err := raster.Frame(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again.Pix, frame.Pix) {
		t.Fatal("dynamic frame changed group cache")
	}
}

func TestNativeRasterMultiplyOpacityAndTexture(t *testing.T) {
	ctx := context.Background()
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	groups := append(multiplyGroupQuad(0, color.NRGBA{R: 255, G: 128, B: 128, A: 128}), multiplyGroupQuad(.1, color.NRGBA{R: 128, G: 255, B: 128, A: 128})...)
	raster, err := NewRaster(ctx, 64, 64, groups, white)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := raster.Frame(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.RGBAAt(28, 32); got != (color.RGBA{R: 191, G: 191, B: 143, A: 255}) {
		t.Fatalf("translucent multiply %v", got)
	}
	texture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			texture.SetRGBA(x, y, color.RGBA{R: 128, G: 64, B: 64, A: 128})
		}
	}
	ts := rasterTestQuad(0, &Material{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 128}, Texture: texture, Unlit: true, Multiply: true})
	for i := range ts {
		ts[i].NoDepthWrite = true
	}
	raster, err = NewRaster(ctx, 64, 64, ts, white)
	if err != nil {
		t.Fatal(err)
	}
	frame, err = raster.Frame(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Texture alpha and material opacity each apply once; RGB interpolation
	// remains premultiplied until rasterTexture produces the straight source.
	if got := frame.RGBAAt(28, 32); got != (color.RGBA{R: 255, G: 223, B: 223, A: 255}) {
		t.Fatalf("texture alpha/color %v", got)
	}
	transparent := multiplyGroupQuad(0, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
	raster, err = NewRaster(ctx, 64, 64, transparent, white)
	if err != nil {
		t.Fatal(err)
	}
	frame, err = raster.Frame(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.RGBAAt(28, 32); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("transparent group changed background: %v", got)
	}
}

func TestRasterMultiplyPremultipliedBackdrop(t *testing.T) {
	// Native export currently resolves its canvas to opaque. Exercise the
	// compositor's transparent-backdrop math independently so it remains valid
	// with premultiplied image.RGBA rather than relying on that canvas default.
	for _, tc := range []struct {
		name  string
		dst   color.RGBA
		src   [3]float64
		alpha float64
		want  color.RGBA
	}{
		{"empty backdrop", color.RGBA{}, [3]float64{1, .5, .25}, .5, color.RGBA{R: 128, G: 64, B: 32, A: 128}},
		{"partial backdrop", color.RGBA{R: 32, G: 64, B: 96, A: 128}, [3]float64{1, .5, .25}, .5, color.RGBA{R: 96, G: 80, B: 76, A: 192}},
		{"opaque source", color.RGBA{R: 32, G: 64, B: 96, A: 128}, [3]float64{0, .5, 1}, 1, color.RGBA{R: 0, G: 96, B: 223, A: 255}},
		{"transparent source", color.RGBA{R: 32, G: 64, B: 96, A: 128}, [3]float64{1, 0, 0}, 0, color.RGBA{R: 32, G: 64, B: 96, A: 128}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pixel := []byte{tc.dst.R, tc.dst.G, tc.dst.B, tc.dst.A}
			rasterMultiplyOver(pixel, tc.src[0], tc.src[1], tc.src[2], tc.alpha)
			got := color.RGBA{R: pixel[0], G: pixel[1], B: pixel[2], A: pixel[3]}
			if got != tc.want {
				t.Fatalf("got %v; want %v", got, tc.want)
			}
			if got.R > got.A || got.G > got.A || got.B > got.A {
				t.Fatal("invalid premultiplied color")
			}
		})
	}
}

func TestNativeRasterNormalMaterialOverUnchanged(t *testing.T) {
	material := &Material{Color: color.NRGBA{R: 64, G: 128, B: 192, A: 128}, Unlit: true}
	raster, err := NewRaster(context.Background(), 64, 64, rasterTestQuad(0, material), color.NRGBA{R: 200, G: 150, B: 100, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := raster.Frame(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.RGBAAt(28, 32); got != (color.RGBA{R: 132, G: 139, B: 146, A: 255}) {
		t.Fatalf("ordinary source-over changed: %v", got)
	}
}
