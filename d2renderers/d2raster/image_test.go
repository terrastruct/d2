package d2raster

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestRasterImageFormatsAndAlpha(t *testing.T) {
	opaque := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			opaque.SetNRGBA(x, y, color.NRGBA{R: 180, G: 70, B: 30, A: 255})
		}
	}
	palette := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{
		color.NRGBA{}, color.NRGBA{R: 220, G: 40, B: 20, A: 255},
	})
	for index := range palette.Pix {
		palette.Pix[index] = 1
	}
	webp, err := base64.StdEncoding.DecodeString(rasterTestWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mime   string
		data   []byte
		width  int
		height int
	}{
		{name: "PNG", mime: "image/png", data: encodeRasterPNG(t, opaque), width: 4, height: 4},
		{name: "JPEG", mime: "image/jpeg", data: encodeRasterJPEG(t, opaque), width: 4, height: 4},
		{name: "GIF", mime: "image/gif", data: encodeRasterGIF(t, palette), width: 4, height: 4},
		{name: "WebP", mime: "image/webp", data: webp, width: 75, height: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := rasterImageDocument(test.mime, test.data, test.width, test.height, d2scene.Box{Width: 4, Height: 4}, d2scene.AspectRatio{})
			frame, err := Render(context.Background(), document, testOptions())
			if err != nil {
				t.Fatal(err)
			}
			if frame.NRGBAAt(2, 2).A == 0 {
				t.Fatalf("decoded %s image rendered transparent", test.name)
			}
		})
	}

	partialFrame := image.NewPaletted(image.Rect(1, 1, 3, 3), color.Palette{color.NRGBA{}, color.NRGBA{G: 255, A: 255}})
	for index := range partialFrame.Pix {
		partialFrame.Pix[index] = 1
	}
	var partialGIF bytes.Buffer
	if err := gif.EncodeAll(&partialGIF, &gif.GIF{
		Image: []*image.Paletted{partialFrame}, Delay: []int{0},
		Config: image.Config{ColorModel: partialFrame.Palette, Width: 4, Height: 4},
	}); err != nil {
		t.Fatal(err)
	}
	partialDocument := rasterImageDocument("image/gif", partialGIF.Bytes(), 4, 4, d2scene.Box{Width: 4, Height: 4}, d2scene.AspectRatio{})
	partial, err := Render(context.Background(), partialDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if partial.NRGBAAt(0, 0).A != 0 || partial.NRGBAAt(1, 1).G < 250 {
		t.Fatalf("single-frame GIF logical canvas = corner %#v, frame %#v", partial.NRGBAAt(0, 0), partial.NRGBAAt(1, 1))
	}
	animatedDocument := rasterImageDocument("image/gif", encodeAnimatedRasterGIF(t), 2, 2, d2scene.Box{Width: 2, Height: 2}, d2scene.AspectRatio{})
	animated, err := Render(context.Background(), animatedDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if pixel := animated.NRGBAAt(1, 1); pixel.R < 250 || pixel.G < 250 || pixel.B < 250 || pixel.A != 255 {
		t.Fatalf("animated GIF rendered %#v, want its white first frame", pixel)
	}

	translucent := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for index := 0; index < len(translucent.Pix); index += 4 {
		translucent.Pix[index] = 255
		translucent.Pix[index+3] = 128
	}
	document := rasterImageDocument("image/png", encodeRasterPNG(t, translucent), 4, 4, d2scene.Box{Width: 4, Height: 4}, d2scene.AspectRatio{})
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	pixel := frame.NRGBAAt(1, 1)
	if pixel.A != 128 || pixel.R < 250 || pixel.G != 0 || pixel.B != 0 {
		t.Fatalf("translucent pixel = %#v, want unpremultiplied red at alpha 128", pixel)
	}
}

func TestImageAspectPlacementAllAlignments(t *testing.T) {
	tests := []struct {
		align d2scene.AspectAlign
		meet  d2scene.Box
		slice d2scene.Box
	}{
		{d2scene.AlignXMinYMin, d2scene.Box{Width: 10, Height: 5}, d2scene.Box{Width: 20, Height: 10}},
		{d2scene.AlignXMidYMin, d2scene.Box{Width: 10, Height: 5}, d2scene.Box{X: -5, Width: 20, Height: 10}},
		{d2scene.AlignXMaxYMin, d2scene.Box{Width: 10, Height: 5}, d2scene.Box{X: -10, Width: 20, Height: 10}},
		{d2scene.AlignXMinYMid, d2scene.Box{Y: 2.5, Width: 10, Height: 5}, d2scene.Box{Width: 20, Height: 10}},
		{d2scene.AlignXMidYMid, d2scene.Box{Y: 2.5, Width: 10, Height: 5}, d2scene.Box{X: -5, Width: 20, Height: 10}},
		{d2scene.AlignXMaxYMid, d2scene.Box{Y: 2.5, Width: 10, Height: 5}, d2scene.Box{X: -10, Width: 20, Height: 10}},
		{d2scene.AlignXMinYMax, d2scene.Box{Y: 5, Width: 10, Height: 5}, d2scene.Box{Width: 20, Height: 10}},
		{d2scene.AlignXMidYMax, d2scene.Box{Y: 5, Width: 10, Height: 5}, d2scene.Box{X: -5, Width: 20, Height: 10}},
		{d2scene.AlignXMaxYMax, d2scene.Box{Y: 5, Width: 10, Height: 5}, d2scene.Box{X: -10, Width: 20, Height: 10}},
	}
	for _, test := range tests {
		meet, err := imagePlacement(d2scene.Box{Width: 10, Height: 10}, 4, 2, d2scene.AspectRatio{Align: test.align, Fit: d2scene.AspectMeet})
		if err != nil {
			t.Fatal(err)
		}
		if meet != test.meet {
			t.Errorf("alignment %d meet = %+v, want %+v", test.align, meet, test.meet)
		}
		slice, err := imagePlacement(d2scene.Box{Width: 10, Height: 10}, 4, 2, d2scene.AspectRatio{Align: test.align, Fit: d2scene.AspectSlice})
		if err != nil {
			t.Fatal(err)
		}
		if slice != test.slice {
			t.Errorf("alignment %d slice = %+v, want %+v", test.align, slice, test.slice)
		}
	}
	box := d2scene.Box{X: 1, Y: 2, Width: 10, Height: 7}
	stretch, err := imagePlacement(box, 4, 2, d2scene.AspectRatio{Align: d2scene.AlignNone, Fit: d2scene.AspectSlice})
	if err != nil || stretch != box {
		t.Fatalf("AlignNone placement = %+v, %v; want stretch %+v", stretch, err, box)
	}
}

func TestImageStretchMeetSliceAndSliceClipping(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	source.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	data := encodeRasterPNG(t, source)

	stretch, err := Render(context.Background(), rasterImageDocument(
		"image/png", data, 2, 1, d2scene.Box{Width: 4, Height: 4},
		d2scene.AspectRatio{Align: d2scene.AlignNone},
	), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if stretch.NRGBAAt(0, 0).R <= stretch.NRGBAAt(0, 0).B || stretch.NRGBAAt(3, 3).B <= stretch.NRGBAAt(3, 3).R {
		t.Fatalf("stretch did not cover the box with expected edge colors: left=%#v right=%#v", stretch.NRGBAAt(0, 0), stretch.NRGBAAt(3, 3))
	}

	meet, err := Render(context.Background(), rasterImageDocument(
		"image/png", data, 2, 1, d2scene.Box{Width: 4, Height: 4},
		d2scene.AspectRatio{Align: d2scene.AlignXMidYMid, Fit: d2scene.AspectMeet},
	), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if meet.NRGBAAt(2, 0).A != 0 || meet.NRGBAAt(2, 1).A == 0 || meet.NRGBAAt(2, 3).A != 0 {
		t.Fatalf("meet letterboxing is wrong: top=%#v middle=%#v bottom=%#v", meet.NRGBAAt(2, 0), meet.NRGBAAt(2, 1), meet.NRGBAAt(2, 3))
	}

	minSlice, err := Render(context.Background(), rasterImageDocument(
		"image/png", data, 2, 1, d2scene.Box{Width: 4, Height: 4},
		d2scene.AspectRatio{Align: d2scene.AlignXMinYMin, Fit: d2scene.AspectSlice},
	), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	maxSlice, err := Render(context.Background(), rasterImageDocument(
		"image/png", data, 2, 1, d2scene.Box{Width: 4, Height: 4},
		d2scene.AspectRatio{Align: d2scene.AlignXMaxYMin, Fit: d2scene.AspectSlice},
	), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if minSlice.NRGBAAt(2, 2).R <= minSlice.NRGBAAt(2, 2).B {
		t.Fatalf("xMin slice center = %#v, want red-dominant left crop", minSlice.NRGBAAt(2, 2))
	}
	if maxSlice.NRGBAAt(1, 2).B <= maxSlice.NRGBAAt(1, 2).R {
		t.Fatalf("xMax slice center = %#v, want blue-dominant right crop", maxSlice.NRGBAAt(1, 2))
	}
	// Slice content is larger than its viewport, but must never paint beyond
	// the destination box.
	document := rasterImageDocument("image/png", data, 2, 1, d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4}, d2scene.AspectRatio{Align: d2scene.AlignXMidYMid, Fit: d2scene.AspectSlice})
	document.ViewBox = d2scene.Box{Width: 8, Height: 8}
	document.LogicalWidth, document.LogicalHeight = 8, 8
	clipped, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if clipped.NRGBAAt(1, 3).A != 0 || clipped.NRGBAAt(6, 3).A != 0 || clipped.NRGBAAt(3, 3).A == 0 {
		t.Fatalf("slice viewport clip: left=%#v inside=%#v right=%#v", clipped.NRGBAAt(1, 3), clipped.NRGBAAt(3, 3), clipped.NRGBAAt(6, 3))
	}

	reflectedDocument := rasterImageDocument("image/png", data, 2, 1, d2scene.Box{Width: 2, Height: 1}, d2scene.AspectRatio{})
	reflectedDocument.Root.Transform = d2scene.Matrix{A: -1, D: 1, E: 2}
	reflected, err := Render(context.Background(), reflectedDocument, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if reflected.NRGBAAt(0, 0).B <= reflected.NRGBAAt(0, 0).R || reflected.NRGBAAt(1, 0).R <= reflected.NRGBAAt(1, 0).B {
		t.Fatalf("reflected image did not reverse texels: left=%#v right=%#v", reflected.NRGBAAt(0, 0), reflected.NRGBAAt(1, 0))
	}
}

func TestImageAffineTransformClipMaskOpacityAndBlend(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for index := 0; index < len(source.Pix); index += 4 {
		source.Pix[index] = 255
		source.Pix[index+3] = 255
	}
	assetID := d2scene.AssetID("image")
	imageNode := d2scene.NewNode(d2scene.Image{Asset: assetID, Box: d2scene.Box{Width: 2, Height: 2}})
	imageNode.Transform = d2scene.Matrix{A: 2, B: .5, C: .25, D: 2, E: 3, F: 2}
	imageNode.Opacity = .5
	imageNode.Blend = d2scene.BlendMultiply
	imageNode.Clip = &d2scene.Clip{Transform: d2scene.Identity(), Path: d2scene.Path{Commands: []d2scene.PathCommand{
		d2scene.MoveTo(0, 0), d2scene.LineTo(1, 0), d2scene.LineTo(1, 2), d2scene.LineTo(0, 2), d2scene.ClosePath(),
	}}}
	imageNode.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Transform: d2scene.Identity(), Root: d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 2, Height: 1.5}, Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 255}},
	})}
	background := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 12, Height: 10}, Fill: d2scene.SolidPaint{Color: color.NRGBA{B: 255, A: 255}},
	})
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{background, imageNode}
	document := d2scene.NewDocument(d2scene.Box{Width: 12, Height: 10}, root)
	data := encodeRasterPNG(t, source)
	document.Assets[assetID] = d2scene.RasterAsset{MIMEType: "image/png", Data: data, PixelWidth: 2, PixelHeight: 2}
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	inside := frame.NRGBAAt(4, 3)
	outsideClip := frame.NRGBAAt(7, 4)
	if inside == (color.NRGBA{B: 255, A: 255}) {
		t.Fatalf("transformed image did not pass through mask/opacity/blend at inside pixel: %#v", inside)
	}
	if outsideClip != (color.NRGBA{B: 255, A: 255}) {
		t.Fatalf("image escaped transformed clip: %#v", outsideClip)
	}

	for _, transform := range []d2scene.Matrix{
		d2scene.Identity(),
		d2scene.Translate(7, -3).Mul(d2scene.Rotate(.37)).Mul(d2scene.Scale(2, .75)),
		{A: -2, B: .25, C: .5, D: 3, E: 9, F: -4},
	} {
		inverse, invertible, err := makeInverseAffine(transform)
		if err != nil || !invertible {
			t.Fatalf("makeInverseAffine(%+v) = %+v, %v, %v", transform, inverse, invertible, err)
		}
		point := d2scene.Point{X: 1.25, Y: -2.5}
		roundTrip, ok := inverse.point(transform.Point(point))
		if !ok || !closeFloat(roundTrip.X, point.X) || !closeFloat(roundTrip.Y, point.Y) {
			t.Fatalf("affine round trip %+v -> %+v, want %+v", transform.Point(point), roundTrip, point)
		}
	}
}

