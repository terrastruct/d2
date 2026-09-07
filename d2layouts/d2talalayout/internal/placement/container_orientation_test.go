package placement

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type containerOrientationFixture struct {
	graph *layoutgraph.Graph
	root  *layoutgraph.Node
	nodes []*layoutgraph.Node
}

func newContainerOrientationFixture(t *testing.T, direction geo.Orientation, positions []geo.Point, reverse bool) containerOrientationFixture {
	t.Helper()
	g := layoutgraph.NewGraph()
	root := layoutgraph.NewNode(1, 30000, 30000)
	root.TopLeft = geo.NewPoint(0, 0)
	root.SetContainer(true)
	g.AddNewNodeToContainer(nil, root)
	g.Directions[nil] = direction
	var nodes []*layoutgraph.Node
	for i, p := range positions {
		n := layoutgraph.NewNode(layoutgraph.EntityID(i+2), 100, 60)
		n.TopLeft = p.Copy()
		g.AddNewNodeToContainer(root, n)
		nodes = append(nodes, n)
	}
	for i := 1; i < len(nodes); i++ {
		e := g.Connect(nodes[i-1], nodes[i])
		if reverse {
			e.SourceArrowhead = "triangle"
		} else {
			e.TargetArrowhead = "triangle"
		}
	}
	internal := append([]*layoutgraph.Edge(nil), g.Edges...)
	for i := 0; i < 3; i++ {
		n := layoutgraph.NewNode(layoutgraph.EntityID(100+i), 100, 60)
		n.TopLeft = geo.NewPoint(float64(40000+i*200), 40000)
		g.AddNewNodeToContainer(nil, n)
		e := g.Connect(root, n)
		e.TargetArrowhead = "triangle"
	}
	// The combined interior shares the container maps and boundary incidence,
	// but only owns the local placement nodes and their internal edges.
	g.Nodes = nodes
	g.Edges = internal
	g.CellSize = 100
	require.NoError(t, layoutgraph.Validate(context.Background(), "ContainerOrientationFixture", g))
	return containerOrientationFixture{graph: g, root: root, nodes: nodes}
}

func TestContainerOrientationFollowsSurroundingFlow(t *testing.T) {
	for _, direction := range []geo.Orientation{geo.Bottom, geo.Top} {
		for _, reverse := range []bool{false, true} {
			name := fmt.Sprint(direction)
			if reverse {
				name += "/leftward-source-arrow"
			} else {
				name += "/rightward-target-arrow"
			}
			t.Run(name, func(t *testing.T) {
				f := newContainerOrientationFixture(t, direction, []geo.Point{{X: 0, Y: 0}, {X: 400, Y: 0}, {X: 800, Y: 0}}, reverse)
				require.NoError(t, orientSourceInterior(context.Background(), f.graph, f.root, nil))
				for _, e := range f.graph.Edges {
					from, to, ok := e.DirectedEndpoints()
					require.True(t, ok)
					if direction == geo.Bottom {
						require.Greater(t, to.Center().Y, from.Center().Y)
					} else {
						require.Less(t, to.Center().Y, from.Center().Y)
					}
				}
				for _, n := range f.nodes {
					require.Equal(t, 100.0, n.Width)
					require.Equal(t, 60.0, n.Height)
				}
				require.Equal(t, 100.0, f.graph.CellSize)
				require.Equal(t, 30000.0, f.root.Width)
				require.Equal(t, 30000.0, f.root.Height)
			})
		}
	}
}

type containerOrientationNodeState struct {
	pointer       *geo.Point
	point         geo.Point
	width, height float64
}

func captureContainerOrientationNodes(nodes []*layoutgraph.Node) map[*layoutgraph.Node]containerOrientationNodeState {
	states := make(map[*layoutgraph.Node]containerOrientationNodeState)
	for _, n := range nodes {
		states[n] = containerOrientationNodeState{n.TopLeft, *n.TopLeft, n.Width, n.Height}
	}
	return states
}

func assertContainerOrientationNodesRestored(t *testing.T, states map[*layoutgraph.Node]containerOrientationNodeState) {
	t.Helper()
	for n, state := range states {
		require.Same(t, state.pointer, n.TopLeft)
		require.Equal(t, state.point, *n.TopLeft)
		require.Equal(t, state.width, n.Width)
		require.Equal(t, state.height, n.Height)
	}
}

