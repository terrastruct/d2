package fontface

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/image/math/fixed"
)

func TestShapeTextKeepsGraphemeOnFallbackFace(t *testing.T) {
	primary := shapingTestFace(t, "FuzzyBubbles-Regular.ttf")
	fallback := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	const base = '\u0416'
	const mark = '\u0301'
	if supported, err := primary.SupportsRenderableRune(base); err != nil || supported {
		t.Fatalf("primary base coverage = %v/%v, want false/nil", supported, err)
	}
	if supported, err := primary.SupportsRenderableRune(mark); err != nil || !supported {
		t.Fatalf("primary mark coverage = %v/%v, want true/nil", supported, err)
	}
	for _, value := range []rune{base, mark} {
		if supported, err := fallback.SupportsRenderableRune(value); err != nil || !supported {
			t.Fatalf("fallback coverage for %U = %v/%v, want true/nil", value, supported, err)
		}
	}

	// The same acute mark belongs to a primary cluster first and a fallback
	// cluster second. Face selection must therefore be occurrence-specific,
	// rather than a map keyed only by rune value.
	shaped, err := ShapeText(context.Background(), "e\u0301 \u0416\u0301", fixed.I(20), []ShapeFace{
		{ID: "primary", Face: primary},
		{ID: "fallback", Face: fallback},
	}, shapingTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	primaryCluster := false
	fallbackCluster := false
	for _, glyph := range shaped.Glyphs {
		switch {
		case glyph.SourceIndex < 2:
			primaryCluster = true
			if glyph.Face != 0 {
				t.Fatalf("primary grapheme glyph uses face %d: %#v", glyph.Face, glyph)
			}
		case glyph.SourceIndex >= 3:
			fallbackCluster = true
			if glyph.Face != 1 {
				t.Fatalf("fallback grapheme glyph uses face %d: %#v", glyph.Face, glyph)
			}
		}
	}
	if !primaryCluster || !fallbackCluster {
		t.Fatalf("shaped glyphs do not contain both grapheme clusters: %#v", shaped.Glyphs)
	}
}

func TestShapeTextIsBoundedAndCancellable(t *testing.T) {
	face := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	faces := []ShapeFace{{ID: "primary", Face: face}}
	limits := shapingTestLimits()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ShapeText(cancelled, "text", fixed.I(12), faces, limits); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ShapeText error = %v", err)
	}

	tooShort := limits
	tooShort.Runes = 1
	if _, err := ShapeText(context.Background(), "ab", fixed.I(12), faces, tooShort); err == nil || !strings.Contains(err.Error(), "rune count") {
		t.Fatalf("rune-limit error = %v", err)
	}

	tooFewGlyphs := limits
	tooFewGlyphs.Glyphs = 1
	if _, err := ShapeText(context.Background(), "ab", fixed.I(12), faces, tooFewGlyphs); err == nil || !strings.Contains(err.Error(), "glyph count") {
		t.Fatalf("glyph-limit error = %v", err)
	}
}

func TestShapeTextUsesDeterministicPlaceholderForMissingScalar(t *testing.T) {
	face := shapingTestFace(t, "SourceSansPro-Regular.ttf")
	var replacement uint32
	var replacementRune rune
	for _, value := range missingGlyphPlaceholderRunes {
		glyph, ok := face.Shaping.NominalGlyph(value)
		if ok && glyph != 0 {
			replacement = uint32(glyph)
			replacementRune = value
			break
		}
	}
	if replacement == 0 {
		t.Fatal("Source Sans Pro has no deterministic placeholder glyph")
	}
	if replacementRune == '?' {
		t.Fatal("Source Sans Pro placeholder unexpectedly fell back to a question mark")
	}
	shaped, err := ShapeText(
		context.Background(), "\U0010ffff", fixed.I(18),
		[]ShapeFace{{ID: "primary", Face: face}}, shapingTestLimits(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(shaped.Glyphs) != 1 {
		t.Fatalf("missing-scalar glyphs = %#v, want one placeholder", shaped.Glyphs)
	}
	glyph := shaped.Glyphs[0]
	if glyph.ID != replacement || glyph.ID == 0 || !glyph.HasInk || glyph.Advance <= 0 || glyph.Source != '\U0010ffff' || glyph.SourceIndex != 0 {
		t.Fatalf("missing-scalar placeholder = %#v, want drawable %U glyph %d", glyph, replacementRune, replacement)
	}
}

func shapingTestFace(t *testing.T, filename string) *ParsedFace {
	t.Helper()
	data := testFontData(t, filename)
	face, err := ParseFace(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

func shapingTestLimits() ShapeLimits {
	return ShapeLimits{Runes: 1_000, Faces: 8, CoverageChecks: 10_000, Runs: 1_000, Glyphs: 10_000}
}
