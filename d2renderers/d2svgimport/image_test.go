package d2svgimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestImportNodeEmbeddedRasterFormatsTopologyAndPixels(t *testing.T) {
	red := color.NRGBA{R: 0xe8, G: 0x20, B: 0x18, A: 0xff}
	webp, err := base64.StdEncoding.DecodeString(embeddedRasterTestWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name          string
		mimeType      string
		data          []byte
		pixelWidth    int
		pixelHeight   int
		wantRed       bool
		wantFrameHash string
	}{
		{name: "PNG", mimeType: "image/png", data: embeddedRasterPNG(t, 2, 2, red), pixelWidth: 2, pixelHeight: 2, wantRed: true, wantFrameHash: "00c725eb8ed54f7fe12dc215e1c606ff578e11f6489f755b7782125679b8b886"},
		{name: "JPEG", mimeType: "image/jpeg", data: embeddedRasterJPEG(t, 2, 2, red), pixelWidth: 2, pixelHeight: 2, wantRed: true, wantFrameHash: "00c725eb8ed54f7fe12dc215e1c606ff578e11f6489f755b7782125679b8b886"},
		{name: "GIF", mimeType: "image/gif", data: embeddedRasterGIF(t, 2, 2, red), pixelWidth: 2, pixelHeight: 2, wantRed: true, wantFrameHash: "00c725eb8ed54f7fe12dc215e1c606ff578e11f6489f755b7782125679b8b886"},
		{name: "WebP", mimeType: "image/webp", data: webp, pixelWidth: 75, pixelHeight: 100, wantFrameHash: "4307a46a64d4d51ad3b666b387a713bfa42086a5c6c06d1a36fc783d36caa32d"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := `<svg width="8" height="8"><image x="1" y="2" width="6" height="4" preserveAspectRatio="none" href="` + embeddedRasterDataURI(test.mimeType, test.data) + `"/></svg>`
			result, err := ImportNode(context.Background(), "embedded-"+test.name+".svg", []byte(source), generousImportLimits())
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Root.Children) != 1 {
				t.Fatalf("root children = %d, want 1", len(result.Root.Children))
			}
			primitive, ok := result.Root.Children[0].Primitive.(d2scene.Image)
			if !ok {
				t.Fatalf("image primitive = %T", result.Root.Children[0].Primitive)
			}
			if primitive.Box != (d2scene.Box{X: 1, Y: 2, Width: 6, Height: 4}) || primitive.Aspect.Align != d2scene.AlignNone {
				t.Fatalf("image primitive = %+v", primitive)
			}
			if !strings.HasPrefix(string(primitive.Asset), embeddedRasterAssetPrefix) || len(result.Assets) != 1 {
				t.Fatalf("asset ID/map = %q, %d", primitive.Asset, len(result.Assets))
			}
			asset, ok := result.Assets[primitive.Asset].(d2scene.RasterAsset)
			if !ok {
				t.Fatalf("asset = %T", result.Assets[primitive.Asset])
			}
			if asset.MIMEType != test.mimeType || asset.PixelWidth != test.pixelWidth || asset.PixelHeight != test.pixelHeight ||
				asset.DecodedBytes != int64(test.pixelWidth*test.pixelHeight*4) || !bytes.Equal(asset.Data, test.data) {
				t.Fatalf("asset = %+v, data equal %v", asset, bytes.Equal(asset.Data, test.data))
			}
			if metrics := result.Metrics; metrics.EmbeddedRasterAssets != 1 || metrics.EmbeddedRasterBytes != len(test.data) ||
				metrics.DecodedRasterBytes != asset.DecodedBytes || metrics.DeclaredResources != 1 {
				t.Fatalf("metrics = %+v", metrics)
			}

			frame := renderImportedRaster(t, result)
			pixel := frame.NRGBAAt(4, 4)
			if pixel.A == 0 {
				t.Fatalf("center pixel = %#v, want painted", pixel)
			}
			if test.wantRed && (pixel.R < 180 || pixel.R <= pixel.G || pixel.R <= pixel.B) {
				t.Fatalf("center pixel = %#v, want red-dominant", pixel)
			}
			digest := sha256.Sum256(frame.Pix)
			if test.wantFrameHash != "" && fmt.Sprintf("%x", digest) != test.wantFrameHash {
				t.Fatalf("frame SHA-256 = %x, want %s", digest, test.wantFrameHash)
			}
		})
	}
}

