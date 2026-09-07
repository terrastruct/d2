package d2scenebuild_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/d2svg"
	svgappendix "github.com/d2lang/d2/d2renderers/d2svg/appendix"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/internal/testlog"
	d2log "github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/textmeasure"
)

// SVG comparison tests live outside the scene builder package because the SVG
// renderer now composes the native isometric renderer, which uses this builder.
var svgDimensionPattern = regexp.MustCompile(`(?:width|height)="([.0-9]+)"`)

func TestAppendixMatchesSVGViewBoxAndScaleQuirk(t *testing.T) {
	t.Parallel()
	diagram := svgParityAppendixDiagram()
	diagram.Root.Stroke = "B1"
	diagram.Root.StrokeWidth = 2
	diagram.Root.DoubleBorder = true
	legendShape := *d2target.BaseShape()
	legendShape.ID, legendShape.Type, legendShape.Label = "legend-entry", d2target.ShapeRectangle, "linked node"
	legendShape.Fill, legendShape.Stroke = "B6", "B1"
	diagram.Legend = &d2target.Legend{Label: "Key", Shapes: []d2target.Shape{legendShape}}
	pad := int64(17)
	scale := 1.75
	options := d2scenebuild.Options{
		Pad: &pad, Scale: &scale, Appendix: true,
		LinkBudget: d2scenebuild.LinkBudget{MaxRegions: 2, MaxStringBytes: 4_096},
	}
	document, err := d2scenebuild.Build(context.Background(), diagram, options)
	if err != nil {
		t.Fatal(err)
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	renderOptions := &d2svg.RenderOpts{Pad: &pad, Scale: &scale}
	svg, err := d2svg.Render(diagram, renderOptions)
	if err != nil {
		t.Fatal(err)
	}
	svg = svgappendix.Append(diagram, renderOptions, ruler, svg)
	if svg == nil {
		t.Fatal("svg appendix returned nil")
	}
	wantViewBox := parseSVGAppendixViewBox(t, svg)
	if document.ViewBox != wantViewBox {
		t.Fatalf("scene viewbox = %+v, svg = %+v", document.ViewBox, wantViewBox)
	}
	wantLogicalWidth, wantLogicalHeight := parseSVGOuterDimensions(t, svg)
	if document.LogicalWidth != wantLogicalWidth || document.LogicalHeight != wantLogicalHeight {
		t.Fatalf("scene logical dimensions = %vx%v, svg = %vx%v", document.LogicalWidth, document.LogicalHeight, wantLogicalWidth, wantLogicalHeight)
	}
	if document.LogicalWidth == scale*document.ViewBox.Width || document.LogicalHeight == scale*document.ViewBox.Height {
		t.Fatal("appendix unexpectedly retained the pre-appendix outer SVG scale")
	}

	outer, ok := document.Root.Children[0].Primitive.(d2scene.Rect)
	if !ok {
		t.Fatalf("double-border outer background = %T, want Rect", document.Root.Children[0].Primitive)
	}
	if outer.Box.Width != document.ViewBox.Width || outer.Box.Height != document.ViewBox.Height {
		t.Fatalf("rewritten outer background = %+v, want final viewbox width/height %+v", outer.Box, document.ViewBox)
	}
	inner := document.Root.Children[1].Primitive.(d2scene.Rect)
	if inner.Box.Width == document.ViewBox.Width || inner.Box.Height == document.ViewBox.Height {
		t.Fatal("svg double-border quirk lost: appendix must rewrite only the first background rectangle")
	}
	if got := document.Root.Children[len(document.Root.Children)-2].ID; got != "legend" {
		t.Fatalf("penultimate root layer = %q, want legend before appendix", got)
	}
	if got := document.Root.Children[len(document.Root.Children)-1].ID; got != "appendix" {
		t.Fatalf("last root layer = %q, want appendix", got)
	}
}

func TestAppendixCompiledCorpusMatchesSVGGeometry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		themeID int64
		script  string
		rows    int
	}{
		{
			name: "tooltip wider than diagram",
			script: `x: { tooltip: Total abstinence is easier than perfect moderation }
y: { tooltip: Gee, I feel kind of LIGHT in the head now,\nknowing I can't make my satellite dish PAYMENTS! }
x -> y
`,
			rows: 2,
		},
		{
			name:    "links dark",
			themeID: 200,
			script: `x: { link: https://d2lang.com }
y: { link: https://fosny.eu; tooltip: two lines\nremain two lines }
x -> y
`,
			rows: 3,
		},
		{
			name: "root fill",
			script: `x: { tooltip: Total abstinence is easier than perfect moderation }
y: { tooltip: the second note }
x -> y
style.fill: PaleVioletRed
`,
			rows: 2,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			ctx = d2log.With(ctx, testlog.New(t))
			ctx = d2log.Leveled(ctx, slog.LevelDebug)
			ruler, err := textmeasure.NewRuler()
			if err != nil {
				t.Fatal(err)
			}
			renderOptions := &d2svg.RenderOpts{ThemeID: &test.themeID}
			layoutResolver := func(string) (d2graph.LayoutGraph, error) {
				return d2dagrelayout.DefaultLayout, nil
			}
			diagram, _, err := d2lib.Compile(ctx, test.script, &d2lib.CompileOptions{
				Ruler: ruler, LayoutResolver: layoutResolver,
			}, renderOptions)
			if err != nil {
				t.Fatal(err)
			}
			document, err := d2scenebuild.Build(ctx, diagram, d2scenebuild.Options{
				ThemeID: &test.themeID, Appendix: true,
				LinkBudget: d2scenebuild.LinkBudget{MaxRegions: 16, MaxStringBytes: 1 << 20},
			})
			if err != nil {
				t.Fatal(err)
			}
			svg, err := d2svg.Render(diagram, renderOptions)
			if err != nil {
				t.Fatal(err)
			}
			svg = svgappendix.Append(diagram, renderOptions, ruler, svg)
			if want := parseSVGAppendixViewBox(t, svg); document.ViewBox != want {
				t.Fatalf("scene viewbox = %+v, svg = %+v", document.ViewBox, want)
			}
			wantWidth, wantHeight := parseSVGOuterDimensions(t, svg)
			if document.LogicalWidth != wantWidth || document.LogicalHeight != wantHeight {
				t.Fatalf("scene dimensions = %vx%v, svg = %vx%v", document.LogicalWidth, document.LogicalHeight, wantWidth, wantHeight)
			}
			appendix := document.Root.Children[len(document.Root.Children)-1]
			if appendix.ID != "appendix" || len(appendix.Children) != test.rows+1 {
				t.Fatalf("appendix ID/rows = %q/%d, want appendix/%d", appendix.ID, len(appendix.Children)-1, test.rows)
			}
		})
	}
}

