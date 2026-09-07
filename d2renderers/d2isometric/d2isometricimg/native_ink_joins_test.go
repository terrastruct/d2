package d2isometricimg

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"math"
	"testing"
)

func TestClassicInkBranchCornersHaveContinuousCoverage(t *testing.T) {
	ink := color.RGBA{223, 16, 64, 255}
	for _, height := range []float64{2, .045} {
		for _, width := range []int{2, 4, 8} {
			t.Run(fmt.Sprintf("height=%g/width=%d", height, width), func(t *testing.T) {
				n := classicInkTestNode()
				n.Stroke, n.StrokeWidth = "#df1040", width
				b := &meshBuilder{ctx: context.Background(), scale: .01}
				center := nv(7.25, .4+height/2, -3.5)
				b.box(center, nv(2, height, 2), nativeMaterial("white", 1, 0, 1), 0)
				b.classicInkEdges(n, b.triangles)
				if b.err != nil {
					t.Fatal(b.err)
				}
				radius := float64(width) * .01 / 2
				for _, side := range []float64{-1, 1} {
					// The two extreme upper cuboid corners each meet three
					// structural edges. Their silhouette rays have a common
					// exterior wedge that separate butt caps cannot cover.
					corner := nadd(center, nv(side, height/2, -side))
					camera := nativeCameraAxes()
					camera.width, camera.height, camera.scale = 256, 256, 12/radius
					camera.centerX, camera.centerY = ndot(corner, camera.right), ndot(corner, camera.up)
					flatRay := nv(-1, 0, 0)
					if side < 0 {
						flatRay = nv(0, 0, -1)
					}
					// Offset-line intersection gives the expected sharp outline,
					// independently of production graph tracing or join patches.
					a := nmul(camera.right, side)
					c := nmul(nunit(ncross(camera.direction, flatRay)), -side)
					miter := nmul(nadd(a, c), radius/(1+ndot(a, c)))
					if nlen(miter) > 2*radius {
						t.Fatal("fixture unexpectedly requires an acute bevel")
					}
					r, err := newRaster(b.ctx, 128, 128, b.triangles, color.NRGBA{255, 255, 255, 255}, &camera, nil)
					if err != nil {
						t.Fatal(err)
					}
					for _, fraction := range []float64{.45, .65} {
						// Probe inside the missing wedge, not just on an edge's
						// centerline (which already passes with disconnected caps).
						p := r.camera.project(nadd(corner, nmul(miter, fraction)))
						if got := r.pixels.RGBAAt(int(p.x), int(p.y)); got != ink {
							t.Errorf("side=%g: exterior corner wedge is not ink at fraction %g: %v", side, fraction, got)
						}
					}
					// Filling a corner must not enlarge its outline arbitrarily.
					outside := nadd(corner, nmul(nunit(miter), 2.15*radius))
					p := r.camera.project(outside)
					if got := r.pixels.RGBAAt(int(p.x), int(p.y)); got == ink {
						t.Errorf("side=%g: ink extends beyond the bounded corner", side)
					}
				}
			})
		}
	}
}

func TestClassicInkBranchJoinsAreBoundedAndRespectPaintedEnds(t *testing.T) {
	camera := nativeCameraAxes()
	paint := &Material{Color: color.NRGBA{223, 16, 64, 255}, Unlit: true, svgContour: true}
	const radius = .02
	for _, spread := range []float64{.02, .8, 2.8} {
		t.Run(fmt.Sprintf("spread=%g", spread), func(t *testing.T) {
			var segments []classicInkSegment
			for _, angle := range []float64{0, spread / 2, spread} {
				direction := nadd(nmul(camera.right, math.Cos(angle)), nmul(camera.up, math.Sin(angle)))
				segments = append(segments, classicInkSegment{a: Vec{}, c: direction})
			}
			for _, count := range []int{0, 1} {
				b := &meshBuilder{ctx: context.Background()}
				b.classicInkJunctions(segments, segments[:count], radius, paint)
				if b.err != nil || len(b.triangles) != 0 {
					t.Fatalf("%d painted endpoint(s) generated a join: %v", count, b.err)
				}
			}
			// A dash starting beyond the vertex is not an active corner end.
			shifted := append([]classicInkSegment(nil), segments...)
			for i := range shifted {
				shifted[i].a = nlerp(shifted[i].a, shifted[i].c, .01)
			}
			b := &meshBuilder{ctx: context.Background()}
			b.classicInkJunctions(segments, shifted, radius, paint)
			if b.err != nil || len(b.triangles) != 0 {
				t.Fatal("join filled an unpainted dash gap", b.err)
			}
			b.classicInkJunctions(segments, segments, radius, paint)
			if b.err != nil || len(b.triangles) == 0 {
				t.Fatal("painted branches did not produce a corner join", b.err)
			}
			for _, triangle := range b.triangles {
				for _, v := range triangle.V {
					p := v.Position
					distance := math.Hypot(ndot(p, camera.right), ndot(p, camera.up))
					if !captionFinite(p.X, p.Y, p.Z) || distance > 2*radius+1e-9 || math.Abs(p.Y) > 1e-9 {
						t.Fatalf("join changed height or exceeded its projected bound: %+v", p)
					}
				}
			}
			// Source traversal order must not change the visible joint.
			for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
				segments[i], segments[j] = segments[j], segments[i]
			}
			reversed := &meshBuilder{ctx: context.Background()}
			reversed.classicInkJunctions(segments, segments, radius, paint)
			if reversed.err != nil {
				t.Fatal(reversed.err)
			}
			camera.width, camera.height, camera.scale = 128, 128, 1000
			first, err := newRaster(b.ctx, 64, 64, b.triangles, color.NRGBA{255, 255, 255, 255}, &camera, nil)
			if err != nil {
				t.Fatal(err)
			}
			second, err := newRaster(b.ctx, 64, 64, reversed.triangles, color.NRGBA{255, 255, 255, 255}, &camera, nil)
			if err != nil || !bytes.Equal(first.output.Pix, second.output.Pix) {
				t.Fatal("source traversal changed branch-join coverage", err)
			}
		})
	}
}

func TestClassicInkBranchJoinKeepsStraightStrokeWidth(t *testing.T) {
	n := classicInkTestNode()
	n.Stroke, n.StrokeWidth = "#df1040", 4
	b := classicInkTestCube(false)
	b.classicInkEdges(n, b.triangles)
	if b.err != nil {
		t.Fatal(b.err)
	}
	camera := nativeCameraAxes()
	camera.width, camera.height, camera.scale = 256, 256, 1000
	mid := nv(1, 0, -1)
	camera.centerX, camera.centerY = ndot(mid, camera.right), ndot(mid, camera.up)
	r, err := newRaster(b.ctx, 128, 128, b.triangles, color.NRGBA{255, 255, 255, 255}, &camera, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, distance := range []float64{-.018, .018, .025} {
		p := r.camera.project(nadd(mid, nmul(camera.right, distance)))
		gotInk := r.pixels.RGBAAt(int(p.x), int(p.y)) == (color.RGBA{223, 16, 64, 255})
		if gotInk != (math.Abs(distance) < .02) {
			t.Errorf("straight stroke coverage at offset %g: ink=%v", distance, gotInk)
		}
	}
}
