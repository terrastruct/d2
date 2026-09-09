package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labelgeom"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func compoundWithPreservedRoutes(ctx context.Context, g *layoutgraph.Graph) (*layoutgraph.Graph, error) {
	candidate, err := CompoundCandidate(ctx, g)
	if err != nil || candidate == g {
		return candidate, err
	}
	return PreserveCompoundRoutes(ctx, g, candidate)
}

type compoundInteriorFixture struct {
	graph    *layoutgraph.Graph
	block    *layoutgraph.Node
	interior []*layoutgraph.Edge
	external []*layoutgraph.Edge
}

// The two branches share a routed bus, while a nested route and a loop retain
// independently chosen ports. All geometry is inside the rigid middle block.
func newCompoundInteriorFixture() compoundInteriorFixture {
	g := layoutgraph.NewGraph()
	g.Directions[nil] = geo.Right
	add := func(id layoutgraph.EntityID, parent *layoutgraph.Node, x, y, w, h float64) *layoutgraph.Node {
		n := layoutgraph.NewNode(id, w, h)
		n.TopLeft = geo.NewPoint(x, y)
		g.AddNewNodeToContainer(parent, n)
		return n
	}
	from := add(1, nil, 0, 180, 100, 80)
	block := add(2, nil, 400, 0, 900, 700)
	to := add(3, nil, 1600, 180, 100, 80)
	hub := add(4, block, 460, 200, 80, 60)
	upper := add(5, block, 850, 100, 100, 70)
	lower := add(6, block, 850, 330, 100, 70)
	nested := add(7, block, 1040, 90, 200, 450)
	first := add(8, nested, 1080, 160, 100, 60)
	second := add(9, nested, 1080, 400, 100, 60)
	g.Directions[block], g.Directions[nested] = geo.Right, geo.Bottom
	connect := func(a, b *layoutgraph.Node, points ...[2]float64) *layoutgraph.Edge {
		e := g.Connect(a, b)
		e.ID = layoutgraph.EntityID(100 + len(g.Edges))
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		for _, p := range points {
			e.Points = append(e.Points, geo.NewPoint(p[0], p[1]))
		}
		return e
	}
	busA := connect(hub, upper, [2]float64{540, 230}, [2]float64{750, 230}, [2]float64{750, 135}, [2]float64{850, 135})
	busB := connect(hub, lower, [2]float64{540, 230}, [2]float64{750, 230}, [2]float64{750, 365}, [2]float64{850, 365})
	insideNested := connect(first, second, [2]float64{1180, 190}, [2]float64{1210, 190}, [2]float64{1210, 430}, [2]float64{1180, 430})
	loop := connect(upper, upper, [2]float64{950, 120}, [2]float64{990, 120}, [2]float64{990, 65}, [2]float64{900, 65}, [2]float64{900, 100})
	busA.Label = &layoutgraph.Label{Text: "upper branch", Position: label.UnlockedTop, Width: 72, Height: 16}
	busA.LabelPercentage = 0.25
	busB.Label = &layoutgraph.Label{Text: "lower branch", Position: label.UnlockedBottom, Width: 72, Height: 16}
	busB.LabelPercentage = 0.35
	busB.Label.FixPosition()
	insideNested.Label = &layoutgraph.Label{Text: "nested", Position: label.UnlockedMiddle, Width: 35, Height: 14}
	insideNested.LabelPercentage = 0.6
	insideNested.Label.FixPosition()
	insideNested.SourceArrowheadLabel = &layoutgraph.Label{Text: "out", Width: 20, Height: 12}
	insideNested.TargetArrowheadLabel = &layoutgraph.Label{Text: "in", Width: 15, Height: 12}
	loop.Label = &layoutgraph.Label{Text: "retry", Position: label.UnlockedTop, Width: 28, Height: 12}
	loop.LabelPercentage = 0.55
	in := connect(from, hub, [2]float64{100, 220}, [2]float64{350, 220}, [2]float64{350, 230}, [2]float64{460, 230})
	out := connect(second, to, [2]float64{1180, 430}, [2]float64{1450, 430}, [2]float64{1450, 220}, [2]float64{1600, 220})
	in.Label = &layoutgraph.Label{Text: "input", Position: label.UnlockedTop, Width: 32, Height: 12}
	in.LabelPercentage = 0.5
	return compoundInteriorFixture{g, block, []*layoutgraph.Edge{busA, busB, insideNested, loop}, []*layoutgraph.Edge{in, out}}
}

