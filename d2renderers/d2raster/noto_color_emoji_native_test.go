//go:build !js || !wasm

package d2raster

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image/color"
	"sync"
	"testing"

	"golang.org/x/image/font/sfnt"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

func TestNotoColorEmojiCOLRv1RendersPaletteAndDeterministically(t *testing.T) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 16, MaxBundledBytes: 8 * 1024 * 1024, MaxResolvedBytes: 8 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{
		// Default-ignorable variation selectors and joiners are intentionally
		// omitted: the scene builder resolves grapheme clusters and asks only
		// for their renderable code points.
		Runes: []rune("😀🇺🇸👩💻👋🏽1⃣✈"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fonts) != 1 || fonts[0].Name != "NotoColorEmoji-COLRv1-v2.051.ttf" {
		t.Fatalf("bundled fonts = %#v", fonts)
	}

	const assetID = d2scene.AssetID("font:noto-color-emoji")
	document := d2scene.NewDocument(d2scene.Box{Width: 420, Height: 80}, d2scene.NewNode(d2scene.TextRun{
		Text: "😀 🇺🇸 👩‍💻 👋🏽 1️⃣ ✈️", Origin: d2scene.Point{X: 8, Y: 58},
		Font: d2scene.Font{Family: "Noto Color Emoji", Size: 46, Asset: assetID},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, B: 255, A: 255}},
	}))
	document.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fonts[0].Data}

	options := testOptions()
	firstFrame, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	palette := make(map[color.NRGBA]bool)
	for y := firstFrame.Bounds().Min.Y; y < firstFrame.Bounds().Max.Y; y++ {
		for x := firstFrame.Bounds().Min.X; x < firstFrame.Bounds().Max.X; x++ {
			pixel := firstFrame.NRGBAAt(x, y)
			if pixel.A == 255 {
				palette[pixel] = true
			}
		}
	}
	if len(palette) < 4 {
		t.Fatalf("fully opaque palette colors = %d, want at least 4: %#v", len(palette), palette)
	}

	first, err := EncodePNG(context.Background(), firstFrame)
	if err != nil {
		t.Fatal(err)
	}
	wantPNG := [sha256.Size]byte{
		0xd0, 0x47, 0x35, 0xb7, 0x5c, 0x3a, 0xc8, 0xbc,
		0x0a, 0x4a, 0xfb, 0xc0, 0x08, 0x29, 0xfd, 0x9a,
		0xac, 0x8b, 0xba, 0x45, 0x2f, 0x4b, 0x90, 0xc1,
		0x87, 0x2b, 0xb7, 0x32, 0x3c, 0x28, 0xe9, 0x31,
	}
	if got := sha256.Sum256(first); got != wantPNG {
		t.Fatalf("representative Noto Color Emoji PNG SHA-256 = %x, want %x", got, wantPNG)
	}
	second, err := renderTestPNG(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("serial Noto Color Emoji renders are not byte-identical")
	}

	const workers = 6
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := renderTestPNG(context.Background(), document, options)
			if err != nil {
				errors <- err
				return
			}
			if !bytes.Equal(got, first) {
				errors <- fmt.Errorf("concurrent Noto Color Emoji render is not byte-identical")
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func TestPreparedTextPartPaintHonorsPaletteAlphaAndForeground(t *testing.T) {
	fill := &preparedPaint{kind: preparedRadialGradient}
	stroke := &preparedStroke{width: 2}

	gotFill, gotStroke := preparedTextPartPaint(preparedTextPart{userPaint: true}, fill, stroke)
	if gotFill != fill || gotStroke != stroke {
		t.Fatalf("user paint = %p/%p, want %p/%p", gotFill, gotStroke, fill, stroke)
	}
	gotFill, gotStroke = preparedTextPartPaint(preparedTextPart{foreground: true}, fill, stroke)
	if gotFill != fill || gotStroke != nil {
		t.Fatalf("foreground paint = %p/%p, want %p/nil", gotFill, gotStroke, fill)
	}
	want := color.NRGBA{R: 10, G: 20, B: 30, A: 40}
	gotFill, gotStroke = preparedTextPartPaint(preparedTextPart{color: want}, fill, stroke)
	if gotFill == nil || gotFill.kind != preparedSolidPaint || gotFill.solid != want || gotStroke != nil {
		t.Fatalf("palette paint = %#v/%#v", gotFill, gotStroke)
	}
}

func TestNotoColorEmojiColorLayersChargeStructuralNodes(t *testing.T) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 * 1024 * 1024, MaxResolvedBytes: 8 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(fonts) != 1 {
		t.Fatalf("bundled fallback = %#v, %v", fonts, err)
	}
	face, err := fontface.ParseFace(fonts[0].Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	var buffer sfnt.Buffer
	glyphID, err := face.Outline.GlyphIndex(&buffer, '😀')
	if err != nil || glyphID == 0 {
		t.Fatalf("emoji glyph = %d, %v", glyphID, err)
	}
	plan, colorGlyph, err := face.CompileBundledNotoColorEmojiCOLRv1Plan(uint32(glyphID))
	if err != nil || !colorGlyph || plan == nil || plan.Usage.PaintNodes == 0 {
		t.Fatalf("emoji plan = %#v/%v, %v", plan, colorGlyph, err)
	}
	prepared, err := parsePreparedFont(fonts[0].Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	first, found, err := prepared.bundledCOLRv1Plan(uint32(glyphID))
	if err != nil || !found || first == nil {
		t.Fatalf("first cached emoji plan = %#v/%v, %v", first, found, err)
	}
	second, found, err := prepared.bundledCOLRv1Plan(uint32(glyphID))
	if err != nil || !found || second != first || len(prepared.colrv1Plans) != 1 {
		t.Fatalf("second cached emoji plan = %#v/%v, %v; cache entries = %d", second, found, err, len(prepared.colrv1Plans))
	}
	if allocations := testing.AllocsPerRun(100, func() {
		cached, cachedFound, cachedErr := prepared.bundledCOLRv1Plan(uint32(glyphID))
		if cachedErr != nil || !cachedFound || cached != first {
			panic("cached COLRv1 lookup changed result")
		}
	}); allocations != 0 {
		t.Fatalf("cached COLRv1 plan lookup allocations = %g, want 0", allocations)
	}

	const assetID = d2scene.AssetID("font:noto-color-emoji-node-limit")
	const repeats = 4
	document := d2scene.NewDocument(d2scene.Box{Width: 240, Height: 80}, d2scene.NewNode(d2scene.TextRun{
		Text: "😀😀😀😀", Origin: d2scene.Point{X: 8, Y: 60},
		Font: d2scene.Font{Family: "Noto Color Emoji", Size: 48, Asset: assetID},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 255}},
	}))
	document.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fonts[0].Data}

	options := testOptions()
	// One scene node, one synthesized color-text root, and every occurrence's
	// COLRv1 paint nodes are charged even when all occurrences share one plan.
	options.MaxNodes = 1 + 1 + repeats*plan.Usage.PaintNodes - 1
	if _, err := Render(context.Background(), document, options); err == nil || !bytes.Contains([]byte(err.Error()), []byte("node count exceeds limit")) {
		t.Fatalf("limit-1 render error = %v", err)
	}
	options.MaxNodes++
	if _, err := Render(context.Background(), document, options); err != nil {
		t.Fatalf("exact structural limit render error = %v", err)
	}
}

