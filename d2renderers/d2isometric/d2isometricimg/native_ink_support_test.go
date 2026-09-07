package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"testing"
)

func TestClassicInkCurvedSilhouettesKeepTheirInsideHalf(t *testing.T) {
	for _, kind := range []string{"cylinder", "queue", "page"} {
		for _, scale := range []float64{.006, .01, .03} {
			for _, width := range []int{2, 6} {
				t.Run(fmt.Sprintf("%s/scale=%g/width=%d", kind, scale, width), func(t *testing.T) {
					n := fidelityNode(kind)
					n.Size, n.Position = nmul(n.Size, scale/.01), nmul(n.Position, scale/.01)
					n.StrokeWidth, n.Stroke = width, "#df1040"
					b := &meshBuilder{ctx: context.Background(), scale: scale}
					b.node(n, "")
					if b.err != nil {
						t.Fatal(b.err)
					}
					var physical, ink []Triangle
					for _, tr := range b.triangles {
						if tr.Material.svgContour {
							ink = append(ink, tr)
						} else if tr.CastShadow && (tr.Material.Texture == nil || nativeSolidNode(n)) {
							// Solid cap triangles have the actual outline. Canonical
							// textured page viewports also include transparent margins.
							physical = append(physical, tr)
						}
					}
					segments, err := classicInkSegments(b.ctx, n, physical)
					if err != nil {
						t.Fatal(err)
					}
					camera := nativeCameraAxes()
					project := func(p Vec) Vec {
						return nv(ndot(p, camera.right), ndot(p, camera.up), ndot(p, camera.direction))
					}
					depth := func(triangles []Triangle, p Vec) float64 {
						best := math.Inf(-1)
						for _, tr := range triangles {
							f := classicInkFacet{points: [3]Vec{project(tr.V[0].Position), project(tr.V[1].Position), project(tr.V[2].Position)}, bias: tr.DepthBias}
							if d, ok := f.depthAt(p); ok {
								best = max(best, d)
							}
						}
						return best
					}
					probes := 0
					for _, s := range segments {
						for _, fraction := range []float64{.2, .5, .8} {
							at := nlerp(s.a, s.c, fraction)
							p := project(at)
							if depth(physical, p) > p.Z+classicInkDepthBias {
								continue // Another part of the object hides this edge.
							}
							side := nunit(ncross(camera.direction, nsub(s.c, s.a)))
							for _, sign := range []float64{-1, 1} {
								// Centerline-only checks miss a half-width notch. Probe
								// well inside both halves of the authored stroke instead.
								probe := project(nadd(at, nmul(side, sign*.3*float64(width)*scale)))
								bodyDepth, inkDepth := depth(physical, probe), depth(ink, probe)
								if math.IsInf(bodyDepth, -1) {
									continue
								}
								probes++
								if bodyDepth > inkDepth+1e-7 {
									t.Fatalf("inside stroke half was hidden at %.1f of %v..%v: body depth=%g ink depth=%g", fraction, s.a, s.c, bodyDepth, inkDepth)
								}
							}
						}
					}
					if probes < 30 {
						t.Fatalf("fixture only exercises %d visible inside-half probes", probes)
					}
				})
			}
		}
	}
}

func TestClassicInkSupportCannotClimbOntoAnOppositeCap(t *testing.T) {
	n := classicInkTestNode()
	n.Type = "queue"
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	material := nativeMaterial("white", 1, 0, 1)
	// A short barrel makes the near cap overlap the rear rim's stroke bounds.
	// Its wall shares vertices with both caps, but the sharp rim separates
	// the supporting surfaces. Ink on the rear cannot climb onto the front.
	b.solidBarrel(Vec{}, .08, 1.2, .7, nativeSolidPaint{cap: material, wall: material})
	segments, err := classicInkSegments(b.ctx, n, b.triangles, .08)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, s := range segments {
		if math.Abs(s.a.X+.04) > 1e-9 || math.Abs(s.c.X+.04) > 1e-9 {
			continue
		}
		checked++
		for _, face := range s.support {
			if face.normal.X > .999999 {
				t.Fatal("rear outline acquired support on the opposite cap")
			}
		}
	}
	if checked == 0 {
		t.Fatal("fixture has no rear rim")
	}
}
