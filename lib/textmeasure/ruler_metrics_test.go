package textmeasure

import (
	"math"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/lib/geo"
)

// Independent pre-optimization drawing oracle: retain its allocations and
// operation order to detect changes in geometry, control handling, and state.
func legacyDrawRune(a *atlas, prevR, r rune, dot *geo.Point) (rect2, frame, bounds *rect, newDot *geo.Point) {
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

func legacyDrawBuf(txt *Ruler, font d2fonts.Font) {
	if !utf8.FullRune(txt.buf) {
		return
	}

	for utf8.FullRune(txt.buf) {
		r, l := utf8.DecodeRune(txt.buf)
		txt.buf = txt.buf[l:]

		var control bool
		txt.Dot, control = txt.controlRune(r, txt.Dot, font)
		if control {
			continue
		}

		var bounds *rect
		_, _, bounds, txt.Dot = legacyDrawRune(txt.atlases[font], txt.prevR, r, txt.Dot)

		txt.prevR = r

		if txt.boundsWithDot {
			txt.bounds = txt.bounds.union(&rect{txt.Dot, txt.Dot})
			txt.bounds = txt.bounds.union(bounds)
		} else {
			if txt.bounds.w()*txt.bounds.h() == 0 {
				txt.bounds = bounds
			} else {
				txt.bounds = txt.bounds.union(bounds)
			}
		}
	}
}

func sameFloat(a, b float64) bool    { return math.Float64bits(a) == math.Float64bits(b) }
func samePoint(a, b *geo.Point) bool { return sameFloat(a.X, b.X) && sameFloat(a.Y, b.Y) }
func sameRect(a, b *rect) bool       { return samePoint(a.tl, b.tl) && samePoint(a.br, b.br) }

func TestRulerMetricsMatchLegacy(t *testing.T) {
	r, err := NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	for spec := range r.ttfs {
		for _, size := range []int{0, 1, 16, 72, 4097} {
			spec.Size = size
			r.addFontSize(spec)
			for _, origin := range []geo.Point{{}, {X: .1, Y: -.3}, {X: -.5, Y: 13.25}, {X: 1 << 50, Y: -(1 << 50)}, {X: math.Copysign(0, -1), Y: math.Copysign(0, -1)}} {
				for _, lineHeight := range []float64{1, 1.3, math.Sqrt2} {
					for _, withDot := range []bool{false, true} {
						r.Orig = origin.Copy()
						r.LineHeightFactor = lineHeight
						r.boundsWithDot = withDot
						old := *r
						old.buf = append([]byte(nil), r.buf...)
						for _, text := range []string{"", "AVATAR To. Wjgy", " \t  ", "first\nsecond\n\nthird", "reset\rtext\tend", "aé界🙂e\u0301", string([]byte{0xff, 'A', 0xc3})} {
							gotW, gotH := r.MeasurePrecise(spec, text)
							old.clear()
							old.buf = append(old.buf, text...)
							legacyDrawBuf(&old, spec)
							if !sameFloat(gotW, old.bounds.w()) || !sameFloat(gotH, old.bounds.h()) || !samePoint(r.Dot, old.Dot) || !sameRect(r.bounds, old.bounds) || r.prevR != old.prevR || string(r.buf) != string(old.buf) {
								t.Fatalf("font=%v origin=%v lineHeight=%v withDot=%v text=%q got=(%v,%v,%v) want=(%v,%v,%v)", spec, origin, lineHeight, withDot, text, gotW, gotH, r.Dot, old.bounds.w(), old.bounds.h(), old.Dot)
							}
						}
					}
				}
			}
		}
	}
}

func TestDrawRuneMetricsMatchLegacy(t *testing.T) {
	parsed, err := parseFont(d2fonts.FontFaces.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)))
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []float64{0, 16.25, 65536} {
		a := NewAtlas(parsed.newFace(size), Runes)
		for _, dot := range []geo.Point{{}, {X: .1, Y: -.3}, {X: 1 << 53, Y: -(1 << 53)}, {X: math.Inf(1), Y: math.Inf(-1)}, {X: math.NaN(), Y: math.NaN()}} {
			for _, r := range []rune{'A', ' ', 'g', '界', unicode.ReplacementChar} {
				for _, previous := range []rune{-1, 'A', '界'} {
					actualDot, wantDot := dot.Copy(), dot.Copy()
					gotPosition, gotFrame, gotBounds, gotDot := a.DrawRune(previous, r, actualDot)
					wantPosition, wantFrame, wantBounds, wantDot := legacyDrawRune(a, previous, r, wantDot)
					if !sameRect(gotPosition, wantPosition) || !sameRect(gotFrame, wantFrame) || !sameRect(gotBounds, wantBounds) || !samePoint(gotDot, wantDot) {
						t.Fatalf("size=%v dot=%v previous=%q rune=%q", size, dot, previous, r)
					}
					if (gotPosition == gotBounds) != (wantPosition == wantBounds) || gotDot != actualDot {
						t.Fatal("DrawRune aliasing changed")
					}
				}
			}
		}
	}
	a := &atlas{mapping: map[rune]glyph{}}
	dot := geo.NewPoint(1, 2)
	_, _, bounds, returned := a.DrawRune('A', 'B', dot)
	if !sameRect(bounds, newRect()) || returned != dot || *dot != (geo.Point{X: 1, Y: 2}) {
		t.Fatal("missing replacement glyph changed")
	}
}