func TestImportNodeEmbeddedJPEGEXIFOrientation(t *testing.T) {
	data := embeddedRasterOrientedJPEG(t, 40, 24, 7)
	source := `<svg width="24" height="40"><image width="24" height="40" preserveAspectRatio="none" href="` +
		embeddedRasterDataURI("image/jpeg", data) + `"/></svg>`
	result, err := ImportNode(context.Background(), "oriented-jpeg.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	primitive := result.Root.Children[0].Primitive.(d2scene.Image)
	asset := result.Assets[primitive.Asset].(d2scene.RasterAsset)
	if asset.PixelWidth != 24 || asset.PixelHeight != 40 || asset.DecodedBytes != 24*40*4 {
		t.Fatalf("oriented JPEG asset = %dx%d decoded=%d, want 24x40/3840", asset.PixelWidth, asset.PixelHeight, asset.DecodedBytes)
	}
	frame := renderImportedRaster(t, result)
	if frame.Bounds() != image.Rect(0, 0, 24, 40) {
		t.Fatalf("oriented JPEG frame bounds = %v, want 24x40", frame.Bounds())
	}
	for _, test := range []struct {
		point image.Point
		gray  uint8
	}{
		{point: image.Pt(4, 4), gray: 230},
		{point: image.Pt(19, 4), gray: 80},
		{point: image.Pt(4, 35), gray: 150},
		{point: image.Pt(19, 35), gray: 20},
	} {
		pixel := frame.NRGBAAt(test.point.X, test.point.Y)
		if pixel.A != 255 || absByteDifference(pixel.R, test.gray) > 20 || absByteDifference(pixel.G, test.gray) > 20 || absByteDifference(pixel.B, test.gray) > 20 {
			t.Fatalf("oriented JPEG pixel %v = %#v, want gray near %d", test.point, pixel, test.gray)
		}
	}
}

func TestImportNodeDefersMalformedRasterPixelsToRender(t *testing.T) {
	data := corruptEmbeddedPNGPixelData(t, embeddedRasterPNG(t, 2, 2, color.NRGBA{R: 0xff, A: 0xff}))
	source := `<svg width="2" height="2"><image width="2" height="2" href="` + embeddedRasterDataURI("image/png", data) + `"/></svg>`
	result, err := ImportNode(context.Background(), "malformed-pixels.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatalf("ImportNode rejected structurally valid PNG before pixel decode: %v", err)
	}
	frame, err := renderImportedRasterResult(result)
	if err == nil || frame != nil || !strings.Contains(err.Error(), "malformed PNG pixel data") {
		t.Fatalf("Render = %#v, %v; want clear malformed PNG pixel error", frame, err)
	}
}

func TestImportNodeEmbeddedRasterCheckedInXLinkCorpus(t *testing.T) {
	corpus, err := os.ReadFile(filepath.Join("..", "d2sketch", "testdata", "dots-real", "sketch.exp.svg"))
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(corpus, []byte(`<image id="image0_`))
	if start < 0 {
		t.Fatal("checked-in sketch corpus has no embedded raster image")
	}
	end := bytes.IndexByte(corpus[start:], '>')
	if end < 0 {
		t.Fatal("checked-in embedded image tag is unterminated")
	}
	tag := corpus[start : start+end+1]
	source := append([]byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="466" height="349">`), tag...)
	source = append(source, []byte(`</svg>`)...)
	limits := generousImportLimits()
	limits.MaxBytes = 1 << 20
	limits.MaxAttributeBytes = 1 << 20
	result, err := ImportNode(context.Background(), "checked-in-sketch-image.svg", source, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Assets) != 1 || len(result.Root.Children) != 1 {
		t.Fatalf("assets/children = %d/%d", len(result.Assets), len(result.Root.Children))
	}
	primitive := result.Root.Children[0].Primitive.(d2scene.Image)
	asset := result.Assets[primitive.Asset].(d2scene.RasterAsset)
	if asset.MIMEType != "image/png" || asset.PixelWidth != 466 || asset.PixelHeight != 349 ||
		asset.DecodedBytes != 466*349*4 || primitive.Box != (d2scene.Box{Width: 466, Height: 349}) {
		t.Fatalf("checked-in image primitive/asset = %+v / %+v", primitive, asset)
	}
}

func TestImportNodeEmbeddedRasterAspectClipTransformAndOpacity(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0xff, A: 0xff})
	img.SetNRGBA(1, 0, color.NRGBA{B: 0xff, A: 0xff})
	data := encodeEmbeddedPNG(t, img)
	source := `<svg width="10" height="10"><defs><clipPath id="c"><rect width="4" height="8"/></clipPath></defs>` +
		`<image width="8" height="8" href="` + embeddedRasterDataURI("image/png", data) +
		`" preserveAspectRatio="xMidYMid meet" transform="translate(1 1)" clip-path="url(#c)" opacity=".5"/></svg>`
	result, err := ImportNode(context.Background(), "image-aspect.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Root.Children) != 1 {
		t.Fatalf("root children = %d, want one visible image", len(result.Root.Children))
	}
	node := result.Root.Children[0]
	primitive, ok := node.Primitive.(d2scene.Image)
	if !ok {
		t.Fatalf("primitive = %T", node.Primitive)
	}
	if primitive.Aspect != (d2scene.AspectRatio{Align: d2scene.AlignXMidYMid, Fit: d2scene.AspectMeet}) ||
		node.Transform != d2scene.Translate(1, 1) || node.Clip == nil || node.Opacity != .5 {
		t.Fatalf("image topology = primitive %+v node %+v", primitive, node)
	}
	frame := renderImportedRaster(t, result)
	inside := frame.NRGBAAt(2, 4)
	letterbox := frame.NRGBAAt(2, 2)
	outside := frame.NRGBAAt(6, 4)
	if inside.A < 100 || inside.A > 155 || letterbox.A != 0 || outside.A != 0 {
		t.Fatalf("aspect/clipped/transformed pixels = inside %#v, letterbox %#v, outside %#v", inside, letterbox, outside)
	}
}

func TestImportNodeEmbeddedRasterXLinkDedupStableOwnedAssets(t *testing.T) {
	data := embeddedRasterPNG(t, 3, 2, color.NRGBA{G: 0xcc, A: 0xff})
	uri := embeddedRasterDataURI("image/png", data)
	source := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="6" height="2">` +
		`<image width="3" height="2" href="` + uri + `"/><image x="3" width="3" height="2" xlink:href="` + uri + `"/></svg>`
	first, err := ImportNode(context.Background(), "dedup.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	second, err := ImportNode(context.Background(), "dedup.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Assets) != 1 || len(first.Root.Children) != 2 {
		t.Fatalf("assets/children = %d/%d", len(first.Assets), len(first.Root.Children))
	}
	left := first.Root.Children[0].Primitive.(d2scene.Image)
	right := first.Root.Children[1].Primitive.(d2scene.Image)
	secondID := second.Root.Children[0].Primitive.(d2scene.Image).Asset
	if left.Asset != right.Asset || left.Asset != secondID || first.Metrics.EmbeddedRasterAssets != 1 ||
		left.Aspect != (d2scene.AspectRatio{Align: d2scene.AlignXMidYMid, Fit: d2scene.AspectMeet}) {
		t.Fatalf("dedup IDs = %q, %q, %q; metrics %+v", left.Asset, right.Asset, secondID, first.Metrics)
	}
	firstAsset := first.Assets[left.Asset].(d2scene.RasterAsset)
	secondAsset := second.Assets[secondID].(d2scene.RasterAsset)
	firstAsset.Data[0] ^= 0xff
	if firstAsset.Data[0] == secondAsset.Data[0] {
		t.Fatal("independent imports alias retained raster bytes")
	}
}

func TestImportNodeEmbeddedRasterForwardUseInstances(t *testing.T) {
	data := embeddedRasterPNG(t, 2, 2, color.NRGBA{R: 0xff, A: 0xff})
	source := `<svg width="4" height="2"><use href="#icon"/><use href="#icon" x="2"/>` +
		`<defs><image id="icon" width="2" height="2" preserveAspectRatio="none" href="` +
		embeddedRasterDataURI("image/png", data) + `"/></defs></svg>`
	result, err := ImportNode(context.Background(), "image-forward-use.svg", []byte(source), generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Root.Children) != 2 || len(result.Assets) != 1 || result.Metrics.ExpandedUseInstances != 2 || result.Metrics.DeclaredResources != 2 {
		t.Fatalf("forward-use topology/metrics = children %d, assets %d, metrics %+v", len(result.Root.Children), len(result.Assets), result.Metrics)
	}
	left, right := result.Root.Children[0], result.Root.Children[1]
	if left == right || len(left.Children) != 1 || len(right.Children) != 1 || left.Children[0] == right.Children[0] {
		t.Fatalf("use instances are not independent: left %#v, right %#v", left, right)
	}
	leftImage := left.Children[0].Primitive.(d2scene.Image)
	rightImage := right.Children[0].Primitive.(d2scene.Image)
	if leftImage.Asset != rightImage.Asset || left.Transform != d2scene.Translate(0, 0) || right.Transform != d2scene.Translate(2, 0) {
		t.Fatalf("forward-use images/transforms = %+v/%+v, %+v/%+v", leftImage, left.Transform, rightImage, right.Transform)
	}
	frame := renderImportedRaster(t, result)
	if frame.NRGBAAt(0, 0).R < 200 || frame.NRGBAAt(3, 1).R < 200 {
		t.Fatalf("forward-use pixels = %#v, %#v", frame.NRGBAAt(0, 0), frame.NRGBAAt(3, 1))
	}
}

func TestImportNodeEmbeddedRasterInclusiveLimits(t *testing.T) {
	data := embeddedRasterPNG(t, 100, 100, color.NRGBA{R: 0x55, G: 0xaa, A: 0xff})
	source := []byte(`<svg width="100" height="100"><image width="100" height="100" href="` + embeddedRasterDataURI("image/png", data) + `"/></svg>`)
	if len(source) >= 40_000 {
		t.Fatalf("fixture source unexpectedly large: %d", len(source))
	}
	limits := generousImportLimits()
	limits.MaxBytes = 40_000
	result, err := ImportNode(context.Background(), "decoded-limit.svg", source, limits)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.DecodedRasterBytes != 40_000 {
		t.Fatalf("decoded bytes = %d", result.Metrics.DecodedRasterBytes)
	}
	sixteenBit := image.NewNRGBA64(image.Rect(0, 0, 2, 1))
	sixteenBit.SetNRGBA64(0, 0, color.NRGBA64{R: 0xffff, A: 0xffff})
	sixteenBit.SetNRGBA64(1, 0, color.NRGBA64{B: 0xffff, A: 0xffff})
	sixteenData := encodeEmbeddedPNG(t, sixteenBit)
	sixteenSource := []byte(`<svg width="2" height="1"><image width="2" height="1" href="` + embeddedRasterDataURI("image/png", sixteenData) + `"/></svg>`)
	sixteenResult, err := ImportNode(context.Background(), "sixteen-bit.svg", sixteenSource, generousImportLimits())
	if err != nil {
		t.Fatal(err)
	}
	if sixteenResult.Metrics.DecodedRasterBytes != 16 {
		t.Fatalf("16-bit PNG decoded bytes = %d, want 16", sixteenResult.Metrics.DecodedRasterBytes)
	}
	limits.MaxBytes = 39_999
	if result, err := ImportNode(context.Background(), "decoded-limit.svg", source, limits); err == nil || result != nil || !strings.Contains(err.Error(), "decoded bytes") {
		t.Fatalf("decoded limit+1 = %#v, %v", result, err)
	}

	firstLarge := embeddedRasterPNG(t, 50, 50, color.NRGBA{R: 0xff, A: 0xff})
	secondLarge := embeddedRasterPNG(t, 50, 50, color.NRGBA{B: 0xff, A: 0xff})
	cumulativeSource := []byte(`<svg width="100" height="50"><image width="50" height="50" href="` + embeddedRasterDataURI("image/png", firstLarge) +
		`"/><image x="50" width="50" height="50" href="` + embeddedRasterDataURI("image/png", secondLarge) + `"/></svg>`)
	limits = generousImportLimits()
	limits.MaxBytes = 20_000
	cumulative, err := ImportNode(context.Background(), "decoded-cumulative-limit.svg", cumulativeSource, limits)
	if err != nil || cumulative == nil || cumulative.Metrics.DecodedRasterBytes != 20_000 {
		t.Fatalf("exact cumulative decoded limit = %#v, %v", cumulative, err)
	}
	limits.MaxBytes = 19_999
	if result, err := ImportNode(context.Background(), "decoded-cumulative-limit.svg", cumulativeSource, limits); err == nil || result != nil || !strings.Contains(err.Error(), "decoded bytes") {
		t.Fatalf("cumulative decoded limit+1 = %#v, %v", result, err)
	}

	one := embeddedRasterPNG(t, 1, 1, color.NRGBA{R: 0xff, A: 0xff})
	two := embeddedRasterPNG(t, 1, 1, color.NRGBA{B: 0xff, A: 0xff})
	oneSource := []byte(`<svg width="1" height="1"><image width="1" height="1" href="` + embeddedRasterDataURI("image/png", one) + `"/></svg>`)
	resourceLimits := generousImportLimits()
	resourceLimits.MaxResources = 1
	if exact, err := ImportNode(context.Background(), "resource-limit.svg", oneSource, resourceLimits); err != nil || exact == nil || exact.Metrics.DeclaredResources != 1 {
		t.Fatalf("exact resource limit = %#v, %v", exact, err)
	}
	twoSource := []byte(`<svg width="2" height="1"><image width="1" height="1" href="` + embeddedRasterDataURI("image/png", one) +
		`"/><image x="1" width="1" height="1" href="` + embeddedRasterDataURI("image/png", two) + `"/></svg>`)
	if over, err := ImportNode(context.Background(), "resource-limit.svg", twoSource, resourceLimits); err == nil || over != nil || !strings.Contains(err.Error(), "resource count") {
		t.Fatalf("resource limit+1 = %#v, %v", over, err)
	}
	combinedSource := []byte(`<svg width="1" height="1"><defs><rect id="r" width="1" height="1"/></defs><image width="1" height="1" href="` +
		embeddedRasterDataURI("image/png", one) + `"/></svg>`)
	resourceLimits.MaxResources = 2
	if exact, err := ImportNode(context.Background(), "combined-resource-limit.svg", combinedSource, resourceLimits); err != nil || exact == nil || exact.Metrics.DeclaredResources != 2 {
		t.Fatalf("exact combined ID/asset resource limit = %#v, %v", exact, err)
	}
	resourceLimits.MaxResources = 1
	if over, err := ImportNode(context.Background(), "combined-resource-limit.svg", combinedSource, resourceLimits); err == nil || over != nil || !strings.Contains(err.Error(), "resource count") {
		t.Fatalf("combined ID/asset resource limit+1 = %#v, %v", over, err)
	}
}

func TestImportNodeEmbeddedRasterBombOverflowAndAnimationRejection(t *testing.T) {
	bomb := embeddedRasterPNG(t, 512, 512, color.NRGBA{A: 0xff})
	source := []byte(`<svg width="1" height="1"><image width="1" height="1" href="` + embeddedRasterDataURI("image/png", bomb) + `"/></svg>`)
	limits := generousImportLimits()
	limits.MaxBytes = 100_000
	if len(source) >= limits.MaxBytes {
		t.Fatalf("compressed bomb source = %d bytes, want below %d", len(source), limits.MaxBytes)
	}
	if result, err := ImportNode(context.Background(), "bomb.svg", source, limits); err == nil || result != nil || !strings.Contains(err.Error(), "decoded bytes") {
		t.Fatalf("compressed bomb = %#v, %v", result, err)
	}
	if _, err := embeddedRasterDecodedBytes(image.Config{Width: math.MaxInt, Height: math.MaxInt, ColorModel: color.NRGBAModel}); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("overflow dimensions error = %v", err)
	}

	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	animatedGIF := &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{1, 1}, Config: image.Config{Width: 1, Height: 1, ColorModel: frame.Palette}}
	var gifBuffer bytes.Buffer
	if err := gif.EncodeAll(&gifBuffer, animatedGIF); err != nil {
		t.Fatal(err)
	}
	staticPNG := embeddedRasterPNG(t, 1, 1, color.NRGBA{A: 0xff})
	animatedPNG := insertEmbeddedPNGChunk(staticPNG, "acTL", []byte{0, 0, 0, 1, 0, 0, 0, 0})
	frameControlPNG := insertEmbeddedPNGChunk(staticPNG, "fcTL", make([]byte, 26))
	frameDataPNG := insertEmbeddedPNGChunk(staticPNG, "fdAT", make([]byte, 4))
	animatedWebP := []byte{'R', 'I', 'F', 'F', 14, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', 'X', 1, 0, 0, 0, 2, 0}
	for _, test := range []struct {
		name string
		mime string
		data []byte
	}{
		{name: "GIF", mime: "image/gif", data: gifBuffer.Bytes()},
		{name: "APNG", mime: "image/png", data: animatedPNG},
		{name: "APNG frame control", mime: "image/png", data: frameControlPNG},
		{name: "APNG frame data", mime: "image/png", data: frameDataPNG},
		{name: "WebP", mime: "image/webp", data: animatedWebP},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(`<svg width="1" height="1"><image width="1" height="1" href="` + embeddedRasterDataURI(test.mime, test.data) + `"/></svg>`)
			if result, err := ImportNode(context.Background(), "animated.svg", source, generousImportLimits()); err == nil || result != nil || !strings.Contains(strings.ToLower(err.Error()), "animated") {
				t.Fatalf("animated %s = %#v, %v", test.name, result, err)
			}
		})
	}
}

