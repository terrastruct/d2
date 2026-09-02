package d2raster

import (
	"context"
	"testing"

	"golang.org/x/image/font/sfnt"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestPreparedBundledFontDefersShapingFaceForExplicitGlyphs(t *testing.T) {
	data, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("Source Sans Pro is not bundled")
	}
	prepared, err := parsePreparedFont(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.source == nil || prepared.face != nil || prepared.shaping != nil {
		t.Fatalf("initial bundled font source/face/shaping = %p/%p/%p", prepared.source, prepared.face, prepared.shaping)
	}
	var buffer sfnt.Buffer
	glyph, err := prepared.outline.GlyphIndex(&buffer, 'A')
	if err != nil || glyph == 0 {
		t.Fatalf("glyph A = %d, %v", glyph, err)
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 64, Height: 64}, d2scene.NewNode(nil))
	options := testOptions()
	options.MaxFontFacesPerText = 8
	options.MaxTextCoverageChecks = 1_000
	options.MaxTextShapingRuns = 1_000
	newPreflight := func() *preflight {
		return &preflight{
			ctx: context.Background(), document: document, options: options,
			fonts: map[d2scene.AssetID]*preparedFont{"font": prepared},
		}
	}
	explicit := d2scene.TextRun{
		Text: "A", Origin: d2scene.Point{X: 8, Y: 40},
		Font: d2scene.Font{Asset: "font", Size: 24}, Fill: black,
		Glyphs: []d2scene.Glyph{{ID: uint32(glyph), Advance: 16}},
	}
	if _, err := newPreflight().text("explicit", explicit, d2scene.Identity(), animationOverrides{}, 0); err != nil {
		t.Fatal(err)
	}
	if prepared.face != nil || prepared.shaping != nil {
		t.Fatal("explicit-glyph preparation constructed a shaping Face")
	}

	raw := explicit
	raw.Glyphs = nil
	if _, err := newPreflight().text("raw", raw, d2scene.Identity(), animationOverrides{}, 0); err != nil {
		t.Fatal(err)
	}
	if prepared.face == nil || prepared.shaping == nil || prepared.face != &prepared.lazyFace {
		t.Fatal("raw text did not construct one render-local shaping Face")
	}
	first := prepared.face
	if _, err := newPreflight().text("raw-again", raw, d2scene.Identity(), animationOverrides{}, 0); err != nil {
		t.Fatal(err)
	}
	if prepared.face != first {
		t.Fatal("repeated raw text replaced the render-local shaping Face")
	}
}

func TestRenderSessionBundledFontDefersShapingFaceForExplicitGlyphs(t *testing.T) {
	data := sessionFontData(t)
	document := sessionAssetDocument(map[d2scene.AssetID]d2scene.Asset{
		"font": d2scene.FontAsset{MIMEType: "font/ttf", Data: data},
	})
	asset := document.Assets["font"].(d2scene.FontAsset)
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries: 2, MaxCacheBytes: 16 << 20, MaxConcurrentLoads: 1,
	})
	for range 2 {
		prepared, err := session.font(context.Background(), document, "font", asset)
		if err != nil {
			t.Fatal(err)
		}
		if prepared.source == nil || prepared.face != nil || prepared.shaping != nil {
			t.Fatalf("session bundled font source/face/shaping = %p/%p/%p", prepared.source, prepared.face, prepared.shaping)
		}
	}
}
