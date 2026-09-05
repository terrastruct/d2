package d2scenebuild

import (
	"context"
	"errors"
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestBuildTypedLinkAndTooltipRegions(t *testing.T) {
	diagram := metadataDiagram()
	withoutMetadata := metadataDiagram()
	for index := range withoutMetadata.Shapes {
		withoutMetadata.Shapes[index].Link = ""
		withoutMetadata.Shapes[index].PrettyLink = ""
		withoutMetadata.Shapes[index].Tooltip = ""
	}
	for index := range withoutMetadata.Connections {
		withoutMetadata.Connections[index].Link = ""
		withoutMetadata.Connections[index].Tooltip = ""
		// Ordinary linked connection labels are blue and underlined. Preserve
		// that visual while removing metadata from the comparison baseline.
		withoutMetadata.Connections[index].Color = "blue"
		withoutMetadata.Connections[index].Underline = true
	}

	pad := int64(0)
	options := Options{Pad: &pad, LinkBudget: LinkBudget{MaxRegions: 4, MaxStringBytes: 1_000}}
	document, err := Build(context.Background(), diagram, options)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	baseline, err := Build(context.Background(), withoutMetadata, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("baseline Build() error = %v", err)
	}
	if document.ViewBox != baseline.ViewBox || document.LogicalWidth != baseline.LogicalWidth || document.LogicalHeight != baseline.LogicalHeight ||
		!reflect.DeepEqual(document.Root, baseline.Root) {
		t.Fatal("non-pixel link metadata changed scene visuals or dimensions")
	}

	want := []d2scene.LinkRegion{
		{
			Box: d2scene.Box{X: 8, Y: 18, Width: 104, Height: 64},
			URL: "vscode://file/example.go:10:2", Tooltip: "open source",
		},
		{
			Box:    d2scene.Box{X: 138, Y: 18, Width: 104, Height: 64},
			Target: "root.layers.next",
		},
		{
			Box:     d2scene.Box{X: 268, Y: 18, Width: 104, Height: 64},
			Tooltip: "hover only",
		},
		{
			Box: d2scene.Box{X: 176, Y: 115, Width: 48, Height: 20},
			URL: "https://example.com/edge", Tooltip: "edge details",
		},
	}
	if !reflect.DeepEqual(document.Links, want) {
		t.Fatalf("Links = %#v, want %#v", document.Links, want)
	}
	linkedLabel := findMetadataSceneNode(document.Root, "edge:label:0")
	if linkedLabel == nil {
		t.Fatal("linked connection label node is missing")
	}
	run, ok := linkedLabel.Primitive.(d2scene.TextRun)
	if !ok || !run.Underline || run.Fill != (d2scene.SolidPaint{Color: color.NRGBA{B: 255, A: 255}}) {
		t.Fatalf("linked connection label = %#v, want blue underlined TextRun", linkedLabel.Primitive)
	}
}

func TestBuildLinkBudgetIsInclusive(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{metadataShape("one", 0, "x", "1234")}
	pad := int64(0)
	options := Options{Pad: &pad, LinkBudget: LinkBudget{MaxRegions: 1, MaxStringBytes: 5}}
	if _, err := Build(context.Background(), diagram, options); err != nil {
		t.Fatalf("metadata exactly at limits failed: %v", err)
	}

	options.LinkBudget.MaxStringBytes--
	if _, err := Build(context.Background(), diagram, options); err == nil || !strings.Contains(err.Error(), "string bytes") {
		t.Fatalf("byte limit+1 error = %v", err)
	}
	options.LinkBudget = LinkBudget{MaxRegions: 1, MaxStringBytes: 10}
	diagram.Shapes = append(diagram.Shapes, metadataShape("two", 30, "", "z"))
	if _, err := Build(context.Background(), diagram, options); err == nil || !strings.Contains(err.Error(), "region count") {
		t.Fatalf("count limit+1 error = %v", err)
	}
}

func TestBuildTooltipOnlyConnectionRegion(t *testing.T) {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{metadataShape("anchor", 0, "", "")}
	connection := metadataConnection()
	connection.Link = ""
	connection.Tooltip = "connection hover"
	diagram.Connections = []d2target.Connection{connection}
	document, err := Build(context.Background(), diagram, Options{
		LinkBudget: LinkBudget{MaxRegions: 1, MaxStringBytes: len(connection.Tooltip)},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := d2scene.LinkRegion{
		Box:     d2scene.Box{X: 176, Y: 115, Width: 48, Height: 20},
		Tooltip: connection.Tooltip,
	}
	if len(document.Links) != 1 || document.Links[0] != want {
		t.Fatalf("tooltip-only connection links = %#v, want %#v", document.Links, want)
	}
	run := findMetadataSceneNode(document.Root, "edge:label:0").Primitive.(d2scene.TextRun)
	if run.Underline {
		t.Fatal("tooltip-only connection unexpectedly received link underline styling")
	}
}

func TestBuildRejectsMalformedOrUnrepresentableLinkMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*d2target.Diagram)
		want   string
	}{
		{name: "missing budget", mutate: func(d *d2target.Diagram) {
			d.Shapes = []d2target.Shape{metadataShape("bad", 0, "", "tip")}
		}, want: "linkBudget"},
		{name: "invalid UTF-8", mutate: func(d *d2target.Diagram) {
			d.Shapes = []d2target.Shape{metadataShape("bad", 0, "", string([]byte{0xff}))}
		}, want: "valid UTF-8"},
		{name: "invalid XML control", mutate: func(d *d2target.Diagram) {
			d.Shapes = []d2target.Shape{metadataShape("bad", 0, "", "bad\x00tooltip")}
		}, want: "forbidden by XML 1.0"},
		{name: "URL tooltip with link", mutate: func(d *d2target.Diagram) {
			d.Shapes = []d2target.Shape{metadataShape("bad", 0, "https://example.com", "https://spoof.invalid")}
		}, want: "must not be a URL when link is also set"},
		{name: "orphan pretty link", mutate: func(d *d2target.Diagram) {
			shape := metadataShape("bad", 0, "", "")
			shape.PrettyLink = "derived label"
			d.Shapes = []d2target.Shape{shape}
		}, want: "requires a non-empty link"},
		{name: "empty hit box", mutate: func(d *d2target.Diagram) {
			shape := metadataShape("bad", 0, "", "tip")
			shape.Width, shape.Height, shape.StrokeWidth = 0, 0, 0
			d.Shapes = []d2target.Shape{shape}
		}, want: "positive dimensions"},
		{name: "orphan tooltip position", mutate: func(d *d2target.Diagram) {
			shape := metadataShape("bad", 0, "", "")
			shape.TooltipPosition = "top-left"
			d.Shapes = []d2target.Shape{shape}
		}, want: "requires a non-empty tooltip"},
		{name: "root metadata", mutate: func(d *d2target.Diagram) {
			d.Root.Tooltip = "tip"
		}, want: "unsupported link/tooltip metadata"},
		{name: "unlabelled connection", mutate: func(d *d2target.Diagram) {
			d.Shapes = []d2target.Shape{metadataShape("a", 0, "", "")}
			connection := metadataConnection()
			connection.Label = ""
			connection.LabelWidth, connection.LabelHeight = 0, 0
			d.Connections = []d2target.Connection{connection}
		}, want: "box-only link representation cannot encode route hit geometry"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := d2target.NewDiagram()
			test.mutate(diagram)
			options := Options{LinkBudget: LinkBudget{MaxRegions: 10, MaxStringBytes: 1_000}}
			if test.name == "missing budget" {
				options.LinkBudget = LinkBudget{}
			}
			result, err := Build(context.Background(), diagram, options)
			if err == nil || result != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build() result/error = %#v/%v, want %q", result, err, test.want)
			}
		})
	}
}