func TestImportNodeEmbeddedRasterStrictAdversarialSubset(t *testing.T) {
	pngData := embeddedRasterPNG(t, 1, 1, color.NRGBA{R: 0xff, A: 0xff})
	jpegData := embeddedRasterJPEG(t, 1, 1, color.NRGBA{R: 0xff, A: 0xff})
	gifData := embeddedRasterGIF(t, 1, 1, color.NRGBA{R: 0xff, A: 0xff})
	webpData, err := base64.StdEncoding.DecodeString(embeddedRasterTestWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	jpegWithTrailingData := append(append([]byte(nil), jpegData...), []byte(`<script/>`)...)
	pngURI := embeddedRasterDataURI("image/png", pngData)
	base := func(href string) string {
		return `<svg width="1" height="1"><image width="1" height="1" href="` + href + `"/></svg>`
	}
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "HTTP", source: base("https://example.invalid/icon.png"), want: "external"},
		{name: "file", source: base("file:///tmp/icon.png"), want: "external"},
		{name: "blob", source: base("blob:https://example.invalid/id"), want: "external"},
		{name: "relative", source: base("icon.png"), want: "external"},
		{name: "local fragment", source: base("#icon"), want: "external"},
		{name: "uppercase scheme", source: base(strings.Replace(pngURI, "data:", "DATA:", 1)), want: "external"},
		{name: "leading space", source: base(" " + pngURI), want: "external"},
		{name: "SVG MIME", source: base(embeddedRasterDataURI("image/svg+xml", []byte(`<svg onload="alert(1)"/>`))), want: "unsupported data URI MIME"},
		{name: "HTML MIME", source: base(embeddedRasterDataURI("text/html", []byte(`<script>alert(1)</script>`))), want: "unsupported data URI MIME"},
		{name: "JPEG MIME alias", source: base(embeddedRasterDataURI("image/jpg", jpegData)), want: "unsupported data URI MIME"},
		{name: "uppercase MIME", source: base(strings.Replace(pngURI, "image/png", "IMAGE/PNG", 1)), want: "unsupported data URI MIME"},
		{name: "duplicate MIME", source: base("data:image/png;image/jpeg;base64,AAAA"), want: "one MIME type"},
		{name: "duplicate encoding", source: base("data:image/png;base64;base64,AAAA"), want: "one MIME type"},
		{name: "parameter", source: base("data:image/png;charset=utf-8;base64,AAAA"), want: "one MIME type"},
		{name: "encoding not last", source: base("data:image/png;base64;charset=utf-8,AAAA"), want: "one MIME type"},
		{name: "raw payload", source: base("data:image/png," + base64.StdEncoding.EncodeToString(pngData)), want: "one MIME type"},
		{name: "oversized metadata", source: base("data:" + strings.Repeat("x", 65) + ","), want: "metadata exceeds"},
		{name: "CSS URL wrapper", source: base("url(" + pngURI + ")"), want: "external"},
		{name: "missing comma", source: base("data:image/png;base64"), want: "comma"},
		{name: "empty payload", source: base("data:image/png;base64,"), want: "empty"},
		{name: "invalid alphabet", source: base("data:image/png;base64,!!!!"), want: "base64"},
		{name: "unpadded base64", source: base("data:image/png;base64,AAA"), want: "base64"},
		{name: "payload whitespace", source: base(strings.Replace(pngURI, ",", ",\n", 1)), want: "base64"},
		{name: "wrong MIME", source: base(embeddedRasterDataURI("image/png", jpegData)), want: "does not match"},
		{name: "fake PNG SVG", source: base(embeddedRasterDataURI("image/png", []byte(`<svg><script/></svg>`))), want: "signature"},
		{name: "truncated PNG", source: base(embeddedRasterDataURI("image/png", pngData[:20])), want: "malformed PNG"},
		{name: "PNG trailing HTML", source: base(embeddedRasterDataURI("image/png", append(append([]byte(nil), pngData...), []byte(`<script/>`)...))), want: "trailing"},
		{name: "JPEG trailing HTML", source: base(embeddedRasterDataURI("image/jpeg", jpegWithTrailingData)), want: "trailing"},
		{name: "GIF trailing HTML", source: base(embeddedRasterDataURI("image/gif", append(append([]byte(nil), gifData...), []byte(`<script/>`)...))), want: "trailing"},
		{name: "WebP trailing HTML", source: base(embeddedRasterDataURI("image/webp", append(append([]byte(nil), webpData...), []byte(`<script/>`)...))), want: "trailing"},
		{name: "missing href", source: `<svg width="1" height="1"><image width="1" height="1"/></svg>`, want: "requires one data"},
		{name: "missing width", source: `<svg width="1" height="1"><image height="1" href="` + pngURI + `"/></svg>`, want: "explicit width"},
		{name: "zero width", source: `<svg width="1" height="1"><image width="0" height="1" href="` + pngURI + `"/></svg>`, want: "positive"},
		{name: "negative height", source: `<svg width="1" height="1"><image width="1" height="-1" href="` + pngURI + `"/></svg>`, want: "negative"},
		{name: "percentage width", source: `<svg width="1" height="1"><image width="100%" height="1" href="` + pngURI + `"/></svg>`, want: "percentage"},
		{name: "overflow geometry", source: `<svg width="1" height="1"><image x="1e308" width="1e308" height="1" href="` + pngURI + `"/></svg>`, want: "finite numeric"},
		{name: "unsupported attribute", source: `<svg width="1" height="1"><image width="1" height="1" crossorigin="anonymous" href="` + pngURI + `"/></svg>`, want: "unsupported attribute"},
		{name: "event", source: `<svg width="1" height="1"><image width="1" height="1" onclick="alert(1)" href="` + pngURI + `"/></svg>`, want: "forbidden event"},
		{name: "child", source: `<svg width="1" height="1"><image width="1" height="1" href="` + pngURI + `"><title>x</title></image></svg>`, want: "cannot contain child"},
		{name: "preserve defer", source: `<svg width="1" height="1"><image width="1" height="1" preserveAspectRatio="defer xMidYMid" href="` + pngURI + `"/></svg>`, want: "defer"},
		{name: "duplicate href forms", source: `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="1" height="1"><image width="1" height="1" href="` + pngURI + `" xlink:href="` + pngURI + `"/></svg>`, want: "duplicate attribute"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ImportNode(context.Background(), "adversarial-image.svg", []byte(test.source), generousImportLimits())
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ImportNode() = %#v, %v; want %q", result, err, test.want)
			}
			if strings.Contains(err.Error(), base64.StdEncoding.EncodeToString(pngData)) {
				t.Fatal("image payload leaked into error")
			}
		})
	}
}

