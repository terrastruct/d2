package routing

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestNewOVGEdgeRouter(t *testing.T) {
	// n2, n3 are cluster
	// n1-n4 is the shortest edge
	// n1-n5 is the midway edge
	// n1-n6 is the longest edge
	graph := layoutgraph.NewGraph()
	n1 := layoutgraph.NewNode(1, 5, 5)
	n1.TopLeft = geo.NewPoint(50, 50)
	n2 := layoutgraph.NewNode(2, 5, 5)
	n2.TopLeft = geo.NewPoint(20, 40)
	n3 := layoutgraph.NewNode(3, 5, 5)
	n3.TopLeft = geo.NewPoint(20, 60)
	n4 := layoutgraph.NewNode(4, 5, 5)
	n4.TopLeft = geo.NewPoint(60, 50)
	n5 := layoutgraph.NewNode(5, 5, 5)
	n5.TopLeft = geo.NewPoint(75, 50)
	n6 := layoutgraph.NewNode(6, 5, 5)
	n6.TopLeft = geo.NewPoint(90, 50)
	graph.AddNode(n1)
	graph.AddNode(n2)
	graph.AddNode(n3)
	graph.AddNode(n4)
	graph.AddNode(n5)
	// add edges in random order to ensure they were sorted
	e16 := graph.Connect(n1, n6)
	e13 := graph.Connect(n1, n3)
	e14 := graph.Connect(n1, n4)
	e12 := graph.Connect(n1, n2)
	e15 := graph.Connect(n1, n5)
	grouping.AddCluster(graph, &layoutgraph.Cluster{
		Vessel:    layoutgraph.NewNode(100, 20, 40),
		Container: layoutgraph.NewNode(101, 20, 40),
		Nodes:     []*layoutgraph.Node{n2, n3},
	})

	ovg := NewOVG(nil)
	ovgNode1 := ovg.AddNode(NewOVGNode(geo.NewPoint(10, 10)))
	ovgNode2 := ovg.AddNode(NewOVGNode(geo.NewPoint(15, 15)))
	existingRoutes := []*Route{
		{GEdge: graph.Edges[0], OVGNodes: []*OVGNode{ovgNode1}},
		{GEdge: graph.Edges[1], OVGNodes: []*OVGNode{ovgNode2}},
	}

	testCases := []struct {
		flavor            RouteGenerationFlavor
		expectedEdgeOrder []*layoutgraph.Edge
	}{
		{
			flavor:            ShortestToLongest,
			expectedEdgeOrder: []*layoutgraph.Edge{e13, e12, e14, e15, e16},
		},
		{
			flavor:            LongestToShortest,
			expectedEdgeOrder: []*layoutgraph.Edge{e13, e12, e16, e15, e14},
		},
		{
			flavor:            Default,
			expectedEdgeOrder: []*layoutgraph.Edge{e13, e12, e16, e14, e15},
		},
	}
	for _, tc := range testCases {
		t.Run(string(tc.flavor), func(t *testing.T) {
			router, _ := newOVGEdgeRouter(context.Background(), tc.flavor, ovg, graph, existingRoutes, graph.Edges)

			// check references
			assert.Equal(t, ovg, router.ovg)
			assert.Equal(t, graph, router.graph)
			assert.Equal(t, tc.flavor, router.flavor)
			assert.Equal(t, graph.TurnCost(), router.turnCost)
			assert.Equal(t, graph.CrossingCost(), router.crossingCost)
			assert.Equal(t, graph.NonCenterPortCost(), router.nonCenterPortCost)
			// check routes were copied
			assert.False(t, &existingRoutes == &router.routes, "Existing routes should be copied to avoid slice reference sharing")
			assert.Equal(t, existingRoutes, router.routes, "Routes should've been copied")
			// check nodes were assigned to routes
			assert.Equal(t, []*Route{existingRoutes[0]}, router.pointToRoute[*ovgNode1.Point])
			assert.Equal(t, []*Route{existingRoutes[1]}, router.pointToRoute[*ovgNode2.Point])

			assert.Equal(t, tc.expectedEdgeOrder, router.edges)
		})
	}
}