func compoundNodeByID(t *testing.T, g *layoutgraph.Graph, id layoutgraph.EntityID) *layoutgraph.Node {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("candidate lost node %d", id)
	return nil
}

func compoundEdgeByID(t *testing.T, g *layoutgraph.Graph, id layoutgraph.EntityID) *layoutgraph.Edge {
	t.Helper()
	for _, e := range g.Edges {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("candidate lost edge %d", id)
	return nil
}

func compoundRouteTranslated(a, b *layoutgraph.Edge, delta geo.Point) bool {
	if len(a.Points) != len(b.Points) {
		return false
	}
	for i, p := range a.Points {
		if p == nil || b.Points[i] == nil || b.Points[i].X != p.X+delta.X || b.Points[i].Y != p.Y+delta.Y {
			return false
		}
	}
	return true
}

func requireCompoundAttachedRoute(t *testing.T, e *layoutgraph.Edge) {
	t.Helper()
	onBorder := func(n *layoutgraph.Node, p *geo.Point) bool {
		return n.ContainsPointOnBox(p) && (p.X == n.TopLeft.X || p.X == n.TopLeft.X+n.Width || p.Y == n.TopLeft.Y || p.Y == n.TopLeft.Y+n.Height)
	}
	if len(e.Points) < 2 || !onBorder(e.From, e.Points[0]) || !onBorder(e.To, e.Points[len(e.Points)-1]) {
		t.Fatalf("edge %d has an incomplete or detached route: %v", e.ID, e.Points)
	}
}

func requireCompoundTranslatedPoint(t *testing.T, original, moved *geo.Point, delta geo.Point) {
	t.Helper()
	want := geo.Point{X: original.X + delta.X, Y: original.Y + delta.Y}
	if *moved != want {
		t.Fatalf("point = %v, want original %v translated by %v to %v", moved, original, delta, want)
	}
}

func TestCompoundCandidatePreservesInteriorBusNestedAndLoopRoutes(t *testing.T) {
	f := newCompoundInteriorFixture()
	before, err := layoutgraph.Clone(t.Context(), f.graph)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := compoundWithPreservedRoutes(t.Context(), f.graph)
	if err != nil {
		t.Fatal(err)
	}
	if candidate == f.graph {
		t.Fatal("fixture did not produce a compound candidate")
	}
	movedBlock := compoundNodeByID(t, candidate, f.block.ID)
	delta := geo.Point{X: movedBlock.TopLeft.X - f.block.TopLeft.X, Y: movedBlock.TopLeft.Y - f.block.TopLeft.Y}
	if delta == (geo.Point{}) {
		t.Fatal("fixture did not move the routed block")
	}
	for _, original := range f.interior {
		moved := compoundEdgeByID(t, candidate, original.ID)
		if !compoundRouteTranslated(original, moved, delta) {
			t.Errorf("interior edge %d lost its translated route: before %v, after %v, delta %v", original.ID, original.Points, moved.Points, delta)
		}
		if original.IsCurve != moved.IsCurve || !reflect.DeepEqual(original.SourceArrowheadLabel, moved.SourceArrowheadLabel) || !reflect.DeepEqual(original.TargetArrowheadLabel, moved.TargetArrowheadLabel) {
			t.Errorf("interior edge %d lost route or label metadata", original.ID)
		}
		if original.Label != nil && (original.Label.PositionFixed() || original.IsLoop()) {
			if original.LabelPercentage != moved.LabelPercentage || !reflect.DeepEqual(original.Label, moved.Label) {
				t.Errorf("interior edge %d changed its fixed or loop label", original.ID)
			}
			requireCompoundTranslatedPoint(t, original.LabelTopLeft(original.Label.Position, original.Label.Width, original.Label.Height), moved.LabelTopLeft(moved.Label.Position, moved.Label.Width, moved.Label.Height), delta)
		}
		for _, target := range []bool{false, true} {
			l := original.SourceArrowheadLabel
			if target {
				l = original.TargetArrowheadLabel
			}
			if l != nil {
				a := labelgeom.ArrowheadTopLeft(original.Points, target, string(original.SourceArrowhead), string(original.TargetArrowhead), l.Width, l.Height)
				b := labelgeom.ArrowheadTopLeft(moved.Points, target, string(moved.SourceArrowhead), string(moved.TargetArrowhead), l.Width, l.Height)
				requireCompoundTranslatedPoint(t, a, b, delta)
			}
		}
		requireCompoundAttachedRoute(t, moved)
	}
	for _, original := range f.external {
		moved := compoundEdgeByID(t, candidate, original.ID)
		requireCompoundAttachedRoute(t, moved)
		if compoundRouteTranslated(original, moved, delta) {
			t.Errorf("external edge %d was translated with only one endpoint's block", original.ID)
		}
	}
	for _, original := range f.graph.Nodes {
		if original == f.block || original.Container == nil {
			continue
		}
		moved := compoundNodeByID(t, candidate, original.ID)
		requireCompoundTranslatedPoint(t, original.TopLeft, moved.TopLeft, delta)
		if moved.Width != original.Width || moved.Height != original.Height || candidate.Direction(moved) != f.graph.Direction(original) {
			t.Errorf("node %d changed its rigid interior dimensions or direction", original.ID)
		}
	}
	requireGraphsSerializeEqual(t.Context(), t, before, f.graph)
	again, err := compoundWithPreservedRoutes(t.Context(), f.graph)
	if err != nil {
		t.Fatal(err)
	}
	requireGraphsSerializeEqual(t.Context(), t, candidate, again)
}

func TestCompoundCandidateKeepsSelectedRouteForUnsafeInterior(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*layoutgraph.Edge, *layoutgraph.Node)
	}{
		{"incomplete", func(e *layoutgraph.Edge, _ *layoutgraph.Node) { e.Points = e.Points[:1] }},
		{"detached", func(e *layoutgraph.Edge, _ *layoutgraph.Node) { e.Points[0] = e.From.Center() }},
		{"escaped", func(e *layoutgraph.Edge, _ *layoutgraph.Node) {
			e.Points = []*geo.Point{e.Points[0], geo.NewPoint(750, -180), geo.NewPoint(850, -180), e.Points[len(e.Points)-1]}
		}},
		{"subpixel-escape", func(e *layoutgraph.Edge, block *layoutgraph.Node) {
			e.Label = nil
			e.Points = []*geo.Point{e.Points[0], geo.NewPoint(750, block.TopLeft.Y-0.1), geo.NewPoint(850, block.TopLeft.Y-0.1), e.Points[len(e.Points)-1]}
		}},
		{"escaped-curve-control", func(e *layoutgraph.Edge, block *layoutgraph.Node) {
			e.IsCurve = true
			e.Label = nil
			e.Points = []*geo.Point{e.Points[0], geo.NewPoint(750, block.TopLeft.Y-0.1), geo.NewPoint(750, 135), e.Points[len(e.Points)-1]}
		}},
		{"escaped-label", func(e *layoutgraph.Edge, _ *layoutgraph.Node) {
			e.Points = []*geo.Point{e.Points[0], geo.NewPoint(700, 230), geo.NewPoint(700, 8), geo.NewPoint(850, 8), e.Points[len(e.Points)-1]}
			e.Label = &layoutgraph.Label{Text: "outside", Position: label.UnlockedTop, Width: 32, Height: 16}
			e.LabelPercentage = 457. / 659.
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCompoundInteriorFixture()
			original := f.interior[0]
			test.change(original, f.block)
			before, err := layoutgraph.Clone(t.Context(), f.graph)
			if err != nil {
				t.Fatal(err)
			}
			selected, err := CompoundCandidate(t.Context(), f.graph)
			if err != nil {
				t.Fatal(err)
			}
			if selected == f.graph {
				t.Fatal("fixture did not produce a compound candidate")
			}
			candidate, err := PreserveCompoundRoutes(t.Context(), f.graph, selected)
			if err != nil {
				t.Fatal(err)
			}
			movedBlock := compoundNodeByID(t, candidate, f.block.ID)
			delta := geo.Point{X: movedBlock.TopLeft.X - f.block.TopLeft.X, Y: movedBlock.TopLeft.Y - f.block.TopLeft.Y}
			moved := compoundEdgeByID(t, candidate, original.ID)
			if compoundRouteTranslated(original, moved, delta) {
				t.Fatal("unsafe route was reused by rigid translation")
			}
			selectedBlock := compoundNodeByID(t, selected, f.block.ID)
			originShift := geo.Point{X: movedBlock.TopLeft.X - selectedBlock.TopLeft.X, Y: movedBlock.TopLeft.Y - selectedBlock.TopLeft.Y}
			selectedEdge := compoundEdgeByID(t, selected, original.ID)
			if !compoundRouteTranslated(selectedEdge, moved, originShift) || moved.IsCurve != selectedEdge.IsCurve {
				t.Fatal("refinement changed the selected route for an ineligible interior edge")
			}
			requireCompoundAttachedRoute(t, moved)
			requireGraphsSerializeEqual(t.Context(), t, before, f.graph)
		})
	}
}

