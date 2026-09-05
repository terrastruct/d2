//go:build !js || !wasm

package fontface

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/go-text/typesetting/font/opentype/tables"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

func TestBundledNotoColorEmojiProvenanceAndCoverage(t *testing.T) {
	data := bundledNotoColorEmojiForTest(t)
	compressed := testBundledNotoColorEmojiCache.compressed
	if len(compressed) != testBundledNotoColorEmojiBrotliSize {
		t.Fatalf("Brotli size = %d, want %d", len(compressed), testBundledNotoColorEmojiBrotliSize)
	}
	if digest := sha256.Sum256(compressed); digest != testBundledNotoColorEmojiBrotliSHA256 {
		t.Fatalf("Brotli SHA-256 = %x, want %x", digest, testBundledNotoColorEmojiBrotliSHA256)
	}
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != testBundledNotoColorEmojiSize {
		t.Fatalf("decoded size = %d, want %d", len(data), testBundledNotoColorEmojiSize)
	}
	if digest := sha256.Sum256(data); digest != bundledNotoColorEmojiCOLRv1SHA256 {
		t.Fatalf("SHA-256 = %x, want %x", digest, bundledNotoColorEmojiCOLRv1SHA256)
	}

	var buffer sfnt.Buffer
	family, err := face.Outline.Name(&buffer, sfnt.NameIDFamily)
	if err != nil {
		t.Fatalf("read family name: %v", err)
	}
	if family != "Noto Color Emoji" {
		t.Fatalf("family = %q, want Noto Color Emoji", family)
	}
	version, err := face.Outline.Name(&buffer, sfnt.NameIDVersion)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if !strings.HasPrefix(version, "Version 2.051;") {
		t.Fatalf("version = %q, want Version 2.051", version)
	}

	for _, value := range []rune{'\u2705', '\U0001F600', '\U0001F6E1'} {
		glyph, err := face.Outline.GlyphIndex(&buffer, value)
		if err != nil {
			t.Fatalf("glyph index for %U: %v", value, err)
		}
		if glyph == 0 {
			t.Fatalf("font does not cover %U", value)
		}
		bounds, hasInk, err := face.GlyphRenderBounds(uint32(glyph), fixed.I(64))
		if err != nil {
			t.Fatalf("load render bounds for %U: %v", value, err)
		}
		if !hasInk || bounds.Empty() {
			t.Fatalf("font has empty render bounds for %U", value)
		}
		if value == '\U0001F600' {
			bounds, _, err := face.GlyphRenderBounds(uint32(glyph), fixed.I(1024))
			if err != nil {
				t.Fatal(err)
			}
			want := fixed.Rectangle26_6{
				Min: fixed.Point26_6{X: fixed.I(64), Y: fixed.I(-896)},
				Max: fixed.Point26_6{X: fixed.I(1184), Y: fixed.I(192)},
			}
			if bounds != want {
				t.Fatalf("U+1F600 Y-down render bounds = %v, want %v", bounds, want)
			}
		}
		supported, err := face.SupportsRenderableRune(value)
		if err != nil || !supported {
			t.Fatalf("renderable coverage for %U = %v, %v", value, supported, err)
		}
	}

	tags := sfntTableTags(t, data)
	for _, required := range []string{"COLR", "CPAL", "glyf", "GSUB"} {
		if !tags[required] {
			t.Fatalf("font does not contain required table %q", required)
		}
	}
	colr := sfntTableData(t, data, "COLR")
	if len(colr) < 2 || binary.BigEndian.Uint16(colr[:2]) != 1 {
		t.Fatal("font does not contain a COLRv1 table")
	}
	for _, forbidden := range []string{"CBDT", "CBLC", "SVG ", "sbix"} {
		if tags[forbidden] {
			t.Fatalf("font unexpectedly contains alternate color table %q", forbidden)
		}
	}
}

