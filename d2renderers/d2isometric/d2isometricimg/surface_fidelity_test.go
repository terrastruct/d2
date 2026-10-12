package d2isometricimg

import (
	"bytes"
	"context"
	"image/color"
	"math"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2latex"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

func TestNativePlainLabelKeepsLongContentAndAuthoredWhitespace(t *testing.T) {
	text := strings.Repeat("A detailed architecture note with  two spaces.\n", 80) + "FINAL SENTENCE"
	p, _ := newTextPainter(context.Background(), 1)
	style := normalPrintStyle()
	style.Width, style.Depth = 6, 25
	_, layout, err := p.texture(text, style)
	if err != nil {
		t.Fatal(err)
	}
	if layout.Truncated || strings.Join(layout.Lines, "\n") != text || len(layout.Lines) != 81 {
		t.Fatalf("long source content was altered: %d lines, truncated=%v", len(layout.Lines), layout.Truncated)
	}
	if math.Abs(layout.FontSize-.16) > 1e-9 {
		t.Fatalf("source font size changed: %v", layout.FontSize)
	}
}

func registerSurfaceTestFamily(t *testing.T, name string, source d2fonts.FontFamily) d2fonts.FontFamily {
	t.Helper()
	data := func(style d2fonts.FontStyle) []byte {
		return d2fonts.FontFaces.Get(d2fonts.Font{Family: source, Style: style})
	}
	family, err := d2fonts.AddFontFamily(name, data(d2fonts.FONT_STYLE_REGULAR), data(d2fonts.FONT_STYLE_ITALIC), data(d2fonts.FONT_STYLE_BOLD), data(d2fonts.FONT_STYLE_SEMIBOLD))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d2fonts.FontFamiliesMu.Lock()
		defer d2fonts.FontFamiliesMu.Unlock()
		for i, f := range d2fonts.FontFamilies {
			if f == *family {
				d2fonts.FontFamilies = append(d2fonts.FontFamilies[:i], d2fonts.FontFamilies[i+1:]...)
				break
			}
		}
		for _, style := range d2fonts.FontStyles {
			key := d2fonts.Font{Family: *family, Style: style}
			d2fonts.FontFaces.Delete(key)
			d2fonts.FontEncodings.Delete(key)
		}
	})
	return *family
}

func TestNativePlainAndRichUseConfiguredAndExplicitFontFamilies(t *testing.T) {
	custom := registerSurfaceTestFamily(t, "SurfaceConfiguredFont", d2fonts.SourceCodePro)
	mono := registerSurfaceTestFamily(t, "SurfaceConfiguredMono", d2fonts.SourceSansPro)
	p, _ := newTextPainter(context.Background(), 3)
	p.configureFontFamilies(&custom, &mono)
	style := normalPrintStyle()
	for _, item := range []struct {
		name   string
		family d2fonts.FontFamily
	}{{"DEFAULT", custom}, {"mono", mono}, {"HandDrawn", d2fonts.HandDrawn}} {
		style.FontFamily = item.name
		if _, _, err := p.texture("Actual configured face", style); err != nil {
			t.Fatal(err)
		}
		if p.faces[d2fonts.Font{Family: item.family, Style: d2fonts.FONT_STYLE_REGULAR}] == nil {
			t.Fatalf("wrong family for %s", item.name)
		}
	}
	original := d2target.Shape{Text: d2target.Text{Label: "Configured **bold** and `code`", Language: "markdown", FontSize: 16, LabelWidth: 350, LabelHeight: 50}}
	doc, err := richLabelDocument(context.Background(), original, richTestStyle(), custom, mono)
	if err != nil {
		t.Fatal(err)
	}
	primaryUsed, monoUsed := false, false
	for _, run := range richRuns(doc) {
		primaryUsed = primaryUsed || run.Font.Family == string(custom)
		monoUsed = monoUsed || run.Font.Family == string(mono)
	}
	if !primaryUsed || !monoUsed {
		t.Fatalf("configured rich fonts lost: primary=%v mono=%v", primaryUsed, monoUsed)
	}
	original.FontFamily = string(d2fonts.HandDrawn)
	doc, err = richLabelDocument(context.Background(), original, richTestStyle(), custom, mono)
	if err != nil {
		t.Fatal(err)
	}
	if richRuns(doc)[0].Font.Family != string(d2fonts.HandDrawn) {
		t.Fatal("explicit rich bundled family ignored")
	}
}

