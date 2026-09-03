package d2scenebuild

import (
	"context"
	"errors"
	"image/color"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/textmeasure"
)

func TestBuildMarkdownShapeUsesPrimitivesFontsThemeAndLinks(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "markdown", Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: 10, Y: 20}, Width: 320, Height: 260,
		Fill: "N7", Stroke: "N1", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{
			Label:    "# Heading\n\n[relative docs](root.html \"Relative documentation\")\n\nParagraph with **bold**, *italic*, `code`, and [docs](https://example.com \"Documentation\").\n\n> quote\n\n- item\n\n| A | B |\n| - | - |\n| 1 | 2 |",
			Language: "markdown", FontFamily: "default", FontSize: 16,
			LabelWidth: 280, LabelHeight: 220, Underline: true,
		},
	}}
	document, err := Build(context.Background(), diagram, Options{LinkBudget: LinkBudget{MaxRegions: 100, MaxStringBytes: 64 * 1024}})
	if err != nil {
		t.Fatal(err)
	}
	group := findSceneNode(t, document.Root, "markdown:markdown")
	if group.Clip == nil || group.Transform != d2scene.Translate(30, 40) {
		t.Fatalf("Markdown viewport clip/placement = clip:%v transform:%+v, want clipped translate(30,40)", group.Clip != nil, group.Transform)
	}
	if len(group.Children) < 10 || group.Children[0].ID != "markdown:markdown:background" {
		t.Fatalf("Markdown children = %d first=%q, want background followed by scene primitives", len(group.Children), group.Children[0].ID)
	}

	primitiveKinds := make(map[string]bool)
	fontAssets := make(map[d2scene.AssetID]bool)
	underlined := 0
	for _, child := range group.Children[1:] {
		switch primitive := child.Primitive.(type) {
		case d2scene.Rect:
			primitiveKinds["rect"] = true
		case d2scene.Path:
			primitiveKinds["line"] = true
		case d2scene.TextRun:
			primitiveKinds["text"] = true
			fontAssets[primitive.Font.Asset] = true
			if primitive.Underline {
				underlined++
			}
		}
	}
	for _, kind := range []string{"rect", "text"} {
		if !primitiveKinds[kind] {
			t.Errorf("Markdown scene omitted %s primitive", kind)
		}
	}
	if len(fontAssets) < 4 {
		t.Fatalf("Markdown font assets = %v, want regular/bold/italic/mono roles", fontAssets)
	}
	if underlined == 0 {
		t.Fatal("shape underline was not applied to Markdown text primitives")
	}
	for id := range fontAssets {
		if asset, ok := document.Assets[id].(d2scene.FontAsset); !ok || len(asset.Data) == 0 {
			t.Fatalf("Markdown font asset %q = %#v", id, document.Assets[id])
		}
	}
	if len(document.Links) == 0 {
		t.Fatal("Markdown inline link did not become typed link metadata")
	}
	foundLink := false
	foundRelativeLink := false
	for _, region := range document.Links {
		if region.URL == "https://example.com" && region.Tooltip == "Documentation" {
			foundLink = true
			if region.Box.X < 30 || region.Box.Y < 40 || region.Box.X+region.Box.Width > 310 || region.Box.Y+region.Box.Height > 260 {
				t.Fatalf("Markdown link region escaped clipped viewport: %+v", region.Box)
			}
		}
		if region.URL == "root.html" && region.Target == "" && region.Tooltip == "Relative documentation" {
			foundRelativeLink = true
		}
	}
	if !foundLink {
		t.Fatalf("typed Markdown links = %+v, want example.com with title", document.Links)
	}
	if !foundRelativeLink {
		t.Fatalf("typed Markdown links = %+v, want root.html preserved as a relative URL", document.Links)
	}
}