func TestBundledNotoColorEmojiCmapPrefilterMatchesAuthenticatedFont(t *testing.T) {
	data := bundledNotoColorEmojiForTest(t)
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundledNotoColorEmojiCmap) != 160 {
		t.Fatalf("cmap range count = %d, want 160", len(bundledNotoColorEmojiCmap))
	}
	for index, span := range bundledNotoColorEmojiCmap {
		if span.first < 0 || span.last < span.first || index != 0 && bundledNotoColorEmojiCmap[index-1].last >= span.first {
			t.Fatalf("invalid cmap range %d: %#v", index, span)
		}
	}

	var buffer sfnt.Buffer
	for value := rune(0); value <= unicode.MaxRune; value++ {
		if value >= 0xd800 && value <= 0xdfff {
			continue
		}
		glyph, err := face.Outline.GlyphIndex(&buffer, value)
		if err != nil {
			t.Fatalf("glyph index for %U: %v", value, err)
		}
		if got, want := BundledNotoColorEmojiCoversRune(value), glyph != 0; got != want {
			t.Fatalf("cmap prefilter for %U = %v, want %v (glyph %d)", value, got, want, glyph)
		}
	}
	if BundledNotoColorEmojiCoversRune('\u2b24') {
		t.Fatal("U+2B24 BLACK LARGE CIRCLE unexpectedly passes the bundled-font cmap prefilter")
	}
	for _, value := range []rune{'\u2705', '\U0001f600'} {
		if !BundledNotoColorEmojiCoversRune(value) {
			t.Fatalf("real emoji %U is absent from the bundled-font cmap prefilter", value)
		}
	}
}

func TestBundledNotoColorEmojiShapesEmojiSequences(t *testing.T) {
	data := bundledNotoColorEmojiForTest(t)
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"single":              "😀",
		"regional indicators": "🇺🇸",
		"ZWJ":                 "👩‍💻",
		"skin tone":           "👋🏽",
		"keycap":              "1️⃣",
		"variation selector":  "✈️",
	} {
		t.Run(name, func(t *testing.T) {
			shaped, err := new(ShapingWorkspace).ShapeTextTransient(context.Background(), text, fixed.I(64), []ShapeFace{{
				ID: "NotoColorEmoji", Face: face,
			}}, ShapeLimits{Runes: 16, Faces: 1, CoverageChecks: 32, Runs: 16, Glyphs: 16})
			if err != nil {
				t.Fatal(err)
			}
			painted := 0
			for _, glyph := range shaped.Glyphs {
				// Some variation selectors survive shaping as zero-ink outline
				// glyphs. They affect selection but do not add a painted layer.
				if glyph.Empty || !glyph.HasInk {
					continue
				}
				painted++
			}
			if painted != 1 {
				t.Fatalf("painted glyph count = %d, want 1; glyphs = %#v", painted, shaped.Glyphs)
			}
		})
	}
}

