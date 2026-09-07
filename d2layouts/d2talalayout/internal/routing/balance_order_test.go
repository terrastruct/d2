package routing

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// This is the geometry entering balancing in the public one_container_loop
// example. The router puts a's stem just left of h's; centering h within its
// narrower range used to reverse that order before a's batch was considered.
func containerBalanceOrderGraph() (*layoutgraph.Graph, *layoutgraph.Edge, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	nodes := make(map[string]*layoutgraph.Node)
	for i, spec := range []struct {
		name       string
		x, y, w, h float64
	}{
		{"a", 47, 0, 393, 186}, {"b", 327, 60, 53, 66},
		{"c", 194, 744, 53, 66}, {"d", 193, 588, 54, 66},
		{"e", 0, 276, 53, 66}, {"f", 195, 432, 51, 66},
		{"g", 193, 276, 54, 66}, {"h", 107, 60, 53, 66},
	} {
		n := layoutgraph.NewNode(layoutgraph.EntityID(i+1), spec.w, spec.h)
		n.TopLeft = geo.NewPoint(spec.x, spec.y)
		nodes[spec.name] = n
		var container *layoutgraph.Node
		if spec.name == "b" || spec.name == "h" {
			container = nodes["a"]
		}
		g.AddNewNodeToContainer(container, n)
	}
	connect := func(from, to string, coordinates ...float64) *layoutgraph.Edge {
		e := g.Connect(nodes[from], nodes[to])
		for i := 0; i < len(coordinates); i += 2 {
			e.Points = append(e.Points, geo.NewPoint(coordinates[i], coordinates[i+1]))
		}
		return e
	}
	connect("b", "c", 340, 126, 340, 761, 247, 761)
	connect("d", "c", 221, 654, 221, 744)
	connect("e", "c", 40, 342, 40, 761, 194, 761)
	connect("f", "d", 221, 498, 221, 588)
	ae := connect("a", "e", 145, 186, 145, 309, 53, 309)
	connect("g", "f", 221, 342, 221, 432)
	hg := connect("h", "g", 147, 126, 147, 293, 193, 293)
	return g, ae, hg
}

func balanceOrderCrossings(g *layoutgraph.Graph) int {
	count := 0
	for i, edge := range g.Edges {
		for _, other := range g.Edges[i+1:] {
			for a := 0; a+1 < len(edge.Points); a++ {
				for b := 0; b+1 < len(other.Points); b++ {
					p, q := edge.Points[a], edge.Points[a+1]
					r, s := other.Points[b], other.Points[b+1]
					if p.Y == q.Y && r.X == s.X {
						p, q, r, s = r, s, p, q
					}
					// These fixtures use orthogonal segments. Count strict
					// through-crossings independently of shared-trunk joins.
					if p.X == q.X && r.Y == s.Y && p.X > math.Min(r.X, s.X) && p.X < math.Max(r.X, s.X) && r.Y > math.Min(p.Y, q.Y) && r.Y < math.Max(p.Y, q.Y) {
						count++
					}
				}
			}
		}
	}
	return count
}

func TestBalanceEdgeSegmentsPreservesContainerRouteOrder(t *testing.T) {
	for _, orientation := range []string{"down", "up", "right", "left"} {
		for _, reverse := range []bool{false, true} {
			t.Run(orientation+map[bool]string{false: "", true: "/reversed_routes"}[reverse], func(t *testing.T) {
				g, ae, _ := containerBalanceOrderGraph()
				transform := func(p *geo.Point) {
					switch orientation {
					case "up":
						p.Y = 810 - p.Y
					case "right":
						p.X, p.Y = p.Y, p.X
					case "left":
						p.X, p.Y = 810-p.Y, p.X
					}
				}
				boxes := make(map[*layoutgraph.Node]geo.Box)
				for _, n := range g.Nodes {
					br := geo.NewPoint(n.TopLeft.X+n.Width, n.TopLeft.Y+n.Height)
					transform(n.TopLeft)
					transform(br)
					n.Width, n.Height = math.Abs(br.X-n.TopLeft.X), math.Abs(br.Y-n.TopLeft.Y)
					n.TopLeft.X, n.TopLeft.Y = math.Min(n.TopLeft.X, br.X), math.Min(n.TopLeft.Y, br.Y)
					boxes[n] = geo.Box{TopLeft: n.TopLeft.Copy(), Width: n.Width, Height: n.Height}
				}
				for _, e := range g.Edges {
					for _, p := range e.Points {
						transform(p)
					}
					if reverse {
						e.From, e.To = e.To, e.From
						for i, j := 0, len(e.Points)-1; i < j; i, j = i+1, j-1 {
							e.Points[i], e.Points[j] = e.Points[j], e.Points[i]
						}
					}
				}
				if got := balanceOrderCrossings(g); got != 0 {
					t.Fatalf("fixture has %d crossings before balancing", got)
				}
				if err := BalanceEdgeSegments(context.Background(), g); err != nil {
					t.Fatal(err)
				}
				if got := balanceOrderCrossings(g); got != 0 {
					t.Fatalf("balancing introduced %d crossings", got)
				}
				for _, e := range g.Edges {
					if !e.From.ContainsPointOnBox(e.Points[0]) || !e.To.ContainsPointOnBox(e.Points[len(e.Points)-1]) {
						t.Fatal("balancing detached an endpoint from its node")
					}
				}
				for n, before := range boxes {
					if *n.TopLeft != *before.TopLeft || n.Width != before.Width || n.Height != before.Height {
						t.Fatal("balancing moved a node")
					}
				}
				length := 0.0
				for i := 1; i < len(ae.Points); i++ {
					length += math.Abs(ae.Points[i].X-ae.Points[i-1].X) + math.Abs(ae.Points[i].Y-ae.Points[i-1].Y)
				}
				if length >= 215 {
					t.Fatalf("useful balancing was lost: a→e length %v, originally 215", length)
				}
			})
		}
	}
}

