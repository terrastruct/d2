package d2scenebuild

import (
	"context"
	"errors"
	"image/color"
	"strconv"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestBuildLegendPreservesEmptyItemAndSeparatorQuirks(t *testing.T) {
	t.Parallel()

	base := legendTestDiagram()
	base.Legend = nil
	pad := int64(0)
	without, err := Build(context.Background(), base, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}

	empty := legendTestDiagram()
	empty.Legend = &d2target.Legend{
		Label:       strings.Repeat("title width is deliberately ignored ", 8),
		Shapes:      []d2target.Shape{{ID: "hidden", Type: d2target.ShapeRectangle}},
		Connections: []d2target.Connection{{ID: "hidden"}},
	}
	withEmpty, err := Build(context.Background(), empty, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	if withEmpty.ViewBox != without.ViewBox {
		t.Fatalf("all-empty legend expanded viewbox: %+v != %+v", withEmpty.ViewBox, without.ViewBox)
	}
	legend := withEmpty.Root.Children[len(withEmpty.Root.Children)-1]
	if got := childIDs(legend.Children); !equalStrings(got, []string{"legend:shadow", "legend:panel", "legend:title"}) {
		t.Fatalf("all-empty legend children = %q", got)
	}
	panel := legend.Children[1].Primitive.(d2scene.Rect)
	if panel.Box.Width != 84 || panel.Box.Height != 73 {
		t.Fatalf("all-empty legend box = %+v, want 84x73", panel.Box)
	}
	if len(withEmpty.Assets) != 1 {
		t.Fatalf("all-empty legend assets = %d, want title bold font only", len(withEmpty.Assets))
	}

	separatorDiagram := legendTestDiagram()
	separatorDiagram.Legend = &d2target.Legend{
		Shapes: []d2target.Shape{legendShape("visible", "Shape")},
		Connections: []d2target.Connection{{
			ID: "empty connection", Text: d2target.Text{Label: ""},
		}},
	}
	withSeparator, err := Build(context.Background(), separatorDiagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	separatorLegend := withSeparator.Root.Children[len(withSeparator.Root.Children)-1]
	separator := findSceneNode(t, separatorLegend, "legend:separator")
	path, ok := separator.Primitive.(d2scene.Path)
	if !ok || path.Stroke == nil || len(path.Stroke.Dashes) != 2 || path.Stroke.Dashes[0] != 2 || path.Stroke.Dashes[1] != 2 {
		t.Fatalf("svg separator = %#v, want dashed path for raw non-empty connection slice", separator.Primitive)
	}
}

func TestBuildLegendRendersThemeAndOpacity(t *testing.T) {
	t.Parallel()

	diagram := legendTestDiagram()
	shape := legendShape("themed", "Themed")
	shape.Fill = "B6"
	shape.Stroke = "none"
	shape.Opacity = .5
	diagram.Legend = &d2target.Legend{Shapes: []d2target.Shape{shape}}
	red := "#ff0000"
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{
		Pad: &pad, ThemeOverrides: &d2target.ThemeOverrides{B6: &red},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := d2raster.Render(context.Background(), document, patternFrameOptions())
	if err != nil {
		t.Fatal(err)
	}
	legend := document.Root.Children[len(document.Root.Children)-1]
	panel := legend.Children[1].Primitive.(d2scene.Rect).Box
	shapeWrapper := findSceneNode(t, legend, "legend:shape:0")
	center := shapeWrapper.Transform.Point(d2scene.Point{X: 60, Y: 60})
	iconPixel := frame.NRGBAAt(int(center.X-document.ViewBox.X), int(center.Y-document.ViewBox.Y))
	if !nearNRGBA(iconPixel, color.NRGBA{R: 255, G: 128, B: 128, A: 255}, 3) {
		t.Fatalf("themed half-opacity legend icon pixel = %#v, want red over white", iconPixel)
	}
	panelPixel := frame.NRGBAAt(int(panel.X-document.ViewBox.X+5), int(panel.Y-document.ViewBox.Y+5))
	if !nearNRGBA(panelPixel, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, 1) {
		t.Fatalf("fixed legend panel pixel = %#v, want white", panelPixel)
	}
}

func TestBuildLegendAssetsAreRetainedOnlyForVisibleItems(t *testing.T) {
	assetURL := testRasterAssetURL(t)
	pad := int64(0)

	visible := legendTestDiagram()
	visible.Legend = &d2target.Legend{Shapes: []d2target.Shape{{
		ID: "photo", Type: d2target.ShapeImage, Icon: assetURL, Opacity: 1,
		Fill: "none", Stroke: "none", Text: d2target.Text{Label: "Photo"},
	}}}
	document, err := Build(context.Background(), visible, Options{Pad: &pad, Assets: testAssetOptions(t)})
	if err != nil {
		t.Fatal(err)
	}
	imageNode := findSceneNode(t, document.Root, "legend:shape:0:icon:image")
	imagePrimitive, ok := imageNode.Primitive.(d2scene.Image)
	if !ok {
		t.Fatalf("legend image primitive = %T, want Image", imageNode.Primitive)
	}
	if _, ok := document.Assets[imagePrimitive.Asset].(d2scene.RasterAsset); !ok {
		t.Fatalf("legend raster asset %q = %T, want RasterAsset", imagePrimitive.Asset, document.Assets[imagePrimitive.Asset])
	}

	hiddenResolver, hiddenResolverCalls := newCountingAssetResolver(t, errors.New("hidden legend resolver called"))
	hidden := legendTestDiagram()
	hidden.Legend = &d2target.Legend{Shapes: []d2target.Shape{{
		ID: "hidden", Type: d2target.ShapeImage, Icon: assetURL, Opacity: 1,
		Fill: "none", Stroke: "none",
	}}}
	if _, err := Build(context.Background(), hidden, Options{Pad: &pad, Assets: &AssetOptions{Resolver: hiddenResolver}}); err != nil {
		t.Fatalf("empty-label legend image Build() error = %v", err)
	}
	if hiddenResolverCalls.Load() != 0 {
		t.Fatalf("empty-label legend image resolver calls = %d, want 0", hiddenResolverCalls.Load())
	}

	rejectedResolver, rejectedResolverCalls := newCountingAssetResolver(t, errors.New("rejected legend resolver called"))
	rejected := legendTestDiagram()
	connection := legendConnection("asset", "Flow")
	connection.Icon = assetURL
	rejected.Legend = &d2target.Legend{Connections: []d2target.Connection{connection}}
	result, err := Build(context.Background(), rejected, Options{Pad: &pad, Assets: &AssetOptions{Resolver: rejectedResolver}})
	if err == nil || result != nil || !strings.Contains(err.Error(), "connection icon asset") {
		t.Fatalf("legend connection asset Build() = %#v/%v, want explicit unsupported error", result, err)
	}
	if rejectedResolverCalls.Load() != 0 {
		t.Fatalf("unsupported legend connection asset resolver calls = %d, want 0", rejectedResolverCalls.Load())
	}
}

func TestBuildLegendPreflightsMalformedFeaturesAndAnimation(t *testing.T) {
	tests := []struct {
		name string
		set  func(*d2target.Diagram)
		want string
	}{
		{
			name: "shape metadata",
			set: func(diagram *d2target.Diagram) {
				shape := legendShape("shape", "Shape")
				shape.Tooltip = "not painted"
				diagram.Legend = &d2target.Legend{Shapes: []d2target.Shape{shape}}
			},
			want: `legend shape[0] "shape"`,
		},
		{
			name: "connection endpoint label",
			set: func(diagram *d2target.Diagram) {
				connection := legendConnection("edge", "Flow")
				connection.SrcLabel = &d2target.Text{Label: "source"}
				diagram.Legend = &d2target.Legend{Connections: []d2target.Connection{connection}}
			},
			want: "endpoint label",
		},
		{
			name: "invalid arrow",
			set: func(diagram *d2target.Diagram) {
				connection := legendConnection("edge", "Flow")
				connection.DstArrow = d2target.Arrowhead("unknown")
				diagram.Legend = &d2target.Legend{Connections: []d2target.Connection{connection}}
			},
			want: "arrowhead unknown",
		},
		{
			name: "invalid opacity",
			set: func(diagram *d2target.Diagram) {
				shape := legendShape("shape", "Shape")
				shape.Opacity = 2
				diagram.Legend = &d2target.Legend{Shapes: []d2target.Shape{shape}}
			},
			want: "opacity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := legendTestDiagram()
			test.set(diagram)
			document, err := Build(context.Background(), diagram, Options{})
			if err == nil || document != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build() = %#v/%v, want nil error containing %q", document, err, test.want)
			}
		})
	}

	animated := legendTestDiagram()
	shape := legendShape("animated", "Animated")
	shape.Animated = true
	shape.Shadow = true
	animated.Legend = &d2target.Legend{Shapes: []d2target.Shape{shape}}
	document, err := Build(context.Background(), animated, Options{})
	if err != nil {
		t.Fatal(err)
	}
	target := findSceneNode(t, document.Root, "legend:shape:0:icon")
	if len(target.Animations) != 3 || len(target.Filters) != 2 {
		t.Fatalf("animated legend shape tracks/filters = %d/%d, want 3/2", len(target.Animations), len(target.Filters))
	}
	geometry := findSceneNode(t, target, "legend:shape:0:icon:shape")
	if len(geometry.Filters) != 1 {
		t.Fatalf("animated shadow legend geometry filters = %d, want static svg shadow", len(geometry.Filters))
	}

	for _, language := range []string{"markdown", "latex"} {
		t.Run("ignored icon language "+language, func(t *testing.T) {
			diagram := legendTestDiagram()
			shape := legendShape("language", "Plain legend row")
			shape.Language = language
			diagram.Legend = &d2target.Legend{Shapes: []d2target.Shape{shape}}
			if _, err := Build(context.Background(), diagram, Options{}); err != nil {
				t.Fatalf("Build() error = %v, want blank icon label to bypass %s rendering", err, language)
			}
		})
	}

	t.Run("opacity zero structured work is still preflighted", func(t *testing.T) {
		assetURL := mustAssetURL(t, "https://assets.example/image.png")
		resolver, resolverCalls := newCountingAssetResolver(t, errors.New("structured legend resolver called"))
		diagram := legendTestDiagram()
		shape := legendShape("class", "Class")
		shape.Type = d2target.ShapeClass
		shape.Opacity = 0
		shape.FontSize = 0
		shape.Fields = []d2target.ClassField{{Name: "field"}}
		shape.Icon = assetURL
		diagram.Legend = &d2target.Legend{Shapes: []d2target.Shape{shape}}
		document, err := Build(context.Background(), diagram, Options{Assets: &AssetOptions{Resolver: resolver}})
		if err == nil || document != nil || !strings.Contains(err.Error(), "fontSize") {
			t.Fatalf("Build() = %#v/%v, want complete structured preflight error", document, err)
		}
		if resolverCalls.Load() != 0 {
			t.Fatalf("resolver calls before structured preflight failure = %d, want 0", resolverCalls.Load())
		}
	})
}

func TestBuildLegendConnectionDoesNotReuseDiagramShapeAdjustment(t *testing.T) {
	t.Parallel()

	diagram := legendTestDiagram()
	// BaseConnection deliberately leaves Src and Dst empty. SVG builds each
	// legend connection with an empty shape lookup, even if the diagram happens
	// to contain a shape whose ID is also empty.
	diagram.Shapes[0].ID = ""
	diagram.Shapes[0].StrokeWidth = 20
	connection := legendConnection("edge", "Flow")
	connection.DstArrow = d2target.NoArrowhead
	diagram.Legend = &d2target.Legend{Connections: []d2target.Connection{connection}}
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	pathNode := findSceneNode(t, document.Root, "legend:connection:0:icon:path")
	path, ok := pathNode.Primitive.(d2scene.Path)
	if !ok || len(path.Commands) != 2 {
		t.Fatalf("legend connection path = %#v, want move+line", pathNode.Primitive)
	}
	if path.Commands[0].P1.X != 1 || path.Commands[1].P1.X != 47 {
		t.Fatalf("legend connection endpoints = %v..%v, want 1..47 from empty svg shape lookup", path.Commands[0].P1.X, path.Commands[1].P1.X)
	}
}

func TestBuildLegendObservesCancellationDuringItems(t *testing.T) {
	t.Parallel()

	diagram := legendTestDiagram()
	for i := 0; i < 256; i++ {
		diagram.Legend = appendLegendShape(diagram.Legend, legendShape(strconv.Itoa(i), "legend item"))
	}
	ctx := newCancelAfterChecksContext(32)
	t.Cleanup(ctx.cancel)
	document, err := Build(ctx, diagram, Options{})
	if !errors.Is(err, context.Canceled) || document != nil {
		t.Fatalf("Build() = %#v/%v, want nil/context.Canceled", document, err)
	}
}

func TestBuildLegendCollapsesSVGWhitespaceBeforeRasterization(t *testing.T) {
	t.Parallel()

	diagram := legendTestDiagram()
	diagram.Legend = &d2target.Legend{
		Label:  "  My\n  Legend\t ",
		Shapes: []d2target.Shape{legendShape("shape", "  one\r\n\ttwo  ")},
	}
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatal(err)
	}
	title := findSceneNode(t, document.Root, "legend:title").Primitive.(d2scene.TextRun)
	item := findSceneNode(t, document.Root, "legend:shape:0:label").Primitive.(d2scene.TextRun)
	if title.Text != "My Legend" || item.Text != "one two" {
		t.Fatalf("collapsed legend text = %q/%q, want %q/%q", title.Text, item.Text, "My Legend", "one two")
	}
	if _, err := d2raster.Render(context.Background(), document, patternFrameOptions()); err != nil {
		t.Fatalf("render collapsed legend whitespace: %v", err)
	}
}

func TestBuildLegendRejectsOverflowEvenWhenEmptyItemsDoNotExpandViewBox(t *testing.T) {
	t.Parallel()

	maxPlatform := int(^uint(0) >> 1)
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "edge", Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: maxPlatform - 101}, Width: 90, Height: 10,
		Opacity: 1, Fill: "#fff", Stroke: "none",
	}}
	diagram.Legend = &d2target.Legend{Shapes: []d2target.Shape{{ID: "empty"}}}
	document, err := Build(context.Background(), diagram, Options{})
	if err == nil || document != nil || !strings.Contains(err.Error(), "legend right edge") {
		t.Fatalf("Build() = %#v/%v, want all-empty legend arithmetic error", document, err)
	}
}

