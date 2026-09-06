package d2scenebuild

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
	"github.com/d2lang/d2/d2target"
)

type staticFontFallbackResolver struct {
	fonts    []d2fonts.FallbackFont
	err      error
	calls    int
	requests []d2fonts.FallbackRequest
}

func (r *staticFontFallbackResolver) ResolveFallbacks(ctx context.Context, request d2fonts.FallbackRequest) ([]d2fonts.FallbackFont, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.calls++
	request.Runes = append([]rune(nil), request.Runes...)
	r.requests = append(r.requests, request)
	if r.err != nil {
		return nil, r.err
	}
	result := make([]d2fonts.FallbackFont, len(r.fonts))
	copy(result, r.fonts)
	for index := range result {
		result[index].Data = append([]byte(nil), result[index].Data...)
	}
	return result, nil
}

func TestBuildResolvesAndRendersMixedFontText(t *testing.T) {
	primaryBytes := handDrawnFontBytes(t)
	fallbackBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	resolver := &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{
		Name: "SourceSansPro-Regular.ttf", MIMEType: "font/ttf", Data: fallbackBytes,
	}}}
	diagram := mixedFontDiagram("A\u0416")
	document, err := Build(context.Background(), diagram, Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 2, MaxBytes: int64(len(primaryBytes) + len(fallbackBytes))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 || len(resolver.requests) != 1 || len(resolver.requests[0].Runes) != 1 || resolver.requests[0].Runes[0] != '\u0416' {
		t.Fatalf("resolver calls/requests = %d/%#v", resolver.calls, resolver.requests)
	}
	run := findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
	if len(run.Fallbacks) != 1 || run.Fallbacks[0] == "" {
		t.Fatalf("font fallbacks = %#v", run.Fallbacks)
	}
	if len(run.Glyphs) != 2 {
		t.Fatalf("shaped glyph count = %d, want 2", len(run.Glyphs))
	}
	if run.Glyphs[0].Asset != run.Font.Asset || run.Glyphs[1].Asset != run.Fallbacks[0] {
		t.Fatalf("shaped glyph assets = %#v, want primary then fallback", run.Glyphs)
	}
	asset, ok := document.Assets[run.Fallbacks[0]].(d2scene.FontAsset)
	if !ok || asset.FaceIndex != 0 || len(asset.Data) != len(fallbackBytes) {
		t.Fatalf("fallback asset = %#v", document.Assets[run.Fallbacks[0]])
	}
	frameOptions := patternFrameOptions()
	if _, err := d2raster.Render(context.Background(), document, frameOptions); err != nil {
		t.Fatalf("raster mixed-font render error = %v", err)
	}
}

func TestBuildKeepsFallbackBaseAndPrimaryMarkOnOneFace(t *testing.T) {
	primaryBytes := handDrawnFontBytes(t)
	fallbackBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	resolver := &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{
		Name: "SourceSansPro-Regular.ttf", MIMEType: "font/ttf", Data: fallbackBytes,
	}}}
	document, err := Build(context.Background(), mixedFontDiagram("\u0416\u0301"), Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 2, MaxBytes: int64(len(primaryBytes) + len(fallbackBytes))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolver.requests) != 1 {
		t.Fatalf("resolver requests = %#v, want one style-aware request", resolver.requests)
	}
	if got := string(resolver.requests[0].Runes); got != "\u0301\u0416" {
		t.Fatalf("resolver runes = %U (%q), want combining mark plus fallback base", resolver.requests[0].Runes, got)
	}
	run := findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
	if len(run.Glyphs) == 0 || len(run.Fallbacks) != 1 {
		t.Fatalf("glyphs/fallbacks = %#v/%#v", run.Glyphs, run.Fallbacks)
	}
	for index, glyph := range run.Glyphs {
		if glyph.Asset != run.Fallbacks[0] {
			t.Fatalf("grapheme glyph %d asset = %q, want fallback %q", index, glyph.Asset, run.Fallbacks[0])
		}
	}
}