func TestBuildMarkdownObjectLinkSuppressesNestedLinks(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "markdown", Type: d2target.ShapeRectangle,
		Width: 180, Height: 80, Fill: "N7", Stroke: "N1", StrokeWidth: 2, Opacity: 1,
		Link: "https://outer.example",
		Text: d2target.Text{
			Label: "[inner](https://inner.example)", Language: "markdown", FontSize: 16,
			LabelWidth: 160, LabelHeight: 50,
		},
	}}
	document, err := Build(context.Background(), diagram, Options{LinkBudget: LinkBudget{MaxRegions: 10, MaxStringBytes: 1024}})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Links) != 1 || document.Links[0].URL != "https://outer.example" {
		t.Fatalf("typed links = %+v, want only outer shape link", document.Links)
	}
}

func TestBuildMarkdownConnectionDoesNotEmitOrdinaryLabelFill(t *testing.T) {
	diagram := validDiagram()
	connection := &diagram.Connections[0]
	connection.Text = d2target.Text{
		Label: "**raster** `markdown`", Language: "markdown", FontSize: 16,
		LabelWidth: 160, LabelHeight: 45,
	}
	connection.LabelPosition = "INSIDE_MIDDLE_CENTER"
	connection.LabelPercentage = .5
	connection.Fill = "N7"
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	group := findSceneNode(t, document.Root, "a-b:markdown")
	if group.Clip == nil || len(group.Children) < 2 {
		t.Fatalf("connection Markdown group = %#v", group)
	}
	if findOptionalSceneNode(document.Root, "a-b:label-fill") != nil {
		t.Fatal("Markdown connection incorrectly emitted the ordinary rounded label fill")
	}
}

func TestBuildMarkdownRejectsUnsafeLinksWithoutInteractiveMetadata(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "unsafe", Type: d2target.ShapeRectangle,
		Width: 180, Height: 80, Fill: "N7", Stroke: "N1", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{
			Label: "[unsafe](javascript:alert(1))", Language: "markdown", FontSize: 16,
			LabelWidth: 160, LabelHeight: 50,
		},
	}}
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Links) != 0 {
		t.Fatalf("unsafe Markdown links became metadata: %+v", document.Links)
	}
}

func TestBuildMarkdownBoundsAndCancellation(t *testing.T) {
	makeDiagram := func(label string) *d2target.Diagram {
		diagram := d2target.NewDiagram()
		diagram.Shapes = []d2target.Shape{{
			ID: "markdown", Type: d2target.ShapeRectangle,
			Width: 180, Height: 80, Fill: "N7", Stroke: "N1", StrokeWidth: 2, Opacity: 1,
			Text: d2target.Text{Label: label, Language: "markdown", FontSize: 16, LabelWidth: 160, LabelHeight: 50},
		}}
		return diagram
	}
	_, err := Build(context.Background(), makeDiagram(strings.Repeat("x", maxMarkdownSourceBytes+1)), Options{})
	if err == nil || !strings.Contains(err.Error(), "Markdown source bytes") {
		t.Fatalf("oversized Markdown error = %v", err)
	}
	_, err = Build(context.Background(), makeDiagram(string([]byte{'x', 0xff})), Options{})
	if err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("invalid UTF-8 Markdown error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Build(canceled, makeDiagram("text"), Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Markdown Build() error = %v", err)
	}
}

func TestBuildMarkdownPreservesEmptyZeroSizedViewport(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "empty", Type: d2target.ShapeRectangle,
		Width: 100, Height: 60, Fill: "N7", Stroke: "N1", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{Label: " \n\n", Language: "markdown", LabelWidth: 0, LabelHeight: 0},
	}}
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	group := findSceneNode(t, document.Root, "empty:markdown")
	if group.Clip != nil || len(group.Children) != 0 {
		t.Fatalf("empty Markdown viewport = clip:%v children:%d, want non-painting group", group.Clip != nil, len(group.Children))
	}
	diagram.Shapes[0].Label = `<video src="movie.mp4"></video>`
	_, err = Build(context.Background(), diagram, Options{})
	if err == nil || !strings.Contains(err.Error(), "does not support HTML element <video>") {
		t.Fatalf("zero-sized raw HTML error = %v", err)
	}
}