func TestRasterImagePreflightErrors(t *testing.T) {
	static := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	pngData := encodeRasterPNG(t, static)
	jpegData := encodeRasterJPEG(t, static)
	animatedPNG := insertRasterPNGChunk(t, pngData, "acTL", []byte{0, 0, 0, 2, 0, 0, 0, 0})
	animatedWebP := make([]byte, 30)
	copy(animatedWebP, "RIFF")
	binary.LittleEndian.PutUint32(animatedWebP[4:8], 22)
	copy(animatedWebP[8:12], "WEBP")
	copy(animatedWebP[12:16], "VP8X")
	binary.LittleEndian.PutUint32(animatedWebP[16:20], 10)
	animatedWebP[20] = 0x02

	tests := []struct {
		name  string
		asset d2scene.Asset
		want  string
	}{
		{name: "nil raster", asset: (*d2scene.RasterAsset)(nil), want: "is nil"},
		{name: "empty data", asset: d2scene.RasterAsset{MIMEType: "image/png", PixelWidth: 2, PixelHeight: 2}, want: "has no data"},
		{name: "bad dimensions", asset: d2scene.RasterAsset{MIMEType: "image/png", Data: pngData}, want: "invalid declared dimensions"},
		{name: "bad MIME", asset: d2scene.RasterAsset{MIMEType: "application/octet-stream", Data: pngData, PixelWidth: 2, PixelHeight: 2}, want: "unsupported MIME"},
		{name: "MIME mismatch", asset: d2scene.RasterAsset{MIMEType: "image/png", Data: jpegData, PixelWidth: 2, PixelHeight: 2}, want: "JPEG signature"},
		{name: "malformed body", asset: d2scene.RasterAsset{MIMEType: "image/png", Data: pngData[:12], PixelWidth: 2, PixelHeight: 2}, want: "malformed PNG"},
		{name: "metadata mismatch", asset: d2scene.RasterAsset{MIMEType: "image/png", Data: pngData, PixelWidth: 3, PixelHeight: 2}, want: "encoded data is 2x2"},
		{name: "decoded footprint", asset: d2scene.RasterAsset{MIMEType: "image/png", Data: pngData, PixelWidth: 2, PixelHeight: 2, DecodedBytes: 15}, want: "below the required 16"},
		{name: "incomplete APNG", asset: d2scene.RasterAsset{MIMEType: "image/png", Data: animatedPNG, PixelWidth: 2, PixelHeight: 2}, want: "invalid APNG"},
		{name: "incomplete animated WebP", asset: d2scene.RasterAsset{MIMEType: "image/webp", Data: animatedWebP, PixelWidth: 1, PixelHeight: 1}, want: "incomplete animated WebP"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := d2scene.NewDocument(d2scene.Box{Width: 2, Height: 2}, d2scene.NewNode(d2scene.Image{Asset: "asset", Box: d2scene.Box{Width: 2, Height: 2}}))
			document.Assets["asset"] = test.asset
			_, err := Render(context.Background(), document, testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Render error = %v, want substring %q", err, test.want)
			}
		})
	}

	missing := d2scene.NewDocument(d2scene.Box{Width: 2, Height: 2}, d2scene.NewNode(d2scene.Image{Asset: "missing", Box: d2scene.Box{Width: 2, Height: 2}}))
	if _, err := Render(context.Background(), missing, testOptions()); err == nil || !strings.Contains(err.Error(), "missing asset") {
		t.Fatalf("missing image error = %v", err)
	}
	invalidAspect := rasterImageDocument("image/png", pngData, 2, 2, d2scene.Box{Width: 2, Height: 2}, d2scene.AspectRatio{Align: d2scene.AspectAlign(255)})
	if _, err := Render(context.Background(), invalidAspect, testOptions()); err == nil || !strings.Contains(err.Error(), "invalid aspect-ratio") {
		t.Fatalf("invalid aspect error = %v", err)
	}
}

