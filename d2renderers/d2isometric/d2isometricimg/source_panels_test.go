package d2isometricimg

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func sourcePanelFixture(t *testing.T, path string) *d2target.Diagram {
	t.Helper()
	data, err := os.ReadFile("../../../e2etests/testdata/" + path)
	if err != nil {
		t.Fatal(err)
	}
	var d d2target.Diagram
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func nativeFixtureScene(t *testing.T, d *d2target.Diagram) *nativeScene {
	t.Helper()
	s, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	n, err := newNativeScene(context.Background(), s, 640, 480)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func panelNode(n *d2scene.Node, id string) *d2scene.Node {
	if n.ID == id {
		return n
	}
	for _, child := range n.Children {
		if found := panelNode(child, id); found != nil {
			return found
		}
	}
	return nil
}

func TestNativeBuiltInLegendKeepsEveryVisibleEntry(t *testing.T) {
	d := sourcePanelFixture(t, "txtar/legend-mono/dagre/board.exp.json")
	before, _ := json.Marshal(d)
	n := nativeFixtureScene(t, d)
	if len(n.panels) != 1 || n.panels[0].document.Root.ID != "legend" {
		t.Fatalf("built-in legend missing from native output: %d panels", len(n.panels))
	}
	for _, id := range []string{"legend:title", "legend:shape:0", "legend:shape:0:label"} {
		if panelNode(n.panels[0].document.Root, id) == nil {
			t.Fatalf("legend lost %s", id)
		}
	}
	if _, err := n.Frame(context.Background(), 0, false); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("native legend mutated source target")
	}
}

func TestNativeSequencePreservesSourceCompositionAndAuthoredAnimation(t *testing.T) {
	d := sourcePanelFixture(t, "stable/sequence_diagram_groups/dagre/board.exp.json")
	before, _ := json.Marshal(d)
	s, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasSequence {
		t.Fatal("compiler-generated sequence not recognized")
	}
	n := nativeFixtureScene(t, d)
	if len(n.panels) != 0 || len(n.packets) != 0 {
		t.Fatal("sequence became a flat diagram panel or acquired traffic")
	}
	a, err := n.Frame(context.Background(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := n.Frame(context.Background(), .5, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("unanimated sequence acquired invented animation")
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("sequence geometry changed the source target")
	}
	d.Connections[0].Animated = true
	n = nativeFixtureScene(t, d)
	a, err = n.Frame(context.Background(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err = n.Frame(context.Background(), .25, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("authored sequence animation is frozen")
	}
}

func TestNativeRootLabelDescriptionAndFrameAreVisible(t *testing.T) {
	d := d2target.NewDiagram()
	shape := d2target.BaseShape()
	shape.ID, shape.Type, shape.Width, shape.Height = "component", d2target.ShapeRectangle, 200, 100
	shape.Label, shape.LabelWidth, shape.LabelHeight = "Component", 90, 24
	d.Shapes = []d2target.Shape{*shape}
	plain := nativeFixtureScene(t, d)
	d.Root.Label, d.Root.LabelWidth, d.Root.LabelHeight = "Authored title", 280, 44
	d.Root.FontSize, d.Root.Color = 30, "#8a1538"
	d.Root.StrokeWidth, d.Root.Stroke, d.Root.DoubleBorder = 2, "#296f85", true
	d.Description = "Authored description"
	decorated := nativeFixtureScene(t, d)
	if len(decorated.panels) != 1 || panelNode(decorated.panels[0].document.Root, "root:double-border:outer") == nil {
		t.Fatal("root border omitted")
	}
	a, _ := plain.Frame(context.Background(), 0, false)
	b, _ := decorated.Frame(context.Background(), 0, false)
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("root content had no visible effect")
	}
	if nativeMeshBounds(decorated.triangles).Height <= nativeMeshBounds(plain.triangles).Height {
		t.Fatal("root title/description did not reserve visible space")
	}
}
