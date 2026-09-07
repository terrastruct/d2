package placementcost

import (
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func flowFixture(points []geo.Point, incoming []bool) (*layoutgraph.Node, *edgeScratch) {
	g := layoutgraph.NewGraph()
	g.CellSize = 40
	g.ResetTurnCost()
	center := g.AddNode(layoutgraph.NewNode(1, 40, 40))
	center.TopLeft = geo.NewPoint(-20, -20)
	s := &edgeScratch{}
	for i, p := range points {
		adj := g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i+2), 40, 40))
		adj.TopLeft = geo.NewPoint(p.X-20, p.Y-20)
		e := layoutgraph.NewEdge(center, adj)
		if incoming[i] {
			e.From, e.To = adj, center
		}
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		center.Edges = append(center.Edges, e)
		adj.Edges = append(adj.Edges, e)
		g.AddEdge(e)
		s.nRepl = append(s.nRepl, center)
		s.aRepl = append(s.aRepl, adj)
	}
	return center, s
}

func TestFlowContinuityRecognizesDirectedPathUnderRotation(t *testing.T) {
	if flowSpineWeight == 0 {
		t.Skip("branch-only experiment")
	}
	for rotation := range 4 {
		rotate := func(p geo.Point) geo.Point {
			for range rotation {
				p.X, p.Y = -p.Y, p.X
			}
			return p
		}
		var costs []float64
		for _, outgoing := range []geo.Point{{X: 0, Y: 160}, {X: 160, Y: 0}, {X: 40, Y: -160}} {
			n, s := flowFixture([]geo.Point{rotate(geo.Point{X: 0, Y: -160}), rotate(outgoing)}, []bool{true, false})
			cost := flowContinuityCost(n, s)
			costs = append(costs, cost)
			for _, e := range n.Edges {
				e.From, e.To = e.To, e.From
				e.SourceArrowhead, e.TargetArrowhead = e.TargetArrowhead, e.SourceArrowhead
			}
			if got := flowContinuityCost(n, s); math.Abs(got-cost) > 1e-8 {
				t.Fatalf("source-arrow declaration changed semantic path cost: %v vs %v", got, cost)
			}
		}
		if costs[0] != 0 || !(costs[0] < costs[1] && costs[1] < costs[2]) {
			t.Fatalf("straight, elbow, hairpin costs %v", costs)
		}
	}
}

func TestFlowBranchesRewardAngularRoomForFanInAndFanOut(t *testing.T) {
	if flowBranchWeight == 0 {
		t.Skip("spine-only experiment")
	}
	var crowded, separated float64
	for _, in := range []bool{false, true} {
		n, s := flowFixture([]geo.Point{{X: 0, Y: 160}, {X: 40, Y: 160}}, []bool{in, in})
		cost := flowContinuityCost(n, s)
		if crowded != 0 && crowded != cost {
			t.Fatal("fan-in and fan-out have different angular preferences")
		}
		crowded = cost
		s.aRepl[1].TopLeft = geo.NewPoint(140, -20)
		separated = flowContinuityCost(n, s)
	}
	if crowded <= 0 || separated != 0 {
		t.Fatalf("crowded=%v separated=%v", crowded, separated)
	}
}

func TestFlowContinuityLeavesSpecialEndpointsToExistingPlacement(t *testing.T) {
	for _, name := range []string{"undirected", "bidirectional", "invisible", "table-port", "sequence", "cluster", "abducted", "different-container"} {
		t.Run(name, func(t *testing.T) {
			n, s := flowFixture([]geo.Point{{X: 0, Y: -160}, {X: 160, Y: 0}}, []bool{true, false})
			switch name {
			case "undirected":
				for _, e := range n.Edges {
					e.TargetArrowhead = layoutgraph.NoArrowhead
				}
			case "bidirectional":
				for _, e := range n.Edges {
					e.SourceArrowhead = layoutgraph.TriangleArrowhead
				}
			case "invisible":
				n.Edges[0].IsInvisible = true
			case "table-port":
				column := 1
				n.Edges[0].FromTableColumnIndex = &column
			case "sequence":
				n.Sequence = &layoutgraph.Sequence{}
			case "cluster":
				n.Cluster = &layoutgraph.Cluster{}
			case "abducted":
				s.nRepl[0] = s.aRepl[0]
			case "different-container":
				s.aRepl[0].Container = layoutgraph.NewNode(99, 40, 40)
			}
			if got := flowContinuityCost(n, s); got != 0 {
				t.Fatalf("special topology has extra cost %v", got)
			}
		})
	}
}

func appendFlowFixtureEdge(node *layoutgraph.Node, s *edgeScratch, adjacent *layoutgraph.Node, incoming bool) {
	e := layoutgraph.NewEdge(node, adjacent)
	if incoming {
		e.From, e.To = adjacent, node
	}
	e.TargetArrowhead = layoutgraph.TriangleArrowhead
	node.Edges = append(node.Edges, e)
	adjacent.Edges = append(adjacent.Edges, e)
	node.Graph.AddEdge(e)
	s.nRepl = append(s.nRepl, node)
	s.aRepl = append(s.aRepl, adjacent)
}

