package d2isometricimg

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestNativeComponentRetainsLayoutFootprint(t *testing.T) {
	for _, size := range []Vec{nv(2.4, .7, 1.2), nv(.01, .7, .02), nv(25, .9, 13)} {
		b := &meshBuilder{ctx: context.Background(), scale: .01}
		n := d2isometric.Node{Position: nv(3, size.Y/2, 7), Size: size, Type: "rectangle", Fill: "#abcdef", FillExplicit: true, Opacity: 1}
		b.node(n, "#7898ba")
		if b.err != nil {
			t.Fatal(b.err)
		}
		low, high := nv(math.Inf(1), math.Inf(1), math.Inf(1)), nv(math.Inf(-1), math.Inf(-1), math.Inf(-1))
		for _, tri := range b.triangles {
			// Inspect physical sidewalls, independent of art-direction material
			// constants or the separately printed cap texture and source ink.
			if tri.Material.Texture != nil || tri.Material.Unlit || math.Abs(tri.V[0].Normal.Y) > .5 {
				continue
			}
			for _, v := range tri.V {
				p := v.Position
				low.X = min(low.X, p.X)
				low.Z = min(low.Z, p.Z)
				high.X = max(high.X, p.X)
				high.Z = max(high.Z, p.Z)
				if math.Abs(nlen(v.Normal)-1) > 1e-8 {
					t.Fatal("invalid surface normal")
				}
			}
		}
		if math.Abs(high.X-low.X-size.X) > 1e-8 || math.Abs(high.Z-low.Z-size.Z) > 1e-8 || math.Abs((high.X+low.X)/2-3) > 1e-8 || math.Abs((high.Z+low.Z)/2-7) > 1e-8 {
			t.Fatalf("source footprint changed: size=%+v low=%+v high=%+v", size, low, high)
		}
	}
}

func TestNativeRouteRemainsFlatAndBounded(t *testing.T) {
	points := []Vec{nv(0, .08, 0), nv(0, .08, 0), nv(3, .08, 0), nv(3, .08, 2), nv(5, .08, 2)}
	route := nativeRoundedRoute(points)
	for i, p := range route {
		if p.Y != .08 || p.X < 0 || p.X > 5 || p.Z < 0 || p.Z > 2 {
			t.Fatalf("route left its flat bounds: %+v", p)
		}
		if i > 0 && nlen(nsub(p, route[i-1])) < 1e-10 {
			t.Fatal("duplicate tessellation point")
		}
	}
	lengths := routeLengths(route)
	if pathPoint(route, lengths, 0) != points[0] || pathPoint(route, lengths, 1) != points[len(points)-1] {
		t.Fatal("route endpoint moved")
	}
	b := &meshBuilder{ctx: context.Background()}
	b.arrow("triangle", nv(1, .08, 2), nv(1, 0, 0), nativeMaterial("#657e9e", .3, .3, 1))
	for _, tri := range b.triangles {
		for _, v := range tri.V {
			if v.Position.Y < .079 || v.Position.Y > .106 {
				t.Fatal("arrow left the connection plane")
			}
		}
	}
}
