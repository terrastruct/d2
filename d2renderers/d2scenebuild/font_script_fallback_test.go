package d2scenebuild

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
	"github.com/d2lang/d2/d2target"
)

func TestBuildCJKFallbackProducesGlyphPixels(t *testing.T) {
	const cjk = "彌楓"
	fallbackBytes := decodeScriptFontFixture(
		t, cjkFontFixtureGZIPBase64, 14_312,
		"1846ff7d7d481e9bd6895123f55f73cb952b315e5c5be231ca6983af17e99d1c",
	)
	resolver := &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{
		Name: "TestGVAROne.ttf", MIMEType: "font/ttf", Data: fallbackBytes,
	}}}
	document, err := Build(context.Background(), scriptFallbackDiagram("A"+cjk, false), Options{
		Fonts: &FontFallbackOptions{
			Resolver: resolver, MaxAssets: 2,
			MaxBytes: int64(len(handDrawnFontBytes(t)) + len(fallbackBytes)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertScriptFallbackRequest(t, resolver, []rune(cjk), d2fonts.FONT_STYLE_REGULAR)

	node := findSceneNode(t, document.Root, "mixed:label:0")
	run := node.Primitive.(d2scene.TextRun)
	fallbackID := assertScriptFallbackAsset(t, document, run, fallbackBytes)
	if len(run.Glyphs) != 3 {
		t.Fatalf("scene glyph count = %d, want primary A plus two CJK glyphs", len(run.Glyphs))
	}
	if run.Glyphs[0].Asset != run.Font.Asset {
		t.Fatalf("primary glyph asset = %q, want %q", run.Glyphs[0].Asset, run.Font.Asset)
	}
	assertDrawableFallbackGlyphs(t, run.Glyphs[1:], fallbackID, []uint32{2, 3})
	assertIncreasingGlyphPositions(t, run.Glyphs)

	face := parseScriptFixtureFace(t, fallbackBytes)
	assertScriptCoverage(t, face, cjk)
	assertFallbackPaint(t, document, node, run, 200, 20)
}

func TestBuildItalicArabicFallbackShapesAndRendersPixels(t *testing.T) {
	const arabic = "وبا"
	fallbackBytes := decodeScriptFontFixture(
		t, arabicFontFixtureGZIPBase64, 6_404,
		"f966d806342dc01dc6a1476a09b20f423321a7c6862258489e196965bbbeeff0",
	)
	resolver := &staticFontFallbackResolver{fonts: []d2fonts.FallbackFont{{
		Name: "Amiri-Arabic-test.ttf", MIMEType: "font/ttf", Data: fallbackBytes,
	}}}
	document, err := Build(context.Background(), scriptFallbackDiagram("A"+arabic, true), Options{
		Fonts: &FontFallbackOptions{
			Resolver: resolver, MaxAssets: 2,
			MaxBytes: int64(len(handDrawnFontBytes(t)) + len(fallbackBytes)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The explicit resolver is deliberately style-agnostic. The builder must
	// still preserve the requested style so a real resolver can choose a
	// matching face.
	assertScriptFallbackRequest(t, resolver, []rune{'ا', 'ب', 'و'}, d2fonts.FONT_STYLE_ITALIC)

	node := findSceneNode(t, document.Root, "mixed:label:0")
	run := node.Primitive.(d2scene.TextRun)
	if run.Font.Style != string(d2fonts.FONT_STYLE_ITALIC) {
		t.Fatalf("scene primary style = %q, want italic", run.Font.Style)
	}
	fallbackID := assertScriptFallbackAsset(t, document, run, fallbackBytes)
	if len(run.Glyphs) != 4 {
		t.Fatalf("scene glyph count = %d, want primary A plus three Arabic glyphs", len(run.Glyphs))
	}

	face := parseScriptFixtureFace(t, fallbackBytes)
	assertScriptCoverage(t, face, arabic)
	// Nominal glyphs for the logical input waw-beh-alef are 3,2,1. Scene-time
	// shaping emits visual RTL order alef-beh-waw and substitutes the joined
	// alef and beh forms (19,21), proving this is not nominal cmap rendering.
	assertDrawableFallbackGlyphs(t, run.Glyphs[1:], fallbackID, []uint32{19, 21, 3})
	assertIncreasingGlyphPositions(t, run.Glyphs)
	for index, value := range []rune{'و', 'ب', 'ا'} {
		nominal, ok := face.Shaping.NominalGlyph(value)
		want := []uint32{3, 2, 1}[index]
		if !ok || uint32(nominal) != want {
			t.Fatalf("Arabic nominal glyph %d for %U = %d/%v, want %d", index, value, nominal, ok, want)
		}
	}
	assertFallbackPaint(t, document, node, run, 40, 8)
}

func scriptFallbackDiagram(label string, italic bool) *d2target.Diagram {
	diagram := mixedFontDiagram(label)
	shape := &diagram.Shapes[0]
	shape.Type = d2target.ShapeText
	shape.Fill, shape.Stroke = "none", "none"
	shape.Italic = italic
	return diagram
}

func assertScriptFallbackRequest(t *testing.T, resolver *staticFontFallbackResolver, runes []rune, style d2fonts.FontStyle) {
	t.Helper()
	if resolver.calls != 1 || len(resolver.requests) != 1 {
		t.Fatalf("resolver calls/requests = %d/%#v, want one", resolver.calls, resolver.requests)
	}
	request := resolver.requests[0]
	if !slices.Equal(request.Runes, runes) ||
		request.Family != string(d2fonts.HandDrawn) ||
		request.Style != string(style) ||
		request.Weight != 400 {
		t.Fatalf("fallback request = %#v, want runes=%U HandDrawn/%s/400", request, runes, style)
	}
}

func assertScriptFallbackAsset(t *testing.T, document *d2scene.Document, run d2scene.TextRun, want []byte) d2scene.AssetID {
	t.Helper()
	if len(run.Fallbacks) != 1 || run.Fallbacks[0] == "" {
		t.Fatalf("scene fallbacks = %#v, want one explicit asset", run.Fallbacks)
	}
	asset, ok := document.Assets[run.Fallbacks[0]].(d2scene.FontAsset)
	if !ok || asset.FaceIndex != 0 || asset.MIMEType != "font/ttf" || !bytes.Equal(asset.Data, want) {
		t.Fatalf("fallback asset = %#v, want exact owned fixture bytes", document.Assets[run.Fallbacks[0]])
	}
	return run.Fallbacks[0]
}

func parseScriptFixtureFace(t *testing.T, data []byte) *fontface.ParsedFace {
	t.Helper()
	face, err := fontface.ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

func assertScriptCoverage(t *testing.T, face *fontface.ParsedFace, text string) {
	t.Helper()
	for _, value := range text {
		covered, err := face.SupportsRenderableRune(value)
		if err != nil {
			t.Fatalf("coverage for %U: %v", value, err)
		}
		nominal, nominalOK := face.Shaping.NominalGlyph(value)
		if !covered || !nominalOK || nominal == 0 {
			t.Fatalf("fixture coverage for %U = covered:%v nominal:%d/%v", value, covered, nominal, nominalOK)
		}
	}
}

func assertDrawableFallbackGlyphs(t *testing.T, glyphs []d2scene.Glyph, asset d2scene.AssetID, wantIDs []uint32) {
	t.Helper()
	if len(glyphs) != len(wantIDs) {
		t.Fatalf("fallback glyph count = %d, want %d", len(glyphs), len(wantIDs))
	}
	for index, glyph := range glyphs {
		if glyph.ID != wantIDs[index] || glyph.Empty || glyph.Asset != asset ||
			glyph.Advance <= 0 || !glyph.Ink.Valid || glyph.Ink.Width() <= 0 || glyph.Ink.Height() <= 0 {
			t.Fatalf("fallback glyph %d = %#v, want drawable ID %d on %q", index, glyph, wantIDs[index], asset)
		}
	}
}

func assertIncreasingGlyphPositions(t *testing.T, glyphs []d2scene.Glyph) {
	t.Helper()
	for index := 1; index < len(glyphs); index++ {
		if glyphs[index].Position.X <= glyphs[index-1].Position.X {
			t.Fatalf("glyph positions are not visually increasing at %d: %#v", index, glyphs)
		}
	}
}

type paintedGeometry struct {
	pixels int
	bounds image.Rectangle
}

func assertFallbackPaint(
	t *testing.T,
	document *d2scene.Document,
	node *d2scene.Node,
	run d2scene.TextRun,
	minExtraPixels, minExtraWidth int,
) {
	t.Helper()
	fullFrame, err := d2raster.Render(context.Background(), document, patternFrameOptions())
	if err != nil {
		t.Fatalf("raster full-text render: %v", err)
	}
	full := nonWhiteGeometry(fullFrame)
	if full.pixels == 0 || full.bounds.Empty() {
		t.Fatalf("raster full-text frame has no painted geometry in %v", fullFrame.Bounds())
	}
	repeatedFrame, err := d2raster.Render(context.Background(), document, patternFrameOptions())
	if err != nil {
		t.Fatalf("repeated raster full-text render: %v", err)
	}
	if !fullFrame.Bounds().Eq(repeatedFrame.Bounds()) || !bytes.Equal(fullFrame.Pix, repeatedFrame.Pix) {
		t.Fatal("repeated raster full-text frames differ")
	}

	primaryOnly := run
	primaryOnly.Glyphs = append([]d2scene.Glyph(nil), run.Glyphs[:1]...)
	node.Primitive = primaryOnly
	defer func() { node.Primitive = run }()
	primaryFrame, err := d2raster.Render(context.Background(), document, patternFrameOptions())
	if err != nil {
		t.Fatalf("raster primary-only control render: %v", err)
	}
	if !fullFrame.Bounds().Eq(primaryFrame.Bounds()) {
		t.Fatalf("full/control frame bounds = %v/%v", fullFrame.Bounds(), primaryFrame.Bounds())
	}
	primary := nonWhiteGeometry(primaryFrame)
	if primary.pixels == 0 || primary.bounds.Empty() {
		t.Fatalf("raster primary-only control has no painted geometry in %v", primaryFrame.Bounds())
	}
	if full.pixels < primary.pixels+minExtraPixels ||
		full.bounds.Dx() < primary.bounds.Dx()+minExtraWidth {
		t.Fatalf(
			"raster fallback pixels/bounds = %d/%v, primary-only = %d/%v; want at least +%d pixels and +%d width",
			full.pixels, full.bounds, primary.pixels, primary.bounds, minExtraPixels, minExtraWidth,
		)
	}
}

func nonWhiteGeometry(frame *image.NRGBA) paintedGeometry {
	result := paintedGeometry{}
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			pixel := frame.NRGBAAt(x, y)
			if pixel.R == 0xff && pixel.G == 0xff && pixel.B == 0xff && pixel.A == 0xff {
				continue
			}
			result.pixels++
			point := image.Rect(x, y, x+1, y+1)
			if result.pixels == 1 {
				result.bounds = point
			} else {
				result.bounds = result.bounds.Union(point)
			}
		}
	}
	return result
}

func decodeScriptFontFixture(t *testing.T, encoded string, wantSize int, wantSHA256 string) []byte {
	t.Helper()
	compressed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		t.Fatalf("decode embedded font fixture: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("open embedded font fixture: %v", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, int64(wantSize)+1))
	closeErr := reader.Close()
	if readErr != nil {
		t.Fatalf("inflate embedded font fixture: %v", readErr)
	}
	if closeErr != nil {
		t.Fatalf("close embedded font fixture: %v", closeErr)
	}
	if len(data) != wantSize {
		t.Fatalf("embedded font fixture size = %d, want %d", len(data), wantSize)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != wantSHA256 {
		t.Fatalf("embedded font fixture SHA-256 = %s, want %s", got, wantSHA256)
	}
	return data
}

// The fixtures are deterministic copies from go-text/typesetting-utils
// v0.0.0-20260223113751-2d88ac90dae3, compressed with gzip -n -9.
//
// TestGVAROne.ttf comes from Unicode's text-rendering-tests. Its embedded name
// metadata retains Copyright 2016 Monotype Hong Kong Ltd. and Monotype Imaging
// Inc. The upstream COPYING retains Copyright 2016 Unicode Inc. All rights
// reserved; the fixture is distributed under Apache-2.0.
//
// The Arabic fixture is the HarfBuzz in-house font
// 7bbd3175734d5d291e1c15271ec0cbb97b626ebf.ttf. Its embedded metadata retains
// Copyright 2010-2022 The Amiri Project Authors. It is distributed under the
// SIL Open Font License 1.1; D2 already distributes the complete OFL text in
// THIRD_PARTY_NOTICES.txt reproduces the complete SIL Open Font License 1.1.
const cjkFontFixtureGZIPBase64 = `
H4sIAAAAAAACA517C3wb1ZnvjGYkjayxNPKMJdmWI9ljR0nk2EmEY7DBBgdkcEJCQwjvgB9yYurYXttJCFt2R3hpC32FAi19AZdLd7O9paHdttBuaUuhD+gvsEkxRBARCywiJVIi2SN7ZI9G+52R/CjQvb13nNE55zuv7/y/57EVDMcwzIwJGIEN77ixYVP4J3Yew/B7gXp3z/6u4fNf5xuhLUH7sb6DXSNf8vZ/HcOqnob2tr1do8NYOQbjq45Am9k7cLhv/MfPHMAwzQ9h0e69MD4Wq3wN+uMYVvrTff6uXsMPjv4Kxsrwbt4HBP2PiWJY/xJo1+zbP3av3oLfAO1T0HYNDPV0VdaVPQPrfRnaq/Z33TuMfZX8DvTfgvoHu/b7XzhT/haGlTCw357hodGx3IvYJthvGPUfhPV1XEkdhjUhfmsOwvrFDLkN2q9CuxpDZ9dgWPozX/r8XebL0xhBRIGCnfzWe59D5VvMQ0HMmfMSrxHAE6ZHY9UH5hF/zBHwGcWcmJN4TV1p5aNRKSbsMcxWmKHBVqnze/N7wF6v4EcwLUZpHtag02zNl/gebBO+GS1AEOo65NOY5jf3vfm5BYQAmnj9ju07sCsx15PHiBtyXljvj1gWI7Gn1G21mtfRyaFmxrrVndpAPjzmxjzw0wBlE+aFT2f+5Bjag4NPXC1J0AMMc2AMUGjMdde2/hvvuWvotyPbHvq3h9/+Gv3IwCP//nX5yWO5HNrjb/fmwk///BvPP/79I9d85ZcH7z6waTg7+ORg936ma88ncPr4czd2N34tHtb8griD+A3ZQr6i9Wpf1Q3rFvQ/ht4irDvnxZ8HaRAgDSNgW6mebCN2BXYthtksVZZabyNva/SWuqEs9UKFb+QbUR19qrRSvgnIeqjnexapFiib1Lq+G9+ew44d4/u/fOyeGr6fP8Z/ud937Jpr+Oprjvn6j/l8P7r6mmMDndXHOjuPk5ULH546tGvjxkcPtdg37rL1X2Pv5Hn3w6tX19R0VvDu6mq3/KvqTTy/CfHflnuGeJnQgsXVgQx3Yf3YfmwM+yL2ZexRDNMC/5y+ugG3ejc1Qt2dr7ZqGnlWZdTC672cXqc3aWrdm5tWu5usts1NNp3eatOvdusa8E/S2Py4VfhKot7iteDQYTPh+KesgzdWlerrcUTF92qadtdutDRX2ZU9yvqNdAvvLNcRR5nvHpv9nv5ZDa7T11yKl9Y6bKXlvL3EbGootZZbPCau0lzjsNvK+GIHZ/JYynnKw5rZoIcvY6vcjJ0s4jwWq7Nord3oCmxVXnQM3e265B/wK9y8nXW5i1eZGeh2FK81c3blPs31+iv2r7Ud7NqjeSg7cauW3G7k8VPZX5PkdqrqtjE3Xvldn3LqkWs67ryi1Wvu9lSvqa1scLrKa51e3u30VDjXnXOvcjldvHn1KqdzbfUGk7uadSiJhip+1eqG0hp9Sb1zw+pLSjwe89ohoko+o/lym829uYxXXvY4KitqYZ691umuWWN12ysc+DMdHQ+WVRmabR4kTz73bfAOa8GeLgMN3IbdADK9BbsD9LgP5OluxZEcuVUaJMWmjTaThq+uz9O0Fp0ewFfVEqlfKRISgt1Su9qtdSMdzUumFqTFI40uvEh67samUiRumw6/Zcu6KjynpyhSq9MSeIu2tNpau45VHldmK7wNne6u/Z233eZh3EXNeieNn9KTSo57YmuNXtduLVVebWde/vzWz7/MtL+49UX6fz9Mf66vZdcf9+E/UHZ3P3S7qXLDpRUVd+513vzGQ/he5RHpJ/hE4MDub73Q+eKLnXscrm6nZ9Wuiqy8xc26O9hHv0RWlZiv3vcOXqJc8PnqfvGLpzo6/owa0j+RD9zh09o6WLBaHizgJUDsckDqNlX778O+irBawmK1ptHLlVptVaX8kl6CGTSpVRPOV7tV5Fa7ea6gsZ/EUqVbcBVA1IVemJ1fo8mm1atGkQeY/+t+Ew4D8M+2jui379y+X0vsIEhcM693XLq6pfnSihqKLNYZ9D49Y3ZbfNu3tjZ0+Hj6MNm2qb5+TY3yYfWahqvXd/VvB8x5R1OFHV9vtbRVMOdZ77rd5rZNnd7NzTZui2/HjWb82vayVW1m5xq7s7W69BznbdhDX7XRt3lDi83e6tnp3Gnf09T9XV9b2xO42X/1wJ8f8V1XV3eZp9OzyqIxuTudO+pd7M7Nrraqja191WXVa1o9tr7t+PEv7N396EtIQIOObdd5tm5WaPwnd2+w72yhWprbSG/XYMUt24gat3vkzpLWm1a7V/HmGm7rNY7OLVs2Xkk3dd3jvL2drFq35tp91aY2DHysO/ckeKz1WBno+FrQ8jasA7se+wx2O3ZXXseRJHQF/AD7jZpFoagkG9hAU6ONM+G2gjfDkZuFdkE4jV53k81bqracOLg50O/8Whbkw0HJeVAAzf3umy9bx9F0683VnZz5P93aqy67tqHT25p9pnMTt51ra25vxnfUdPzJu9F6+aYSTvs9+kzFJcr3W1pbrr6MfLpip/sJpa2lufmqS/uefXa7v0y/s9j84PSzz+IvV2v3aDSW7o5r79j/+KpK7++muO7mLcqF7/RcsXXHTXe28Uee4459z9v4jQETec/X7rCR7TpLR0v7f15txV/e3tR01dXfbfdwbe343u1e7xVX/6X9Qfy69Wu52krlhT+1KjuOvEISYQyjMA9o/c+IDZgFW42tw+ohBjeBBVyJYbUIH37pyFWqS260gBqDmSNVBuO38FwT+GQtUAhvXs3VgKBD4Qp/ye25pvT+ozZme7H1grKwuk/5nK+ls7Oz6c9Pd+5+QbNdOdup9NrIf37wwRe+9wWD8cEHn/IVr7Xi315/Kev2KH3NPiXVMHhSgyl4ufKR7yGlG7/lTd7n+yE+pDyCn6hr9nUr031XfaEbvJ0HdOElohaibhVWC9qwXo28V4FG7IToC/IG5sC38aVcCw6CBlvSePDqK3Be565qVH+QSG2ohFdvqSpVIy+8KGznFUK7pEm6TidTds76ouaHr1Q8zVmNGh+1enPpK9sfsj+robISelsDgVa1xMuUs9vx1374xBPPK4FAQPPY5h0t25uUWd+tNduLzX85V9XdgeOmZvwW5WiDzbymwrHFbexsOfwfaza7L3o+dE97XsHvaPI84Wma9yjvXHOpt62Z8jzwb8xla33bsWKsAaT3e7ABE2aFfHcVZI55KW6C03di27EbIW9DbOc1v7SqUYerTpsvRaJCTptFIbdQulF8zbfzJgDavxj0vRZOjb+1VSgQ/6P3l0PbzUP/2tCxTnmlTflfs53d3d2ds22dSmD2nc6j+FNHO7ONDzr4ttpa70N4sryUKS3nOuuUG/GnlKN4sa99M/5S230vcM5v9mzZWOvxNGX/gO9SXvVs+cr9R7ZuVe7BJYXSuDs7FQz/juJ/1ed7FZWaI1vrG9Z4mvit2etIH2nHv610b1yL/8Tne6f5tutR1HMDHs+BD68An5DPLVuwLaAFW7EdoAf5lMqST63y/hjAUEsCiR9BUihZFAn55YynyaKqvcWNVB3m1gIUf3z66UDgyJGOI0cE4bHHHmxtXfr3Ah4ZuXyVuWmTRzla0/p46w99Pp9yBhf3PO7rxMfbO3bv7mhX7u/0PY4P3HrrrdDcnX1S89Psz+/RXEd68LnsN5/H71bO4VblaQ/+HID2y6eeErZuhey3Cc73JOg5BVkwA3mxAzJqlD17wf8hfUeRHsX5QoT3WhgtMKtVZeuFwIxU36pDsiZA+72lnLUFX4rfqqbo8q2qUjafnaGxLMq5CnU82nxZXYdSMaO8u4WZ+am31II34WWPtPubmoq09dmZ3zbh+GaWatLPNSn46J+JojpDEW7uqCCNWk9xscbYcVvTXaERkNwe5aOnOzrcmuezN7zZ0XH/6vojt+OHjyj/jAj4UMvkSyZDx3xHx7qjR5U4N/ww8+Vnjh79fEeHcl//PzCfGeGUrxAb5P/Cb9k5WM3suM+q3EP8o/wg5N1e1Z/Vqlm4GWMLHqFB9WstECMWteFG7GY1UvRge7HP5jNcHqn3x+Ha3OjNA9aEnEMeNMCsdhEzoCLzsalmA6gV1lBNqaAntUtmlqfhU/iTPuWi0rmgnNzCLLy4mWM1CMTH2x/CLcrF861N5k3KwsKrTXDhgLbyTtNjbYobH7odH1Sm2h/DB7/ZrpxV66+3KU5cDrfvwe9QnnU6+Y4OXvPD7K43t2y5v3bd/b4yw4GHAVCgdLe+2XbyN2zbnM9X89xzyk0t+PBzz+XwK5Xf4g8DpgdbW2sk6c7W1jsl6X2onzypvKt2jra3X3/yZD7aPkM8Q1yiYloGqOat61LQuO3YTYUbwgp7qXXXltbmIyfCTtUshIWaJy57JAgqakRB0CPDBL3UL2b9kO7gX9Vwfd5L2A3tvPKAsl+Pb1Dmi8paKmjOt7ONY1pLlAheO/vKOU9fu8/c6fW2e1ztYF8N9+NX3r+h/WhbeU1rdfOqVVvt1cY9xN1Z0yC5TVej+T/yWX5rw9bdt9xiu/HurXhMcTZxN95i2/VkR8fRjg5qy/D3WP729s23XHLlXUozfvksvhGs981Z5VU86V6/0V2/98qbKpo6SjHwv87cv4I9rgW9a8TaMZ/qc5e1aggbxTB00VnhcPKOmG/UEcsXHH45S1m86hDuj917kFIiJ0Qs5zSALXQQQFzUPqRpaCMfP+L71rd8TzzRc417W6Pvysvcd1/rhNTKvdryoLfZe2NzK6V/wdnQuL5h48bth5sanC0aUw3r4dgS/Cj44YO+5i3Tlau0JV6nvVlPraluxXd9ZUtzh3IAOme3NG/BNzZ3tB/1dXT4nm1uarmqssXborRtqW6r4JrsFZu9d25pufFrbIml46a2mnUtG6s68AzfcSurr6kqb3CZa7NnfD6P5ky26mbN7dnv46fWrKJ5fuMNbKm1av0MaGMckW9GAzRPdHSs/zU8+Tu4C//80j39pvxvO9RPI7Twwm839HDDydcJyGn2FurkijFa8J33Fuq6FXQKK8X+pVBHN/hvFer0inoxcQp7vlA3qXQCw0kDtH5AfrFQxzGrdlOhrsFM2msLdQKr1u4u1MkVY7SYXXtvoa5bQaewddojhboRq9f+slCnV9SL9c9ro4W6aQWdWbGOZZkHYJwufqJQx7Gi4mcL9RV84gRGFf+oUCexkuKfF+pajCn+05ah4cMj/Xv3jbk2bdi42XX90ODQ2OFhv+vaocG9rq3oY9tYb72ra7B3ue+6/V17+6HnusGeetdVAwMudYFR14h/1D9y0N9bv8s/Otax+6qdOwb9O/17Dwx0jayguDbWb9iwYbd/ZLR/aFBtfOrC+8bGhpsbGg4dOlS/v9Bf3zO0f9e+/lFX39DgmGt0qG/sUNeI3wWEsX1+1/DI0LB/ZOywa6jv01mtcw3B7tDbD7x29fX1D/R3jfl7Xf7Bsf6xfv8o9I+ofQP9Pf7B0aGRUdeanqGBAX/PWP9B/8DhuqVl16p4oKEHRv2u7sOuw0MHEBs9Qwf9I7DigcFe/4jK1Jh/ZP8o2rOrsKrf1bV3xO/fD5vWu26Fafu6DgJT3WNd/YMwc+yTx/P3w0Ijrt7+EWBk4LCrb2Ro//IBgWWVgDa7abC/Z6jX79oyhLgf6z+wv3554L6uUdfeka7BMXWXlYPUw8Di/cP9wJXKrcr6Eiu9/aNjI/3dB9BUOOxfz65zAer7+0eRNOuAF78Kcc++rpG9/jrX2BCCqA6QGT7s2j/U298HMA4f6B7oH91Xt2LhOtcoIqoI1SGGGuBgo37QLZgJsllkCvGj9qOV1Y3HUDEK7CDKoX0FKFS+gf+VrEN/7xDgWg/a4Orq7QWhI5aXEFLRUZf5GD6oeWhoZKD3UH8vcDcIanRvz8CBUdCKOtfI0OGugbHD69HJ4Whd/b3rDwznWewfGfEfHOrp6h7wL5pIXjMWt3SPqsDk7a8wArYf8YMu9x7o8Reg6gGpw0GGB7oOr6DAsfuGRhD+MB4pCqzcf7AL6Sri9rMIszwfyyD8bTT3FVhE+y+LwjWEdG8U4eoa8O/tGoB9QVP8aI9PLlXvUjW6p2sQBneNDILAga2u7qEDYyv0ECyk2eX6uF1DHjeEDWOHsRGsH7z8PmwMMuFN2Aa4922G2vXQOwjvGIwYxvxAuVal7IXa1qXaNujvhduSC+sCSu+nzrsO2w+9e2GX/JzroOxR51yFDcCPawUHo2rLD6UfyoPwiVbfpVLGIO/cDXN2Qu45CJSd8O7FDsAKXTD208e44DT1cCb0s1tdcxT2Qhwu9/z9HCMOx2BMM2TDDdgh9acexv71/HoYOwTUXTC+Xz1RnzoC4TsKtT6oHVJ5RnvlR4zBWNQaBuqQusqIup5LHf//gmqdOid/9vzc/gKuXdDqg9YAvF1A86vy8quc9asvQjA/f2TFPDS+Rx03qvYg2hr1jEh6fqihuUhWA8Bd3adwu3aFfiyuekCVsQvrVk95GEYfWEIDrX1QxSDP4wF1rl/lahGpMbW9Xx2fP2fXx3j1q7S96ip+GJk/KdK7Wwu77YP+gwWkuqGvS0Uzv+fY3yU9v3qefQXeeqE1UkBkQD1XnyrP/Z8qwTzKyyMWT3YTjOtXMehV21vUmXnsEdIHYHT9p66IzoM4RmfuUnlePsvfWmlZMnnO+2Gt/gJWy9guo/5JVHpVyphqxd2w5uKuecn+T3vXqRqflyNaY9E26wq4+FdocY96uhE4m1/tHwP6ohbVFXRmWN1xv4pcP8w6XNjhAPAyoO6wT6V8Gsd1qnwXRy7rUN0SQg0FiY2qmj6wtGfebj6O1CI+dSsQzvO8fOKxpdZoAZ3FMYdglb/WimW88/j/LdTz83vVT7RqfcE3IC561VljK1D+pA4t684yN/+z/iz2HlLpA7DHIejrLWA3WPBG9wJaA6q88r6iTtW4Idi3C+ho//VLMs9LrUtdZT3MGf4rFJGNoXEHYXYPULtVL/TxKLLSZ3z8lO4lL/Px+PfXa+RPj/bK++VeWLVnib9lXcnbel4iw2o8Ovw3xuSl3acitaj/+fUXPUqeZ4RR15JfXcT2s0t6thKPT9OE/x/d3PcxFBfP/2lW4VLR3FfQ3UV9damS2KtKNH/evE/xL53j7+EKaeyyj+5ROcyvjDAaLFj4SMG/d6vjxv6GP8zHkGZ1xf9bvL5Z5TN/9m1LtXY1zvViN6oxpH9FzyJlcUT+L+7w5H6GvqfwKQ+u/m27CGMxfS6HmdVbMKN+n4BD3xY4BNkgXgfDjsA1/Ff5CXDHxFSaJk+Hu6XaB/dK9H0CDO6UmPodA7zwTQQMuwHusai1q7DrR/BKwNwYXNyBL/0zGGboxrCif4JLehQu5DvhIv4DYGQNhlmmMYzdh2Gld/8WuxITNHBTvgJmfwfW+yqO44/C+wC8qP41LBcoyi0/ATz/fmnFG9Dm+/6lTVFmZ2VJkhcWMhkoFxbm5zOZhQUZ6kCQ5+ehDzrhH3xI0vy8rH4WqLJESQsZ1JiXBUwMYLZAm62khGEs8JhMNF2sPjRtNBoMRfBQlF5PUQaDTqvV6fU6HQkPQWjUhyBK4WG1bEkJy5aO50+pKXyvYxwelYK3qN9cwTGSpkVRFIwZXLtAsSwbj9uIQBHJ0RTsQNEkGaiJRhNT0WjsXCJFWC9mSKNeTmcJnAJmZdJI6Ckxq6VIgczoNVpK0MzOTwtFVBGZFkmdxaSXBWCRocmcUCQXsaV0McMkk7JAAlxxUSBppsRiFLQlhmKGIgT9vI7WGvSyHMA1WYEgCVEeL5OSyciMJEknJuDhOEBJfgtqx98UAzXxOMMkEmZzKHQuA0xkZVISZ8VsZGrqQ4mUZY1EV3KcYBDTkiInp2LnRcGdzhhEUmPMkmJcms9G4MmgocUSrS8tkcxmiYzGsqyBFAzJNMguOYlmlUgZTZJkTXoumeRIo4XNzsTHLbIMECWn5WwkNDk5yfOsSTDK03J86sNwOJUIBgNtRqOLN5stJdbyssoSgpK1BEfhsijjlEYjzWrMrFa3QJJcqUZKygo1p5FyhIEi8Zl5wojPL2TixTOzCxdnk5KsSyRsNpvARyN6zmYmF2YWCKNpfi4+M7cgZuIzco4gMplIOk1uaiDHNSmWFTwIM4fT6bSFQqlEIsG73W7b+V+99BJNnzjBMMePg/TMikmfTlBmWtCk5bRAGEhJFpwUrafNWd3UxVQifj55dn46noyliKnoB2fOUHSxBhcuTWsoqkiSs1RlEa0skLFQKPauJOnjsdNTiXMXkheTkbnZeDJ6UYpFp95/N8SyoRNvRaa055LjqjYGQRvfAd1jZEcZnAqYSyRnZmVcgwfMyZmZufkLOE3TwD7DmASKLuMoimFSqUBxKhVKzCmlVCaVstsYgaE4kIeVY9JyxmaTZYIIaBEFyiK9Ph5HdZLMBbDM+OXZcOh3P4rHxQ+Ov/b73//h7Q9CZ2BP2ELylosM8/MLUudqW/AjszkWk6RYDNQgGZ1LU2wRLSVFUig6HU6nQ1ORZCKYEvBoNICHQoI+iuNROhqlEUUwzBQXzzCJpMxbxvmdNZxWr+c4zlJM6QgNDlaJ4wuz4tRHsVjM6/XGYtOUgIuJgDEWczho2uFwr3dVCnbC6nBQxeZNTbLsdJpZK2OQed4BT0BLgXU6HONG5Caczga3Ji4axivTZFFaZFmR4DasXbu21mHlACwE6IV5MRgU8POpK3M36B/9vXk+HHY6A4TTGQ7DNsE3/vTrV37289Tp4yeCLD03RxvnJfHVUDBgCAY5Ljg7NTX7hwD+h2CgKPjBzMwHQYbRn3ir4DnAx+IbQHqVJ5Is+A6KikQ48FXx8+ejKdj7XIquILOhC4JWU2TiqisEgtBSOgHEmTiRIBkTAwhohCozLs5SlFmmWIok0RqSFImcOCPRciTCWMn4jBjA7Y6AkaZiUxHawIizE6Tgkhxl1FRi1lzGylmJJB0VFGkyJs/HmCJzZiEeZsjxGjlymqbzUnz9pCyza8hYDJQpFCc5k/jWxMmTJ985kxL0egI8sERrBMNUTAR108kXYkI5Z8tILevY4Ic0necIRMNWMzUeryRgZ4QiaU6kJYajpRlJoM8l01HZWBS/aIqLsgD2nBSwhKARzyUF3RTIKwmOo+hsLDEpkoZ09KJ5nLAn/ysc8IZCNJ2aTqUqKirKHSTprKQJieUcBmkWnNeCyNhYcy4VkqRwKBVLJhkGJCoKOjNYAE5CfIgFuFTKARMdDleVWSp2OFJzkhFXBIMspklSSjEsLxjmSMZhlzL8hbTg2LTBk1QolxWwnhOdlC5Jl3t0C7KjdgOfk8fpBLmA/KgIdvC+NVBVtNYI0owbWTMlzYoaGuKOToxEcuIMqFXow4RZJxiVOMBZxkgXY1FQsAH9zyZoGmlOgADtCQosmcqk3/5DCgRLleIJuoiUnYzAyTqng7IVWYzaeJrEpcTFi3SaErCcgEUEiyhJVjtEQjAHM2V16LWpQux6F7zFKdA3al6rLZHm0+e0Aj37TvB9mk4mp85GwTrb5qeDcmkDlZQ4rtbtJkkcr7DS1jXuRs/atWaIj6uuZRgqmkrabB6nCNHYVmkWcUkqL5cqKlx2XjLNpOfmUHOeMZ+dOhMdF7jTb4nr6hl58pRERqJROMdfZuKnhGoNLcsJycyR4pm3g9L8Ar4wB2oUeu+992iQSCTyuWFJILWcLEYFbTicjIO1mcQ5WoyGolHQ86mp9yXBdG5qCmok+fobINnJmXGbM3khBSE2kZgIUkXgDDiuzFNDhy4KbSVOcV72bPCyBETparujQdYbGSk8GQ6H3W6WLStOzEnuWpvRGBXX20gymoxyNoaPRHhRjIWn2PJiWAs1QbtLbWXUuCFVqpf1mRxzvV2wSianMxT6hSLl9KnJN87GqLRZSscYoezs6feCj/7uQ4iNrNVqZeWJCZLszEWCH4gCnYoGKUpKU1Rw4p24YJDiLJtKgcOkAiYG3CcCoXk6FHr/zIaC/L4A8vsiyM+RoV11DaDOHJOIJtJpGVfCpNFojMdpOpEIVMXjyaTZHI9T4BBsNklKWUqtqRQ4/gTHzcylOC41btB+FJ1HzmIuIzQmIxHIg3Tp0GQabCUsiuEw2hx5pA8gKWDEdBrUNqXXRyL5N4M+xtuscvr9txE8EUeV227n61fTkdOnT5vNkYjdTpJms802NWWmQWOoZNLt9mhSFzyeWMzjDYer1rnCYcw13pY00Zk4nNxAyiaUM83PRKRZSPTcbvQinw6mEpoMOxzhRCqVAFFFoyx76pTbHY06oIKdykfC5wGb5wAbOg1ZDVGB7A6ewCZzidUmS1lCsUFKlozGWDaG3jjgo5YcB9VkMh5PpdQZQBewOUE7FY6eELWCJvTuRAAPvzdeJgeD734I8QFihDwFT5bRM2UGlC8KOCQJfCYDoU5Cxq/XozIFLpwG4yaTSb1eFGkUQAV9MSfPTJEiGdCS6XRFRfF4GynPJ78xkQY1gLTCHQyGz0a5Ugsli8CMLdNw/HgDep3BoFMtf/1rZzQqimYzeicnnUEi6IxEBX0DwzKTr78MNgN5KMmOr00biwxZtHcySYFuGJGbNi6A7ouQBoIZSuD102mbDUL8auQbIXdGQFAUQYDOIIklL8RlsyySWdAih0MUyTzSPwKkjwHSVyQpC5dIRmTGhALlDE1nMnY7eDb4QHWki4slUiWknxBFihlxPkmbzULJgpTVgpLApskMGhBylAjY9HiRHP4vUZSPh+TzqQCZWpBjMZT+oWjGcJCKpCAlZBMJVszlRKSNqAS6nhSjQdSdSr75xhtvTITDJ8IpEjT59OkzzLlxnYfLSLy7QlgXAe2i6VpkEOGwywV66JLgvEjdUAmpnsPtjkSSSZYNOCanE3yDExSY1JwLfxSLICYYA+pvfj0oYOy4OeOuNUrkaj5zERiLzaYCq4PBEydA+K+9dlKvP5kAbCDoB8Xjx8VToK+oDNtsCxlCKAP7CANe7jVr1rgpKn4+cZyijldUcEExjzTcv7B/B6QNtPowDE0H3DQolSTF47FYvhTh2gFatlQyTBS8rFEWrEYUNFIpo1EUCQLJIv9+oU0CS0iidWjIuSQJ8i61jk6sNvKv2YxeaXIS4ok0/w5cFMChQFYhR8wRQBBcRRq0JWlPJu3jeDgcMINcUnnvYTbLstkc0OZLAXoFAsScr7lRLZEgCII2puOnQ7Cww2E0arXnz0ci09O53MSE3Y6kM06nTpyg6VCIgUyH5wNtPB+LMQwK+gAwYBdGL0VNTKAXzimnUqkZDzxuaCZkSPggxBRduFA0PTs7DbkzFsrj+iLg+gv0t+MYSdHo7PFcjoIMWJJQ+REkJMgvx5GNQQBnHqA1gCTqQtoBWSvNwOUM+GPA+DVZEeJWHkA9GYAQTKB72bgrFXz9jVOyHHoTAhQlTU1JCFZpKj4vZRamUTugFzOZEHqEOjRmGsenATxRiqAauh6JdEaiisRkJpMUp2fBRjTppCwnx1dx4WgMuTEWJaHZLFxNoDTbrMU0SRhBFCVIZMiAEx98gLpQ1IFbY0aU4KKM1DxgLcLjqTTFRCIQb/SzF9MpXXE4TNPjbeGzyXePk2QwFHM6YymSTCGUU1nynQ+nTkfVdhj8H2OF60b4DZJ8IzYzE4tFdRAGUC3Ekmxo0iILJe7S6Ug2G4n/OTktrTFnIhBR8ui/D+i/D+hrXa50GhId9fqfBz9QPDknawBVMXUWXTsEHPq1ICLw6Q+gcQR0iYgqrEXEigqCQP7OAE95eS6HSqt1YWGxzTBINAEC0cYr0vF4Op3NplVBgOIjmUmR81IO7hSCLoVIodC4Di49yM6FFmM0alzQahciEaNagicn7aJohzSYnFGUGXB6FCqNyaQRrhIS6ge3z6D6eJssc9zkZENDRcXkZCYDGSqUZpulyAAeFjdMTpJkA3SDFUngkhOKgvpPnCAIREVC8pDk5s3IUzU2BoPoXoPKmRmCQCWKi0FwKyjhn5lZvXq8LUUQEB4TKEImUpAVg8HE5PnZ8PmPUpDVI4rZjAwMmHTxPB8BwzoRz2bjOt2HH6LScx76QzZbCJWaXE4D9pRCJWrDzXoBleAaJVQv5CP/AZL8GUjSpV4x6TmNoYKx2+3xOKiqyOhgKwlJMSITgjZ2UQbzHNeDQtOJ6FRccNGQISHflM+X02kUPiWOIiLnTS6XTMrT45WgwxPoZvkXuGugX1iQp5N2mSWzOukCHFcoh4szbUTiRPZDTQUTYhRPGwkqN0sHDOiykn8DJJ2Si+FqTsRSAqE3Z1PjqzIZZM0LUjKGLJDnU5BFvh0Mi7HIqVOgcEIjBD4DwEUmEiQZS34QpClLiUxJ54Jms8k0Pc1CCDKZINgkDfFZo6cyc/ZkPGfhxu0pltWDH0qtRY4I/FCI5WNSRszEBT0HG9lYLiMY9UGWDSK0U80VAiYLvI2bPBGcALnG3O7jx2n6+HGU/MROnYMLw7umhUhcZ7MXUJ8D1GcAdQGjhVKzGeVZIngSFNnlYpQV2EVBa7OVUxIh4FkYkxXpMiOwAOvF5uKAbE5iWE7AFMGCU4yFNs7AhRsJCgV9gZ9OxKMyRN1zU1EZ9SAjQuaJ6mgEao8LfGxy6bIJwXMmSuKQKfOseOIjaZ5KfhQ5zXELggY8h1A0D75MEiNRkmAEQkxMysKls3JWpAwcKU1MSOLUlBhSlJCoo7Qk+uVdVpyezAaDWZUInUi6FyTpAnrHSYaahsTOgH4pQZIcB5FOVwpZXFxiBc18cl4odjBkIsxUVpByyr6KDzjK3cXy1AJFVVBKiqE5vdFQRFt4PhQCfvkAAQlIJqA3yrNxUm/UCTbUk0zmcijOoHoohFKU6upxfYgxapGTDJhoGq4ksjH0xolIZOK9mEDgifMxECsdffvt2TRtSsXFwPq3pxIZMvz2ifdiSTEWDUYzC5NvnzgVcjhCCZ5PgLuTopMTE4lzmVj0fDIjMBM22wSiot4wz4e1AVyrDWDacQzDGQwXFRrDyEr1K1aVS9/IWnwIRCMriRuIKLYVc8MPr35fdfF/fTmgxP4b/2zZReg3AAA=
`

const arabicFontFixtureGZIPBase64 = `
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
`
