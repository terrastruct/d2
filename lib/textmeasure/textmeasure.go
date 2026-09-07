// Ported from https://github.com/faiface/pixel/tree/master/text
// Trimmed down to essentials of measuring text

package textmeasure

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/lib/geo"
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

	ttfs  map[d2fonts.Font]*parsedFont
	faces map[d2fonts.Font]font.Face

	buf    []byte
	prevR  rune
	bounds *rect

	// when drawing text also union Ruler.bounds with Dot
	boundsWithDot bool
}

// New creates a new Ruler capable of drawing runes contained in the provided atlas. Orig and Dot
// will be initially set to orig.
//
// Here we create a Ruler capable of drawing ASCII characters using the Go Regular font.
//
//	ttf, err := parseFont(goregular.TTF)
//	if err != nil {
//	    panic(err)
//	}
//	face := ttf.newFace(14)
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
		ttfs:             make(map[d2fonts.Font]*parsedFont),
		faces:            make(map[d2fonts.Font]font.Face),
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
				ttf, err := parseFont(face)
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
	face := r.fontFace(font, r.ttfs[sizeless])
	atlas := NewAtlas(face, Runes)
	r.atlases[font] = atlas
	r.lineHeights[font] = atlas.lineHeight
	r.tabWidths[font] = atlas.glyph(' ').advance * TAB_SIZE
}

// fontFace retains the per-size scratch buffers used by immutable font metrics.
// A Ruler already owns mutable drawing state and must not be used concurrently.
func (r *Ruler) fontFace(spec d2fonts.Font, parsed *parsedFont) font.Face {
	if face := r.faces[spec]; face != nil {
		return face
	}
	face := parsed.newFace(float64(spec.Size))
	r.faces[spec] = face
	return face
}

func (t *Ruler) measureFontWidth(fontSpec d2fonts.Font, s string) (float64, bool) {
	sizelessFont := fontSpec
	sizelessFont.Size = SIZELESS_FONT_SIZE
	ttf, ok := t.ttfs[sizelessFont]
	if !ok {
		return 0, false
	}

	var buffer sfnt.Buffer
	for _, r := range s {
		index, err := ttf.font.GlyphIndex(&buffer, r)
		if err != nil || (index == 0 && r != 0) {
			return 0, false
		}
	}

	face := t.fontFace(fontSpec, ttf)
	bounds, advance := font.BoundString(face, s)
	left := min(bounds.Min.X, 0)
	right := max(bounds.Max.X, advance)
	return float64(right-left) / 64, true
}

// measureFontAdvance returns the cursor advance used by CSS/SVG text layout.
// MeasurePrecise intentionally includes ink bearings for legacy D2 box
// measurement; using that ink box to position adjacent styled Markdown runs
// adds visible gaps after bold and italic spans.
func (t *Ruler) measureFontAdvance(fontSpec d2fonts.Font, s string) (float64, bool) {
	sizelessFont := fontSpec
	sizelessFont.Size = SIZELESS_FONT_SIZE
	ttf, ok := t.ttfs[sizelessFont]
	if !ok {
		return 0, false
	}

	var buffer sfnt.Buffer
	for _, r := range s {
		index, err := ttf.font.GlyphIndex(&buffer, r)
		if err != nil || (index == 0 && r != 0) {
			return 0, false
		}
	}
	face := t.fontFace(fontSpec, ttf)
	return float64(font.MeasureString(face, s)) / 64, true
}

// cssFontBoxMetrics returns the integer CSS-pixel font box that browsers use
// to place an SVG text baseline. These are the original OpenType metrics, not
// the deliberately legacy-compatible em metrics used by D2's text measurer.
func (t *Ruler) cssFontBoxMetrics(fontSpec d2fonts.Font, size float64) (ascent, descent float64, ok bool) {
	sizelessFont := fontSpec
	sizelessFont.Size = SIZELESS_FONT_SIZE
	ttf, ok := t.ttfs[sizelessFont]
	if !ok {
		return 0, 0, false
	}

	scale := fixed.Int26_6(math.Round(size * 64))
	metrics, err := ttf.font.Metrics(nil, scale, font.HintingNone)
	if err != nil {
		return 0, 0, false
	}
	return math.Round(float64(metrics.Ascent) / 64), math.Round(float64(metrics.Descent) / 64), true
}