func TestImportNodeEmbeddedRasterCancellationAndConcurrentDeterminism(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*31 + y*17), G: uint8(x*7 + y*13), B: uint8(x ^ y), A: 0xff})
		}
	}
	data := encodeEmbeddedPNG(t, img)
	source := []byte(`<svg width="128" height="128"><image width="128" height="128" href="` + embeddedRasterDataURI("image/png", data) + `"/></svg>`)
	succeeded := false
	for remaining := int64(1); remaining <= 1024; remaining++ {
		ctx := &cancelAfterContext{remaining: remaining}
		result, err := ImportNode(ctx, "cancel-image.svg", source, generousImportLimits())
		if errors.Is(err, context.Canceled) {
			if result != nil {
				t.Fatalf("canceled import returned partial result at checkpoint %d", remaining)
			}
			continue
		}
		if err != nil || result == nil {
			t.Fatalf("checkpoint %d = %#v, %v", remaining, result, err)
		}
		succeeded = true
		break
	}
	if !succeeded {
		t.Fatal("image import never reached success across cancellation checkpoints")
	}

	const goroutines = 16
	const iterations = 10
	type outcome struct {
		id      d2scene.AssetID
		digest  [sha256.Size]byte
		metrics Metrics
		err     error
	}
	outcomes := make(chan outcome, goroutines*iterations)
	var group sync.WaitGroup
	for worker := 0; worker < goroutines; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				result, err := ImportNode(context.Background(), "concurrent-image.svg", source, generousImportLimits())
				if err != nil {
					outcomes <- outcome{err: err}
					continue
				}
				primitive := result.Root.Children[0].Primitive.(d2scene.Image)
				asset := result.Assets[primitive.Asset].(d2scene.RasterAsset)
				outcomes <- outcome{id: primitive.Asset, digest: sha256.Sum256(asset.Data), metrics: result.Metrics}
			}
		}()
	}
	group.Wait()
	close(outcomes)
	var baseline outcome
	for current := range outcomes {
		if current.err != nil {
			t.Fatal(current.err)
		}
		if baseline.id == "" {
			baseline = current
			continue
		}
		if current.id != baseline.id || current.digest != baseline.digest || current.metrics != baseline.metrics {
			t.Fatalf("nondeterministic result = %+v, want %+v", current, baseline)
		}
	}
}

