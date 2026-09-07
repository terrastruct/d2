package routing

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

	"github.com/d2lang/d2/lib/geo"
)

func TestAddNodeUnchecked(t *testing.T) {
	ovg := NewOVG(nil)

	ovg.AddNodeUnchecked(NewOVGNode(geo.NewPoint(1, 1)))
	ovg.AddNodeUnchecked(NewOVGNode(geo.NewPoint(1, 1)))

	assert.Equal(t, 2, len(ovg.Nodes))
}

func TestAddNode(t *testing.T) {
	ovg := NewOVG(nil)

	p := geo.NewPoint(42, 23)
	node1 := NewOVGNode(p)
	n1 := ovg.AddNode(node1)
	assert.Equal(t, node1, n1)

	node2 := NewOVGNode(p)
	n2 := ovg.AddNode(node2)
	assert.Equal(t, node1, n2)

	assert.Equal(t, 1, len(ovg.Nodes))
	assert.Equal(t, node1, ovg.Nodes[0])
}

func TestBuildOVGFromGraphCanonicalizesCoincidentPorts(t *testing.T) {
	g := layoutgraph.NewGraph()
	left := layoutgraph.NewNode(1, 100, 100)
	left.TopLeft = geo.NewPoint(0, 0)
	right := layoutgraph.NewNode(2, 100, 100)
	right.TopLeft = geo.NewPoint(100, 0)
	g.AddNodeUnchecked(left)
	g.AddNodeUnchecked(right)
	edge := g.Connect(left, right)

	ovg, err := buildOVGFromGraph(context.Background(), g, nil)
	if err != nil {
		t.Fatal(err)
	}

	ovgNodes := make(map[*OVGNode]struct{}, len(ovg.Nodes))
	for _, node := range ovg.Nodes {
		ovgNodes[node] = struct{}{}
	}
	for graphNode, ports := range ovg.Ports {
		for _, port := range ports {
			if _, ok := ovgNodes[port]; !ok {
				t.Fatalf("port for node %d at %v is not a canonical OVG node", graphNode.ID, port.Point)
			}
			if !port.isPortOf(graphNode) {
				t.Fatalf("port at %v lost ownership by node %d", port.Point, graphNode.ID)
			}
		}
	}

	sharedPorts := 0
	for _, leftPort := range ovg.Ports[left] {
		for _, rightPort := range ovg.Ports[right] {
			if *leftPort.Point != *rightPort.Point {
				continue
			}
			sharedPorts++
			if leftPort != rightPort {
				t.Fatalf("coincident ports at %v were not canonicalized", leftPort.Point)
			}
			if !leftPort.hasPortDirection(left, geo.Right) || !leftPort.hasPortDirection(right, geo.Left) {
				t.Fatalf("shared port at %v lost per-owner directions: owners=%v leftPort=%p rightPort=%p", leftPort.Point, leftPort.portOwnersByNode, leftPort, rightPort)
			}
		}
	}
	if sharedPorts == 0 {
		t.Fatal("expected touching nodes to share at least one port")
	}

	router, err := newOVGEdgeRouter(context.Background(), Default, ovg, g, nil, g.Edges)
	if err != nil {
		t.Fatal(err)
	}
	route, _, err := router.search(context.Background(), edge)
	if err != nil {
		t.Fatalf("could not route between touching nodes: %v", err)
	}
	if len(route) < 3 {
		t.Fatalf("expected center-to-port-to-center route, got %d OVG nodes", len(route))
	}
}

func (ovg *OVG) ports(n *layoutgraph.Node, o geo.Orientation) []*OVGNode {
	var ports []*OVGNode
	seen := make(map[*OVGNode]struct{}, len(ovg.Ports[n]))
	for _, port := range ovg.Ports[n] {
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		if port.hasPortDirection(n, o) {
			ports = append(ports, port)
		}
	}

	return ports
}

