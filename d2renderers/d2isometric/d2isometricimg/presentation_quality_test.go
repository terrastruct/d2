package d2isometricimg

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestMatteLightingPreservesTopColorAndRevealsSidewalls(t *testing.T) {
	r := &Raster{camera: nativeCameraAxes()}
	for _, fill := range []string{"#e5edf7", "#5b91b7", "#eaafc9"} {
		material := nativeMaterial(fill, .55, .04, 1)
		paint := rasterMaterial(material)
		r0, g0, b0 := r.shade(paint, Vec{}, nv(0, 1, 0))
		r1, g1, b1 := r.shade(paint, Vec{}, nv(1, 0, 0))
		for i, values := range [][2]float64{{r0, float64(material.Color.R) / 255}, {g0, float64(material.Color.G) / 255}, {b0, float64(material.Color.B) / 255}} {
			if math.Abs(values[0]-values[1]) > .05 {
				t.Fatalf("%s channel %d top paint is bleached or darkened: %v", fill, i, values)
			}
		}
		if (r0+g0+b0)-(r1+g1+b1) < .25 {
			t.Fatalf("%s sidewall has too little contrast to establish volume", fill)
		}
	}
}

func TestPrintedRouteInkKeepsColorAndPlaneThroughTurns(t *testing.T) {
	points := []Vec{nv(0, .08, 0), nv(2, .08, 0), nv(2, .08, 2), nv(4, .08, 3)}
	mat := nativeMaterial("#36776b", .6, .05, .7)
	mat.Unlit = true
	b := &meshBuilder{ctx: context.Background()}
	b.routeInk(points, .02, mat)
	r := &Raster{camera: nativeCameraAxes()}
	for _, tri := range b.triangles {
		if tri.CastShadow || !tri.Material.Unlit || tri.Material != mat {
			t.Fatal("route ink acquired physical shading or changed authored paint")
		}
		for _, vertex := range tri.V {
			if vertex.Position.Y != .08 {
				t.Fatal("flat source route acquired raised geometry")
			}
			red, green, blue := r.shade(rasterMaterial(mat), vertex.Position, vertex.Normal)
			if math.Abs(red-float64(mat.Color.R)/255) > 1e-10 || math.Abs(green-float64(mat.Color.G)/255) > 1e-10 || math.Abs(blue-float64(mat.Color.B)/255) > 1e-10 {
				t.Fatal("ink color varies with route direction")
			}
		}
	}
}

func TestReliefSymbolsStayInsideAuthoredContourBounds(t *testing.T) {
	for _, kind := range []string{d2target.ShapePerson, d2target.ShapeC4Person, d2target.ShapeCloud} {
		source := d2target.BaseShape()
		source.Type, source.Width, source.Height = kind, 200, 120
		node := d2isometric.Node{Type: kind, Size: nv(2, 1, 1.2), Position: nv(3, .5, 7), Fill: "#d4e5ef", FillExplicit: true, Opacity: 1, Metadata: d2isometric.NodeMetadata{Original: *source}}
		// D2's person cubic has a small authored overshoot past its box.
		// Preserve that contour rather than clipping or widening it to a base.
		profiles, err := nativeShapeProfiles(*source)
		if err != nil {
			t.Fatal(err)
		}
		minX, minZ, maxX, maxZ := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, profile := range profiles {
			for _, p := range profile {
				x := node.Position.X - node.Size.X/2 + p.X*.01
				z := node.Position.Z - node.Size.Z/2 + p.Z*.01
				minX, minZ, maxX, maxZ = min(minX, x), min(minZ, z), max(maxX, x), max(maxZ, z)
			}
		}
		b := &meshBuilder{ctx: context.Background(), scale: .01}
		b.node(node, "#849ebc")
		if b.err != nil {
			t.Fatal(b.err)
		}
		for _, tri := range b.triangles {
			// Printed texture viewports and centered ink include raster padding.
			// The physical side walls must follow the exact source footprint.
			if tri.Material == nil || tri.Material.Texture != nil || tri.Material.Unlit {
				continue
			}
			for _, vertex := range tri.V {
				if vertex.Position.X < minX-1e-9 || vertex.Position.X > maxX+1e-9 || vertex.Position.Z < minZ-1e-9 || vertex.Position.Z > maxZ+1e-9 {
					t.Fatalf("%s adds hardware or artwork outside its source footprint: %+v", kind, vertex.Position)
				}
			}
		}
	}
}
