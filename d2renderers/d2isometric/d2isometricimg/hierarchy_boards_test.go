package d2isometricimg

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestHierarchyReliefPreservesFootprintsAndSurfaceTextures(t *testing.T) {
	n := d2isometric.Node{Size: nv(3, .7, 2), Position: nv(5, .42, 6), Opacity: 1}
	base, relief := &meshBuilder{ctx: context.Background()}, &meshBuilder{ctx: context.Background()}
	base.node(n, "#8ba4be")
	relief.hierarchyNode(n, "#8ba4be")
	if base.err != nil || relief.err != nil || len(base.triangles) != len(relief.triangles) {
		t.Fatalf("relief changed content: %v / %v", base.err, relief.err)
	}
	for i := range base.triangles {
		for j, a := range base.triangles[i].V {
			b := relief.triangles[i].V[j]
			floor := n.Position.Y - n.Size.Y/2
			if a.Position.X != b.Position.X || a.Position.Z != b.Position.Z || a.U != b.U || a.V != b.V || math.Abs(b.Position.Y-(floor+(a.Position.Y-floor)*hierarchyRelief)) > 1e-10 {
				t.Fatal("relief changed horizontal layout or texture coordinates")
			}
			if math.Abs(nlen(b.Normal)-1) > 1e-10 {
				t.Fatal("relief lost normalized lighting normals")
			}
		}
	}
	copy := hierarchyRenderNodes([]d2isometric.Node{n})
	if n.Size.Y != .7 || copy[0].Size.X != n.Size.X || copy[0].Size.Z != n.Size.Z || copy[0].Size.Y != n.Size.Y*hierarchyRelief {
		t.Fatal("header projection changed source footprint or input")
	}
}

func TestHierarchyHeaderContrastUsesAuthoredGround(t *testing.T) {
	board := d2isometric.Board{ID: "system", SourceID: "system", Kind: "platform"}
	nodes := map[string]*d2isometric.Node{"system": {Label: "System", Fill: "transparent", Opacity: 1}}
	for _, c := range []struct{ background, ink string }{{"#101827", "#f5f8fc"}, {"#f5f7fb", "#253650"}} {
		if got := hierarchyHeaderInk(board, map[string]d2isometric.Board{"system": board}, nodes, map[string]string{"system": "transparent"}, c.background); got != c.ink {
			t.Fatalf("header on %s got %s, want %s", c.background, got, c.ink)
		}
	}
}

func TestHierarchySurfacesStayFlatInsideOriginalFootprints(t *testing.T) {
	owner := &d2isometric.Node{Stroke: "#476a80", StrokeWidth: 2}
	board := d2isometric.Board{Kind: "group", Level: 3, Position: nv(4, 0, 7), Size: nv(6, .14, 3)}
	for _, dashed := range []float64{0, 5} {
		owner.StrokeDash = dashed
		b := &meshBuilder{ctx: context.Background()}
		b.hierarchyBoard(board, owner, "#b8d2df", 1)
		if b.err != nil || len(b.triangles) == 0 {
			t.Fatalf("missing region: %v", b.err)
		}
		for _, triangle := range b.triangles {
			if triangle.CastShadow {
				t.Fatal("organizational region casts a slab shadow")
			}
			for _, v := range triangle.V {
				p := v.Position
				// Source stroke ink extends beyond its centerline footprint.
				margin := float64(owner.StrokeWidth)*.01 + .00001
				if p.Y < .028 || p.Y >= .04 || p.X < 1-margin || p.X > 7+margin || p.Z < 5.5-margin || p.Z > 8.5+margin {
					t.Fatalf("paint leaves source footprint or plane: %+v", p)
				}
			}
		}
	}
}

func TestHierarchyPlatformsAndInvisibleWrappers(t *testing.T) {
	board := d2isometric.Board{Kind: "platform", Size: nv(5, .14, 4)}
	owner := &d2isometric.Node{Stroke: "#476a80", StrokeWidth: 2}
	b := &meshBuilder{ctx: context.Background()}
	b.hierarchyBoard(board, owner, "#d9e5ef", 1)
	hasSolid, hasShadow := false, false
	for _, triangle := range b.triangles {
		for _, v := range triangle.V {
			hasSolid = hasSolid || v.Position.Y < 0
		}
		if triangle.CastShadow {
			hasShadow = true
			if triangle.ShadowOpacity == nil || *triangle.ShadowOpacity <= 0 || *triangle.ShadowOpacity >= 1 {
				t.Fatal("platform lost its restrained physical shadow")
			}
		}
	}
	if !hasSolid || !hasShadow {
		t.Fatal("outer system lost its solid platform or physical shadow")
	}
	for _, kind := range []string{"platform", "group", "ungrouped"} {
		board.Kind = kind
		owner.Stroke = "transparent"
		b := &meshBuilder{ctx: context.Background()}
		b.hierarchyBoard(board, owner, "transparent", 1)
		if len(b.triangles) != 0 {
			t.Fatalf("invisible %s became hardware", kind)
		}
	}
}

func TestHierarchySpacerKeepsMeaningfulContent(t *testing.T) {
	n := d2isometric.Node{Fill: "transparent", Stroke: "transparent", StrokeWidth: 2}
	if !hierarchySpacer(n) {
		t.Fatal("invisible layout spacer remains a module")
	}
	n.Label = "Boundary"
	if hierarchySpacer(n) {
		t.Fatal("named transparent component was hidden")
	}
}
