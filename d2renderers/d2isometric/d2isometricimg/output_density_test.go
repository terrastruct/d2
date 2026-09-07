package d2isometricimg

import (
	"context"
	"image"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2target"
)

func visibleTexturePixels(tex *image.RGBA) int {
	visible := 0
	for i := 3; i < len(tex.Pix); i += 4 {
		if tex.Pix[i] > 0 {
			visible++
		}
	}
	return visible
}

func TestOutputDensityTextPreservesPhysicalLayout(t *testing.T) {
	// The long line and authored newline must survive changes in output size.
	value := strings.Repeat("Complete authored source. ", 12) + "END\nSecond line"
	style := normalPrintStyle()
	style.Width, style.Depth, style.Underline = 20, .8, true
	var previous *image.RGBA
	var reference textLayout
	for _, density := range []float64{16, 64} {
		p, _ := newTextPainter(context.Background(), 1)
		p.configureOutputDensity(density)
		tex, layout, err := p.texture(value, style)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(layout.Lines, "\n") != value || layout.Truncated || visibleTexturePixels(tex) == 0 {
			t.Fatal("output sampling lost authored content")
		}
		if previous != nil {
			if !reflect.DeepEqual(reference, layout) {
				t.Fatalf("output resolution changed physical text layout: %+v != %+v", reference, layout)
			}
			if tex.Rect.Dx() < previous.Rect.Dx()*3 || tex.Rect.Dy() < previous.Rect.Dy()*3 || visibleTexturePixels(tex) <= visibleTexturePixels(previous)*4 {
				t.Fatalf("larger output reused a coarse glyph texture: %v -> %v", previous.Rect, tex.Rect)
			}
		}
		previous, reference = tex, layout
	}
}

func TestOutputDensityRichLabelsAndIcons(t *testing.T) {
	ctx := context.Background()
	s := d2target.Shape{Text: d2target.Text{Label: "# Release\n\n**All source text** remains.", Language: "md", FontSize: 16, LabelWidth: 280, LabelHeight: 120}}
	style := richTestStyle()
	style.Width, style.Depth = 2.8, 1.2
	var previous *image.RGBA
	for _, density := range []float64{40, 160} {
		p, _ := newRichLabelPainter(ctx, 1)
		p.configureOutputDensity(density)
		tex, err := p.texture(s, style)
		if err != nil {
			t.Fatal(err)
		}
		if visibleTexturePixels(tex) == 0 {
			t.Fatal("rich print is blank")
		}
		if previous != nil && (tex.Rect.Dx() < previous.Rect.Dx()*3 || tex.Rect.Dy() < previous.Rect.Dy()*3) {
			t.Fatalf("rich print did not follow output size: %v -> %v", previous.Rect, tex.Rect)
		}
		previous = tex
	}
	icons, _ := newSurfaceIconPainter(ctx, 2, nil)
	source := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
	icons.configureOutputDensity(40)
	small, err := icons.texture(source, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	usedBytes, usedImport := icons.bytes, icons.remaining
	icons.configureOutputDensity(160)
	large, err := icons.texture(source, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if large.Rect.Dx() != small.Rect.Dx()*4 || large.Rect.Dy() != small.Rect.Dy()*4 {
		t.Fatalf("icon resolution was fixed: %v -> %v", small.Rect, large.Rect)
	}
	if large.RGBAAt(large.Rect.Dx()/2, 0).A != 0 || large.RGBAAt(large.Rect.Dx()*3/4, large.Rect.Dy()/2).A != 128 {
		t.Fatal("output sampling lost icon aspect fitting or alpha")
	}
	again, err := icons.texture(source, 1, 1)
	if err != nil || again != large || icons.bytes != usedBytes || icons.remaining != usedImport || len(icons.content) != 1 {
		t.Fatal("output size cache reimported or failed to reuse SVG artwork")
	}
}

func TestRichSurfaceRetainsInlineLinkDocument(t *testing.T) {
	p, _ := newRichLabelPainter(context.Background(), 1)
	p.configureOutputDensity(80)
	s := d2target.Shape{Text: d2target.Text{Label: "Read the [source](https://example.com/source).", Language: "md", FontSize: 16, LabelWidth: 280, LabelHeight: 80}}
	if _, err := p.texture(s, richTestStyle()); err != nil {
		t.Fatal(err)
	}
	if p.lastDocument == nil || len(p.lastDocument.Links) != 1 {
		t.Fatal("rich surface dropped its inline link region")
	}
	if _, err := p.texture(s, richTestStyle()); err == nil || p.lastDocument != nil {
		t.Fatal("failed rich texture retained stale document metadata")
	}
}

func TestOutputDensityNativeFaceKeepsSourceArea(t *testing.T) {
	n := fidelityNode("rectangle")
	s := nativeFaceSource(n, n.Fill)
	var previous *image.RGBA
	var reference labelSurface
	for _, density := range []float64{40, 160} {
		b := &meshBuilder{ctx: context.Background(), scale: .01, outputDensity: density, faceMaxPixels: 1 << 20}
		tex, area := b.nativeFace(s)
		if b.err != nil {
			t.Fatal(b.err)
		}
		if tex == nil {
			t.Fatal("face paint is blank")
		}
		if previous != nil {
			if area != reference {
				t.Fatal("output sampling changed the source face footprint")
			}
			if tex.Rect.Dx() < previous.Rect.Dx()*3 || tex.Rect.Dy() < previous.Rect.Dy()*3 {
				t.Fatalf("face paint did not follow output size: %v -> %v", previous.Rect, tex.Rect)
			}
		}
		previous, reference = tex, area
	}
	b := &meshBuilder{ctx: context.Background(), scale: .01, outputDensity: 1e9, faceMaxPixels: 64}
	tex, _ := b.nativeFace(s)
	if b.err != nil || tex == nil || tex.Rect.Dx()*tex.Rect.Dy() > 64 {
		t.Fatalf("high density bypassed face budget: %v", b.err)
	}
}

func TestOutputDensityTextureSharesStayBounded(t *testing.T) {
	for _, count := range []int{1, 3, 100, 7000, maxTextLabels} {
		for _, total := range []int{maxTextPixels, maxRichPixels, maxSurfaceIconPixels, 24 << 20} {
			budget := total / count
			for _, size := range [][2]float64{{1e-9, 1}, {1, 1e-9}, {.01, .01}, {4, 3}, {1e9, 1}} {
				oldW, oldH := 0, 0
				for _, density := range []float64{1e-6, 1, 100, 1e9, math.MaxFloat64} {
					w, h := surfaceTextureDimensionsAtDensity(size[0], size[1], 4096, budget, density)
					if w < 1 || h < 1 || w > 4096 || h > 4096 || w*h*count > total || w < oldW || h < oldH {
						t.Fatalf("unsafe/nonmonotonic allocation count=%d size=%v density=%g: %dx%d", count, size, density, w, h)
					}
					oldW, oldH = w, h
				}
			}
		}
	}
	w, h := surfaceTextureDimensions(2.8, .35, 4096, maxTextPixels)
	for _, density := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		actualW, actualH := surfaceTextureDimensionsAtDensity(2.8, .35, 4096, maxTextPixels, density)
		if actualW != w || actualH != h {
			t.Fatal("unconfigured density changed standalone allocation")
		}
	}
}
