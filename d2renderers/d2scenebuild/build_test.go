package d2scenebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestBuildSimpleTypedScene(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.Fill = "N7"
	diagram.Root.Stroke = "N1"
	diagram.Root.StrokeWidth = 2
	diagram.Shapes = []d2target.Shape{
		{
			ID: "a", Type: d2target.ShapeRectangle,
			Pos: d2target.Point{X: 10, Y: 20}, Width: 100, Height: 80,
			Fill: "B3", Stroke: "N1", StrokeWidth: 2, Opacity: 1,
			Text: d2target.Text{Label: "alpha", FontSize: 16, FontFamily: "default", Bold: true, LabelWidth: 42, LabelHeight: 19},
		},
		{
			ID: "b", Type: d2target.ShapeOval,
			Pos: d2target.Point{X: 200, Y: 20}, Width: 100, Height: 80,
			Fill: "AA4", Stroke: "N1", StrokeWidth: 2, Opacity: 1,
		},
	}
	diagram.Connections = []d2target.Connection{{
		ID: "a -> b", Src: "a", Dst: "b",
		Route:    []*geo.Point{{X: 110, Y: 60}, {X: 200, Y: 60}},
		SrcArrow: d2target.NoArrowhead,
		DstArrow: d2target.TriangleArrowhead,
		Stroke:   "N1", StrokeWidth: 2, Opacity: 1,
	}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	wantViewBox := (d2scene.Box{X: 7, Y: 17, Width: 296, Height: 86})
	if document.ViewBox != wantViewBox {
		t.Fatalf("ViewBox = %+v, want %+v", document.ViewBox, wantViewBox)
	}
	if document.LogicalWidth != 296 || document.LogicalHeight != 86 {
		t.Fatalf("logical dimensions = %vx%v, want 296x86", document.LogicalWidth, document.LogicalHeight)
	}
	if document.Root == nil || len(document.Root.Children) != 4 {
		t.Fatalf("root children = %d, want background + 3 objects", len(document.Root.Children))
	}
	if document.Root.Children[1].ID != "a" || document.Root.Children[2].ID != "b" || document.Root.Children[3].ID != "a -> b" {
		t.Fatalf("unexpected paint order: %q, %q, %q", document.Root.Children[1].ID, document.Root.Children[2].ID, document.Root.Children[3].ID)
	}

	shapeA := document.Root.Children[1]
	if _, ok := shapeA.Children[0].Primitive.(d2scene.Rect); !ok {
		t.Fatalf("shape a primitive = %T, want Rect", shapeA.Children[0].Primitive)
	}
	text, ok := shapeA.Children[1].Primitive.(d2scene.TextRun)
	if !ok {
		t.Fatalf("shape a label primitive = %T, want TextRun", shapeA.Children[1].Primitive)
	}
	if text.Text != "alpha" || text.Font.Asset == "" || text.Anchor != d2scene.AnchorMiddle {
		t.Fatalf("unexpected text run: %+v", text)
	}
	if len(text.Glyphs) == 0 {
		t.Fatal("ordinary label was not shaped during Build")
	}
	for index, glyph := range text.Glyphs {
		if glyph.Asset != text.Font.Asset {
			t.Fatalf("glyph %d asset = %q, want primary asset %q", index, glyph.Asset, text.Font.Asset)
		}
	}
	if asset, ok := document.Assets[text.Font.Asset].(d2scene.FontAsset); !ok || len(asset.Data) == 0 {
		t.Fatalf("font asset %q missing or empty", text.Font.Asset)
	}

	connection := document.Root.Children[3]
	path, ok := connection.Children[0].Primitive.(d2scene.Path)
	if !ok {
		t.Fatalf("connection primitive = %T, want Path", connection.Children[0].Primitive)
	}
	if len(path.Commands) != 2 || path.Commands[0].Kind != d2scene.MoveCommand || path.Commands[1].Kind != d2scene.LineCommand {
		t.Fatalf("connection commands = %+v, want move+line", path.Commands)
	}
	if len(connection.Children) != 2 {
		t.Fatalf("connection children = %d, want path + arrowhead", len(connection.Children))
	}

	// The scene owns copied geometry: mutating the target cannot alter it.
	diagram.Connections[0].Route[0].X = -999
	if path.Commands[0].P1.X == -999 {
		t.Fatal("scene path aliases d2target route")
	}
}

func TestBuildLogicalScaleIsSeparateFromViewBox(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "a", Type: d2target.ShapeRectangle, Pos: d2target.Point{}, Width: 10, Height: 20,
		Fill: "#fff", Stroke: "none", Opacity: 1,
	}}
	pad := int64(0)
	scale := 1.25
	document, err := Build(context.Background(), diagram, Options{Pad: &pad, Scale: &scale})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if document.ViewBox.Width != 10 || document.ViewBox.Height != 20 {
		t.Fatalf("viewbox changed by logical scale: %+v", document.ViewBox)
	}
	if document.LogicalWidth != 13 || document.LogicalHeight != 25 {
		t.Fatalf("logical dimensions = %vx%v, want ceil(12.5)x25", document.LogicalWidth, document.LogicalHeight)
	}
	if document.ViewportFit != d2scene.ViewportMeet || document.ViewportAlign != d2scene.ViewportAlignXMinYMin {
		t.Fatalf("viewport policy = %d/%d, want meet/xMinYMin", document.ViewportFit, document.ViewportAlign)
	}

	center := true
	centered, err := Build(context.Background(), diagram, Options{Pad: &pad, Scale: &scale, Center: &center})
	if err != nil {
		t.Fatalf("centered Build() error = %v", err)
	}
	if centered.ViewportFit != d2scene.ViewportMeet || centered.ViewportAlign != d2scene.ViewportAlignXMidYMid {
		t.Fatalf("centered viewport policy = %d/%d, want meet/xMidYMid", centered.ViewportFit, centered.ViewportAlign)
	}
	if centered.ViewBox != document.ViewBox {
		t.Fatalf("centering mutated viewbox: centered=%+v uncentered=%+v", centered.ViewBox, document.ViewBox)
	}
}