func parseSVGAppendixViewBox(t *testing.T, svg []byte) d2scene.Box {
	t.Helper()
	parts := svgappendix.FindViewboxSlice(svg)
	if len(parts) != 4 {
		t.Fatalf("svg viewbox = %#v", parts)
	}
	values := make([]float64, 4)
	for index, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			t.Fatal(err)
		}
		values[index] = value
	}
	return d2scene.Box{X: values[0], Y: values[1], Width: values[2], Height: values[3]}
}

func parseSVGOuterDimensions(t *testing.T, svg []byte) (float64, float64) {
	t.Helper()
	matches := svgDimensionPattern.FindAllSubmatch(svg, 2)
	if len(matches) != 2 {
		t.Fatalf("svg outer dimensions not found in %q", svg[:min(200, len(svg))])
	}
	width, err := strconv.ParseFloat(string(matches[0][1]), 64)
	if err != nil {
		t.Fatal(err)
	}
	height, err := strconv.ParseFloat(string(matches[1][1]), 64)
	if err != nil {
		t.Fatal(err)
	}
	return width, height
}

func TestBuildLegendMatchesCheckedInSVGDimensionsAndStructure(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile("../../e2etests/testdata/txtar/legend-mono/dagre/board.exp.json")
	if err != nil {
		t.Fatal(err)
	}
	var diagram d2target.Diagram
	if err := json.Unmarshal(encoded, &diagram); err != nil {
		t.Fatal(err)
	}
	pad := int64(0)
	options := d2scenebuild.Options{Pad: &pad}
	svgOptions := &d2svg.RenderOpts{Pad: &pad}
	if diagram.Config != nil {
		options.ThemeID = diagram.Config.ThemeID
		svgOptions.ThemeID = diagram.Config.ThemeID
	}
	document, err := d2scenebuild.Build(context.Background(), &diagram, options)
	if err != nil {
		t.Fatalf("d2scenebuild.Build() error = %v", err)
	}
	svg, err := d2svg.Render(&diagram, svgOptions)
	if err != nil {
		t.Fatalf("d2svg.Render() error = %v", err)
	}
	wantViewBox := svgInnerViewBox(t, svg)
	if document.ViewBox != wantViewBox {
		t.Fatalf("scene viewbox = %+v, svg = %+v", document.ViewBox, wantViewBox)
	}

	if len(document.Root.Children) == 0 {
		t.Fatal("document root has no children")
	}
	legend := document.Root.Children[len(document.Root.Children)-1]
	if legend.ID != "legend" {
		t.Fatalf("last root child = %q, want legend", legend.ID)
	}
	wantChildren := []string{
		"legend:shadow", "legend:panel", "legend:title",
		"legend:shape:0", "legend:shape:0:label",
	}
	if got := svgParityChildIDs(legend.Children); !slices.Equal(got, wantChildren) {
		t.Fatalf("legend children = %q, want %q", got, wantChildren)
	}
	shapeWrapper := legend.Children[3]
	if shapeWrapper.Transform.A != .2 || shapeWrapper.Transform.D != .2 ||
		shapeWrapper.Transform.E != 86 || shapeWrapper.Transform.F != 53 {
		t.Fatalf("legend shape transform = %+v, want translate(86,53) scale(.2)", shapeWrapper.Transform)
	}
	shadow, ok := legend.Children[0].Primitive.(d2scene.Rect)
	if !ok || shadow.Box != (d2scene.Box{X: 66, Y: -1, Width: 184, Height: 105}) || shadow.RadiusX != 4 || shadow.RadiusY != 4 {
		t.Fatalf("legend shadow panel = %#v, want svg box/radius", legend.Children[0].Primitive)
	}
	if len(legend.Children[0].Filters) != 1 {
		t.Fatalf("legend shadow filter count = %d, want 1", len(legend.Children[0].Filters))
	}
	filter, ok := legend.Children[0].Filters[0].(d2scene.DropShadow)
	if !ok || filter.OffsetX != 0 || filter.OffsetY != 2 || filter.SigmaX != 3 || filter.SigmaY != 3 || filter.Color.A != 26 {
		t.Fatalf("legend shadow filter = %#v, want CSS drop-shadow equivalent", legend.Children[0].Filters[0])
	}

	title := legend.Children[2].Primitive.(d2scene.TextRun)
	item := legend.Children[4].Primitive.(d2scene.TextRun)
	if title.Font.Family != string(d2fonts.SourceCodePro) || title.Font.Weight != 700 || title.Font.Size != 16 {
		t.Fatalf("legend title font = %+v, want diagram primary bold 16", title.Font)
	}
	if item.Font.Family != string(d2fonts.SourceCodePro) || item.Font.Weight != 400 || item.Font.Size != 14 {
		t.Fatalf("legend item font = %+v, want diagram primary regular 14", item.Font)
	}
	for _, id := range []d2scene.AssetID{
		"font:" + d2scene.AssetID(d2fonts.SourceCodePro) + ":bold",
		"font:" + d2scene.AssetID(d2fonts.SourceCodePro) + ":regular",
	} {
		asset, ok := document.Assets[id].(d2scene.FontAsset)
		if !ok || len(asset.Data) == 0 {
			t.Fatalf("legend font asset %q = %T, want owned non-empty font", id, document.Assets[id])
		}
	}
}

func svgInnerViewBox(t *testing.T, svg []byte) d2scene.Box {
	t.Helper()
	pattern := regexp.MustCompile(`<svg class="[^"]*\bd2-svg\b[^"]*"[^>]*viewBox="(-?[0-9]+) (-?[0-9]+) ([0-9]+) ([0-9]+)"`)
	match := pattern.FindSubmatch(svg)
	if match == nil {
		t.Fatalf("SVG output has no inner d2-svg viewBox")
	}
	values := make([]int, 4)
	for index := range values {
		value, err := strconv.Atoi(string(match[index+1]))
		if err != nil {
			t.Fatal(err)
		}
		values[index] = value
	}
	return d2scene.Box{X: float64(values[0]), Y: float64(values[1]), Width: float64(values[2]), Height: float64(values[3])}
}

func svgParityAppendixDiagram() *d2target.Diagram {
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

func svgParityChildIDs(nodes []*d2scene.Node) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return ids
}