func BenchmarkPreparedFontBundledCOLRv1Plan(b *testing.B) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 * 1024 * 1024, MaxResolvedBytes: 8 * 1024 * 1024,
	})
	if err != nil {
		b.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(fonts) != 1 {
		b.Fatalf("bundled fallback = %#v, %v", fonts, err)
	}
	prepared, err := parsePreparedFont(fonts[0].Data, 0)
	if err != nil {
		b.Fatal(err)
	}
	var buffer sfnt.Buffer
	glyphID, err := prepared.outline.GlyphIndex(&buffer, '😀')
	if err != nil || glyphID == 0 {
		b.Fatalf("emoji glyph = %d, %v", glyphID, err)
	}
	parsed := prepared.parsedFace()

	b.Run("compile", func(b *testing.B) {
		b.ReportAllocs()
		var plan *fontface.COLRv1Plan
		var found bool
		var compileErr error
		for b.Loop() {
			plan, found, compileErr = parsed.CompileBundledNotoColorEmojiCOLRv1Plan(uint32(glyphID))
		}
		if compileErr != nil || !found || plan == nil {
			b.Fatalf("compiled emoji plan = %#v/%v, %v", plan, found, compileErr)
		}
	})
	b.Run("cached", func(b *testing.B) {
		if _, found, err := prepared.bundledCOLRv1Plan(uint32(glyphID)); err != nil || !found {
			b.Fatalf("prime cached emoji plan = %v/%v", found, err)
		}
		b.ReportAllocs()
		var plan *fontface.COLRv1Plan
		var found bool
		var cachedErr error
		for b.Loop() {
			plan, found, cachedErr = prepared.bundledCOLRv1Plan(uint32(glyphID))
		}
		if cachedErr != nil || !found || plan == nil {
			b.Fatalf("cached emoji plan = %#v/%v, %v", plan, found, cachedErr)
		}
	})
}