func TestBuildLegendSceneLimitsAreExactAndInclusive(t *testing.T) {
	t.Parallel()

	diagram := legendTestDiagram()
	diagram.Legend = &d2target.Legend{Shapes: []d2target.Shape{legendShape("shape", "Shape")}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	options := patternFrameOptions()
	options.MaxNodes = countLegendSceneNodes(document.Root) - 1
	if _, err := d2raster.Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "node count") {
		t.Fatalf("node limit-1 error = %v", err)
	}
	options.MaxNodes++
	if _, err := d2raster.Render(context.Background(), document, options); err != nil {
		t.Fatalf("exact node limit error = %v", err)
	}
}

func legendTestDiagram() *d2target.Diagram {
	diagram := d2target.NewDiagram()
	diagram.Root.Fill = "#eeeeee"
	diagram.Root.Stroke = "none"
	diagram.Shapes = []d2target.Shape{{
		ID: "ordinary", Type: d2target.ShapeRectangle,
		Pos: d2target.Point{}, Width: 100, Height: 100,
		Opacity: 1, Fill: "#eeeeee", Stroke: "none",
	}}
	return diagram
}

func legendShape(id, text string) d2target.Shape {
	return d2target.Shape{
		ID: id, Type: d2target.ShapeRectangle, Width: 100, Height: 100,
		Opacity: 1, Fill: "#6699cc", Stroke: "#112233", StrokeWidth: 2,
		Text: d2target.Text{Label: text, FontSize: 16, FontFamily: "DEFAULT"},
	}
}

func legendConnection(id, text string) d2target.Connection {
	return d2target.Connection{
		ID: id, SrcArrow: d2target.NoArrowhead, DstArrow: d2target.TriangleArrowhead,
		Opacity: 1, Stroke: "#112233", StrokeWidth: 2,
		Text: d2target.Text{Label: text, FontSize: 16, FontFamily: "DEFAULT"},
	}
}

func appendLegendShape(legend *d2target.Legend, shape d2target.Shape) *d2target.Legend {
	if legend == nil {
		legend = &d2target.Legend{}
	}
	legend.Shapes = append(legend.Shapes, shape)
	return legend
}

func countLegendSceneNodes(node *d2scene.Node) int {
	if node == nil {
		return 0
	}
	count := 1
	if node.Mask != nil {
		count += countLegendSceneNodes(node.Mask.Root)
	}
	for _, child := range node.Children {
		count += countLegendSceneNodes(child)
	}
	return count
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func nearNRGBA(got, want color.NRGBA, tolerance uint8) bool {
	near := func(left, right uint8) bool {
		if left > right {
			return left-right <= tolerance
		}
		return right-left <= tolerance
	}
	return near(got.R, want.R) && near(got.G, want.G) && near(got.B, want.B) && near(got.A, want.A)
}
