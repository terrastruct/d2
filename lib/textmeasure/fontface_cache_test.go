package textmeasure

import (
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
)

func legacyFontWidth(t *Ruler, fontSpec d2fonts.Font, s string) (float64, bool) {
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

	face := ttf.newFace(float64(fontSpec.Size))
	bounds, advance := font.BoundString(face, s)
	left := min(bounds.Min.X, 0)
	right := max(bounds.Max.X, advance)
	return float64(right-left) / 64, true
}

func legacyFontAdvance(t *Ruler, fontSpec d2fonts.Font, s string) (float64, bool) {
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
	face := ttf.newFace(float64(fontSpec.Size))
	return float64(font.MeasureString(face, s)) / 64, true
}

func TestRulerFontFaceReuseMatchesFreshFaces(t *testing.T) {
	r, err := NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	for spec := range r.ttfs {
		for _, size := range []int{-1, 0, 1, 16, 72, 4097} {
			spec.Size = size
			for _, text := range []string{"", "AV To Wjgy", " é 	", "界🙂e\u0301", strings.Repeat("longer table text ", 40), "after long text", string([]byte{0xff, 'A'})} {
				wantWidth, wantSupported := legacyFontWidth(r, spec, text)
				gotWidth, gotSupported := r.measureFontWidth(spec, text)
				if !sameFloat(gotWidth, wantWidth) || gotSupported != wantSupported {
					t.Fatalf("width font=%v text=%q got=%v,%v want=%v,%v", spec, text, gotWidth, gotSupported, wantWidth, wantSupported)
				}
				wantAdvance, wantSupported := legacyFontAdvance(r, spec, text)
				gotAdvance, gotSupported := r.measureFontAdvance(spec, text)
				if !sameFloat(gotAdvance, wantAdvance) || gotSupported != wantSupported {
					t.Fatalf("advance font=%v text=%q got=%v,%v want=%v,%v", spec, text, gotAdvance, gotSupported, wantAdvance, wantSupported)
				}
			}
		}
	}
	missing := d2fonts.Font{Family: "not loaded", Size: 16}
	if _, ok := r.measureFontWidth(missing, "text"); ok {
		t.Fatal("missing width font accepted")
	}
	if _, ok := r.measureFontAdvance(missing, "text"); ok {
		t.Fatal("missing advance font accepted")
	}
}

func TestRulerFontFacesHavePrivateScratchBuffers(t *testing.T) {
	first, err := NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	spec := d2fonts.SourceSansPro.Font(16, d2fonts.FONT_STYLE_REGULAR)
	first.measureFontAdvance(spec, "first")
	cached := first.faces[spec]
	first.measureFontWidth(spec, "second")
	first.addFontSize(spec)
	if cached == nil || first.faces[spec] != cached || first.atlases[spec].face != cached {
		t.Fatal("font face not reused within ruler")
	}
	second.measureFontAdvance(spec, "other")
	if second.faces[spec] == cached {
		t.Fatal("mutable font face shared between rulers")
	}
	larger := spec
	larger.Size++
	first.measureFontAdvance(larger, "size")
	if first.faces[larger] == cached {
		t.Fatal("font face shared between sizes")
	}
}