func renderImportedRaster(t *testing.T, result *Result) *image.NRGBA {
	t.Helper()
	frame, err := renderImportedRasterResult(result)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func renderImportedRasterResult(result *Result) (*image.NRGBA, error) {
	content := d2scene.NewNode(nil)
	content.Transform = result.ViewportTransform
	content.Children = []*d2scene.Node{result.Root}
	document := d2scene.NewDocument(d2scene.Box{Width: result.Width, Height: result.Height}, content)
	for id, asset := range result.Assets {
		document.Assets[id] = asset
	}
	width, height := int(math.Ceil(result.Width)), int(math.Ceil(result.Height))
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: width, MaxHeight: height, MaxPixels: int64(width * height),
		MaxNodes: result.Metrics.EmittedElements + 10, MaxDepth: 100, MaxPathCommands: 100_000,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1,
		MaxAssets: len(result.Assets) + 1, MaxAssetBytes: 2 << 20, MaxDecodedAssetBytes: 2 << 20, MaxImportDepth: 10,
		MaxOffscreenBytes: 4 << 20, MaxEvenOddClipWork: 4 << 20,
	})
	return frame, err
}

func embeddedRasterDataURI(mimeType string, data []byte) string {
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func embeddedRasterPNG(t testing.TB, width, height int, value color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, value)
		}
	}
	return encodeEmbeddedPNG(t, img)
}