func TestAddRoute(t *testing.T) {
	g := layoutgraph.NewGraph()
	ovg := NewOVG(nil)

	router, _ := newOVGEdgeRouter(context.Background(), ShortestToLongest, ovg, g, []*Route{}, []*layoutgraph.

		/*
			*
			|
			*--*
			   |
			   *--*
		*/Edge{})

	route := &Route{
		GEdge: layoutgraph.NewEdge(layoutgraph.NewNode(1, 100, 100), layoutgraph.NewNode(2, 100, 100)),
		OVGNodes: []*OVGNode{
			NewOVGNode(geo.NewPoint(5, 5)),
			NewOVGNode(geo.NewPoint(5, 10)),
			NewOVGNode(geo.NewPoint(15, 10)),
			NewOVGNode(geo.NewPoint(15, 15)),
			NewOVGNode(geo.NewPoint(20, 15)),
		},
	}
	router.addRoute(route)
	router.addRoute(route) // add twice won't duplicate edges

	assert.Equal(t, 2, len(router.routes))

	// the first and last edges are from port to center and we don't need to store then for intersection purposes
	assert.Equal(t, 1, len(router.edgeSet.verticalEdges))
	assert.Equal(t, 1, len(router.edgeSet.horizontalEdges))

	for _, node := range route.OVGNodes {
		assert.Equal(t, router.pointToRoute[*node.Point], []*Route{route, route})
	}
}