func TestBalanceOrderChecksClosedIntervalsAndEverySharedMember(t *testing.T) {
	makeSegment := func(x, start, end float64) *layoutgraph.EdgeSegment {
		e := layoutgraph.NewEdge(layoutgraph.NewNode(1, 10, 10), layoutgraph.NewNode(2, 10, 10))
		return layoutgraph.NewEdgeSegment(geo.NewPoint(x, start), geo.NewPoint(x, end), e)
	}
	for _, tc := range []struct {
		name               string
		start, end, target float64
		want               bool
	}{
		{"overtakes overlapping route", 5, 15, 30, false},
		{"touches at dragged elbow", 10, 20, 30, false},
		{"disjoint routes may reorder", 11, 20, 30, true},
		{"safe movement remains available", 5, 15, 19, true},
		{"new route contact is rejected", 5, 15, 20, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			moving, outside := makeSegment(10, 0, 10), makeSegment(20, tc.start, tc.end)
			guard, err := newRouteWorkGuard(context.Background(), "order test", 100)
			if err != nil {
				t.Fatal(err)
			}
			got, err := checkBalanceOrder([]*layoutgraph.EdgeSegment{moving}, map[*layoutgraph.EdgeSegment]bool{moving: true}, []*layoutgraph.EdgeSegment{moving, outside}, []float64{tc.target}, true, guard)
			if err != nil || (got == balanceOrderPreserved) != tc.want {
				t.Fatalf("accepted=%v, err=%v; want %v", got, err, tc.want)
			}
		})
	}
	// Only the second member of a shared trunk reaches the foreign route.
	first, second, outside := makeSegment(10, 0, 10), makeSegment(10, 5, 30), makeSegment(20, 20, 40)
	batch := []*layoutgraph.EdgeSegment{first, second}
	set := map[*layoutgraph.EdgeSegment]bool{first: true, second: true}
	guard, err := newRouteWorkGuard(context.Background(), "shared order test", 100)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := checkBalanceOrder(batch, set, []*layoutgraph.EdgeSegment{first, second, outside}, []float64{30, 30}, true, guard)
	if err != nil || ok == balanceOrderPreserved {
		t.Fatalf("partial shared trunk ignored its second member's constraint: accepted=%v, err=%v", ok, err)
	}
	if first.Start.X != 10 || second.Start.X != 10 {
		t.Fatal("a rejected proposal mutated the shared trunk")
	}

	guard, err = newRouteWorkGuard(context.Background(), "limited order test", 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = checkBalanceOrder(batch, set, []*layoutgraph.EdgeSegment{outside}, []float64{30, 30}, true, guard)
	if !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("work limit error = %v", err)
	}
	if first.Start.X != 10 || second.Start.X != 10 {
		t.Fatal("exhausted order check mutated routes")
	}
}