func TestRasterImageDecodedStorageLimitAndRepeatability(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 50), G: uint8(y * 50), B: 120, A: uint8(100 + x*30)})
		}
	}
	document := rasterImageDocument("image/png", encodeRasterPNG(t, img), 4, 4, d2scene.Box{Width: 4, Height: 4}, d2scene.AspectRatio{})
	options := testOptions()
	options.MaxDecodedAssetBytes = 63
	if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "requires 64 bytes") {
		t.Fatalf("below-limit Render error = %v", err)
	}
	options.MaxDecodedAssetBytes = 64
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.resources.peakOffscreenBytes != 0 {
		t.Fatalf("image-only offscreen peak = %d, want decoded storage tracked separately", prepared.resources.peakOffscreenBytes)
	}
	document.Root.Opacity = .5
	options.MaxOffscreenBytes = 63
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage 64") {
		t.Fatalf("effect below-limit error = %v", err)
	}
	options.MaxOffscreenBytes = 64
	prepared, err = prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.resources.peakOffscreenBytes != 64 {
		t.Fatalf("effect peak = %d, want 64", prepared.resources.peakOffscreenBytes)
	}
	document.Root.Opacity = 1
	secondNode := d2scene.NewNode(d2scene.Image{Asset: "image", Box: d2scene.Box{Width: 4, Height: 4}})
	document.Root.Children = []*d2scene.Node{secondNode}
	prepared, err = prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	document.Root.Children = nil
	var first []byte
	for iteration := 0; iteration < 5; iteration++ {
		frame, err := Render(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := EncodePNG(context.Background(), frame)
		if err != nil {
			t.Fatal(err)
		}
		if iteration == 0 {
			first = encoded
		} else if !bytes.Equal(encoded, first) {
			t.Fatalf("render %d is not deterministic", iteration)
		}
	}

	sixteenBit := image.NewNRGBA64(image.Rect(0, 0, 2, 1))
	sixteenBit.SetNRGBA64(0, 0, color.NRGBA64{R: 0x1234, G: 0x5678, B: 0x9abc, A: 0xffff})
	sixteenBitDocument := rasterImageDocument("image/png", encodeRasterPNG(t, sixteenBit), 2, 1, d2scene.Box{Width: 2, Height: 1}, d2scene.AspectRatio{})
	sixteenBitAsset := sixteenBitDocument.Assets["image"].(d2scene.RasterAsset)
	sixteenBitAsset.DecodedBytes = 8
	sixteenBitDocument.Assets["image"] = sixteenBitAsset
	if _, err := Render(context.Background(), sixteenBitDocument, testOptions()); err == nil || !strings.Contains(err.Error(), "below the required 16") {
		t.Fatalf("undercharged 16-bit PNG error = %v", err)
	}
}