func TestNotoColorEmojiCOLRv1SoftLightGlyphRenders(t *testing.T) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 * 1024 * 1024, MaxResolvedBytes: 8 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(fonts) != 1 {
		t.Fatalf("bundled fallback = %#v, %v", fonts, err)
	}
	face, err := fontface.ParseFace(fonts[0].Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	const glyphID = 3744 // Pinned Noto sequence glyph exercising SoftLight.
	plan, found, err := face.CompileBundledNotoColorEmojiCOLRv1Plan(glyphID)
	if err != nil || !found || !colrv1PlanHasMode(plan.Root, fontface.COLRv1CompositeSoftLight) {
		t.Fatalf("soft-light plan = %#v/%v/%v", plan, found, err)
	}

	const assetID = d2scene.AssetID("font:noto-color-emoji-soft-light")
	document := d2scene.NewDocument(d2scene.Box{Width: 96, Height: 96}, d2scene.NewNode(d2scene.TextRun{
		Origin: d2scene.Point{X: 8, Y: 72},
		Font:   d2scene.Font{Family: "Noto Color Emoji", Size: 64, Asset: assetID},
		Fill:   d2scene.SolidPaint{Color: color.NRGBA{A: 255}},
		Glyphs: []d2scene.Glyph{{ID: glyphID, Advance: 76}},
	}))
	document.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fonts[0].Data}
	options := testOptions()
	options.MaxNodes = plan.Usage.PaintNodes + 2
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !preparedTreeHasIsolatedCOLRv1SoftLight(prepared.root) {
		t.Fatal("prepared SoftLight glyph has no isolated PaintComposite group")
	}
	frame, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	painted := 0
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			if frame.NRGBAAt(x, y).A != 0 {
				painted++
			}
		}
	}
	if painted < 100 {
		t.Fatalf("soft-light glyph painted pixels = %d, want at least 100", painted)
	}
	pngData, err := EncodePNG(context.Background(), frame)
	if err != nil {
		t.Fatal(err)
	}
	wantPNG := [sha256.Size]byte{
		0xfb, 0x36, 0xe3, 0x7a, 0xf7, 0x4d, 0xfd, 0x07,
		0x07, 0x15, 0x62, 0x09, 0x51, 0x73, 0xa7, 0x3e,
		0x64, 0x0b, 0xdd, 0xbc, 0xe0, 0xdf, 0xc9, 0x3f,
		0xf8, 0x5d, 0xcd, 0x98, 0x64, 0xe3, 0xad, 0xd4,
	}
	if got := sha256.Sum256(pngData); got != wantPNG {
		t.Fatalf("soft-light Noto Color Emoji PNG SHA-256 = %x, want %x", got, wantPNG)
	}
}

func preparedTreeHasIsolatedCOLRv1SoftLight(node *preparedNode) bool {
	if node == nil {
		return false
	}
	if node.isolated {
		for _, child := range node.children {
			if child != nil && child.blend == preparedCOLRv1SoftLight {
				return true
			}
		}
	}
	if node.primitive != nil && preparedTreeHasIsolatedCOLRv1SoftLight(node.primitive.vector) {
		return true
	}
	for _, child := range node.children {
		if preparedTreeHasIsolatedCOLRv1SoftLight(child) {
			return true
		}
	}
	if node.mask != nil && preparedTreeHasIsolatedCOLRv1SoftLight(node.mask.root) {
		return true
	}
	return false
}

func colrv1PlanHasMode(paint fontface.COLRv1Paint, mode fontface.COLRv1CompositeMode) bool {
	switch paint := paint.(type) {
	case fontface.COLRv1Layers:
		for _, child := range paint.Paints {
			if colrv1PlanHasMode(child, mode) {
				return true
			}
		}
	case fontface.COLRv1Glyph:
		return colrv1PlanHasMode(paint.Paint, mode)
	case fontface.COLRv1Transform:
		return colrv1PlanHasMode(paint.Paint, mode)
	case fontface.COLRv1Composite:
		return paint.Mode == mode || colrv1PlanHasMode(paint.Source, mode) || colrv1PlanHasMode(paint.Backdrop, mode)
	}
	return false
}
