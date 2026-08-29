package d2raster_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/internal/testutil/imagediff"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/imageasset"
)

const updateTargetPixelGoldens = "D2_UPDATE_TARGET_PIXEL_GOLDENS"

func TestTargetShapePixels(t *testing.T) {
	// Keep the concrete constants in this executable test so adding a shape to
	// d2target.Shapes requires adding it to the final-pixel composite too.
	shapeValues := []string{
		d2target.ShapeRectangle,
		d2target.ShapeSquare,
		d2target.ShapePage,
		d2target.ShapeParallelogram,
		d2target.ShapeDocument,
		d2target.ShapeCylinder,
		d2target.ShapeQueue,
		d2target.ShapePackage,
		d2target.ShapeStep,
		d2target.ShapeCallout,
		d2target.ShapeStoredData,
		d2target.ShapePerson,
		d2target.ShapeC4Person,
		d2target.ShapeDiamond,
		d2target.ShapeOval,
		d2target.ShapeCircle,
		d2target.ShapeHexagon,
		d2target.ShapeCloud,
		d2target.ShapeText,
		d2target.ShapeCode,
		d2target.ShapeClass,
		d2target.ShapeSQLTable,
		d2target.ShapeImage,
		d2target.ShapeSequenceDiagram,
		d2target.ShapeHierarchy,
	}
	if !reflect.DeepEqual(shapeValues, d2target.Shapes) {
		t.Fatalf("pixel-covered shapes = %#v, want d2target.Shapes %#v", shapeValues, d2target.Shapes)
	}

	diagram := targetShapeDiagram(t, shapeValues)
	document, actual, frame := renderTargetPixels(t, diagram, targetAssetOptions(t))
	for index, shape := range diagram.Shapes {
		minimum := 100
		if shape.Type == d2target.ShapeText {
			minimum = 10
		}
		assertTargetRegionInk(t, frame, document.ViewBox.X, document.ViewBox.Y, image.Rect(
			shape.Pos.X, shape.Pos.Y, shape.Pos.X+shape.Width, shape.Pos.Y+shape.Height,
		), minimum, fmt.Sprintf("shape[%d] %q", index, shape.Type))
	}
	assertTargetPixelGolden(t, "target-shapes", actual)
}

func TestTargetArrowheadPixels(t *testing.T) {
	// DefaultArrowhead aliases TriangleArrowhead and is deliberately absent.
	// Every distinct final-pixel marker, including the derived filled variants
	// and the fat-arrow line marker, is named explicitly.
	arrowheadValues := []d2target.Arrowhead{
		d2target.NoArrowhead,
		d2target.ArrowArrowhead,
		d2target.UnfilledTriangleArrowhead,
		d2target.TriangleArrowhead,
		d2target.DiamondArrowhead,
		d2target.FilledDiamondArrowhead,
		d2target.CircleArrowhead,
		d2target.FilledCircleArrowhead,
		d2target.CrossArrowhead,
		d2target.BoxArrowhead,
		d2target.FilledBoxArrowhead,
		d2target.LineArrowhead,
		d2target.CfOne,
		d2target.CfMany,
		d2target.CfOneRequired,
		d2target.CfManyRequired,
	}
	assertTargetArrowheadCoverage(t, arrowheadValues)

	diagram := targetArrowheadDiagram(arrowheadValues)
	document, actual, frame := renderTargetPixels(t, diagram, nil)
	for index, arrowhead := range arrowheadValues {
		y := 36 + index*46
		assertTargetRegionInk(t, frame, document.ViewBox.X, document.ViewBox.Y,
			image.Rect(152, y-18, 668, y+19), 100, fmt.Sprintf("arrowhead[%d] %q", index, arrowhead))
	}
	assertTargetPixelGolden(t, "target-arrowheads", actual)
}

