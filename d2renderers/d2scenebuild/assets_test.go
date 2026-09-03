package d2scenebuild

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/imageasset"
)

func TestResolvedJPEGEXIFOrientationRenders(t *testing.T) {
	assetURL := orientedJPEGAssetURL(t, 40, 24, 6)
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "oriented", Type: d2target.ShapeImage, Width: 24, Height: 40,
		Icon: assetURL, Opacity: 1, Fill: "none", Stroke: "none",
	}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad, Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Assets) != 1 {
		t.Fatalf("assets = %d, want one oriented JPEG", len(document.Assets))
	}
	for _, raw := range document.Assets {
		asset := raw.(d2scene.RasterAsset)
		if asset.MIMEType != "image/jpeg" || asset.PixelWidth != 24 || asset.PixelHeight != 40 || asset.DecodedBytes != 24*40*4 {
			t.Fatalf("oriented JPEG asset = %+v", asset)
		}
	}
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 24, MaxHeight: 40, MaxPixels: 24 * 40,
		MaxNodes: 10, MaxDepth: 10, MaxPathCommands: 100,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1,
		MaxAssets: 2, MaxAssetBytes: 1 << 20, MaxDecodedAssetBytes: 1 << 20, MaxImportDepth: 2,
		MaxOffscreenBytes: 1 << 20, MaxEvenOddClipWork: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if frame.Bounds() != image.Rect(0, 0, 24, 40) {
		t.Fatalf("oriented JPEG frame bounds = %v, want 24x40", frame.Bounds())
	}
	for _, test := range []struct {
		point image.Point
		gray  uint8
	}{
		{point: image.Pt(4, 4), gray: 150},
		{point: image.Pt(19, 4), gray: 20},
		{point: image.Pt(4, 35), gray: 230},
		{point: image.Pt(19, 35), gray: 80},
	} {
		pixel := frame.NRGBAAt(test.point.X, test.point.Y)
		if pixel.A != 255 || byteDistance(pixel.R, test.gray) > 20 || byteDistance(pixel.G, test.gray) > 20 || byteDistance(pixel.B, test.gray) > 20 {
			t.Fatalf("oriented JPEG pixel %v = %#v, want gray near %d", test.point, pixel, test.gray)
		}
	}
}

func TestBuildShapeImageAndOrdinaryIconUseResolvedOwnedAssets(t *testing.T) {
	t.Parallel()
	assetURL := testRasterAssetURL(t)
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{
		{
			ID: "photo", Type: d2target.ShapeImage,
			Pos: d2target.Point{X: 10, Y: 20}, Width: 100, Height: 50,
			Icon: assetURL, IconBorderRadius: 80, Opacity: 1,
			Fill: "#fff", Stroke: "none",
		},
		{
			ID: "card", Type: d2target.ShapeRectangle,
			Pos: d2target.Point{X: 150, Y: 20}, Width: 100, Height: 80,
			Icon: assetURL, IconPosition: "INSIDE_TOP_LEFT", IconBorderRadius: 7,
			Opacity: 1, Fill: "#fff", Stroke: "none",
		},
	}
	document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Assets) != 1 {
		t.Fatalf("deduplicated assets = %d, want 1", len(document.Assets))
	}
	var raster d2scene.RasterAsset
	for _, asset := range document.Assets {
		var ok bool
		raster, ok = asset.(d2scene.RasterAsset)
		if !ok {
			continue
		}
	}
	if raster.PixelWidth != 2 || raster.PixelHeight != 1 || raster.DecodedBytes < 8 || len(raster.Data) == 0 {
		t.Fatalf("raster asset = %+v", raster)
	}

	photo := findSceneNode(t, document.Root, "photo:image")
	photoImage := photo.Primitive.(d2scene.Image)
	if photoImage.Box != (d2scene.Box{X: 10, Y: 20, Width: 100, Height: 50}) || photoImage.Aspect != defaultImageAspect {
		t.Fatalf("shape image = %+v", photoImage)
	}
	if photo.Clip == nil || len(photo.Clip.Path.Commands) != 12 || photo.Clip.Path.Commands[2].Kind != d2scene.CubicCommand {
		t.Fatalf("shape image svg clip = %+v", photo.Clip)
	}
	// Radius 80 is clamped to half the 50px height.
	if got := photo.Clip.Path.Commands[0].P1.Y; got != 45 {
		t.Fatalf("shape image clamped clip starts at y=%v, want 45", got)
	}

	icon := findSceneNode(t, document.Root, "card:icon")
	iconImage := icon.Primitive.(d2scene.Image)
	if iconImage.Box != (d2scene.Box{X: 155, Y: 25, Width: 40, Height: 40}) || iconImage.Aspect != defaultImageAspect {
		t.Fatalf("ordinary icon = %+v", iconImage)
	}
	if icon.Clip == nil || len(icon.Clip.Path.Commands) != 10 || icon.Clip.Path.Commands[2].Kind != d2scene.ArcCommand {
		t.Fatalf("ordinary rounded icon clip = %+v", icon.Clip)
	}
	if photoImage.Asset != iconImage.Asset {
		t.Fatalf("repeated source asset IDs differ: %q != %q", photoImage.Asset, iconImage.Asset)
	}
}

