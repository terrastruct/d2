package layoutgraph

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func (node *Node) allReachableNodes(includeContainers, includeNears, traverseTrees bool, ignore map[*Node]struct{}) []*Node {
	return node.reachableNodes(func(_ *Node) bool { return true }, includeContainers, includeNears, traverseTrees, ignore)
}

func (node *Node) reachableNodes(shouldVisit func(*Node) bool, includeContainers, includeNears, traverseTrees bool, ignore map[*Node]struct{}) []*Node {
	guard := unboundedWork
	reachableNodes, err := node.reachableNodesGuarded(
		shouldVisit,
		includeContainers,
		includeNears,
		traverseTrees,
		ignore,
		guard,
	)
	if err != nil {
		panic(err)
	}
	return reachableNodes
}

func TestSortNodesByIDPreservesShortSlices(t *testing.T) {
	singletonNil := make([]*Node, 1, 2)
	singletonNode := append(make([]*Node, 0, 2), NewNode(1, 10, 10))
	tests := []struct {
		name  string
		nodes []*Node
	}{
		{name: "nil"},
		{name: "non-nil empty", nodes: make([]*Node, 0, 4)},
		{name: "singleton nil", nodes: singletonNil},
		{name: "singleton node", nodes: singletonNode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wasNil := test.nodes == nil
			beforeLen, beforeCap := len(test.nodes), cap(test.nodes)
			var beforeNode *Node
			if len(test.nodes) == 1 {
				beforeNode = test.nodes[0]
			}
			var tailSentinel *Node
			if cap(test.nodes) > len(test.nodes) {
				tailSentinel = NewNode(99, 10, 10)
				test.nodes[:cap(test.nodes)][cap(test.nodes)-1] = tailSentinel
			}

			sortNodesByID(test.nodes)

			if (test.nodes == nil) != wasNil || len(test.nodes) != beforeLen || cap(test.nodes) != beforeCap {
				t.Fatal("short node slice header changed")
			}
			if len(test.nodes) == 1 && test.nodes[0] != beforeNode {
				t.Fatal("singleton node changed")
			}
			if tailSentinel != nil && test.nodes[:cap(test.nodes)][cap(test.nodes)-1] != tailSentinel {
				t.Fatal("short node slice backing array changed")
			}
		})
	}

	higherID := NewNode(2, 10, 10)
	nodes := []*Node{higherID, nil}
	sortNodesByID(nodes)
	if nodes[0] != nil || nodes[1] != higherID {
		t.Fatal("two-node ordering changed")
	}
}

func nodeByIDForTest(graph *Graph, id EntityID) *Node {
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	return nil
}

func TestReachableNodes(t *testing.T) {
	graph := NewGraph()

	graph.AddNode(NewNode(1, 5, 5))
	graph.AddNode(NewNode(2, 2, 5))
	graph.connectByID(1, 2)

	reachableNodes := nodeByIDForTest(graph, 1).allReachableNodes(false, true, false, nil)
	if len(reachableNodes) != 2 {
		t.Fatal("Expected 2 reachable nodes")
	}

	graph.AddNode(NewNode(3, 5, 5))

	reachableNodes = nodeByIDForTest(graph, 3).allReachableNodes(false, true, false, nil)
	if len(reachableNodes) != 1 {
		t.Fatal("Expected 1 reachable nodes")
	}

	graph.AddNode(NewNode(4, 5, 5))
	graph.AddNode(NewNode(5, 5, 5))
	graph.AddNode(NewNode(6, 5, 5))
	graph.connectByID(4, 5)
	graph.connectByID(5, 6)
	graph.connectByID(4, 6)

	reachableNodes = nodeByIDForTest(graph, 4).allReachableNodes(false, true, false, nil)
	if len(reachableNodes) != 3 {
		t.Fatal("Expected 3 reachable nodes")
	}
}

func TestMoveNodeWithChildrenSupportsFractionalOffsets(t *testing.T) {
	t.Setenv("DEV_MODE", "on")
	g := NewGraph()
	parent := NewNode(1, 20, 20)
	parent.TopLeft = geo.NewPoint(10, 10)
	child := NewNode(2, 10, 10)
	child.TopLeft = geo.NewPoint(15, 15)
	g.AddNewNodeToContainer(nil, parent)
	g.AddNewNodeToContainer(parent, child)

	parent.moveNodeWithChildren(0.5, 1.25)
	if !parent.TopLeft.Equals(geo.NewPoint(10.5, 11.25)) {
		t.Fatalf("parent position = %v", parent.TopLeft)
	}
	if !child.TopLeft.Equals(geo.NewPoint(15.5, 16.25)) {
		t.Fatalf("child position = %v", child.TopLeft)
	}
}