func TestTinyNodeRoutesDirectlyToNodeAbove(t *testing.T) {
	g := layoutgraph.NewGraph()
	tiny := g.AddNode(layoutgraph.NewNode(1, 1, 1))
	tiny.TopLeft = geo.NewPoint(100, 100)
	target := g.AddNode(layoutgraph.NewNode(2, 20, 20))
	target.TopLeft = geo.NewPoint(90, 0)
	edge := g.Connect(tiny, target)

	ovg := NewOVG(g.Nodes)
	ovg.addPorts(g, newBackgroundOVGBuildGuardForTest(t))
	for _, direction := range []geo.Orientation{geo.Top, geo.Right, geo.Bottom, geo.Left} {
		if len(ovg.ports(tiny, direction)) == 0 {
			t.Fatalf("1x1 node lost its %v port role", direction)
		}
	}

	g.ComputeCellSize()
	if _, err := routeEdges(withTestLogger(context.Background(), t), g, nil); err != nil {
		t.Fatal(err)
	}
	want := []geo.Point{{X: 100, Y: 100}, {X: 100, Y: 20}}
	if len(edge.Points) != len(want) {
		t.Fatalf("route = %v, want direct route %v", edge.Points, want)
	}
	for i, point := range edge.Points {
		if point == nil || *point != want[i] {
			t.Fatalf("route point %d = %v, want %v (route %v)", i, point, want[i], edge.Points)
		}
	}
}

func TestHierarchyMirrorTransformsSharedPortsOnce(t *testing.T) {
	for _, direction := range []geo.Orientation{geo.Top, geo.Left} {
		t.Run(direction.ToString(), func(t *testing.T) {
			g := layoutgraph.NewGraph()
			left := g.AddNode(layoutgraph.NewNode(1, 100, 100))
			left.TopLeft = geo.NewPoint(0, 0)
			right := g.AddNode(layoutgraph.NewNode(2, 100, 100))
			right.TopLeft = geo.NewPoint(100, 0)
			g.Directions[nil] = direction
			hierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{left: 0, right: 0})
			left.Hierarchy = hierarchy
			right.Hierarchy = hierarchy

			ovg, err := newOVGForHierarchy(g, hierarchy, newBackgroundOVGBuildGuardForTest(t))
			if err != nil {
				t.Fatal(err)
			}
			shared := 0
			for _, leftPort := range ovg.Ports[left] {
				if !left.ContainsPointOnBox(leftPort.Point) {
					t.Fatalf("left port %v was not restored to its owner after %v mirroring", leftPort.Point, direction)
				}
				for _, rightPort := range ovg.Ports[right] {
					if *leftPort.Point != *rightPort.Point {
						continue
					}
					shared++
					if leftPort != rightPort {
						t.Fatalf("shared hierarchy port at %v is not canonical", leftPort.Point)
					}
					if !right.ContainsPointOnBox(rightPort.Point) {
						t.Fatalf("right port %v was not restored to its owner after %v mirroring", rightPort.Point, direction)
					}
					if !leftPort.hasPortDirection(left, geo.Right) || !leftPort.hasPortDirection(right, geo.Left) {
						t.Fatalf("shared port directions were not restored: %v", leftPort.portOwnersByNode)
					}
				}
			}
			if shared == 0 {
				t.Fatal("test setup produced no shared hierarchy ports")
			}
			for _, node := range ovg.Nodes {
				if ovg.OccupiedPoints[*node.Point] != node {
					t.Fatalf("occupied-point index was not restored for OVG node %v", node.Point)
				}
			}
		})
	}
}

func TestMergePortsCanonicalizesBaseAndHierarchyPorts(t *testing.T) {
	g := layoutgraph.NewGraph()
	base := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	base.TopLeft = geo.NewPoint(0, 0)
	hierarchical := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	hierarchical.TopLeft = geo.NewPoint(100, 0)
	hierarchy := newHierarchyWithLevels(map[*layoutgraph.Node]int{hierarchical: 0})
	hierarchical.Hierarchy = hierarchy

	ovg, err := buildOVGFromGraph(context.Background(), g, nil)
	if err != nil {
		t.Fatal(err)
	}
	sharedPoint := geo.Point{X: 100, Y: 50}
	canonical := ovg.OccupiedPoints[sharedPoint]
	if canonical == nil {
		t.Fatalf("no canonical OVG node at shared point %v", sharedPoint)
	}
	if !canonical.hasPortDirection(base, geo.Right) || !canonical.hasPortDirection(hierarchical, geo.Left) {
		t.Fatalf("shared base/hierarchy port lost ownership: %v", canonical.portOwnersByNode)
	}
	for _, owner := range []*layoutgraph.Node{base, hierarchical} {
		found := false
		for _, port := range ovg.Ports[owner] {
			if *port.Point == sharedPoint {
				found = true
				if port != canonical {
					t.Fatalf("owner %d references noncanonical shared port %p, want %p", owner.ID, port, canonical)
				}
			}
		}
		if !found {
			t.Fatalf("owner %d has no port at shared point", owner.ID)
		}
	}
	for _, edge := range ovg.Edges {
		if *edge.From.Point == sharedPoint && edge.From != canonical {
			t.Fatalf("edge source at shared coordinate was not rewired to canonical port")
		}
		if *edge.To.Point == sharedPoint && edge.To != canonical {
			t.Fatalf("edge target at shared coordinate was not rewired to canonical port")
		}
	}
}