func TestCompoundRefinementPreservesSelectedExternalRoutesAndCurves(t *testing.T) {
	for _, curved := range []bool{false, true} {
		name := "selected-polyline"
		if curved {
			name = "selected-bezier"
		}
		t.Run(name, func(t *testing.T) {
			f := newCompoundInteriorFixture()
			selected, err := CompoundCandidate(t.Context(), f.graph)
			if err != nil {
				t.Fatal(err)
			}
			if selected == f.graph {
				t.Fatal("fixture did not produce a selected compound placement")
			}
			if curved {
				// A completed selected result may contain curves. Preserve these
				// exact control points even when the earlier ordinary edge was not curved.
				edge := compoundEdgeByID(t, selected, f.external[0].ID)
				start, end := edge.Points[0], edge.Points[len(edge.Points)-1]
				edge.Points = []*geo.Point{start.Copy(), geo.NewPoint(start.X+(end.X-start.X)/3, start.Y), geo.NewPoint(start.X+2*(end.X-start.X)/3, end.Y), end.Copy()}
				edge.IsCurve = true
			}
			before, err := layoutgraph.Clone(t.Context(), selected)
			if err != nil {
				t.Fatal(err)
			}
			refined, err := PreserveCompoundRoutes(t.Context(), f.graph, selected)
			if err != nil {
				t.Fatal(err)
			}
			if refined == selected {
				t.Fatal("refinement must return an independent graph")
			}
			for _, original := range f.external {
				completed := compoundEdgeByID(t, selected, original.ID)
				actual := compoundEdgeByID(t, refined, original.ID)
				if !compoundRouteTranslated(completed, actual, geo.Point{}) || actual.IsCurve != completed.IsCurve {
					t.Fatalf("refinement redrew selected external edge %d: selected %v, actual %v, curve %v -> %v", original.ID, completed.Points, actual.Points, completed.IsCurve, actual.IsCurve)
				}
				requireCompoundAttachedRoute(t, actual)
			}
			for index, actual := range refined.Nodes {
				completed := selected.Nodes[index]
				if *actual.TopLeft != *completed.TopLeft || actual.Width != completed.Width || actual.Height != completed.Height {
					t.Fatalf("refinement changed selected placement of node %d", actual.ID)
				}
			}
			requireGraphsSerializeEqual(t.Context(), t, before, selected)
			if curved && !compoundEdgeByID(t, selected, f.external[0].ID).IsCurve {
				t.Fatal("refinement mutated the selected input's curve flag")
			}
		})
	}
}