func TestReachableNodesWithContainers(t *testing.T) {
	graph := NewGraph()
	a := NewNode(1, 5, 5)
	b := NewNode(2, 5, 5)
	c := NewNode(3, 5, 5)
	d := NewNode(4, 5, 5)

	graph.AddNode(a)
	graph.AddNode(b)
	graph.AddNode(c)
	graph.AddNode(d)

	graph.connectByID(1, 2)
	graph.connectByID(3, 4)

	graph.Containers[b] = []*Node{c}

	// Random isolated guys
	e := NewNode(5, 5, 5)
	f := NewNode(6, 2, 5)
	graph.AddNode(e)
	graph.AddNode(f)
	graph.connectByID(5, 6)

	graph.Containers[nil] = []*Node{a, b, d, e, f}

	reachableNodes := a.allReachableNodes(false, true, false, nil)
	if len(reachableNodes) != 2 {
		t.Fatalf("Expected 2 reachable nodes, got %v", len(reachableNodes))
	}
	reachableNodes = b.allReachableNodes(false, true, false, nil)
	if len(reachableNodes) != 2 {
		t.Fatal("Expected 2 reachable nodes")
	}
	reachableNodes = c.allReachableNodes(false, true, false, nil)
	if len(reachableNodes) != 2 {
		t.Fatal("Expected 2 reachable nodes")
	}
	reachableNodes = d.allReachableNodes(false, true, false, nil)
	if len(reachableNodes) != 2 {
		t.Fatal("Expected 2 reachable nodes")
	}
}

func TestReachableNodesNears(t *testing.T) {
	g := NewGraph()
	n1 := g.AddNode(NewNode(1, 10, 10))
	n2 := g.AddNode(NewNode(2, 10, 10))
	n1.AddNear(n2)

	reachable := n1.allReachableNodes(true, true, false, nil)
	assert.Equal(t, 2, len(reachable))
	assert.Equal(t, n1, reachable[0])
	assert.Equal(t, n2, reachable[1])

	reachable = n1.allReachableNodes(true, false, false, nil)
	assert.Equal(t, 1, len(reachable))
	assert.Equal(t, n1, reachable[0])
}

func TestAddNearIsSymmetricAndInitializesZeroValueNearMaps(t *testing.T) {
	g := NewGraph()
	a := NewNode(1, 10, 10)
	b := NewNode(2, 10, 10)
	c := NewNode(3, 10, 10)
	for _, n := range []*Node{a, b, c} {
		g.AddNewNodeToContainer(nil, n)
	}

	a.AddNear(b)
	b.AddNear(c)
	c.AddNear(a)
	a.AddNear(b) // Idempotent.
	a.AddNear(a) // Self-nears are ignored.
	a.AddNear(nil)
	var nilNode *Node
	nilNode.AddNear(a)
	zeroA := &Node{}
	zeroB := &Node{}
	zeroA.AddNear(zeroB)
	if _, ok := zeroA.Nears[zeroB]; !ok {
		t.Fatal("zero-value node is missing near")
	}
	if _, ok := zeroB.Nears[zeroA]; !ok {
		t.Fatal("zero-value node is missing reverse near")
	}

	for _, pair := range [][2]*Node{{a, b}, {b, c}, {c, a}} {
		if _, ok := pair[0].Nears[pair[1]]; !ok {
			t.Fatalf("%v is missing near %v", pair[0].ID, pair[1].ID)
		}
		if _, ok := pair[1].Nears[pair[0]]; !ok {
			t.Fatalf("%v is missing reverse near %v", pair[1].ID, pair[0].ID)
		}
	}
	if _, ok := a.Nears[a]; ok {
		t.Fatal("self-near was added")
	}

}

func TestReachableNodesNearsDeterminism(t *testing.T) {
	g := NewGraph()
	n1 := g.AddNode(NewNode(1, 10, 10))
	n2 := g.AddNode(NewNode(2, 10, 10))
	n3 := g.AddNode(NewNode(3, 10, 10))
	n4 := g.AddNode(NewNode(4, 10, 10))
	n1.AddNear(n2)
	n1.AddNear(n3)
	n1.AddNear(n4)

	for j := 0; j < 10; j++ {
		reachable := n1.allReachableNodes(true, true, false, nil)
		assert.Equal(t, 4, len(reachable))
		for i, n := range []*Node{n1, n2, n3, n4} {
			assert.Equalf(t, n.ID, reachable[i].ID, "failed on iteration %v", j)
		}
	}
}

