package d2scenebuild

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestAppendixPaintOrderNumbersAndGeometry(t *testing.T) {
	t.Parallel()
	diagram := appendixTestDiagram()
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{
		Pad: &pad, Appendix: true,
		LinkBudget: LinkBudget{MaxRegions: 2, MaxStringBytes: 4_096},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{
		"root:background", "second", "first",
		"appendix:shape:1:tooltip", "appendix:shape:1:link", "appendix:shape:0:link",
		"appendix",
	}
	if len(document.Root.Children) != len(wantOrder) {
		t.Fatalf("root children = %d, want %d", len(document.Root.Children), len(wantOrder))
	}
	for index, want := range wantOrder {
		if got := document.Root.Children[index].ID; got != want {
			t.Fatalf("root child %d = %q, want %q", index, got, want)
		}
	}

	for index, want := range []string{"2", "3", "1"} {
		icon := document.Root.Children[3+index]
		run, ok := icon.Children[1].Primitive.(d2scene.TextRun)
		if !ok || run.Text != want {
			t.Fatalf("icon %q number = %#v, want %q", icon.ID, icon.Children[1].Primitive, want)
		}
		if len(icon.Filters) != 1 {
			t.Fatalf("icon %q filters = %#v, want one drop shadow", icon.ID, icon.Filters)
		}
		shadow, ok := icon.Filters[0].(d2scene.DropShadow)
		if !ok || shadow.SigmaX != 32 || shadow.SigmaY != 32 || shadow.Color != (color.NRGBA{R: 31, G: 36, B: 58, A: 26}) {
			t.Fatalf("icon %q shadow = %#v", icon.ID, icon.Filters[0])
		}
	}

	appendix := document.Root.Children[len(document.Root.Children)-1]
	wantAppendixOrder := []string{"appendix:separator", "appendix:row:1", "appendix:row:2", "appendix:row:3"}
	if len(appendix.Children) != len(wantAppendixOrder) {
		t.Fatalf("appendix children = %d, want %d", len(appendix.Children), len(wantAppendixOrder))
	}
	for index, want := range wantAppendixOrder {
		if got := appendix.Children[index].ID; got != want {
			t.Fatalf("appendix child %d = %q, want %q", index, got, want)
		}
	}
	wantRows := []string{"https://example.com/first", "second tooltip\nline two", "https://example.com/second"}
	for index, want := range wantRows {
		row := appendix.Children[index+1]
		var got []string
		for _, child := range row.Children[1:] {
			got = append(got, child.Primitive.(d2scene.TextRun).Text)
		}
		if strings.Join(got, "\n") != want {
			t.Fatalf("row %d text = %q, want %q", index+1, strings.Join(got, "\n"), want)
		}
	}

	secondTooltip, secondLink, err := appendixIconCenters(diagram.Shapes[1])
	if err != nil {
		t.Fatal(err)
	}
	firstTooltip, firstLink, err := appendixIconCenters(diagram.Shapes[0])
	if err != nil {
		t.Fatal(err)
	}
	_ = firstTooltip
	wantCenters := []d2scene.Point{*secondTooltip, *secondLink, *firstLink}
	for index, want := range wantCenters {
		circle := document.Root.Children[3+index].Children[0].Primitive.(d2scene.Ellipse)
		if circle.Center != want {
			t.Fatalf("icon %d center = %+v, want %+v", index, circle.Center, want)
		}
	}
}

func TestAppendixPaintsConnectionTooltipAndMarkdownLinkTitle(t *testing.T) {
	t.Parallel()
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "markdown", Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: 10, Y: 20}, Width: 260, Height: 100,
		Fill: "N7", Stroke: "N1", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{
			Label: "Read [the docs](https://example.com \"link help\")", Language: "markdown",
			FontFamily: "default", FontSize: 16, LabelWidth: 220, LabelHeight: 60,
		},
	}}
	connection := metadataConnection()
	connection.Link = ""
	connection.PrettyLink = ""
	connection.Tooltip = "connection hover"
	diagram.Connections = []d2target.Connection{connection}

	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{
		Pad: &pad, Appendix: true,
		LinkBudget: LinkBudget{MaxRegions: 4, MaxStringBytes: 4_096},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Links) != 2 {
		t.Fatalf("typed links = %#v, want connection tooltip and Markdown link", document.Links)
	}
	appendix := document.Root.Children[len(document.Root.Children)-1]
	if appendix.ID != "appendix" || len(appendix.Children) != 3 {
		t.Fatalf("appendix ID/children = %q/%d, want appendix separator plus two rows", appendix.ID, len(appendix.Children))
	}
	wantRows := []string{"connection hover", "link help"}
	for index, want := range wantRows {
		row := appendix.Children[index+1]
		var lines []string
		for _, child := range row.Children[1:] {
			lines = append(lines, child.Primitive.(d2scene.TextRun).Text)
		}
		if got := strings.Join(lines, "\n"); got != want {
			t.Fatalf("appendix row %d = %q, want %q", index+1, got, want)
		}
	}
}

