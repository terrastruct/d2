package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func shortcutTestPoints(xy ...float64) []*geo.Point {
	points := make([]*geo.Point, 0, len(xy)/2)
	for i := 0; i < len(xy); i += 2 {
		points = append(points, geo.NewPoint(xy[i], xy[i+1]))
	}
	return points
}

func shortcutTestGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	from.TopLeft = geo.NewPoint(0, 200)
	to := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	to.TopLeft = geo.NewPoint(500, 0)
	e := g.Connect(from, to)
	e.Points = shortcutTestPoints(100, 250, 200, 250, 200, 150, 400, 150, 400, 50, 500, 50)
	return g, e
}

func TestShortcutRemovesMonotoneStaircaseBends(t *testing.T) {
	for _, transpose := range []bool{false, true} {
		g, e := shortcutTestGraph()
		if transpose {
			for _, n := range g.Nodes {
				n.TopLeft.X, n.TopLeft.Y = n.TopLeft.Y, n.TopLeft.X
			}
			for _, p := range e.Points {
				p.X, p.Y = p.Y, p.X
			}
		}
		before := append([]*geo.Point(nil), e.Points...)
		guard, _ := newRouteWorkGuard(context.Background(), "old simplify", maxRouteStageWorkUnits)
		oldResult, err := simplifyPoints(g, e, guard)
		if err != nil || len(oldResult) != len(before) {
			t.Fatalf("test no longer distinguishes the new monotone case: %v", err)
		}
		if err := ShortcutEdgeRoutes(context.Background(), g); err != nil {
			t.Fatal(err)
		}
		if shortcutBends(e.Points) != 2 || shortcutLength(e.Points) != shortcutLength(before) {
			t.Fatalf("want four to two bends at unchanged length: %v", e.Points)
		}
		if e.Points[0] != before[0] || e.Points[len(e.Points)-1] != before[len(before)-1] ||
			!sameRouteDirection(before[0], before[1], e.Points[0], e.Points[1]) ||
			!sameRouteDirection(before[4], before[5], e.Points[2], e.Points[3]) {
			t.Fatal("changed endpoint identity or signed port approach")
		}
		after := captureExactRouteTest(e)
		if err := ShortcutEdgeRoutes(context.Background(), g); err != nil {
			t.Fatal(err)
		}
		after.assertRestored(t)
	}
}

func TestShortcutRejectsObstaclesLabelsAndBadPorts(t *testing.T) {
	for _, kind := range []string{"node", "label", "node wall", "reversed port", "short port"} {
		t.Run(kind, func(t *testing.T) {
			g, e := shortcutTestGraph()
			candidate := shortcutTestPoints(100, 250, 400, 250, 400, 50, 500, 50)
			var boxes []geo.Box
			switch kind {
			case "node":
				n := g.AddNode(layoutgraph.NewNode(3, 40, 40))
				n.TopLeft = geo.NewPoint(280, 230)
			case "label":
				boxes = append(boxes, geo.Box{TopLeft: geo.NewPoint(280, 230), Width: 40, Height: 40})
			case "node wall":
				n := g.AddNode(layoutgraph.NewNode(3, 40, 40))
				n.TopLeft = geo.NewPoint(280, 280) // new30 gap versus old130
			case "reversed port":
				candidate = shortcutTestPoints(100, 250, 60, 250, 60, 50, 500, 50)
			case "short port":
				candidate = shortcutTestPoints(100, 250, 120, 250, 120, 50, 500, 50)
			}
			guard, _ := newRouteWorkGuard(context.Background(), kind, maxRouteStageWorkUnits)
			safe, err := shortcutCandidateSafe(g, e, candidate, boxes, guard)
			if err != nil || safe {
				t.Fatalf("unsafe candidate accepted: safe=%v err=%v", safe, err)
			}
		})
	}
}

func TestShortcutKeepsShortUnchangedLegsButRejectsNewTinyJogs(t *testing.T) {
	g, e := shortcutTestGraph()
	e.Points = shortcutTestPoints(100, 250, 200, 250, 200, 150, 300, 150, 300, 60, 400, 60, 400, 50, 500, 50)
	for _, x := range []float64{400, 350} {
		candidate := shortcutTestPoints(100, 250, 300, 250, 300, 60, x, 60, x, 50, 500, 50)
		guard, _ := newRouteWorkGuard(context.Background(), "short leg", maxRouteStageWorkUnits)
		safe, err := shortcutCandidateSafe(g, e, candidate, nil, guard)
		if err != nil || safe != (x == 400) {
			t.Fatalf("x=%v safe=%v err=%v", x, safe, err)
		}
	}
}