func TestBuildStructuredAndCodeIconPaintOrder(t *testing.T) {
	t.Parallel()
	assetURL := testRasterAssetURL(t)
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{
		{
			ID: "class", Type: d2target.ShapeClass, Pos: d2target.Point{}, Width: 120, Height: 80,
			Icon: assetURL, IconPosition: "INSIDE_TOP_LEFT", IconBorderRadius: 9,
			Opacity: 1, Fill: "#fff", Stroke: "#000", StrokeWidth: 1,
			Text: d2target.Text{Label: "C", FontSize: 16, LabelWidth: 10, LabelHeight: 20},
		},
		{
			ID: "code", Type: d2target.ShapeCode, Pos: d2target.Point{X: 160}, Width: 120, Height: 80,
			Icon: assetURL, IconPosition: "INSIDE_TOP_LEFT",
			Opacity: 1, Fill: "#fff", Stroke: "#000", StrokeWidth: 1,
			Text: d2target.Text{Label: "x := 1", Language: "go", FontSize: 16, LabelWidth: 100, LabelHeight: 40},
		},
	}
	document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	class := findSceneNode(t, document.Root, "class")
	if got := class.Children[len(class.Children)-1]; got.ID != "class:icon" || got.Clip != nil {
		t.Fatalf("class final icon = %q clip=%v", got.ID, got.Clip)
	}
	classIcon := class.Children[len(class.Children)-1].Primitive.(d2scene.Image)
	if classIcon.Box.X != 5 || classIcon.Box.Y != 5 {
		t.Fatalf("class icon must use raw target box: %+v", classIcon.Box)
	}
	code := findSceneNode(t, document.Root, "code")
	if len(code.Children) < 2 || code.Children[0].ID != "code:icon" || code.Children[1].ID != "code:code-background" {
		t.Fatalf("code paint order = %v", childIDs(code.Children))
	}
}