func TestBuiltTextRendersRepeatedFramesFromExplicitGlyphs(t *testing.T) {
	document, err := Build(context.Background(), mixedFontDiagram("office"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	node := findSceneNode(t, document.Root, "mixed:label:0")
	run := node.Primitive.(d2scene.TextRun)
	if len(run.Glyphs) == 0 {
		t.Fatal("Build emitted no explicit glyphs")
	}
	for index, glyph := range run.Glyphs {
		if glyph.Asset == "" {
			t.Fatalf("glyph %d has no explicit asset", index)
		}
	}
	// If d2raster tried to shape this source again, its bounded UTF-8
	// preflight would reject it. Explicit glyph rendering must instead be
	// independent of the retained source string on every frame.
	run.Text = string([]byte{0xff})
	node.Primitive = run
	options := patternFrameOptions()
	first, err := d2raster.Render(context.Background(), document, options)
	if err != nil {
		t.Fatalf("first explicit-glyph frame: %v", err)
	}
	second, err := d2raster.Render(context.Background(), document, options)
	if err != nil {
		t.Fatalf("second explicit-glyph frame: %v", err)
	}
	if !first.Rect.Eq(second.Rect) || !bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("repeated explicit-glyph frames differ")
	}
}

func TestBuildRepeatedTextReusesShapingWithoutAliasingOrEvadingLimits(t *testing.T) {
	diagram := mixedFontDiagram("office")
	second := diagram.Shapes[0]
	second.ID = "mixed-2"
	second.Pos.Y = 80
	diagram.Shapes = append(diagram.Shapes, second)

	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	firstRun := findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
	secondRun := findSceneNode(t, document.Root, "mixed-2:label:0").Primitive.(d2scene.TextRun)
	if !reflect.DeepEqual(firstRun.Glyphs, secondRun.Glyphs) || len(firstRun.Glyphs) == 0 {
		t.Fatalf("repeated text glyphs differ: %#v / %#v", firstRun.Glyphs, secondRun.Glyphs)
	}
	if &firstRun.Glyphs[0] == &secondRun.Glyphs[0] {
		t.Fatal("repeated text nodes share mutable glyph storage")
	}
	secondID := secondRun.Glyphs[0].ID
	firstRun.Glyphs[0].ID++
	if secondRun.Glyphs[0].ID != secondID {
		t.Fatal("mutating one repeated text run changed another")
	}

	primaryBytes := handDrawnFontBytes(t)
	limited, err := Build(context.Background(), diagram, Options{Fonts: &FontFallbackOptions{
		MaxAssets: 1, MaxBytes: int64(len(primaryBytes)), MaxShapingRuns: 1,
	}})
	if err == nil || limited != nil || !strings.Contains(err.Error(), "text shaping run count exceeds limit 1") {
		t.Fatalf("repeated shaping limit result/error = %#v/%v", limited, err)
	}
}

func TestBuildFontFallbackFailuresAreExplicitAndAtomic(t *testing.T) {
	diagram := mixedFontDiagram("\u0416")
	primaryBytes := handDrawnFontBytes(t)
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil || document == nil {
		t.Fatalf("missing resolver result/error = %#v/%v", document, err)
	}
	run := findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
	if len(run.Glyphs) != 1 || run.Glyphs[0].ID == 0 {
		t.Fatalf("missing resolver glyphs = %#v, want a drawable placeholder", run.Glyphs)
	}
	if _, err := d2raster.Render(context.Background(), document, patternFrameOptions()); err != nil {
		t.Fatalf("render missing resolver placeholder: %v", err)
	}

	fallbackBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	resolver := &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{Name: "fallback", MIMEType: "font/ttf", Data: fallbackBytes}}}
	if document, err := Build(context.Background(), diagram, Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 2, MaxBytes: int64(len(primaryBytes) + len(fallbackBytes) - 1)},
	}); err == nil || document != nil || !strings.Contains(err.Error(), "bytes exceed limit") {
		t.Fatalf("byte limit result/error = %#v/%v", document, err)
	}

	resolver = &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{Name: "wrong", MIMEType: "font/ttf", Data: primaryBytes}}}
	document, err = Build(context.Background(), diagram, Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 2, MaxBytes: int64(2 * len(primaryBytes))},
	})
	if err != nil || document == nil {
		t.Fatalf("uncovered resolver result/error = %#v/%v", document, err)
	}
	run = findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
	if len(run.Glyphs) != 1 || run.Glyphs[0].ID == 0 {
		t.Fatalf("uncovered resolver glyphs = %#v, want a drawable placeholder", run.Glyphs)
	}

	resolver = &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{Name: "malformed", MIMEType: "font/ttf", Data: []byte("not a font")}}}
	if document, err := Build(context.Background(), diagram, Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 2, MaxBytes: int64(len(primaryBytes) + 10)},
	}); err == nil || document != nil || !strings.Contains(err.Error(), "parse TrueType/OpenType font") {
		t.Fatalf("malformed fallback result/error = %#v/%v", document, err)
	}

	resolver = &staticFontFallbackResolver{err: errors.New("resolver failed")}
	if document, err := Build(context.Background(), diagram, Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 2, MaxBytes: int64(len(primaryBytes) + 1)},
	}); err == nil || document != nil || !strings.Contains(err.Error(), "resolver failed") {
		t.Fatalf("resolver failure result/error = %#v/%v", document, err)
	}

	resolver = &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{Name: "missing MIME", Data: fallbackBytes}}}
	if document, err := Build(context.Background(), diagram, Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 2, MaxBytes: int64(len(primaryBytes) + len(fallbackBytes))},
	}); err == nil || document != nil || !strings.Contains(err.Error(), "has no MIME type") {
		t.Fatalf("MIME failure result/error = %#v/%v", document, err)
	}
}

