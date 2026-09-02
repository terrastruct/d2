package d2scenebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestBuildClassShapeExactRowsAndPaintOrder(t *testing.T) {
	t.Parallel()

	shape := structuredTestShape("class", d2target.ShapeClass, 10, 20, 200, 120)
	shape.Label = "Person"
	shape.LabelWidth = 60
	shape.LabelHeight = 20
	shape.Text.Bold = true
	shape.Text.Italic = true
	shape.BorderRadius = 12
	shape.Fields = []d2target.ClassField{{Name: "name", Type: "string", Visibility: "private", Underline: true}}
	shape.Methods = []d2target.ClassMethod{{Name: "save()", Return: "error", Visibility: "protected"}}
	document := buildStructuredDocument(t, shape)
	node := findSceneNode(t, document.Root, shape.ID)
	assertChildIDs(t, node, []string{
		"class:outline", "class:class-header", "class:class-header-label",
		"class:class-field:0:prefix", "class:class-field:0:name", "class:class-field:0:type",
		"class:class-separator",
		"class:class-method:0:prefix", "class:class-method:0:name", "class:class-method:0:type",
	})

	outline := node.Children[0].Primitive.(d2scene.Rect)
	if outline.Box != (d2scene.Box{X: 10, Y: 20, Width: 200, Height: 120}) || outline.RadiusX != 12 || outline.RadiusY != 12 {
		t.Fatalf("class outline = %+v", outline)
	}
	header, ok := node.Children[1].Primitive.(d2scene.Path)
	if !ok {
		t.Fatalf("rounded class header = %T, want Path", node.Children[1].Primitive)
	}
	wantHeader := []d2scene.PathCommand{
		d2scene.MoveTo(10, 32), d2scene.CubicTo(10, 32, 10, 20, 22, 20),
		d2scene.LineTo(198, 20), d2scene.CubicTo(198, 20, 210, 20, 210, 32),
		d2scene.LineTo(210, 80), d2scene.LineTo(10, 80), d2scene.ClosePath(),
	}
	if !equalCommands(header.Commands, wantHeader) {
		t.Fatalf("class header commands = %+v, want %+v", header.Commands, wantHeader)
	}

	headerText := node.Children[2].Primitive.(d2scene.TextRun)
	if headerText.Origin != (d2scene.Point{X: 110, Y: 56}) || headerText.Anchor != d2scene.AnchorMiddle || headerText.Font.Size != 20 || headerText.Font.Family != "SourceCodePro" || headerText.Font.Style != "regular" || headerText.Font.Weight != 400 {
		t.Fatalf("class header text = %+v", headerText)
	}
	prefix := node.Children[3].Primitive.(d2scene.TextRun)
	name := node.Children[4].Primitive.(d2scene.TextRun)
	typeRun := node.Children[5].Primitive.(d2scene.TextRun)
	if prefix.Text != "-" || prefix.Origin != (d2scene.Point{X: 20, Y: 99}) || prefix.Anchor != d2scene.AnchorStart {
		t.Fatalf("class prefix = %+v", prefix)
	}
	if name.Text != "name" || name.Origin != (d2scene.Point{X: 40, Y: 99}) || !name.Underline {
		t.Fatalf("class name = %+v", name)
	}
	if typeRun.Text != "string" || typeRun.Origin != (d2scene.Point{X: 190, Y: 99}) || typeRun.Anchor != d2scene.AnchorEnd {
		t.Fatalf("class type = %+v", typeRun)
	}
	for _, run := range []d2scene.TextRun{prefix, name, typeRun} {
		if run.Font.Style != "regular" || run.Font.Weight != 400 {
			t.Fatalf("class row font = %+v, want regular weight 400", run.Font)
		}
	}
	separator := node.Children[6].Primitive.(d2scene.Path)
	if !equalCommands(separator.Commands, []d2scene.PathCommand{d2scene.MoveTo(10, 110), d2scene.LineTo(210, 110)}) || separator.Stroke == nil || separator.Stroke.Width != 1 {
		t.Fatalf("class separator = %+v", separator)
	}
}

