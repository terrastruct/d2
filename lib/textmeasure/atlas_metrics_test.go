package textmeasure

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2fonts"
)

// These two functions retain the pre-optimization packing algorithm as an
// independent oracle, including all rounding, wrapping and duplicate handling.
func legacySquareMapping(face font.Face, runes []rune, padding fixed.Int26_6) (map[rune]fixedGlyph, fixed.Rectangle26_6) {
	width := sort.Search(int(fixed.I(1024*1024)), func(i int) bool {
		width := fixed.Int26_6(i)
		_, bounds := legacyAtlasMapping(face, runes, padding, width)
		return bounds.Max.X-bounds.Min.X >= bounds.Max.Y-bounds.Min.Y
	})
	return legacyAtlasMapping(face, runes, padding, fixed.Int26_6(width))
}

// makeMapping arranges glyphs of the given runes into rows in such a way, that no glyph is located
// fully to the right of the specified width. Specifically, it places glyphs in a row one by one and
// once it reaches the specified width, it starts a new row.
func legacyAtlasMapping(face font.Face, runes []rune, padding, width fixed.Int26_6) (map[rune]fixedGlyph, fixed.Rectangle26_6) {
	mapping := make(map[rune]fixedGlyph)
	bounds := fixed.Rectangle26_6{}

	dot := fixed.P(0, 0)

	for _, r := range runes {
		b, advance, ok := face.GlyphBounds(r)
		if !ok {
			continue
		}

		// this is important for drawing, artifacts arise otherwise
		frame := fixed.Rectangle26_6{
			Min: fixed.P(b.Min.X.Floor(), b.Min.Y.Floor()),
			Max: fixed.P(b.Max.X.Ceil(), b.Max.Y.Ceil()),
		}

		dot.X -= frame.Min.X
		frame = frame.Add(dot)

		mapping[r] = fixedGlyph{
			dot:     dot,
			frame:   frame,
			advance: advance,
		}
		bounds = bounds.Union(frame)

		dot.X = frame.Max.X

		// padding + align to integer
		dot.X += padding
		dot.X = fixed.I(dot.X.Ceil())

		// width exceeded, new row
		if frame.Max.X >= width {
			dot.X = 0
			dot.Y += face.Metrics().Ascent + face.Metrics().Descent

			// padding + align to integer
			dot.Y += padding
			dot.Y = fixed.I(dot.Y.Ceil())
		}
	}

	return mapping, bounds
}

func TestAtlasMetricsMatchesLegacy(t *testing.T) {
	runeSets := [][]rune{
		{}, {unicode.ReplacementChar}, {'A', 'g', ' ', '\t', 'A', unicode.ReplacementChar, unicode.ReplacementChar, '界', '🙂'},
		append([]rune{unicode.ReplacementChar}, Runes...),
	}
	for _, family := range d2fonts.FontFamilies {
		for _, style := range d2fonts.FontStyles {
			data, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: family, Style: style})
			if !ok {
				continue
			}
			parsed, err := parseFont(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, size := range []float64{0, 1, 13, 16.25, 72, 256} {
				face := parsed.newFace(size)
				for set, runes := range runeSets {
					for _, padding := range []fixed.Int26_6{0, 1, fixed.I(2)} {
						expected, expectedBounds := legacySquareMapping(face, runes, padding)
						actual, actualBounds := makeSquareMapping(face, runes, padding)
						if expectedBounds != actualBounds || !reflect.DeepEqual(expected, actual) {
							t.Fatalf("family=%s style=%s size=%v set=%d padding=%v bounds=%v/%v", family, style, size, set, padding, expectedBounds, actualBounds)
						}
					}
				}
			}
		}
	}
}

type atlasCountingFace struct {
	font.Face
	boundsCalls, metricsCalls int
}

func (face *atlasCountingFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	face.boundsCalls++
	return face.Face.GlyphBounds(r)
}
func (face *atlasCountingFace) Metrics() font.Metrics {
	face.metricsCalls++
	return face.Face.Metrics()
}

func TestAtlasMetricsQueriesEachGlyphOnce(t *testing.T) {
	parsed, err := parseFont(d2fonts.FontFaces.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)))
	if err != nil {
		t.Fatal(err)
	}
	face := &atlasCountingFace{Face: parsed.newFace(16)}
	_, _ = makeMetricsSquareMapping(face, Runes, fixed.I(2))
	if face.boundsCalls != len(Runes) || face.metricsCalls != 1 {
		t.Fatalf("bounds calls=%d, metrics calls=%d", face.boundsCalls, face.metricsCalls)
	}
	// Custom public faces retain the original observation/call sequence.
	oldFace := &atlasCountingFace{Face: parsed.newFace(16)}
	newFace := &atlasCountingFace{Face: parsed.newFace(16)}
	oldMapping, oldBounds := legacySquareMapping(oldFace, Runes, fixed.I(2))
	newMapping, newBounds := makeSquareMapping(newFace, Runes, fixed.I(2))
	if oldFace.boundsCalls != newFace.boundsCalls || oldFace.metricsCalls != newFace.metricsCalls || oldBounds != newBounds || !reflect.DeepEqual(oldMapping, newMapping) {
		t.Fatal("custom font.Face behavior changed")
	}
}

func TestAtlasMetricsPreservesFixedArithmetic(t *testing.T) {
	parsed, err := parseFont(d2fonts.FontFaces.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)))
	if err != nil {
		t.Fatal(err)
	}
	face := parsed.newFace(16)
	runes := []rune{'A', 'g', ' ', 'A', unicode.ReplacementChar}
	for _, padding := range []fixed.Int26_6{-fixed.I(2), math.MaxInt32, math.MinInt32} {
		oldMapping, oldBounds := legacySquareMapping(face, runes, padding)
		newMapping, newBounds := makeSquareMapping(face, runes, padding)
		if oldBounds != newBounds || !reflect.DeepEqual(oldMapping, newMapping) {
			t.Fatalf("padding=%v fixed arithmetic changed", padding)
		}
	}
}

func BenchmarkAtlasMetrics(b *testing.B) {
	parsed, err := parseFont(d2fonts.FontFaces.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)))
	if err != nil {
		b.Fatal(err)
	}
	for _, legacy := range []bool{true, false} {
		b.Run(fmt.Sprintf("legacy=%v", legacy), func(b *testing.B) {
			face := parsed.newFace(16)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if legacy {
					_, _ = legacySquareMapping(face, Runes, fixed.I(2))
				} else {
					_, _ = makeSquareMapping(face, Runes, fixed.I(2))
				}
			}
		})
	}
}
