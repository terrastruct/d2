// Ported from https://github.com/faiface/pixel/tree/master/text
// Trimmed down to essentials of measuring text

package textmeasure

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/golang/freetype/truetype"
	"github.com/rivo/uniseg"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/lib/geo"
)

const TAB_SIZE = 4
const SIZELESS_FONT_SIZE = 0
const CODE_LINE_HEIGHT = 1.3

// Runes encompasses ASCII, Latin-1, and geometric shapes like black square
var Runes []rune

func init() {
	// ASCII range (U+0000 to U+007F)
	for r := rune(0x0000); r <= rune(0x007F); r++ {
		Runes = append(Runes, r)
	}

	// Latin-1 Supplement (U+0080 to U+00FF)
	for r := rune(0x0080); r <= rune(0x00FF); r++ {
		Runes = append(Runes, r)
	}

	// Geometric Shapes (U+25A0 to U+25FF)
	for r := rune(0x25A0); r <= rune(0x25FF); r++ {
		Runes = append(Runes, r)
	}
}

// Ruler allows for effiecient and convenient text drawing.
//
// To create a Ruler object, use the New constructor:
//
//	txt := text.New(pixel.ZV, text.NewAtlas(face, text.ASCII))
//
// As suggested by the constructor, a Ruler object is always associated with one font face and a
// fixed set of runes. For example, the Ruler we created above can draw text using the font face
// contained in the face variable and is capable of drawing ASCII characters.
//
// Here we create a Ruler object which can draw ASCII and Katakana characters:
//
//	txt := text.New(0, text.NewAtlas(face, text.ASCII, text.RangeTable(unicode.Katakana)))
//
// Similarly to IMDraw, Ruler functions as a buffer. It implements io.Writer interface, so writing
// text to it is really simple:
//
//	fmt.Print(txt, "Hello, world!")
//
// Newlines, tabs and carriage returns are supported.
//
// Finally, if we want the written text to show up on some other Target, we can draw it:
//
//	txt.Draw(target)
//
// Ruler exports two important fields: Orig and Dot. Dot is the position where the next character
// will be written. Dot is automatically moved when writing to a Ruler object, but you can also
// manipulate it manually. Orig specifies the text origin, usually the top-left dot position. Dot is
// always aligned to Orig when writing newlines. The Clear method resets the Dot to Orig.
type Ruler struct {
	// Orig specifies the text origin, usually the top-left dot position. Dot is always aligned
	// to Orig when writing newlines.
	Orig *geo.Point

	// Dot is the position where the next character will be written. Dot is automatically moved
	// when writing to a Ruler object, but you can also manipulate it manually
	Dot *geo.Point

	// lineHeight is the vertical distance between two lines of text.
	//
	// Example:
	//   txt.lineHeight = 1.5 * txt.atlas.lineHeight
	LineHeightFactor float64
	lineHeights      map[d2fonts.Font]float64

	// tabWidth is the horizontal tab width. Tab characters will align to the multiples of this
	// width.
	//
	// Example:
	//   txt.tabWidth = 8 * txt.atlas.glyph(' ').Advance
	tabWidths map[d2fonts.Font]float64

	atlases map[d2fonts.Font]*atlas

	ttfs map[d2fonts.Font]*truetype.Font

	buf    []byte
	prevR  rune
	bounds *rect

	// when drawing text also union Ruler.bounds with Dot
	boundsWithDot bool
	
	// isASCII indicates this ruler should use 1x1 measurements for ASCII rendering
	isASCII bool
}