func TestRasterImageAssetCountAndEncodedByteLimits(t *testing.T) {
	data := encodeRasterPNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	document := rasterImageDocument("image/png", data, 1, 1, d2scene.Box{Width: 1, Height: 1}, d2scene.AspectRatio{})
	options := testOptions()
	options.MaxAssets = 1
	options.MaxAssetBytes = int64(len(data))
	if _, err := prepare(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive asset limits: %v", err)
	}
	options.MaxAssets = 2
	options.MaxAssetBytes = int64(len(data)) - 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "retained asset bytes") {
		t.Fatalf("encoded-byte limit error = %v", err)
	}
	options.MaxAssetBytes = 2 * int64(len(data))
	document.Assets["unused"] = d2scene.RasterAsset{MIMEType: "image/png", Data: data, PixelWidth: 1, PixelHeight: 1}
	options.MaxAssets = 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "asset count 2 exceeds limit 1") {
		t.Fatalf("asset-count limit error = %v", err)
	}
	options.MaxAssets = 2
	options.MaxDecodedAssetBytes = 7
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "MaxDecodedAssetBytes") {
		t.Fatalf("cumulative decoded-byte limit error = %v", err)
	}
	options.MaxDecodedAssetBytes = 8
	if _, err := prepare(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive cumulative decoded limit: %v", err)
	}
}

func TestRasterImageCancellationDuringSampling(t *testing.T) {
	asset := newPreparedRasterAsset(image.NewNRGBA(image.Rect(0, 0, 32, 32)))
	prepared := &preparedImage{
		asset: asset, box: d2scene.Box{Width: 32, Height: 32}, placement: d2scene.Box{Width: 32, Height: 32},
		inverse: inverseAffine{a: 1, d: 1}, bounds: image.Rect(0, 0, 32, 32),
	}
	ctx := &cancelAfterErrChecks{remaining: 4}
	err := drawPreparedImage(ctx, image.NewRGBA(image.Rect(0, 0, 32, 32)), prepared)
	if err != context.Canceled {
		t.Fatalf("drawPreparedImage error = %v, want context.Canceled", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	document := rasterImageDocument("image/png", encodeRasterPNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1))), 1, 1, d2scene.Box{Width: 1, Height: 1}, d2scene.AspectRatio{})
	if _, err := Render(canceled, document, testOptions()); err != context.Canceled {
		t.Fatalf("canceled Render error = %v, want context.Canceled", err)
	}
}

func TestRasterImageConcurrentRepeatability(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 23), G: uint8(y * 31), B: uint8((x + y) * 17), A: uint8(80 + x*20)})
		}
	}
	document := rasterImageDocument("image/png", encodeRasterPNG(t, img), 8, 6, d2scene.Box{Width: 13, Height: 11}, d2scene.AspectRatio{Align: d2scene.AlignXMidYMid, Fit: d2scene.AspectSlice})
	const workers = 8
	outputs := make([][]byte, workers)
	errors := make([]error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			frame, err := Render(context.Background(), document, testOptions())
			if err == nil {
				outputs[index], err = EncodePNG(context.Background(), frame)
			}
			errors[index] = err
		}(worker)
	}
	wait.Wait()
	for worker := 0; worker < workers; worker++ {
		if errors[worker] != nil {
			t.Fatalf("worker %d: %v", worker, errors[worker])
		}
		if worker != 0 && !bytes.Equal(outputs[worker], outputs[0]) {
			t.Fatalf("worker %d produced non-deterministic PNG bytes", worker)
		}
	}
}

func TestBilinearPremultipliedBoundSamplerEquivalence(t *testing.T) {
	for _, test := range rasterSamplerTestImages() {
		t.Run(test.name, func(t *testing.T) {
			asset := newPreparedRasterAsset(test.image)
			bounds := test.image.Bounds()
			if asset.sampleQuad != nil {
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						samples := asset.sampleQuad(asset.image, x, x, y, y)
						want := rgba64(test.image.At(x, y))
						if samples.c00 != want || samples.c10 != want || samples.c01 != want || samples.c11 != want {
							t.Fatalf("sampleQuad(%d, %d) = %+v, want four copies of %v", x, y, samples, want)
						}
					}
				}
			}

			coordinates := [][2]float64{
				{0, 0}, {.125, .875}, {.5, .5}, {.75, 1.25},
				{float64(asset.width) - .5, float64(asset.bounds.Dy()) - .5},
				{math.Nextafter(float64(asset.width), 0), math.Nextafter(float64(asset.bounds.Dy()), 0)},
			}
			for index := 0; index < 512; index++ {
				coordinates = append(coordinates, [2]float64{
					math.Mod(float64(index*97)/64, float64(asset.width)),
					math.Mod(float64(index*193)/128, float64(asset.bounds.Dy())),
				})
			}
			for _, coordinate := range coordinates {
				got := bilinearPremultiplied(asset, coordinate[0], coordinate[1])
				want := bilinearPremultipliedGeneric(test.image, asset, coordinate[0], coordinate[1])
				if got != want {
					t.Fatalf("bilinearPremultiplied(%g, %g) = %v, want %v", coordinate[0], coordinate[1], got, want)
				}
			}
		})
	}
}

func TestBilinearPremultipliedBoundSamplerAllocations(t *testing.T) {
	for _, test := range rasterSamplerTestImages() {
		if strings.HasPrefix(test.name, "generic") || strings.HasPrefix(test.name, "custom") {
			continue
		}
		asset := newPreparedRasterAsset(test.image)
		var sample [4]uint32
		if allocations := testing.AllocsPerRun(1000, func() {
			sample = bilinearPremultiplied(asset, 2.375, 3.625)
		}); allocations != 0 {
			t.Errorf("%s sampler allocations = %g, want 0", test.name, allocations)
		}
		if sample == ([4]uint32{}) {
			t.Errorf("%s sampler unexpectedly returned transparent black", test.name)
		}
	}
}