func TestBuildRejectsScaleThatOverflowsLogicalDimensions(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "a", Type: d2target.ShapeRectangle, Width: 10, Height: 20,
		Fill: "#fff", Stroke: "none", Opacity: 1,
	}}
	pad := int64(0)
	scale := math.MaxFloat64
	_, err := Build(context.Background(), diagram, Options{Pad: &pad, Scale: &scale})
	if err == nil || !strings.Contains(err.Error(), "logical dimensions") {
		t.Fatalf("Build() error = %v, want logical-dimension overflow error", err)
	}
}

func TestBuildClampsRootAndRectangleBorderRadius(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.BorderRadius = 100
	diagram.Shapes = []d2target.Shape{{
		ID: "wide", Type: d2target.ShapeRectangle,
		Width: 100, Height: 20, BorderRadius: 100,
		Fill: "#fff", Stroke: "none", Opacity: 1,
	}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	rootRect, ok := document.Root.Children[0].Primitive.(d2scene.Rect)
	if !ok {
		t.Fatalf("root background primitive = %T, want Rect", document.Root.Children[0].Primitive)
	}
	if rootRect.RadiusX != 10 || rootRect.RadiusY != 10 {
		t.Errorf("root radius = (%v,%v), want (10,10)", rootRect.RadiusX, rootRect.RadiusY)
	}
	shapeRect, ok := document.Root.Children[1].Children[0].Primitive.(d2scene.Rect)
	if !ok {
		t.Fatalf("shape primitive = %T, want Rect", document.Root.Children[1].Children[0].Primitive)
	}
	if shapeRect.RadiusX != 10 || shapeRect.RadiusY != 10 {
		t.Errorf("shape radius = (%v,%v), want (10,10)", shapeRect.RadiusX, shapeRect.RadiusY)
	}
}

func TestBuildRootStrokeUsesMiterJoin(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.Stroke = "#000"
	diagram.Root.StrokeWidth = 2
	diagram.Shapes = []d2target.Shape{{
		ID: "a", Type: d2target.ShapeRectangle,
		Width: 10, Height: 10, Fill: "#fff", Stroke: "none", Opacity: 1,
	}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	rootRect := document.Root.Children[0].Primitive.(d2scene.Rect)
	if rootRect.Stroke == nil || rootRect.Stroke.Join != d2scene.JoinMiter {
		t.Fatalf("root stroke = %+v, want miter join", rootRect.Stroke)
	}
}

func TestBuildTreatsNoneFillPatternAsNoPattern(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.FillPattern = "none"
	diagram.Shapes = []d2target.Shape{{
		ID: "a", Type: d2target.ShapeRectangle,
		Width: 10, Height: 10, Fill: "#fff", FillPattern: "none", Stroke: "none", Opacity: 1,
	}}
	if _, err := Build(context.Background(), diagram, Options{}); err != nil {
		t.Fatalf("Build() error = %v, want fill-pattern none accepted", err)
	}
}

func TestBuildAssignsStableLabelChildIDs(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "labelled", Type: d2target.ShapeRectangle,
		Width: 100, Height: 40, Fill: "#fff", Stroke: "none", Opacity: 1,
		Text: d2target.Text{
			Label: "one\ntwo", FontSize: 16, FontFamily: "DEFAULT",
			LabelWidth: 40, LabelHeight: 38, LabelFill: "#eee",
		},
	}}
	document, err := Build(context.Background(), diagram, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	children := document.Root.Children[1].Children
	want := []string{"", "labelled:label-fill", "labelled:label:0", "labelled:label:1"}
	if len(children) != len(want) {
		t.Fatalf("labelled children = %d, want %d", len(children), len(want))
	}
	for i := range want {
		if children[i].ID != want[i] {
			t.Errorf("child %d ID = %q, want %q", i, children[i].ID, want[i])
		}
	}
}

func TestBuildCustomShapeUsesTypedCommands(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "decision", Type: d2target.ShapeDiamond,
		Pos: d2target.Point{X: 5, Y: 7}, Width: 80, Height: 60,
		Fill: "#fff", Stroke: "#123456", StrokeWidth: 2, Opacity: 1,
	}}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	shapeNode := document.Root.Children[1]
	if len(shapeNode.Children) != 1 {
		t.Fatalf("diamond children = %d, want one typed path", len(shapeNode.Children))
	}
	path, ok := shapeNode.Children[0].Primitive.(d2scene.Path)
	if !ok {
		t.Fatalf("diamond primitive = %T, want Path", shapeNode.Children[0].Primitive)
	}
	if len(path.Commands) < 5 || path.Commands[0].Kind != d2scene.MoveCommand || path.Commands[len(path.Commands)-1].Kind != d2scene.CloseCommand {
		t.Fatalf("diamond path commands = %+v, want closed typed geometry", path.Commands)
	}
}