func TestNativeLatexIsTypesetAndLabelBackingPreserved(t *testing.T) {
	t.Setenv("PATH", "")
	formula := `\frac{x^2 + 1}{\sqrt{y}}`
	w, h, err := d2latex.Measure(formula)
	if err != nil {
		t.Fatal(err)
	}
	original := d2target.Shape{Text: d2target.Text{Label: formula, Language: "latex", FontSize: 16, LabelWidth: w, LabelHeight: h}}
	style := richTestStyle()
	bg := color.NRGBA{R: 180, G: 210, B: 230, A: 255}
	style.Background = &bg
	style.Opacity = .5
	doc, err := richLabelDocument(context.Background(), original, style)
	if err != nil {
		t.Fatal(err)
	}
	if len(richRuns(doc)) != 0 {
		t.Fatal("LaTeX was printed as raw text")
	}
	vectors := 0
	for _, asset := range doc.Assets {
		if _, ok := asset.(d2scene.VectorAsset); ok {
			vectors++
		}
	}
	if vectors == 0 {
		t.Fatal("typeset mathematical artwork missing")
	}
	p, _ := newRichLabelPainter(context.Background(), 1)
	tex, err := p.texture(original, style)
	if err != nil {
		t.Fatal(err)
	}
	corner := tex.RGBAAt(0, 0)
	if corner.A < 127 || corner.A > 128 || corner.R < 89 || corner.R > 91 {
		t.Fatalf("label backing lost or opacity doubled: %v", corner)
	}
	different := 0
	for i := 0; i < len(tex.Pix); i += 4 {
		if !bytes.Equal(tex.Pix[i:i+4], []byte{corner.R, corner.G, corner.B, corner.A}) {
			different++
		}
	}
	if different < 50 {
		t.Fatal("typeset math texture is blank")
	}
}

func TestNativeSQLRowsRetainOriginalCoordinates(t *testing.T) {
	s := d2target.Shape{Type: d2target.ShapeSQLTable, Width: 360, Height: 300, Text: d2target.Text{Label: "Accounts", FontSize: 16, LabelWidth: 80, LabelHeight: 20}}
	for _, name := range []string{"id", "owner", "status", "created", "updated"} {
		s.Columns = append(s.Columns, d2target.SQLColumn{Name: d2target.Text{Label: name, LabelWidth: 65}, Type: d2target.Text{Label: "text", LabelWidth: 32}})
	}
	doc, err := richLabelDocument(context.Background(), s, richTestStyle())
	if err != nil {
		t.Fatal(err)
	}
	row := 0
	for _, run := range richRuns(doc) {
		if row < len(s.Columns) && run.Text == s.Columns[row].Name.Label {
			want := float64(row+1)*50 + 25 + 4 // compiled row center plus font baseline adjustment
			if math.Abs(run.Origin.Y-want) > 1e-9 {
				t.Fatalf("row %d origin %v, want %v", row, run.Origin.Y, want)
			}
			row++
		}
	}
	if row != 5 || doc.ViewBox != (d2scene.Box{Width: 360, Height: 300}) {
		t.Fatalf("SQL coordinate contract changed: rows=%d box=%v", row, doc.ViewBox)
	}
}

