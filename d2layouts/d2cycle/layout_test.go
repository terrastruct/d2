package d2cycle

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func compileGraph(t *testing.T, script string) *d2graph.Graph {
	t.Helper()
	g, _, err := d2compiler.Compile("", strings.NewReader(script), nil)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// fakeCoreLayout gives every object a 100x100 box, spaces root children
// horizontally and routes all edges as straight lines between centers.
func fakeCoreLayout(_ context.Context, g *d2graph.Graph) error {
	x := 0.
	for _, obj := range g.Root.ChildrenArray {
		setBox(obj, geo.NewPoint(x, 0), 100, 100)
		x += 300
	}
	for _, obj := range g.Objects {
		if obj.Parent == nil || obj.Parent == g.Root {
			continue
		}
		p := obj.Parent
		setBox(obj, geo.NewPoint(p.TopLeft.X+20, p.TopLeft.Y+20), 30, 30)
	}
	for _, edge := range g.Edges {
		edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
	}
	return nil
}

func setBox(obj *d2graph.Object, tl *geo.Point, w, h float64) {
	obj.Box = geo.NewBox(tl, w, h)
}

func TestCalculateRadius(t *testing.T) {
	g := compileGraph(t, `shape: cycle
a
`)
	if got := calculateRadius(g.Root.ChildrenArray); got != 0 {
		t.Fatalf("expected radius 0 for single child, got %v", got)
	}

	g = compileGraph(t, `shape: cycle
a; b; c
`)
	for _, obj := range g.Objects {
		setBox(obj, geo.NewPoint(0, 0), 300, 300)
	}
	radius := calculateRadius(g.Root.ChildrenArray)
	// Children must fit: half diagonal + padding away from each other along
	// the chord between adjacent positions.
	want := (150*math.Sqrt2 + padding) / math.Sin(math.Pi/3)
	if math.Abs(radius-want) > 0.001 {
		t.Fatalf("expected radius %v, got %v", want, radius)
	}

	g = compileGraph(t, `shape: cycle
a; b
`)
	for _, obj := range g.Objects {
		setBox(obj, geo.NewPoint(0, 0), 40, 40)
	}
	if got := calculateRadius(g.Root.ChildrenArray); got != minRadius {
		t.Fatalf("expected small children to use the minimum radius %v, got %v", minRadius, got)
	}
}

func TestLayoutPositionsChildrenOnCircle(t *testing.T) {
	g := compileGraph(t, `shape: cycle
a -> b -> c -> d -> a
`)
	if err := Layout(context.Background(), g, fakeCoreLayout); err != nil {
		t.Fatal(err)
	}

	children := g.Root.ChildrenArray
	if len(children) != 4 {
		t.Fatalf("expected 4 children, got %d", len(children))
	}
	var cx, cy float64
	for _, obj := range children {
		cx += obj.Center().X
		cy += obj.Center().Y
	}
	cx /= float64(len(children))
	cy /= float64(len(children))

	radius := -1.
	for _, obj := range children {
		d := math.Hypot(obj.Center().X-cx, obj.Center().Y-cy)
		if radius == -1 {
			radius = d
		}
		if math.Abs(d-radius) > 0.001 {
			t.Fatalf("expected %q at radius %v from (%v,%v), got %v", obj.ID, radius, cx, cy, d)
		}
	}
	if radius < minRadius-0.001 {
		t.Fatalf("expected radius >= %v, got %v", minRadius, radius)
	}

	// First child sits at 12 o'clock, the rest clockwise.
	first := children[0]
	if math.Abs(first.Center().X-cx) > 0.001 || first.Center().Y >= cy {
		t.Fatalf("expected first child centered on top, got (%v,%v) with center (%v,%v)", first.Center().X, first.Center().Y, cx, cy)
	}
	second := children[1]
	if second.Center().X <= cx {
		t.Fatalf("expected second child on the right side, got (%v,%v)", second.Center().X, second.Center().Y)
	}
}

func TestLayoutAddsContainerInset(t *testing.T) {
	g := compileGraph(t, `shape: cycle
a -> b -> c -> d -> a
`)
	g.Root.Label.Value = "Cycle"
	g.Root.LabelDimensions = d2target.TextDimensions{Width: 80, Height: 40}
	labelPosition := label.InsideTopCenter.String()
	g.Root.LabelPosition = &labelPosition
	if err := Layout(context.Background(), g, fakeCoreLayout); err != nil {
		t.Fatal(err)
	}

	root := g.Root.Box
	if root == nil {
		t.Fatal("expected cycle root box")
	}
	for _, child := range g.Root.ChildrenArray {
		if child.TopLeft.X <= root.TopLeft.X || child.TopLeft.Y < root.TopLeft.Y+50 ||
			child.TopLeft.X+child.Width >= root.TopLeft.X+root.Width ||
			child.TopLeft.Y+child.Height >= root.TopLeft.Y+root.Height {
			t.Fatalf("expected child %q to be inset from cycle container, got child %v and root %v", child.ID, child.TopLeft, root.TopLeft)
		}
	}
	for _, edge := range g.Edges {
		if !edge.IsCurve {
			t.Fatalf("expected cycle edge %q to remain curved", edge.AbsID())
		}
		for _, point := range edge.Route {
			if point.X <= root.TopLeft.X || point.Y <= root.TopLeft.Y ||
				point.X >= root.TopLeft.X+root.Width || point.Y >= root.TopLeft.Y+root.Height {
				t.Fatalf("expected edge %q point %v to be inset from cycle container %v", edge.AbsID(), point, root.TopLeft)
			}
		}
	}
}

func TestLayoutRoutesCircularArcsTrimmedToBorders(t *testing.T) {
	g := compileGraph(t, `shape: cycle
a -> b -> c -> d -> a
`)
	if err := Layout(context.Background(), g, fakeCoreLayout); err != nil {
		t.Fatal(err)
	}

	byID := make(map[string]*d2graph.Object)
	for _, obj := range g.Objects {
		byID[obj.ID] = obj
	}
	var cx, cy, radius float64
	for i, obj := range g.Root.ChildrenArray {
		cx += obj.Center().X
		cy += obj.Center().Y
		if i == len(g.Root.ChildrenArray)-1 {
			cx /= float64(len(g.Root.ChildrenArray))
			cy /= float64(len(g.Root.ChildrenArray))
			radius = math.Hypot(byID["a"].Center().X-cx, byID["a"].Center().Y-cy)
		}
	}

	for _, edge := range g.Edges {
		if !edge.IsCurve {
			t.Fatalf("expected edge %q to be routed as a curve", edge.AbsID())
		}
		route := edge.Route
		// Bezier chain: 1 + 3n points.
		if len(route) < 4 || (len(route)-1)%3 != 0 {
			t.Fatalf("expected edge %q route to be a Bezier chain, got %d points", edge.AbsID(), len(route))
		}
		for i, p := range route {
			if i%3 != 0 {
				continue
			}
			if d := math.Abs(math.Hypot(p.X-cx, p.Y-cy) - radius); d > 0.001 {
				t.Fatalf("expected chain endpoint %v of edge %q on the circle (radius %v), off by %v", p, edge.AbsID(), radius, d)
			}
		}
		// Endpoints must stop at the endpoints' borders, not their centers.
		if borderGap(route[0], byID[edge.Src.ID]) > 0.001 {
			t.Fatalf("expected edge %q to start on the source border, got %v", edge.AbsID(), route[0])
		}
		if borderGap(route[len(route)-1], byID[edge.Dst.ID]) > 0.001 {
			t.Fatalf("expected edge %q to end on the destination border, got %v", edge.AbsID(), route[len(route)-1])
		}
	}
}

func borderGap(p *geo.Point, obj *d2graph.Object) float64 {
	dx := math.Max(math.Abs(p.X-obj.Center().X)-obj.Width/2, 0)
	dy := math.Max(math.Abs(p.Y-obj.Center().Y)-obj.Height/2, 0)
	return math.Hypot(dx, dy)
}

func TestLayoutKeepsNestedContentAndCrossChildEdges(t *testing.T) {
	g := compileGraph(t, `shape: cycle
x: {
  p -> q
}
y
x.p -> y
`)
	if err := Layout(context.Background(), g, fakeCoreLayout); err != nil {
		t.Fatal(err)
	}

	var p, q *d2graph.Object
	for _, obj := range g.Objects {
		switch obj.AbsID() {
		case "x.p":
			p = obj
		case "x.q":
			q = obj
		}
	}
	if p == nil || q == nil {
		t.Fatal("expected nested objects to survive the cycle layout")
	}
	// p and q keep their internal offset inside x after x moved onto the ring.
	x := g.Root.ChildrenArray[0]
	if math.Abs(p.Center().X-(x.TopLeft.X+20+15)) > 0.001 || math.Abs(p.Center().Y-(x.TopLeft.Y+20+15)) > 0.001 {
		t.Fatalf("expected nested content to move with its container, got p at (%v,%v) with x at %v", p.Center().X, p.Center().Y, x.TopLeft)
	}

	for _, edge := range g.Edges {
		srcInX, dstInX := edge.Src.Parent == x, edge.Dst.Parent == x
		if srcInX && dstInX {
			if !samePoint(edge.Route[0], edge.Src.Center()) || !samePoint(edge.Route[len(edge.Route)-1], edge.Dst.Center()) {
				t.Fatalf("expected internal edge of %q to move with the container", x.ID)
			}
		}
		if !srcInX && dstInX || srcInX && !dstInX {
			if !samePoint(edge.Route[0], edge.Src.Center()) || !samePoint(edge.Route[len(edge.Route)-1], edge.Dst.Center()) {
				t.Fatalf("expected cross-child edge to stay attached to its endpoints")
			}
		}
	}
}

func TestLayoutOppositeEdgesOnTwoNodeCycleDoNotOverlap(t *testing.T) {
	g := compileGraph(t, `shape: cycle
a -> b
b -> a
`)
	if err := Layout(context.Background(), g, fakeCoreLayout); err != nil {
		t.Fatal(err)
	}

	var cx, cy float64
	for _, obj := range g.Root.ChildrenArray {
		cx += obj.Center().X
		cy += obj.Center().Y
	}
	cx /= 2
	cy /= 2

	angles := make([][]float64, 0, 2)
	for _, edge := range g.Edges {
		var as, ae float64
		for i, p := range edge.Route {
			if i%3 != 0 {
				continue
			}
			a := math.Atan2(p.Y-cy, p.X-cx)
			switch i {
			case 0:
				as = a
			default:
				ae = a
			}
		}
		angles = append(angles, []float64{modulo2Pi(as), modulo2Pi(ae)})
	}

	// One arc must pass through the right side (angle 0), the other through
	// the left side (angle pi).
	passesZero, passesPi := false, false
	for _, a := range angles {
		mid := modulo2Pi(a[0] + modulo2Pi(a[1]-a[0])/2)
		if math.Abs(mid) < math.Pi/4 || math.Abs(mid) > 7*math.Pi/4 {
			passesZero = true
		}
		if math.Abs(mid-math.Pi) < math.Pi/4 {
			passesPi = true
		}
	}
	if !passesZero || !passesPi {
		t.Fatalf("expected opposite edges to take opposite semicircles, got %v", angles)
	}
}

func samePoint(a, b *geo.Point) bool {
	return math.Abs(a.X-b.X) < 0.001 && math.Abs(a.Y-b.Y) < 0.001
}