func TestBuildRejectsUnsupportedFeatureByObject(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "rounded icon", Type: d2target.ShapeRectangle, Width: 10, Height: 10,
		Fill: "#fff", Stroke: "#000", StrokeWidth: 2, Opacity: 1,
		Text: d2target.Text{Label: "x", FontSize: 16, LabelWidth: 8, LabelHeight: 19, Language: "latex"},
	}}
	_, err := Build(context.Background(), diagram, Options{})
	if err == nil {
		t.Fatal("Build() succeeded with an unsupported label language")
	}
	if !strings.Contains(err.Error(), `shape "rounded icon"`) || !strings.Contains(err.Error(), "label language latex") {
		t.Fatalf("Build() error = %q, want object and unsupported feature", err)
	}
}

func TestBuildRejectsCancelledContextBeforeAllocation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Build(ctx, d2target.NewDiagram(), Options{})
	if err != context.Canceled {
		t.Fatalf("Build() error = %v, want context.Canceled", err)
	}
}

func TestBuildRejectsPaddingOverflow(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{{ID: "a", Type: d2target.ShapeRectangle, Width: 1, Height: 1, Fill: "#fff", Opacity: 1}}
	pad := int64(math.MaxInt64)
	_, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err == nil || !strings.Contains(err.Error(), "padding") {
		t.Fatalf("Build() error = %v, want padding overflow", err)
	}
}