func TestDrawNativeSizePreparedImageMatchesSampledPath(t *testing.T) {
	checkerBounds := image.Rect(-4, 7, 5, 14)
	checkerNRGBA := image.NewNRGBA(checkerBounds)
	checkerRGBA := image.NewRGBA(checkerBounds)
	for y := checkerBounds.Min.Y; y < checkerBounds.Max.Y; y++ {
		for x := checkerBounds.Min.X; x < checkerBounds.Max.X; x++ {
			var value color.NRGBA
			if (x+y)&1 == 0 {
				value = color.NRGBA{R: 251, G: 3, B: 193, A: 37}
			} else {
				value = color.NRGBA{R: 7, G: 239, B: 29, A: 251}
			}
			checkerNRGBA.SetNRGBA(x, y, value)
			checkerRGBA.Set(x, y, value)
		}
	}
	testImages := append(rasterSamplerTestImages(),
		rasterSamplerTestImage{name: "NRGBA/checkerboard", image: checkerNRGBA},
		rasterSamplerTestImage{name: "RGBA/checkerboard", image: checkerRGBA},
	)
	for _, test := range testImages {
		t.Run(test.name, func(t *testing.T) {
			asset := newPreparedRasterAsset(test.image)
			for _, mapping := range []struct {
				name       string
				a, d, x, y float64
			}{
				{name: "identity", a: 1, d: 1, x: 2, y: 3},
				{name: "forward scale 2", a: .5, d: .5, x: 1, y: 1.5},
				{name: "forward scale 0.5", a: 2, d: 2, x: 4, y: 6},
			} {
				t.Run(mapping.name, func(t *testing.T) {
					box := d2scene.Box{
						X: mapping.x, Y: mapping.y,
						Width: mapping.a * float64(asset.width), Height: mapping.d * float64(asset.bounds.Dy()),
					}
					prepared := &preparedImage{
						asset: asset, box: box, placement: box,
						inverse: inverseAffine{a: mapping.a, d: mapping.d, e: 5, f: -2},
						bounds:  image.Rect(7, 1, 7+asset.width, 1+asset.bounds.Dy()),
					}
					fast := image.NewRGBA(image.Rect(-2, -2, 20, 15))
					for y := fast.Rect.Min.Y; y < fast.Rect.Max.Y; y++ {
						for x := fast.Rect.Min.X; x < fast.Rect.Max.X; x++ {
							fast.SetRGBA(x, y, color.RGBA{
								R: uint8((x-fast.Rect.Min.X)*7 + 11),
								G: uint8((y-fast.Rect.Min.Y)*9 + 13),
								B: uint8((x+y-fast.Rect.Min.X-fast.Rect.Min.Y)*5 + 17),
								A: 255,
							})
						}
					}
					sampled := image.NewRGBA(fast.Rect)
					copy(sampled.Pix, fast.Pix)
					if err := drawPreparedImage(context.Background(), fast, prepared); err != nil {
						t.Fatal(err)
					}
					if err := drawSampledPreparedImage(context.Background(), sampled, prepared, prepared.bounds.Intersect(sampled.Bounds())); err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(fast.Pix, sampled.Pix) {
						t.Fatal("native-size image path differs from supersampled path")
					}
				})
			}
		})
	}

	asset := newPreparedRasterAsset(image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	base := &preparedImage{
		asset:     asset,
		box:       d2scene.Box{X: 1, Y: 2, Width: 2, Height: 2},
		placement: d2scene.Box{X: 1, Y: 2, Width: 2, Height: 2},
		inverse:   inverseAffine{a: 1, d: 1},
	}
	for name, mutate := range map[string]func(*preparedImage){
		"scale":             func(p *preparedImage) { p.inverse.a = .5 },
		"rotation or shear": func(p *preparedImage) { p.inverse.b = .25 },
		"fractional origin": func(p *preparedImage) { p.box.X = 1.25; p.placement.X = 1.25 },
		"scaled placement":  func(p *preparedImage) { p.placement.Width = 3 },
		"scaled box":        func(p *preparedImage) { p.box.Width = 3; p.placement.Width = 3 },
		"large cancellation": func(p *preparedImage) {
			p.box.X = 1 << 50
			p.placement.X = 1 << 50
			p.inverse.e = -(1 << 50)
		},
	} {
		t.Run("reject "+name, func(t *testing.T) {
			candidate := *base
			mutate(&candidate)
			if _, ok := nativeRasterImageOrigin(&candidate); ok {
				t.Fatal("nativeRasterImageOrigin accepted non-native mapping")
			}
		})
	}
}

func TestDrawNativeSizePreparedImageConcretePathsMatchGeneric(t *testing.T) {
	sourceBounds := image.Rect(-17, 9, 520, 15)
	origin := image.Pt(101, -23)
	drawBounds := image.Rect(103, -21, 624, -17)
	destinationBounds := image.Rect(80, -30, 650, 0)

	for _, pattern := range []struct {
		name  string
		alpha func(x, y int) byte
	}{
		{name: "opaque", alpha: func(_, _ int) byte { return 0xff }},
		{name: "transparent", alpha: func(_, _ int) byte { return 0 }},
		{name: "alternating", alpha: func(x, y int) byte {
			if (x+y)&1 == 0 {
				return 0xff
			}
			return 0
		}},
		{name: "mixed", alpha: func(x, y int) byte {
			index := (x*17 + y*31) % 9
			if index < 0 {
				index += 9
			}
			return [...]byte{0, 1, 2, 63, 127, 128, 191, 254, 255}[index]
		}},
	} {
		for _, kind := range []string{"RGBA", "NRGBA"} {
			t.Run(kind+"/"+pattern.name, func(t *testing.T) {
				stride := sourceBounds.Dx()*4 + 23
				pixels := make([]byte, stride*sourceBounds.Dy())
				for index := range pixels {
					pixels[index] = byte(index*47 + 19)
				}
				for y := sourceBounds.Min.Y; y < sourceBounds.Max.Y; y++ {
					for x := sourceBounds.Min.X; x < sourceBounds.Max.X; x++ {
						offset := (y-sourceBounds.Min.Y)*stride + (x-sourceBounds.Min.X)*4
						pixels[offset] = byte(x*29 + y*11)
						pixels[offset+1] = byte(x*7 + y*43)
						pixels[offset+2] = byte(x*53 + y*3)
						pixels[offset+3] = pattern.alpha(x, y)
					}
				}
				var source image.Image
				if kind == "RGBA" {
					source = &image.RGBA{Pix: pixels, Stride: stride, Rect: sourceBounds}
				} else {
					source = &image.NRGBA{Pix: pixels, Stride: stride, Rect: sourceBounds}
				}
				assertNativeImagePathMatchesGeneric(t, source, drawBounds, destinationBounds, origin)
			})
		}
	}
	palette := color.Palette{
		color.NRGBA{R: 251, G: 17, B: 91, A: 0},
		color.RGBA{R: 3, G: 7, B: 11, A: 1},
		color.NRGBA{R: 197, G: 151, B: 103, A: 127},
		color.RGBA{R: 89, G: 61, B: 37, A: 128},
		color.NRGBA{R: 223, G: 227, B: 229, A: 254},
		color.RGBA{R: 239, G: 241, B: 251, A: 255},
		color.Gray{Y: 83},
		color.Alpha{A: 191},
	}
	palettedStride := sourceBounds.Dx() + 13
	palettedPixels := make([]byte, palettedStride*sourceBounds.Dy())
	for index := range palettedPixels {
		palettedPixels[index] = byte(index % len(palette))
	}
	paletted := &image.Paletted{Pix: palettedPixels, Stride: palettedStride, Rect: sourceBounds, Palette: palette}
	t.Run("Paletted", func(t *testing.T) {
		assertNativeImagePathMatchesGeneric(t, paletted, drawBounds, destinationBounds, origin)
	})

	for _, ratio := range []image.YCbCrSubsampleRatio{
		image.YCbCrSubsampleRatio444,
		image.YCbCrSubsampleRatio422,
		image.YCbCrSubsampleRatio420,
		image.YCbCrSubsampleRatio440,
		image.YCbCrSubsampleRatio411,
		image.YCbCrSubsampleRatio410,
	} {
		t.Run("YCbCr/"+ratio.String(), func(t *testing.T) {
			source := image.NewYCbCr(sourceBounds, ratio)
			for index := range source.Y {
				source.Y[index] = byte(index*17 + 13)
			}
			for index := range source.Cb {
				source.Cb[index] = byte(index*31 + 29)
				source.Cr[index] = byte(index*43 + 37)
			}
			assertNativeImagePathMatchesGeneric(t, source, drawBounds, destinationBounds, origin)
		})
	}
	for _, test := range rasterSamplerTestImages() {
		switch test.name {
		case "RGBA64", "NRGBA64", "Alpha", "Alpha16", "Gray", "Gray16", "CMYK", "NYCbCrA":
		default:
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			origin := image.Pt(37, -41)
			bounds := image.Rect(origin.X, origin.Y, origin.X+test.image.Bounds().Dx(), origin.Y+test.image.Bounds().Dy())
			assertNativeImagePathMatchesGeneric(t, test.image, bounds, bounds.Inset(-5), origin)
		})
	}
}

