package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
	"github.com/d2lang/d2/d2target"
)

func vectorSurfaceFragment(t *testing.T, ctx context.Context, tex *image.RGBA) string {
	t.Helper()
	surface := nativeVectorForTexture(ctx, tex)
	if surface == nil {
		t.Fatal("generated surface has no retained vector content")
	}
	fragment, err := nativeSurfaceSVG(ctx, surface, "surface")
	if err != nil {
		t.Fatal(err)
	}
	again, err := nativeSurfaceSVG(ctx, surface, "surface")
	if err != nil || fragment != again {
		t.Fatalf("nondeterministic vector surface: %v", err)
	}
	decoder := xml.NewDecoder(strings.NewReader("<svg>" + fragment + "</svg>"))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return fragment
}

func rasterVectorFragment(t *testing.T, fragment string, size image.Rectangle) *image.NRGBA {
	t.Helper()
	source := `<svg xmlns="http://www.w3.org/2000/svg" width="` + nativeSVGNumber(float64(size.Dx())) + `" height="` + nativeSVGNumber(float64(size.Dy())) + `" viewBox="0 0 1 1">` + fragment + `</svg>`
	imported, err := d2svgimport.ImportNode(context.Background(), "retained.svg", []byte(source), d2svgimport.Limits{MaxBytes: 8 << 20, MaxDepth: 128, MaxElements: 100000, MaxAttributes: 500000, MaxAttributeBytes: 8 << 20, MaxPathCommands: 1000000, MaxTransformFunctions: 100000, MaxUseDepth: 32, MaxResources: 100000})
	if err != nil {
		t.Fatal(err)
	}
	doc := &d2scene.Document{Root: imported.Root, ViewBox: imported.ViewBox, LogicalWidth: float64(size.Dx()), LogicalHeight: float64(size.Dy()), Assets: imported.Assets}
	result, err := d2raster.Render(context.Background(), doc, richRasterOptions())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func vectorSurfacePixelError(t *testing.T, expected *image.RGBA, actual *image.NRGBA) float64 {
	t.Helper()
	if expected.Bounds() != actual.Bounds() {
		t.Fatalf("size changed: %v / %v", expected.Bounds(), actual.Bounds())
	}
	var errorSum, coverage float64
	for y := 0; y < expected.Bounds().Dy(); y++ {
		for x := 0; x < expected.Bounds().Dx(); x++ {
			a := expected.RGBAAt(x, y)
			c := color.RGBAModel.Convert(actual.NRGBAAt(x, y)).(color.RGBA)
			errorSum += math.Abs(float64(a.R)-float64(c.R)) + math.Abs(float64(a.G)-float64(c.G)) + math.Abs(float64(a.B)-float64(c.B)) + math.Abs(float64(a.A)-float64(c.A))
			coverage += float64(a.A) * 4
		}
	}
	return errorSum / max(1, coverage)
}

func TestNativeVectorTextRetainsCurvesAndTypography(t *testing.T) {
	for _, name := range []string{"regular", "bold-italic", "mono-underline"} {
		t.Run(name, func(t *testing.T) {
			ctx := nativeVectorContext(context.Background())
			p, err := newTextPainter(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			style := normalPrintStyle()
			style.Width, style.Depth = 4, 1
			style.FontSize = 32
			if name == "bold-italic" {
				style.Bold, style.Italic = true, true
			}
			if name == "mono-underline" {
				style.FontFamily, style.Underline = "MONO", true
			}
			texture, _, err := p.texture("Café e\u0301 → AV\nBuild()", style)
			if err != nil {
				t.Fatal(err)
			}
			fragment := vectorSurfaceFragment(t, ctx, texture)
			if strings.Contains(fragment, "<image") || strings.Contains(fragment, "<text") || !strings.Contains(fragment, "Q") && !strings.Contains(fragment, "C") {
				t.Fatal("text was not retained as font curves")
			}
			got := rasterVectorFragment(t, fragment, texture.Bounds())
			if difference := vectorSurfacePixelError(t, texture, got); difference > .035 {
				t.Fatalf("vector typography differs from original texture: %.4f", difference)
			}
		})
	}
}

func TestNativeVectorRichTextRetainsCurves(t *testing.T) {
	for _, language := range []string{"md", "go"} {
		t.Run(language, func(t *testing.T) {
			ctx := nativeVectorContext(context.Background())
			p, err := newRichLabelPainter(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			text := "# Build\n\n**Bold** and *italic* with `code`."
			if language == "go" {
				text = "func main() {\n  println(\"ready\")\n}"
			}
			shape := d2target.Shape{Text: d2target.Text{Label: text, Language: language, FontSize: 16, LabelWidth: 320, LabelHeight: 180}}
			style := richTestStyle()
			style.Width, style.Depth = 3.2, 1.8
			tex, err := p.texture(shape, style)
			if err != nil {
				t.Fatal(err)
			}
			fragment := vectorSurfaceFragment(t, ctx, tex)
			if strings.Contains(fragment, "<image") || strings.Contains(fragment, "<text") || strings.Count(fragment, "<path") < 10 {
				t.Fatal("rich label was not retained as shaped glyph paths")
			}
			got := rasterVectorFragment(t, fragment, tex.Bounds())
			if difference := vectorSurfacePixelError(t, tex, got); difference > .035 {
				t.Fatalf("vector rich typography differs from original texture: %.4f", difference)
			}
		})
	}
}

func TestNativeVectorColorEmojiStaysVector(t *testing.T) {
	ctx := nativeVectorContext(context.Background())
	p, _ := newTextPainter(ctx, 1)
	style := normalPrintStyle()
	style.Width, style.Depth, style.FontSize = 1, 1, 60
	texture, _, err := p.texture("🚀", style)
	if err != nil {
		t.Fatal(err)
	}
	fragment := vectorSurfaceFragment(t, ctx, texture)
	if strings.Contains(fragment, "<image") || strings.Contains(fragment, "<text") || strings.Count(fragment, "<path") < 5 {
		t.Fatal("emoji should retain its colored vector artwork")
	}
	if !strings.Contains(fragment, "clipPath") {
		t.Fatal("emoji paint graph lost its outline clips")
	}
}

func TestNativeVectorIconsPreserveArtworkAndFirstRasterFrame(t *testing.T) {
	ctx := nativeVectorContext(context.Background())
	p, err := newSurfaceIconPainter(ctx, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	tex, err := p.texture(iconData(t, "image/svg+xml", []byte(surfaceTestSVG)), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	fragment := vectorSurfaceFragment(t, ctx, tex)
	if strings.Contains(fragment, "<image") {
		t.Fatal("SVG icon became a raster image")
	}
	got := rasterVectorFragment(t, fragment, tex.Bounds())
	if difference := vectorSurfacePixelError(t, tex, got); difference > .01 {
		t.Fatalf("vector icon aspect or alpha changed: %.4f", difference)
	}
	palette := color.Palette{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	a, b := image.NewPaletted(image.Rect(0, 0, 2, 1), palette), image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	b.Pix[0], b.Pix[1] = 1, 1
	var data bytes.Buffer
	if err := gif.EncodeAll(&data, &gif.GIF{Image: []*image.Paletted{a, b}, Delay: []int{1, 1}, LoopCount: 0}); err != nil {
		t.Fatal(err)
	}
	tex, err = p.texture(iconData(t, "image/gif", data.Bytes()), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	fragment = vectorSurfaceFragment(t, ctx, tex)
	if strings.Count(fragment, "<image") != 1 || !strings.Contains(fragment, "data:image/png;base64,") || strings.Contains(fragment, "data:image/gif") {
		t.Fatal("authored animated raster should embed its static first frame")
	}
}

func TestNativeVectorCaptureIsScopedAndCancelable(t *testing.T) {
	ctx := nativeVectorContext(nil)
	doc := &d2scene.Document{Root: d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 2, Height: 1}, Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 255}}}), ViewBox: d2scene.Box{Width: 2, Height: 1}, LogicalWidth: 2, LogicalHeight: 1}
	tex := image.NewRGBA(image.Rect(0, 0, 2, 1))
	if err := retainNativeVectorSurface(context.Background(), tex, doc); err != nil {
		t.Fatal(err)
	}
	if nativeVectorForTexture(ctx, tex) != nil {
		t.Fatal("capture leaked between exports")
	}
	if err := retainNativeVectorSurface(ctx, tex, doc); err != nil {
		t.Fatal(err)
	}
	if nativeVectorForTexture(nativeVectorContext(ctx), tex) != nil {
		t.Fatal("nested export reused mutable registry")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := nativeSurfaceSVG(canceled, nativeVectorForTexture(ctx, tex), "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := nativeSurfaceSVG(ctx, nativeVectorForTexture(ctx, tex), `invalid"prefix`); err == nil {
		t.Fatal("invalid resource prefix accepted")
	}
}

func TestNativeVectorBorderAperturePreservesSourceOpening(t *testing.T) {
	ctx := nativeVectorContext(context.Background())
	source := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 100, Height: 50}, Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 255}}})
	doc := &d2scene.Document{Root: source, ViewBox: d2scene.Box{Width: 100, Height: 50}, LogicalWidth: 100, LogicalHeight: 50}
	tex, err := rasterNativeSurfaceDocument(ctx, doc, 100, 50)
	if err != nil {
		t.Fatal(err)
	}
	surface := nativeVectorForTexture(ctx, tex)
	nativeVectorAperture(surface, d2scene.Box{X: 35, Y: 0, Width: 30, Height: 5})
	fragment := vectorSurfaceFragment(t, ctx, tex)
	got := rasterVectorFragment(t, fragment, tex.Bounds())
	if got.NRGBAAt(50, 2).A != 0 || got.NRGBAAt(20, 2).A != 255 || got.NRGBAAt(50, 10).A != 255 {
		t.Fatal("border opening differs from authored aperture")
	}
	if doc.Root != source || doc.Root.Clip != nil {
		t.Fatal("retained aperture mutated source document")
	}
}

func TestNativeVectorInitialAnimationPreservesStaticFrame(t *testing.T) {
	ctx := nativeVectorContext(context.Background())
	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 5, Y: 5, Width: 20, Height: 10}, Fill: d2scene.SolidPaint{Color: color.NRGBA{B: 255, A: 255}}})
	node.Opacity = 0
	node.Animations = []d2scene.Track{
		{Property: d2scene.AnimateOpacity, Duration: time.Second, Keyframes: []d2scene.Keyframe{{Offset: 0, Value: d2scene.NumberValue(.6)}, {Offset: 1, Value: d2scene.NumberValue(1)}}},
		{Property: d2scene.AnimateFillColor, Duration: time.Second, Keyframes: []d2scene.Keyframe{{Offset: 0, Value: d2scene.ColorValue(color.NRGBA{R: 255, A: 255})}, {Offset: 1, Value: d2scene.ColorValue(color.NRGBA{G: 255, A: 255})}}},
	}
	doc := &d2scene.Document{Root: node, ViewBox: d2scene.Box{Width: 30, Height: 20}, LogicalWidth: 30, LogicalHeight: 20}
	texture, err := rasterNativeSurfaceDocument(ctx, doc, 120, 80)
	if err != nil {
		t.Fatal(err)
	}
	fragment := vectorSurfaceFragment(t, ctx, texture)
	if strings.Contains(fragment, "<animate") {
		t.Fatal("static export contains animation")
	}
	got := rasterVectorFragment(t, fragment, texture.Bounds())
	if difference := vectorSurfacePixelError(t, texture, got); difference > .001 {
		t.Fatalf("time-zero source paint changed: %.4f", difference)
	}
	if node.Opacity != 0 || node.Primitive.(d2scene.Rect).Fill.(d2scene.SolidPaint).Color.B != 255 {
		t.Fatal("initial frame mutated source animation")
	}
}

func TestNativeVectorSplitPaintCoverageAppliesOpacityOnce(t *testing.T) {
	for _, opacity := range []float64{.1, .5, .95} {
		t.Run(nativeSVGNumber(opacity), func(t *testing.T) {
			ctx := nativeVectorContext(context.Background())
			doc := &d2scene.Document{Root: d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: d2scene.SolidPaint{Color: color.NRGBA{A: 255}}}), ViewBox: d2scene.Box{Width: 1, Height: 1}, LogicalWidth: 1, LogicalHeight: 1}
			fill, ink := image.NewRGBA(image.Rect(0, 0, 1, 1)), image.NewRGBA(image.Rect(0, 0, 1, 1))
			fill.Pix[3], ink.Pix[3] = 255, 128
			if err := retainNativeVectorSurface(ctx, fill, doc); err != nil {
				t.Fatal(err)
			}
			if err := retainNativeVectorSurface(ctx, ink, doc); err != nil {
				t.Fatal(err)
			}
			builder := meshBuilder{ctx: ctx}
			builder.nativeFaceOpacity(fill, ink, opacity)
			// A solid cap's opaque substrate must remain inside this same
			// compensation mask; source opacity still composites exactly once.
			surface := nativeVectorSolidCap(nativeVectorForTexture(ctx, fill), color.NRGBA{A: 255})
			fragment, err := nativeSurfaceSVG(ctx, surface, "surface")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(fragment, `><rect width="1" height="1" fill="#000000"/><g transform=`) {
				t.Fatal("cap substrate is not inside the compensated source paint")
			}
			decoder := xml.NewDecoder(strings.NewReader("<svg>" + fragment + "</svg>"))
			var values []string
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				element, ok := token.(xml.StartElement)
				if !ok || element.Name.Local != "feFuncA" {
					continue
				}
				for _, attr := range element.Attr {
					if attr.Name.Local == "tableValues" {
						values = strings.Fields(attr.Value)
					}
				}
			}
			if len(values) != 256 {
				t.Fatalf("missing vector alpha compensation table: %d", len(values))
			}
			for i, value := range values {
				factor, err := strconv.ParseFloat(value, 64)
				if err != nil {
					t.Fatal(err)
				}
				inkCoverage := float64(i) / 255
				composited := opacity*inkCoverage + opacity*factor*(1-opacity*inkCoverage)
				if math.Abs(composited-opacity) > 1e-12 {
					t.Fatalf("split paint applies opacity twice at ink alpha %d: %g / %g", i, composited, opacity)
				}
			}
		})
	}
}

func TestNativeVectorCaptureCoversPhysicalAndStructuredSurfaces(t *testing.T) {
	for _, fixture := range []string{"stable/all_shapes/dagre/board.exp.json", "patterns/all_shapes/dagre/board.exp.json", "stable/class_and_sqlTable_border_radius/dagre/board.exp.json", "stable/sequence_diagram_groups/dagre/board.exp.json"} {
		t.Run(fixture, func(t *testing.T) {
			ctx := nativeVectorContext(context.Background())
			target := sourcePanelFixture(t, fixture)
			scene, err := d2isometric.BuildScene(target, &d2isometric.RenderOpts{})
			if err != nil {
				t.Fatal(err)
			}
			native, err := newNativeSceneWithOptions(ctx, scene, 640, 480, nil, nil, nativeSceneOptions{deferRaster: true, vector: true})
			if err != nil {
				t.Fatal(err)
			}
			surfaces := map[*nativeVectorSurface]bool{}
			for i, triangle := range native.triangles {
				m := triangle.Material
				if m == nil || m.Texture == nil {
					continue
				}
				if m.Vector == nil {
					t.Fatalf("surface triangle %d lost vector source", i)
				}
				if surfaces[m.Vector] {
					continue
				}
				surfaces[m.Vector] = true
				fragment, err := nativeSurfaceSVG(ctx, m.Vector, "fixture")
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(fragment, "<image") {
					t.Fatal("source fixture without raster assets generated embedded image")
				}
			}
			if len(surfaces) < 3 {
				t.Fatal("fixture omitted physical surface content")
			}
		})
	}
}

func TestNativeVectorColorGradientKeepsLinearLight(t *testing.T) {
	doc := &d2scene.Document{ViewBox: d2scene.Box{Width: 100, Height: 100}, LogicalWidth: 100, LogicalHeight: 100}
	writer := &nativeSurfaceSVGWriter{ctx: context.Background(), doc: doc, prefix: "emoji", transform: d2scene.Identity()}
	gradient := fontface.COLRv1LinearGradient{X0: 0, Y0: 0, X1: 100, Y1: 0, X2: 0, Y2: 100, ColorLine: fontface.COLRv1ColorLine{Stops: []fontface.COLRv1ColorStop{{Offset: 0, Color: color.NRGBA{A: 255}}, {Offset: 1, Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}}}}
	body := writer.colorPaint(nil, gradient, doc.ViewBox, d2scene.Identity(), 0)
	if writer.err != nil {
		t.Fatal(writer.err)
	}
	if !strings.Contains(writer.defs.String(), `color-interpolation="linearRGB"`) || !strings.Contains(body, `fill="url(#emoji-paint-1)"`) {
		t.Fatal("COLRv1 gradient no longer interpolates in linear light")
	}
}
