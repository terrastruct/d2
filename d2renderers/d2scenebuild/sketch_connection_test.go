package d2scenebuild

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestBuildSketchConnectionPreservesNormalStructureMaskAndLabels(t *testing.T) {
	diagram := validDiagram()
	connection := &diagram.Connections[0]
	connection.Classes = []string{"one", "two"}
	connection.SrcArrow = d2target.TriangleArrowhead
	connection.DstArrow = d2target.DiamondArrowhead
	connection.IsCurve = true
	connection.Route = []*geo.Point{
		{X: 20, Y: 10}, {X: 35, Y: -5}, {X: 65, Y: 25}, {X: 80, Y: 10},
	}
	connection.Text = d2target.Text{
		Label: "edge", FontSize: 16, FontFamily: "default", LabelWidth: 32, LabelHeight: 18,
	}
	connection.Fill = "#eeeeee"
	connection.LabelPosition = "INSIDE_MIDDLE_CENTER"

	pad := int64(0)
	normal, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	sketchOptions := testSketchOptions(&pad)
	sketch, err := Build(context.Background(), diagram, sketchOptions)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := Build(context.Background(), diagram, sketchOptions)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sketch, repeated) {
		t.Fatal("repeated sketch Build produced a different seeded scene")
	}
	if sketch.ViewBox != normal.ViewBox || sketch.LogicalWidth != normal.LogicalWidth || sketch.LogicalHeight != normal.LogicalHeight {
		t.Fatalf("sketch bounds = %+v/%vx%v, normal = %+v/%vx%v", sketch.ViewBox, sketch.LogicalWidth, sketch.LogicalHeight, normal.ViewBox, normal.LogicalWidth, normal.LogicalHeight)
	}

	normalGroup := findSceneNode(t, normal.Root, connection.ID)
	sketchGroup := findSceneNode(t, sketch.Root, connection.ID)
	if sketchGroup.ID != normalGroup.ID || !reflect.DeepEqual(sketchGroup.Classes, normalGroup.Classes) || sketchGroup.Opacity != normalGroup.Opacity {
		t.Fatalf("sketch connection metadata = %+v, normal = %+v", sketchGroup, normalGroup)
	}
	if len(sketchGroup.Children) != len(normalGroup.Children) {
		t.Fatalf("sketch/normal connection child counts = %d/%d", len(sketchGroup.Children), len(normalGroup.Children))
	}
	for index := range normalGroup.Children {
		if sketchGroup.Children[index].ID != normalGroup.Children[index].ID {
			t.Fatalf("connection child %d IDs = %q/%q", index, sketchGroup.Children[index].ID, normalGroup.Children[index].ID)
		}
	}

	normalGeometry := findSceneNode(t, normalGroup, connection.ID+":geometry")
	sketchGeometry := findSceneNode(t, sketchGroup, connection.ID+":geometry")
	if normalGeometry.Mask == nil || sketchGeometry.Mask == nil || normalGeometry.Mask.Type != sketchGeometry.Mask.Type {
		t.Fatalf("sketch/normal masks = %+v/%+v", sketchGeometry.Mask, normalGeometry.Mask)
	}
	wantGeometryIDs := []string{connection.ID + ":path", connection.ID + ":src-arrowhead", connection.ID + ":dst-arrowhead"}
	assertChildIDs(t, normalGeometry, wantGeometryIDs)
	assertChildIDs(t, sketchGeometry, wantGeometryIDs)

	normalRoute := findSceneNode(t, normalGeometry, connection.ID+":path").Primitive.(d2scene.Path)
	sketchRoute := findSceneNode(t, sketchGeometry, connection.ID+":path").Primitive.(d2scene.Path)
	if reflect.DeepEqual(sketchRoute.Commands, normalRoute.Commands) || len(sketchRoute.Commands) <= len(normalRoute.Commands) {
		t.Fatalf("sketch route did not expand typed rough geometry: sketch=%d normal=%d", len(sketchRoute.Commands), len(normalRoute.Commands))
	}
	if !reflect.DeepEqual(sketchRoute.Stroke, normalRoute.Stroke) || sketchRoute.Fill != normalRoute.Fill {
		t.Fatalf("sketch route paint changed: sketch=%+v normal=%+v", sketchRoute, normalRoute)
	}
	for _, endpoint := range []string{"src", "dst"} {
		arrow := findSceneNode(t, sketchGeometry, connection.ID+":"+endpoint+"-arrowhead")
		if _, ok := arrow.Primitive.(d2scene.Path); !ok {
			t.Fatalf("%s sketch arrowhead primitive = %T, want typed Path", endpoint, arrow.Primitive)
		}
	}

	// Geometry is the only connection subtree allowed to differ. Label fill and
	// text remain byte-for-byte identical scene data.
	if !reflect.DeepEqual(sketchGroup.Children[1:], normalGroup.Children[1:]) {
		t.Fatal("sketch mode changed connection label nodes")
	}
}