// New creates a new Ruler capable of drawing runes contained in the provided atlas. Orig and Dot
// will be initially set to orig.
//
// Here we create a Ruler capable of drawing ASCII characters using the Go Regular font.
//
//	ttf, err := truetype.Parse(goregular.TTF)
//	if err != nil {
//	    panic(err)
//	}
//	face := truetype.NewFace(ttf, &truetype.Options{
//	    Size: 14,
//	})
//	txt := text.New(orig, text.NewAtlas(face, text.ASCII))
func NewRuler() (*Ruler, error) {
	origin := geo.NewPoint(0, 0)
	r := &Ruler{
		Orig:             origin,
		Dot:              origin.Copy(),
		LineHeightFactor: 1.,
		lineHeights:      make(map[d2fonts.Font]float64),
		tabWidths:        make(map[d2fonts.Font]float64),
		atlases:          make(map[d2fonts.Font]*atlas),
		ttfs:             make(map[d2fonts.Font]*truetype.Font),
	}

	for _, fontFamily := range d2fonts.FontFamilies {
		for _, fontStyle := range d2fonts.FontStyles {
			font := d2fonts.Font{
				Family: fontFamily,
				Style:  fontStyle,
			}
			// Note: FontFaces lookup is size-agnostic
			face, has := d2fonts.FontFaces.Lookup(font)
			if !has {
				continue
			}
			if _, loaded := r.ttfs[font]; !loaded {
				ttf, err := truetype.Parse(face)
				if err != nil {
					return nil, err
				}
				r.ttfs[font] = ttf
			}
		}
	}

	r.clear()

	return r, nil
}

func (r *Ruler) HasFontFamilyLoaded(fontFamily *d2fonts.FontFamily) bool {
	if r.isASCII {
		return true
	}
	for _, fontStyle := range d2fonts.FontStyles {
		font := d2fonts.Font{
			Family: *fontFamily,
			Style:  fontStyle,
			Size:   SIZELESS_FONT_SIZE,
		}
		_, ok := r.ttfs[font]
		if !ok {
			return false
		}
	}

	return true
}

func (r *Ruler) addFontSize(font d2fonts.Font) {
	sizeless := font
	sizeless.Size = SIZELESS_FONT_SIZE
	face := truetype.NewFace(r.ttfs[sizeless], &truetype.Options{
		Size: float64(font.Size),
	})
	atlas := NewAtlas(face, Runes)
	r.atlases[font] = atlas
	r.lineHeights[font] = atlas.lineHeight
	r.tabWidths[font] = atlas.glyph(' ').advance * TAB_SIZE
}

func (t *Ruler) scaleUnicode(w float64, font d2fonts.Font, s string) float64 {
	// Weird unicode stuff is going on when this is true
	// See https://github.com/rivo/uniseg#grapheme-clusters
	// This method is a good-enough approximation. It overshoots, but not by much.
	// I suspect we need to import a font with the right glyphs to get the precise measurements
	// but Hans fonts are heavy.
	if uniseg.GraphemeClusterCount(s) != len(s) {
		for _, line := range strings.Split(s, "\n") {
			lineW, _ := t.MeasurePrecise(font, line)
			gr := uniseg.NewGraphemes(line)

			mono := d2fonts.SourceCodePro.Font(font.Size, font.Style)
			for gr.Next() {
				if gr.Width() == 1 {
					continue
				}
				// For each grapheme which doesn't have width=1, the ruler measured wrongly.
				// So, replace the measured width with a scaled measurement of a monospace version
				var prevRune rune
				dot := t.Orig.Copy()
				b := newRect()
				for _, r := range gr.Runes() {
					var control bool
					dot, control = t.controlRune(r, dot, font)
					if control {
						continue
					}

					var bounds *rect
					_, _, bounds, dot = t.atlases[font].DrawRune(prevRune, r, dot)
					b = b.union(bounds)

					prevRune = r
				}
				lineW -= b.w()
				lineW += t.spaceWidth(mono) * float64(gr.Width())
			}
			w = math.Max(w, lineW)
		}
	}
	return w
}

