package routing

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func TestChannelGapSolvesCoupledDifferentIntervals(t *testing.T) {
	p := channelProblem{
		groups: []*channelGroup{{lower: 0, upper: 120}, {lower: 20, upper: 180}},
		arcs:   []channelArc{{from: 0, to: 1, separate: true}},
	}
	guard, _ := newRouteWorkGuard(context.Background(), "channel test", 10000)
	lo, hi, feasible, err := solveChannelGap(p, 60, guard)
	if err != nil || !feasible {
		t.Fatalf("gap60 feasible=%v err=%v", feasible, err)
	}
	for i, want := range []float64{60, 120} {
		if lo[i] != want || hi[i] != want {
			t.Fatalf("variable%d interval=[%v,%v] want%v", i, lo[i], hi[i], want)
		}
	}
	_, _, feasible, err = solveChannelGap(p, 61, guard)
	if err != nil || feasible {
		t.Fatalf("gap61 feasible=%v err=%v", feasible, err)
	}
	// Reusing an already feasible gap must produce feasible midpoint positions.
	lo, hi, feasible, err = solveChannelGap(p, 20, guard)
	if err != nil || !feasible {
		t.Fatal(err)
	}
	mid := []float64{(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2}
	if channelGap(p, mid) < 20 {
		t.Fatalf("midpoint violates separation: %v", mid)
	}
}

func TestChannelGapAllowsSamePathLegsToMeet(t *testing.T) {
	p := channelProblem{groups: []*channelGroup{{lower: 0, upper: 100}, {lower: 0, upper: 100}}, arcs: []channelArc{{from: 0, to: 1}}}
	guard, _ := newRouteWorkGuard(context.Background(), "channel test", 10000)
	lo, hi, feasible, err := solveChannelGap(p, 50, guard)
	if err != nil || !feasible || lo[0] != 50 || lo[1] != 50 || hi[0] != 50 || hi[1] != 50 {
		t.Fatalf("same-path zero separation: %v %v %v %v", lo, hi, feasible, err)
	}
}

func TestChannelGapAppliesNodeClearanceAsAnAbsoluteBound(t *testing.T) {
	p := channelProblem{groups: []*channelGroup{{lower: 0, upper: 100, nodeClearance: &Range{start: 10, end: 30}}}}
	guard, _ := newRouteWorkGuard(context.Background(), "channel clearance test", 100)
	lo, hi, feasible, err := solveChannelGap(p, 30, guard)
	if err != nil || !feasible || lo[0] != 30 || hi[0] != 30 {
		t.Fatalf("gap and absolute bound were not combined correctly: %v %v %v %v", lo, hi, feasible, err)
	}
	_, _, feasible, err = solveChannelGap(p, 31, guard)
	if err != nil || feasible {
		t.Fatalf("gap escaped its node bound: feasible=%v err=%v", feasible, err)
	}
}

func TestNudgeChannelsDoesNotTradeNodeClearanceForShorterWire(t *testing.T) {
	for _, initialGap := range []float64{21, 60} {
		t.Run(map[float64]string{21: "preserve existing narrow gap", 60: "cap comfortable gap"}[initialGap], func(t *testing.T) {
			g := layoutgraph.NewGraph()
			container := layoutgraph.NewNode(1, 200, 300)
			container.TopLeft = geo.NewPoint(0, 200)
			g.AddNewNodeToContainer(nil, container)
			from := layoutgraph.NewNode(2, 50, 50)
			from.TopLeft = geo.NewPoint(50, 300)
			g.AddNewNodeToContainer(container, from)
			to := layoutgraph.NewNode(3, 50, 50)
			to.TopLeft = geo.NewPoint(400, 300)
			g.AddNewNodeToContainer(nil, to)
			originalY := container.TopLeft.Y - initialGap
			ceiling := layoutgraph.NewNode(4, 500, originalY-10)
			ceiling.TopLeft = geo.NewPoint(0, 0)
			g.AddNewNodeToContainer(nil, ceiling)
			e := g.Connect(from, to)
			e.Points = []*geo.Point{geo.NewPoint(75, 300), geo.NewPoint(75, originalY), geo.NewPoint(425, originalY), geo.NewPoint(425, 300)}
			if err := NudgeEdgeChannels(context.Background(), g); err != nil {
				t.Fatal(err)
			}
			gap := container.TopLeft.Y - e.Points[1].Y
			minimum := math.Min(initialGap, segmentSpacingBuffer)
			if gap+channelEpsilon < minimum {
				t.Fatalf("wire shortening reduced node clearance from%v to%v; minimum%v", initialGap, gap, minimum)
			}
			if initialGap < segmentSpacingBuffer && e.Points[1].Y != originalY {
				t.Fatal("a narrow existing node gap changed")
			}
			if initialGap > segmentSpacingBuffer && e.Points[1].Y <= originalY {
				t.Fatal("comfortable clearance prevented useful wire shortening")
			}
		})
	}
}

func channelZGraph() (*layoutgraph.Graph, *layoutgraph.Edge) {
	g := layoutgraph.NewGraph()
	from := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	from.TopLeft = geo.NewPoint(0, 0)
	to := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	to.TopLeft = geo.NewPoint(300, 200)
	e := g.Connect(from, to)
	e.Points = []*geo.Point{geo.NewPoint(100, 50), geo.NewPoint(120, 50), geo.NewPoint(120, 250), geo.NewPoint(300, 250)}
	return g, e
}

func TestNudgeChannelsBalancesFourPointRoutesAndPreservesPorts(t *testing.T) {
	for _, kind := range []string{shape.SQUARE_TYPE, shape.DIAMOND_TYPE, shape.TABLE_TYPE} {
		t.Run(kind, func(t *testing.T) {
			g, e := channelZGraph()
			e.From.SetShape(kind)
			e.To.SetShape(kind)
			if kind == shape.TABLE_TYPE {
				e.From.SetNumColumns(1)
				e.To.SetNumColumns(1)
				column := 0
				e.FromTableColumnIndex = &column
				e.ToTableColumnIndex = &column
			}
			first, last := *e.Points[0], *e.Points[3]
			from, to := *e.From.TopLeft, *e.To.TopLeft
			if err := NudgeEdgeChannels(context.Background(), g); err != nil {
				t.Fatal(err)
			}
			if *e.Points[0] != first || *e.Points[3] != last || *e.From.TopLeft != from || *e.To.TopLeft != to {
				t.Fatal("moved a fixed port or node")
			}
			if math.Abs(e.Points[1].X-200) > channelEpsilon || math.Abs(e.Points[2].X-200) > channelEpsilon {
				t.Fatalf("interior corridor was not centered: %v", e.Points)
			}
			if e.Points[1].Y != first.Y || e.Points[2].Y != last.Y {
				t.Fatal("changed port approach direction")
			}
			before := captureExactRouteTest(e)
			if err := NudgeEdgeChannels(context.Background(), g); err != nil {
				t.Fatal(err)
			}
			before.assertRestored(t)
		})
	}
}

func TestNudgeChannelsKeepsSharedTrunksAndUnrelatedLockedRoutes(t *testing.T) {
	g, e := channelZGraph()
	duplicate := g.Connect(e.From, e.To)
	for _, point := range e.Points {
		duplicate.Points = append(duplicate.Points, point.Copy())
	}
	loop := g.Connect(e.From, e.From)
	p := geo.NewPoint(50, 50)
	loop.Points = []*geo.Point{p, p}
	locked := captureExactRouteTest(loop)
	if err := NudgeEdgeChannels(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	locked.assertRestored(t)
	for i := range e.Points {
		if *e.Points[i] != *duplicate.Points[i] {
			t.Fatal("split a shared trunk")
		}
	}
	if math.Abs(e.Points[1].X-200) > channelEpsilon {
		t.Fatalf("shared corridor did not move: %v", e.Points[1])
	}
}

func TestNudgeChannelsLeavesUnboundedOuterRoutesAlone(t *testing.T) {
	g, e := channelZGraph()
	e.To.TopLeft.X = 0
	e.Points = []*geo.Point{geo.NewPoint(100, 50), geo.NewPoint(160, 50), geo.NewPoint(160, 250), geo.NewPoint(100, 250)}
	snapshot := captureExactRouteTest(e)
	if err := NudgeEdgeChannels(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	snapshot.assertRestored(t)
}

func TestChannelProposalRejectsShortLegAndNewPointContact(t *testing.T) {
	for _, contact := range []bool{false, true} {
		t.Run(map[bool]string{false: "short leg", true: "new T contact"}[contact], func(t *testing.T) {
			g, e := channelZGraph()
			x := 105.0
			if contact {
				x = 200
				other := g.Connect(e.From, e.To)
				other.Points = []*geo.Point{geo.NewPoint(200, 150), geo.NewPoint(250, 150)}
			}
			proposal := map[*geo.Point]*geo.Point{e.Points[1]: geo.NewPoint(x, 50), e.Points[2]: geo.NewPoint(x, 250)}
			guard, _ := newRouteWorkGuard(context.Background(), "channel test", 10000)
			safe, err := channelProposalSafe(g, proposal, guard)
			if err != nil || safe {
				t.Fatalf("unsafe proposal accepted=%v err=%v", safe, err)
			}
		})
	}
}

func TestNudgeChannelsRestoresOnCancellationAndWorkLimit(t *testing.T) {
	g, e := channelZGraph()
	snapshot := captureExactRouteTest(e)
	ctx := &cancelWhenRouteMutates{Context: context.Background(), snapshot: snapshot}
	err := NudgeEdgeChannels(ctx, g)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
	snapshot.assertRestored(t)
	err = nudgeChannelsWithLimit(context.Background(), g, 1)
	if !errors.Is(err, errRouteStageWorkLimit) {
		t.Fatalf("work limit error=%v", err)
	}
	snapshot.assertRestored(t)
}

func oversizedChannelGraph(kind string) (*layoutgraph.Graph, *layoutgraph.Edge) {
	g, e := channelZGraph()
	switch kind {
	case "nodes":
		for len(g.Nodes) <= maxChannelSegments {
			n := g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(len(g.Nodes)+1), 100, 100))
			n.TopLeft = geo.NewPoint(float64(len(g.Nodes)*200), 500)
		}
	case "edges":
		for len(g.Edges) <= maxChannelSegments {
			other := g.Connect(e.From, e.To)
			for _, point := range e.Points {
				other.Points = append(other.Points, point.Copy())
			}
		}
	case "points":
		for len(e.Points) <= maxChannelSegments {
			e.Points = append(e.Points, e.Points[len(e.Points)-1].Copy())
		}
	default:
		panic("unknown oversized channel graph kind")
	}
	return g, e
}

func TestNudgeChannelsSkipsOversizedInputWithoutSnapshotAllocations(t *testing.T) {
	for _, kind := range []string{"nodes", "edges", "points"} {
		t.Run(kind, func(t *testing.T) {
			g, e := oversizedChannelGraph(kind)
			snapshot := captureExactRouteTest(e)
			// A one-unit budget cannot validate or snapshot this graph. Admission
			// must skip it before either operation starts.
			if err := nudgeChannelsWithLimit(context.Background(), g, 1); err != nil {
				t.Fatal(err)
			}
			allocs := testing.AllocsPerRun(100, func() {
				if err := NudgeEdgeChannels(context.Background(), g); err != nil {
					t.Fatal(err)
				}
			})
			if allocs != 0 {
				t.Fatalf("oversized optional skip allocated %v times", allocs)
			}
			snapshot.assertRestored(t)
		})
	}
}

func TestChannelAdmissionBoundary(t *testing.T) {
	for _, count := range []int{maxChannelSegments, maxChannelSegments + 1} {
		for _, g := range []*layoutgraph.Graph{
			{Nodes: make([]*layoutgraph.Node, count)},
			{Edges: make([]*layoutgraph.Edge, count)},
			{Edges: []*layoutgraph.Edge{nil, {Points: make([]*geo.Point, count)}}},
		} {
			if got := channelInputTooLarge(g); got != (count > maxChannelSegments) {
				t.Fatalf("count %d: oversized=%v", count, got)
			}
		}
	}
}

func TestNudgeChannelsAdmissionPreservesInputErrors(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		g    *layoutgraph.Graph
	}{
		{"nil context", nil, layoutgraph.NewGraph()},
		{"nil graph", context.Background(), nil},
		{"nil graph and canceled context", canceled, nil},
		{"nil node", context.Background(), &layoutgraph.Graph{Nodes: []*layoutgraph.Node{nil}}},
		{"nil edge", context.Background(), &layoutgraph.Graph{Edges: []*layoutgraph.Edge{nil}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Admitted inputs retain the existing validation error, rather than
			// letting the admission check panic or mistake them for no work.
			want := runAtomicRouteStage(tc.ctx, "NudgeChannels", tc.g, nil, maxRouteStageWorkUnits, func(*routeWorkGuard) error { return nil })
			got := NudgeEdgeChannels(tc.ctx, tc.g)
			if want == nil || got == nil || got.Error() != want.Error() {
				t.Fatalf("error=%v; existing validation error=%v", got, want)
			}
		})
	}
	// An oversized graph is deliberately outside this optional stage's
	// validation scope, including when entries are malformed.
	g := &layoutgraph.Graph{Nodes: make([]*layoutgraph.Node, maxChannelSegments+1)}
	if err := NudgeEdgeChannels(context.Background(), g); err != nil {
		t.Fatalf("oversized graph was unnecessarily validated: %v", err)
	}
}

