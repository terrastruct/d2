package d2isometricimg

import (
	"bytes"
	"context"
	"image/color"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

func normalPrintStyle() labelTextStyle {
	return labelTextStyle{Width: 2.8, Depth: .35, FontSize: 16, PixelScale: .01, Color: color.NRGBA{R: 32, G: 49, B: 72, A: 255}, Opacity: 1}
}

func TestNativeTextPreservesSourceFontSizeAndMultiline(t *testing.T) {
	p, err := newTextPainter(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	style := normalPrintStyle()
	texture, layout, err := p.texture("TenantCluster3 [Tantanga-Medium]", style)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Truncated || len(layout.Lines) != 1 || math.Abs(layout.FontSize-.16) > 1e-9 {
		t.Fatalf("source font space changed: %+v", layout)
	}
	ink := 0
	partial := false
	for i := 0; i < len(texture.Pix); i += 4 {
		a := texture.Pix[i+3]
		if a > 0 {
			ink++
		}
		if a > 0 && a < 255 {
			partial = true
		}
		if texture.Pix[i] > a || texture.Pix[i+1] > a || texture.Pix[i+2] > a {
			t.Fatal("texture is not premultiplied RGBA")
		}
	}
	if ink < 100 || !partial {
		t.Fatalf("missing antialiased native glyphs: %d ink pixels, partial=%v", ink, partial)
	}
	style.Depth = 1.6
	text := "first row\nsecond row\nthird row\nfourth row\nfifth row\nsixth row"
	_, layout, err = p.texture(text, style)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Truncated || len(layout.Lines) != 6 || math.Abs(layout.FontSize-.16) > 1e-9 {
		t.Fatalf("authored lines changed: %+v", layout)
	}
}

func TestNativeTextStylesAreDeterministicAndUseBundledFaces(t *testing.T) {
	styles := []labelTextStyle{normalPrintStyle(), normalPrintStyle(), normalPrintStyle(), normalPrintStyle(), normalPrintStyle()}
	styles[1].Bold = true
	styles[2].Italic = true
	styles[3].Bold, styles[3].Italic = true, true
	styles[4].FontFamily = "MONO"
	var variants [][]byte
	for _, style := range styles {
		p, _ := newTextPainter(context.Background(), 2)
		first, _, err := p.texture("Café e\u0301 → AV glyphs", style)
		if err != nil {
			t.Fatal(err)
		}
		second, _, err := p.texture("Café e\u0301 → AV glyphs", style)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first.Pix, second.Pix) {
			t.Fatal("native texture changed between identical calls")
		}
		for _, previous := range variants {
			if bytes.Equal(previous, first.Pix) {
				t.Fatal("font styles produced identical glyph outlines")
			}
		}
		variants = append(variants, first.Pix)
		if len(p.faces) != 1 {
			t.Fatal("unexpected font cache growth")
		}
	}
}

func TestNativeTextTextureAndGlyphBudgets(t *testing.T) {
	p, err := newTextPainter(context.Background(), maxTextLabels)
	if err != nil {
		t.Fatal(err)
	}
	if p.tileWidth*p.tileHeight*maxTextLabels > maxTextPixels {
		t.Fatal("admission texture plan exceeds pixel budget")
	}
	style := normalPrintStyle()
	for _, aspect := range []float64{1e-8, .1, 1, 10, 1e8} {
		style.Width, style.Depth = aspect, 1
		texture, layout, err := p.texture("W", style)
		if err != nil {
			t.Fatal(err)
		}
		if texture.Rect.Dx() < 1 || texture.Rect.Dy() < 1 || math.IsNaN(layout.FontSize) || math.IsInf(layout.FontSize, 0) || layout.FontSize <= 0 {
			t.Fatalf("invalid tiny print surface: %v %+v", texture.Rect, layout)
		}
	}
	p.glyphWork = maxTextGlyphWork
	if _, _, err := p.texture("bounded", normalPrintStyle()); err == nil {
		t.Fatal("glyph work cap was ignored")
	}
	if _, err := newTextPainter(context.Background(), maxTextLabels+1); err == nil {
		t.Fatal("label admission cap was ignored")
	}
	one, _ := newTextPainter(context.Background(), 1)
	if _, _, err := one.texture("one", normalPrintStyle()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := one.texture("two", normalPrintStyle()); err == nil {
		t.Fatal("declared label allocation count was ignored")
	}
}

func TestNativeTextClippingAndCancellation(t *testing.T) {
	p0, _ := newTextPainter(context.Background(), 1)
	if _, _, err := p0.texture(strings.Repeat("😀", maxLabelRunes+1), normalPrintStyle()); err == nil {
		t.Fatal("oversized text must fail explicitly, not silently truncate")
	}
	measure := func(text string, size float64) (float64, error) {
		return float64(utf8.RuneCountInString(text)) * size, nil
	}
	for _, width := range []float64{.001, 1, 100} {
		layout, err := fitNativeLabel("WWW word wide glyph", false, measure, width, width, 32, 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range layout.Lines {
			actual, _ := measure(line, layout.FontSize)
			if actual > width+1e-9 {
				t.Fatalf("glyphs exceed print width: %+v", layout)
			}
		}
		if float64(len(layout.Lines))*layout.LineHeight > width+1e-9 {
			t.Fatalf("glyphs exceed print depth: %+v", layout)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	p, _ := newTextPainter(ctx, 1)
	cancel()
	if _, _, err := p.texture("cancel", normalPrintStyle()); err != context.Canceled {
		t.Fatalf("want cancellation, got %v", err)
	}
}