func (t *Ruler) cssNormalLineHeight(fontSpec d2fonts.Font, size float64) (float64, bool) {
	sizelessFont := fontSpec
	sizelessFont.Size = SIZELESS_FONT_SIZE
	ttf, ok := t.ttfs[sizelessFont]
	if !ok {
		return 0, false
	}
	scale := fixed.Int26_6(math.Round(size * 64))
	metrics, err := ttf.font.Metrics(nil, scale, font.HintingNone)
	if err != nil {
		return 0, false
	}
	return math.Round(float64(metrics.Height) / 64), true
}

func (t *Ruler) scaleUnicode(w float64, font d2fonts.Font, s string) float64 {
	// Keep D2's established approximation for ordinary labels. Native
	// Markdown has a separate CSS fallback path below; changing this function
	// would resize unrelated diagrams that happen to contain Unicode text.
	if uniseg.GraphemeClusterCount(s) != len(s) {
		for _, line := range strings.Split(s, "\n") {
			lineW, _ := t.MeasurePrecise(font, line)
			graphemes := uniseg.NewGraphemes(line)

			mono := d2fonts.SourceCodePro.Font(font.Size, font.Style)
			for graphemes.Next() {
				if graphemes.Width() == 1 {
					continue
				}
				var previous rune
				dot := t.Orig.Copy()
				bounds := newRect()
				for _, r := range graphemes.Runes() {
					var control bool
					dot, control = t.controlRune(r, dot, font)
					if control {
						continue
					}

					var glyphBounds *rect
					_, _, glyphBounds, dot = t.atlases[font].DrawRune(previous, r, dot)
					bounds = bounds.union(glyphBounds)
					previous = r
				}
				lineW -= bounds.w()
				lineW += t.spaceWidth(mono) * float64(graphemes.Width())
			}
			w = math.Max(w, lineW)
		}
	}
	return w
}

// scaleUnicodeCSS approximates the host-font fallback advances used by
// Chromium and SVG viewers when native Markdown contains glyphs absent from
// D2's embedded fonts.
func (t *Ruler) scaleUnicodeCSS(w float64, font d2fonts.Font, s string) float64 {
	return t.scaleUnicodeFallback(w, font, s, true)
}

func (t *Ruler) scaleUnicodeFallback(w float64, font d2fonts.Font, s string, cssEmoji bool) float64 {
	// Weird unicode stuff is going on when this is true
	// See https://github.com/rivo/uniseg#grapheme-clusters
	// This method is a good-enough approximation. It overshoots, but not by much.
	// I suspect we need to import a font with the right glyphs to get the precise measurements
	// but Hans fonts are heavy.
	if uniseg.GraphemeClusterCount(s) != len(s) {
		w = 0
		for _, line := range strings.Split(s, "\n") {
			lineW, _ := t.MeasurePrecise(font, line)
			// Font subsets omit layout tables such as GPOS. Use the original
			// glyph advances so supported Unicode runes are not measured as the
			// replacement glyph used by the fixed-size atlas.
			if originalWidth, ok := t.measureFontWidth(font, line); ok {
				w = math.Max(w, originalWidth)
				continue
			}
			gr := uniseg.NewGraphemes(line)
			lineW = 0
			for gr.Next() {
				grapheme := gr.Str()
				originalWidth, supported := t.measureFontWidth(font, grapheme)
				if supported {
					lineW += originalWidth
					continue
				}
				colorEmoji := cssEmoji && isColorEmojiGrapheme(grapheme)
				if cssEmoji && !colorEmoji {
					filtered := stripDefaultIgnorableRunes(grapheme)
					if filtered != grapheme {
						if filtered == "" {
							continue
						}
						if filteredWidth, ok := t.measureFontWidth(font, filtered); ok {
							lineW += filteredWidth
							continue
						}
						grapheme = filtered
					}
				}
				if isZeroWidthGrapheme(grapheme) {
					continue
				}
				// The eventual SVG viewer supplies its own fallback font. CSS emoji
				// and East Asian clusters normally occupy one em (two terminal
				// cells); narrow missing glyphs occupy half an em. Reserving 2.5
				// monospace cells made mixed-script Markdown boxes far wider than
				// Chromium's actual layout.
				cells := gr.Width()
				if cells < 1 {
					cells = 1
				}
				advance := float64(font.Size) * float64(cells) / 2
				if cssEmoji {
					if colorEmoji {
						advance = math.Max(advance, float64(font.Size)*1.25)
					} else if hasUnicodeSymbol(grapheme) {
						// Text-presentation emoji-capable symbols such as bare ⚠
						// occupy one em in Chromium's fallback font.
						advance = math.Max(advance, float64(font.Size))
					}
				}
				lineW += advance
			}
			w = math.Max(w, lineW)
		}
	}
	return w
}

