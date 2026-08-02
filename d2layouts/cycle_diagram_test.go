package d2layouts

import (
	"context"
	"strings"
	"testing"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/util-go/go2"
)

func TestNestedCycleInjectsExternalEdgesInsideContainer(t *testing.T) {
	g, _, err := d2compiler.Compile("", strings.NewReader(`
cluster: {
  shape: cycle
  a -> b -> c -> a
}
outside
outside -> cluster.b
cluster.c -> outside
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	sizeTestObjects(g)

	ctx := log.WithDefault(context.Background())
	err = LayoutNested(ctx, g, NestedGraphInfo(g.Root), fixedCycleRootLayout, cycleTestRouter)
	if err != nil {
		t.Fatal(err)
	}

	cluster := testObjectByID(t, g, "cluster")
	if cluster.LabelPosition == nil {
		t.Fatal("expected nested cycle label position to be set")
	}
	if *cluster.LabelPosition != label.OutsideTopCenter.String() {
		t.Fatalf("expected nested cycle label outside top center, got %q", *cluster.LabelPosition)
	}
	for _, obj := range g.Objects {
		if obj != cluster && obj.IsDescendantOf(cluster) {
			assertObjectInside(t, obj, cluster)
		}
	}
	for _, edge := range g.Edges {
		if edge.Src.IsDescendantOf(cluster) && edge.Dst.IsDescendantOf(cluster) {
			for _, p := range edge.Route {
				assertPointInside(t, p, cluster.Box)
			}
		}
	}
}

func fixedCycleRootLayout(ctx context.Context, g *d2graph.Graph) error {
	ensureTestLabelPositions(g.Objects)
	x := 0.0
	for _, obj := range g.Root.ChildrenArray {
		obj.TopLeft = geo.NewPoint(x, 0)
		x += obj.Width + 80
	}
	for _, edge := range g.Edges {
		edge.Route = []*geo.Point{edge.Src.Center(), edge.Dst.Center()}
	}
	return nil
}

func cycleTestRouter(ctx context.Context, g *d2graph.Graph, edges []*d2graph.Edge) error {
	ensureTestLabelPositions(g.Objects)
	return DefaultRouter(ctx, g, edges)
}

func sizeTestObjects(g *d2graph.Graph) {
	for _, obj := range g.Objects {
		if obj.Box == nil {
			obj.Box = geo.NewBox(geo.NewPoint(0, 0), 100, 100)
			continue
		}
		obj.Box.Width = 100
		obj.Box.Height = 100
	}
	ensureTestLabelPositions(g.Objects)
}

func ensureTestLabelPositions(objects []*d2graph.Object) {
	for _, obj := range objects {
		if obj.HasLabel() && obj.LabelPosition == nil {
			obj.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}
}

func testObjectByID(t *testing.T, g *d2graph.Graph, id string) *d2graph.Object {
	t.Helper()
	for _, obj := range g.Objects {
		if obj.ID == id {
			return obj
		}
	}
	t.Fatalf("object %q not found", id)
	return nil
}

func assertObjectInside(t *testing.T, obj, container *d2graph.Object) {
	t.Helper()
	assertPointInside(t, obj.TopLeft, container.Box)
	assertPointInside(t, geo.NewPoint(obj.TopLeft.X+obj.Width, obj.TopLeft.Y+obj.Height), container.Box)
}

func assertPointInside(t *testing.T, p *geo.Point, box *geo.Box) {
	t.Helper()
	if p.X < box.TopLeft.X-0.001 || p.Y < box.TopLeft.Y-0.001 ||
		p.X > box.TopLeft.X+box.Width+0.001 ||
		p.Y > box.TopLeft.Y+box.Height+0.001 {
		t.Fatalf("point %v is outside box %v", p, box)
	}
}