func TestAppendixAndPositionedTooltipShareSortedMetadataLayer(t *testing.T) {
	t.Parallel()
	diagram := appendixTestDiagram()
	diagram.Shapes[1].TooltipPosition = "top-left"
	legendShape := *d2target.BaseShape()
	legendShape.ID, legendShape.Type, legendShape.Label = "legend-entry", d2target.ShapeRectangle, "metadata"
	legendShape.Fill, legendShape.Stroke = "B6", "B1"
	diagram.Legend = &d2target.Legend{Label: "Key", Shapes: []d2target.Shape{legendShape}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{
		Pad: &pad, Appendix: true,
		LinkBudget: LinkBudget{MaxRegions: 2, MaxStringBytes: 4_096},
	})
	if err != nil {
		t.Fatal(err)
	}

	wantOrder := []string{
		"root:background", "second", "first",
		"second:positioned-tooltip", "appendix:shape:1:link", "appendix:shape:0:link",
		"legend", "appendix",
	}
	if len(document.Root.Children) != len(wantOrder) {
		t.Fatalf("root children = %d, want %d", len(document.Root.Children), len(wantOrder))
	}
	for index, want := range wantOrder {
		if got := document.Root.Children[index].ID; got != want {
			t.Fatalf("root child %d = %q, want %q", index, got, want)
		}
	}
}

func TestAppendixRasterPaintsSeparatorAndBadges(t *testing.T) {
	t.Parallel()
	diagram := appendixTestDiagram()
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{
		Pad: &pad, Appendix: true,
		LinkBudget: LinkBudget{MaxRegions: 2, MaxStringBytes: 4_096},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 2, Background: color.White,
		MaxWidth: 4_096, MaxHeight: 4_096, MaxPixels: 16_777_216,
		MaxNodes: 10_000, MaxDepth: 128, MaxPathCommands: 100_000,
		MaxAnimationTracks: 100, MaxAnimationKeyframes: 1_000,
		MaxAssets: 16, MaxAssetBytes: 32 << 20, MaxDecodedAssetBytes: 32 << 20, MaxImportDepth: 16,
		MaxOffscreenBytes: 64 << 20, MaxEvenOddClipWork: 10_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantWidth := int(document.LogicalWidth * 2)
	wantHeight := int(document.LogicalHeight * 2)
	if frame.Bounds().Dx() != wantWidth || frame.Bounds().Dy() != wantHeight {
		t.Fatalf("frame = %dx%d, want %dx%d", frame.Bounds().Dx(), frame.Bounds().Dy(), wantWidth, wantHeight)
	}

	appendix := document.Root.Children[len(document.Root.Children)-1]
	separator := appendix.Children[0].Primitive.(d2scene.Path)
	start, end := separator.Commands[0].P1, separator.Commands[1].P1
	midpoint := d2scene.Point{X: (start.X + end.X) / 2, Y: start.Y}
	if !pixelNeighborhoodContains(frame, document.ViewBox, midpoint, 2, func(pixel color.NRGBA) bool {
		return pixel.B > pixel.R+40 && pixel.B > pixel.G+20 && pixel.A == 255
	}) {
		t.Fatalf("separator near %+v contains no theme-blue painted pixel", midpoint)
	}

	rowIcon := appendix.Children[1].Children[0]
	circle := rowIcon.Children[0].Primitive.(d2scene.Ellipse)
	top := d2scene.Point{X: circle.Center.X, Y: circle.Center.Y - circle.RadiusY}
	if !pixelNeighborhoodContains(frame, document.ViewBox, top, 3, func(pixel color.NRGBA) bool {
		return pixel.R >= 200 && pixel.R < 250 && pixel.G >= 200 && pixel.G < 250 && pixel.B >= 200 && pixel.B < 250 && pixel.A == 255
	}) {
		t.Fatalf("appendix badge border near %+v was not rasterized", top)
	}
}

func TestAppendixBoundsStringsAndCancellation(t *testing.T) {
	t.Parallel()
	diagram := d2target.NewDiagram()
	shape := *d2target.BaseShape()
	shape.ID, shape.Type = "bounded", d2target.ShapeRectangle
	shape.Width, shape.Height = 100, 60
	shape.Fill, shape.Stroke = "#fff", "#000"
	shape.Tooltip = "tip"
	shape.Link = "x"
	shape.PrettyLink = "pretty-link"
	diagram.Shapes = []d2target.Shape{shape}
	stringBytes := len(shape.Tooltip) + len(shape.PrettyLink)
	options := Options{Appendix: true, LinkBudget: LinkBudget{MaxRegions: 1, MaxStringBytes: stringBytes}}
	if _, err := Build(context.Background(), diagram, options); err != nil {
		t.Fatalf("appendix exactly at string/item limits failed: %v", err)
	}
	options.LinkBudget.MaxStringBytes--
	if _, err := Build(context.Background(), diagram, options); err == nil || !strings.Contains(err.Error(), "appendix string bytes") {
		t.Fatalf("appendix string limit+1 error = %v", err)
	}

	invalid := appendixTestDiagram()
	invalid.Shapes[0].PrettyLink = string([]byte{0xff})
	if _, err := Build(context.Background(), invalid, Options{
		Appendix: true, LinkBudget: LinkBudget{MaxRegions: 2, MaxStringBytes: 4_096},
	}); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("invalid pretty-link error = %v", err)
	}

	cancelDiagram := appendixTestDiagram()
	cancelDiagram.Shapes[0].PrettyLink = strings.Repeat("long appendix text ", 4_096)
	ctx := newCancelAfterChecksContext(4)
	t.Cleanup(ctx.cancel)
	b := builder{
		ctx: ctx, diagram: cancelDiagram,
		options: Options{Appendix: true, LinkBudget: LinkBudget{MaxRegions: 2, MaxStringBytes: 1 << 20}},
	}
	if err := b.preflightAppendix(); !errors.Is(err, context.Canceled) {
		t.Fatalf("preflightAppendix() error = %v, want context.Canceled", err)
	}
}