func (t *Ruler) MeasureMono(font d2fonts.Font, s string) (width, height int) {
	if t.isASCII {
		return t.measureASCII(s)
	}
	originalBoundsWithDot := t.boundsWithDot
	t.boundsWithDot = true
	width, height = t.Measure(font, s)
	t.boundsWithDot = originalBoundsWithDot
	return width, height
}

func (t *Ruler) Measure(font d2fonts.Font, s string) (width, height int) {
	if t.isASCII {
		return t.measureASCII(s)
	}
	w, h := t.MeasurePrecise(font, s)
	w = t.scaleUnicode(w, font, s)
	return int(math.Ceil(w)), int(math.Ceil(h))
}

func (t *Ruler) MeasurePrecise(font d2fonts.Font, s string) (width, height float64) {
	if t.isASCII {
		w, h := t.measureASCII(s)
		return float64(w), float64(h)
	}
	if _, ok := t.atlases[font]; !ok {
		t.addFontSize(font)
	}
	t.clear()
	t.buf = append(t.buf, s...)
	t.drawBuf(font)
	b := t.bounds
	return b.w(), b.h()
}

// clear removes all written text from the Ruler. The Dot field is reset to Orig.
func (txt *Ruler) clear() {
	txt.prevR = -1
	txt.bounds = newRect()
	txt.Dot = txt.Orig.Copy()
}

// controlRune checks if r is a control rune (newline, tab, ...). If it is, a new dot position and
// true is returned. If r is not a control rune, the original dot and false is returned.
func (txt *Ruler) controlRune(r rune, dot *geo.Point, font d2fonts.Font) (newDot *geo.Point, control bool) {
	switch r {
	case '\n':
		dot.X = txt.Orig.X
		dot.Y -= txt.LineHeightFactor * txt.lineHeights[font]
	case '\r':
		dot.X = txt.Orig.X
	case '\t':
		rem := math.Mod(dot.X-txt.Orig.X, txt.tabWidths[font])
		rem = math.Mod(rem, rem+txt.tabWidths[font])
		if rem == 0 {
			rem = txt.tabWidths[font]
		}
		dot.X += rem
	default:
		return dot, false
	}
	return dot, true
}

func (txt *Ruler) drawBuf(font d2fonts.Font) {
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
		_, _, bounds, txt.Dot = txt.atlases[font].DrawRune(txt.prevR, r, txt.Dot)

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

func (ruler *Ruler) spaceWidth(font d2fonts.Font) float64 {
	if _, has := ruler.atlases[font]; !has {
		ruler.addFontSize(font)
	}
	spaceRune, _ := utf8.DecodeRuneInString(" ")
	return ruler.atlases[font].glyph(spaceRune).advance
}

// NewASCIIRuler creates a fake ruler for ASCII rendering that measures each character as 1x1
func NewASCIIRuler() (*Ruler, error) {
	origin := geo.NewPoint(0, 0)
	return &Ruler{
		Orig:             origin,
		Dot:              origin.Copy(),
		LineHeightFactor: 1.,
		lineHeights:      make(map[d2fonts.Font]float64),
		tabWidths:        make(map[d2fonts.Font]float64),
		atlases:          make(map[d2fonts.Font]*atlas),
		ttfs:             make(map[d2fonts.Font]*truetype.Font),
		isASCII:          true,
	}, nil
}

// measureASCII returns 1x1 measurement for each character in ASCII rendering
func (r *Ruler) measureASCII(s string) (width, height int) {
	if s == "" {
		return 0, 0
	}
	
	lines := strings.Split(s, "\n")
	maxWidth := 0
	for _, line := range lines {
		lineWidth := 0
		for _, ch := range line {
			if ch == '\t' {
				lineWidth += TAB_SIZE
			} else {
				lineWidth++
			}
		}
		if lineWidth > maxWidth {
			maxWidth = lineWidth
		}
	}
	
	return maxWidth, len(lines)
}

// IsASCII returns true if this ruler is in ASCII mode
func (r *Ruler) IsASCII() bool {
	return r.isASCII
}
