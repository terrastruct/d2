package d2cycle

import (
	"context"
	"math"
	"strings"
	"testing"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

func TestCycleEdgesStartOnNonRectangularShapeBorder(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a.shape: circle
a -> b -> c
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	var edgeStart *geo.Point
	var srcCenter *geo.Point
	for _, edge := range g.Edges {
		if edge.Src.ID == "a" {
			edgeStart = edge.Route[0]
			srcCenter = edge.Src.Center()
			break
		}
	}
	if edgeStart == nil {
		t.Fatal("expected edge from a")
	}

	got := geo.EuclideanDistance(srcCenter.X, srcCenter.Y, edgeStart.X, edgeStart.Y)
	if math.Abs(got-50) > 0.5 {
		t.Fatalf("expected cycle edge to start on circle border radius 50, got %.2f", got)
	}
}

func TestCompileCycleSetsRootShape(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b
`)

	if !g.Root.IsCycleDiagram() {
		t.Fatalf("expected root shape cycle, got %q", g.Root.Shape.Value)
	}
	for _, obj := range g.Objects {
		if obj.ID == "shape" {
			t.Fatalf("reserved shape key compiled as object: %v", obj)
		}
	}
}

func TestCycleEdgeEndpointsRoundToVisibleBoxBorder(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b -> c -> d -> a
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	for _, edge := range g.Edges {
		assertRoundedOnBoxBorder(t, edge.Route[0], edge.Src)
		assertRoundedOnBoxBorder(t, edge.Route[len(edge.Route)-1], edge.Dst)
	}
}

func TestCycleEdgesEndOnCircleAndHexagonBorders(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a.shape: circle
b.shape: hexagon
c.shape: circle
a -> b -> c -> a
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	for _, edge := range g.Edges {
		assertOnActualShapeBorder(t, edge.Route[0], edge.Src)
		assertOnActualShapeBorder(t, edge.Route[len(edge.Route)-1], edge.Dst)
	}
}

func TestCycleEdgesEndOnBezierShapeBorders(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a.shape: cloud
b.shape: cylinder
c.shape: queue
d.shape: document
a -> b -> c -> d -> a
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	for _, edge := range g.Edges {
		assertOnActualShapeBorder(t, edge.Route[0], edge.Src)
		assertOnActualShapeBorder(t, edge.Route[len(edge.Route)-1], edge.Dst)
	}
}

func TestCircleBezierIntersectionsFindsOffSampleTangent(t *testing.T) {
	radius := 100.0
	curve := straightBezier(geo.NewPoint(-100.6, radius), geo.NewPoint(99.4, radius))

	points := circleBezierIntersections(radius, curve)

	assertHasPointNear(t, points, geo.NewPoint(0, radius), 0.001)
}

func TestCircleBezierIntersectionsKeepsCloseRoots(t *testing.T) {
	t1 := 0.0945
	t2 := 0.0955
	p1 := pointOnCircle(1, 0.35)
	p2 := pointOnCircle(1, 0.38)
	curve := bezierThrough(
		t1,
		t2,
		geo.NewPoint(1.3, 0.0),
		geo.NewPoint(1.3, 0.8),
		p1,
		p2,
	)

	points := circleBezierIntersections(1, curve)

	assertHasPointNear(t, points, p1, 0.001)
	assertHasPointNear(t, points, p2, 0.001)
}

func TestCyclePerimeterIntersectionsIncludesCompositeCircleEllipse(t *testing.T) {
	head := geo.NewEllipse(geo.NewPoint(0, 80), 30, 30)

	points := cyclePerimeterIntersections(100, []geo.Intersectable{head})

	if len(points) != 2 {
		t.Fatalf("expected two circle/head intersections, got %d: %v", len(points), points)
	}
	for _, p := range points {
		if math.Abs(math.Hypot(p.X, p.Y)-100) > 0.001 {
			t.Fatalf("intersection %v is not on the cycle circle", p)
		}
		if math.Abs(geo.EuclideanDistance(0, 80, p.X, p.Y)-30) > 0.001 {
			t.Fatalf("intersection %v is not on the head ellipse", p)
		}
	}
}

func TestCycleArcSamplesStayOnCycleRadiusForNonRectangularShapes(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a.shape: circle
b.shape: hexagon
c.shape: circle
a -> b -> c -> a
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	for _, edge := range g.Edges {
		radius := math.Hypot(edge.Src.Center().X, edge.Src.Center().Y)
		for i := 0; i+3 < len(edge.Route); i += 3 {
			for step := 0; step <= 10; step++ {
				p := cubicPoint(edge.Route[i], edge.Route[i+1], edge.Route[i+2], edge.Route[i+3], float64(step)/10)
				got := math.Hypot(p.X, p.Y)
				if math.Abs(got-radius) > 0.15 {
					t.Fatalf("edge %s -> %s sample radius got %.3f want %.3f at segment %d step %d point %v",
						edge.Src.AbsID(), edge.Dst.AbsID(), got, radius, i/3, step, p)
				}
			}
		}
	}
}

func TestSingleObjectCycleIsCentered(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	a := g.Root.ChildrenArray[0]
	if math.Abs(a.Center().X) > 0.001 || math.Abs(a.Center().Y) > 0.001 {
		t.Fatalf("expected single cycle object centered at origin, got %v", a.Center())
	}
}

func TestSingleObjectSelfEdgeHasFiniteRoute(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> a: retry
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	edge := g.Edges[0]
	if edge.IsCurve {
		t.Fatal("expected self edge to use a normal self-loop route")
	}
	for _, p := range edge.Route {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			t.Fatalf("self edge route contains invalid point %v", p)
		}
	}
}

func TestSingleEdgeCycleUsesCircularArc(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	edge := edgeByEndpoints(t, g, objectByID(t, g, "a"), objectByID(t, g, "b"))
	if !edge.IsCurve {
		t.Fatal("expected single root edge to use a circular cycle arc")
	}
}

func TestNestedEdgeRoutesMoveWithSameCycleNode(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a: {
  x -> y
}
b
a -> b
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, fixedNestedLayout); err != nil {
		t.Fatal(err)
	}

	x := objectByID(t, g, "x")
	y := objectByID(t, g, "y")
	edge := edgeByEndpoints(t, g, x, y)

	if edge.IsCurve {
		t.Fatal("expected internal nested edge to keep core-layout route instead of cycle arc")
	}
	assertRouteEndpoint(t, edge.Route[0], x.Center())
	assertRouteEndpoint(t, edge.Route[1], y.Center())
}

func TestNonAdjacentDirectEdgeIsNotCycleArc(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b -> c -> d -> a
a -> c
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	edge := edgeByEndpoints(t, g, objectByID(t, g, "a"), objectByID(t, g, "c"))
	if edge.IsCurve {
		t.Fatal("expected non-adjacent direct edge to use normal routing instead of a cycle arc")
	}
	assertOnShapeBorder(t, edge.Route[0], edge.Src)
	assertOnShapeBorder(t, edge.Route[len(edge.Route)-1], edge.Dst)
}

func TestSingleStatementNonAdjacentEdgeIsNotCycleArc(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a
b
c
a -> c
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	edge := edgeByEndpoints(t, g, objectByID(t, g, "a"), objectByID(t, g, "c"))
	if edge.IsCurve {
		t.Fatal("expected non-adjacent single edge to use normal routing instead of a cycle arc")
	}
	assertOnShapeBorder(t, edge.Route[0], edge.Src)
	assertOnShapeBorder(t, edge.Route[len(edge.Route)-1], edge.Dst)
}

func TestPredeclaredNodeKeepsEdgeChainOrder(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
c: C has a longer label
a -> b -> c -> d -> a
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	for _, pair := range [][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}, {"d", "a"}} {
		edge := edgeByEndpoints(t, g, objectByID(t, g, pair[0]), objectByID(t, g, pair[1]))
		if !edge.IsCurve {
			t.Fatalf("expected edge %s -> %s to follow the cycle arc", pair[0], pair[1])
		}
	}
}

func TestOpenChainCanCloseWithSingleStatementEdge(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b -> c
c -> a
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	for _, pair := range [][2]string{{"a", "b"}, {"b", "c"}, {"c", "a"}} {
		edge := edgeByEndpoints(t, g, objectByID(t, g, pair[0]), objectByID(t, g, pair[1]))
		if !edge.IsCurve {
			t.Fatalf("expected edge %s -> %s to follow the cycle arc", pair[0], pair[1])
		}
	}
}

func TestOpenChainCanContinueWithSingleStatementEdge(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b -> c
c -> d
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	edge := edgeByEndpoints(t, g, objectByID(t, g, "c"), objectByID(t, g, "d"))
	if !edge.IsCurve {
		t.Fatal("expected single edge continuing an open chain to follow the cycle arc")
	}
}

func TestOpenChainCanContinueAcrossSourceChains(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b -> c
c -> d -> a
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	for _, pair := range [][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}, {"d", "a"}} {
		edge := edgeByEndpoints(t, g, objectByID(t, g, pair[0]), objectByID(t, g, pair[1]))
		if !edge.IsCurve {
			t.Fatalf("expected edge %s -> %s to follow the cycle arc", pair[0], pair[1])
		}
	}
}

func TestParallelEdgeDoesNotReuseCycleArc(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b -> c -> a
a -> b: duplicate
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	edges := edgesByEndpoints(t, g, objectByID(t, g, "a"), objectByID(t, g, "b"))
	if len(edges) != 2 {
		t.Fatalf("expected two a -> b edges, got %d", len(edges))
	}
	curved := 0
	for _, edge := range edges {
		if edge.IsCurve {
			curved++
		}
	}
	if curved != 1 {
		t.Fatalf("expected exactly one a -> b edge on the cycle arc, got %d", curved)
	}
}

func TestSingleStatementParallelEdgeUsesOnlyOneCycleArc(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a
b
a -> b
a -> b: duplicate
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	edges := edgesByEndpoints(t, g, objectByID(t, g, "a"), objectByID(t, g, "b"))
	curved := 0
	for _, edge := range edges {
		if edge.IsCurve {
			curved++
		}
	}
	if curved != 1 {
		t.Fatalf("expected exactly one parallel edge on the cycle arc, got %d", curved)
	}
}

func TestClosedCycleKeepsClosingArcWithLongerTail(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b -> c -> a
c -> d -> e
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	closing := edgeByEndpoints(t, g, objectByID(t, g, "c"), objectByID(t, g, "a"))
	if !closing.IsCurve {
		t.Fatal("expected closing cycle edge c -> a to stay on the circular route")
	}
	for _, pair := range [][2]string{{"c", "d"}, {"d", "e"}} {
		edge := edgeByEndpoints(t, g, objectByID(t, g, pair[0]), objectByID(t, g, pair[1]))
		if edge.IsCurve {
			t.Fatalf("expected tail edge %s -> %s to use normal routing", pair[0], pair[1])
		}
	}
}

func TestNestedCycleFitsObjectsAndRoutes(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b
`)
	g.RootLevel = 1
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}
	if g.Root.Box == nil {
		t.Fatal("expected nested cycle root box to be set")
	}

	for _, obj := range g.Objects {
		assertPointInsideBox(t, obj.TopLeft, g.Root.Box)
		assertPointInsideBox(t, geo.NewPoint(obj.TopLeft.X+obj.Width, obj.TopLeft.Y+obj.Height), g.Root.Box)
	}
	for _, edge := range g.Edges {
		for _, p := range edge.Route {
			assertPointInsideBox(t, p, g.Root.Box)
		}
	}
}