func TestBuildPositionedTooltipUsesMarkdownBounds(t *testing.T) {
	t.Parallel()
	shape := metadataShape("tip", 100, "", "**details** and [docs](https://example.com)")
	shape.Pos.Y = 100
	shape.TooltipPosition = "top-left"
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{shape}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{
		Pad: &pad, LinkBudget: LinkBudget{MaxRegions: 4, MaxStringBytes: 4 << 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	tooltip := findMetadataSceneNode(document.Root, "tip:positioned-tooltip")
	if tooltip == nil || len(tooltip.Children) != 3 {
		t.Fatalf("positioned tooltip = %#v, want background, tail, and Markdown content", tooltip)
	}
	if got := document.Root.Children[len(document.Root.Children)-1]; got != tooltip {
		t.Fatalf("final scene child = %q, want positioned tooltip on top", got.ID)
	}
	background, ok := tooltip.Children[0].Primitive.(d2scene.Rect)
	if !ok || background.RadiusX != positionedTooltipRadius || background.RadiusY != positionedTooltipRadius || background.Stroke == nil || background.Stroke.Width != 1 {
		t.Fatalf("positioned tooltip background = %#v", tooltip.Children[0].Primitive)
	}
	if background.Box.X != float64(shape.Pos.X) || background.Box.Y >= float64(shape.Pos.Y) || background.Box.Width <= 20 || background.Box.Height <= 20 {
		t.Fatalf("positioned tooltip box = %+v", background.Box)
	}
	tail, ok := tooltip.Children[1].Primitive.(d2scene.Path)
	if !ok || len(tail.Commands) != 4 || tail.Commands[2].P1.Y != background.Box.Y+background.Box.Height+positionedTooltipTailSize {
		t.Fatalf("positioned tooltip tail = %#v", tooltip.Children[1].Primitive)
	}
	markdown := tooltip.Children[2]
	if markdown.ID != "tip:positioned-tooltip:markdown" || markdown.Clip == nil || len(markdown.Children) == 0 {
		t.Fatalf("positioned tooltip Markdown = %+v", markdown)
	}
	if document.ViewBox.X > background.Box.X || document.ViewBox.Y > background.Box.Y ||
		document.ViewBox.X+document.ViewBox.Width < background.Box.X+background.Box.Width ||
		document.ViewBox.Y+document.ViewBox.Height < background.Box.Y+background.Box.Height {
		t.Fatalf("document viewBox %+v does not contain tooltip %+v", document.ViewBox, background.Box)
	}
	inlineLink := false
	for _, region := range document.Links {
		if region.URL == "https://example.com" {
			inlineLink = true
		}
	}
	if !inlineLink {
		t.Fatalf("positioned tooltip Markdown link missing from %#v", document.Links)
	}
	if _, err := d2raster.Render(context.Background(), document, markdownRasterOptions()); err != nil {
		t.Fatalf("raster positioned tooltip: %v", err)
	}
}

func TestLinkMetadataCancellationDoesNotCommitRegion(t *testing.T) {
	// Cancel while scanning the second region, after the first region has
	// reached the builder's provisional accumulator. compileLinkRegions must
	// roll the whole metadata batch back.
	ctx := newCancelAfterChecksContext(9)
	t.Cleanup(ctx.cancel)
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{
		metadataShape("first", 0, "x", ""),
		metadataShape("cancel", 120, "", strings.Repeat("tooltip", 4_096)),
	}
	b := builder{
		ctx: ctx, diagram: diagram,
		options: Options{LinkBudget: LinkBudget{MaxRegions: 2, MaxStringBytes: 1 << 20}},
	}
	err := b.compileLinkRegions()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("compileLinkRegions() error = %v, want context.Canceled", err)
	}
	if len(b.links) != 0 || b.linkBytes != 0 {
		t.Fatalf("canceled metadata compile committed regions=%d bytes=%d", len(b.links), b.linkBytes)
	}
}

func metadataDiagram() *d2target.Diagram {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{
		metadataShape("external", 10, "vscode://file/example.go:10:2", "open source"),
		metadataShape("internal", 140, "root.layers.next", ""),
		metadataShape("tooltip", 270, "", "hover only"),
	}
	diagram.Shapes[0].PrettyLink = "vscode://file/example.go:10:2"
	diagram.Connections = []d2target.Connection{metadataConnection()}
	return diagram
}

func metadataShape(id string, x int, link, tooltip string) d2target.Shape {
	return d2target.Shape{
		ID: id, Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: x, Y: 20}, Width: 100, Height: 60,
		Fill: "#fff", Stroke: "#000", StrokeWidth: 2, Opacity: 1,
		Link: link, Tooltip: tooltip,
	}
}