func TestRegisteredBundledNotoColorEmojiClonesAndMatchesExactly(t *testing.T) {
	data := bundledNotoColorEmojiForTest(t)
	if _, err := RegisterOwnedBundledNotoColorEmoji(append([]byte(nil), data...)); err != nil {
		t.Fatal(err)
	}
	source, matched, err := RegisteredBundledNotoColorEmoji(data, 0)
	if err != nil || !matched || source == nil {
		t.Fatalf("registered source = %p, matched %v, %v", source, matched, err)
	}
	first, err := source.CloneReadOnly()
	if err != nil || !first.IsBundledNotoColorEmoji() {
		t.Fatalf("first clone = %p, %v", first, err)
	}
	second, err := source.CloneReadOnly()
	if err != nil || !second.IsBundledNotoColorEmoji() {
		t.Fatalf("second clone = %p, %v", second, err)
	}
	if first == second || first.Shaping == second.Shaping {
		t.Fatal("registered source reused mutable shaping state between clones")
	}
	if first.Shaping.Font != second.Shaping.Font {
		t.Fatal("registered source did not reuse the immutable shaping font")
	}
	outline, err := source.Outline()
	if err != nil || outline != first.Outline {
		t.Fatalf("source outline = %p, %v; want %p", outline, err, first.Outline)
	}
	var sourceBuffer sfnt.Buffer
	emojiGlyph, err := outline.GlyphIndex(&sourceBuffer, '😀')
	if err != nil || emojiGlyph == 0 {
		t.Fatalf("source emoji glyph = %d, %v", emojiGlyph, err)
	}
	if kind, err := source.GlyphDataKind(uint32(emojiGlyph)); err != nil || kind != "color" {
		t.Fatalf("source emoji glyph kind = %q, %v", kind, err)
	}
	plan, found, err := source.CompileBundledNotoColorEmojiCOLRv1Plan(uint32(emojiGlyph))
	if err != nil || !found || plan == nil {
		t.Fatalf("source emoji plan = %#v/%v, %v", plan, found, err)
	}
	bounds, hasInk, err := source.GlyphRenderBounds(uint32(emojiGlyph), fixed.I(64))
	if err != nil || !hasInk || bounds.Empty() {
		t.Fatalf("source emoji bounds = %v/%v, %v", bounds, hasInk, err)
	}
	for _, value := range []rune{'😀', '✅', '1', '©', '✈', '\u2b24', 'A', '\ufffd'} {
		got, err := source.SupportsRenderableRune(value)
		if err != nil {
			t.Fatalf("source coverage for %U: %v", value, err)
		}
		want, err := second.SupportsRenderableRune(value)
		if err != nil {
			t.Fatalf("cloned coverage for %U: %v", value, err)
		}
		if got != want {
			t.Fatalf("source coverage for %U = %v, cloned face = %v", value, got, want)
		}
	}
	isolated, err := ParseFace(data, 0)
	if err != nil || isolated.Shaping.Font == first.Shaping.Font {
		t.Fatalf("generic parsed face is not isolated from registered source: %p, %v", isolated, err)
	}
	isolated.Shaping.Font.COLR = nil
	isolated.Shaping.Font.CPAL = nil
	var third ParsedFace
	err = source.CloneReadOnlyInto(&third)
	if err != nil || third.Shaping.COLR == nil || len(third.Shaping.CPAL) == 0 || !third.IsBundledNotoColorEmoji() {
		t.Fatalf("clone source was changed through a prior clone: %#v, %v", &third, err)
	}
	third.Outline = nil
	third.Shaping = nil
	if got, err := source.Outline(); err != nil || got != outline {
		t.Fatalf("source was poisoned through clone fields: %p, %v", got, err)
	}
	if boundsAfter, hasInk, err := source.GlyphRenderBounds(uint32(emojiGlyph), fixed.I(64)); err != nil || !hasInk || boundsAfter != bounds {
		t.Fatalf("source bounds after clone mutation = %v/%v, %v; want %v", boundsAfter, hasInk, err, bounds)
	}

	if source, matched, err := RegisteredBundledNotoColorEmoji(data, 1); source != nil || !matched || err == nil || !strings.Contains(err.Error(), "collection has 1 faces") {
		t.Fatalf("face 1 source/match/error = %p/%v/%v", source, matched, err)
	}
	mutated := append([]byte(nil), data...)
	mutated[len(mutated)-1] ^= 0xff
	if source, matched, err := RegisteredBundledNotoColorEmoji(mutated, 0); source != nil || matched || err != nil {
		t.Fatalf("mutated source/match/error = %p/%v/%v", source, matched, err)
	}
	mutatedFace, err := ParseFace(mutated, 0)
	if err != nil || mutatedFace.IsBundledNotoColorEmoji() {
		t.Fatalf("mutated ordinary ParseFace() = %p, %v", mutatedFace, err)
	}

	ordinaryData := testFontData(t, "SourceSansPro-Regular.ttf")
	paddedOrdinary := make([]byte, len(data))
	copy(paddedOrdinary, ordinaryData)
	if source, matched, err := RegisteredBundledNotoColorEmoji(paddedOrdinary, 0); source != nil || matched || err != nil {
		t.Fatalf("same-sized ordinary source/match/error = %p/%v/%v", source, matched, err)
	}
	ordinary, err := ParseFace(paddedOrdinary, 0)
	if err != nil || ordinary.IsBundledNotoColorEmoji() {
		t.Fatalf("same-sized ordinary ParseFace() = %p, %v", ordinary, err)
	}

	malformed := make([]byte, len(data))
	if source, matched, err := RegisteredBundledNotoColorEmoji(malformed, 0); source != nil || matched || err != nil {
		t.Fatalf("same-sized malformed source/match/error = %p/%v/%v", source, matched, err)
	}
	if face, err := ParseFace(malformed, 0); face != nil || err == nil {
		t.Fatalf("same-sized malformed ParseFace() = %p, %v", face, err)
	}
}