func TestDrawNativeSizePreparedImageEightBitDomainEquivalence(t *testing.T) {
	sourceBounds := image.Rect(-5, 7, 251, 263)
	for _, kind := range []string{"RGBA", "NRGBA"} {
		t.Run(kind, func(t *testing.T) {
			stride := sourceBounds.Dx()*4 + 11
			pixels := make([]byte, stride*sourceBounds.Dy())
			for y := sourceBounds.Min.Y; y < sourceBounds.Max.Y; y++ {
				alpha := byte(y - sourceBounds.Min.Y)
				for x := sourceBounds.Min.X; x < sourceBounds.Max.X; x++ {
					channel := byte(x - sourceBounds.Min.X)
					offset := (y-sourceBounds.Min.Y)*stride + (x-sourceBounds.Min.X)*4
					pixels[offset] = channel
					pixels[offset+1] = 255 - channel
					pixels[offset+2] = byte(uint16(channel)*197 + uint16(alpha)*61)
					pixels[offset+3] = alpha
				}
			}
			var source image.Image
			if kind == "RGBA" {
				source = &image.RGBA{Pix: pixels, Stride: stride, Rect: sourceBounds}
			} else {
				source = &image.NRGBA{Pix: pixels, Stride: stride, Rect: sourceBounds}
			}
			origin := image.Pt(19, -31)
			drawBounds := image.Rect(origin.X, origin.Y, origin.X+sourceBounds.Dx(), origin.Y+sourceBounds.Dy())
			assertNativeImagePathMatchesGeneric(t, source, drawBounds, drawBounds.Inset(-3), origin)
		})
	}
}

func TestDrawNativeSizePreparedImageSixteenBitDomainEquivalence(t *testing.T) {
	sourceBounds := image.Rect(-7, 11, 249, 267)
	origin := image.Pt(23, -37)
	drawBounds := image.Rect(origin.X, origin.Y, origin.X+sourceBounds.Dx(), origin.Y+sourceBounds.Dy())
	for _, kind := range []string{"RGBA64", "NRGBA64"} {
		t.Run(kind, func(t *testing.T) {
			stride := sourceBounds.Dx()*8 + 13
			pixels := make([]byte, stride*sourceBounds.Dy())
			for y := 0; y < sourceBounds.Dy(); y++ {
				for x := 0; x < sourceBounds.Dx(); x++ {
					index := uint16(y*sourceBounds.Dx() + x)
					offset := y*stride + x*8
					binary.BigEndian.PutUint16(pixels[offset:], index*40503)
					binary.BigEndian.PutUint16(pixels[offset+2:], index*32771+1)
					binary.BigEndian.PutUint16(pixels[offset+4:], index*8193+127)
					binary.BigEndian.PutUint16(pixels[offset+6:], index)
				}
			}
			var source image.Image
			if kind == "RGBA64" {
				source = &image.RGBA64{Pix: pixels, Stride: stride, Rect: sourceBounds}
			} else {
				source = &image.NRGBA64{Pix: pixels, Stride: stride, Rect: sourceBounds}
			}
			assertNativeImagePathMatchesGeneric(t, source, drawBounds, drawBounds.Inset(-3), origin)
		})
	}
	for _, kind := range []string{"Gray16", "Alpha16"} {
		t.Run(kind, func(t *testing.T) {
			stride := sourceBounds.Dx()*2 + 13
			pixels := make([]byte, stride*sourceBounds.Dy())
			for y := 0; y < sourceBounds.Dy(); y++ {
				for x := 0; x < sourceBounds.Dx(); x++ {
					index := uint16(y*sourceBounds.Dx() + x)
					binary.BigEndian.PutUint16(pixels[y*stride+x*2:], index)
				}
			}
			var source image.Image
			if kind == "Gray16" {
				source = &image.Gray16{Pix: pixels, Stride: stride, Rect: sourceBounds}
			} else {
				source = &image.Alpha16{Pix: pixels, Stride: stride, Rect: sourceBounds}
			}
			assertNativeImagePathMatchesGeneric(t, source, drawBounds, drawBounds.Inset(-3), origin)
		})
	}
}

func assertNativeImagePathMatchesGeneric(t *testing.T, source image.Image, drawBounds, destinationBounds image.Rectangle, origin image.Point) {
	t.Helper()
	asset := newPreparedRasterAsset(source)
	stride := destinationBounds.Dx()*4 + 17
	pixels := make([]byte, stride*destinationBounds.Dy())
	for index := range pixels {
		pixels[index] = byte(index*71 + 23)
	}
	sourceBefore := snapshotRasterSource(source)

	for _, checks := range []int{0, 1, 2, 3, 4, 5, 7, 11, 1 << 20} {
		fast := &image.RGBA{Pix: append([]byte(nil), pixels...), Stride: stride, Rect: destinationBounds}
		generic := &image.RGBA{Pix: append([]byte(nil), pixels...), Stride: stride, Rect: destinationBounds}
		fastContext := &nativeImageOracleContext{remaining: checks}
		genericContext := &nativeImageOracleContext{remaining: checks}
		fastError := drawNativeSizePreparedImage(fastContext, fast, asset, drawBounds, origin)
		genericError := drawNativeSizeGeneric(genericContext, generic, asset, drawBounds, origin)
		if fastError != genericError {
			t.Fatalf("after %d successful context checks: error = %v, want %v", checks, fastError, genericError)
		}
		if fastContext.calls != genericContext.calls {
			t.Fatalf("after %d successful context checks: Err calls = %d, want %d", checks, fastContext.calls, genericContext.calls)
		}
		if !bytes.Equal(fast.Pix, generic.Pix) {
			t.Fatalf("after %d successful context checks: concrete path differs from generic path", checks)
		}
	}
	if after := snapshotRasterSource(source); !bytes.Equal(after, sourceBefore) {
		t.Fatal("native image drawing mutated its source")
	}
}

