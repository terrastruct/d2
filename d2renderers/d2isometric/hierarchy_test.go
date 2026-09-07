package d2isometric

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func hierarchyDiagram() *d2target.Diagram {
	d := d2target.NewDiagram()
	outer := testNode(`"data.zone"`, -100, 50)
	outer.Width, outer.Height = 600, 500
	outer.LabelWidth, outer.LabelHeight = 350, 32
	inner := testNode(`"data.zone".private`, -40, 140)
	inner.Width, inner.Height = 300, 240
	inner.LabelHeight = 24
	table := testNode(`"data.zone".private.accounts`, 0, 200)
	table.Type = d2target.ShapeSQLTable
	table.Width, table.Height = 130, 90
	table.Columns = []d2target.SQLColumn{{Name: d2target.Text{Label: "account_id"}, Type: d2target.Text{Label: "uuid"}, Constraint: []string{"primary_key"}}}
	worker := testNode(`"data.zone".worker`, 300, 350)
	worker.Type = d2target.ShapeClass
	worker.Fields = []d2target.ClassField{{Name: "jobs", Type: "[]Job"}}
	root := testNode("client", 800, -200)
	root.Width, root.Height = 170, 120
	// Children deliberately precede their parents: containment cannot depend on
	// export order, even though the original node order must remain unchanged.
	d.Shapes = []d2target.Shape{table, worker, inner, outer, root}
	edge := testEdge("query", root.ID, table.ID)
	edge.Route = []*geo.Point{{X: 800, Y: -140}, {X: 690, Y: -140}, {X: 690, Y: 245}, {X: 130, Y: 245}}
	edge.SrcArrow = d2target.FilledDiamondArrowhead
	edge.SrcLabel = &d2target.Text{Label: "many"}
	edge.DstLabel = &d2target.Text{Label: "one"}
	edge.Link, edge.Tooltip = "https://example.test/query", "Read accounts"
	feedback := testEdge("feedback", outer.ID, root.ID)
	feedback.SrcArrow, feedback.DstArrow = d2target.TriangleArrowhead, d2target.NoArrowhead
	feedback.Route = []*geo.Point{{X: 500, Y: 100}, {X: 885, Y: 100}, {X: 885, Y: -80}}
	d.Connections = []d2target.Connection{edge, feedback}
	return d
}