func TestCOLR0LayerValidation(t *testing.T) {
	layers := []tables.Layer{
		{GlyphID: 1, PaletteIndex: 0},
		{GlyphID: 2, PaletteIndex: math.MaxUint16},
	}
	palette := []tables.ColorRecord{{Red: 10, Green: 20, Blue: 30, Alpha: 40}}
	got, err := validateCOLR0Layers(layers, palette, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].GlyphID != 1 || got[0].Color != (color.NRGBA{R: 10, G: 20, B: 30, A: 40}) || got[0].Foreground {
		t.Fatalf("palette layer = %#v", got)
	}
	if got[1].GlyphID != 2 || !got[1].Foreground || got[1].Color != (color.NRGBA{}) {
		t.Fatalf("foreground layer = %#v", got[1])
	}
	if _, err := validateCOLR0Layers([]tables.Layer{{GlyphID: 3}}, palette, 3); err == nil || !strings.Contains(err.Error(), "glyph ID 3 is out of range") {
		t.Fatalf("glyph range error = %v", err)
	}
	if _, err := validateCOLR0Layers([]tables.Layer{{GlyphID: 1, PaletteIndex: 1}}, palette, 3); err == nil || !strings.Contains(err.Error(), "palette index 1 is out of range") {
		t.Fatalf("palette range error = %v", err)
	}
	if _, err := validateCOLR0Layers(nil, palette, 3); err == nil || !strings.Contains(err.Error(), "no layers") {
		t.Fatalf("empty layer error = %v", err)
	}
	if _, err := validateCOLR0Layers(make([]tables.Layer, maxCOLR0GlyphLayers+1), palette, 3); err == nil || !strings.Contains(err.Error(), "layer count") {
		t.Fatalf("layer-count error = %v", err)
	}
}