func TestNestedCycleBoundsIncludeOutsideLabel(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a: wide label
b
a -> b
`)
	g.RootLevel = 1
	sizeObjects(g)
	a := objectByID(t, g, "a")
	a.LabelDimensions = d2target.TextDimensions{Width: 420, Height: 24}

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	if a.LabelPosition == nil || !label.FromString(*a.LabelPosition).IsOutside() {
		t.Fatalf("expected outside label position, got %v", a.LabelPosition)
	}
	labelTL := a.GetLabelTopLeft()
	assertPointInsideBox(t, labelTL, g.Root.Box)
	assertPointInsideBox(t, geo.NewPoint(labelTL.X+float64(a.LabelDimensions.Width), labelTL.Y+float64(a.LabelDimensions.Height)), g.Root.Box)
}

func TestNestedCycleBoundsIncludeOutsideIcon(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a: {
  icon: https://icons.terrastruct.com/essentials/004-picture.svg
  x
}
b
a -> b
`)
	g.RootLevel = 1
	sizeObjects(g)

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	a := objectByID(t, g, "a")
	if a.IconPosition == nil || !label.FromString(*a.IconPosition).IsOutside() {
		t.Fatalf("expected outside icon position, got %v", a.IconPosition)
	}
	iconPosition := label.FromString(*a.IconPosition)
	iconBox := a.ToShape().GetBox()
	if !iconPosition.IsOutside() {
		iconBox = a.ToShape().GetInnerBox()
	}
	iconSize := float64(d2target.GetIconSize(iconBox, *a.IconPosition))
	iconTL := iconPosition.GetPointOnBox(iconBox, label.PADDING, iconSize, iconSize)
	assertPointInsideBox(t, iconTL, g.Root.Box)
	assertPointInsideBox(t, geo.NewPoint(iconTL.X+iconSize, iconTL.Y+iconSize), g.Root.Box)
}

