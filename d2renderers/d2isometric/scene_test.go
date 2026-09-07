package d2isometric

import (
	"bytes"
	"encoding/json"
	"math"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func testNode(id string, x, y int) d2target.Shape {
	s := *d2target.BaseShape()
	s.ID = id
	s.Type = d2target.ShapeRectangle
	s.Label = "Label " + id
	s.Pos = d2target.Point{X: x, Y: y}
	s.Width = 100
	s.Height = 70
	s.Fill = "N6"
	s.Stroke = "N1"
	return s
}
func testEdge(id, src, dst string) d2target.Connection {
	c := *d2target.BaseConnection()
	c.ID = id
	c.Src = src
	c.Dst = dst
	c.DstArrow = d2target.TriangleArrowhead
	c.Label = "edge " + id
	c.Route = []*geo.Point{{X: 1, Y: 2}, {X: 50, Y: 90}}
	return c
}
func jsonBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestScenePreservesMetadataAndOwnsData(t *testing.T) {
	d := d2target.NewDiagram()
	table := testNode("db", 0, 0)
	table.Type = d2target.ShapeSQLTable
	table.Columns = []d2target.SQLColumn{{Name: d2target.Text{Label: "account_id"}, Type: d2target.Text{Label: "uuid"}, Constraint: []string{"primary_key"}, Reference: "users.id"}}
	class := testNode("worker", 0, 100)
	class.Type = d2target.ShapeClass
	class.Fields = []d2target.ClassField{{Name: "id", Type: "uuid", Visibility: "private"}}
	class.Methods = []d2target.ClassMethod{{Name: "run()", Return: "error", Visibility: "public"}}
	c := testEdge("read", table.ID, class.ID)
	c.SrcArrow = d2target.FilledDiamondArrowhead
	c.SrcLabel = &d2target.Text{Label: "many"}
	c.DstLabel = &d2target.Text{Label: "one"}
	c.Link = "https://example.test/doc"
	c.Tooltip = "Read path"
	d.Shapes = []d2target.Shape{table, class}
	d.Connections = []d2target.Connection{c}
	before := jsonBytes(t, d)
	s, err := BuildScene(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Nodes) != 2 || len(s.Edges) != 1 {
		t.Fatal("shape/connection cardinality changed")
	}
	for i, n := range s.Nodes {
		if !bytes.Equal(jsonBytes(t, n.Metadata.Original), jsonBytes(t, d.Shapes[i])) {
			t.Fatal("original shape metadata changed")
		}
	}
	if !bytes.Equal(jsonBytes(t, s.Edges[0].Metadata.Original), jsonBytes(t, c)) || s.Edges[0].SourceArrow != c.SrcArrow || s.Edges[0].TargetLabel.Label != "one" {
		t.Fatal("connection semantics changed")
	}
	s.Nodes[0].Metadata.Original.Columns[0].Constraint[0] = "changed"
	s.Nodes[1].Metadata.Original.Fields[0].Name = "changed"
	s.Edges[0].Metadata.Original.Route[0].X = 444
	s.Edges[0].SourceLabel.Label = "changed"
	if !bytes.Equal(before, jsonBytes(t, d)) {
		t.Fatal("returned scene aliases the input")
	}
}

func TestQuotedContainmentAndFeedbackPreserveSource(t *testing.T) {
	d := d2target.NewDiagram()
	d.Shapes = []d2target.Shape{testNode(`"a.b"`, 0, 0), testNode(`"a.b".x`, 0, 20), testNode("a", 0, 80), testNode("a.b", 0, 100), testNode("a.b.c", 0, 120)}
	c := testEdge("feedback", `"a.b"`, "a.b")
	c.SrcArrow = d2target.TriangleArrowhead
	d.Connections = []d2target.Connection{c}
	s, err := BuildScene(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Nodes[1].ParentID != `"a.b"` || s.Nodes[3].ParentID != "a" || s.Nodes[4].ParentID != "a.b" {
		t.Fatalf("incorrect quoted containment: %#v", s.Nodes)
	}
	if s.Boards[0].Level != 0 || s.Boards[1].Level != 0 || s.Boards[2].Level != 1 {
		t.Fatal("container depth does not match authored nesting")
	}

	if len(s.Boards) != 3 || !s.Nodes[0].Container || !s.Nodes[2].Container || !s.Nodes[3].Container {
		t.Fatal("container identity lost")
	}
	if s.Edges[0].Source != `"a.b"` || s.Edges[0].Target != "a.b" || len(s.Edges[0].Points) < 2 {
		t.Fatal("container endpoints lost")
	}
}

func TestParallelSelfLoopsAndFloatingEndpoints(t *testing.T) {
	d := d2target.NewDiagram()
	d.Shapes = []d2target.Shape{testNode("a", 0, 0), testNode("b", 0, 100)}
	d.Connections = []d2target.Connection{testEdge("one", "a", "b"), testEdge("two", "a", "b"), testEdge("three", "a", "b"), testEdge("loop", "b", "b"), testEdge("lifeline", "a", "a-lifeline-end")}
	s, err := BuildScene(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, edge := range s.Edges {
		if len(edge.Points) != len(d.Connections[i].Route) {
			t.Fatal("compiled parallel/self-loop route was replaced")
		}
		for j, p := range edge.Points {
			original := d.Connections[i].Route[j]
			if p.X != original.X*SceneScale || p.Z != original.Y*SceneScale {
				t.Fatal("compiled route geometry changed")
			}
		}
	}
	if s.Edges[4].Target != "a-lifeline-end" || len(s.Edges[4].Points) != 2 {
		t.Fatal("floating endpoint removed")
	}
	for _, e := range s.Edges {
		for _, p := range e.Points {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsNaN(p.Z) {
				t.Fatal("nonfinite route")
			}
		}
	}
}

func TestStableSceneAndSourceOrdering(t *testing.T) {
	d := d2target.NewDiagram()
	d.Shapes = []d2target.Shape{testNode("c", 0, 200), testNode("a", 0, 0), testNode("b", 0, 100)}
	a, err := BuildScene(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Boards[0].NodeIDs, []string{"c", "a", "b"}) {
		t.Fatal("scene changed source node ordering")
	}
	for i := 0; i < 5; i++ {
		b, e := BuildScene(d, nil)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(jsonBytes(t, a), jsonBytes(t, b)) {
			t.Fatal("nondeterministic scene")
		}
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*d2target.Diagram)
	}{
		{"duplicate node", func(d *d2target.Diagram) { d.Shapes = append(d.Shapes, d.Shapes[0]) }},
		{"nonfinite route", func(d *d2target.Diagram) { d.Connections[0].Route[0].X = math.Inf(1) }},
		{"nil route point", func(d *d2target.Diagram) { d.Connections[0].Route[0] = nil }},
		{"nonfinite ratio", func(d *d2target.Diagram) { v := math.NaN(); d.Shapes[0].ContentAspectRatio = &v }},
		{"negative size", func(d *d2target.Diagram) { d.Shapes[0].Width = -1 }},
		{"missing endpoint", func(d *d2target.Diagram) { d.Connections[0].Dst = "absent"; d.Connections[0].Route = nil }},
		{"duplicate edge", func(d *d2target.Diagram) { d.Connections = append(d.Connections, d.Connections[0]) }},
		{"text budget", func(d *d2target.Diagram) { d.Shapes[0].Label = strings.Repeat("x", maxBytes+1) }},
		{"node budget", func(d *d2target.Diagram) { d.Shapes = make([]d2target.Shape, MaxNodes+1) }},
		{"invalid UTF8", func(d *d2target.Diagram) { d.Shapes[0].Label = string([]byte{0xff}) }},
		{"ID size", func(d *d2target.Diagram) { d.Shapes[0].ID = strings.Repeat("x", MaxIDBytes+1) }},
		{"ID depth", func(d *d2target.Diagram) { d.Shapes[0].ID = strings.Repeat("x.", MaxIDDepth) + "x" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := d2target.NewDiagram()
			d.Shapes = []d2target.Shape{testNode("a", 0, 0)}
			d.Connections = []d2target.Connection{testEdge("loop", "a", "a")}
			tt.change(d)
			if _, e := BuildScene(d, nil); e == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	if _, e := BuildScene(nil, nil); e == nil {
		t.Fatal("nil diagram accepted")
	}
}

func TestIconURLIdentityAndOwnership(t *testing.T) {
	d := d2target.NewDiagram()
	s := testNode("a", 0, 0)
	s.Icon, _ = url.Parse("https://user:pass@example.test/icon.svg?variant=1")
	d.Shapes = []d2target.Shape{s}
	scene, e := BuildScene(d, nil)
	if e != nil {
		t.Fatal(e)
	}
	if scene.Nodes[0].Icon != s.Icon.String() {
		t.Fatal("icon URL identity changed")
	}
	if scene.Nodes[0].Metadata.Original.Icon == s.Icon || scene.Nodes[0].Metadata.Original.Icon.User == s.Icon.User {
		t.Fatal("icon metadata aliases the source")
	}
}

func TestSceneTextRoundTrip(t *testing.T) {
	sourceText := `</script><script>window.pwned=true</script><img src=x onerror=alert(1)>` + "\u2028"
	d := d2target.NewDiagram()
	n := testNode("a", 0, 0)
	n.Label = sourceText
	n.Tooltip = sourceText
	n.Link = "javascript:alert(1)"
	d.Shapes = []d2target.Shape{n}
	scene, e := BuildScene(d, nil)
	if e != nil {
		t.Fatal(e)
	}
	var s Scene
	if e = json.Unmarshal(jsonBytes(t, scene), &s); e != nil {
		t.Fatal(e)
	}
	if s.Nodes[0].Label != sourceText || s.Nodes[0].Tooltip != sourceText || s.Nodes[0].Link != n.Link {
		t.Fatal("scene encoding changed source text")
	}
}

func TestAuthoredPaintAndThemeOverrides(t *testing.T) {
	d := d2target.NewDiagram()
	d.Shapes = []d2target.Shape{testNode("default", 0, 0), testNode("literal", 0, 100)}
	d.Shapes[1].Fill = "#123456"
	d.Shapes[1].Color = "yellow"
	d.Shapes[1].Stroke = "#b02663"
	c := testEdge("paint", "default", "literal")
	c.Stroke = "N1"
	c.Color = "N1"
	d.Connections = []d2target.Connection{c}
	s, e := BuildScene(d, nil)
	if e != nil {
		t.Fatal(e)
	}
	if s.Nodes[0].FillExplicit || !s.Nodes[1].FillExplicit || s.Nodes[1].Fill != "#123456" {
		t.Fatal("paint provenance lost")
	}
	if s.Nodes[0].StrokeExplicit || !s.Nodes[1].StrokeExplicit || s.Nodes[1].Stroke != "#b02663" {
		t.Fatal("node stroke provenance lost")
	}
	if !s.Nodes[1].FontExplicit || s.Nodes[1].FontColor != "yellow" || s.Edges[0].StrokeExplicit || s.Edges[0].FontExplicit {
		t.Fatal("font or edge default provenance changed")
	}
	fill := "#abcdef"
	s, e = BuildScene(d, &RenderOpts{ThemeOverrides: &d2target.ThemeOverrides{N6: &fill}})
	if e != nil {
		t.Fatal(e)
	}
	if !s.Nodes[0].FillExplicit || s.Nodes[0].Fill != fill || s.Nodes[1].Fill != "#123456" {
		t.Fatal("theme override ignored or literal changed")
	}
	if !s.Nodes[0].StrokeExplicit || !s.Nodes[0].FontExplicit || !s.Edges[0].StrokeExplicit || !s.Edges[0].FontExplicit {
		t.Fatal("override provenance not passed to runtime")
	}
}