func TestBuildUsesConfiguredPrimaryAndMonoFontAssets(t *testing.T) {
	primary := d2fonts.FontFamily("SceneBuildPrimaryTest")
	mono := d2fonts.FontFamily("SceneBuildMonoTest")
	primarySpec := d2fonts.Font{Family: primary, Style: d2fonts.FONT_STYLE_REGULAR}
	monoSpec := d2fonts.Font{Family: mono, Style: d2fonts.FONT_STYLE_REGULAR}
	primaryBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("bundled primary test font is not loaded")
	}
	monoBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceCodePro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		t.Fatal("bundled mono test font is not loaded")
	}
	d2fonts.FontFaces.Set(primarySpec, append([]byte(nil), primaryBytes...))
	d2fonts.FontFaces.Set(monoSpec, append([]byte(nil), monoBytes...))
	t.Cleanup(func() {
		d2fonts.FontFaces.Delete(primarySpec)
		d2fonts.FontFaces.Delete(monoSpec)
	})

	diagram := d2target.NewDiagram()
	diagram.FontFamily = &primary
	diagram.MonoFontFamily = &mono
	diagram.Shapes = []d2target.Shape{
		labelledShape("default", 0, "DEFAULT"),
		labelledShape("empty", 30, ""),
		labelledShape("mono", 60, "mono"),
	}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	wantFamilies := map[string]d2fonts.FontFamily{
		"default": primary,
		"empty":   primary,
		"mono":    mono,
	}
	wantBytes := map[d2fonts.FontFamily][]byte{primary: primaryBytes, mono: monoBytes}
	for _, shapeNode := range document.Root.Children[1:] {
		if len(shapeNode.Children) != 2 {
			t.Fatalf("shape %q children = %d, want outline + text", shapeNode.ID, len(shapeNode.Children))
		}
		run, ok := shapeNode.Children[1].Primitive.(d2scene.TextRun)
		if !ok {
			t.Fatalf("shape %q label = %T, want TextRun", shapeNode.ID, shapeNode.Children[1].Primitive)
		}
		wantFamily := wantFamilies[shapeNode.ID]
		if run.Font.Family != string(wantFamily) {
			t.Errorf("shape %q font family = %q, want %q", shapeNode.ID, run.Font.Family, wantFamily)
		}
		asset, ok := document.Assets[run.Font.Asset].(d2scene.FontAsset)
		if !ok {
			t.Fatalf("shape %q font asset %q is missing", shapeNode.ID, run.Font.Asset)
		}
		if !bytes.Equal(asset.Data, wantBytes[wantFamily]) {
			t.Errorf("shape %q selected bytes for the wrong font family", shapeNode.ID)
		}
	}
}

func TestBuildFilledDiamondArrowheadVertices(t *testing.T) {
	t.Parallel()

	diagram := validDiagram()
	diagram.Connections[0].SrcArrow = d2target.FilledDiamondArrowhead
	diagram.Connections[0].DstArrow = d2target.FilledDiamondArrowhead
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	connection := document.Root.Children[len(document.Root.Children)-1]
	if len(connection.Children) != 3 {
		t.Fatalf("connection children = %d, want route + src/dst arrowheads", len(connection.Children))
	}
	width, height := d2target.FilledDiamondArrowhead.Dimensions(float64(diagram.Connections[0].StrokeWidth))
	want := []d2scene.PathCommand{
		d2scene.MoveTo(0, height/2),
		d2scene.LineTo(width/2, 0),
		d2scene.LineTo(width, height/2),
		d2scene.LineTo(width/2, height),
		d2scene.ClosePath(),
	}
	for _, childIndex := range []int{1, 2} {
		path, ok := connection.Children[childIndex].Primitive.(d2scene.Path)
		if !ok {
			t.Fatalf("arrowhead child %d = %T, want Path", childIndex, connection.Children[childIndex].Primitive)
		}
		if !reflect.DeepEqual(path.Commands, want) {
			t.Errorf("arrowhead child %d commands = %+v, want %+v", childIndex, path.Commands, want)
		}
	}
}