func TestCompoundCandidatePreservesEnclosedBezierAndUnassignedEdgeIDs(t *testing.T) {
	for _, unsetIDs := range []bool{false, true} {
		name := "assigned-edge-ids"
		if unsetIDs {
			name = "unassigned-edge-ids"
		}
		t.Run(name, func(t *testing.T) {
			f := newCompoundInteriorFixture()
			curved := f.interior[2]
			curved.IsCurve = true
			curved.Points = []*geo.Point{geo.NewPoint(1180, 190), geo.NewPoint(1220, 190), geo.NewPoint(1220, 430), geo.NewPoint(1180, 430)}
			if unsetIDs {
				for _, e := range f.graph.Edges {
					e.ID = 0 // NewEdge/Connect permit this internal representation.
				}
			}
			candidate, err := compoundWithPreservedRoutes(t.Context(), f.graph)
			if err != nil {
				t.Fatal(err)
			}
			if candidate == f.graph {
				t.Fatal("fixture did not produce a compound candidate")
			}
			movedBlock := compoundNodeByID(t, candidate, f.block.ID)
			delta := geo.Point{X: movedBlock.TopLeft.X - f.block.TopLeft.X, Y: movedBlock.TopLeft.Y - f.block.TopLeft.Y}
			for index, original := range f.interior {
				moved := candidate.Edges[index]
				if moved.From.ID != original.From.ID || moved.To.ID != original.To.ID || !compoundRouteTranslated(original, moved, delta) || moved.IsCurve != original.IsCurve {
					t.Fatalf("route %d was aliased, redrawn, or lost curve semantics", index)
				}
			}
		})
	}
}