func TestBuildMarkdownRendersPixels(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Root.Fill = "transparent"
	diagram.Root.Stroke = "none"
	diagram.Shapes = []d2target.Shape{{
		ID: "markdown", Type: d2target.ShapeText,
		Width: 180, Height: 80, Fill: "none", Stroke: "none", Opacity: 1,
		Text: d2target.Text{
			Label: "# Raster\n\n**Go** renderer", Language: "markdown", FontSize: 16,
			LabelWidth: 180, LabelHeight: 80, Color: "#0055cc",
		},
	}}
	pad := int64(2)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := d2raster.Render(context.Background(), document, markdownRasterOptions())
	if err != nil {
		t.Fatal(err)
	}
	painted, blue := 0, 0
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			pixel := frame.NRGBAAt(x, y)
			if pixel.A == 0 {
				continue
			}
			painted++
			if pixel.B > pixel.G && pixel.G > pixel.R {
				blue++
			}
		}
	}
	if painted == 0 || blue == 0 {
		t.Fatalf("raster Markdown frame painted=%d blue=%d bounds=%v", painted, blue, frame.Bounds())
	}
}

func TestBuildMarkdownPrimitiveCoversLineAndSyntheticText(t *testing.T) {
	diagram := d2target.NewDiagram()
	b := &builder{ctx: context.Background(), diagram: diagram, assets: make(map[d2scene.AssetID]d2scene.Asset)}
	black := d2scene.SolidPaint{Color: color.NRGBA{A: 0xff}}
	paints := map[textmeasure.MarkdownColorRole]d2scene.Paint{
		textmeasure.MarkdownColorForeground:       black,
		textmeasure.MarkdownColorForegroundStroke: black,
	}
	lineNode, lineBounds, err := b.buildMarkdownPrimitive("md", 0, textmeasure.MarkdownPrimitive{
		Kind: textmeasure.MarkdownLinePrimitive,
		X:    1, Y: 2, X2: 11, Y2: 2, StrokeWidth: 2,
		StrokeRole: textmeasure.MarkdownColorForegroundStroke,
	}, d2fonts.SourceSansPro, d2fonts.SourceCodePro, paints, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lineNode.Primitive.(d2scene.Path); !ok || !lineBounds.Valid {
		t.Fatalf("line primitive = %T bounds=%+v", lineNode.Primitive, lineBounds)
	}
	textNode, textBounds, err := b.buildMarkdownPrimitive("md", 1, textmeasure.MarkdownPrimitive{
		Kind: textmeasure.MarkdownTextPrimitive,
		X:    5, Y: 20, Width: 6, Height: 12,
		Text: "-", Font: textmeasure.MarkdownFontItalic, FontSize: 16,
		FillRole:      textmeasure.MarkdownColorForeground,
		SyntheticBold: true, SyntheticItalic: true, TextLength: true,
	}, d2fonts.SourceSansPro, d2fonts.SourceCodePro, paints, true)
	if err != nil {
		t.Fatal(err)
	}
	run, ok := textNode.Primitive.(d2scene.TextRun)
	if !ok || run.Stroke == nil || !run.Underline || textNode.Transform == d2scene.Identity() || !textBounds.Valid {
		t.Fatalf("synthetic text primitive = %#v transform=%+v bounds=%+v", textNode.Primitive, textNode.Transform, textBounds)
	}
}

func markdownRasterOptions() d2raster.FrameOptions {
	return d2raster.FrameOptions{
		Scale:    3,
		MaxWidth: 2_000, MaxHeight: 2_000, MaxPixels: 4_000_000,
		MaxNodes: 100_000, MaxDepth: 100, MaxPathCommands: 2_000_000,
		MaxAnimationTracks: 10_000, MaxAnimationKeyframes: 100_000,
		MaxAssets: 100, MaxAssetBytes: 64 * 1024 * 1024,
		MaxDecodedAssetBytes: 64 * 1024 * 1024, MaxImportDepth: 100,
		MaxOffscreenBytes: 64 * 1024 * 1024, MaxEvenOddClipWork: 1_000_000_000,
	}
}