func TestAddRouteRejectsDuplicateOVGNode(t *testing.T) {
	router, err := newOVGEdgeRouter(context.Background(), ShortestToLongest, NewOVG(nil), layoutgraph.NewGraph(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := NewOVGNode(geo.NewPoint(5, 5))
	route := &Route{
		GEdge: layoutgraph.NewEdge(layoutgraph.NewNode(1, 10, 10), layoutgraph.NewNode(2, 10, 10)),
		OVGNodes: []*OVGNode{
			duplicate,
			NewOVGNode(geo.NewPoint(10, 5)),
			duplicate,
		},
	}
	if err := router.addRoute(route); err == nil || !strings.Contains(err.Error(), "duplicate OVGNode") {
		t.Fatalf("addRoute error = %v, want duplicate OVGNode", err)
	}
}

func TestAddRouteAllowsLoopCenterAtBothEnds(t *testing.T) {
	router, err := newOVGEdgeRouter(context.Background(), ShortestToLongest, NewOVG(nil), layoutgraph.NewGraph(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	node := layoutgraph.NewNode(1, 10, 10)
	center := NewOVGNode(geo.NewPoint(5, 5))
	route := &Route{
		GEdge: layoutgraph.NewEdge(node, node),
		OVGNodes: []*OVGNode{
			center,
			NewOVGNode(geo.NewPoint(10, 5)),
			center,
		},
	}
	if err := router.addRoute(route); err != nil {
		t.Fatalf("add loop route: %v", err)
	}
}

func TestSortEdgesTopDownLeftRight(t *testing.T) {
	// ┌──────────┐                    ┌──────────┐
	// │          │                    │          │
	// │    n1    ├────────┐      ┌────┤    n2    │
	// │          │        │      │    │          │
	// └─────┬────┘        │      │    └──────┬───┘
	//       │             │      │           │
	//       │             │      │           │
	//       │             │      │           │
	//    e4 │           e2│      │e1         │e5
	//       │             │      │           │
	//       │             │      │           │
	//       │             │      │           │
	// ┌─────▼────┐        │      │     ┌─────▼────┐
	// │          │        └──────┼─────►          │
	// │    n3    │               │     │    n4    │
	// │          ◄───────────────┘     │          │
	// └─────▲────┘                     └─────▲────┘
	//       │                                │
	//       │                                │
	//       │                                │
	//       │e6                              │e3
	//       │                                │
	//       │                                │
	// ┌─────┴────┐                     ┌─────┴────┐
	// │          │                     │          │
	// │    n6    │                     │    n5    │
	// │          │                     │          │
	// └──────────┘                     └──────────┘
	g := layoutgraph.NewGraph()

	n1 := g.AddNode(layoutgraph.NewNode(1, 1, 1))
	n1.TopLeft = geo.NewPoint(0, 0)
	n2 := g.AddNode(layoutgraph.NewNode(2, 1, 1))
	n2.TopLeft = geo.NewPoint(100, 0)
	n3 := g.AddNode(layoutgraph.NewNode(3, 1, 1))
	n3.TopLeft = geo.NewPoint(0, 100)
	n4 := g.AddNode(layoutgraph.NewNode(4, 1, 1))
	n4.TopLeft = geo.NewPoint(100, 100)
	n5 := g.AddNode(layoutgraph.NewNode(5, 1, 1))
	n5.TopLeft = geo.NewPoint(100, 200)
	n6 := g.AddNode(layoutgraph.NewNode(6, 1, 1))
	n6.TopLeft = geo.NewPoint(0, 200)

	e1 := g.Connect(n2, n3)
	e1.SourceArrowhead = layoutgraph.NoArrowhead
	e1.TargetArrowhead = layoutgraph.TriangleArrowhead
	e2 := g.Connect(n1, n4)
	e2.SourceArrowhead = layoutgraph.NoArrowhead
	e2.TargetArrowhead = layoutgraph.TriangleArrowhead
	e3 := g.Connect(n5, n4)
	e3.SourceArrowhead = layoutgraph.NoArrowhead
	e3.TargetArrowhead = layoutgraph.TriangleArrowhead
	e4 := g.Connect(n1, n3)
	e4.SourceArrowhead = layoutgraph.NoArrowhead
	e4.TargetArrowhead = layoutgraph.TriangleArrowhead
	e5 := g.Connect(n2, n4)
	e5.SourceArrowhead = layoutgraph.NoArrowhead
	e5.TargetArrowhead = layoutgraph.TriangleArrowhead
	e6 := g.Connect(n6, n3)
	e6.SourceArrowhead = layoutgraph.NoArrowhead
	e6.TargetArrowhead = layoutgraph.TriangleArrowhead

	sortEdges(TopDownLeftRight, g.Edges, make(map[*layoutgraph.Node]struct{}))

	expectedOrder := []*layoutgraph.Edge{e4, e2, e1, e5, e6, e3}
	/*
		e4: ties with e2, e1, e5 on the top row
			wins e1 because n1 is on the left of n2
			wins e5 because n1 is on the left of n2
			wins e2 because n3 in on the left of n4
		e2: ties with e1 and e5 on the top row
			wins e1 because n1 is on the left of n2
			wins e5 because n1 is on the left of n2
		e1: ties with e5 on the top row
			wins e5 because n3 is on the left of n4
		e5: is above e3 and e6
		e6: wins e3 because n3 is on the left of n4
	*/
	assert.Equal(t, len(expectedOrder), len(g.Edges))
	for i := 0; i < len(expectedOrder); i++ {
		assert.Equal(t, expectedOrder[i], g.Edges[i])
	}
}

func TestSortEdgesPreservesDistanceTies(t *testing.T) {
	g := layoutgraph.NewGraph()
	from := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	from.TopLeft = geo.NewPoint(0, 0)
	to := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	to.TopLeft = geo.NewPoint(100, 0)
	first := g.Connect(from, to)
	second := g.Connect(from, to)

	for _, flavor := range []RouteGenerationFlavor{ShortestToLongest, LongestToShortest} {
		edges := []*layoutgraph.Edge{second, first}
		sortEdges(flavor, edges, nil)
		if edges[0] != second || edges[1] != first {
			t.Fatalf("%s changed equal-distance order", flavor)
		}
	}
}

func TestSortEdgesPreservesClusterTies(t *testing.T) {
	g := layoutgraph.NewGraph()
	clusterNode := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	clusterNode.TopLeft = geo.NewPoint(0, 0)
	outside := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	outside.TopLeft = geo.NewPoint(100, 0)
	otherOutside := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	otherOutside.TopLeft = geo.NewPoint(200, 0)
	firstClusterEdge := g.Connect(clusterNode, outside)
	secondClusterEdge := g.Connect(clusterNode, otherOutside)
	plainEdge := g.Connect(outside, otherOutside)

	edges := []*layoutgraph.Edge{plainEdge, secondClusterEdge, firstClusterEdge}
	sortEdges(Default, edges, map[*layoutgraph.Node]struct{}{clusterNode: {}})
	if edges[0] != secondClusterEdge || edges[1] != firstClusterEdge || edges[2] != plainEdge {
		t.Fatal("cluster-first sorting changed the relative order within a tie")
	}
}

func TestSortEdgesPreservesNaNDistanceTie(t *testing.T) {
	g := layoutgraph.NewGraph()
	nonFiniteFrom := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	nonFiniteFrom.TopLeft = geo.NewPoint(math.NaN(), 0)
	finiteFrom := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	finiteFrom.TopLeft = geo.NewPoint(0, 0)
	to := g.AddNode(layoutgraph.NewNode(3, 10, 10))
	to.TopLeft = geo.NewPoint(100, 0)
	nonFinite := g.Connect(nonFiniteFrom, to)
	finite := g.Connect(finiteFrom, to)

	for _, flavor := range []RouteGenerationFlavor{ShortestToLongest, LongestToShortest} {
		edges := []*layoutgraph.Edge{nonFinite, finite}
		sortEdges(flavor, edges, nil)
		if edges[0] != nonFinite || edges[1] != finite {
			t.Fatalf("%s no longer treats NaN distance as equivalent", flavor)
		}
	}
}
