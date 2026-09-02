//go:build !js || !wasm

package d2scenebuild

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontfallback"
)

type countingFontFallbackResolver struct {
	inner d2fonts.FallbackResolver
	calls int
}

func (r *countingFontFallbackResolver) ResolveFallbacks(ctx context.Context, request d2fonts.FallbackRequest) ([]d2fonts.FallbackFont, error) {
	r.calls++
	return r.inner.ResolveFallbacks(ctx, request)
}

func TestBuildResolvesAndRendersBundledNotoColorEmojiWithinFontBudget(t *testing.T) {
	newResolver := func(t *testing.T) *d2fonts.BundledFallbackResolver {
		t.Helper()
		resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
			MaxRequestedRunes: 10, MaxBundledBytes: 8 * 1024 * 1024, MaxResolvedBytes: 8 * 1024 * 1024,
		})
		if err != nil {
			t.Fatal(err)
		}
		return resolver
	}
	resolver := newResolver(t)
	resolved, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune{'\u2705', '\U0001F6E1'}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 {
		t.Fatalf("bundled fallback resources = %d, want 1", len(resolved))
	}
	primaryBytes := handDrawnFontBytes(t)
	fontBudget := int64(len(primaryBytes) + len(resolved[0].Data))
	diagram := mixedFontDiagram("✅ 🛡️")
	resolver = newResolver(t)
	document, err := Build(context.Background(), diagram, Options{
		Fonts: &FontFallbackOptions{Resolver: resolver, MaxAssets: 2, MaxBytes: fontBudget},
	})
	if err != nil {
		t.Fatal(err)
	}
	run := findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
	if len(run.Fallbacks) != 1 {
		t.Fatalf("font fallbacks = %#v, want one bundled face", run.Fallbacks)
	}
	asset, ok := document.Assets[run.Fallbacks[0]].(d2scene.FontAsset)
	if !ok || len(asset.Data) != len(resolved[0].Data) || asset.FaceIndex != 0 {
		t.Fatalf("bundled font asset = %#v", document.Assets[run.Fallbacks[0]])
	}
	if _, err := d2raster.Render(context.Background(), document, patternFrameOptions()); err != nil {
		t.Fatalf("raster bundled-emoji render error = %v", err)
	}

	limited, err := Build(context.Background(), diagram, Options{
		Fonts: &FontFallbackOptions{Resolver: newResolver(t), MaxAssets: 2, MaxBytes: fontBudget - 1},
	})
	if err == nil || limited != nil || !strings.Contains(err.Error(), "bytes exceed limit") {
		t.Fatalf("font budget result/error = %#v/%v", limited, err)
	}
}