// Two short source stubs are crossed by a longer return route. Their shared
// destination does not make those crossings intentional. Reversing the two
// horizontal lanes removes both crossings and must remain available.
func TestBalanceEdgeSegmentsStillUncrossesDifferentRanges(t *testing.T) {
	g := layoutgraph.NewGraph()
	source := layoutgraph.NewNode(1, 80, 40)
	source.TopLeft = geo.NewPoint(80, 80)
	sink := layoutgraph.NewNode(2, 60, 40)
	sink.TopLeft = geo.NewPoint(0, 200)
	remote := layoutgraph.NewNode(3, 80, 40)
	remote.TopLeft = geo.NewPoint(180, 300)
	for _, n := range []*layoutgraph.Node{source, sink, remote} {
		g.AddNewNodeToContainer(nil, n)
	}
	for _, x := range []float64{100, 140} {
		e := g.Connect(source, sink)
		e.Points = []*geo.Point{geo.NewPoint(x, 80), geo.NewPoint(x, 40), geo.NewPoint(30, 40), geo.NewPoint(30, 200)}
	}
	returnEdge := g.Connect(remote, sink)
	returnEdge.Points = []*geo.Point{geo.NewPoint(260, 320), geo.NewPoint(300, 320), geo.NewPoint(300, 60), geo.NewPoint(30, 60), geo.NewPoint(30, 200)}
	if got := balanceOrderCrossings(g); got != 2 {
		t.Fatalf("fixture crossings = %d; want 2", got)
	}
	if err := BalanceEdgeSegments(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if got := balanceOrderCrossings(g); got != 0 {
		t.Fatalf("balancing left %d avoidable crossings", got)
	}
}

func TestBalanceOrderReversalDoesNotHideLaterContact(t *testing.T) {
	makeSegment := func(x float64) *layoutgraph.EdgeSegment {
		e := layoutgraph.NewEdge(layoutgraph.NewNode(1, 10, 10), layoutgraph.NewNode(2, 10, 10))
		return layoutgraph.NewEdgeSegment(geo.NewPoint(x, 0), geo.NewPoint(x, 10), e)
	}
	moving, reversed, contact := makeSegment(10), makeSegment(20), makeSegment(30)
	guard, err := newRouteWorkGuard(context.Background(), "mixed order test", 100)
	if err != nil {
		t.Fatal(err)
	}
	status, err := checkBalanceOrder([]*layoutgraph.EdgeSegment{moving}, map[*layoutgraph.EdgeSegment]bool{moving: true}, []*layoutgraph.EdgeSegment{reversed, contact}, []float64{30}, true, guard)
	if err != nil || status != balanceOrderContactChanged {
		t.Fatalf("status=%v, err=%v; a later contact must prohibit fallback", status, err)
	}
}

func TestBalanceReversalDoesNotTradeCrossingsOrAddSharedRuns(t *testing.T) {
	for _, sharedRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "crossing trade", true: "new shared run"}[sharedRun], func(t *testing.T) {
			g := layoutgraph.NewGraph()
			addRoute := func(coordinates ...float64) *layoutgraph.Edge {
				n := layoutgraph.NewNode(layoutgraph.EntityID(len(g.Nodes)+1), 10, 10)
				n.TopLeft = geo.NewPoint(1000, float64(len(g.Nodes)*20))
				g.AddNewNodeToContainer(nil, n)
				m := layoutgraph.NewNode(layoutgraph.EntityID(len(g.Nodes)+1), 10, 10)
				m.TopLeft = geo.NewPoint(1100, n.TopLeft.Y)
				g.AddNewNodeToContainer(nil, m)
				e := g.Connect(n, m)
				for i := 0; i < len(coordinates); i += 2 {
					e.Points = append(e.Points, geo.NewPoint(coordinates[i], coordinates[i+1]))
				}
				return e
			}
			var moving *layoutgraph.Edge
			var segment *layoutgraph.EdgeSegment
			var target float64
			if sharedRun {
				moving = addRoute(-20, 0, 10, 0, 10, -10)
				segment = layoutgraph.NewEdgeSegment(moving.Points[1], moving.Points[2], moving)
				target = 50
				addRoute(-30, -5, 30, -5)               // This pair loses one crossing.
				addRoute(20, 30, 20, 0, 40, 0, 40, -30) // This pair would gain a shared run.
			} else {
				moving = addRoute(10, 0, 10, 20)
				segment = layoutgraph.NewEdgeSegment(moving.Points[0], moving.Points[1], moving)
				target = 30
				addRoute(0, 5, 20, 5)    // This pair loses one crossing.
				addRoute(20, 15, 40, 15) // This pair would gain one crossing.
			}
			snapshot := captureExactRouteTest(moving)
			guard, err := newRouteWorkGuard(context.Background(), "fallback test", 10000)
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := balanceReversalRemovesCrossings(g, []*layoutgraph.EdgeSegment{segment}, []float64{target}, true, guard)
			if err != nil || accepted {
				t.Fatalf("unsafe reversal accepted=%v err=%v", accepted, err)
			}
			snapshot.assertRestored(t)
		})
	}
}