func TestFlowContinuityReciprocalDirectionsIgnoreDeclarationOrder(t *testing.T) {
	orders := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, order := range orders {
		// Every edge can independently be written using a source arrow while
		// retaining its semantic direction. Check all eight representations.
		for sourceArrows := range 8 {
			n, s := flowFixture([]geo.Point{{X: 0, Y: -160}, {X: 160, Y: 0}}, []bool{true, false})
			appendFlowFixtureEdge(n, s, s.aRepl[0], false)
			edges := append([]*layoutgraph.Edge(nil), n.Edges...)
			adjacent := append([]*layoutgraph.Node(nil), s.aRepl...)
			for i, index := range order {
				e := edges[index]
				if sourceArrows&(1<<index) != 0 {
					e.From, e.To = e.To, e.From
					e.SourceArrowhead, e.TargetArrowhead = e.TargetArrowhead, e.SourceArrowhead
				}
				n.Edges[i], s.aRepl[i] = e, adjacent[index]
			}
			// The reciprocal neighbor supplies an incoming continuation into
			// the right-angle outgoing edge. Their shared outgoing role incurs
			// no branch penalty because the rays are 90 degrees apart.
			want := flowSpineWeight * n.Graph.TurnCost()
			if got := flowContinuityCost(n, s); math.Abs(got-want) > 1e-8 {
				t.Fatalf("order %v source arrows %03b: got %v want %v", order, sourceArrows, got, want)
			}
			options := EdgeLengthOptions{IncludeNodeSizes: true, PenalizeDirection: true}
			direct, err := NodeEdgeLength(t.Context(), n, options)
			if err != nil {
				t.Fatal(err)
			}
			scorer := NewNodeEdgeLengthScorer(n, options)
			prepared, err := scorer.Score(t.Context())
			scorer.Close()
			if err != nil || direct != prepared {
				t.Fatalf("direct=%v prepared=%v error=%v", direct, prepared, err)
			}
		}
	}
}

func TestFlowContinuityReciprocalPairHasBothRoles(t *testing.T) {
	n, s := flowFixture([]geo.Point{{X: 0, Y: 160}, {X: 96, Y: 128}}, []bool{true, false})
	appendFlowFixtureEdge(n, s, s.aRepl[0], false)
	appendFlowFixtureEdge(n, s, s.aRepl[1], true)
	// The rays have dot product 0.8. Both neighbors are reciprocal, so this
	// one geometric pair has a 1.8 continuation cost and a 0.6 branch cost.
	want := n.Graph.TurnCost() * (1.8*flowSpineWeight + 0.6*flowBranchWeight)
	if got := flowContinuityCost(n, s); math.Abs(got-want) > 1e-8 {
		t.Fatalf("reciprocal pair got %v want %v", got, want)
	}
	for range 2 {
		appendFlowFixtureEdge(n, s, s.aRepl[0], true)
		appendFlowFixtureEdge(n, s, s.aRepl[1], false)
	}
	if got := flowContinuityCost(n, s); math.Abs(got-want) > 1e-8 {
		t.Fatalf("duplicate reciprocal edges changed pair weight: got %v want %v", got, want)
	}
}

func TestFlowContinuityNeverPairsANeighborWithItself(t *testing.T) {
	n, s := flowFixture([]geo.Point{{X: 0, Y: 160}}, []bool{true})
	for i := 1; i < 8; i++ {
		appendFlowFixtureEdge(n, s, s.aRepl[0], i%2 == 0)
		if got := flowContinuityCost(n, s); got != 0 {
			t.Fatalf("%d parallel/reciprocal edges to one neighbor have cost %v", i+1, got)
		}
	}
}

func TestFlowContinuityParallelEdgesDoNotReweightBranches(t *testing.T) {
	n, s := flowFixture([]geo.Point{{X: 0, Y: 160}, {X: 96, Y: 128}, {X: -160, Y: 0}}, []bool{false, false, false})
	// Only the first two of the three neighbor pairs crowd each other.
	// Repeating that neighbor must not increase its share of the average.
	want := n.Graph.TurnCost() * flowBranchWeight * 0.2
	for i := 0; i < 6; i++ {
		if got := flowContinuityCost(n, s); math.Abs(got-want) > 1e-8 {
			t.Fatalf("%d duplicates changed branch weight: got %v want %v", i, got, want)
		}
		if i < 5 {
			appendFlowFixtureEdge(n, s, s.aRepl[0], false)
		}
	}
}

func TestFlowContinuityPreservesBoundedAllocationFreeEvaluation(t *testing.T) {
	n, s := flowFixture([]geo.Point{{X: 0, Y: 160}, {X: 96, Y: 128}}, []bool{true, false})
	for range 3 {
		appendFlowFixtureEdge(n, s, s.aRepl[0], false)
		appendFlowFixtureEdge(n, s, s.aRepl[1], true)
	}
	if got := flowContinuityCost(n, s); got <= 0 || got > 3*n.Graph.TurnCost() {
		t.Fatalf("eight-edge fixture has invalid cost %v", got)
	}
	if allocations := testing.AllocsPerRun(100, func() { flowContinuityCost(n, s) }); allocations != 0 {
		t.Fatalf("flow evaluation allocated %v times", allocations)
	}
	appendFlowFixtureEdge(n, s, s.aRepl[0], false)
	// More than eight incident edges must return before consulting scratch.
	if got := flowContinuityCost(n, nil); got != 0 {
		t.Fatalf("degree-nine node has cost %v", got)
	}
}