type cancelAfterChannelAdmission struct {
	context.Context
	checks int
}

func (ctx *cancelAfterChannelAdmission) Err() error {
	ctx.checks++
	if ctx.checks > 1 {
		return context.Canceled
	}
	return nil
}

func TestNudgeChannelsOversizedSkipPreservesCancellation(t *testing.T) {
	g, e := oversizedChannelGraph("points")
	snapshot := captureExactRouteTest(e)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{canceled, &cancelAfterChannelAdmission{Context: context.Background()}} {
		if err := NudgeEdgeChannels(ctx, g); !errors.Is(err, context.Canceled) {
			t.Fatalf("oversized skip lost cancellation: %v", err)
		}
	}
	snapshot.assertRestored(t)
}

// This test-only control retains the original admission ordering so the
// benchmark measures the validation/snapshot cost avoided by an early skip.
func nudgeChannelsWithLateAdmission(ctx context.Context, g *layoutgraph.Graph) error {
	return runAtomicRouteStage(ctx, "NudgeChannels", g, nil, maxRouteStageWorkUnits, func(guard *routeWorkGuard) error {
		if len(g.Nodes) > maxChannelSegments || len(g.Edges) > maxChannelSegments {
			return guard.check()
		}
		count := 0
		for _, edge := range g.Edges {
			if err := guard.step(); err != nil {
				return err
			}
			count += len(edge.Points)
			if count > maxChannelSegments {
				return nil
			}
		}
		return nudgeChannelsGuarded(g, guard)
	})
}