func TestTunnelKeepsExistingPortDirections(t *testing.T) {
	g := layoutgraph.NewGraph()
	left := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	left.TopLeft = geo.NewPoint(0, 0)
	right := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	right.TopLeft = geo.NewPoint(300, 0)
	g.Connect(left, right)

	ovg := NewOVG(g.Nodes)
	ovg.addPorts(g, newBackgroundOVGBuildGuardForTest(t))
	if err := ovg.addTunnels(g, newBackgroundOVGBuildGuardForTest(t)); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		owner     *layoutgraph.Node
		point     geo.Point
		direction geo.Orientation
	}{
		{owner: left, point: geo.Point{X: 100, Y: 50}, direction: geo.Right},
		{owner: right, point: geo.Point{X: 300, Y: 50}, direction: geo.Left},
	} {
		port := ovg.OccupiedPoints[test.point]
		if port == nil || !port.IsTunnel {
			t.Fatalf("expected tunnel endpoint at %v", test.point)
		}
		directions, ok := port.portDirectionsFor(test.owner)
		if !ok || !directions.has(test.direction) {
			t.Fatalf("tunnel endpoint %v lost %v role: %v", test.point, test.direction, directions)
		}
		if directions.has(geo.TopLeft) {
			t.Fatalf("tunnel endpoint %v gained zero-value diagonal role: %v", test.point, directions)
		}
	}
}

func TestAddTreeNodesSharesOccupiedAlignedPort(t *testing.T) {
	for _, tc := range []struct {
		name          string
		childIsSource bool
	}{
		{name: "child is target"},
		{name: "child is source", childIsSource: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			parent := layoutgraph.NewNode(1, 101, 100)
			parent.TopLeft = geo.NewPoint(0, 0)
			child := layoutgraph.NewNode(2, 100, 100)
			child.TopLeft = geo.NewPoint(0, 200)
			other := layoutgraph.NewNode(3, 20, 20)
			// The top-center port is (51, 200), the aligned child port that
			// results from the parent's odd width below.
			other.TopLeft = geo.NewPoint(41, 200)
			g.AddNodeUnchecked(parent)
			g.AddNodeUnchecked(child)
			g.AddNodeUnchecked(other)
			var edge *layoutgraph.Edge
			if tc.childIsSource {
				edge = g.Connect(child, parent)
			} else {
				edge = g.Connect(parent, child)
			}
			tree := layoutgraph.NewTree(child)
			tree.SentinelEdge = edge
			tree.Orientation = geo.Bottom
			g.NodeToTree = make(map[*layoutgraph.Node]*layoutgraph.Tree)
			g.NodeToTree[child] = tree

			ovg, err := buildOVGFromGraph(context.Background(), g, nil)
			if err != nil {
				t.Fatal(err)
			}

			// The aligned child coordinate is preoccupied by the unrelated node's
			// top port. The complete OVG must use that node as the child's port
			// too, including when routing starts from the child.
			alignedPoint := geo.Point{X: 51, Y: 200}
			canonical := ovg.OccupiedPoints[alignedPoint]
			if canonical == nil || !canonical.isPortOf(other) {
				t.Fatal("test setup did not preoccupy the aligned port")
			}
			if !canonical.isPortOf(child) || !canonical.isPortOf(other) {
				t.Fatalf("aligned tree port lost an owner: %v", canonical.portOwnersByNode)
			}
			if !canonical.hasPortDirection(child, geo.Top) {
				t.Fatalf("aligned child port directions = %v, want to include %v", canonical.portOwnersByNode[child].directions, geo.Top)
			}
			foundCanonical := slices.Contains(ovg.Ports[child], canonical)
			if !foundCanonical {
				t.Fatal("child port list did not receive canonical aligned port")
			}

			treePath := treeEdgePath(tree, ovg.Ports, nil)
			childPort := treePath.TargetPortNode
			if tc.childIsSource {
				childPort = treePath.SourcePortNode
			}
			if childPort != canonical {
				t.Fatalf("tree path used disconnected aligned port %p, want canonical %p", childPort, canonical)
			}
			route, err := routeSentinelEdge(tree, ovg.Ports, ovg.Centers, nil)
			if err != nil {
				t.Fatalf("routeSentinelEdge failed: %v (child port=%v edges=%d source midpoint=%v target midpoint=%v)", err, canonical.Point, len(canonical.Edges), treePath.SourceMidpoint, treePath.TargetMidpoint)
			}
			routeChildPort := route.OVGNodes[len(route.OVGNodes)-2]
			if tc.childIsSource {
				routeChildPort = route.OVGNodes[1]
			}
			if routeChildPort != canonical {
				t.Fatalf("route used child port %p, want canonical %p", routeChildPort, canonical)
			}
		})
	}
}