func metadataConnection() d2target.Connection {
	connection := *d2target.BaseConnection()
	connection.ID = "edge"
	connection.Route = []*geo.Point{{X: 100, Y: 125}, {X: 300, Y: 125}}
	connection.Label = "edge"
	connection.LabelPosition = "INSIDE_MIDDLE_CENTER"
	connection.LabelPercentage = .5
	connection.LabelWidth = 48
	connection.LabelHeight = 20
	connection.FontSize = 16
	connection.FontFamily = "default"
	connection.Link = "https://example.com/edge"
	connection.Tooltip = "edge details"
	return connection
}

func findMetadataSceneNode(root *d2scene.Node, id string) *d2scene.Node {
	stack := []*d2scene.Node{root}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		if node.ID == id {
			return node
		}
		stack = append(stack, node.Children...)
	}
	return nil
}

func TestLinkRegionBoxesRemainFiniteAtLargeValidCoordinates(t *testing.T) {
	shape := metadataShape("large", math.MaxInt32-200, "https://example.com", "")
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{shape}
	document, err := Build(context.Background(), diagram, Options{LinkBudget: LinkBudget{MaxRegions: 1, MaxStringBytes: 100}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	box := document.Links[0].Box
	if math.IsNaN(box.X) || math.IsInf(box.X, 0) || math.IsNaN(box.Width) || math.IsInf(box.Width, 0) {
		t.Fatalf("non-finite large link box: %+v", box)
	}
}