func TestBuildConnectionIconAndDiagramWideMask(t *testing.T) {
	t.Parallel()
	assetURL := testRasterAssetURL(t)
	diagram := d2target.NewDiagram()
	diagram.Connections = []d2target.Connection{{
		ID: "edge", Route: []*geo.Point{{X: 0, Y: 50}, {X: 100, Y: 50}},
		Stroke: "#000", StrokeWidth: 2, Opacity: 1,
		SrcArrow: d2target.NoArrowhead, DstArrow: d2target.NoArrowhead,
		Icon: assetURL, IconPosition: "INSIDE_MIDDLE_CENTER", IconBorderRadius: 5,
		Text:          d2target.Text{Label: "label", FontSize: 16, LabelWidth: 30, LabelHeight: 20},
		LabelPosition: "INSIDE_MIDDLE_CENTER",
	}}
	document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	connection := findSceneNode(t, document.Root, "edge")
	if len(connection.Children) < 3 || connection.Children[0].ID != "edge:icon" || connection.Children[1].ID != "edge:geometry" {
		t.Fatalf("connection paint order = %v", childIDs(connection.Children))
	}
	topLeft := diagram.Connections[0].GetIconPosition()
	icon := connection.Children[0].Primitive.(d2scene.Image)
	if icon.Box != (d2scene.Box{X: topLeft.X, Y: topLeft.Y, Width: 32, Height: 32}) || connection.Children[0].Clip == nil {
		t.Fatalf("connection icon = %+v", icon)
	}
	mask := connection.Children[1].Mask
	if mask == nil || len(mask.Root.Children) != 2 {
		t.Fatalf("connection mask = %+v", mask)
	}
	hole := mask.Root.Children[1]
	holeRect := hole.Primitive.(d2scene.Rect)
	labelTopLeft := diagram.Connections[0].GetLabelTopLeft()
	want := d2scene.Box{
		X: math.Round(labelTopLeft.X) - 42, Y: math.Round(labelTopLeft.Y),
		Width: 74, Height: 20,
	}
	if holeRect.Box != want || hole.Opacity != 1 {
		t.Fatalf("label+icon mask hole = %+v opacity=%v, want %+v/1", holeRect.Box, hole.Opacity, want)
	}
}