func TestBuildAnimatedSketchConnectionKeepsTypedTracksAndExactRoutes(t *testing.T) {
	diagram := validDiagram()
	connection := &diagram.Connections[0]
	connection.Animated = true
	connection.SrcArrow = d2target.TriangleArrowhead
	connection.DstArrow = d2target.TriangleArrowhead
	connection.Text = d2target.Text{
		Label: "edge", FontSize: 16, FontFamily: "default", LabelWidth: 32, LabelHeight: 18,
	}
	connection.Fill = "#eeeeee"
	connection.LabelPosition = "INSIDE_MIDDLE_CENTER"
	pad := int64(0)

	normal, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatal(err)
	}
	sketch, err := Build(context.Background(), diagram, testSketchOptions(&pad))
	if err != nil {
		t.Fatal(err)
	}
	normalGeometry := findSceneNode(t, normal.Root, connection.ID+":geometry")
	sketchGeometry := findSceneNode(t, sketch.Root, connection.ID+":geometry")
	wantIDs := []string{
		connection.ID + ":path:reverse", connection.ID + ":src-arrowhead",
		connection.ID + ":path:forward", connection.ID + ":dst-arrowhead",
	}
	assertChildIDs(t, normalGeometry, wantIDs)
	assertChildIDs(t, sketchGeometry, wantIDs)
	for _, suffix := range []string{":path:reverse", ":path:forward"} {
		normalPath := findSceneNode(t, normalGeometry, connection.ID+suffix)
		sketchPath := findSceneNode(t, sketchGeometry, connection.ID+suffix)
		if !reflect.DeepEqual(sketchPath.Primitive, normalPath.Primitive) || !reflect.DeepEqual(sketchPath.Animations, normalPath.Animations) {
			t.Fatalf("animated sketch route %q changed typed geometry or track", suffix)
		}
		if len(sketchPath.Animations) != 1 {
			t.Fatalf("animated sketch route %q tracks = %d, want one", suffix, len(sketchPath.Animations))
		}
	}
	if reflect.DeepEqual(
		findSceneNode(t, normalGeometry, connection.ID+":src-arrowhead").Primitive,
		findSceneNode(t, sketchGeometry, connection.ID+":src-arrowhead").Primitive,
	) {
		t.Fatal("animated sketch connection did not roughen its arrowhead")
	}
}

func TestBuildSketchConnectionBudgetsAreExplicitAndPreGeneration(t *testing.T) {
	diagram := validDiagram()
	pad := int64(0)
	_, err := Build(context.Background(), diagram, Options{Pad: &pad, Sketch: true})
	if err == nil || !strings.Contains(err.Error(), "positive MaxOperationSets") {
		t.Fatalf("missing sketch budget error = %v", err)
	}
	// Keep this test scoped to the connection's pre-generation limit now that
	// ordinary shapes also consume the shared document-wide sketch budget.
	for index := range diagram.Shapes {
		diagram.Shapes[index].Opacity = 0
	}

	options := testSketchOptions(&pad)
	options.SketchBudget.MaxOperations = 3
	_, err = Build(context.Background(), diagram, options)
	if err == nil || !strings.Contains(err.Error(), "input expands beyond operation limit 3") {
		t.Fatalf("bounded connection generation error = %v", err)
	}

	options = testSketchOptions(&pad)
	options.SketchBudget.MaxPathCommands = 3
	_, err = Build(context.Background(), diagram, options)
	if err == nil || !strings.Contains(err.Error(), "input expands beyond operation limit 3") {
		t.Fatalf("bounded retained connection error = %v", err)
	}
}

func TestBuildSketchSupportsEveryArrowheadAtBothEndpoints(t *testing.T) {
	for _, arrowhead := range allArrowheads {
		t.Run(string(arrowhead), func(t *testing.T) {
			diagram := validDiagram()
			diagram.Connections[0].SrcArrow = arrowhead
			diagram.Connections[0].DstArrow = arrowhead
			pad := int64(0)
			document, err := Build(context.Background(), diagram, testSketchOptions(&pad))
			if err != nil {
				t.Fatal(err)
			}
			connection := findSceneNode(t, document.Root, "a-b")
			if arrowhead == d2target.NoArrowhead {
				assertChildIDs(t, connection, []string{"a-b:path"})
				return
			}
			assertChildIDs(t, connection, []string{"a-b:path", "a-b:src-arrowhead", "a-b:dst-arrowhead"})
			for _, endpoint := range []string{"src", "dst"} {
				arrow := findSceneNode(t, connection, "a-b:"+endpoint+"-arrowhead")
				if _, ok := arrow.Primitive.(d2scene.Path); !ok {
					t.Fatalf("%s primitive = %T, want Path", endpoint, arrow.Primitive)
				}
			}
		})
	}
}

func testSketchOptions(pad *int64) Options {
	return Options{
		Pad:    pad,
		Sketch: true,
		SketchBudget: SketchBudget{
			MaxOperationSets: 1_000,
			MaxOperations:    100_000,
			MaxPathCommands:  100_000,
		},
	}
}
