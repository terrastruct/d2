package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

const surfaceTestSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20" viewBox="0 0 40 20"><rect width="20" height="20" fill="#ff0000"/><rect x="20" width="20" height="20" fill="#0000ff" fill-opacity="0.5"/></svg>`

func iconData(t *testing.T, mime string, data []byte) *url.URL {
	t.Helper()
	u, err := url.Parse("data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestSurfaceIconSVGAspectAlphaAndCache(t *testing.T) {
	p, err := newSurfaceIconPainter(context.Background(), 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	u := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
	frame, err := p.texture(u, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Bounds() != image.Rect(0, 0, 160, 160) {
		t.Fatal(frame.Bounds())
	}
	if got := frame.RGBAAt(80, 20); got.A != 0 {
		t.Fatalf("aspect-fit letterbox lost transparency: %v", got)
	}
	if got := frame.RGBAAt(40, 80); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("red SVG face = %v", got)
	}
	if got := frame.RGBAAt(120, 80); got.A < 126 || got.A > 129 || got.B != got.A || got.R != 0 {
		t.Fatalf("SVG alpha was not premultiplied: %v", got)
	}
	usedBytes, usedPixels, usedImport := p.bytes, p.pixels, p.remaining
	again, err := p.texture(u, 1, 1)
	if err != nil || again != frame || p.bytes != usedBytes || p.pixels != usedPixels || p.remaining != usedImport {
		t.Fatalf("repeated icon was not reused: %v", err)
	}
	if _, err := p.texture(u, 2, 1); err != nil {
		t.Fatal(err)
	}
	if p.bytes != usedBytes || p.remaining != usedImport || len(p.content) != 1 {
		t.Fatal("second resolution reimported the same SVG")
	}
}

func TestSurfaceIconRasterAndExplicitLocalResolver(t *testing.T) {
	raster := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	raster.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	raster.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 128})
	var data bytes.Buffer
	if err := png.Encode(&data, raster); err != nil {
		t.Fatal(err)
	}
	p, err := newSurfaceIconPainter(context.Background(), 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	u := iconData(t, "image/png", data.Bytes())
	frame, err := p.texture(u, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.RGBAAt(240, 80); got.G != got.A || got.A != 128 || got.R != 0 {
		t.Fatalf("raster alpha = %v", got)
	}
	path := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	local := &url.URL{Path: path}
	if _, err := p.texture(local, 2, 1); err == nil || !strings.Contains(err.Error(), "explicit asset resolver") {
		t.Fatalf("implicit local read accepted: %v", err)
	}
	withAssets, err := newSurfaceIconPainter(context.Background(), 1, &d2scenebuild.AssetOptions{Resolver: p.resolver})
	if err != nil {
		t.Fatal(err)
	}
	localFrame, err := withAssets.texture(local, 2, 1)
	if err != nil || !bytes.Equal(frame.Pix, localFrame.Pix) {
		t.Fatalf("explicit local resolver differs from embedded asset: %v", err)
	}
	remote, _ := url.Parse("https://example.invalid/icon.svg")
	if _, err := p.texture(remote, 1, 1); err == nil || !strings.Contains(err.Error(), "explicit asset resolver") {
		t.Fatalf("implicit network access accepted: %v", err)
	}
}

func TestSurfaceIconCancellationAndBudgets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p, err := newSurfaceIconPainter(ctx, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	u := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
	if _, err := p.texture(u, 1, 1); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := p.texture(u, 1, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cached texture ignored cancellation: %v", err)
	}
	if _, err := newSurfaceIconPainter(ctx, 1, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	p, err = newSurfaceIconPainter(context.Background(), 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.remaining.MaxSourceBytes = len(surfaceTestSVG)*2 - 1
	if _, err := p.texture(u, 1, 1); err != nil {
		t.Fatal(err)
	}
	other := iconData(t, "image/svg+xml", []byte(strings.Replace(surfaceTestSVG, "#ff0000", "#00ff00", 1)))
	if _, err := p.texture(other, 1, 1); err == nil {
		t.Fatal("aggregate SVG source budget was not enforced")
	}
	if p.pixels != 160*160 || len(p.textures) != 1 {
		t.Fatal("failed render retained a partial texture")
	}
	for _, count := range []int{0, 1, 32, 1000, maxSurfaceIcons} {
		p, err := newSurfaceIconPainter(context.Background(), count, nil)
		if err != nil || int64(p.dimension)*int64(p.dimension)*int64(max(1, count)) > maxSurfaceIconPixels {
			t.Fatalf("count %d exceeds aggregate texture budget: %v", count, err)
		}
	}
}

func TestSurfaceIconRejectsExternalSVGReferences(t *testing.T) {
	p, err := newSurfaceIconPainter(context.Background(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	u := iconData(t, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><image href="https://example.invalid/photo.png" width="20" height="20"/></svg>`))
	if _, err := p.texture(u, 1, 1); err == nil {
		t.Fatal("an unresolved external SVG image was accepted")
	}
}

func TestSurfaceIconLayoutStaysOnFace(t *testing.T) {
	u := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
	positions := []string{"", "INSIDE_TOP_LEFT", "INSIDE_TOP_CENTER", "INSIDE_TOP_RIGHT", "INSIDE_MIDDLE_LEFT", "INSIDE_MIDDLE_CENTER", "INSIDE_MIDDLE_RIGHT", "INSIDE_BOTTOM_CENTER", "OUTSIDE_LEFT_MIDDLE", "OUTSIDE_TOP_CENTER"}
	for _, w := range []float64{.00000001, .01, .8, 4, 100000000} {
		for _, d := range []float64{.00000001, .01, 1, 3, 100000000} {
			for _, angle := range []float64{0, .6, -2.8} {
				for _, position := range positions {
					for _, kind := range []string{"node", "board", "edge"} {
						s := labelSurface{center: nv(2, 5, -3), width: w, depth: d, angle: angle}
						shape := d2target.Shape{Icon: u, IconPosition: position, Width: 123, Height: 87, Text: d2target.Text{Label: "API", LabelWidth: 80, LabelHeight: 20}}
						icon, text := surfaceIconLayout(s, shape, .01, kind)
						if shape.Width != 123 || shape.Height != 87 {
							t.Fatal("shape footprint mutated")
						}
						local := func(r labelSurface) (float64, float64) {
							delta := nsub(r.center, s.center)
							return math.Cos(angle)*delta.X + math.Sin(angle)*delta.Z, -math.Sin(angle)*delta.X + math.Cos(angle)*delta.Z
						}
						for _, r := range []labelSurface{icon, text} {
							x, z := local(r)
							tolerance := max(w, d)*1e-12 + 1e-12
							if !captionFinite(x, z, r.width, r.depth) || r.width <= 0 || r.depth <= 0 || math.Abs(x)+r.width/2 > w/2+tolerance || math.Abs(z)+r.depth/2 > d/2+tolerance || r.center.Y != s.center.Y || r.angle != angle {
								t.Fatalf("surface escaped face %gx%g %s: %+v", w, d, position, r)
							}
						}
						x1, z1 := local(icon)
						x2, z2 := local(text)
						tolerance := max(w, d)*1e-12 + 1e-12
						if math.Abs(x1-x2)+tolerance < (icon.width+text.width)/2 && math.Abs(z1-z2)+tolerance < (icon.depth+text.depth)/2 {
							t.Fatalf("icon overlaps label: %+v %+v", icon, text)
						}
					}
				}
			}
		}
	}
}

func TestSurfaceIconDimensionsExtremeAspect(t *testing.T) {
	for _, size := range [][2]float64{{1e-200, 1e200}, {1e200, 1e-200}, {1e200, 1e200}, {.01, .01}} {
		w, h := surfaceIconDimensions(size[0], size[1], 512)
		if w < 1 || h < 1 || w > 512 || h > 512 {
			t.Fatalf("invalid texture dimensions %dx%d for %v", w, h, size)
		}
	}
}

func TestSurfaceIconNativeRenderPreservesFootprint(t *testing.T) {
	diagram := captureDiagram()
	u := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
	beforeWidth, beforeHeight := diagram.Shapes[0].Width, diagram.Shapes[0].Height
	diagram.Shapes[0].Icon = u
	diagram.Shapes[0].IconPosition = "INSIDE_MIDDLE_LEFT"
	withIcon, err := Render(context.Background(), diagram, &Options{Width: 320, Height: 200})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(withIcon))
	if err != nil {
		t.Fatal(err)
	}
	redPixels := 0
	for y := 0; y < decoded.Bounds().Dy(); y++ {
		for x := 0; x < decoded.Bounds().Dx(); x++ {
			c := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA)
			if c.R > 220 && c.G < 50 && c.B < 50 {
				redPixels++
			}
		}
	}
	if redPixels < 5 || diagram.Shapes[0].Width != beforeWidth || diagram.Shapes[0].Height != beforeHeight {
		t.Fatalf("native icon absent or footprint changed: red pixels=%d", redPixels)
	}
}

func TestSurfaceIconReservesCompiledLabelAndWideImage(t *testing.T) {
	u := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
	s := labelSurface{width: 1, depth: .6}
	shape := d2target.Shape{Icon: u, IconPosition: "INSIDE_MIDDLE_LEFT", Text: d2target.Text{Label: "Healthy", LabelWidth: 62, LabelHeight: 21, FontSize: 16}}
	_, label := surfaceIconLayout(s, shape, .01, "node")
	if label.width < .62 {
		t.Fatalf("icon consumed source-allocated label width: %g", label.width)
	}
	p, err := newTextPainter(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, layout, err := p.texture(shape.Label, labelTextStyle{Width: label.width, Depth: label.depth, FontSize: 16, PixelScale: .01, Bold: true, Color: color.NRGBA{A: 255}, Opacity: 1})
	if err != nil || len(layout.Lines) != 1 || layout.Lines[0] != "Healthy" {
		t.Fatalf("ordinary single word was split by icon placement: %+v %v", layout, err)
	}
	shape.Type = d2target.ShapeImage
	icon, _ := surfaceIconLayout(labelSurface{width: 2.4, depth: 1.4}, shape, .01, "node")
	if icon.width != 2.4 || icon.depth <= .7 {
		t.Fatalf("wide image confined to a square badge: %+v", icon)
	}
}

func TestSurfaceIconEmptyStructuredHeaderRetainsRows(t *testing.T) {
	u := iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
	for _, shape := range []d2target.Shape{
		{Icon: u, Type: d2target.ShapeSQLTable, SQLTable: d2target.SQLTable{Columns: []d2target.SQLColumn{{Name: d2target.Text{Label: "owner_id"}}}}},
		{Icon: u, Type: d2target.ShapeClass, Class: d2target.Class{Fields: []d2target.ClassField{{Name: "owner"}}}},
		{Icon: u, Type: d2target.ShapeClass, Class: d2target.Class{Methods: []d2target.ClassMethod{{Name: "save()"}}}},
	} {
		icon, label := surfaceIconLayout(labelSurface{width: 2, depth: 3}, shape, .01, "node")
		if icon.width <= 0 || label.width != 2 || label.depth < 2.5 {
			t.Fatalf("empty header discarded or narrowed structured rows: icon=%+v label=%+v", icon, label)
		}
	}
}