func TestContainerHierarchyPreservesCompiledLayout(t *testing.T) {
	d := hierarchyDiagram()
	s, err := BuildScene(d, &RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if s.PixelScale != .01 || len(s.Boards) != 3 {
		t.Fatalf("container board identity lost: %#v", s.Boards)
	}
	boards := map[string]Board{}
	for _, b := range s.Boards {
		boards[b.ID] = b
		if b.Position.Y != 0 || b.HeaderDepth > b.Size.Z {
			t.Fatal("container left source plane or header enlarged footprint")
		}
	}
	for i, n := range s.Nodes {
		original := d.Shapes[i]
		if n.ID != original.ID || !bytes.Equal(jsonBytes(t, n.Metadata.Original), jsonBytes(t, original)) {
			t.Fatal("node identity, order or original metadata changed")
		}
		want := Vec3{(float64(original.Pos.X) + float64(original.Width)/2) * .01, .07 + moduleSize(original).Y/2, (float64(original.Pos.Y) + float64(original.Height)/2) * .01}
		if n.Container {
			want.Y = 0
			b := boards[n.BoardID]
			if b.Position != want || b.Size != (Vec3{float64(original.Width) * .01, .14, float64(original.Height) * .01}) || n.Position != b.Position || n.Size != b.Size {
				t.Fatalf("container %s footprint was recomposed: %#v", n.ID, b)
			}
		} else if n.Size != moduleSize(original) {
			t.Fatalf("leaf %s lost source dimensions or physical subtype", n.ID)
		}
		if n.Position != want {
			t.Fatalf("%s moved: %#v != %#v", n.ID, n.Position, want)
		}
		if n.ParentID != "" {
			parent := boards["@container:"+n.ParentID]
			if n.Position.X-n.Size.X/2 < parent.Position.X-parent.Size.X/2 || n.Position.X+n.Size.X/2 > parent.Position.X+parent.Size.X/2 || n.Position.Z-n.Size.Z/2 < parent.Position.Z-parent.Size.Z/2 || n.Position.Z+n.Size.Z/2 > parent.Position.Z+parent.Size.Z/2 {
				t.Fatalf("%s no longer fits its compiled parent", n.ID)
			}
		}
	}
	outer, inner := boards[`@container:"data.zone"`], boards[`@container:"data.zone".private`]
	if outer.Kind != "platform" || outer.ParentID != "" || outer.Level != 0 || inner.Kind != "group" || inner.ParentID != outer.ID || inner.Level != 1 {
		t.Fatal("quoted hierarchy was flattened or assigned invented dependency stages")
	}
	if !reflect.DeepEqual(outer.NodeIDs, []string{`"data.zone".worker`, `"data.zone".private`}) || !reflect.DeepEqual(inner.NodeIDs, []string{`"data.zone".private.accounts`}) {
		t.Fatal("direct membership omitted nested group or included grandchildren")
	}
	if root := boards["@ungrouped"]; root.Kind != "ungrouped" || root.Label != "" || root.SourceID != "" || !reflect.DeepEqual(root.NodeIDs, []string{"client"}) {
		t.Fatal("root leaf gained a fictitious visible dependency container")
	}
	for i, e := range s.Edges {
		original := d.Connections[i]
		if e.ID != original.ID || e.Source != original.Src || e.Target != original.Dst || e.SourceArrow != original.SrcArrow || e.TargetArrow != original.DstArrow || !bytes.Equal(jsonBytes(t, e.Metadata.Original), jsonBytes(t, original)) {
			t.Fatal("source edge semantics changed")
		}
		if len(e.Points) != len(original.Route) {
			t.Fatal("source polyline was rerouted")
		}
		for j, p := range e.Points {
			if p != (Vec3{original.Route[j].X * .01, .08, original.Route[j].Y * .01}) {
				t.Fatal("source route point or direction changed")
			}
		}
	}
}

func TestContainerHierarchyOwnsDataAndIsDeterministic(t *testing.T) {
	d := hierarchyDiagram()
	before := jsonBytes(t, d)
	s, err := BuildScene(d, &RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	first := jsonBytes(t, s)
	for range 3 {
		next, err := BuildScene(d, &RenderOpts{})
		if err != nil || !bytes.Equal(first, jsonBytes(t, next)) {
			t.Fatal("hierarchy composition is not deterministic", err)
		}
	}
	s.Edges[0].Points[0].X = 999
	s.Edges[0].Metadata.Original.Route[0].X = 998
	s.Edges[0].SourceLabel.Label = "changed"
	s.Nodes[0].Metadata.Original.Columns[0].Constraint[0] = "changed"
	s.Nodes[1].Metadata.Original.Fields[0].Name = "changed"
	if !bytes.Equal(before, jsonBytes(t, d)) {
		t.Fatal("hierarchy scene aliases or mutates original target")
	}
}

func TestDefaultOptionsPreserveCompiledLayout(t *testing.T) {
	d := hierarchyDiagram()
	implicit, err := BuildScene(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := BuildScene(d, &RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(jsonBytes(t, implicit), jsonBytes(t, explicit)) {
		t.Fatal("default options changed source-preserving composition")
	}
	for i, n := range implicit.Nodes {
		want := hierarchyCenter(d.Shapes[i])
		if n.Position.X != want.X || n.Position.Z != want.Z {
			t.Fatalf("default composition moved %s from its compiled position", n.ID)
		}
	}
}

func TestContainerHierarchyFloatingAndAbsentRoutes(t *testing.T) {
	d := d2target.NewDiagram()
	d.Shapes = []d2target.Shape{testNode("a", -100, -100), testNode("b", 200, 30)}
	lifeline := testEdge("lifeline", "a", "generated-lifeline-end")
	lifeline.Route = []*geo.Point{{X: -50, Y: -30}, {X: -50, Y: 300}}
	straight, loop := testEdge("ab", "a", "b"), testEdge("loop", "b", "b")
	straight.Route, loop.Route = nil, nil
	d.Connections = []d2target.Connection{lifeline, straight, loop}
	s, err := BuildScene(d, &RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Boards) != 1 || s.Boards[0].Kind != "ungrouped" || s.Boards[0].Label != "" || len(s.Warnings) != 1 {
		t.Fatal("root-only hierarchy or explicit fallback information lost")
	}
	if s.Edges[0].Target != "generated-lifeline-end" || !reflect.DeepEqual(s.Edges[0].Points, []Vec3{{-.5, .08, -.3}, {-.5, .08, 3}}) {
		t.Fatal("generated sequence endpoint route was replaced")
	}
	for _, e := range s.Edges {
		if len(e.Points) < 2 || e.Points[0] == e.Points[len(e.Points)-1] {
			t.Fatal("fallback has no direction or collapsed the self-loop")
		}
		for _, p := range e.Points {
			if p.Y != .08 || math.IsNaN(p.X+p.Z) || math.IsInf(p.X+p.Z, 0) {
				t.Fatal("fallback is not finite and flat")
			}
		}
	}
	d.Connections[0].Route = nil
	if _, err := BuildScene(d, &RenderOpts{}); err == nil {
		t.Fatal("unknown endpoint without any source route was invented")
	}
}

func TestContainerHierarchyZeroDimensionsAndLargeCoordinates(t *testing.T) {
	d := d2target.NewDiagram()
	p := testNode("p", 1_000_000_000, -1_000_000_000)
	p.Width, p.Height = 0, 0
	p.LabelWidth, p.LabelHeight = 1_000_000_000, 1_000_000_000
	c := testNode("p.tiny", p.Pos.X, p.Pos.Y)
	c.Width, c.Height = 1, 0
	d.Shapes = []d2target.Shape{p, c}
	s, err := BuildScene(d, &RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range s.Nodes {
		if n.Size.X != .01 || n.Size.Z != .01 || math.Abs(n.Position.X) > coordinateLimit || math.Abs(n.Position.Z) > coordinateLimit {
			t.Fatal("zero fallback or safe scaled bounds changed")
		}
	}
	if s.Boards[0].HeaderDepth != .01 || s.Boards[0].Size.X != .01 {
		t.Fatal("oversized header changed the original footprint")
	}
	empty, err := BuildScene(d2target.NewDiagram(), &RenderOpts{})
	if err != nil || len(empty.Boards) != 0 {
		t.Fatal("empty hierarchy should not create a fictitious board", err)
	}
}

func TestContainerHierarchyCubicRouteIsSampledFaithfully(t *testing.T) {
	d := d2target.NewDiagram()
	d.Shapes = []d2target.Shape{testNode("a", -100, 0), testNode("b", 100, 0)}
	c := testEdge("curve", "a", "b")
	c.IsCurve = true
	c.Route = []*geo.Point{{X: 0, Y: 0}, {X: 0, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: 0}}
	d.Connections = []d2target.Connection{c}
	s, err := BuildScene(d, &RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	points := s.Edges[0].Points
	if len(points) <= 4 || points[0] != (Vec3{0, .08, 0}) || points[len(points)-1] != (Vec3{1, .08, 0}) {
		t.Fatal("cubic control points were treated as a polyline")
	}
	for _, p := range points {
		if p.Y != .08 || p.Z > .75 || p.X < 0 || p.X > 1 {
			t.Fatal("curve geometry escaped true cubic bounds")
		}
	}
	for i := range 101 {
		u := float64(i) / 100
		p := Vec3{3*(1-u)*u*u + u*u*u, .08, 3*(1-u)*(1-u)*u + 3*(1-u)*u*u}
		closest := math.Inf(1)
		for j := 1; j < len(points); j++ {
			closest = math.Min(closest, hierarchyControlDistance(p, points[j-1], points[j]))
		}
		if closest > .0025 {
			t.Fatalf("curve exceeds quarter-pixel error: %g", closest)
		}
	}
	if !bytes.Equal(jsonBytes(t, s.Edges[0].Metadata.Original), jsonBytes(t, c)) {
		t.Fatal("curve controls or original IsCurve metadata changed")
	}
	c.Route = c.Route[:3]
	d.Connections[0] = c
	if _, err := BuildScene(d, &RenderOpts{}); err == nil || !strings.Contains(err.Error(), "cubic control point") {
		t.Fatal("malformed cubic must fail explicitly", err)
	}
	c.Route = []*geo.Point{{X: 0, Y: 0}, {X: 0, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: 0}}
	count := maxEntries - 1
	if _, err := hierarchyRoute(c, &count); err == nil || count != maxEntries {
		t.Fatal("flattening ignored shared point budget", err)
	}
}