func TestFixedOverlapsCacheIncludesNodeSubset(t *testing.T) {
	g := layoutgraph.NewGraph()
	fixed := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	fixed.TopLeft = geo.NewPoint(0, 0)
	fixed.FixedTopLeft = fixed.TopLeft.Copy()
	overlapping := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	overlapping.TopLeft = geo.NewPoint(0, 0)

	subset := layoutgraph.Nodes{fixed}
	full := layoutgraph.Nodes{fixed, overlapping}
	ovg := NewOVG(full)
	guard := newOVGBuildGuardForTest(t.Context(), t)

	got, err := ovg.fixedOverlapsForBuild(g, subset, guard)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("single-node subset unexpectedly had fixed overlaps: %v", got)
	}
	got, err = ovg.fixedOverlapsForBuild(g, full, guard)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("full node set did not recompute fixed overlaps: %v", got)
	} else if _, ok := got[fixed]; !ok {
		t.Fatalf("full node set did not include overlapping fixed node: %v", got)
	}
	got, err = ovg.fixedOverlapsForBuild(g, subset, guard)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("switching back to the subset reused the full-set cache: %v", got)
	}
}

func TestFixedOverlapsCacheConcurrentSubsets(t *testing.T) {
	g := layoutgraph.NewGraph()
	fixed := g.AddNode(layoutgraph.NewNode(1, 10, 10))
	fixed.TopLeft = geo.NewPoint(0, 0)
	fixed.FixedTopLeft = fixed.TopLeft.Copy()
	overlapping := g.AddNode(layoutgraph.NewNode(2, 10, 10))
	overlapping.TopLeft = geo.NewPoint(0, 0)

	subset := layoutgraph.Nodes{fixed}
	full := layoutgraph.Nodes{fixed, overlapping}
	ovg := NewOVG(full)

	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(2)
		go func() {
			defer workers.Done()
			guard, err := newOVGBuildGuard(context.Background(), defaultOVGBuildLimits())
			if err != nil {
				t.Errorf("create subset guard: %v", err)
				return
			}
			got, err := ovg.fixedOverlapsForBuild(g, subset, guard)
			if err != nil {
				t.Errorf("compute subset overlaps: %v", err)
				return
			}
			if len(got) != 0 {
				t.Errorf("subset unexpectedly had fixed overlaps: %v", got)
			}
		}()
		go func() {
			defer workers.Done()
			guard, err := newOVGBuildGuard(context.Background(), defaultOVGBuildLimits())
			if err != nil {
				t.Errorf("create full-set guard: %v", err)
				return
			}
			got, err := ovg.fixedOverlapsForBuild(g, full, guard)
			if err != nil {
				t.Errorf("compute full-set overlaps: %v", err)
				return
			}
			if len(got) != 1 {
				t.Errorf("full node set did not have one fixed overlap: %v", got)
			}
		}()
	}
	workers.Wait()
}

func TestBoundingBox(t *testing.T) {
	ovg := NewOVG(nil)
	guard := newOVGBuildGuardForTest(t.Context(), t)

	tl, br, err := guard.ovgBoundingBox(ovg)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, *geo.NewPoint(math.Inf(-1), math.Inf(-1)), *tl)
	assert.Equal(t, *geo.NewPoint(math.Inf(1), math.Inf(1)), *br)

	ovg.AddNode(NewOVGNode(geo.NewPoint(1, 45)))
	ovg.AddNode(NewOVGNode(geo.NewPoint(42, 23)))
	ovg.AddNode(NewOVGNode(geo.NewPoint(45, 0)))
	ovg.AddNode(NewOVGNode(geo.NewPoint(33, 8)))
	ovg.AddNode(NewOVGNode(geo.NewPoint(12, 7)))

	tl, br, err = guard.ovgBoundingBox(ovg)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, *geo.NewPoint(1, 0), *tl)
	assert.Equal(t, *geo.NewPoint(45, 45), *br)
}