func BenchmarkNudgeChannelsAdmission(b *testing.B) {
	for _, kind := range []string{"nodes", "edges", "points", "four_point_channel", "shared_channel"} {
		for _, early := range []bool{false, true} {
			name := map[bool]string{false: "late", true: "early"}[early]
			b.Run(kind+"/"+name, func(b *testing.B) {
				var g *layoutgraph.Graph
				if kind == "four_point_channel" || kind == "shared_channel" {
					var e *layoutgraph.Edge
					g, e = channelZGraph()
					if kind == "shared_channel" {
						for len(g.Edges) < 16 {
							other := g.Connect(e.From, e.To)
							for _, point := range e.Points {
								other.Points = append(other.Points, point.Copy())
							}
						}
					}
				} else {
					g, _ = oversizedChannelGraph(kind)
				}
				run := nudgeChannelsWithLateAdmission
				if early {
					run = NudgeEdgeChannels
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// Restore the admitted channel's original geometry so every
					// iteration includes a useful move, rather than a settled no-op.
					if kind == "four_point_channel" || kind == "shared_channel" {
						for _, edge := range g.Edges {
							edge.Points[1].X, edge.Points[2].X = 120, 120
						}
					}
					if err := run(context.Background(), g); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func TestNudgeChannelsSkipsUninspectedExternalPointAliases(t *testing.T) {
	g, e := channelZGraph()
	external := layoutgraph.NewEdge(e.From, e.To)
	external.Points = []*geo.Point{e.Points[1], geo.NewPoint(120, 150)}
	e.From.Edges = append(e.From.Edges, external)
	original, outside := captureExactRouteTest(e), captureExactRouteTest(external)
	if err := NudgeEdgeChannels(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	original.assertRestored(t)
	outside.assertRestored(t)
}