func TestNativeIconRoundedCornersAreCachedSeparately(t *testing.T) {
	p, _ := newSurfaceIconPainter(context.Background(), 3, nil)
	u := iconData(t, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="red"/></svg>`))
	square, err := p.texture(u, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	round, err := p.texture(u, 1, 1, .25)
	if err != nil {
		t.Fatal(err)
	}
	if square.RGBAAt(0, 0).A != 255 || round.RGBAAt(0, 0).A != 0 || round.RGBAAt(80, 80).A != 255 {
		t.Fatal("rounded clip did not preserve center and remove corners")
	}
	again, err := p.texture(u, 1, 1, .25)
	if err != nil || again != round || round == square {
		t.Fatal("radius-aware texture cache failed")
	}
}

func TestNativePlanarDocumentFrameIsDeterministicAndBounded(t *testing.T) {
	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, A: 255}}})
	node.Animations = []d2scene.Track{{Property: d2scene.AnimateFillColor, Duration: time.Second, Keyframes: []d2scene.Keyframe{{Offset: 0, Value: d2scene.ColorValue(color.NRGBA{R: 255, A: 255})}, {Offset: 1, Value: d2scene.ColorValue(color.NRGBA{B: 255, A: 255})}}}}
	doc := d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, node)
	before := *doc
	a, err := rasterNativeSurfaceDocument(context.Background(), doc, 2048, 1024, .5)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rasterNativeSurfaceDocument(context.Background(), doc, 2048, 1024, .5)
	if err != nil || !bytes.Equal(a.Pix, b.Pix) {
		t.Fatalf("same-time surface capture differs: %v", err)
	}
	if doc.LogicalWidth != before.LogicalWidth || doc.ViewBox != before.ViewBox {
		t.Fatal("surface raster mutated source document")
	}
	if a.RGBAAt(0, 0).A != 0 {
		t.Fatal("transparent letterbox became opaque")
	}
	if _, err := rasterNativeSurfaceDocument(context.Background(), doc, 8192, 8192); err == nil {
		t.Fatal("texture budget not enforced")
	}
	if _, err := rasterNativeSurfaceDocument(context.Background(), doc, 10, 10, math.Inf(1)); err == nil {
		t.Fatal("non-finite time accepted")
	}
}

func TestNativeSurfaceEmbeddedAssetsOnlyAndTheme(t *testing.T) {
	d := d2target.NewDiagram()
	d.Root.Fill, d.Root.Stroke = "transparent", "none"
	fill := "#ff0077"
	theme := int64(0)
	d.Config = &d2target.Config{ThemeID: &theme, ThemeOverrides: &d2target.ThemeOverrides{B1: &fill}}
	d.Shapes = []d2target.Shape{{ID: "image", Type: d2target.ShapeImage, Width: 40, Height: 40, Opacity: 1, Icon: iconData(t, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="blue"/></svg>`))}, {ID: "themed", Type: d2target.ShapeRectangle, Pos: d2target.Point{X: 60}, Width: 40, Height: 40, Fill: "B1", Stroke: "none", Opacity: 1}}
	doc, err := nativeSurfaceDocument(context.Background(), d, nil)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := rasterNativeSurfaceDocument(context.Background(), doc, 1000, 400)
	if err != nil {
		t.Fatal(err)
	}
	blue, pink := 0, 0
	for i := 0; i < len(frame.Pix); i += 4 {
		if frame.Pix[i+2] > 200 && frame.Pix[i] < 20 {
			blue++
		}
		if frame.Pix[i] > 240 && frame.Pix[i+1] < 10 && frame.Pix[i+2] > 100 && frame.Pix[i+2] < 140 {
			pink++
		}
	}
	if blue < 100 || pink < 100 {
		t.Fatalf("embedded icon or theme override missing: blue=%d pink=%d", blue, pink)
	}
	d.Shapes[0].Icon = &url.URL{Scheme: "https", Host: "not-contacted.invalid", Path: "/icon.svg"}
	if _, err := nativeSurfaceDocument(context.Background(), d, nil); err == nil || !strings.Contains(err.Error(), "explicit asset resolver") {
		t.Fatalf("external asset admission failed: %v", err)
	}
}

func TestNativeTextColorEmojiAndUnavailableGlyphError(t *testing.T) {
	p, _ := newTextPainter(context.Background(), 2)
	style := normalPrintStyle()
	style.Width, style.Depth = 1, 1
	style.FontSize = 60
	tex, layout, err := p.texture("🚀", style)
	if err != nil {
		t.Fatal(err)
	}
	colors := map[color.RGBA]bool{}
	for y := 0; y < tex.Bounds().Dy(); y++ {
		for x := 0; x < tex.Bounds().Dx(); x++ {
			c := tex.RGBAAt(x, y)
			if c.A > 200 {
				colors[c] = true
			}
		}
	}
	if len(colors) < 5 || layout.Truncated || strings.Join(layout.Lines, "\n") != "🚀" {
		t.Fatalf("bundled emoji artwork missing: colors=%d layout=%+v", len(colors), layout)
	}
	if _, _, err := p.texture("日本語", style); err == nil || !strings.Contains(err.Error(), "no available font") {
		t.Fatalf("missing Japanese face must return explicit error: %v", err)
	}
}

type surfaceFixedFallback struct {
	font      d2fonts.FallbackFont
	requested bool
}

func (r *surfaceFixedFallback) ResolveFallbacks(ctx context.Context, request d2fonts.FallbackRequest) ([]d2fonts.FallbackFont, error) {
	r.requested = true
	return []d2fonts.FallbackFont{r.font}, ctx.Err()
}

func TestNativeTextUsesExplicitFallbackResolver(t *testing.T) {
	family := d2fonts.HandDrawn
	p, _ := newTextPainter(context.Background(), 1)
	p.configureFontFamilies(&family, nil)
	resolver := &surfaceFixedFallback{font: d2fonts.FallbackFont{Name: "supplied sans", MIMEType: "font/ttf", Data: d2fonts.FontFaces.Get(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})}}
	p.configureFallbackFonts(&d2scenebuild.FontFallbackOptions{Resolver: resolver, MaxAssets: 4, MaxBytes: 8 << 20})
	style := normalPrintStyle()
	style.Width, style.Depth = 2, 1
	_, layout, err := p.texture("Ж Ω", style)
	if err != nil {
		t.Fatal(err)
	}
	if !resolver.requested || layout.Truncated || strings.Join(layout.Lines, "\n") != "Ж Ω" {
		t.Fatal("explicit fallback resolver was not used for complete source text")
	}
}