func TestContainerOrientationKeepsUnsupportedOrAlreadyAlignedInterior(t *testing.T) {
	for _, name := range []string{"already-down", "already-up", "no-internal-edges", "surrounding-left", "surrounding-right", "local-direction", "fixed", "loop-envelope", "near", "ancestor-obstacle", "parallel-boundary", "inbound-boundary"} {
		t.Run(name, func(t *testing.T) {
			positions := []geo.Point{{X: 0, Y: 0}, {X: 400, Y: 0}, {X: 800, Y: 0}}
			if name == "already-down" || name == "already-up" {
				positions = []geo.Point{{X: 0, Y: 0}, {X: 0, Y: 400}, {X: 0, Y: 800}}
			}
			f := newContainerOrientationFixture(t, geo.Bottom, positions, name == "already-up")
			var obstacles []geo.Box
			switch name {
			case "no-internal-edges":
				f.graph.ReplaceEdgesUnchecked(nil)
			case "surrounding-left":
				f.graph.Directions[nil] = geo.Left
			case "surrounding-right":
				f.graph.Directions[nil] = geo.Right
			case "local-direction":
				f.graph.Directions[f.root] = geo.Right
			case "fixed":
				f.nodes[0].FixedTopLeft = f.nodes[0].TopLeft.Copy()
			case "loop-envelope":
				f.nodes[0].LoopOffsets = map[geo.Orientation]float64{geo.Right: 80}
			case "near":
				f.nodes[0].Nears[f.root.Edges[0].To] = struct{}{}
			case "ancestor-obstacle":
				obstacles = []geo.Box{{TopLeft: geo.NewPoint(0, 1000), Width: 100, Height: 100}}
			case "parallel-boundary":
				for _, e := range append([]*layoutgraph.Edge(nil), f.root.Edges...) {
					e.Reconnect(f.root.Edges[0].To, true)
				}
			case "inbound-boundary":
				f.root.Edges[0].SourceArrowhead = "triangle"
				f.root.Edges[0].TargetArrowhead = ""
			}
			states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
			require.NoError(t, orientSourceInterior(context.Background(), f.graph, f.root, obstacles))
			assertContainerOrientationNodesRestored(t, states)
		})
	}
}

func newClusterContainerOrientationFixture(t *testing.T, width float64) (containerOrientationFixture, *layoutgraph.Cluster) {
	t.Helper()
	f := newContainerOrientationFixture(t, geo.Bottom, []geo.Point{{X: 0, Y: 0}, {X: 0, Y: 500}, {X: 1000, Y: 0}, {X: 1000, Y: 100}}, false)
	f.graph.ReplaceEdgesUnchecked(nil)
	for i := 0; i < 2; i++ {
		e := f.graph.Connect(f.nodes[i], f.nodes[i+2])
		e.TargetArrowhead = "triangle"
		f.nodes[i+2].Width = width
		f.nodes[i+2].Height = 40
	}
	vessel := layoutgraph.NewNode(50, width, 100)
	vessel.TopLeft = geo.NewPoint(1000, 0)
	vessel.SetClusterVessel(true)
	cluster := &layoutgraph.Cluster{Nodes: f.nodes[2:], Vessel: vessel, Graph: f.graph, Container: f.root, Arrangement: layoutgraph.Column, DesiredArrangement: layoutgraph.Column, Padding: 20}
	grouping.AddCluster(f.graph, cluster)
	cluster.Resize(vessel)
	cluster.SyncGeometry()
	abductClusterEdgesForOptimizationFixture(cluster)
	f.nodes = append(f.nodes, vessel)
	require.NoError(t, layoutgraph.Validate(context.Background(), "ContainerOrientationClusterFixture", f.graph))
	return f, cluster
}

type observeContainerClusterTurn struct {
	context.Context
	cluster      *layoutgraph.Cluster
	observed     bool
	cancel       bool
	maximumWidth float64
}

func (ctx *observeContainerClusterTurn) Err() error {
	if ctx.cluster.Arrangement == layoutgraph.Row {
		ctx.observed = true
		if ctx.cluster.Vessel.Width > ctx.maximumWidth {
			ctx.maximumWidth = ctx.cluster.Vessel.Width
		}
		if ctx.cancel {
			return context.Canceled
		}
	}
	return ctx.Context.Err()
}