func TestBuildSVGAssetPreservesIntrinsicViewportAndClipping(t *testing.T) {
	t.Parallel()
	source := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg width="40" height="20" viewBox="0 0 10 10" preserveAspectRatio="xMinYMin meet"><rect width="10" height="10" fill="red"/></svg>`
	raw := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(source))
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "vector", Type: d2target.ShapeImage, Width: 100, Height: 100,
		Icon: mustAssetURL(t, raw), Opacity: 1, Fill: "#fff", Stroke: "none",
	}}
	document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	imageNode := findSceneNode(t, document.Root, "vector:image")
	imagePrimitive := imageNode.Primitive.(d2scene.Image)
	asset, ok := document.Assets[imagePrimitive.Asset].(d2scene.VectorAsset)
	if !ok {
		t.Fatalf("SVG asset = %T", document.Assets[imagePrimitive.Asset])
	}
	if asset.ViewBox != (d2scene.Box{Width: 40, Height: 20}) || asset.Root == nil || asset.Root.Clip == nil || len(asset.Root.Children) != 1 {
		t.Fatalf("vector viewport = %+v root=%+v", asset.ViewBox, asset.Root)
	}
	content := asset.Root.Children[0]
	if content.Transform != (d2scene.Matrix{A: 2, D: 2}) || len(content.Children) != 1 {
		t.Fatalf("vector content transform = %+v", content.Transform)
	}
	if imagePrimitive.Aspect != defaultImageAspect {
		t.Fatalf("outer vector aspect = %+v", imagePrimitive.Aspect)
	}
}

func TestBuildSVGAssetsMergeAndDeduplicateEmbeddedRasterResources(t *testing.T) {
	t.Parallel()
	embedded := testRasterAssetURL(t).String()
	sources := []*url.URL{
		mustAssetURL(t, "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(
			`<svg width="2" height="1"><image width="2" height="1" preserveAspectRatio="none" href="`+embedded+`"/></svg>`,
		))),
		mustAssetURL(t, "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(
			`<svg width="2" height="1"><g><image width="2" height="1" preserveAspectRatio="none" href="`+embedded+`"/></g></svg>`,
		))),
	}
	diagram := d2target.NewDiagram()
	for index, source := range sources {
		diagram.Shapes = append(diagram.Shapes, d2target.Shape{
			ID: string(rune('a' + index)), Type: d2target.ShapeImage,
			Pos: d2target.Point{X: index * 2}, Width: 2, Height: 1,
			Icon: source, Opacity: 1, Fill: "none", Stroke: "none",
		})
	}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad, Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Assets) != 3 {
		t.Fatalf("document assets = %d, want two vectors and one deduplicated embedded raster", len(document.Assets))
	}
	rasterCount := 0
	for _, asset := range document.Assets {
		if raster, ok := asset.(d2scene.RasterAsset); ok {
			rasterCount++
			if raster.MIMEType != "image/png" || raster.PixelWidth != 2 || raster.PixelHeight != 1 || len(raster.Data) == 0 {
				t.Fatalf("embedded raster = %+v", raster)
			}
		}
	}
	if rasterCount != 1 {
		t.Fatalf("embedded raster assets = %d, want 1", rasterCount)
	}
	for _, id := range []string{"a:image", "b:image"} {
		outer := findSceneNode(t, document.Root, id).Primitive.(d2scene.Image)
		vector := document.Assets[outer.Asset].(d2scene.VectorAsset)
		importedRoot := vector.Root.Children[0].Children[0]
		var importedImage d2scene.Image
		stack := []*d2scene.Node{importedRoot}
		for len(stack) != 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if image, ok := node.Primitive.(d2scene.Image); ok {
				importedImage = image
			}
			stack = append(stack, node.Children...)
		}
		if _, ok := document.Assets[importedImage.Asset].(d2scene.RasterAsset); !ok {
			t.Fatalf("%s embedded image asset %q = %T", id, importedImage.Asset, document.Assets[importedImage.Asset])
		}
	}

	if _, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 100, MaxHeight: 100, MaxPixels: 10_000,
		MaxNodes: 100, MaxDepth: 32, MaxPathCommands: 1_000,
		MaxAnimationTracks: 100, MaxAnimationKeyframes: 1_000,
		MaxAssets: 10, MaxAssetBytes: 1 << 20, MaxDecodedAssetBytes: 1 << 20, MaxImportDepth: 16,
		MaxOffscreenBytes: 1 << 20, MaxEvenOddClipWork: 1_000_000,
	}); err != nil {
		t.Fatalf("render embedded raster SVG assets: %v", err)
	}
}

func TestMergeSVGImportAssetsRejectsCollisionWithoutPartialMutation(t *testing.T) {
	t.Parallel()
	id := d2scene.AssetID("svg-raster:test")
	existing := d2scene.RasterAsset{MIMEType: "image/png", Data: []byte{1}, PixelWidth: 1, PixelHeight: 1, DecodedBytes: 4}
	b := &builder{ctx: context.Background(), assets: map[d2scene.AssetID]d2scene.Asset{id: existing}}
	imported := map[d2scene.AssetID]d2scene.Asset{
		id:  d2scene.RasterAsset{MIMEType: "image/png", Data: []byte{2}, PixelWidth: 1, PixelHeight: 1, DecodedBytes: 4},
		"z": existing,
	}
	if err := b.mergeSVGImportAssets("collision", imported); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("collision error = %v", err)
	}
	if len(b.assets) != 1 {
		t.Fatalf("assets after rejected collision = %d, want 1", len(b.assets))
	}
}

func TestBuildSVGAssetsEnforcesSharedImportWorkBudget(t *testing.T) {
	firstSource := `<svg width="10" height="10"><path d="M0 0L10 10"/></svg>`
	secondSource := `<svg width="10" height="10"><path d="M0 10L10 0"/></svg>`
	first := mustAssetURL(t, "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(firstSource)))
	second := mustAssetURL(t, "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(secondSource)))
	makeDiagram := func(sources ...*url.URL) *d2target.Diagram {
		diagram := d2target.NewDiagram()
		for index, source := range sources {
			diagram.Shapes = append(diagram.Shapes, d2target.Shape{
				ID: string(rune('a' + index)), Type: d2target.ShapeImage,
				Pos: d2target.Point{X: index * 20}, Width: 10, Height: 10,
				Icon: source, Opacity: 1, Fill: "none", Stroke: "none",
			})
		}
		return diagram
	}
	buildWithBudget := func(maxElements, maxCommands int, sources ...*url.URL) (*d2scene.Document, error) {
		options := testAssetOptions(t)
		options.SVGImportBudget = SVGImportBudget{
			MaxSourceBytes: 10_000, MaxElements: maxElements, MaxAttributes: 100, MaxAttributeBytes: 10_000,
			MaxPathCommands: maxCommands, MaxTransformFunctions: 100,
			MaxDeclaredResources: 100, MaxExpandedUseInstances: 100,
		}
		return Build(context.Background(), makeDiagram(sources...), Options{Assets: options})
	}

	if document, err := buildWithBudget(2, 2, first); err != nil || document == nil {
		t.Fatalf("single SVG exactly at shared limits = %#v/%v", document, err)
	}
	if document, err := buildWithBudget(3, 4, first, second); err == nil || document != nil || !strings.Contains(err.Error(), "element count exceeds limit 1") {
		t.Fatalf("aggregate element limit = %#v/%v", document, err)
	}
	if document, err := buildWithBudget(4, 3, first, second); err == nil || document != nil || !strings.Contains(err.Error(), "path command count exceeds limit 1") {
		t.Fatalf("aggregate command limit = %#v/%v", document, err)
	}
	if document, err := buildWithBudget(4, 4, first, second); err != nil || document == nil || len(document.Assets) != 2 {
		t.Fatalf("two SVGs exactly at shared limits = %#v/%v", document, err)
	}
	sourceOptions := testAssetOptions(t)
	sourceOptions.SVGImportBudget.MaxSourceBytes = len(firstSource) + len(secondSource) - 1
	if document, err := Build(context.Background(), makeDiagram(first, second), Options{Assets: sourceOptions}); err == nil || document != nil || !strings.Contains(err.Error(), "exceeding limit") {
		t.Fatalf("aggregate source-byte limit = %#v/%v", document, err)
	}
	sourceOptions = testAssetOptions(t)
	sourceOptions.SVGImportBudget.MaxSourceBytes = len(firstSource) + len(secondSource)
	if document, err := Build(context.Background(), makeDiagram(first, second), Options{Assets: sourceOptions}); err != nil || document == nil {
		t.Fatalf("SVGs exactly at shared source-byte limit = %#v/%v", document, err)
	}

	// Unused definitions create parse work without retained output. Charge that
	// work too, and clamp the second parser before it can allocate its full tree.
	parseHeavyA := mustAssetURL(t, "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(
		`<svg width="10" height="10"><defs><path id="a" d="M0 0L1 1"/></defs></svg>`,
	)))
	parseHeavyB := mustAssetURL(t, "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString([]byte(
		`<svg width="10" height="10"><defs><path id="b" d="M0 0L2 2"/></defs></svg>`,
	)))
	if document, err := buildWithBudget(5, 100, parseHeavyA, parseHeavyB); err == nil || document != nil || !strings.Contains(err.Error(), "element count exceeds limit 2") {
		t.Fatalf("parse-heavy aggregate element limit = %#v/%v", document, err)
	}
	if document, err := buildWithBudget(6, 100, parseHeavyA, parseHeavyB); err != nil || document == nil || len(document.Assets) != 2 {
		t.Fatalf("parse-heavy SVGs exactly at shared limit = %#v/%v", document, err)
	}
}

func TestBuildImageAssetsFailBeforeReturningPartialDocument(t *testing.T) {
	t.Parallel()
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "missing", Type: d2target.ShapeImage, Width: 10, Height: 10,
		Opacity: 1, Fill: "#fff", Stroke: "none",
	}}
	if document, err := Build(context.Background(), diagram, Options{}); err == nil || document != nil {
		t.Fatalf("missing source = %#v/%v", document, err)
	}
	diagram.Shapes[0].Icon = mustAssetURL(t, "data:image/png;base64,not-base64")
	if document, err := Build(context.Background(), diagram, Options{}); err == nil || document != nil {
		t.Fatalf("missing resolver = %#v/%v", document, err)
	}
	if document, err := Build(context.Background(), diagram, Options{Assets: testAssetOptions(t)}); err == nil || document != nil {
		t.Fatalf("invalid data = %#v/%v", document, err)
	}
}

func TestBuildUnavailableImagesUseSharedPlaceholder(t *testing.T) {
	t.Parallel()
	first := mustAssetURL(t, "https://example.invalid/first.png")
	second := mustAssetURL(t, "https://example.invalid/second.png")
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{
		{
			ID: "first", Type: d2target.ShapeImage, Width: 64, Height: 64,
			Icon: first, Opacity: 1, Fill: "none", Stroke: "none",
		},
		{
			ID: "second", Type: d2target.ShapeImage, Pos: d2target.Point{X: 80}, Width: 64, Height: 64,
			Icon: second, Opacity: 1, Fill: "none", Stroke: "none",
		},
		{
			ID: "first-again", Type: d2target.ShapeImage, Pos: d2target.Point{X: 160}, Width: 64, Height: 64,
			Icon: first, Opacity: 1, Fill: "none", Stroke: "none",
		},
	}
	resolver, resolverCalls := newCountingAssetResolver(t, errors.New("test transport unavailable"))
	assetOptions := testAssetOptions(t)
	assetOptions.Resolver = resolver
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad, Assets: assetOptions})
	if err != nil {
		t.Fatal(err)
	}
	if resolverCalls.Load() != 2 {
		t.Fatalf("resolver calls = %d, want one per distinct source", resolverCalls.Load())
	}
	if len(document.Assets) != 1 {
		t.Fatalf("placeholder assets = %d, want 1", len(document.Assets))
	}
	asset, ok := document.Assets[unavailableImageAssetID].(d2scene.VectorAsset)
	if !ok || asset.ViewBox != (d2scene.Box{Width: 64, Height: 64}) || asset.Root == nil {
		t.Fatalf("placeholder asset = %#v", document.Assets[unavailableImageAssetID])
	}
	firstImage := findSceneNode(t, document.Root, "first:image").Primitive.(d2scene.Image)
	secondImage := findSceneNode(t, document.Root, "second:image").Primitive.(d2scene.Image)
	firstAgainImage := findSceneNode(t, document.Root, "first-again:image").Primitive.(d2scene.Image)
	if firstImage.Asset != unavailableImageAssetID || secondImage.Asset != unavailableImageAssetID || firstAgainImage.Asset != unavailableImageAssetID {
		t.Fatalf("placeholder image assets = %q/%q/%q", firstImage.Asset, secondImage.Asset, firstAgainImage.Asset)
	}

	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 250, MaxHeight: 100, MaxPixels: 25_000,
		MaxNodes: 100, MaxDepth: 32, MaxPathCommands: 1_000,
		MaxAnimationTracks: 100, MaxAnimationKeyframes: 1_000,
		MaxAssets: 10, MaxAssetBytes: 1 << 20, MaxDecodedAssetBytes: 1 << 20, MaxImportDepth: 16,
		MaxOffscreenBytes: 1 << 20, MaxEvenOddClipWork: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if frame.Bounds() != image.Rect(0, 0, 224, 64) {
		t.Fatalf("placeholder frame bounds = %v, want 224x64", frame.Bounds())
	}
	for _, x := range []int{16, 96, 176} {
		pixel := frame.NRGBAAt(x, 28)
		if int(pixel.B) <= int(pixel.R)+20 {
			t.Fatalf("placeholder sky pixel at (%d,28) = %#v", x, pixel)
		}
	}
}

func TestResolvedRasterDocumentRendersAfterRemoteOriginShutdown(t *testing.T) {
	assetURL := testRasterAssetURL(t)
	_, encodedAsset, ok := strings.Cut(assetURL.String(), ",")
	if !ok {
		t.Fatal("test raster data URI has no payload")
	}
	assetBytes, err := base64.StdEncoding.DecodeString(encodedAsset)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write(assetBytes)
	}))
	remote, err := url.Parse(server.URL + "/asset.png")
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "remote", Type: d2target.ShapeImage, Width: 10, Height: 10,
		Icon: remote, Opacity: 1, Fill: "none", Stroke: "none",
	}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad, Assets: testAssetOptions(t)})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	server.Close()

	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 100, MaxHeight: 100, MaxPixels: 10_000,
		MaxNodes: 100, MaxDepth: 32, MaxPathCommands: 1_000,
		MaxAnimationTracks: 100, MaxAnimationKeyframes: 1_000,
		MaxAssets: 10, MaxAssetBytes: 1 << 20, MaxDecodedAssetBytes: 1 << 20, MaxImportDepth: 16,
		MaxOffscreenBytes: 1 << 20, MaxEvenOddClipWork: 1_000_000,
	})
	if err != nil {
		t.Fatalf("render after remote origin shutdown: %v", err)
	}
	if frame.Bounds().Dx() == 0 || frame.Bounds().Dy() == 0 {
		t.Fatalf("render after remote origin shutdown returned empty frame %v", frame.Bounds())
	}
}

func TestOpacityZeroShapeAssetResolution(t *testing.T) {
	assetURL := mustAssetURL(t, "https://assets.example/image.png")
	tests := []struct {
		name        string
		shapeType   string
		wantResolve bool
	}{
		{name: "ordinary rectangle", shapeType: d2target.ShapeRectangle},
		{name: "text", shapeType: d2target.ShapeText},
		{name: "code", shapeType: d2target.ShapeCode},
		{name: "image", shapeType: d2target.ShapeImage, wantResolve: true},
		{name: "class", shapeType: d2target.ShapeClass, wantResolve: true},
		{name: "SQL table", shapeType: d2target.ShapeSQLTable, wantResolve: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver, resolverCalls := newCountingAssetResolver(t, &imageasset.LimitError{Name: "test resolver", Actual: 2, Limit: 1})
			diagram := d2target.NewDiagram()
			diagram.Shapes = []d2target.Shape{{
				ID: "hidden", Type: test.shapeType, Width: 20, Height: 20,
				Icon: assetURL, Opacity: 0, Fill: "#fff", Stroke: "none",
			}}
			document, err := Build(context.Background(), diagram, Options{Assets: &AssetOptions{Resolver: resolver}})
			if test.wantResolve {
				if err == nil || document != nil || resolverCalls.Load() != 1 {
					t.Fatalf("emitted opacity-zero asset = %#v/%v, resolver calls %d; want nil/error/1", document, err, resolverCalls.Load())
				}
				return
			}
			if err != nil || document == nil || resolverCalls.Load() != 0 {
				t.Fatalf("omitted opacity-zero asset = %#v/%v, resolver calls %d; want document/nil/0", document, err, resolverCalls.Load())
			}
			if len(document.Assets) != 0 {
				t.Fatalf("omitted opacity-zero asset retained %d scene assets", len(document.Assets))
			}
		})
	}
}

func TestInvalidAssetLayoutFailsBeforeResolverIO(t *testing.T) {
	assetURL := mustAssetURL(t, "https://assets.example/image.png")
	tests := []struct {
		name      string
		diagram   *d2target.Diagram
		wantField string
	}{
		{
			name: "connection icon position",
			diagram: &d2target.Diagram{Connections: []d2target.Connection{{
				ID: "edge", Route: []*geo.Point{{X: 0, Y: 0}, {X: 100, Y: 0}},
				Stroke: "#000", StrokeWidth: 2, Opacity: 1,
				Icon: assetURL, IconPosition: "NOT_A_POSITION",
			}}},
			wantField: "iconPosition",
		},
		{
			name: "structured shape dimensions",
			diagram: &d2target.Diagram{Shapes: []d2target.Shape{{
				ID: "class", Type: d2target.ShapeClass, Width: 0, Height: 40,
				Opacity: 1, Fill: "#fff", Stroke: "none",
				Icon: assetURL, IconPosition: "INSIDE_TOP_LEFT",
			}}},
			wantField: "dimensions",
		},
		{
			name: "shape icon position",
			diagram: &d2target.Diagram{Shapes: []d2target.Shape{{
				ID: "card", Type: d2target.ShapeRectangle, Width: 40, Height: 40,
				Opacity: 1, Fill: "#fff", Stroke: "none",
				Icon: assetURL, IconPosition: "NOT_A_POSITION",
			}}},
			wantField: "iconPosition",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver, resolverCalls := newCountingAssetResolver(t, &imageasset.LimitError{Name: "test resolver", Actual: 2, Limit: 1})
			document, err := Build(context.Background(), test.diagram, Options{Assets: &AssetOptions{Resolver: resolver}})
			if err == nil || document != nil || !strings.Contains(err.Error(), test.wantField) {
				t.Fatalf("Build() = %#v/%v, want nil error containing %q", document, err, test.wantField)
			}
			if resolverCalls.Load() != 0 {
				t.Fatalf("resolver calls = %d, want 0", resolverCalls.Load())
			}
		})
	}
}

func TestHashImageAssetHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hashImageAsset(ctx, imageasset.KindRaster, "image/png", make([]byte, 64<<10)); !errors.Is(err, context.Canceled) {
		t.Fatalf("hashImageAsset error = %v, want context.Canceled", err)
	}
}

func testAssetOptions(t *testing.T) *AssetOptions {
	t.Helper()
	return &AssetOptions{Resolver: newTestAssetResolver(t, nil), SVGImportLimits: d2svgimport.Limits{
		MaxBytes: 1 << 20, MaxDepth: 256, MaxElements: 10_000, MaxAttributes: 20_000,
		MaxAttributeBytes: 1 << 20, MaxPathCommands: 100_000, MaxTransformFunctions: 10_000,
		MaxUseDepth: 128, MaxResources: 10_000,
	}, SVGImportBudget: SVGImportBudget{
		MaxSourceBytes: 2 << 20, MaxElements: 20_000, MaxAttributes: 40_000, MaxAttributeBytes: 2 << 20,
		MaxPathCommands: 200_000, MaxTransformFunctions: 20_000,
		MaxDeclaredResources: 20_000, MaxExpandedUseInstances: 20_000,
	}}
}

type assetRoundTripFunc func(*http.Request) (*http.Response, error)

func (f assetRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newCountingAssetResolver(t *testing.T, failure error) (*imageasset.Resolver, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	client := &http.Client{Transport: assetRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, failure
	})}
	return newTestAssetResolver(t, client), calls
}

func newTestAssetResolver(t *testing.T, client *http.Client) *imageasset.Resolver {
	t.Helper()
	resolver, err := imageasset.New(imageasset.Options{HTTPClient: client, Limits: imageasset.Limits{
		MaxFetchedBytes: 1 << 20, MaxEncodedBytes: 1 << 20, MaxDecompressedBytes: 1 << 20, MaxSVGBytes: 1 << 20,
		MaxDecodedWidth: 1024, MaxDecodedHeight: 1024, MaxDecodedPixels: 1 << 20,
		MaxAssets: 64, MaxCumulativeEncodedBytes: 8 << 20, MaxCumulativeDecodedBytes: 16 << 20,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func testRasterAssetURL(t *testing.T) *url.URL {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 128})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return mustAssetURL(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(encoded.Bytes()))
}

func orientedJPEGAssetURL(t *testing.T, width, height int, orientation uint8) *url.URL {
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
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 100}); err != nil {
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
	jpegData := encoded.Bytes()
	data := make([]byte, 0, len(jpegData)+len(segment)+len(payload))
	data = append(data, jpegData[:2]...)
	data = append(data, segment...)
	data = append(data, payload...)
	data = append(data, jpegData[2:]...)
	return mustAssetURL(t, "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(data))
}

func byteDistance(left, right uint8) int {
	if left > right {
		return int(left - right)
	}
	return int(right - left)
}

func mustAssetURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func childIDs(nodes []*d2scene.Node) []string {
	ids := make([]string, len(nodes))
	for index, node := range nodes {
		ids[index] = node.ID
	}
	return ids
}