func TestBundledNotoColorEmojiLicenseAndNotice(t *testing.T) {
	notice, err := os.ReadFile("../../d2fonts/NOTICE.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Source Sans Pro, Source Code Pro, and Fuzzy Bubbles",
		"SIL Open Font License 1.1",
		"http://scripts.sil.org/OFL",
		"Noto Color Emoji version 2.051",
		"Upstream license copyright: Copyright 2013 Google LLC",
		"Font metadata copyright: Copyright 2022 Google Inc.",
		"8998f5dd683424a73e2314a8c1f1e359c19e8742",
		"0ae57fe58645638523ba35f388d93739d292539a9acb84df5700c81b1e1a28d2",
		"342beffe73f7fa450d2486d3ad62ab2308df68192718965baf23f5a85eef8247",
		"d2renderers/d2fonts/encoded/NotoColorEmoji-COLRv1-v2.051.ttf.br",
		"NotoColorEmoji-OFL.txt",
	} {
		if !bytes.Contains(notice, []byte(required)) {
			t.Fatalf("NOTICE.txt does not contain %q", required)
		}
	}
	thirdPartyNotices, err := os.ReadFile("../../../THIRD_PARTY_NOTICES.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Noto Color Emoji",
		"Upstream license copyright: Copyright 2013 Google LLC",
		"Font metadata copyright: Copyright 2022 Google Inc.",
		"8998f5dd683424a73e2314a8c1f1e359c19e8742",
		"0ae57fe58645638523ba35f388d93739d292539a9acb84df5700c81b1e1a28d2",
	} {
		if !bytes.Contains(thirdPartyNotices, []byte(required)) {
			t.Fatalf("THIRD_PARTY_NOTICES.txt does not contain %q", required)
		}
	}
	license, err := os.ReadFile("../../d2fonts/NotoColorEmoji-OFL.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprintf("%x", sha256.Sum256(license)), "79d348cb5161481f603dcda564fb129a9b2c11d7012dcad820af045fed75c020"; got != want {
		t.Fatalf("NotoColorEmoji-OFL.txt SHA-256 = %s, want pinned v2.051 LICENSE %s", got, want)
	}
	for _, required := range []string{"Copyright 2013 Google LLC", "https://scripts.sil.org/OFL", "SIL OPEN FONT LICENSE Version 1.1", "any document created using the fonts or their derivatives"} {
		if !bytes.Contains(license, []byte(required)) {
			t.Fatalf("NotoColorEmoji-OFL.txt does not contain %q", required)
		}
	}
}

func BenchmarkCloneReadOnlyBundledNotoColorEmojiRegistered(b *testing.B) {
	data := bundledNotoColorEmojiForTest(b)
	if _, err := RegisterOwnedBundledNotoColorEmoji(append([]byte(nil), data...)); err != nil {
		b.Fatal(err)
	}
	if _, matched, err := RegisteredBundledNotoColorEmoji(data, 0); err != nil || !matched {
		b.Fatalf("prime clone = matched %v, %v", matched, err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		source, matched, err := RegisteredBundledNotoColorEmoji(data, 0)
		var face *ParsedFace
		if err == nil && matched {
			face, err = source.CloneReadOnly()
		}
		if err != nil || face == nil || !face.IsBundledNotoColorEmoji() {
			b.Fatalf("RegisteredBundledNotoColorEmoji() matched %v, %v", matched, err)
		}
	}
}

func sfntTableTags(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	if len(data) < 12 {
		t.Fatal("font is too short for an sfnt header")
	}
	tableCount := int(binary.BigEndian.Uint16(data[4:6]))
	if tableCount > (len(data)-12)/16 {
		t.Fatalf("sfnt table count %d exceeds directory bounds", tableCount)
	}
	tags := make(map[string]bool, tableCount)
	for index := 0; index < tableCount; index++ {
		offset := 12 + index*16
		tags[string(data[offset:offset+4])] = true
	}
	return tags
}

func sfntTableData(t *testing.T, data []byte, wanted string) []byte {
	t.Helper()
	if len(wanted) != 4 || len(data) < 12 {
		t.Fatal("invalid sfnt table lookup")
	}
	tableCount := int(binary.BigEndian.Uint16(data[4:6]))
	if tableCount > (len(data)-12)/16 {
		t.Fatalf("sfnt table count %d exceeds directory bounds", tableCount)
	}
	for index := 0; index < tableCount; index++ {
		record := 12 + index*16
		if string(data[record:record+4]) != wanted {
			continue
		}
		offset := int(binary.BigEndian.Uint32(data[record+8 : record+12]))
		length := int(binary.BigEndian.Uint32(data[record+12 : record+16]))
		if offset < 0 || length < 0 || offset > len(data)-length {
			t.Fatalf("sfnt table %q range %d+%d exceeds %d bytes", wanted, offset, length, len(data))
		}
		return data[offset : offset+length]
	}
	t.Fatalf("sfnt table %q not found", wanted)
	return nil
}