func TestShortcutComparesContactsGeometrically(t *testing.T) {
	before := shortcutTestPoints(100, 250, 200, 250, 200, 150, 400, 150, 400, 50, 500, 50)
	after := shortcutTestPoints(100, 250, 400, 250, 400, 50, 500, 50)
	for _, tc := range []struct {
		name   string
		points []*geo.Point
		want   bool
	}{
		{"new crossing", shortcutTestPoints(300, 220, 300, 280), false},
		{"new point contact", shortcutTestPoints(300, 250, 300, 300), false},
		{"new shared run", shortcutTestPoints(300, 250, 350, 250), false},
		{"extended shared run", shortcutTestPoints(150, 250, 350, 250), false},
		{"existing shared run", shortcutTestPoints(150, 250, 180, 250), true},
		{"existing crossing in longer segment", shortcutTestPoints(150, 220, 150, 280), true},
		{"disjoint", shortcutTestPoints(250, 300, 350, 300), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			guard, _ := newRouteWorkGuard(context.Background(), tc.name, maxRouteStageWorkUnits)
			safe, err := shortcutContactsPreserved(before, after, tc.points, guard)
			if err != nil || safe != tc.want {
				t.Fatalf("safe=%v want=%v err=%v", safe, tc.want, err)
			}
		})
	}
}

func TestShortcutDoesNotTurnSharedCornerIntoThroughCrossing(t *testing.T) {
	before := shortcutTestPoints(0, 0, 100, 0, 100, 100, 200, 100, 200, 200, 300, 200, 300, 300)
	after := shortcutTestPoints(0, 0, 300, 0, 300, 300)
	for _, other := range [][]*geo.Point{
		shortcutTestPoints(100, -100, 100, 300),
		shortcutTestPoints(100, -100, 100, 0, 100, 300),
	} {
		guard, _ := newRouteWorkGuard(context.Background(), "crossing kind", maxRouteStageWorkUnits)
		safe, err := shortcutContactsPreserved(before, after, other, guard)
		if err != nil || safe {
			t.Fatalf("old shared corner became a new crossing: safe=%v err=%v", safe, err)
		}
	}
}

func TestShortcutPreservesParallelRouteClearance(t *testing.T) {
	for _, x := range []float64{401, 440} {
		g, e := shortcutTestGraph()
		from := g.AddNode(layoutgraph.NewNode(3, 20, 20))
		from.TopLeft = geo.NewPoint(600, 170)
		to := g.AddNode(layoutgraph.NewNode(4, 20, 20))
		to.TopLeft = geo.NewPoint(600, 220)
		other := g.Connect(from, to)
		other.Points = shortcutTestPoints(600, 180, x, 180, x, 230, 600, 230)
		candidate := shortcutTestPoints(100, 250, 400, 250, 400, 50, 500, 50)
		guard, _ := newRouteWorkGuard(context.Background(), "parallel clearance", maxRouteStageWorkUnits)
		safe, err := shortcutCandidateSafe(g, e, candidate, nil, guard)
		if err != nil || safe != (x == 440) {
			t.Fatalf("parallel gap%v safe=%v err=%v", x-400, safe, err)
		}
	}
}

func TestShortcutParallelClearanceKeepsExistingSharedPortGeometry(t *testing.T) {
	g, e := shortcutTestGraph()
	other := g.Connect(e.From, e.To)
	other.Points = shortcutTestPoints(100, 250, 200, 250, 200, 350, 500, 350, 500, 50)
	// The retained shared source leg may become part of a longer segment;
	// its actual shared interval remains exactly the original100 units.
	guard, _ := newRouteWorkGuard(context.Background(), "shared port", maxRouteStageWorkUnits)
	safe, err := shortcutParallelClearance(g, e, geo.NewPoint(100, 250), geo.NewPoint(400, 250), guard)
	if err != nil || !safe {
		t.Fatalf("existing source trunk rejected: %v %v", safe, err)
	}
	guard, _ = newRouteWorkGuard(context.Background(), "shared trunk peeling", maxRouteStageWorkUnits)
	safe, err = shortcutParallelClearance(g, e, geo.NewPoint(100, 251), geo.NewPoint(400, 251), guard)
	if err != nil || safe {
		t.Fatalf("shared trunk justified a new1px corridor: %v %v", safe, err)
	}
}

