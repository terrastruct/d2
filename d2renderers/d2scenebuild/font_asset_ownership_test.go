package d2scenebuild

import (
	"bytes"
	"context"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestBuildOwnsIndependentBundledAndCustomFontAssets(t *testing.T) {
	bundledSpec := d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR}
	bundled, ok := d2fonts.FontFaces.Lookup(bundledSpec)
	if !ok || len(bundled) == 0 {
		t.Fatal("bundled primary font is unavailable")
	}
	first := buildFontOwnershipDocument(t, nil)
	second := buildFontOwnershipDocument(t, nil)
	firstAsset := first.Assets["font:SourceSansPro:regular"].(d2scene.FontAsset)
	secondAsset := second.Assets["font:SourceSansPro:regular"].(d2scene.FontAsset)
	if len(firstAsset.Data) == 0 || !bytes.Equal(firstAsset.Data, bundled) || !bytes.Equal(secondAsset.Data, bundled) {
		t.Fatal("built-in scene assets differ from the bundled source")
	}
	if &firstAsset.Data[0] == &bundled[0] || &secondAsset.Data[0] == &bundled[0] || &firstAsset.Data[0] == &secondAsset.Data[0] {
		t.Fatal("built-in scene assets do not have independent backing allocations")
	}
	wantBundled := bundled[0]
	wantSecond := secondAsset.Data[0]
	firstAsset.Data[0] ^= 1
	if bundled[0] != wantBundled || secondAsset.Data[0] != wantSecond {
		t.Fatal("mutating one built-in scene asset changed another owner")
	}
	firstAsset.Data[0] ^= 1
	wantFirst := firstAsset.Data[0]
	bundled[0] ^= 1
	if firstAsset.Data[0] != wantFirst || secondAsset.Data[0] != wantSecond {
		t.Fatal("mutating the public FontFaces entry changed an existing scene")
	}
	bundled[0] = wantBundled

	customFamily := d2fonts.FontFamily("SceneFontOwnershipTest")
	customSpec := d2fonts.Font{Family: customFamily, Style: d2fonts.FONT_STYLE_REGULAR}
	custom := append([]byte(nil), bundled...)
	d2fonts.FontFaces.Set(customSpec, custom)
	t.Cleanup(func() { d2fonts.FontFaces.Delete(customSpec) })
	firstCustomDocument := buildFontOwnershipDocument(t, &customFamily)
	secondCustomDocument := buildFontOwnershipDocument(t, &customFamily)
	firstCustom := firstCustomDocument.Assets["font:SceneFontOwnershipTest:regular"].(d2scene.FontAsset)
	secondCustom := secondCustomDocument.Assets["font:SceneFontOwnershipTest:regular"].(d2scene.FontAsset)
	if len(firstCustom.Data) == 0 || !bytes.Equal(firstCustom.Data, custom) || !bytes.Equal(secondCustom.Data, custom) {
		t.Fatal("custom scene font was not retained as an exact copy")
	}
	if &firstCustom.Data[0] == &custom[0] || &secondCustom.Data[0] == &custom[0] || &firstCustom.Data[0] == &secondCustom.Data[0] {
		t.Fatal("custom scene assets do not have independent backing allocations")
	}
	wantFirst = firstCustom.Data[0]
	wantSecond = secondCustom.Data[0]
	custom[0] ^= 1
	if firstCustom.Data[0] != wantFirst || secondCustom.Data[0] != wantSecond {
		t.Fatal("mutating caller-owned custom font bytes changed built scenes")
	}
	wantCustom := custom[0]
	firstCustom.Data[0] ^= 1
	if secondCustom.Data[0] != wantSecond || custom[0] != wantCustom {
		t.Fatal("mutating one custom scene asset changed another owner")
	}
}

func TestMarkdownAndMissingGlyphAssetsOwnBundledFontCopies(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "markdown", Type: d2target.ShapeRectangle,
		Width: 320, Height: 160, Fill: "#ffffff", Stroke: "#000000", StrokeWidth: 1, Opacity: 1,
		Text: d2target.Text{
			Label: "plain **bold** *italic* `code`", Language: "markdown", FontFamily: "default", FontSize: 16,
			LabelWidth: 280, LabelHeight: 120,
		},
	}}
	firstDocument, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	secondDocument, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for id, assetValue := range firstDocument.Assets {
		asset, ok := assetValue.(d2scene.FontAsset)
		if !ok {
			continue
		}
		var spec d2fonts.Font
		switch id {
		case "font:SourceSansPro:regular":
			spec = d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR}
		case "font:SourceSansPro:bold":
			spec = d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_BOLD}
		case "font:SourceSansPro:italic":
			spec = d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_ITALIC}
		case "font:SourceCodePro:regular":
			spec = d2fonts.Font{Family: d2fonts.SourceCodePro, Style: d2fonts.FONT_STYLE_REGULAR}
		default:
			continue
		}
		checked++
		bundled, ok := d2fonts.FontFaces.Lookup(spec)
		second := secondDocument.Assets[id].(d2scene.FontAsset)
		if !ok || len(asset.Data) == 0 || !bytes.Equal(asset.Data, bundled) || !bytes.Equal(second.Data, bundled) {
			t.Fatalf("markdown font asset %q differs from bundled source", id)
		}
		if &asset.Data[0] == &bundled[0] || &second.Data[0] == &bundled[0] || &asset.Data[0] == &second.Data[0] {
			t.Fatalf("markdown font asset %q does not own an independent copy", id)
		}
		wantSource, wantSecond := bundled[0], second.Data[0]
		asset.Data[0] ^= 1
		if bundled[0] != wantSource || second.Data[0] != wantSecond {
			t.Fatalf("mutating markdown font asset %q changed another owner", id)
		}
		asset.Data[0] ^= 1
	}
	if checked != 4 {
		t.Fatalf("checked %d markdown font assets, want 4", checked)
	}

	b := &builder{ctx: context.Background(), assets: make(map[d2scene.AssetID]d2scene.Asset)}
	id, _, err := b.ensureMissingGlyphFont(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	asset := b.assets[id].(d2scene.FontAsset)
	bundled, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	firstRegular := firstDocument.Assets["font:SourceSansPro:regular"].(d2scene.FontAsset)
	if !ok || len(asset.Data) == 0 || !bytes.Equal(asset.Data, bundled) {
		t.Fatal("missing-glyph font asset differs from bundled source")
	}
	if &asset.Data[0] == &bundled[0] || &asset.Data[0] == &firstRegular.Data[0] {
		t.Fatal("missing-glyph font asset does not own an independent copy")
	}
}

func buildFontOwnershipDocument(t *testing.T, family *d2fonts.FontFamily) *d2scene.Document {
	t.Helper()
	diagram := d2target.NewDiagram()
	diagram.FontFamily = family
	diagram.Shapes = []d2target.Shape{{
		ID: "label", Type: d2target.ShapeRectangle,
		Width: 120, Height: 60, Fill: "#ffffff", Stroke: "#000000", StrokeWidth: 1, Opacity: 1,
		Text: d2target.Text{Label: "owned", FontFamily: "default", FontSize: 16, LabelWidth: 50, LabelHeight: 20},
	}}
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return document
}