func TestConnect(t *testing.T) {
	ovg := NewOVG(nil)

	n1 := ovg.AddNode(NewOVGNode(geo.NewPoint(1, 2)))
	n2 := ovg.AddNode(NewOVGNode(geo.NewPoint(5, 10)))
	e := ovg.Connect(n1, n2)

	assert.Equal(t, 1, len(ovg.Edges))
	assert.Equal(t, 1, len(n1.Edges))
	assert.Equal(t, 1, len(n2.Edges))
	assert.Equal(t, e, n1.Edges[0])
	assert.Equal(t, e, n2.Edges[0])
	assert.Equal(t, n1, e.From)
	assert.Equal(t, n2, e.To)
	assert.Equal(t, 0, len(ovg.VerticalEdges))
	assert.Equal(t, 0, len(ovg.HorizontalEdges))

	n1 = ovg.AddNode(NewOVGNode(geo.NewPoint(1, 2)))
	n2 = ovg.AddNode(NewOVGNode(geo.NewPoint(1, 10)))
	e = ovg.Connect(n1, n2)
	assert.Equal(t, 1, len(ovg.VerticalEdges))
	assert.Equal(t, e, ovg.VerticalEdges[1.][0])
	assert.Equal(t, 0, len(ovg.HorizontalEdges))

	n1 = ovg.AddNode(NewOVGNode(geo.NewPoint(1, 2)))
	n2 = ovg.AddNode(NewOVGNode(geo.NewPoint(10, 2)))
	e = ovg.Connect(n1, n2)
	assert.Equal(t, 1, len(ovg.VerticalEdges))
	assert.Equal(t, 1, len(ovg.HorizontalEdges))
	assert.Equal(t, e, ovg.HorizontalEdges[2.][0])
}

func TestRemoveIsolatedNodes(t *testing.T) {
	ovg := NewOVG(nil)

	n1 := ovg.AddNode(NewOVGNode(geo.NewPoint(1, 2)))
	n2 := ovg.AddNode(NewOVGNode(geo.NewPoint(5, 10)))
	ovg.AddNode(NewOVGNode(geo.NewPoint(7, 11)))

	ovg.Connect(n1, n2)
	ovg.removeIsolatedNodes(newBackgroundOVGBuildGuardForTest(t))

	assert.Equal(t, 2, len(ovg.Nodes))
	assert.Equal(t, n1, ovg.Nodes[0])
	assert.Equal(t, n2, ovg.Nodes[1])
}

func TestEquals(t *testing.T) {
	/* OVG
	   n1----n4----n5
	   |           |
	   n2          |
	   |           |
	   n3----------n6
	*/
	ovg1 := NewOVG(nil)
	n1 := ovg1.AddNode(NewOVGNode(geo.NewPoint(10, 10)))
	n2 := ovg1.AddNode(NewOVGNode(geo.NewPoint(10, 15)))
	n3 := ovg1.AddNode(NewOVGNode(geo.NewPoint(10, 20)))
	n4 := ovg1.AddNode(NewOVGNode(geo.NewPoint(15, 10)))
	n5 := ovg1.AddNode(NewOVGNode(geo.NewPoint(20, 10)))
	n6 := ovg1.AddNode(NewOVGNode(geo.NewPoint(20, 20)))

	ovg1.Connect(n1, n2)
	ovg1.Connect(n1, n4)
	ovg1.Connect(n2, n3)
	ovg1.Connect(n4, n5)
	ovg1.Connect(n5, n6)
	ovg1.Connect(n3, n6)

	ovg2 := NewOVG(nil)
	n1 = ovg2.AddNode(NewOVGNode(geo.NewPoint(10, 15)))
	n2 = ovg2.AddNode(NewOVGNode(geo.NewPoint(10, 10)))
	n3 = ovg2.AddNode(NewOVGNode(geo.NewPoint(10, 20)))
	n4 = ovg2.AddNode(NewOVGNode(geo.NewPoint(15, 10)))
	n5 = ovg2.AddNode(NewOVGNode(geo.NewPoint(20, 10)))
	assert.False(t, ovg1.Equals(ovg2))
	n6 = ovg2.AddNode(NewOVGNode(geo.NewPoint(20, 20)))

	// some edges have inverted From/To to ensure the connection direction is not important for equality
	ovg2.Connect(n2, n1)
	ovg2.Connect(n2, n4)
	ovg2.Connect(n1, n3)
	ovg2.Connect(n5, n4)
	ovg2.Connect(n5, n6)
	assert.False(t, ovg1.Equals(ovg2))
	ovg2.Connect(n6, n3)

	assert.True(t, ovg1.Equals(ovg2))
}