func appendixTestDiagram() *d2target.Diagram {
	diagram := d2target.NewDiagram()
	primary, mono := d2fonts.SourceSansPro, d2fonts.SourceCodePro
	diagram.FontFamily, diagram.MonoFontFamily = &primary, &mono
	first := *d2target.BaseShape()
	first.ID, first.Type = "first", d2target.ShapeRectangle
	first.Pos = d2target.Point{X: -20, Y: 10}
	first.Width, first.Height = 80, 50
	first.Fill, first.Stroke = "B6", "B1"
	first.Link = "https://example.com/first"
	first.PrettyLink = first.Link

	second := *d2target.BaseShape()
	second.ID, second.Type = "second", d2target.ShapeOval
	second.Pos = d2target.Point{X: 140, Y: 120}
	second.Width, second.Height = 100, 60
	second.Fill, second.Stroke = "B6", "B1"
	second.Tooltip = "second tooltip\nline two"
	second.Link = "https://example.com/second"
	second.PrettyLink = second.Link
	second.ZIndex = -1
	diagram.Shapes = []d2target.Shape{first, second}
	return diagram
}

func pixelNeighborhoodContains(frame image.Image, viewBox d2scene.Box, point d2scene.Point, radius int, predicate func(color.NRGBA) bool) bool {
	x := int((point.X - viewBox.X) * 2)
	y := int((point.Y - viewBox.Y) * 2)
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			candidateX, candidateY := x+dx, y+dy
			if !image.Pt(candidateX, candidateY).In(frame.Bounds()) {
				continue
			}
			pixel := color.NRGBAModel.Convert(frame.At(candidateX, candidateY)).(color.NRGBA)
			if predicate(pixel) {
				return true
			}
		}
	}
	return false
}

func TestAppendixIDsAreStableAcrossRepeatedBuilds(t *testing.T) {
	t.Parallel()
	diagram := appendixTestDiagram()
	options := Options{Appendix: true, LinkBudget: LinkBudget{MaxRegions: 2, MaxStringBytes: 4_096}}
	first, err := Build(context.Background(), diagram, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(context.Background(), diagram, options)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprint(appendixNodeIDs(first.Root)), fmt.Sprint(appendixNodeIDs(second.Root)); got != want {
		t.Fatalf("repeated appendix IDs differ: %s != %s", got, want)
	}
}

func appendixNodeIDs(root *d2scene.Node) []string {
	var result []string
	var walk func(*d2scene.Node)
	walk = func(node *d2scene.Node) {
		if node == nil {
			return
		}
		if strings.HasPrefix(node.ID, "appendix") {
			result = append(result, node.ID)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return result
}