func TestBuildSQLTableExactColumnsAndSeparators(t *testing.T) {
	t.Parallel()

	shape := structuredTestShape("table", d2target.ShapeSQLTable, 10, 10, 240, 120)
	shape.Label = "users"
	shape.LabelWidth = 50
	shape.LabelHeight = 20
	shape.Text.Bold = true
	shape.Text.Italic = true
	shape.BorderRadius = 10
	shape.Columns = []d2target.SQLColumn{
		{Name: d2target.Text{Label: "id", LabelWidth: 20}, Type: d2target.Text{Label: "uuid", LabelWidth: 35}, Constraint: []string{"primary_key"}},
		{Name: d2target.Text{Label: "owner_id", LabelWidth: 45}, Type: d2target.Text{Label: "uuid", LabelWidth: 35}, Constraint: []string{"foreign_key", "unique"}},
	}
	document := buildStructuredDocument(t, shape)
	node := findSceneNode(t, document.Root, shape.ID)
	assertChildIDs(t, node, []string{
		"table:outline", "table:table-header", "table:table-header-label",
		"table:table-row:0:name", "table:table-row:0:type", "table:table-row:0:constraint", "table:table-row:0:separator",
		"table:table-row:1:name", "table:table-row:1:type", "table:table-row:1:constraint", "table:table-row:1:separator",
	})
	headerText := node.Children[2].Primitive.(d2scene.TextRun)
	if headerText.Origin != (d2scene.Point{X: 20, Y: 35}) || headerText.Font.Size != 20 || headerText.Font.Family != "SourceSansPro" || headerText.Font.Style != "regular" || headerText.Font.Weight != 400 {
		t.Fatalf("table header text = %+v", headerText)
	}
	name := node.Children[3].Primitive.(d2scene.TextRun)
	typeRun := node.Children[4].Primitive.(d2scene.TextRun)
	constraint := node.Children[5].Primitive.(d2scene.TextRun)
	if name.Origin != (d2scene.Point{X: 20, Y: 74}) || name.Text != "id" {
		t.Fatalf("first column name = %+v", name)
	}
	if typeRun.Origin != (d2scene.Point{X: 85, Y: 74}) || typeRun.Text != "uuid" {
		t.Fatalf("first column type = %+v", typeRun)
	}
	if constraint.Origin != (d2scene.Point{X: 240, Y: 74}) || constraint.Anchor != d2scene.AnchorEnd || constraint.Text != "PK" {
		t.Fatalf("first column constraint = %+v", constraint)
	}
	for _, run := range []d2scene.TextRun{name, typeRun, constraint} {
		if run.Font.Style != "regular" || run.Font.Weight != 400 {
			t.Fatalf("table row font = %+v, want regular weight 400", run.Font)
		}
	}
	firstSeparator := node.Children[6].Primitive.(d2scene.Path)
	if !equalCommands(firstSeparator.Commands, []d2scene.PathCommand{d2scene.MoveTo(10, 90), d2scene.LineTo(250, 90)}) {
		t.Fatalf("first separator = %+v", firstSeparator.Commands)
	}
	lastSeparator := node.Children[10].Primitive.(d2scene.Path)
	if !equalCommands(lastSeparator.Commands, []d2scene.PathCommand{d2scene.MoveTo(20, 130), d2scene.LineTo(240, 130)}) || lastSeparator.Stroke == nil || lastSeparator.Stroke.Width != 2 {
		t.Fatalf("last rounded separator = %+v", lastSeparator)
	}
}

func TestBuildClassShapeMultilineHeader(t *testing.T) {
	t.Parallel()

	shape := structuredTestShape("class", d2target.ShapeClass, 0, 0, 200, 120)
	shape.Label = "First\nSecond\nThird"
	shape.LabelWidth = 100
	shape.LabelHeight = 60
	shape.Text.Bold = true
	document := buildStructuredDocument(t, shape)
	node := findSceneNode(t, document.Root, shape.ID)
	assertChildIDs(t, node, []string{
		"class:outline", "class:class-header",
		"class:class-header-label", "class:class-header-label:1", "class:class-header-label:2",
		"class:class-separator",
	})

	wantText := []string{"First", "Second", "Third"}
	for index, want := range wantText {
		run := node.Children[2+index].Primitive.(d2scene.TextRun)
		wantOrigin := d2scene.Point{X: 100, Y: 46 + float64(index*20)}
		if run.Text != want || run.Origin != wantOrigin || run.Anchor != d2scene.AnchorMiddle {
			t.Fatalf("class header line %d = %+v, want text %q at %+v", index, run, want, wantOrigin)
		}
		if run.Font.Family != "SourceCodePro" || run.Font.Style != "regular" || run.Font.Weight != 400 || run.Font.Size != 20 {
			t.Fatalf("class header line %d font = %+v, want regular SourceCodePro 20", index, run.Font)
		}
	}
}