func assertTargetArrowheadCoverage(t *testing.T, values []d2target.Arrowhead) {
	t.Helper()
	want := make(map[d2target.Arrowhead]bool, len(d2target.Arrowheads)+5)
	for value := range d2target.Arrowheads {
		want[d2target.Arrowhead(value)] = true
	}
	for _, value := range []d2target.Arrowhead{
		d2target.UnfilledTriangleArrowhead,
		d2target.FilledDiamondArrowhead,
		d2target.FilledCircleArrowhead,
		d2target.FilledBoxArrowhead,
		d2target.LineArrowhead,
	} {
		want[value] = true
	}
	got := make(map[d2target.Arrowhead]bool, len(values))
	for _, value := range values {
		if got[value] {
			t.Fatalf("duplicate pixel-covered arrowhead %q", value)
		}
		got[value] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pixel-covered arrowheads = %#v, want %#v", got, want)
	}
}

func targetShapeDiagram(t *testing.T, values []string) *d2target.Diagram {
	t.Helper()
	diagram := d2target.NewDiagram()
	diagram.Root.Fill, diagram.Root.Stroke = "#ffffff", "none"
	fontFamily, monoFontFamily := d2fonts.SourceSansPro, d2fonts.SourceCodePro
	diagram.FontFamily, diagram.MonoFontFamily = &fontFamily, &monoFontFamily
	assetURL := targetAssetURL(t)
	palette := []string{"#e0f2fe", "#dcfce7", "#fef3c7", "#fae8ff", "#ffe4e6"}
	const (
		columns     = 5
		cellWidth   = 184
		cellHeight  = 142
		shapeWidth  = 156
		shapeHeight = 108
	)
	for index, shapeType := range values {
		column, row := index%columns, index/columns
		shape := d2target.Shape{
			ID: fmt.Sprintf("shape-%02d-%s", index, shapeType), Type: shapeType,
			Pos:   d2target.Point{X: 18 + column*cellWidth, Y: 18 + row*cellHeight},
			Width: shapeWidth, Height: shapeHeight,
			Fill: palette[index%len(palette)], Stroke: "#1f2937", StrokeWidth: 2, Opacity: 1,
			BorderRadius: 10,
			Text: d2target.Text{
				Label: shapeType, FontSize: 14, FontFamily: "DEFAULT", Color: "#111827",
				LabelWidth: len(shapeType) * 8, LabelHeight: 18,
			},
			LabelPosition:      "INSIDE_MIDDLE_CENTER",
			PrimaryAccentColor: "#be123c", SecondaryAccentColor: "#1d4ed8", NeutralAccentColor: "#475569",
		}
		if shapeType == d2target.ShapeSquare || shapeType == d2target.ShapeCircle {
			shape.Pos.X += (shapeWidth - shapeHeight) / 2
			shape.Width = shapeHeight
		}
		switch shapeType {
		case d2target.ShapeCode:
			shape.Text = d2target.Text{
				Label: "package main\nfunc main() {}", Language: "go", FontSize: 12, FontFamily: "mono",
				LabelWidth: 140, LabelHeight: 34,
			}
		case d2target.ShapeClass:
			shape.Fill, shape.Stroke = "#1d4ed8", "#eff6ff"
			shape.Label, shape.LabelWidth, shape.LabelHeight = "Widget", 48, 18
			shape.Fields = []d2target.ClassField{{Name: "id", Type: "int", Visibility: "private"}}
			shape.Methods = []d2target.ClassMethod{{Name: "save()", Return: "error", Visibility: "public"}}
		case d2target.ShapeSQLTable:
			shape.Fill, shape.Stroke = "#047857", "#ecfdf5"
			shape.Label, shape.LabelWidth, shape.LabelHeight = "users", 38, 18
			shape.Columns = []d2target.SQLColumn{{
				Name: d2target.Text{Label: "id", LabelWidth: 16}, Type: d2target.Text{Label: "uuid", LabelWidth: 30},
				Constraint: []string{"primary_key"},
			}}
		case d2target.ShapeImage:
			shape.Icon, shape.Fill, shape.Stroke = assetURL, "none", "none"
			shape.Label, shape.LabelWidth, shape.LabelHeight = "", 0, 0
		}
		diagram.Shapes = append(diagram.Shapes, shape)
	}
	return diagram
}