func TestContainerOrientationCancellationRestoresClusterAndGeometry(t *testing.T) {
	f, cluster := newClusterContainerOrientationFixture(t, 100)
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	ctx := &observeContainerClusterTurn{Context: context.Background(), cluster: cluster, cancel: true}
	err := orientSourceInterior(ctx, f.graph, f.root, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, ctx.observed, "cancellation must happen after the cluster was mutated")
	assertContainerOrientationNodesRestored(t, states)
	require.Equal(t, layoutgraph.Column, cluster.Arrangement)
	require.Equal(t, layoutgraph.Column, cluster.DesiredArrangement)
	require.Equal(t, 20.0, cluster.Padding)
	require.Equal(t, 100.0, f.graph.CellSize)
}

func TestContainerOrientationOversizeCandidateRetainsValidOriginal(t *testing.T) {
	f, cluster := newClusterContainerOrientationFixture(t, 16000)
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	ctx := &observeContainerClusterTurn{Context: context.Background(), cluster: cluster}
	require.NoError(t, orientSourceInterior(ctx, f.graph, f.root, nil))
	require.True(t, ctx.observed, "the oversized candidate must actually have been considered")
	require.Greater(t, ctx.maximumWidth, float64(limits.MaxGraphSize))
	assertContainerOrientationNodesRestored(t, states)
	require.Equal(t, layoutgraph.Column, cluster.Arrangement)
	require.Equal(t, layoutgraph.Column, cluster.DesiredArrangement)
	require.Equal(t, 20.0, cluster.Padding)
	require.Equal(t, 100.0, f.graph.CellSize)
}

func TestContainerOrientationTwoDestinationsAndInternalNears(t *testing.T) {
	f := newContainerOrientationFixture(t, geo.Bottom, []geo.Point{{X: 0, Y: 0}, {X: 400, Y: 0}, {X: 800, Y: 0}}, false)
	f.root.Edges[2].Reconnect(f.root.Edges[0].To, true)
	f.nodes[0].Nears[f.nodes[1]] = struct{}{}
	f.nodes[1].Nears[f.nodes[0]] = struct{}{}
	require.NoError(t, orientSourceInterior(context.Background(), f.graph, f.root, nil))
	require.Greater(t, f.nodes[1].Center().Y, f.nodes[0].Center().Y)
	require.Greater(t, f.nodes[2].Center().Y, f.nodes[1].Center().Y)
}

func TestContainerOrientationCountsSemanticBoundaryArrows(t *testing.T) {
	f := newContainerOrientationFixture(t, geo.Bottom, []geo.Point{{X: 0, Y: 0}, {X: 400, Y: 0}, {X: 800, Y: 0}}, false)
	for _, e := range append([]*layoutgraph.Edge(nil), f.root.Edges...) {
		destination := e.To
		e.Reconnect(destination, false)
		e.Reconnect(f.root, true)
		e.SourceArrowhead = "triangle"
		e.TargetArrowhead = ""
	}
	require.NoError(t, orientSourceInterior(context.Background(), f.graph, f.root, nil))
	require.Greater(t, f.nodes[1].Center().Y, f.nodes[0].Center().Y)
}

func TestContainerOrientationOpposingVerticalEdgesDoNotManufactureTransverseFlow(t *testing.T) {
	f := newContainerOrientationFixture(t, geo.Bottom, []geo.Point{{X: 0, Y: 0}, {X: 0, Y: 400}, {X: 400, Y: 0}, {X: 400, Y: 400}}, false)
	f.graph.ReplaceEdgesUnchecked(nil)
	for _, pair := range [][2]int{{0, 1}, {3, 2}, {0, 2}} {
		e := f.graph.Connect(f.nodes[pair[0]], f.nodes[pair[1]])
		e.TargetArrowhead = "triangle"
	}
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	require.NoError(t, orientSourceInterior(context.Background(), f.graph, f.root, nil))
	assertContainerOrientationNodesRestored(t, states)
}