func TestBuildStructuredShapesImmutableAndRasterizable(t *testing.T) {
	t.Parallel()

	classShape := structuredTestShape("class", d2target.ShapeClass, 0, 0, 180, 100)
	classShape.Label, classShape.LabelWidth, classShape.LabelHeight = "C", 15, 20
	classShape.Fields = []d2target.ClassField{{Name: "x", Type: "int"}}
	tableShape := structuredTestShape("table", d2target.ShapeSQLTable, 220, 0, 180, 100)
	tableShape.Label, tableShape.LabelWidth, tableShape.LabelHeight = "T", 15, 20
	tableShape.Columns = []d2target.SQLColumn{{Name: d2target.Text{Label: "id", LabelWidth: 20}, Type: d2target.Text{Label: "int", LabelWidth: 20}}}
	diagram := d2target.NewDiagram()
	diagram.Root.Fill, diagram.Root.Stroke = "#ffffff", "none"
	diagram.Shapes = []d2target.Shape{classShape, tableShape}
	before, err := json.Marshal(diagram)
	if err != nil {
		t.Fatal(err)
	}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(diagram)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("structured shape build mutated d2target diagram")
	}
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, MaxWidth: 1_000, MaxHeight: 1_000, MaxPixels: 1_000_000,
		MaxNodes: 10_000, MaxDepth: 100, MaxPathCommands: 1_000_000,
		MaxAnimationTracks: 10_000, MaxAnimationKeyframes: 100_000,
		MaxAssets: 100, MaxAssetBytes: 64 * 1024 * 1024,
		MaxDecodedAssetBytes: 64 * 1024 * 1024, MaxImportDepth: 100,
		MaxOffscreenBytes: 64 * 1024 * 1024, MaxEvenOddClipWork: 1_000_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if frame.Bounds().Dx() != 402 || frame.Bounds().Dy() != 102 {
		t.Fatalf("structured raster bounds = %v, want 402x102 including outer stroke", frame.Bounds())
	}
}

func TestBuildSQLTableWithoutColumnsRoundsWholeHeader(t *testing.T) {
	t.Parallel()

	shape := structuredTestShape("empty", d2target.ShapeSQLTable, 0, 0, 100, 40)
	shape.Label, shape.LabelWidth, shape.LabelHeight, shape.BorderRadius = "empty", 50, 20, 8
	document := buildStructuredDocument(t, shape)
	node := findSceneNode(t, document.Root, shape.ID)
	header, ok := node.Children[1].Primitive.(d2scene.Path)
	if !ok || len(header.Commands) != 10 || header.Commands[4] != d2scene.LineTo(100, 32) || header.Commands[5] != d2scene.CubicTo(100, 32, 100, 40, 92, 40) || header.Commands[7] != d2scene.CubicTo(8, 40, 0, 40, 0, 32) {
		t.Fatalf("empty table header = %#v (%T), want fully rounded clip geometry", node.Children[1].Primitive, node.Children[1].Primitive)
	}
}

func TestBuildAndRenderCheckedInStructuredTargets(t *testing.T) {
	t.Parallel()

	fixtures := []string{
		"measured/empty-class/dagre/board.exp.json",
		"regression/sql_table_overflow/dagre/board.exp.json",
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("..", "..", "e2etests", "testdata", filepath.FromSlash(fixture))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var diagram d2target.Diagram
			if err := json.Unmarshal(data, &diagram); err != nil {
				t.Fatal(err)
			}
			pad := int64(0)
			document, err := Build(context.Background(), &diagram, Options{Pad: &pad})
			if err != nil {
				t.Fatal(err)
			}
			_, err = d2raster.Render(context.Background(), document, d2raster.FrameOptions{
				Scale: 1, MaxWidth: 4_096, MaxHeight: 4_096, MaxPixels: 16_777_216,
				MaxNodes: 100_000, MaxDepth: 1_000, MaxPathCommands: 1_000_000,
				MaxAnimationTracks: 100_000, MaxAnimationKeyframes: 1_000_000,
				MaxAssets: 1_000, MaxAssetBytes: 64 * 1024 * 1024,
				MaxDecodedAssetBytes: 64 * 1024 * 1024, MaxImportDepth: 1_000,
				MaxOffscreenBytes: 64 * 1024 * 1024, MaxEvenOddClipWork: 1_000_000_000,
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func structuredTestShape(id, shapeType string, x, y, width, height int) d2target.Shape {
	return d2target.Shape{
		ID: id, Type: shapeType, Pos: d2target.Point{X: x, Y: y}, Width: width, Height: height,
		Fill: "#ddeeff", Stroke: "#112233", StrokeWidth: 2, Opacity: 1,
		Text:               d2target.Text{FontSize: 16, FontFamily: "default", Color: "#112233"},
		PrimaryAccentColor: "#cc0000", SecondaryAccentColor: "#0000cc", NeutralAccentColor: "#555555",
	}
}

func buildStructuredDocument(t *testing.T, shape d2target.Shape) *d2scene.Document {
	t.Helper()
	diagram := d2target.NewDiagram()
	diagram.Root.Fill, diagram.Root.Stroke = "#ffffff", "none"
	diagram.Shapes = []d2target.Shape{shape}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return document
}

func equalCommands(left, right []d2scene.PathCommand) bool {
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