func targetArrowheadDiagram(values []d2target.Arrowhead) *d2target.Diagram {
	diagram := d2target.NewDiagram()
	diagram.Root.Fill, diagram.Root.Stroke = "#ffffff", "none"
	fontFamily := d2fonts.SourceSansPro
	diagram.FontFamily = &fontFamily
	// This transparent target establishes a stable view box without adding
	// painted geometry to any marker row.
	diagram.Shapes = []d2target.Shape{{
		ID: "coverage-bounds", Type: "rectangle", Pos: d2target.Point{X: 120, Y: 10},
		Width: 580, Height: 46 * len(values), Opacity: 0,
	}}
	for index, arrowhead := range values {
		y := 36 + index*46
		diagram.Connections = append(diagram.Connections, d2target.Connection{
			ID: fmt.Sprintf("arrowhead-%02d-%s", index, arrowhead), Src: "coverage-bounds", Dst: "coverage-bounds",
			Route:    []*geo.Point{{X: 170, Y: float64(y)}, {X: 650, Y: float64(y)}},
			SrcArrow: arrowhead, DstArrow: arrowhead,
			Stroke: "#111827", Fill: "#ffffff", StrokeWidth: 3, Opacity: 1,
			Text: d2target.Text{
				Label: fmt.Sprintf("%02d  %s", index, arrowhead), FontSize: 13, FontFamily: "DEFAULT", Color: "#7c3aed",
				LabelWidth: 150, LabelHeight: 17, LabelFill: "#ffffff",
			},
			LabelPosition: "INSIDE_MIDDLE_CENTER", LabelPercentage: 0.5,
		})
	}
	return diagram
}

func renderTargetPixels(t *testing.T, diagram *d2target.Diagram, assets *d2scenebuild.AssetOptions) (*d2scene.Document, []byte, *image.NRGBA) {
	t.Helper()
	pad := int64(14)
	document, err := d2scenebuild.Build(context.Background(), diagram, d2scenebuild.Options{Pad: &pad, Assets: assets})
	if err != nil {
		t.Fatalf("build target coverage scene: %v", err)
	}
	options := d2raster.FrameOptions{
		Scale: 1, Background: color.White,
		MaxWidth: 2_000, MaxHeight: 2_000, MaxPixels: 4_000_000,
		MaxNodes: 100_000, MaxDepth: 1_000, MaxPathCommands: 1_000_000, MaxTextRunesPerRun: 100_000,
		MaxFontFacesPerText: 16, MaxTextCoverageChecks: 1_000_000, MaxTextShapingRuns: 10_000,
		MaxAnimationTracks: 1_000, MaxAnimationKeyframes: 10_000,
		MaxAssets: 100, MaxAssetBytes: 64 << 20, MaxDecodedAssetBytes: 64 << 20, MaxImportDepth: 64,
		MaxOffscreenBytes: 64 << 20, MaxEvenOddClipWork: 1_000_000_000,
	}
	first, err := d2raster.Render(context.Background(), document, options)
	if err != nil {
		t.Fatalf("render target coverage pixels: %v", err)
	}
	actual, err := d2raster.EncodePNG(context.Background(), first)
	if err != nil {
		t.Fatalf("encode target coverage PNG: %v", err)
	}
	secondFrame, err := d2raster.Render(context.Background(), document, options)
	if err != nil {
		t.Fatalf("repeat target coverage render: %v", err)
	}
	second, err := d2raster.EncodePNG(context.Background(), secondFrame)
	if err != nil {
		t.Fatalf("encode repeated target coverage PNG: %v", err)
	}
	if !bytes.Equal(actual, second) {
		t.Fatal("repeated target coverage render changed deterministic PNG bytes")
	}
	return document, actual, first
}

