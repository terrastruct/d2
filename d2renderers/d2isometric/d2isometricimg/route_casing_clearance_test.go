package d2isometricimg

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestRouteCasingClearsTerracesWithoutMovingInkOrBridge(t *testing.T) {
	for _, radius := range []float64{.012, .024, .075} {
		for _, points := range [][]Vec{
			{nv(0, .08, 0), nv(2, .08, 0)},
			{nv(0, .08, 0), nv(1, .16, 0), nv(2, .08, 0)},
		} {
			before := append([]Vec(nil), points...)
			base := &meshBuilder{ctx: context.Background()}
			raised := &meshBuilder{ctx: context.Background(), routeCasingFloor: hierarchyTerraceCeiling + .0005}
			ink := nativeMaterial("#123456", 1, 0, 1)
			base.routeInk(points, radius, ink)
			raised.routeInk(points, radius, ink)
			if !reflect.DeepEqual(base.triangles, raised.triangles) {
				t.Fatal("casing clearance changed colored route geometry")
			}
			base.triangles, raised.triangles = nil, nil
			base.routeCasing(points, radius, 1)
			raised.routeCasing(points, radius, 1)
			if len(base.triangles) != len(raised.triangles) || base.err != nil || raised.err != nil {
				t.Fatal("casing clearance changed route tessellation")
			}
			upperBridgeVertices := 0
			for i, triangle := range raised.triangles {
				for j, vertex := range triangle.V {
					if vertex.Position.Y < raised.routeCasingFloor-1e-12 {
						t.Fatalf("casing is hidden inside the deepest container: radius=%g y=%g", radius, vertex.Position.Y)
					}
					previous := base.triangles[i].V[j]
					if previous.Position.Y > raised.routeCasingFloor {
						if vertex != previous {
							t.Fatal("clear casing or upper bridge geometry changed")
						}
						upperBridgeVertices++
					}
					if len(points) == 2 && vertex.Position.Y >= .08 {
						t.Fatal("paper casing reached the colored route plane")
					}
				}
			}
			if len(points) == 3 && upperBridgeVertices == 0 {
				t.Fatal("crossing bridge lost its clearance")
			}
			if !reflect.DeepEqual(points, before) {
				t.Fatal("casing clearance mutated compiled route points")
			}
		}
	}
}

func TestHierarchyCasingFloorOnlyUsesVisibleRaisedCaps(t *testing.T) {
	nodes := map[string]*d2isometric.Node{
		"root":       {Label: "root", Fill: "#eef0fc", Opacity: 1},
		"child":      {Label: "child", Fill: "#eef0fc", Opacity: 1},
		"grandchild": {Label: "grandchild", Fill: "#eef0fc", Opacity: 1},
	}
	boards := []d2isometric.Board{
		{ID: "root", SourceID: "root", Kind: "platform", Size: nv(3, .14, 2)},
		{ID: "child", SourceID: "child", Kind: "group", ParentID: "root", Level: 1, Size: nv(2, .14, 1)},
		{ID: "grandchild", SourceID: "grandchild", Kind: "group", ParentID: "child", Level: 2, Size: nv(1, .14, .5)},
	}
	if hierarchyCasingFloor(hierarchyPresentationBoards(boards[:1], nodes), nodes) != 0 {
		t.Fatal("ordinary platform scene changed existing casing geometry")
	}
	presented := hierarchyPresentationBoards(boards, nodes)
	floor := hierarchyCasingFloor(presented, nodes)
	if math.Abs(floor-hierarchySurfaceY(presented[2])-.0005) > 1e-12 || floor >= .08 {
		t.Fatalf("casing does not track actual deepest cap: %g", floor)
	}
	// A mixed sequence root retains its existing small upward relief. Those
	// caps need the same clearance as downward support terraces.
	presented[2].Kind, presented[2].Position.Y = "group", .03
	if got := hierarchyCasingFloor(presented, nodes); math.Abs(got-hierarchySurfaceY(presented[2])-.0005) > 1e-12 {
		t.Fatal("existing shallow relief lost its casing clearance")
	}
	nodes["child"].StrokeDash = 3
	nodes["grandchild"].Fill = "transparent"
	if hierarchyCasingFloor(hierarchyPresentationBoards(boards, nodes), nodes) != 0 {
		t.Fatal("dashed or transparent regions raised decorative route casing")
	}
}