func TestBuildVariationSelectorEmojiUsesBundledBaseCoverage(t *testing.T) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 4, MaxBundledBytes: 8 * 1024 * 1024, MaxResolvedBytes: 8 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingFontFallbackResolver{inner: resolver}
	document, err := Build(context.Background(), mixedFontDiagram("\u2708\ufe0f"), Options{Fonts: &FontFallbackOptions{
		Resolver: counting, MaxAssets: 2, MaxBytes: int64(len(handDrawnFontBytes(t)) + 8*1024*1024),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if counting.calls != 1 {
		t.Fatalf("variation-selector fallback calls = %d, want one base-rune request", counting.calls)
	}
	run := findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
	if len(run.Fallbacks) != 1 || len(run.Glyphs) == 0 {
		t.Fatalf("variation-selector fallbacks/glyphs = %#v/%#v", run.Fallbacks, run.Glyphs)
	}
	for _, glyph := range run.Glyphs {
		if !glyph.Empty && glyph.Asset != run.Fallbacks[0] {
			t.Fatalf("variation-selector glyph = %#v, want bundled fallback asset %q", glyph, run.Fallbacks[0])
		}
	}
}

func TestBuildReusesBundledNotoAcrossStyleBuckets(t *testing.T) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 4, MaxBundledBytes: 8 * 1024 * 1024, MaxResolvedBytes: 8 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingFontFallbackResolver{inner: resolver}
	diagram := mixedFontDiagram("✅")
	italic := diagram.Shapes[0]
	italic.ID = "italic"
	italic.Pos.Y = 80
	italic.Text.Italic = true
	diagram.Shapes = append(diagram.Shapes, italic)
	primaryBytes := handDrawnFontBytes(t)
	document, err := Build(context.Background(), diagram, Options{Fonts: &FontFallbackOptions{
		Resolver: counting, MaxAssets: 3, MaxBytes: int64(2*len(primaryBytes) + 8*1024*1024),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if counting.calls != 1 {
		t.Fatalf("bundled resolver calls = %d, want one shared resolution across regular and italic", counting.calls)
	}
	regularRun := findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
	italicRun := findSceneNode(t, document.Root, "italic:label:0").Primitive.(d2scene.TextRun)
	if len(regularRun.Fallbacks) != 1 || len(italicRun.Fallbacks) != 1 || regularRun.Fallbacks[0] != italicRun.Fallbacks[0] {
		t.Fatalf("regular/italic fallbacks = %#v/%#v, want one shared Noto asset", regularRun.Fallbacks, italicRun.Fallbacks)
	}
}

func TestBuildReusesBundledNotoAcrossConcurrentBuilds(t *testing.T) {
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 16, MaxBundledBytes: 16 * 1024 * 1024, MaxResolvedBytes: 16 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	primaryBytes := handDrawnFontBytes(t)
	options := Options{Fonts: &FontFallbackOptions{
		Resolver: resolver, MaxAssets: 2, MaxBytes: int64(len(primaryBytes) + 8*1024*1024),
	}}

	var documents [2]*d2scene.Document
	var errorsByBuild [2]error
	var group sync.WaitGroup
	for index := range documents {
		group.Add(1)
		go func() {
			defer group.Done()
			documents[index], errorsByBuild[index] = Build(context.Background(), mixedFontDiagram("✅"), options)
		}()
	}
	group.Wait()

	var assets [2]d2scene.FontAsset
	var ids [2]d2scene.AssetID
	for index, document := range documents {
		if errorsByBuild[index] != nil {
			t.Fatalf("Build %d: %v", index, errorsByBuild[index])
		}
		run := findSceneNode(t, document.Root, "mixed:label:0").Primitive.(d2scene.TextRun)
		if len(run.Fallbacks) != 1 {
			t.Fatalf("Build %d fallbacks = %#v, want one", index, run.Fallbacks)
		}
		ids[index] = run.Fallbacks[0]
		var ok bool
		assets[index], ok = document.Assets[ids[index]].(d2scene.FontAsset)
		if !ok || len(assets[index].Data) == 0 {
			t.Fatalf("Build %d bundled asset = %#v", index, document.Assets[ids[index]])
		}
	}
	if ids[0] != ids[1] || &assets[0].Data[0] != &assets[1].Data[0] {
		t.Fatalf("cross-build bundled assets do not share one resolver-owned resource: ids=%q/%q", ids[0], ids[1])
	}
	const notoColorEmojiBytes = 4_991_984
	if len(assets[0].Data) != notoColorEmojiBytes {
		t.Fatalf("bundled Noto bytes = %d, want %d", len(assets[0].Data), notoColorEmojiBytes)
	}
	stats, ok := fontfallback.CacheStatsFor(resolver)
	if !ok || stats.Assets != 1 || stats.Hashes != 1 || stats.Copies != 1 || stats.CopiedBytes != notoColorEmojiBytes {
		t.Fatalf("bundled scene cache stats = %+v/%v, want one resolver-owned copy", stats, ok)
	}

	callerOwned, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune{'✅'}})
	if err != nil {
		t.Fatal(err)
	}
	if len(callerOwned) != 1 || len(callerOwned[0].Data) != len(assets[0].Data) {
		t.Fatalf("public fallback result = %#v", callerOwned)
	}
	wantFirst := assets[0].Data[0]
	callerOwned[0].Data[0] ^= 0xff
	if assets[0].Data[0] != wantFirst || assets[1].Data[0] != wantFirst {
		t.Fatal("public mutable fallback result aliases the resolver-owned scene cache")
	}
}