func assertTargetRegionInk(t *testing.T, frame *image.NRGBA, viewX, viewY float64, logical image.Rectangle, minimum int, name string) {
	t.Helper()
	region := image.Rect(
		int(float64(logical.Min.X)-viewX), int(float64(logical.Min.Y)-viewY),
		int(float64(logical.Max.X)-viewX), int(float64(logical.Max.Y)-viewY),
	).Intersect(frame.Bounds())
	ink := 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			pixel := frame.NRGBAAt(x, y)
			if pixel != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
				ink++
			}
		}
	}
	if ink < minimum {
		t.Errorf("%s final-pixel region has %d non-background pixels, want at least %d", name, ink, minimum)
	}
}

func assertTargetPixelGolden(t *testing.T, name string, actual []byte) {
	t.Helper()
	expectedPath := filepath.Join("testdata", name+".exp.png")
	if os.Getenv(updateTargetPixelGoldens) == "1" {
		if err := os.MkdirAll(filepath.Dir(expectedPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(expectedPath, actual, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s: sha256:%x", expectedPath, sha256.Sum256(actual))
		return
	}
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected target golden %q: %v", expectedPath, err)
	}
	result, compareErr := imagediff.Compare(expected, actual, imagediff.Options{
		ExpectedName: "expected target golden", ActualName: "rendered target pixels",
	})
	if compareErr == nil {
		return
	}
	actualPath := filepath.Join("testdata", name+".got.png")
	reportPath := filepath.Join("testdata", name+".got.diff.html")
	if writeErr := os.WriteFile(actualPath, actual, 0o644); writeErr != nil {
		t.Fatalf("%v; also could not write actual pixels: %v", compareErr, writeErr)
	}
	if result == nil {
		t.Fatalf("target golden differs: %v; actual pixels: %s", compareErr, actualPath)
	}
	if writeErr := result.WriteReport(reportPath); writeErr != nil {
		t.Fatalf("%v; actual pixels: %s; could not write diff report: %v", compareErr, actualPath, writeErr)
	}
	t.Fatalf("target golden differs: %v; actual pixels: %s; self-contained diff: %s", compareErr, actualPath, reportPath)
}

func targetAssetURL(t *testing.T) *url.URL {
	t.Helper()
	asset := image.NewNRGBA(image.Rect(0, 0, 16, 12))
	for y := 0; y < asset.Bounds().Dy(); y++ {
		for x := 0; x < asset.Bounds().Dx(); x++ {
			asset.SetNRGBA(x, y, color.NRGBA{R: uint8(32 + x*12), G: uint8(48 + y*15), B: uint8(220 - x*8), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, asset); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse("data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func targetAssetOptions(t *testing.T) *d2scenebuild.AssetOptions {
	t.Helper()
	resolver, err := imageasset.New(imageasset.Options{Limits: imageasset.Limits{
		MaxFetchedBytes: 1 << 20, MaxEncodedBytes: 1 << 20, MaxDecompressedBytes: 1 << 20, MaxSVGBytes: 1 << 20,
		MaxDecodedWidth: 1_024, MaxDecodedHeight: 1_024, MaxDecodedPixels: 1 << 20,
		MaxAssets: 8, MaxCumulativeEncodedBytes: 8 << 20, MaxCumulativeDecodedBytes: 16 << 20,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return &d2scenebuild.AssetOptions{
		Resolver: resolver,
		SVGImportLimits: d2svgimport.Limits{
			MaxBytes: 1 << 20, MaxDepth: 128, MaxElements: 10_000, MaxAttributes: 20_000,
			MaxAttributeBytes: 1 << 20, MaxPathCommands: 100_000, MaxTransformFunctions: 10_000,
			MaxUseDepth: 64, MaxResources: 10_000,
		},
		SVGImportBudget: d2scenebuild.SVGImportBudget{
			MaxSourceBytes: 2 << 20, MaxElements: 20_000, MaxAttributes: 40_000, MaxAttributeBytes: 2 << 20,
			MaxPathCommands: 200_000, MaxTransformFunctions: 20_000,
			MaxDeclaredResources: 20_000, MaxExpandedUseInstances: 20_000,
		},
	}
}