func encodeEmbeddedPNG(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), buffer.Bytes()...)
}

func embeddedRasterJPEG(t testing.TB, width, height int, value color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, value)
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), buffer.Bytes()...)
}

func embeddedRasterOrientedJPEG(t testing.TB, width, height int, orientation uint8) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, width, height))
	values := [4]uint8{20, 80, 150, 230}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			quadrant := 0
			if x >= width/2 {
				quadrant++
			}
			if y >= height/2 {
				quadrant += 2
			}
			img.SetGray(x, y, color.Gray{Y: values[quadrant]})
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 6+8+2+12+4)
	copy(payload, "Exif\x00\x00II")
	binary.LittleEndian.PutUint16(payload[8:10], 42)
	binary.LittleEndian.PutUint32(payload[10:14], 8)
	binary.LittleEndian.PutUint16(payload[14:16], 1)
	binary.LittleEndian.PutUint16(payload[16:18], 0x0112)
	binary.LittleEndian.PutUint16(payload[18:20], 3)
	binary.LittleEndian.PutUint32(payload[20:24], 1)
	binary.LittleEndian.PutUint16(payload[24:26], uint16(orientation))
	segment := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:4], uint16(len(payload)+2))
	encoded := buffer.Bytes()
	result := make([]byte, 0, len(encoded)+len(segment)+len(payload))
	result = append(result, encoded[:2]...)
	result = append(result, segment...)
	result = append(result, payload...)
	return append(result, encoded[2:]...)
}

