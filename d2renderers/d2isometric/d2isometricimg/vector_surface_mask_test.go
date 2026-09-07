package d2isometricimg

import (
	"context"
	"encoding/xml"
	"image"
	"image/color"
	"io"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

func TestNativeVectorMaskTypeUsesPresentationAttribute(t *testing.T) {
	for _, kind := range []d2scene.MaskType{d2scene.MaskAlpha, d2scene.MaskLuminance} {
		node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, A: 255}}})
		node.Mask = &d2scene.Mask{Type: kind, Transform: d2scene.Identity(), Root: d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 128}}})}
		surface := &nativeVectorSurface{document: &d2scene.Document{Root: node, LogicalWidth: 1, LogicalHeight: 1, ViewBox: d2scene.Box{Width: 1, Height: 1}}}
		fragment, err := nativeSurfaceSVG(context.Background(), surface, "mask")
		if err != nil {
			t.Fatal(err)
		}
		want := "alpha"
		if kind == d2scene.MaskLuminance {
			want = "luminance"
		}
		if !strings.Contains(fragment, `mask-type="`+want+`"`) || strings.Contains(fragment, "mask-type:") {
			t.Fatalf("typed %s mask relies on CSS property support", want)
		}
	}
	w := &nativeSurfaceSVGWriter{ctx: context.Background(), prefix: "glyph"}
	w.colorPaint(nil, fontface.COLRv1Composite{Mode: fontface.COLRv1CompositeSrcIn, Source: fontface.COLRv1Solid{Color: color.NRGBA{R: 255, A: 255}}, Backdrop: fontface.COLRv1Solid{Color: color.NRGBA{A: 128}}}, d2scene.Box{Width: 10, Height: 10}, d2scene.Identity(), 0)
	if w.err != nil {
		t.Fatal(w.err)
	}
	if !strings.Contains(w.defs.String(), `mask-type="alpha"`) || strings.Contains(w.defs.String(), "mask-type:") {
		t.Fatal("color glyph source-in lost alpha-mask semantics")
	}
}

func TestNativeVectorCompensationMaskHasWhiteRGB(t *testing.T) {
	ctx := nativeVectorContext(context.Background())
	doc := &d2scene.Document{Root: d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 255}}}), ViewBox: d2scene.Box{Width: 1, Height: 1}, LogicalWidth: 1, LogicalHeight: 1}
	fill, ink := image.NewRGBA(image.Rect(0, 0, 1, 1)), image.NewRGBA(image.Rect(0, 0, 1, 1))
	for _, texture := range []*image.RGBA{fill, ink} {
		if err := retainNativeVectorSurface(ctx, texture, doc); err != nil {
			t.Fatal(err)
		}
	}
	b := meshBuilder{ctx: ctx}
	b.nativeFaceOpacity(fill, ink, .5)
	fragment := vectorSurfaceFragment(t, ctx, fill)
	decoder := xml.NewDecoder(strings.NewReader("<svg>" + fragment + "</svg>"))
	channels := map[string]bool{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Local != "feFuncR" && element.Name.Local != "feFuncG" && element.Name.Local != "feFuncB" {
			continue
		}
		attrs := map[string]string{}
		for _, attr := range element.Attr {
			attrs[attr.Name.Local] = attr.Value
		}
		if attrs["type"] != "linear" || attrs["slope"] != "0" || attrs["intercept"] != "1" {
			t.Fatalf("%s does not make SourceAlpha white for a luminance-mask reader: %v", element.Name.Local, attrs)
		}
		channels[element.Name.Local] = true
	}
	if len(channels) != 3 {
		t.Fatal("compensated alpha mask becomes black under SVG 1.1 luminance semantics")
	}
}