func TestBuildRejectsMalformedNumericFieldsBeforeBoundingBox(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*d2target.Diagram)
		wantObject string
		wantField  string
	}{
		{name: "nil route point", mutate: func(d *d2target.Diagram) { d.Connections[0].Route[1] = nil }, wantObject: `connection "a-b"`, wantField: "route[1]"},
		{name: "route NaN", mutate: func(d *d2target.Diagram) { d.Connections[0].Route[0].X = math.NaN() }, wantObject: `connection "a-b"`, wantField: "route[0].x"},
		{name: "route infinity", mutate: func(d *d2target.Diagram) { d.Connections[0].Route[1].Y = math.Inf(1) }, wantObject: `connection "a-b"`, wantField: "route[1].y"},
		{name: "shape opacity", mutate: func(d *d2target.Diagram) { d.Shapes[0].Opacity = -0.01 }, wantObject: `shape "a"`, wantField: "opacity"},
		{name: "connection opacity", mutate: func(d *d2target.Diagram) { d.Connections[0].Opacity = 1.01 }, wantObject: `connection "a-b"`, wantField: "opacity"},
		{name: "root dash", mutate: func(d *d2target.Diagram) { d.Root.StrokeDash = -1 }, wantObject: "root", wantField: "strokeDash"},
		{name: "shape dash NaN", mutate: func(d *d2target.Diagram) { d.Shapes[0].StrokeDash = math.NaN() }, wantObject: `shape "a"`, wantField: "strokeDash"},
		{name: "connection dash infinity", mutate: func(d *d2target.Diagram) { d.Connections[0].StrokeDash = math.Inf(1) }, wantObject: `connection "a-b"`, wantField: "strokeDash"},
		{name: "shape radius", mutate: func(d *d2target.Diagram) { d.Shapes[0].BorderRadius = -1 }, wantObject: `shape "a"`, wantField: "borderRadius"},
		{name: "connection radius NaN", mutate: func(d *d2target.Diagram) { d.Connections[0].BorderRadius = math.NaN() }, wantObject: `connection "a-b"`, wantField: "borderRadius"},
		{name: "negative width", mutate: func(d *d2target.Diagram) { d.Shapes[0].Width = -1 }, wantObject: `shape "a"`, wantField: "width"},
		{name: "negative stroke width", mutate: func(d *d2target.Diagram) { d.Connections[0].StrokeWidth = -1 }, wantObject: `connection "a-b"`, wantField: "strokeWidth"},
		{name: "negative font size", mutate: func(d *d2target.Diagram) { d.Shapes[0].FontSize = -1 }, wantObject: `shape "a"`, wantField: "fontSize"},
		{name: "negative label height", mutate: func(d *d2target.Diagram) { d.Shapes[0].LabelHeight = -1 }, wantObject: `shape "a"`, wantField: "labelHeight"},
		{name: "content aspect NaN", mutate: func(d *d2target.Diagram) { d.Shapes[0].ContentAspectRatio = floatPointer(math.NaN()) }, wantObject: `shape "a"`, wantField: "contentAspectRatio"},
		{name: "zero source tangent", mutate: func(d *d2target.Diagram) {
			d.Connections[0].SrcArrow = d2target.FilledDiamondArrowhead
			d.Connections[0].Route[1] = d.Connections[0].Route[0].Copy()
		}, wantObject: `connection "a-b"`, wantField: "route"},
		{name: "zero destination tangent", mutate: func(d *d2target.Diagram) {
			d.Connections[0].DstArrow = d2target.FilledDiamondArrowhead
			d.Connections[0].Route[0] = d.Connections[0].Route[1].Copy()
		}, wantObject: `connection "a-b"`, wantField: "route"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := validDiagram()
			test.mutate(diagram)
			_, err := Build(context.Background(), diagram, Options{})
			if err == nil {
				t.Fatal("Build() succeeded, want numeric preflight error")
			}
			if !strings.Contains(err.Error(), test.wantObject) || !strings.Contains(err.Error(), test.wantField) {
				t.Fatalf("Build() error = %q, want object %q and field %q", err, test.wantObject, test.wantField)
			}
		})
	}
}