func corruptEmbeddedPNGPixelData(t testing.TB, data []byte) []byte {
	t.Helper()
	result := append([]byte(nil), data...)
	marker := bytes.Index(result, []byte("IDAT"))
	if marker < 4 {
		t.Fatal("PNG fixture has no IDAT chunk")
	}
	length := int(binary.BigEndian.Uint32(result[marker-4 : marker]))
	if length < 4 || marker+4+length+4 > len(result) {
		t.Fatalf("PNG fixture has invalid IDAT length %d", length)
	}
	result[marker+4+length-1] ^= 0xff
	binary.BigEndian.PutUint32(result[marker+4+length:marker+4+length+4], crc32.ChecksumIEEE(result[marker:marker+4+length]))
	return result
}

func absByteDifference(left, right uint8) int {
	if left > right {
		return int(left - right)
	}
	return int(right - left)
}

func embeddedRasterGIF(t testing.TB, width, height int, value color.NRGBA) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.NRGBA{}, value})
	for index := range img.Pix {
		img.Pix[index] = 1
	}
	var buffer bytes.Buffer
	if err := gif.Encode(&buffer, img, nil); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), buffer.Bytes()...)
}

func insertEmbeddedPNGChunk(data []byte, chunkType string, payload []byte) []byte {
	chunk := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(payload)))
	copy(chunk[4:8], chunkType)
	copy(chunk[8:8+len(payload)], payload)
	binary.BigEndian.PutUint32(chunk[8+len(payload):], crc32.ChecksumIEEE(chunk[4:8+len(payload)]))
	const afterIHDR = 8 + 12 + 13
	result := make([]byte, 0, len(data)+len(chunk))
	result = append(result, data[:afterIHDR]...)
	result = append(result, chunk...)
	result = append(result, data[afterIHDR:]...)
	return result
}

const embeddedRasterTestWebPBase64 = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