func TestCycleRadiusIncludesOutsideLabels(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a: long
b: long
c: long
d: long
e: long
f: long
g: long
h: long
a -> b -> c -> d -> e -> f -> g -> h -> a
`)
	for _, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(0, 0), 20, 20)
		obj.LabelDimensions = d2target.TextDimensions{Width: 300, Height: 24}
		position := label.OutsideBottomCenter.String()
		obj.LabelPosition = &position
	}

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	var boxes []geo.Box
	for _, obj := range g.Objects {
		boxes = append(boxes, geo.Box{
			TopLeft: obj.GetLabelTopLeft(),
			Width:   float64(obj.LabelDimensions.Width),
			Height:  float64(obj.LabelDimensions.Height),
		})
	}
	assertNoBoxOverlaps(t, boxes)
}

func TestNestedCycleBoundsIncludeEdgeLabels(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b: very long label
b -> c: very long label
c -> a: very long label
`)
	g.RootLevel = 1
	sizeObjects(g)
	for _, edge := range g.Edges {
		edge.LabelDimensions = d2target.TextDimensions{Width: 240, Height: 24}
	}

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	for _, edge := range g.Edges {
		labelTL, width, height := edgeLabelBox(edge)
		assertPointInsideBox(t, labelTL, g.Root.Box)
		assertPointInsideBox(t, geo.NewPoint(labelTL.X+width, labelTL.Y+height), g.Root.Box)
	}
}