func snapshotRasterSource(source image.Image) []byte {
	switch source := source.(type) {
	case *image.RGBA:
		return append([]byte(nil), source.Pix...)
	case *image.NRGBA:
		return append([]byte(nil), source.Pix...)
	case *image.Paletted:
		return append([]byte(nil), source.Pix...)
	case *image.RGBA64:
		return append([]byte(nil), source.Pix...)
	case *image.NRGBA64:
		return append([]byte(nil), source.Pix...)
	case *image.Gray:
		return append([]byte(nil), source.Pix...)
	case *image.Gray16:
		return append([]byte(nil), source.Pix...)
	case *image.Alpha:
		return append([]byte(nil), source.Pix...)
	case *image.Alpha16:
		return append([]byte(nil), source.Pix...)
	case *image.CMYK:
		return append([]byte(nil), source.Pix...)
	case *image.YCbCr:
		result := append([]byte(nil), source.Y...)
		result = append(result, source.Cb...)
		return append(result, source.Cr...)
	case *image.NYCbCrA:
		result := append([]byte(nil), source.Y...)
		result = append(result, source.Cb...)
		result = append(result, source.Cr...)
		return append(result, source.A...)
	default:
		return nil
	}
}

type nativeImageOracleContext struct {
	remaining int
	calls     int
}

func (*nativeImageOracleContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*nativeImageOracleContext) Done() <-chan struct{}       { return nil }
func (*nativeImageOracleContext) Value(any) any               { return nil }
func (ctx *nativeImageOracleContext) Err() error {
	ctx.calls++
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return nil
}

func TestCompositePremultipliedRGBA64FastPathsEquivalence(t *testing.T) {
	values := []uint32{0, 1, 127, 128, 255, 256, 32767, 32768, 65407, 65535}
	destinations := [][4]byte{
		{0, 0, 0, 0}, {1, 2, 3, 4}, {127, 128, 129, 130}, {252, 253, 254, 255},
	}
	for _, red := range values {
		for _, green := range values {
			for _, blue := range values {
				for _, alpha := range values {
					source := [4]uint32{red, green, blue, alpha}
					for _, initial := range destinations {
						got := append([]byte(nil), initial[:]...)
						want := append([]byte(nil), initial[:]...)
						compositePremultipliedRGBA64(got, source)
						compositePremultipliedRGBA64Reference(want, source)
						if !bytes.Equal(got, want) {
							t.Fatalf("source=%v destination=%v: got %v, want %v", source, initial, got, want)
						}
					}
				}
			}
		}
	}
}

func bilinearPremultipliedGeneric(source image.Image, asset *preparedRasterAsset, x, y float64) [4]uint32 {
	centerX := x - .5
	centerY := y - .5
	x0 := int(math.Floor(centerX))
	y0 := int(math.Floor(centerY))
	weightX := centerX - float64(x0)
	weightY := centerY - float64(y0)
	x1, y1 := x0+1, y0+1
	x0 = clampInt(x0, 0, asset.width-1)
	x1 = clampInt(x1, 0, asset.width-1)
	y0 = clampInt(y0, 0, asset.bounds.Dy()-1)
	y1 = clampInt(y1, 0, asset.bounds.Dy()-1)
	c00 := rgba64(source.At(asset.bounds.Min.X+x0, asset.bounds.Min.Y+y0))
	c10 := rgba64(source.At(asset.bounds.Min.X+x1, asset.bounds.Min.Y+y0))
	c01 := rgba64(source.At(asset.bounds.Min.X+x0, asset.bounds.Min.Y+y1))
	c11 := rgba64(source.At(asset.bounds.Min.X+x1, asset.bounds.Min.Y+y1))
	var result [4]uint32
	for channel := range result {
		top := float64(c00[channel]) + (float64(c10[channel])-float64(c00[channel]))*weightX
		bottom := float64(c01[channel]) + (float64(c11[channel])-float64(c01[channel]))*weightX
		value := top + (bottom-top)*weightY
		result[channel] = uint32(math.Round(math.Max(0, math.Min(65535, value))))
	}
	return result
}

func compositePremultipliedRGBA64Reference(destination []byte, source [4]uint32) {
	toByte := func(value uint32) uint32 { return (value + 128) / 257 }
	sourceAlpha := toByte(source[3])
	if sourceAlpha == 0 {
		return
	}
	inverseAlpha := 255 - sourceAlpha
	mul255 := func(left, right uint32) uint32 { return (left*right + 127) / 255 }
	for channel := 0; channel < 3; channel++ {
		sourceChannel := toByte(source[channel])
		if sourceChannel > sourceAlpha {
			sourceChannel = sourceAlpha
		}
		result := sourceChannel + mul255(uint32(destination[channel]), inverseAlpha)
		if result > 255 {
			result = 255
		}
		destination[channel] = uint8(result)
	}
	alpha := sourceAlpha + mul255(uint32(destination[3]), inverseAlpha)
	if alpha > 255 {
		alpha = 255
	}
	destination[3] = uint8(alpha)
}

type rasterSamplerTestImage struct {
	name  string
	image image.Image
}