func TestNativeSurfaceTextureDensityPreservesAggregateBudget(t *testing.T) {
	w, h := surfaceTextureDimensions(24, 3, 4096, maxTextPixels)
	if w != 4096 || h != 512 {
		t.Fatalf("wide source label lost high-resolution texture density: %dx%d", w, h)
	}
	for _, count := range []int{1, 3, 25, 1000, maxTextLabels} {
		budget := maxTextPixels / count
		for _, size := range [][2]float64{{1, 1}, {24, 3}, {3, 24}, {1e-300, 1e300}, {1e300, 1e-300}} {
			w, h := surfaceTextureDimensions(size[0], size[1], 4096, budget)
			if w < 1 || h < 1 || w > 4096 || h > 4096 || w*h*count > maxTextPixels {
				t.Fatalf("count=%d surface=%v exceeds texture bounds: %dx%d", count, size, w, h)
			}
		}
	}
}

func TestNativeCanonicalArtworkStaysOnFaceWithOutsideCaption(t *testing.T) {
	for _, kind := range []string{d2target.ShapeImage, d2target.ShapeRectangle} {
		t.Run(kind, func(t *testing.T) {
			n := fidelityNode(kind)
			n.Metadata.Original.Label = "Dashboard"
			n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 90, 20
			n.Metadata.Original.LabelPosition = "OUTSIDE_BOTTOM_CENTER"
			n.Metadata.Original.Icon = iconData(t, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="200" height="120"><rect width="200" height="120" fill="#ff0080"/></svg>`))
			text, _ := newTextPainter(context.Background(), 1)
			icons, _ := newSurfaceIconPainter(context.Background(), 1, nil)
			b := &meshBuilder{ctx: context.Background(), scale: .01, text: text, icons: icons}
			b.hierarchyNode(n, "#849ebc")
			if b.err != nil {
				t.Fatal(b.err)
			}
			minX, maxX, minZ, maxZ := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
			iconTriangles := 0
			for _, tri := range b.triangles {
				tex := tri.Material.Texture
				if tex == nil {
					continue
				}
				c := color.NRGBAModel.Convert(tex.At(tex.Bounds().Dx()/2, tex.Bounds().Dy()/2)).(color.NRGBA)
				if c.R < 250 || c.G > 5 || c.B < 120 || c.B > 135 || c.A < 250 {
					continue
				}
				iconTriangles++
				for _, vertex := range tri.V {
					p := vertex.Position
					minX, maxX, minZ, maxZ = min(minX, p.X), max(maxX, p.X), min(minZ, p.Z), max(maxZ, p.Z)
				}
			}
			if iconTriangles != 2 || minX < 2-1e-9 || maxX > 4+1e-9 || minZ < 3.4-1e-9 || maxZ > 4.6+1e-9 {
				t.Fatalf("artwork left its original face: triangles=%d X[%g,%g] Z[%g,%g]", iconTriangles, minX, maxX, minZ, maxZ)
			}
			if kind == d2target.ShapeImage && (math.Abs(maxX-minX-2) > 1e-9 || math.Abs(maxZ-minZ-1.2) > 1e-9) {
				t.Fatal("image artwork no longer covers the complete source footprint")
			}
			// The last two triangles print the separate caption at its compiled
			// 90x20 source-pixel box, not the image-sized outside rectangle.
			minX, maxX, minZ, maxZ = math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
			for _, tri := range b.triangles[len(b.triangles)-2:] {
				for _, vertex := range tri.V {
					p := vertex.Position
					minX, maxX, minZ, maxZ = min(minX, p.X), max(maxX, p.X), min(minZ, p.Z), max(maxZ, p.Z)
				}
			}
			if math.Abs(maxX-minX-.9) > 1e-9 || math.Abs(maxZ-minZ-.2) > 1e-9 || minZ <= 4.6 || math.Abs((minX+maxX)/2-3) > 1e-9 {
				t.Fatalf("outside caption lost its source placement: X[%g,%g] Z[%g,%g]", minX, maxX, minZ, maxZ)
			}
			camera := rasterFit(b.triangles, nativeViewDirection(), 640, 480, 1.08)
			wallBottom := camera.project(nv(n.Position.X, n.Position.Y-n.Size.Y/2, n.Position.Z+n.Size.Z/2))
			captionY := b.triangles[len(b.triangles)-1].V[0].Position.Y
			captionTop := camera.project(nv(n.Position.X, captionY, minZ))
			if captionTop.y <= wallBottom.y+1 {
				t.Fatal("outside caption touches the raised body's projected front edge")
			}
		})
	}
}