func TestNestedCycleBoundsIncludeConnectionIcon(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b: {
  label: edge icon beside label
  icon: https://icons.terrastruct.com/essentials/004-picture.svg
}
`)
	g.RootLevel = 1
	sizeObjects(g)
	g.Edges[0].LabelDimensions = d2target.TextDimensions{Width: 220, Height: 24}

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	edge := g.Edges[0]
	iconTL, width, height := edgeIconBox(edge)
	assertPointInsideBox(t, iconTL, g.Root.Box)
	assertPointInsideBox(t, geo.NewPoint(iconTL.X+width, iconTL.Y+height), g.Root.Box)
}

func TestNestedCycleBoundsIncludeArrowheadLabels(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a -> b: {
  source-arrowhead: very long source label
  target-arrowhead: very long target label
  style.stroke-width: 10
}
`)
	g.RootLevel = 1
	sizeObjects(g)
	edge := g.Edges[0]
	if edge.SrcArrowhead == nil {
		t.Fatal("expected source arrowhead")
	}
	if edge.DstArrowhead == nil {
		t.Fatal("expected target arrowhead")
	}
	edge.SrcArrowhead.LabelDimensions = d2target.TextDimensions{Width: 460, Height: 24}
	edge.DstArrowhead.LabelDimensions = d2target.TextDimensions{Width: 520, Height: 24}

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	labelTL, width, height := edgeArrowheadLabelBox(edge, false)
	assertPointInsideBox(t, labelTL, g.Root.Box)
	assertPointInsideBox(t, geo.NewPoint(labelTL.X+width, labelTL.Y+height), g.Root.Box)

	labelTL, width, height = edgeArrowheadLabelBox(edge, true)
	assertPointInsideBox(t, labelTL, g.Root.Box)
	assertPointInsideBox(t, geo.NewPoint(labelTL.X+width, labelTL.Y+height), g.Root.Box)
}

