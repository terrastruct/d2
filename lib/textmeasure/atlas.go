package textmeasure

import (
	"sort"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"oss.terrastruct.com/d2/lib/geo"
)

// glyph describes one glyph in an atlas.
type glyph struct {
	dot     *geo.Point
	frame   *rect
	advance float64
}

// atlas is a set of pre-drawn glyphs of a fixed set of runes. This allows for efficient text drawing.
type atlas struct {
	face       font.Face
	mapping    map[rune]glyph
	ascent     float64
	descent    float64
	lineHeight float64
}

// NewAtlas creates a new atlas containing glyphs of the union of the given sets of runes (plus
// unicode.ReplacementChar) from the given font face.
//
// Creating an atlas is rather expensive, do not create a new atlas each frame.
//
// Do not destroy or close the font.Face after creating the atlas. atlas still uses it.
func NewAtlas(face font.Face, runeSets ...[]rune) *atlas {
	seen := make(map[rune]bool)
	runes := []rune{unicode.ReplacementChar}
	for _, set := range runeSets {
		for _, r := range set {
			if !seen[r] {
				runes = append(runes, r)
				seen[r] = true
			}
		}
	}

	fixedMapping, fixedBounds := makeSquareMapping(face, runes, fixed.I(2))

	bounds := &rect{
		tl: geo.NewPoint(
			i2f(fixedBounds.Min.X),
			i2f(fixedBounds.Min.Y),
		),
		br: geo.NewPoint(
			i2f(fixedBounds.Max.X),
			i2f(fixedBounds.Max.Y),
		),
	}

	mapping := make(map[rune]glyph)
	for r, fg := range fixedMapping {
		mapping[r] = glyph{
			dot: geo.NewPoint(
				i2f(fg.dot.X),
				bounds.br.Y-(i2f(fg.dot.Y)-bounds.tl.Y),
			),
			frame: rect{
				tl: geo.NewPoint(
					i2f(fg.frame.Min.X),
					bounds.br.Y-(i2f(fg.frame.Min.Y)-bounds.tl.Y),
				),
				br: geo.NewPoint(
					i2f(fg.frame.Max.X),
					bounds.br.Y-(i2f(fg.frame.Max.Y)-bounds.tl.Y),
				),
			}.norm(),
			advance: i2f(fg.advance),
		}
	}

	return &atlas{
		face:       face,
		mapping:    mapping,
		ascent:     i2f(face.Metrics().Ascent),
		descent:    i2f(face.Metrics().Descent),
		lineHeight: i2f(face.Metrics().Height),
	}
}

func (a *atlas) contains(r rune) bool {
	_, ok := a.mapping[r]
	return ok
}

// glyph returns the description of r within the atlas.
func (a *atlas) glyph(r rune) glyph {
	return a.mapping[r]
}

// Kern returns the kerning distance between runes r0 and r1. Positive distance means that the
// glyphs should be further apart.
func (a *atlas) Kern(r0, r1 rune) float64 {
	return i2f(a.face.Kern(r0, r1))
}

// Ascent returns the distance from the top of the line to the baseline.
func (a *atlas) Ascent() float64 {
	return a.ascent
}

// Descent returns the distance from the baseline to the bottom of the line.
func (a *atlas) Descent() float64 {
	return a.descent
}

// DrawRune returns parameters necessary for drawing a rune glyph.
//
// Rect is a rectangle where the glyph should be positioned. frame is the glyph frame inside the
// atlas's Picture. NewDot is the new position of the dot.
func (a *atlas) DrawRune(prevR, r rune, dot *geo.Point) (rect2, frame, bounds *rect, newDot *geo.Point) {
	if !a.contains(r) {
		r = unicode.ReplacementChar
	}
	if !a.contains(unicode.ReplacementChar) {
		return newRect(), newRect(), newRect(), dot
	}
	if !a.contains(prevR) {
		prevR = unicode.ReplacementChar
	}

	if prevR >= 0 {
		dot.X += a.Kern(prevR, r)
	}

	glyph := a.glyph(r)

	subbed := geo.NewPoint(
		dot.X-glyph.dot.X,
		dot.Y-glyph.dot.Y,
	)

	rect2 = &rect{
		tl: geo.NewPoint(
			glyph.frame.tl.X+subbed.X,
			glyph.frame.tl.Y+subbed.Y,
		),
		br: geo.NewPoint(
			glyph.frame.br.X+subbed.X,
			glyph.frame.br.Y+subbed.Y,
		),
	}
	bounds = rect2

	if bounds.w()*bounds.h() != 0 {
		bounds = &rect{
			tl: geo.NewPoint(
				bounds.tl.X,
				dot.Y-a.Descent(),
			),
			br: geo.NewPoint(
				bounds.br.X,
				dot.Y+a.Ascent(),
			),
		}
	}

	dot.X += glyph.advance

	return rect2, glyph.frame, bounds, dot
}

type fixedGlyph struct {
	dot     fixed.Point26_6
	frame   fixed.Rectangle26_6
	advance fixed.Int26_6
}

// makeSquareMapping finds an optimal glyph arrangement of the given runes, so that their common
// bounding box is as square as possible.
func makeSquareMapping(face font.Face, runes []rune, padding fixed.Int26_6) (map[rune]fixedGlyph, fixed.Rectangle26_6) {
	width := sort.Search(int(fixed.I(1024*1024)), func(i int) bool {
		width := fixed.Int26_6(i)
		_, bounds := makeMapping(face, runes, padding, width)
		return bounds.Max.X-bounds.Min.X >= bounds.Max.Y-bounds.Min.Y
	})
	return makeMapping(face, runes, padding, fixed.Int26_6(width))
}

// makeMapping arranges glyphs of the given runes into rows in such a way, that no glyph is located
// fully to the right of the specified width. Specifically, it places glyphs in a row one by one and
// once it reaches the specified width, it starts a new row.
func makeMapping(face font.Face, runes []rune, padding, width fixed.Int26_6) (map[rune]fixedGlyph, fixed.Rectangle26_6) {
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

func i2f(i fixed.Int26_6) float64 {
	return float64(i) / (1 << 6)
}