func TestCompoundCandidateCancellationPreservesRoutedInterior(t *testing.T) {
	f := newCompoundInteriorFixture()
	before, err := layoutgraph.Clone(t.Context(), f.graph)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := compoundWithPreservedRoutes(ctx, f.graph); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context cancellation", err)
	}
	requireGraphsSerializeEqual(t.Context(), t, before, f.graph)
}

func TestCompoundCandidateReconsidersNodeLabelAtReroutedPort(t *testing.T) {
	f := newCompoundInteriorFixture()
	hub := compoundNodeByID(t, f.graph, 4)
	hub.Label = &layoutgraph.Label{Text: "controller", Position: label.OutsideLeftMiddle, Width: 50, Height: 12}
	inside := compoundNodeByID(t, f.graph, 8)
	inside.Label = &layoutgraph.Label{Text: "fixed", Position: label.InsideTopCenter, Width: 25, Height: 12}
	inside.Label.FixPosition()
	inside.Icon = &layoutgraph.Icon{Position: label.InsideBottomRight}
	inside.Icon.FixPosition()
	candidate, err := compoundWithPreservedRoutes(t.Context(), f.graph)
	if err != nil {
		t.Fatal(err)
	}
	if candidate == f.graph {
		t.Fatal("fixture did not produce a compound candidate")
	}
	moved := compoundNodeByID(t, candidate, hub.ID)
	box := layoutgraph.NewNode(999, moved.Label.Width, moved.Label.Height)
	box.TopLeft = moved.LabelTopLeft(moved.Label.Position, moved.Label.Width, moved.Label.Height)
	for _, original := range f.external {
		edge := compoundEdgeByID(t, candidate, original.ID)
		for i := 1; i < len(edge.Points); i++ {
			if box.PassesThrough(edge.Points[i-1], edge.Points[i]) {
				t.Fatalf("external edge %d crosses the node label after compound routing", edge.ID)
			}
		}
	}
	if got := compoundNodeByID(t, candidate, inside.ID).Label; !reflect.DeepEqual(got, inside.Label) {
		t.Fatalf("authored node label changed: got %+v, want %+v", got, inside.Label)
	}
	if got := compoundNodeByID(t, candidate, inside.ID).Icon; !reflect.DeepEqual(got, inside.Icon) {
		t.Fatalf("authored icon position changed: got %+v, want %+v", got, inside.Icon)
	}
}

type compoundCancelAfterChecks struct {
	context.Context
	checks, limit int
}

func (ctx *compoundCancelAfterChecks) Err() error {
	ctx.checks++
	if ctx.limit > 0 && ctx.checks >= ctx.limit {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func TestCompoundCandidateMidflightCancellationPreservesRoutedInput(t *testing.T) {
	f := newCompoundInteriorFixture()
	probe := &compoundCancelAfterChecks{Context: t.Context()}
	if _, err := compoundWithPreservedRoutes(probe, f.graph); err != nil {
		t.Fatal(err)
	}
	before, err := layoutgraph.Clone(t.Context(), f.graph)
	if err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{probe.checks / 3, 2 * probe.checks / 3, probe.checks - 1} {
		if limit < 2 {
			t.Fatal("fixture did not enter compound work")
		}
		ctx := &compoundCancelAfterChecks{Context: t.Context(), limit: limit}
		candidate, err := compoundWithPreservedRoutes(ctx, f.graph)
		if !errors.Is(err, context.Canceled) || candidate != nil {
			t.Fatalf("cancel at check %d: candidate=%p err=%v", limit, candidate, err)
		}
		requireGraphsSerializeEqual(t.Context(), t, before, f.graph)
	}
}
