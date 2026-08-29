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
	asset := &preparedRasterAsset{
		image: image.NewNRGBA(image.Rect(0, 0, 32, 32)), bounds: image.Rect(0, 0, 32, 32), width: 32, height: 32,
	}
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