func rasterSamplerTestImages() []rasterSamplerTestImage {
	bounds := image.Rect(-3, 5, 4, 11)
	rgba := image.NewRGBA(bounds)
	rgba64Image := image.NewRGBA64(bounds)
	nrgba := image.NewNRGBA(bounds)
	nrgba64 := image.NewNRGBA64(bounds)
	alpha := image.NewAlpha(bounds)
	alpha16 := image.NewAlpha16(bounds)
	gray := image.NewGray(bounds)
	gray16 := image.NewGray16(bounds)
	cmyk := image.NewCMYK(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			value := color.NRGBA64{
				R: uint16((x-bounds.Min.X)*7311 + (y-bounds.Min.Y)*977),
				G: uint16((x-bounds.Min.X)*1777 + (y-bounds.Min.Y)*5119),
				B: uint16((x-bounds.Min.X)*3251 + (y-bounds.Min.Y)*2377),
				A: uint16(1 + ((x-bounds.Min.X)*7919+(y-bounds.Min.Y)*4441)%65535),
			}
			rgba.Set(x, y, value)
			rgba64Image.Set(x, y, value)
			nrgba.Set(x, y, value)
			nrgba64.Set(x, y, value)
			alpha.Set(x, y, value)
			alpha16.Set(x, y, value)
			gray.Set(x, y, value)
			gray16.Set(x, y, value)
			cmyk.Set(x, y, value)
		}
	}

	palette := color.Palette{
		color.RGBA{R: 13, G: 29, B: 47, A: 61},
		color.NRGBA{R: 251, G: 199, B: 149, A: 101},
		color.Gray{Y: 83},
		color.Gray16{Y: 0x1234},
		color.CMYK{C: 17, M: 71, Y: 113, K: 31},
	}
	paletted := image.NewPaletted(bounds, palette)
	for index := range paletted.Pix {
		paletted.Pix[index] = uint8((index * 3) % len(palette))
	}

	result := []rasterSamplerTestImage{
		{name: "RGBA", image: rgba},
		{name: "RGBA64", image: rgba64Image},
		{name: "NRGBA", image: nrgba},
		{name: "NRGBA64", image: nrgba64},
		{name: "Alpha", image: alpha},
		{name: "Alpha16", image: alpha16},
		{name: "Gray", image: gray},
		{name: "Gray16", image: gray16},
		{name: "CMYK", image: cmyk},
		{name: "Paletted", image: paletted},
		{name: "generic image.Image", image: samplerFallbackImage{bounds: bounds}},
		{name: "custom RGBA64Image", image: samplerCustomRGBA64Image{bounds: bounds}},
	}
	for _, ratio := range []image.YCbCrSubsampleRatio{
		image.YCbCrSubsampleRatio444,
		image.YCbCrSubsampleRatio422,
		image.YCbCrSubsampleRatio420,
		image.YCbCrSubsampleRatio440,
		image.YCbCrSubsampleRatio411,
		image.YCbCrSubsampleRatio410,
	} {
		ycbcr := image.NewYCbCr(bounds, ratio)
		for index := range ycbcr.Y {
			ycbcr.Y[index] = uint8(index*17 + int(ratio)*7)
		}
		for index := range ycbcr.Cb {
			ycbcr.Cb[index] = uint8(index*29 + int(ratio)*11)
			ycbcr.Cr[index] = uint8(index*43 + int(ratio)*13)
		}
		result = append(result, rasterSamplerTestImage{name: "YCbCr/" + ratio.String(), image: ycbcr})
	}
	ycbcr := image.NewYCbCr(bounds, image.YCbCrSubsampleRatio420)
	for index := range ycbcr.Y {
		ycbcr.Y[index] = uint8(index*23 + 7)
	}
	for index := range ycbcr.Cb {
		ycbcr.Cb[index] = uint8(index*31 + 17)
		ycbcr.Cr[index] = uint8(index*37 + 29)
	}
	nycbcra := &image.NYCbCrA{
		YCbCr: *ycbcr, A: make([]uint8, bounds.Dx()*bounds.Dy()), AStride: bounds.Dx(),
	}
	for index := range nycbcra.A {
		nycbcra.A[index] = uint8(index*41 + 3)
	}
	return append(result, rasterSamplerTestImage{name: "NYCbCrA", image: nycbcra})
}

type samplerFallbackImage struct {
	bounds image.Rectangle
}

func (img samplerFallbackImage) ColorModel() color.Model { return color.RGBAModel }
func (img samplerFallbackImage) Bounds() image.Rectangle { return img.bounds }
func (img samplerFallbackImage) At(x, y int) color.Color {
	return samplerFallbackColor{uint32((x-img.bounds.Min.X)*3001 + (y-img.bounds.Min.Y)*607)}
}

type samplerFallbackColor struct {
	seed uint32
}

type samplerCustomRGBA64Image struct {
	bounds image.Rectangle
}

func (img samplerCustomRGBA64Image) ColorModel() color.Model { return color.RGBA64Model }
func (img samplerCustomRGBA64Image) Bounds() image.Rectangle { return img.bounds }
func (img samplerCustomRGBA64Image) At(x, y int) color.Color {
	return samplerFallbackColor{uint32((x-img.bounds.Min.X)*4111 + (y-img.bounds.Min.Y)*953)}
}
func (img samplerCustomRGBA64Image) RGBA64At(_, _ int) color.RGBA64 {
	// A third-party implementation is not required to derive RGBA64At from At.
	// The optimized path is intentionally limited to standard-library concrete
	// images so the renderer continues to honor Image.At for custom images.
	return color.RGBA64{R: 0xffff, A: 0xffff}
}

func (value samplerFallbackColor) RGBA() (r, g, b, a uint32) {
	a = 10000 + value.seed%55536
	r = value.seed % (a + 1)
	g = (value.seed * 7) % (a + 1)
	b = (value.seed * 31) % (a + 1)
	return r, g, b, a
}

func FuzzRasterAssetPreflightBounded(f *testing.F) {
	seed, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed, "image/png")
	f.Add([]byte("GIF89a"), "image/gif")
	f.Add([]byte("RIFF\x04\x00\x00\x00WEBP"), "image/webp")
	f.Fuzz(func(t *testing.T, data []byte, mimeType string) {
		if len(data) > 64*1024 || len(mimeType) > 64 {
			return
		}
		_, _, _ = prepareRasterAsset(context.Background(), "fuzz", d2scene.RasterAsset{
			MIMEType: mimeType, Data: data, PixelWidth: 1, PixelHeight: 1,
		}, 1<<20)
	})
}

type cancelAfterErrChecks struct {
	remaining int
}

func (c *cancelAfterErrChecks) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterErrChecks) Done() <-chan struct{}       { return nil }
func (c *cancelAfterErrChecks) Value(any) any               { return nil }
func (c *cancelAfterErrChecks) Err() error {
	if c.remaining == 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func rasterImageDocument(mimeType string, data []byte, width, height int, box d2scene.Box, aspect d2scene.AspectRatio) *d2scene.Document {
	assetID := d2scene.AssetID("image")
	document := d2scene.NewDocument(d2scene.Box{Width: box.X + box.Width, Height: box.Y + box.Height}, d2scene.NewNode(d2scene.Image{
		Asset: assetID, Box: box, Aspect: aspect,
	}))
	document.Assets[assetID] = d2scene.RasterAsset{
		MIMEType: mimeType, Data: data, PixelWidth: width, PixelHeight: height,
	}
	return document
}

func encodeRasterPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodeRasterJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := jpeg.Encode(&output, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodeRasterGIF(t *testing.T, img *image.Paletted) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := gif.Encode(&output, img, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodeAnimatedRasterGIF(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	for index := range first.Pix {
		first.Pix[index] = 1
	}
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{0, 0}}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func insertRasterPNGChunk(t *testing.T, encoded []byte, chunkType string, payload []byte) []byte {
	t.Helper()
	iend := bytes.Index(encoded, []byte("IEND"))
	if iend < 4 {
		t.Fatal("PNG has no IEND chunk")
	}
	chunk := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(payload)))
	copy(chunk[4:8], chunkType)
	copy(chunk[8:8+len(payload)], payload)
	binary.BigEndian.PutUint32(chunk[8+len(payload):], crc32.ChecksumIEEE(chunk[4:8+len(payload)]))
	result := append([]byte(nil), encoded[:iend-4]...)
	result = append(result, chunk...)
	result = append(result, encoded[iend-4:]...)
	return result
}

func closeFloat(left, right float64) bool {
	difference := left - right
	if difference < 0 {
		difference = -difference
	}
	return difference < 1e-10
}

const rasterTestWebPBase64 = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