func TestSerialize(t *testing.T) {
	ovg1 := NewOVG(nil)
	n1 := ovg1.AddNode(NewOVGNode(geo.NewPoint(10, 10)))
	n2 := ovg1.AddNode(NewOVGNode(geo.NewPoint(10, 15)))
	n3 := ovg1.AddNode(NewOVGNode(geo.NewPoint(10, 20)))
	n4 := ovg1.AddNode(NewOVGNode(geo.NewPoint(15, 10)))
	n5 := ovg1.AddNode(NewOVGNode(geo.NewPoint(20, 10)))
	n6 := ovg1.AddNode(NewOVGNode(geo.NewPoint(20, 20)))

	ovg1.Connect(n1, n2)
	ovg1.Connect(n1, n4)
	ovg1.Connect(n2, n3)
	ovg1.Connect(n4, n5)
	ovg1.Connect(n5, n6)
	ovg1.Connect(n3, n6)

	serialized, err := ovg1.MarshalJSON()
	assert.Nil(t, err)

	ovg2 := NewOVG(nil)
	err = json.Unmarshal(serialized, ovg2)
	assert.NoError(t, err)

	assert.True(t, ovg1.Equals(ovg2))
}

func TestBuildOVGFromGraphEnsureThereAreEdgesToTraceRoute(t *testing.T) {
	/* The issue checked by this test is there must be a path between nodes 9 and 19
	┌──────────────────────────────────────────────────────────┐        ┌───────┐       ┌────────────────────────────┐
	│                                                          │        │       │       │                            │
	│                                                          │        │       │       │                            │
	│   ┌──────┐       ┌──────┐      ┌──────┐                  │        │       │       │                            │
	│   │      │       │      │      │      │                  │        │       │       │                            │
	│   │  18  │       │  12  │      │  15  │                  │        │       │       │                            │
	│   │      │       │      │      │      │                  │        │       │       │                            │
	│   └──────┘       └──────┘      └──────┘                  │        │       │       │                            │
	│                                                          │        │       │       │                            │
	│                                                          │        │       │       │                            │
	│   ┌──────┐       ┌──────┐      ┌──────┐                  │        │       │       │                            │
	│   │      │       │      │      │      │                  │        │       │       │                            │
	│   │ 16   │       │ 9    │      │  13  │                  │        │       │       │                            │
	│   │      │       │      │      │      │                  │        │       │       │                            │
	│   └──────┘       └──────┘      └──────┘                  │        │   20  │       │         19                 │
	│                                                          │        │       │       │                            │
	│                                                          │        │       │       │                            │
	│   ┌──────┐       ┌──────┐      ┌──────┐    ┌──────┐      │        │       │       │                            │
	│   │      │       │      │      │      │    │      │      │        │       │       │                            │
	│   │  10  │       │ 17   │      │  14  │    │ 11   │      │        │       │       │                            │
	│   │      │       │      │      │      │    │      │      │        │       │       │                            │
	│   └──────┘       └──────┘      └──────┘    └──────┘      │        │       │       │                            │
	│                                                          │        │       │       │                            │
	│                                                          │        │       │       │                            │
	└──────────────────────────────────────────────────────────┘        └───────┘       └────────────────────────────┘
	*/
	g := layoutgraph.NewGraph()

	c8 := g.AddNode(layoutgraph.NewNode(8, 602.0, 490.0))
	c8.TopLeft = geo.NewPoint(2091.0, 1622.0)
	c8.SetShape("Step")
	n9 := layoutgraph.NewNode(9, 57.0, 61.0)
	n9.TopLeft = geo.NewPoint(2311.0, 1840.0)
	n9.SetShape("Page")
	g.AddNewNodeToContainer(c8, n9)
	n10 := layoutgraph.NewNode(10, 57.0, 57.0)
	n10.TopLeft = geo.NewPoint(2166.0, 1978.0)
	n10.SetShape("Page")
	g.AddNewNodeToContainer(c8, n10)
	n11 := layoutgraph.NewNode(11, 51.0, 53.0)
	n11.TopLeft = geo.NewPoint(2567.0, 1978.0)
	n11.SetShape("Square")
	g.AddNewNodeToContainer(c8, n11)
	n12 := layoutgraph.NewNode(12, 59.0, 63.0)
	n12.TopLeft = geo.NewPoint(2311.0, 1697.0)
	n12.SetShape("Step")
	g.AddNewNodeToContainer(c8, n12)
	n13 := layoutgraph.NewNode(13, 51.0, 64.0)
	n13.TopLeft = geo.NewPoint(2448.0, 1840.0)
	n13.SetShape("Square")
	g.AddNewNodeToContainer(c8, n13)
	n14 := layoutgraph.NewNode(14, 50.0, 59.0)
	n14.TopLeft = geo.NewPoint(2437.0, 1978.0)
	n14.SetShape("Cloud")
	g.AddNewNodeToContainer(c8, n14)
	n15 := layoutgraph.NewNode(15, 57.0, 57.0)
	n15.TopLeft = geo.NewPoint(2450.0, 1697.0)
	n15.SetShape("Page")
	g.AddNewNodeToContainer(c8, n15)
	n16 := layoutgraph.NewNode(16, 61.0, 56.0)
	n16.TopLeft = geo.NewPoint(2166.0, 1842.0)
	n16.SetShape("StoredData")
	g.AddNewNodeToContainer(c8, n16)
	n17 := layoutgraph.NewNode(17, 54.0, 59.0)
	n17.TopLeft = geo.NewPoint(2303.0, 1978.0)
	n17.SetShape("Document")
	g.AddNewNodeToContainer(c8, n17)
	n18 := layoutgraph.NewNode(18, 65.0, 65.0)
	n18.TopLeft = geo.NewPoint(2166.0, 1697.0)
	n18.SetShape("Circle")
	g.AddNewNodeToContainer(c8, n18)

	n19 := g.AddNode(layoutgraph.NewNode(19, 602.0, 490.0))
	n19.TopLeft = geo.NewPoint(3000.0, 1622.0)
	n19.SetShape("Step")

	n20 := g.AddNode(layoutgraph.NewNode(20, 100, 490))
	n20.TopLeft = geo.NewPoint(2800.0, 1622.0)
	n20.SetShape("Step")

	edge := g.Connect(n9, n19)

	ovg, err := buildOVGFromGraph(context.Background(), g, nil)
	if err != nil {
		t.Fatal("error creating the OVG")
	}

	// the real issue is that the OVG is not properly built, but to test it,
	// we must check that the route exists, so using `router.search` seems like a good assertion
	router, _ := newOVGEdgeRouter(context.Background(), LongestToShortest, ovg, g, []*Route{}, g.Edges)
	ctx := context.Background()
	_, _, err = router.search(ctx, edge)
	if err != nil {
		t.Fatalf("expected to find route from node 9 to node 19, got error: %v", err)
	}
}