func TestContainerOrientationParallelInternalEdgesDoNotOutvoteFlow(t *testing.T) {
	f := newContainerOrientationFixture(t, geo.Bottom, []geo.Point{{X: 0, Y: 0}, {X: 0, Y: 400}, {X: 400, Y: 0}}, false)
	f.graph.ReplaceEdgesUnchecked(nil)
	e := f.graph.Connect(f.nodes[0], f.nodes[1])
	e.TargetArrowhead = "triangle"
	for i := 0; i < 20; i++ {
		e := f.graph.Connect(f.nodes[0], f.nodes[2])
		e.TargetArrowhead = "triangle"
	}
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	require.NoError(t, orientSourceInterior(context.Background(), f.graph, f.root, nil))
	assertContainerOrientationNodesRestored(t, states)
}

func TestContainerOrientationContainerPaddingCannotPushCandidatePastLimit(t *testing.T) {
	// Two 14950px-wide members fit as a column. In a row they require 29920px,
	// which fits the raw content limit but not the containing box with padding.
	f, cluster := newClusterContainerOrientationFixture(t, 14950)
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	ctx := &observeContainerClusterTurn{Context: context.Background(), cluster: cluster}
	require.NoError(t, orientSourceInterior(ctx, f.graph, f.root, nil))
	require.True(t, ctx.observed)
	if cluster.Arrangement == layoutgraph.Row {
		f.root.FitToGraph(f.graph, f.graph.ContainerPadding(f.root, false))
		t.Fatalf("orientation accepted content width %.0f that fits to a %.0f-wide container (limit %d)", ctx.maximumWidth, f.root.Width, limits.MaxGraphSize)
	}
	assertContainerOrientationNodesRestored(t, states)
	require.Equal(t, layoutgraph.Column, cluster.Arrangement)
	require.Equal(t, 20.0, cluster.Padding)
}

func TestContainerOrientationSiblingLoopDisablesOtherwiseEligibleSource(t *testing.T) {
	f := newContainerOrientationFixture(t, geo.Bottom, []geo.Point{{X: 0, Y: 0}, {X: 400, Y: 0}, {X: 800, Y: 0}}, false)
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	whole := layoutgraph.NewGraph()
	sibling := layoutgraph.NewNode(200, 100, 60)
	sibling.TopLeft = geo.NewPoint(3000, 0)
	whole.AddNewNodeToContainer(nil, sibling)
	whole.Connect(sibling, sibling).TargetArrowhead = "triangle"
	ctx, err := orientationContext(context.Background(), whole)
	require.NoError(t, err)
	require.NoError(t, orientSourceInterior(ctx, f.graph, f.root, nil))
	assertContainerOrientationNodesRestored(t, states)

	// The same source is eligible when the attempt has no sibling loop.
	whole.ReplaceEdgesUnchecked(nil)
	ctx, err = orientationContext(context.Background(), whole)
	require.NoError(t, err)
	require.NoError(t, orientSourceInterior(ctx, f.graph, f.root, nil))
	require.Greater(t, f.nodes[1].Center().Y, f.nodes[0].Center().Y)
}

func TestContainerOrientationCanceledPreflightDoesNotMutate(t *testing.T) {
	f := newContainerOrientationFixture(t, geo.Bottom, []geo.Point{{X: 0, Y: 0}, {X: 400, Y: 0}, {X: 800, Y: 0}}, false)
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := orientationContext(ctx, f.graph)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, orientSourceInterior(ctx, f.graph, f.root, nil), context.Canceled)
	assertContainerOrientationNodesRestored(t, states)
}

func TestContainerOrientationPreflightSeesLoopOnHiddenGroupMember(t *testing.T) {
	f, cluster := newClusterContainerOrientationFixture(t, 100)
	member := cluster.Nodes[0]
	require.NotContains(t, f.graph.Nodes, member)
	// Sequence preprocessing can leave a member's internal self-edge active
	// even while that member is represented by a vessel in graph.Nodes.
	f.graph.Connect(member, member).TargetArrowhead = "triangle"
	states := captureContainerOrientationNodes(append(append([]*layoutgraph.Node(nil), f.nodes...), f.root))
	ctx, err := orientationContext(context.Background(), f.graph)
	require.NoError(t, err)
	require.NoError(t, orientSourceInterior(ctx, f.graph, f.root, nil))
	assertContainerOrientationNodesRestored(t, states)
	require.Equal(t, layoutgraph.Column, cluster.Arrangement)
}