func TestShortcutSkipsLabeledTargetsAndOpenInventories(t *testing.T) {
	for _, kind := range []string{"main label", "source label", "target label", "external edge"} {
		t.Run(kind, func(t *testing.T) {
			g, e := shortcutTestGraph()
			l := &layoutgraph.Label{Text: "keep position", Position: label.InsideMiddleCenter, Width: 40, Height: 20}
			switch kind {
			case "main label":
				e.Label = l
			case "source label":
				e.SourceArrowheadLabel = l
			case "target label":
				e.TargetArrowheadLabel = l
			case "external edge":
				other := layoutgraph.NewEdge(e.From, e.To)
				other.Points = shortcutTestPoints(100, 250, 100, 400, 500, 400, 500, 50)
				e.From.Edges = append(e.From.Edges, other)
			}
			before := captureExactRouteTest(e)
			if err := ShortcutEdgeRoutes(context.Background(), g); err != nil {
				t.Fatal(err)
			}
			before.assertRestored(t)
		})
	}
}

func TestShortcutAtomicRollbackAfterAUsefulMutation(t *testing.T) {
	for _, kind := range []string{"cancel", "budget"} {
		t.Run(kind, func(t *testing.T) {
			g, e := shortcutTestGraph()
			before := captureExactRouteTest(e)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := runAtomicRouteStage(ctx, "shortcut injected failure", g, nil, maxRouteStageWorkUnits, func(guard *routeWorkGuard) error {
				if err := shortcutRoutesGuarded(g, guard); err != nil {
					return err
				}
				if shortcutBends(e.Points) != 2 {
					t.Fatal("did not reach useful mutation")
				}
				if kind == "cancel" {
					cancel()
					return nil
				}
				return guard.add(guard.limit)
			})
			want := error(errRouteStageWorkLimit)
			if kind == "cancel" {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("got%v want%v", err, want)
			}
			before.assertRestored(t)
		})
	}
}

func TestShortcutAdmissionAndCancellation(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.Nodes = make(layoutgraph.Nodes, maxShortcutInput+1)
	if allocations := testing.AllocsPerRun(20, func() {
		if err := ShortcutEdgeRoutes(context.Background(), g); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("oversized no-op allocated%v times", allocations)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ShortcutEdgeRoutes(ctx, g); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := ShortcutEdgeRoutes(nil, g); err == nil {
		t.Fatal("nil context accepted")
	}
	if err := ShortcutEdgeRoutes(context.Background(), nil); err == nil {
		t.Fatal("nil graph accepted")
	}
	g, e := shortcutTestGraph()
	before := captureExactRouteTest(e)
	if err := shortcutRoutesWithLimit(context.Background(), g, 1); !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatal(err)
	}
	before.assertRestored(t)
}

// Eight31-leg staircases reach the admitted256-point envelope. Their spaces
// are disjoint, so obstacle discovery cannot immediately reject every trial.
func shortcutEnvelopeGraph() *layoutgraph.Graph {
	g := layoutgraph.NewGraph()
	for i := 0; i < 8; i++ {
		y := float64(i * 2000)
		from := g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(2*i+1), 100, 100))
		from.TopLeft = geo.NewPoint(0, y)
		to := g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(2*i+2), 100, 100))
		to.TopLeft = geo.NewPoint(1700, y+1500)
		e := g.Connect(from, to)
		x := 100.0
		y += 50
		e.Points = append(e.Points, geo.NewPoint(x, y))
		for j := 0; j < 31; j++ {
			if j%2 == 0 {
				x += 100
			} else {
				y += 100
			}
			e.Points = append(e.Points, geo.NewPoint(x, y))
		}
	}
	return g
}

func TestShortcutAdmittedEnvelopeHonorsBudget(t *testing.T) {
	g := shortcutEnvelopeGraph()
	if shortcutInputTooLarge(g) {
		t.Fatal("envelope unexpectedly skipped")
	}
	before := make([]exactRouteTestSnapshot, len(g.Edges))
	for i, e := range g.Edges {
		before[i] = captureExactRouteTest(e)
	}
	if err := shortcutRoutesWithLimit(context.Background(), g, 10_000); !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatal(err)
	}
	for _, snapshot := range before {
		snapshot.assertRestored(t)
	}
}

func BenchmarkShortcutAdmission(b *testing.B) {
	for _, admitted := range []bool{false, true} {
		name := "oversized"
		if admitted {
			name = "admitted256points"
		}
		b.Run(name, func(b *testing.B) {
			g := shortcutEnvelopeGraph()
			if !admitted {
				g.Nodes = make(layoutgraph.Nodes, maxShortcutInput+1)
			}
			original := make([][]*geo.Point, len(g.Edges))
			for i, e := range g.Edges {
				original[i] = e.Points
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j, e := range g.Edges {
					e.Points = original[j]
				}
				if err := ShortcutEdgeRoutes(context.Background(), g); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