func TestAddCornerNodes(t *testing.T) {
	// Depending on the height of the containers, if they are not connected, it breaks when routing a -> c.
	// The reason is that we don't create OVG nodes for graph nodes that don't have edges.
	// This way, the OVG bounding box is Nodes([a, b, c]).BoundingBox(), which would make the edge a -> c
	// go through the ancestors of b, while it should go all the way on the outside as shown below
	// That's why we need to use the whole graph bounding box and not only the OVG bounding box.
	// Prior to this fix, the OVG was bounded as shown by `*`
	//                    ┌───────────────────────────────────────────────────────────────────────────────────────────────┐
	//                    │                                                                                               │
	// ┌──────────────────┼─────────────────┐          ┌────────────────────────────────────┐         ┌───────────────────┼────────────────┐
	// │  *               │                 │          │                                    │         │                   │             *  │
	// │    ┌─────────────┼─────────────┐   │          │    ┌───────────────────────────┐   │         │    ┌──────────────┼────────────┐   │
	// │    │             │             │   │          │    │                           │   │         │    │              │            │   │
	// │    │   ┌─────────┼──────────┐  │   │          │    │   ┌────────────────────┐  │   │         │    │   ┌──────────┼─────────┐  │   │
	// │    │   │         │          │  │   │          │    │   │                    │  │   │         │    │   │          │         │  │   │
	// │    │   │         │          │  │   │          │    │   │                    │  │   │         │    │   │          │         │  │   │
	// │    │   │     ┌───┴────┐     │  │   │          │    │   │     ┌────────┐     │  │   │         │    │   │     ┌────▼───┐     │  │   │
	// │    │   │     │        │     │  │   │          │    │   │     │        │     │  │   │         │    │   │     │        │     │  │   │
	// │    │   │     │   a    ├─────┼──┼───┼──────────┼────┼───┼─────►   b    ├─────┼──┼───┼─────────┼────┼───┼─────►   c    │     │  │   │
	// │    │   │     │        │     │  │   │          │    │   │     │        │     │  │   │         │    │   │     │        │     │  │   │
	// │    │   │     └────────┘     │  │   │          │    │   │     └────────┘     │  │   │         │    │   │     └────────┘     │  │   │
	// │    │   │                    │  │   │          │    │   │                    │  │   │         │    │   │                    │  │   │
	// │    │   │                    │  │   │          │    │   │                    │  │   │         │    │   │                    │  │   │
	// │    │   └────────────────────┘  │   │          │    │   └────────────────────┘  │   │         │    │   └────────────────────┘  │   │
	// │    │                           │   │          │    │                           │   │         │    │                           │   │
	// │    └───────────────────────────┘   │          │    └───────────────────────────┘   │         │    └───────────────────────────┘   │
	// │  *                                 │          │                                    │         │                                 *  │
	// └────────────────────────────────────┘          └────────────────────────────────────┘         └────────────────────────────────────┘
	g := layoutgraph.NewGraph()
	var leaves []*layoutgraph.Node
	for i := 0; i < 3; i++ {
		baseX := float64(i) * (450)
		baseY := 0.
		n1 := g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i*10), 400, 450))
		n1.TopLeft = geo.NewPoint(baseX, baseY)
		baseY += layoutgraph.ContainerPadding
		baseX += layoutgraph.ContainerPadding
		n2 := g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i*10+1), 300, 300))
		n2.TopLeft = geo.NewPoint(baseX, baseY)
		baseY += layoutgraph.ContainerPadding
		baseX += layoutgraph.ContainerPadding
		n3 := g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i*10+2), 200, 200))
		n3.TopLeft = geo.NewPoint(baseX, baseY)
		n4 := g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i*10+3), 70, 70))
		baseY += layoutgraph.ContainerPadding
		baseX += layoutgraph.ContainerPadding
		n4.TopLeft = geo.NewPoint(baseX, baseY)

		leaves = append(leaves, n4)

		g.AddNodeToContainer(nil, n1)
		g.AddNodeToContainer(n1, n2)
		g.AddNodeToContainer(n2, n3)
		g.AddNodeToContainer(n3, n4)
	}

	for i, leave := range leaves {
		for j := i + 1; j < len(leaves); j++ {
			g.Connect(leave, leaves[j])
		}
	}

	if len(g.Edges) != 3 {
		t.Fatalf("expected 3 edges, got=%d", len(g.Edges))
	}

	ac := g.Edges[1]
	if ac.From != leaves[0] || ac.To != leaves[2] {
		t.Fatal("wrong edge")
	}

	ovg, err := buildOVGFromGraph(context.Background(), g, nil)
	if err != nil {
		t.Fatal(err)
	}

	router, err := newOVGEdgeRouter(context.Background(), Default, ovg, g, nil, g.Edges)
	if err != nil {
		t.Fatal(err)
	}

	ctx := withTestLogger(context.Background(), t)
	route, d, err := router.search(ctx, ac)
	if err != nil {
		t.Fatal(err)
	}

	if d <= 0 {
		t.Fatalf("expected distance > 0, got=%f", d)
	}

	if len(route) == 0 {
		t.Fatal("expected a route")
	}
}