func TestBoxesOverlapWithPadding(t *testing.T) {
	box := func(x, y, width, height float64) geo.Box {
		return geo.Box{TopLeft: geo.NewPoint(x, y), Width: width, Height: height}
	}

	tests := []struct {
		name    string
		b1      geo.Box
		b2      geo.Box
		padding float64
		want    bool
	}{
		{
			name: "partial overlap",
			b1:   box(0, 0, 10, 10),
			b2:   box(5, 5, 10, 10),
			want: true,
		},
		{
			name: "containment",
			b1:   box(0, 0, 10, 10),
			b2:   box(2, 2, 3, 3),
			want: true,
		},
		{
			name: "edge contact is not overlap",
			b1:   box(0, 0, 10, 10),
			b2:   box(10, 2, 4, 4),
		},
		{
			name: "corner contact is not overlap",
			b1:   box(0, 0, 10, 10),
			b2:   box(10, 10, 4, 4),
		},
		{
			name:    "padding bridges a gap",
			b1:      box(0, 0, 10, 10),
			b2:      box(15, 3, 4, 4),
			padding: 6,
			want:    true,
		},
		{
			name:    "padding boundary remains open",
			b1:      box(0, 0, 10, 10),
			b2:      box(15, 3, 4, 4),
			padding: 5,
		},
		{
			name: "separated on one axis",
			b1:   box(-10, -10, 5, 5),
			b2:   box(-4, -20, 5, 5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boxesOverlapWithPadding(tt.b1, tt.b2, tt.padding); got != tt.want {
				t.Fatalf("boxesOverlapWithPadding() = %v, want %v", got, tt.want)
			}
			if got := boxesOverlapWithPadding(tt.b2, tt.b1, tt.padding); got != tt.want {
				t.Fatalf("boxesOverlapWithPadding() with reversed arguments = %v, want %v", got, tt.want)
			}
			if tt.padding == 0 {
				if got := tt.b1.Overlaps(tt.b2); got != tt.want {
					t.Fatalf("Box.Overlaps() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func (node *Node) distanceToPoint(point *geo.Point, includeSizes bool) float64 {
	pointBox := geo.Box{TopLeft: point}
	return distanceBetweenBoxes(nodeDistanceBox(node, includeSizes), pointBox)
}

func TestNodeDistance(t *testing.T) {
	node := func(id EntityID, x, y, width, height float64) *Node {
		n := NewNode(id, width, height)
		n.TopLeft = geo.NewPoint(x, y)
		return n
	}

	a := node(1, 0, 0, 10, 10)
	tests := []struct {
		name string
		b    *Node
		want float64
	}{
		{name: "overlap", b: node(2, 5, 5, 10, 10), want: 0},
		{name: "edge contact", b: node(2, 10, 3, 4, 4), want: 0},
		{name: "horizontal gap", b: node(2, 15, 2, 4, 4), want: 5},
		{name: "vertical gap", b: node(2, 2, 16, 4, 4), want: 6},
		{name: "diagonal gap", b: node(2, 13, 14, 4, 4), want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := a.distanceTo(tt.b, true); math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("distanceTo() = %v, want %v", got, tt.want)
			}
			if got := tt.b.distanceTo(a, true); math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("distanceTo() with reversed arguments = %v, want %v", got, tt.want)
			}
		})
	}

	b := node(2, 3, 4, 100, 100)
	if got := a.distanceTo(b, false); got != 5 {
		t.Fatalf("distanceTo() without sizes = %v, want 5", got)
	}
	if got := a.distanceToPoint(geo.NewPoint(13, 14), true); got != 5 {
		t.Fatalf("distanceToPoint() with size = %v, want 5", got)
	}
	if got := a.distanceToPoint(geo.NewPoint(3, 4), false); got != 5 {
		t.Fatalf("distanceToPoint() without size = %v, want 5", got)
	}
}

func TestOrientation(t *testing.T) {
	p := geo.NewPoint(0, 0)
	q := geo.NewPoint(2, 0)

	if got := orientation(p, q, geo.NewPoint(1, -1)); got <= 0 {
		t.Fatalf("clockwise orientation = %v, want positive", got)
	}
	if got := orientation(p, q, geo.NewPoint(1, 1)); got >= 0 {
		t.Fatalf("counter-clockwise orientation = %v, want negative", got)
	}
	if got := orientation(p, q, geo.NewPoint(5, 0)); got != 0 {
		t.Fatalf("collinear orientation = %v, want 0", got)
	}
}

func TestIntersects(t *testing.T) {
	tests := []struct {
		name       string
		p1, q1     *geo.Point
		p2, q2     *geo.Point
		intersects bool
	}{
		{
			name: "parallel",
			p1:   geo.NewPoint(0, 0), q1: geo.NewPoint(0, 100),
			p2: geo.NewPoint(40, 0), q2: geo.NewPoint(40, 100),
		},
		{
			name: "collinear disjoint",
			p1:   geo.NewPoint(0, 0), q1: geo.NewPoint(0, 100),
			p2: geo.NewPoint(0, 110), q2: geo.NewPoint(0, 140),
		},
		{
			name: "collinear overlap",
			p1:   geo.NewPoint(0, 0), q1: geo.NewPoint(0, 100),
			p2: geo.NewPoint(0, 90), q2: geo.NewPoint(0, 140), intersects: true,
		},
		{
			name: "diagonal collinear overlap",
			p1:   geo.NewPoint(0, 0), q1: geo.NewPoint(10, 10),
			p2: geo.NewPoint(5, 5), q2: geo.NewPoint(15, 15), intersects: true,
		},
		{
			name: "proper crossing",
			p1:   geo.NewPoint(0, 0), q1: geo.NewPoint(50, 50),
			p2: geo.NewPoint(30, 0), q2: geo.NewPoint(0, 40), intersects: true,
		},
		{
			name: "shared endpoint",
			p1:   geo.NewPoint(0, 0), q1: geo.NewPoint(50, 50),
			p2: geo.NewPoint(50, 50), q2: geo.NewPoint(70, 70), intersects: true,
		},
		{
			name: "point on segment",
			p1:   geo.NewPoint(5, 5), q1: geo.NewPoint(5, 5),
			p2: geo.NewPoint(0, 0), q2: geo.NewPoint(10, 10), intersects: true,
		},
		{
			name: "distinct points",
			p1:   geo.NewPoint(5, 5), q1: geo.NewPoint(5, 5),
			p2: geo.NewPoint(6, 6), q2: geo.NewPoint(6, 6),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := intersects(tt.p1, tt.q1, tt.p2, tt.q2); got != tt.intersects {
				t.Fatalf("intersects() = %v, want %v", got, tt.intersects)
			}
			if got := intersects(tt.p2, tt.q2, tt.p1, tt.q1); got != tt.intersects {
				t.Fatalf("intersects() with reversed segments = %v, want %v", got, tt.intersects)
			}
		})
	}
}

func TestSegmentIntersectsBoxIsSymmetric(t *testing.T) {
	tests := []struct {
		name   string
		p1, p2 *geo.Point
		box    *geo.Box
		want   bool
	}{
		{
			name: "oblique crossing that was direction dependent",
			p1:   geo.NewPoint(-20, -20), p2: geo.NewPoint(0.5, 10.5),
			box:  geo.NewBox(geo.NewPoint(0, 0), 10, 10),
			want: true,
		},
		{
			name: "outside",
			p1:   geo.NewPoint(-20, -20), p2: geo.NewPoint(-1, -1),
			box: geo.NewBox(geo.NewPoint(0, 0), 10, 10),
		},
		{
			name: "single corner tangency is not a crossover",
			p1:   geo.NewPoint(-10, 10), p2: geo.NewPoint(10, -10),
			box: geo.NewBox(geo.NewPoint(0, 0), 10, 10),
		},
		{
			name: "endpoint on boundary",
			p1:   geo.NewPoint(-10, -10), p2: geo.NewPoint(0, 0),
			box:  geo.NewBox(geo.NewPoint(0, 0), 10, 10),
			want: true,
		},
		{
			name: "collinear boundary overlap",
			p1:   geo.NewPoint(-10, 0), p2: geo.NewPoint(20, 0),
			box:  geo.NewBox(geo.NewPoint(0, 0), 10, 10),
			want: true,
		},
		{
			name: "negative box dimensions",
			p1:   geo.NewPoint(-1, 5), p2: geo.NewPoint(11, 5),
			box:  geo.NewBox(geo.NewPoint(10, 10), -10, -10),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := segmentIntersectsBox(tt.p1, tt.p2, tt.box); got != tt.want {
				t.Fatalf("segmentIntersectsBox() = %v, want %v", got, tt.want)
			}
			if got := segmentIntersectsBox(tt.p2, tt.p1, tt.box); got != tt.want {
				t.Fatalf("segmentIntersectsBox() with endpoints reversed = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPassesThrough(t *testing.T) {
	a := NewNode(1, 5, 5)
	a.TopLeft = geo.NewPoint(10, 10)

	// Test passing through from inside a node to outside
	pointX := geo.NewPoint(12, 12)
	pointY := geo.NewPoint(19, 19)

	if !a.passesThrough(pointX, pointY) {
		t.Fatal("Should pass through")
	}

	// Test passing through from outside a node to other side
	pointX = geo.NewPoint(9, 9)
	pointY = geo.NewPoint(19, 19)

	if !a.passesThrough(pointX, pointY) {
		t.Fatal("Should pass through")
	}
}

// TODO: restore symmetry coverage for differently sized and mirrored nodes
// using the current geometry APIs.

func TestIsMajorityTarget(t *testing.T) {
	g := NewGraph()
	a := NewNode(1, 10, 10)
	b := NewNode(2, 10, 10)

	g.AddNode(a)
	g.AddNode(b)

	g.Connect(a, b)
	g.Edges[0].TargetArrowhead = TriangleArrowhead

	if a.isMajorityTarget() {
		t.Fatal("Node a should not be considered a target node")
	}

	if !b.isMajorityTarget() {
		t.Fatal("Node b should be considered a target node")
	}

	c := NewNode(3, 10, 10)
	g.AddNode(c)
	g.Connect(b, c)

	if !b.isMajorityTarget() {
		// a -> b - c
		t.Fatal("Node b should be a target node")
	}

	if c.isMajorityTarget() {
		t.Fatal("Node c should not be considered a target node")
	}

	d := NewNode(4, 10, 10)
	e := NewNode(5, 10, 10)
	g.AddNode(d)
	g.AddNode(e)
	g.Connect(d, c)
	g.Edges[2].SourceArrowhead = TriangleArrowhead
	g.Edges[2].TargetArrowhead = TriangleArrowhead
	g.Connect(e, c)
	g.Edges[3].TargetArrowhead = TriangleArrowhead

	if !c.isMajorityTarget() {
		// c is a target of d and e, though with d it's bidirectional
		t.Fatal("Node c should be considered a target node")
	}

	if d.isMajorityTarget() {
		// we ignore bidirectional edges, as it doesn't matter what is the target/source node
		t.Fatal("Node d should not be considered a target node")
	}
}

// . ┌─────┐    ┌──────────────────────────────┐
// . │     │    │                              │
// . │ a   │    │             b                │
// . └─────┘    │                              │
// .            │     ┌───────────────────┐    │
// .            │     │     vessel        │    │
// .            │     │   ┌───┐    ┌───┐  │    │
// .            │     │   │c  │    │d  │  │    │
// .            │     │   └───┘    └───┘  │    │
// .            │     │                   │    │
// .            │     └───────────────────┘    │
// .            │                              │
// .            │                              │
// .            └──────────────────────────────┘
func TestMoveNodeAbs(t *testing.T) {
	g := NewGraph()

	a := NewNode(1, 10, 10)
	a.TopLeft = geo.NewPoint(0, 0)
	g.AddNode(a)

	b := NewNode(2, 1000, 1000)
	b.TopLeft = geo.NewPoint(20, 0)
	g.AddNode(b)

	c := NewNode(3, 10, 10)
	c.TopLeft = geo.NewPoint(200, 200)

	d := NewNode(4, 10, 10)
	d.TopLeft = geo.NewPoint(220, 200)

	cluster := &Cluster{
		Nodes:       []*Node{c, d},
		Container:   b,
		Graph:       g,
		FixedSize:   false,
		Arrangement: Row,
	}
	vessel := createClusterVesselFixture(cluster, 5)
	g.AddNode(vessel)

	g.Containers = map[*Node][]*Node{
		nil: {a, b},
		b:   {vessel},
	}
	g.Clusters = map[*Node]*Cluster{vessel: cluster}

	positionsBefore := map[*Node]geo.Point{
		a:      *a.TopLeft.Copy(),
		b:      *b.TopLeft.Copy(),
		c:      *c.TopLeft.Copy(),
		d:      *d.TopLeft.Copy(),
		vessel: *vessel.TopLeft.Copy(),
	}

	checkPositions := func() {
		assert.Equal(t, positionsBefore[a], *a.TopLeft)
		assert.Equal(t, positionsBefore[b], *b.TopLeft)
		assert.Equal(t, positionsBefore[c], *c.TopLeft)
		assert.Equal(t, positionsBefore[d], *d.TopLeft)
		assert.Equal(t, positionsBefore[vessel], *vessel.TopLeft)
	}

	b.moveNodeAbsWithChildren(1000, 1000)
	b.moveNodeAbsWithChildren(20, 0)

	checkPositions()

	b.moveNodeWithChildren(1000, 1000)
	b.moveNodeWithChildren(-1000, -1000)

	checkPositions()
}

// ┌─────────────────────────────────────────────────────────────────────────────────┐
// │                                                                                 │
// │                                       a                                         │
// │                                                                                 │
// │ ┌──────────────────────────────────────────────────────────────────────────┐    │
// │ │                                 vessel                                   │    │
// │ │  ┌───────────────────────────┐                                           │    │
// │ │  │                b          │       ┌──────────────────────────────┐    │    │
// │ │  │                           │       │               c              │    │    │
// │ │  │     ┌─────┐               │       │                              │    │    │
// │ │  │     │ d   │               │       │     ┌───────────────────┐    │    │    │
// │ │  │     │     │               │       │     │      vessel       │    │    │    │
// │ │  │     └─────┘               │       │     │                   │    │    │    │
// │ │  │                           │       │     │   ┌──────┐ ┌────┐ │    │    │    │
// │ │  │                           │       │     │   │      │ │    │ │    │    │    │
// │ │  │                           │       │     │   │   f  │ │ g  │ │    │    │    │
// │ │  │      ┌──────┐             │       │     │   │      │ │    │ │    │    │    │
// │ │  │      │      │             │       │     │   └──────┘ └────┘ │    │    │    │
// │ │  │      │  e   │             │       │     │                   │    │    │    │
// │ │  │      │      │             │       │     └───────────────────┘    │    │    │
// │ │  │      └──────┘             │       │                              │    │    │
// │ │  │                           │       │                              │    │    │
// │ │  │                           │       └──────────────────────────────┘    │    │
// │ │  └───────────────────────────┘                                           │    │
// │ │                                                                          │    │
// │ └──────────────────────────────────────────────────────────────────────────┘    │
// │                                                                                 │
// │                                                                                 │
// └─────────────────────────────────────────────────────────────────────────────────┘
func TestMoveNodeAbsSuperNested(t *testing.T) {
	graph := NewGraph()

	a := NewNode(1, 1000, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	graph.AddNode(a)

	b := NewNode(2, 100, 300)
	b.TopLeft = geo.NewPoint(a.TopLeft.X+200, a.TopLeft.Y+200)

	c := NewNode(3, b.Width, b.Height)
	c.TopLeft = geo.NewPoint(b.TopLeft.X+b.Width+200, b.TopLeft.Y)

	bcCluster := &Cluster{
		Nodes:       []*Node{b, c},
		Container:   a,
		Graph:       graph,
		FixedSize:   false,
		Arrangement: Row,
	}
	bcVessel := createClusterVesselFixture(bcCluster, 50)
	graph.AddNode(bcVessel)

	d := NewNode(4, 10, 10)
	d.TopLeft = geo.NewPoint(b.TopLeft.X+20, b.TopLeft.Y+20)
	graph.AddNode(d)

	e := NewNode(5, 10, 10)
	e.TopLeft = geo.NewPoint(b.TopLeft.X+30, b.TopLeft.Y+50)
	graph.AddNode(e)

	f := NewNode(6, 10, 10)
	f.TopLeft = geo.NewPoint(c.TopLeft.X+30, c.TopLeft.Y+50)

	g := NewNode(7, 10, 10)
	g.TopLeft = geo.NewPoint(f.TopLeft.X+f.Width+30, f.TopLeft.Y)

	fgCluster := &Cluster{
		Nodes:       []*Node{f, g},
		Container:   c,
		Graph:       graph,
		FixedSize:   false,
		Arrangement: Row,
	}
	fgVessel := createClusterVesselFixture(fgCluster, 51)
	graph.AddNode(fgVessel)

	graph.Containers = map[*Node][]*Node{
		nil: {a},
		a:   {bcVessel},
		b:   {d, e},
		c:   {fgVessel},
	}
	graph.Clusters = map[*Node]*Cluster{
		bcVessel: bcCluster,
		fgVessel: fgCluster,
	}

	positionsBefore := map[*Node]geo.Point{
		a:        *a.TopLeft.Copy(),
		b:        *b.TopLeft.Copy(),
		c:        *c.TopLeft.Copy(),
		d:        *d.TopLeft.Copy(),
		e:        *e.TopLeft.Copy(),
		f:        *f.TopLeft.Copy(),
		g:        *g.TopLeft.Copy(),
		bcVessel: *bcVessel.TopLeft.Copy(),
		fgVessel: *fgVessel.TopLeft.Copy(),
	}

	checkPositions := func() {
		assert.Equal(t, positionsBefore[a], *a.TopLeft)
		assert.Equal(t, positionsBefore[b], *b.TopLeft)
		assert.Equal(t, positionsBefore[c], *c.TopLeft)
		assert.Equal(t, positionsBefore[d], *d.TopLeft)
		assert.Equal(t, positionsBefore[e], *e.TopLeft)
		assert.Equal(t, positionsBefore[f], *f.TopLeft)
		assert.Equal(t, positionsBefore[g], *g.TopLeft)
		assert.Equal(t, positionsBefore[bcVessel], *bcVessel.TopLeft)
		assert.Equal(t, positionsBefore[fgVessel], *fgVessel.TopLeft)
	}

	a.moveNodeAbsWithChildren(1000, 1000)
	a.moveNodeAbsWithChildren(0, 0)

	checkPositions()

	a.moveNodeWithChildren(1000, 1000)
	a.moveNodeWithChildren(-1000, -1000)

	checkPositions()
}

func TestSetNumColumns(t *testing.T) {
	n := NewNode(0, 100, 100)

	// Non-table shapes ignore table-column metadata.
	n.SetNumColumns(5)
	if n.NumColumns() != 0 {
		t.Fatalf("square columns=%d, want 0", n.NumColumns())
	}

	n.SetShape(shape.TABLE_TYPE)
	n.SetNumColumns(5)
	if n.NumColumns() != 5 {
		t.Fatal("table columns=0, want 5")
	}
}

func TestSetShapeUnknownPreservesShape(t *testing.T) {
	n := NewNode(0, 100, 100)
	n.SetShape(shape.STEP_TYPE)
	beforeShape := n.Shape
	beforeType := n.shapeType

	n.SetShape("unsupported")

	if n.Shape != beforeShape {
		t.Fatal("unsupported shape replaced the existing shape")
	}
	if n.shapeType != beforeType {
		t.Fatal("unsupported shape replaced the existing shape kind")
	}
}

func TestMirrorWithLoops(t *testing.T) {
	// when mirroring with loop offsets, we need to consider them to get the right mirrored positions
	// otherwise, the node bounding box changes and it can result in overlaps or container escape

	t.Run("TopLeftLoop", func(t *testing.T) {
		n1 := NewNode(1, 50, 50)
		n1.TopLeft = geo.NewPoint(10, 10)

		// size (40, 40) + (10, 10) of loop offsets on the left and above, so the bbox has 50 x 50
		n2 := NewNode(1, 40, 40)
		n2.TopLeft = geo.NewPoint(20, 20)
		n2.LoopOffsets = make(map[geo.Orientation]float64)
		n2.LoopOffsets[geo.Top] = 10
		n2.LoopOffsets[geo.Left] = 10

		// both n1 and n2 have the same bounding box
		n1TL, n1BR := n1.bounds(nil)
		n2TL, n2BR := n2.bounds(nil)
		if !n1TL.Equals(n2TL) {
			t.Fatalf("expected bounding boxes to be equal, n1tl=%v, n2tl=%v", n1TL, n2TL)
		}
		if !n1BR.Equals(n2BR) {
			t.Fatalf("expected bounding boxes to be equal, n1br=%v, n2br=%v", n1BR, n2BR)
		}

		// after mirroring, they must have the same bounding box
		n1.mirror(true, true)
		n1MirroredTL, n1MirroredBR := n1.bounds(nil)
		n2.mirror(true, true)
		n2MirroredTL, n2MirroredBR := n2.bounds(nil)
		if !n1MirroredTL.Equals(n2MirroredTL) {
			t.Fatalf("expected mirrored bounding boxes to be equal, n1tl=%v, n2tl=%v", n1MirroredTL, n2MirroredTL)
		}
		if !n1MirroredBR.Equals(n2MirroredBR) {
			t.Fatalf("expected mirrored bounding boxes to be equal, n1br=%v, n2br=%v", n1MirroredBR, n2MirroredBR)
		}
	})

	t.Run("BottomRightLoop", func(t *testing.T) {
		n1 := NewNode(1, 50, 50)
		n1.TopLeft = geo.NewPoint(10, 10)

		// size (40, 40) + (10, 10) of loop offsets on the right and below, so the bbox has 50 x 50
		n2 := NewNode(1, 40, 40)
		n2.TopLeft = geo.NewPoint(10, 10)
		n2.LoopOffsets = make(map[geo.Orientation]float64)
		n2.LoopOffsets[geo.Bottom] = 10
		n2.LoopOffsets[geo.Right] = 10

		// both n1 and n2 have the same bounding box
		n1TL, n1BR := n1.bounds(nil)
		n2TL, n2BR := n2.bounds(nil)
		if !n1TL.Equals(n2TL) {
			t.Fatalf("expected bounding boxes to be equal, n1tl=%v, n2tl=%v", n1TL, n2TL)
		}
		if !n1BR.Equals(n2BR) {
			t.Fatalf("expected bounding boxes to be equal, n1br=%v, n2br=%v", n1BR, n2BR)
		}

		// after mirroring, they must have the same bounding box
		n1.mirror(true, true)
		n1MirroredTL, n1MirroredBR := n1.bounds(nil)
		n2.mirror(true, true)
		n2MirroredTL, n2MirroredBR := n2.bounds(nil)
		if !n1MirroredTL.Equals(n2MirroredTL) {
			t.Fatalf("expected mirrored bounding boxes to be equal, n1tl=%v, n2tl=%v", n1MirroredTL, n2MirroredTL)
		}
		if !n1MirroredBR.Equals(n2MirroredBR) {
			t.Fatalf("expected mirrored bounding boxes to be equal, n1br=%v, n2br=%v", n1MirroredBR, n2MirroredBR)
		}
	})

	t.Run("AsymetricalLoops", func(t *testing.T) {
		// sanity checking for loops on all sides, without symmetry
		n1 := NewNode(1, 50, 50)
		n1.TopLeft = geo.NewPoint(10, 10)

		// size (25, 25) + (25, 25) of loop offsets, so the bbox has 50 x 50
		n2 := NewNode(1, 25, 25)
		n2.TopLeft = geo.NewPoint(20, 25)
		n2.LoopOffsets = make(map[geo.Orientation]float64)
		n2.LoopOffsets[geo.Top] = 15
		n2.LoopOffsets[geo.Bottom] = 10
		n2.LoopOffsets[geo.Right] = 15
		n2.LoopOffsets[geo.Left] = 10

		// both n1 and n2 have the same bounding box
		n1TL, n1BR := n1.bounds(nil)
		n2TL, n2BR := n2.bounds(nil)
		if !n1TL.Equals(n2TL) {
			t.Fatalf("expected bounding boxes to be equal, n1tl=%v, n2tl=%v", n1TL, n2TL)
		}
		if !n1BR.Equals(n2BR) {
			t.Fatalf("expected bounding boxes to be equal, n1br=%v, n2br=%v", n1BR, n2BR)
		}

		// after mirroring, they must have the same bounding box
		n1.mirror(true, true)
		n1MirroredTL, n1MirroredBR := n1.bounds(nil)
		n2.mirror(true, true)
		n2MirroredTL, n2MirroredBR := n2.bounds(nil)
		if !n1MirroredTL.Equals(n2MirroredTL) {
			t.Fatalf("expected mirrored bounding boxes to be equal, n1tl=%v, n2tl=%v", n1MirroredTL, n2MirroredTL)
		}
		if !n1MirroredBR.Equals(n2MirroredBR) {
			t.Fatalf("expected mirrored bounding boxes to be equal, n1br=%v, n2br=%v", n1MirroredBR, n2MirroredBR)
		}
	})
}

func TestHasLeakyEdge(t *testing.T) {
	graph := NewGraph()

	n1 := NewNode(1, 1000, 1000)
	n2 := NewNode(2, 100, 100)
	n3 := NewNode(3, 10, 10)
	n4 := NewNode(4, 1, 1)

	graph.AddNewNodeToContainer(nil, n1)
	graph.AddNewNodeToContainer(n1, n2)
	graph.AddNewNodeToContainer(n2, n3)
	graph.AddNewNodeToContainer(n3, n4)

	graph.connectByID(1, 4)

	if n1.HasLeakyEdge() {
		t.Fatal("n1 doesn't have a leaky edge")
	}
	if n4.HasLeakyEdge() {
		t.Fatal("n4 doesn't has have a leaky edge")
	}
	if !n3.HasLeakyEdge() {
		t.Fatal("n3 has a leaky edge")
	}
	if !n2.HasLeakyEdge() {
		t.Fatal("n2 has a leaky edge")
	}

	n5 := NewNode(5, 10, 10)
	n6 := NewNode(6, 1, 1)
	graph.AddNewNodeToContainer(n2, n5)
	graph.AddNewNodeToContainer(n5, n6)

	graph.Disconnect(graph.Edges[0])
	graph.connectByID(4, 6)

	if !n3.HasLeakyEdge() {
		t.Fatal("n3 still has a leaky edge")
	}
	if !n5.HasLeakyEdge() {
		t.Fatal("n5 has a leaky edge")
	}
	if n2.HasLeakyEdge() {
		t.Fatal("n2 doesn't has have a leaky edge")
	}
}