func TestBuildDoesNotMutateTarget(t *testing.T) {
	t.Parallel()

	diagram := validDiagram()
	diagram.Root.BorderRadius = 999
	diagram.Shapes[0].BorderRadius = 999
	diagram.Shapes[0].Classes = []string{"one", "two"}
	before, err := json.Marshal(diagram)
	if err != nil {
		t.Fatalf("marshal target before Build: %v", err)
	}
	if _, err := Build(context.Background(), diagram, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	after, err := json.Marshal(diagram)
	if err != nil {
		t.Fatalf("marshal target after Build: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("Build mutated target\nbefore: %s\n after: %s", before, after)
	}
}

func TestBuildAdmissionLimitsBoundTargetLowerWork(t *testing.T) {
	t.Parallel()

	t.Run("nodes", func(t *testing.T) {
		t.Parallel()
		diagram := validDiagram()
		// Root + background + two shape groups + connection group + route.
		if _, err := Build(context.Background(), diagram, Options{MaxNodes: 6}); err != nil {
			t.Fatalf("Build() at node lower-bound limit error = %v", err)
		}
		_, err := Build(context.Background(), diagram, Options{MaxNodes: 5})
		if err == nil || !strings.Contains(err.Error(), "node count exceeds limit 5") {
			t.Fatalf("Build() below node lower-bound limit error = %v", err)
		}
	})

	t.Run("route path commands", func(t *testing.T) {
		t.Parallel()
		diagram := validDiagram()
		diagram.Connections[0].Route = []*geo.Point{{X: 20, Y: 10}, {X: 50, Y: 10}, {X: 80, Y: 10}}
		if _, err := Build(context.Background(), diagram, Options{MaxPathCommands: 3}); err != nil {
			t.Fatalf("Build() at route lower-bound limit error = %v", err)
		}
		_, err := Build(context.Background(), diagram, Options{MaxPathCommands: 2})
		if err == nil || !strings.Contains(err.Error(), "path command count exceeds limit 2") {
			t.Fatalf("Build() below route lower-bound limit error = %v", err)
		}
	})
}

func TestBuildObservesCancellationDuringRoutePreflight(t *testing.T) {
	t.Parallel()

	ctx := newCancelAfterChecksContext(4)
	t.Cleanup(ctx.cancel)
	diagram := d2target.NewDiagram()
	diagram.Connections = []d2target.Connection{{
		ID: "cancelled", Route: []*geo.Point{{X: 0, Y: 0}, {X: 1, Y: 1}},
		Stroke: "#000", StrokeWidth: 2, Opacity: 1,
	}}
	// The route exactly fits the admission lower bound; cancellation must still
	// be observed while its individual points are validated.
	_, err := Build(ctx, diagram, Options{MaxPathCommands: 2})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Build() error = %v, want context.Canceled", err)
	}
}

func labelledShape(id string, x int, family string) d2target.Shape {
	return d2target.Shape{
		ID: id, Type: d2target.ShapeRectangle,
		Pos: d2target.Point{X: x}, Width: 20, Height: 20,
		Fill: "#fff", Stroke: "#000", StrokeWidth: 1, Opacity: 1,
		Text: d2target.Text{
			Label: id, FontSize: 16, FontFamily: family,
			LabelWidth: 18, LabelHeight: 18,
		},
	}
}

func validDiagram() *d2target.Diagram {
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{
		{ID: "a", Type: d2target.ShapeRectangle, Pos: d2target.Point{}, Width: 20, Height: 20, Fill: "#fff", Stroke: "#000", StrokeWidth: 2, Opacity: 1},
		{ID: "b", Type: d2target.ShapeRectangle, Pos: d2target.Point{X: 80}, Width: 20, Height: 20, Fill: "#fff", Stroke: "#000", StrokeWidth: 2, Opacity: 1},
	}
	diagram.Connections = []d2target.Connection{{
		ID: "a-b", Src: "a", Dst: "b",
		Route:    []*geo.Point{{X: 20, Y: 10}, {X: 80, Y: 10}},
		SrcArrow: d2target.NoArrowhead, DstArrow: d2target.NoArrowhead,
		Stroke: "#000", StrokeWidth: 2, BorderRadius: 10, Opacity: 1,
	}}
	return diagram
}

func floatPointer(value float64) *float64 {
	return &value
}

type cancelAfterChecksContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func newCancelAfterChecksContext(remaining int) *cancelAfterChecksContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &cancelAfterChecksContext{Context: ctx, cancel: cancel, remaining: remaining}
}

func (c *cancelAfterChecksContext) Err() error {
	if c.remaining == 0 {
		c.cancel()
	} else {
		c.remaining--
	}
	return c.Context.Err()
}