// OVG regression tests.

func TestOVGJSONRoundTripPreservesFractionalCoordinates(t *testing.T) {
	ovg := NewOVG(nil)
	// These points deliberately share their integer bucket. OVG identity must
	// use exact coordinates, not FormattedCoordinates' integer formatting.
	from := ovg.AddNode(NewOVGNode(geo.NewPoint(0.1, 0)))
	to := ovg.AddNode(NewOVGNode(geo.NewPoint(0.9, 0)))
	ovg.Connect(from, to)

	data, err := json.Marshal(ovg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded OVG
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Nodes) != 2 || len(decoded.Edges) != 1 {
		t.Fatalf("unexpected OVG round trip: %d nodes, %d edges", len(decoded.Nodes), len(decoded.Edges))
	}
	if decoded.OccupiedPoints[*geo.NewPoint(0.1, 0)] == nil || decoded.OccupiedPoints[*geo.NewPoint(0.9, 0)] == nil {
		t.Fatalf("fractional coordinates were not preserved: %s", data)
	}
	if decoded.Edges[0].From == decoded.Edges[0].To {
		t.Fatalf("fractional endpoints collapsed into a self-edge: %s", data)
	}
	if !ovg.Equals(&decoded) {
		t.Fatalf("fractional OVG changed during round trip: %s", data)
	}
}