func TestCycleContainerDefaultIconLabelAvoidOverlap(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a: long container label {
  icon: https://icons.terrastruct.com/essentials/004-picture.svg
  x
}
b
a -> b
`)
	sizeObjects(g)
	a := objectByID(t, g, "a")
	a.LabelDimensions = d2target.TextDimensions{Width: 360, Height: 24}

	if err := Layout(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}

	labelTL := a.GetLabelTopLeft()
	iconTL, iconSize := objectIconBox(a)
	labelBox := geo.Box{
		TopLeft: labelTL,
		Width:   float64(a.LabelDimensions.Width),
		Height:  float64(a.LabelDimensions.Height),
	}
	iconBox := geo.Box{
		TopLeft: iconTL,
		Width:   iconSize,
		Height:  iconSize,
	}
	if boxesOverlap(labelBox, iconBox) {
		t.Fatalf("label %v overlaps icon %v", labelBox, iconBox)
	}
}

func TestCrossNodeDescendantEdgeIsReroutedAfterCycleMove(t *testing.T) {
	g := compileCycle(t, `
shape: cycle
a: {
  x
}
b: {
  y
}
a -> b
a.x -> b.y
`)
	sizeObjects(g)

	if err := Layout(context.Background(), g, fixedNestedLayout); err != nil {
		t.Fatal(err)
	}

	x := objectByID(t, g, "x")
	y := objectByID(t, g, "y")
	edge := edgeByEndpoints(t, g, x, y)

	if edge.IsCurve {
		t.Fatal("expected cross-node descendant edge to be rerouted as a normal edge")
	}
	assertOnShapeBorder(t, edge.Route[0], x)
	assertOnShapeBorder(t, edge.Route[1], y)
}

func compileCycle(t *testing.T, text string) *d2graph.Graph {
	t.Helper()
	g, _, err := d2compiler.Compile("", strings.NewReader(text), nil)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func sizeObjects(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(0, 0), 100, 100)
	}
}

func fixedNestedLayout(ctx context.Context, g *d2graph.Graph) error {
	for _, obj := range g.Objects {
		switch obj.ID {
		case "x":
			obj.TopLeft = geo.NewPoint(20, 30)
		case "y":
			obj.TopLeft = geo.NewPoint(160, 30)
		}
	}
	for _, edge := range g.Edges {
		edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
	}
	return nil
}

func objectByID(t *testing.T, g *d2graph.Graph, id string) *d2graph.Object {
	t.Helper()
	for _, obj := range g.Objects {
		if obj.ID == id {
			return obj
		}
	}
	t.Fatalf("object %q not found", id)
	return nil
}

func edgeByEndpoints(t *testing.T, g *d2graph.Graph, src, dst *d2graph.Object) *d2graph.Edge {
	t.Helper()
	for _, edge := range g.Edges {
		if edge.Src == src && edge.Dst == dst {
			return edge
		}
	}
	t.Fatalf("edge %s -> %s not found", src.AbsID(), dst.AbsID())
	return nil
}

func edgesByEndpoints(t *testing.T, g *d2graph.Graph, src, dst *d2graph.Object) []*d2graph.Edge {
	t.Helper()
	var edges []*d2graph.Edge
	for _, edge := range g.Edges {
		if edge.Src == src && edge.Dst == dst {
			edges = append(edges, edge)
		}
	}
	return edges
}

func assertRouteEndpoint(t *testing.T, got, want *geo.Point) {
	t.Helper()
	if math.Abs(got.X-want.X) > 0.001 || math.Abs(got.Y-want.Y) > 0.001 {
		t.Fatalf("route endpoint got %v want %v", got, want)
	}
}

func assertOnShapeBorder(t *testing.T, got *geo.Point, obj *d2graph.Object) {
	t.Helper()
	box := obj.Box
	onX := math.Abs(got.X-box.TopLeft.X) <= 0.001 ||
		math.Abs(got.X-(box.TopLeft.X+box.Width)) <= 0.001
	onY := math.Abs(got.Y-box.TopLeft.Y) <= 0.001 ||
		math.Abs(got.Y-(box.TopLeft.Y+box.Height)) <= 0.001
	if !onX && !onY {
		t.Fatalf("route endpoint %v is not on border of %s box %v", got, obj.AbsID(), box)
	}
}

func assertPointInsideBox(t *testing.T, p *geo.Point, box *geo.Box) {
	t.Helper()
	if p.X < box.TopLeft.X-0.001 || p.Y < box.TopLeft.Y-0.001 ||
		p.X > box.TopLeft.X+box.Width+0.001 ||
		p.Y > box.TopLeft.Y+box.Height+0.001 {
		t.Fatalf("point %v is outside box %v", p, box)
	}
}

func assertRoundedOnBoxBorder(t *testing.T, got *geo.Point, obj *d2graph.Object) {
	t.Helper()
	box := obj.Box
	x := math.Round(got.X)
	y := math.Round(got.Y)
	left := math.Round(box.TopLeft.X)
	right := math.Round(box.TopLeft.X + box.Width)
	top := math.Round(box.TopLeft.Y)
	bottom := math.Round(box.TopLeft.Y + box.Height)
	if x != left && x != right && y != top && y != bottom {
		t.Fatalf("rounded route endpoint %v is not on visible box border of %s box %v", got, obj.AbsID(), box)
	}
}

func assertOnActualShapeBorder(t *testing.T, got *geo.Point, obj *d2graph.Object) {
	t.Helper()
	s := obj.ToShape()
	switch s.GetType() {
	case shape.CIRCLE_TYPE:
		radius := obj.Width / 2
		dist := geo.EuclideanDistance(obj.Center().X, obj.Center().Y, got.X, got.Y)
		if math.Abs(dist-radius) > 0.001 {
			t.Fatalf("route endpoint %v is not on circle border of %s: radius got %.3f want %.3f", got, obj.AbsID(), dist, radius)
		}
	case shape.HEXAGON_TYPE:
		for _, side := range s.Perimeter() {
			segment, ok := side.(*geo.Segment)
			if !ok {
				continue
			}
			if got.DistanceToLine(segment.Start, segment.End) <= 0.001 && pointWithinSegment(got, segment, 0.001) {
				return
			}
		}
		t.Fatalf("route endpoint %v is not on hexagon border of %s", got, obj.AbsID())
	default:
		if !pointNearShapePerimeter(got, s, 1.0) {
			t.Fatalf("route endpoint %v is not on shape border of %s", got, obj.AbsID())
		}
	}
}

func pointNearShapePerimeter(p *geo.Point, s shape.Shape, tolerance float64) bool {
	for _, side := range s.Perimeter() {
		switch side := side.(type) {
		case *geo.Segment:
			if p.DistanceToLine(side.Start, side.End) <= tolerance && pointWithinSegment(p, side, tolerance) {
				return true
			}
		case geo.Segment:
			if p.DistanceToLine(side.Start, side.End) <= tolerance && pointWithinSegment(p, &side, tolerance) {
				return true
			}
		case *geo.BezierCurve:
			if pointNearBezier(p, side, tolerance) {
				return true
			}
		case geo.BezierCurve:
			if pointNearBezier(p, &side, tolerance) {
				return true
			}
		case *geo.Ellipse:
			if pointNearEllipse(p, side, tolerance) {
				return true
			}
		case geo.Ellipse:
			if pointNearEllipse(p, &side, tolerance) {
				return true
			}
		}
	}
	return false
}

func pointNearBezier(p *geo.Point, curve *geo.BezierCurve, tolerance float64) bool {
	const samples = 200
	for i := 0; i <= samples; i++ {
		cp := curve.At(float64(i) / samples)
		if geo.EuclideanDistance(p.X, p.Y, cp.X, cp.Y) <= tolerance {
			return true
		}
	}
	return false
}

func pointNearEllipse(p *geo.Point, ellipse *geo.Ellipse, tolerance float64) bool {
	if ellipse.Rx <= 0 || ellipse.Ry <= 0 {
		return false
	}
	dx := (p.X - ellipse.Center.X) / ellipse.Rx
	dy := (p.Y - ellipse.Center.Y) / ellipse.Ry
	return math.Abs(dx*dx+dy*dy-1) <= tolerance/math.Min(ellipse.Rx, ellipse.Ry)
}

func pointWithinSegment(p *geo.Point, segment *geo.Segment, tolerance float64) bool {
	minX := math.Min(segment.Start.X, segment.End.X) - tolerance
	maxX := math.Max(segment.Start.X, segment.End.X) + tolerance
	minY := math.Min(segment.Start.Y, segment.End.Y) - tolerance
	maxY := math.Max(segment.Start.Y, segment.End.Y) + tolerance
	return p.X >= minX && p.X <= maxX && p.Y >= minY && p.Y <= maxY
}

func assertNoBoxOverlaps(t *testing.T, boxes []geo.Box) {
	t.Helper()
	for i := 0; i < len(boxes); i++ {
		for j := i + 1; j < len(boxes); j++ {
			if boxesOverlap(boxes[i], boxes[j]) {
				t.Fatalf("box %d %v overlaps box %d %v", i, boxes[i], j, boxes[j])
			}
		}
	}
}

func boxesOverlap(a, b geo.Box) bool {
	return a.TopLeft.X < b.TopLeft.X+b.Width &&
		a.TopLeft.X+a.Width > b.TopLeft.X &&
		a.TopLeft.Y < b.TopLeft.Y+b.Height &&
		a.TopLeft.Y+a.Height > b.TopLeft.Y
}

func assertHasPointNear(t *testing.T, points []*geo.Point, want *geo.Point, tolerance float64) {
	t.Helper()
	for _, got := range points {
		if geo.EuclideanDistance(got.X, got.Y, want.X, want.Y) <= tolerance {
			return
		}
	}
	t.Fatalf("expected point near %v in %v", want, points)
}

func straightBezier(start, end *geo.Point) *geo.BezierCurve {
	return geo.NewBezierCurve([]*geo.Point{
		start,
		geo.NewPoint(start.X+(end.X-start.X)/3, start.Y+(end.Y-start.Y)/3),
		geo.NewPoint(start.X+2*(end.X-start.X)/3, start.Y+2*(end.Y-start.Y)/3),
		end,
	})
}

func bezierThrough(t1, t2 float64, p0, p3, at1, at2 *geo.Point) *geo.BezierCurve {
	mt1 := 1 - t1
	mt2 := 1 - t2
	b0t1 := mt1 * mt1 * mt1
	b1t1 := 3 * mt1 * mt1 * t1
	b2t1 := 3 * mt1 * t1 * t1
	b3t1 := t1 * t1 * t1
	b0t2 := mt2 * mt2 * mt2
	b1t2 := 3 * mt2 * mt2 * t2
	b2t2 := 3 * mt2 * t2 * t2
	b3t2 := t2 * t2 * t2
	den := b1t1*b2t2 - b1t2*b2t1

	solve := func(v1, v2, p0, p3 float64) (float64, float64) {
		r1 := v1 - b0t1*p0 - b3t1*p3
		r2 := v2 - b0t2*p0 - b3t2*p3
		return (r1*b2t2 - r2*b2t1) / den, (b1t1*r2 - b1t2*r1) / den
	}

	p1x, p2x := solve(at1.X, at2.X, p0.X, p3.X)
	p1y, p2y := solve(at1.Y, at2.Y, p0.Y, p3.Y)
	return geo.NewBezierCurve([]*geo.Point{
		p0,
		geo.NewPoint(p1x, p1y),
		geo.NewPoint(p2x, p2y),
		p3,
	})
}

func cubicPoint(p0, p1, p2, p3 *geo.Point, t float64) *geo.Point {
	mt := 1 - t
	return geo.NewPoint(
		mt*mt*mt*p0.X+3*mt*mt*t*p1.X+3*mt*t*t*p2.X+t*t*t*p3.X,
		mt*mt*mt*p0.Y+3*mt*mt*t*p1.Y+3*mt*t*t*p2.Y+t*t*t*p3.Y,
	)
}
