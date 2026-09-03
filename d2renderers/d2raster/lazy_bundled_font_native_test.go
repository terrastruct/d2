//go:build !js || !wasm

package d2raster

import (
	"context"
	"testing"

	"golang.org/x/image/font/sfnt"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

var benchmarkLazyBundledFontPrepared *preparedDocument

func TestPreparedBundledEmojiDefersShapingFaceForExplicitGlyphs(t *testing.T) {
	data := lazyBundledEmojiData(t)
	prepared, err := parsePreparedFont(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.source == nil || !prepared.source.IsBundledNotoColorEmoji() || prepared.face != nil || prepared.shaping != nil {
		t.Fatalf("initial emoji source/face/shaping = %p/%p/%p", prepared.source, prepared.face, prepared.shaping)
	}
	var buffer sfnt.Buffer
	glyph, err := prepared.outline.GlyphIndex(&buffer, '😀')
	if err != nil || glyph == 0 {
		t.Fatalf("emoji glyph = %d, %v", glyph, err)
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 96, Height: 96}, d2scene.NewNode(nil))
	options := testOptions()
	options.MaxNodes = 20_000
	options.MaxPathCommands = 1_000_000
	p := &preflight{
		ctx: context.Background(), document: document, options: options,
		fonts: map[d2scene.AssetID]*preparedFont{"font": prepared},
	}
	run := d2scene.TextRun{
		Text: "😀", Origin: d2scene.Point{X: 8, Y: 72},
		Font: d2scene.Font{Asset: "font", Size: 64}, Fill: black,
		Glyphs: []d2scene.Glyph{{ID: uint32(glyph), Advance: 76}},
	}
	if _, err := p.text("emoji", run, d2scene.Identity(), animationOverrides{}, 0); err != nil {
		t.Fatal(err)
	}
	if prepared.face != nil || prepared.shaping != nil {
		t.Fatal("explicit emoji preparation constructed a shaping Face")
	}
}

func BenchmarkPrepareBundledFontText(b *testing.B) {
	ordinary := sessionFontData(b)
	emoji := lazyBundledEmojiData(b)
	for _, fontCase := range []struct {
		name  string
		data  []byte
		value rune
		size  float64
	}{
		{name: "ordinary", data: ordinary, value: 'A', size: 28},
		{name: "emoji", data: emoji, value: '😀', size: 64},
	} {
		parsed, err := parsePreparedFont(fontCase.data, 0)
		if err != nil {
			b.Fatal(err)
		}
		var buffer sfnt.Buffer
		glyph, err := parsed.outline.GlyphIndex(&buffer, fontCase.value)
		if err != nil || glyph == 0 {
			b.Fatalf("glyph %U = %d, %v", fontCase.value, glyph, err)
		}
		for _, textCase := range []struct {
			name     string
			explicit bool
		}{
			{name: "explicit-glyph", explicit: true},
			{name: "raw-text"},
		} {
			b.Run(fontCase.name+"/"+textCase.name, func(b *testing.B) {
				assetID := d2scene.AssetID("font")
				run := d2scene.TextRun{
					Text: string(fontCase.value), Origin: d2scene.Point{X: 8, Y: 72},
					Font: d2scene.Font{Asset: assetID, Size: fontCase.size}, Fill: black,
				}
				if textCase.explicit {
					run.Glyphs = []d2scene.Glyph{{ID: uint32(glyph), Advance: fontCase.size}}
				}
				document := d2scene.NewDocument(d2scene.Box{Width: 128, Height: 96}, d2scene.NewNode(run))
				document.Assets[assetID] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontCase.data}
				options := testOptions()
				options.MaxNodes = 20_000
				options.MaxPathCommands = 1_000_000
				ctx := context.Background()
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					prepared, err := prepare(ctx, document, options)
					if err != nil {
						b.Fatal(err)
					}
					benchmarkLazyBundledFontPrepared = prepared
				}
			})
		}
	}
}

func lazyBundledEmojiData(t testing.TB) []byte {
	t.Helper()
	resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{
		MaxRequestedRunes: 1, MaxBundledBytes: 8 << 20, MaxResolvedBytes: 8 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := resolver.ResolveFallbacks(context.Background(), d2fonts.FallbackRequest{Runes: []rune("😀")})
	if err != nil || len(fonts) != 1 {
		t.Fatalf("bundled emoji = %#v, %v", fonts, err)
	}
	return fonts[0].Data
}