func stripDefaultIgnorableRunes(s string) string {
	var out strings.Builder
	for _, r := range s {
		if !isDefaultIgnorableRune(r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func isColorEmojiGrapheme(s string) bool {
	hasEmojiVariation := false
	hasEmojiBase := false
	hasKeycap := false
	for _, r := range s {
		if r == '\ufe0f' {
			hasEmojiVariation = true
			continue
		}
		if r == '\u20e3' {
			hasKeycap = true
			continue
		}
		if unicode.IsSymbol(r) {
			hasEmojiBase = true
		}
		// Uniseg's width table incorporates Unicode Emoji_Presentation.
		// Restricting the two-cell result to Other_Symbol distinguishes default
		// emoji from wide East Asian letters. Checking each base rune also keeps
		// default emoji wide when an explicit text variation selector is present,
		// matching Chromium's host fallback behavior.
		if unicode.Is(unicode.So, r) && uniseg.StringWidth(string(r)) == 2 {
			return true
		}
	}
	if hasKeycap {
		for _, r := range s {
			if r == '#' || r == '*' || (r >= '0' && r <= '9') {
				hasEmojiBase = true
				break
			}
		}
	}
	return hasEmojiVariation && hasEmojiBase
}

func hasUnicodeSymbol(s string) bool {
	for _, r := range s {
		if unicode.IsSymbol(r) {
			return true
		}
	}
	return false
}

// scaleUnicodeLegacy preserves the approximation historically used by D2's
// Markdown box measurement while filtering controls that browsers do not
// advance. Native painting uses scaleUnicodeCSS above.
func (t *Ruler) scaleUnicodeLegacy(w float64, fontSpec d2fonts.Font, s string) float64 {
	if uniseg.GraphemeClusterCount(s) == len(s) {
		return w
	}
	// Preserve the old fallback algorithm (including its original ink width)
	// after removing controls that Blink gives no advance. Uniseg can attach
	// such a control to a visible narrow base (for example x + ZWJ), so filtering
	// only wholly invisible clusters still leaves an atlas replacement glyph.
	// Keep wide emoji/CJK clusters intact: their variation selectors, tags, and
	// joiners can determine the browser's glyph and terminal-cell width.
	measurementText, filteredIgnorables := legacyMeasurementText(s)
	if filteredIgnorables {
		w = 0
	}
	for _, line := range strings.Split(measurementText, "\n") {
		lineW, _ := t.MeasurePrecise(fontSpec, line)
		lineFloor := lineW
		graphemes := uniseg.NewGraphemes(line)
		mono := d2fonts.SourceCodePro.Font(fontSpec.Size, fontSpec.Style)
		for graphemes.Next() {
			if graphemes.Width() == 1 {
				continue
			}
			var previous rune
			dot := t.Orig.Copy()
			bounds := newRect()
			for _, r := range graphemes.Runes() {
				var control bool
				dot, control = t.controlRune(r, dot, fontSpec)
				if control {
					continue
				}
				var glyphBounds *rect
				_, _, glyphBounds, dot = t.atlases[fontSpec].DrawRune(previous, r, dot)
				bounds = bounds.union(glyphBounds)
				previous = r
			}
			lineW -= bounds.w()
			lineW += t.spaceWidth(mono) * float64(graphemes.Width())
		}
		if filteredIgnorables {
			lineW = math.Max(lineW, lineFloor)
		}
		w = math.Max(w, lineW)
	}
	return w
}

func legacyMeasurementText(s string) (string, bool) {
	var out strings.Builder
	filtered := false
	graphemes := uniseg.NewGraphemes(s)
	for graphemes.Next() {
		cluster := graphemes.Str()
		if isDefaultIgnorableGrapheme(cluster) {
			filtered = true
			continue
		}
		if graphemes.Width() != 1 {
			out.WriteString(cluster)
			continue
		}
		for _, r := range cluster {
			if isDefaultIgnorableRune(r) {
				filtered = true
				continue
			}
			out.WriteRune(r)
		}
	}
	if !filtered {
		return s, false
	}
	return out.String(), true
}

func isZeroWidthGrapheme(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || isDefaultIgnorableRune(r) {
			continue
		}
		return false
	}
	return true
}

func isDefaultIgnorableGrapheme(s string) bool {
	for _, r := range s {
		if !isDefaultIgnorableRune(r) {
			return false
		}
	}
	return s != ""
}

func isDefaultIgnorableRune(r rune) bool {
	// Blink gives most format controls and variation selectors no advance, but
	// renders several controls and Unicode "default ignorables" through fallback
	// fonts. Follow that observed behavior instead of erasing the whole derived
	// Unicode property.
	if r == '\u00ad' || // conditional; handled by line wrapping
		r == '\u180f' || // Mongolian free variation selector four has fallback ink in Blink
		r == '\u3164' || r == '\uffa0' || // visibly advancing Hangul fillers
		(r >= '\ufff9' && r <= '\ufffb') || // interlinear annotations
		(r >= '\U00013430' && r <= '\U0001343f') || // Egyptian hieroglyph format controls
		(r >= '\U0001bca0' && r <= '\U0001bca3') || // visibly advancing shorthand controls
		unicode.Is(unicode.Prepended_Concatenation_Mark, r) ||
		unicode.IsSpace(r) {
		return false
	}
	return unicode.Is(unicode.Cf, r) ||
		unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) ||
		unicode.Is(unicode.Variation_Selector, r)
}

func (t *Ruler) MeasureMono(font d2fonts.Font, s string) (width, height int) {
	originalBoundsWithDot := t.boundsWithDot
	t.boundsWithDot = true
	width, height = t.Measure(font, s)
	t.boundsWithDot = originalBoundsWithDot
	return width, height
}

func (t *Ruler) Measure(font d2fonts.Font, s string) (width, height int) {
	w, h := t.MeasurePrecise(font, s)
	w = t.scaleUnicode(w, font, s)
	return int(math.Ceil(w)), int(math.Ceil(h))
}

func (t *Ruler) MeasurePrecise(font d2fonts.Font, s string) (width, height float64) {
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

		_, bounds, _ := txt.atlases[font].measureRune(txt.prevR, r, txt.Dot)
		txt.prevR = r

		if txt.boundsWithDot {
			txt.bounds.tl.X = math.Min(txt.bounds.tl.X, txt.Dot.X)
			txt.bounds.tl.Y = math.Min(txt.bounds.tl.Y, txt.Dot.Y)
			txt.bounds.br.X = math.Max(txt.bounds.br.X, txt.Dot.X)
			txt.bounds.br.Y = math.Max(txt.bounds.br.Y, txt.Dot.Y)
		} else if txt.bounds.w()*txt.bounds.h() == 0 {
			*txt.bounds.tl, *txt.bounds.br = bounds.tl, bounds.br
			continue
		}
		txt.bounds.tl.X = math.Min(txt.bounds.tl.X, bounds.tl.X)
		txt.bounds.tl.Y = math.Min(txt.bounds.tl.Y, bounds.tl.Y)
		txt.bounds.br.X = math.Max(txt.bounds.br.X, bounds.br.X)
		txt.bounds.br.Y = math.Max(txt.bounds.br.Y, bounds.br.Y)
	}
}

func (ruler *Ruler) spaceWidth(font d2fonts.Font) float64 {
	if _, has := ruler.atlases[font]; !has {
		ruler.addFontSize(font)
	}
	spaceRune, _ := utf8.DecodeRuneInString(" ")
	return ruler.atlases[font].glyph(spaceRune).advance
}
