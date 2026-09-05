package textmeasure

import (
	"sort"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type atlasGlyphMetrics struct {
	rune    rune
	frame   fixed.Rectangle26_6
	advance fixed.Int26_6
}

// Ruler faces have immutable metrics. Measure each glyph once, then search for
// the packing width using bounds alone. Only the chosen width needs a mapping.
// Arbitrary font.Face implementations retain makeSquareMapping's original
// method-call sequence, including repeated metric queries during the search.
func makeMetricsSquareMapping(face font.Face, runes []rune, padding fixed.Int26_6) (map[rune]fixedGlyph, fixed.Rectangle26_6) {
	glyphs := make([]atlasGlyphMetrics, 0, len(runes))
	for _, r := range runes {
		bounds, advance, ok := face.GlyphBounds(r)
		if !ok {
			continue
		}
		glyphs = append(glyphs, atlasGlyphMetrics{
			rune: r,
			frame: fixed.Rectangle26_6{
				Min: fixed.P(bounds.Min.X.Floor(), bounds.Min.Y.Floor()),
				Max: fixed.P(bounds.Max.X.Ceil(), bounds.Max.Y.Ceil()),
			},
			advance: advance,
		})
	}
	metrics := face.Metrics()
	lineHeight := metrics.Ascent + metrics.Descent
	width := sort.Search(int(fixed.I(1024*1024)), func(i int) bool {
		bounds := arrangeAtlasGlyphs(glyphs, padding, fixed.Int26_6(i), lineHeight, nil)
		return bounds.Max.X-bounds.Min.X >= bounds.Max.Y-bounds.Min.Y
	})
	mapping := make(map[rune]fixedGlyph, len(glyphs))
	bounds := arrangeAtlasGlyphs(glyphs, padding, fixed.Int26_6(width), lineHeight, mapping)
	return mapping, bounds
}

func arrangeAtlasGlyphs(glyphs []atlasGlyphMetrics, padding, width, lineHeight fixed.Int26_6, mapping map[rune]fixedGlyph) fixed.Rectangle26_6 {
	bounds := fixed.Rectangle26_6{}
	dot := fixed.P(0, 0)
	for _, glyph := range glyphs {
		frame := glyph.frame
		dot.X -= frame.Min.X
		frame = frame.Add(dot)
		if mapping != nil {
			mapping[glyph.rune] = fixedGlyph{dot: dot, frame: frame, advance: glyph.advance}
		}
		bounds = bounds.Union(frame)
		dot.X = frame.Max.X
		dot.X += padding
		dot.X = fixed.I(dot.X.Ceil())
		if frame.Max.X >= width {
			dot.X = 0
			dot.Y += lineHeight
			dot.Y += padding
			dot.Y = fixed.I(dot.Y.Ceil())
		}
	}
	return bounds
}