func TestBuildDoesNotResolveFontsForCoveredText(t *testing.T) {
	primaryBytes := handDrawnFontBytes(t)
	resolver := &staticFontFallbackResolver{err: errors.New("must not be called")}
	document, err := Build(context.Background(), mixedFontDiagram("covered ASCII"), Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 1, MaxBytes: int64(len(primaryBytes))},
	})
	if err != nil || document == nil {
		t.Fatalf("covered Build result/error = %#v/%v", document, err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want zero", resolver.calls)
	}
}

func TestBuilderSharesParsedFaceAcrossCoverageAndShaping(t *testing.T) {
	data := handDrawnFontBytes(t)
	id := d2scene.AssetID("font:test")
	b := &builder{
		ctx:       context.Background(),
		assets:    map[d2scene.AssetID]d2scene.Asset{id: d2scene.FontAsset{MIMEType: "font/ttf", Data: append([]byte(nil), data...)}},
		fontFaces: make(map[d2scene.AssetID]*fontface.ParsedFace),
	}
	coverage, err := b.fontCoverage(id, make(map[d2scene.AssetID]sceneFontCoverage))
	if err != nil {
		t.Fatal(err)
	}
	shaping, err := b.fontFace(id)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.font != shaping || len(b.fontFaces) != 1 {
		t.Fatalf("coverage/shaping faces = %p/%p, cache=%#v; want one shared parse", coverage.font, shaping, b.fontFaces)
	}
	if coverage.source == nil {
		t.Fatal("bundled font coverage did not retain its authenticated read-only source")
	}
	for _, test := range []struct {
		value rune
		want  uint8
	}{
		{value: 'A', want: sceneRuneSupported},
		{value: utf8.MaxRune, want: sceneRuneUnsupported},
	} {
		first, err := coverage.supports(context.Background(), test.value)
		if err != nil {
			t.Fatalf("first coverage lookup for U+%04X: %v", test.value, err)
		}
		if got := coverage.runes[test.value]; got != test.want {
			t.Fatalf("cached coverage for U+%04X = %d, want %d", test.value, got, test.want)
		}
		second, err := coverage.supports(context.Background(), test.value)
		if err != nil || second != first {
			t.Fatalf("repeated coverage lookup for U+%04X = %v, %v; want %v, nil", test.value, second, err, first)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coverage.supports(canceled, 'A'); !errors.Is(err, context.Canceled) {
		t.Fatalf("cached coverage lookup with canceled context error = %v, want context.Canceled", err)
	}
}

func TestBuildFontFallbackWorkAndResultLimits(t *testing.T) {
	primaryBytes := handDrawnFontBytes(t)
	fallbackBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	baseOptions := func(resolver d2fonts.FallbackResolver) Options {
		return Options{Fonts: &FontFallbackOptions{
			Resolver: resolver, MaxAssets: 3, MaxBytes: int64(len(primaryBytes) + 2*len(fallbackBytes)),
			MaxRunesPerText: 100, MaxTotalRunes: 100, MaxCoverageChecks: 100,
		}}
	}

	t.Run("per text runes", func(t *testing.T) {
		options := baseOptions(&staticFontFallbackResolver{})
		options.Fonts.MaxRunesPerText = 2
		document, err := Build(context.Background(), mixedFontDiagram("ABC"), options)
		if err == nil || document != nil || !strings.Contains(err.Error(), "rune count exceeds per-text limit 2") {
			t.Fatalf("result/error = %#v/%v", document, err)
		}
	})

	t.Run("total runes", func(t *testing.T) {
		diagram := mixedFontDiagram("AB")
		second := diagram.Shapes[0]
		second.ID = "mixed-2"
		second.Pos.Y = 80
		diagram.Shapes = append(diagram.Shapes, second)
		options := baseOptions(&staticFontFallbackResolver{})
		options.Fonts.MaxTotalRunes = 3
		document, err := Build(context.Background(), diagram, options)
		if err == nil || document != nil || !strings.Contains(err.Error(), "text rune count exceeds total limit 3") {
			t.Fatalf("result/error = %#v/%v", document, err)
		}
	})

	t.Run("coverage checks", func(t *testing.T) {
		options := baseOptions(&staticFontFallbackResolver{})
		options.Fonts.MaxCoverageChecks = 1
		document, err := Build(context.Background(), mixedFontDiagram("AB"), options)
		if err == nil || document != nil || !strings.Contains(err.Error(), "font coverage checks exceed limit 1") {
			t.Fatalf("result/error = %#v/%v", document, err)
		}
	})

	t.Run("resolver result count before hashing", func(t *testing.T) {
		resolver := &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{
			{Name: "one", MIMEType: "font/ttf", Data: fallbackBytes},
			{Name: "duplicate", MIMEType: "font/ttf", Data: fallbackBytes},
			{Name: "duplicate-again", MIMEType: "font/ttf", Data: fallbackBytes},
		}}
		options := baseOptions(resolver)
		options.Fonts.MaxAssets = 2 // one primary leaves room for one result
		document, err := Build(context.Background(), mixedFontDiagram("\u0416"), options)
		if err == nil || document != nil || !strings.Contains(err.Error(), "returned 3 resources") {
			t.Fatalf("result/error = %#v/%v", document, err)
		}
	})

	t.Run("negative work limit", func(t *testing.T) {
		options := baseOptions(&staticFontFallbackResolver{})
		options.Fonts.MaxTotalRunes = -1
		document, err := Build(context.Background(), mixedFontDiagram("A"), options)
		if err == nil || document != nil || !strings.Contains(err.Error(), "work limits must not be negative") {
			t.Fatalf("result/error = %#v/%v", document, err)
		}
	})
}

func TestBuildPreservesStyleAcrossFallbackRequests(t *testing.T) {
	primaryBytes := handDrawnFontBytes(t)
	fallbackBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not loaded")
	}
	resolver := &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{
		Name: "fallback", MIMEType: "font/ttf", Data: fallbackBytes,
	}}}
	diagram := mixedFontDiagram("\u0416")
	italic := diagram.Shapes[0]
	italic.ID = "italic"
	italic.Pos.Y = 80
	italic.Text.Italic = true
	diagram.Shapes = append(diagram.Shapes, italic)
	document, err := Build(context.Background(), diagram, Options{Fonts: &FontFallbackOptions{
		Resolver: resolver, MaxAssets: 3, MaxBytes: int64(2*len(primaryBytes) + len(fallbackBytes)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if document == nil || len(resolver.requests) != 2 {
		t.Fatalf("document/requests = %#v/%#v", document, resolver.requests)
	}
	styles := map[string]bool{}
	for _, request := range resolver.requests {
		styles[request.Style] = true
		if request.Family != string(d2fonts.HandDrawn) || request.Weight < 400 || len(request.Runes) != 1 || request.Runes[0] != '\u0416' {
			t.Fatalf("fallback request = %#v", request)
		}
	}
	if !styles[string(d2fonts.FONT_STYLE_REGULAR)] || !styles[string(d2fonts.FONT_STYLE_ITALIC)] {
		t.Fatalf("fallback request styles = %#v", styles)
	}
}

func handDrawnFontBytes(t *testing.T) []byte {
	t.Helper()
	data, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.HandDrawn, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("hand-drawn font is not loaded")
	}
	return data
}

func TestSafeFallbackNameTruncatesOnRuneBoundary(t *testing.T) {
	name := strings.Repeat("é", 81)
	got := safeFallbackName(name)
	if !utf8.ValidString(got) || len([]rune(got)) != 80 {
		t.Fatalf("safeFallbackName = %q (%d runes)", got, len([]rune(got)))
	}
}

func mixedFontDiagram(label string) *d2target.Diagram {
	diagram := d2target.NewDiagram()
	family := d2fonts.HandDrawn
	diagram.FontFamily = &family
	diagram.Shapes = []d2target.Shape{{
		ID: "mixed", Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: 0, Y: 0}, Width: 120, Height: 60,
		Fill: "#ffffff", Stroke: "#000000", StrokeWidth: 1, Opacity: 1,
		Text: d2target.Text{
			Label: label, FontSize: 18, FontFamily: "default", LabelWidth: 60, LabelHeight: 24,
		},
	}}
	return diagram
}
