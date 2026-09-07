package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/d2lang/d2/d2target"
)

func nativeInkUVContains(t Triangle, u, v float64) bool {
	a, b, c := t.V[0], t.V[1], t.V[2]
	det := (b.V-c.V)*(a.U-c.U) + (c.U-b.U)*(a.V-c.V)
	if math.Abs(det) < 1e-15 {
		return false
	}
	x := ((b.V-c.V)*(u-c.U) + (c.U-b.U)*(v-c.V)) / det
	y := ((c.V-a.V)*(u-c.U) + (a.U-c.U)*(v-c.V)) / det
	return min(x, y, 1-x-y) >= -1e-9
}

// Check the actual cap triangles, not the fit helper's calculated radius. The
// old centerline UV mapping drops a substantial outer half of this pixel set.
func TestNativeDecoratedSolidCapPreservesWholeCenteredStroke(t *testing.T) {
	for _, kind := range nativeSolidTestKinds {
		for _, size := range [][2]int{{240, 160}, {600, 24}} {
			t.Run(fmt.Sprintf("%s/%dx%d", kind, size[0], size[1]), func(t *testing.T) {
				n := solidTestNode(kind)
				n.Size.X, n.Size.Z = float64(size[0])*.01, float64(size[1])*.01
				n.Metadata.Original.Width, n.Metadata.Original.Height = size[0], size[1]
				n.Stroke, n.Metadata.Original.Stroke = "#e42683", "#e42683"
				n.StrokeWidth, n.Metadata.Original.StrokeWidth = 1, 1
				n.StrokeDash, n.Metadata.Original.StrokeDash = 2, 2
				b := solidTestBuild(t, n)
				var ink []Triangle
				var texture *image.RGBA
				for _, tri := range b.triangles {
					if tri.Material.Unlit && tri.Material.Texture != nil {
						ink = append(ink, tri)
						texture = tri.Material.Texture.(*image.RGBA)
						if tri.CastShadow || tri.DepthBias <= 0 {
							t.Fatal("printed ink casts a second physical shadow or lacks depth separation")
						}
					}
				}
				if len(ink) == 0 || texture == nil {
					t.Fatal("missing independent, unlit authored outline")
				}
				// Queue repeats this texture on both physical end caps.
				seen := 0
				bounds := texture.Bounds()
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						if texture.RGBAAt(x, y).A == 0 {
							continue
						}
						seen++
						for _, corner := range [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
							u := (float64(x-bounds.Min.X) + corner[0]) / float64(bounds.Dx())
							v := (float64(y-bounds.Min.Y) + corner[1]) / float64(bounds.Dy())
							covered := false
							for _, tri := range ink {
								if nativeInkUVContains(tri, u, v) {
									covered = true
									break
								}
							}
							if !covered {
								t.Fatalf("authored outline clipped at texture pixel (%d,%d), uv=(%g,%g)", x, y, u, v)
							}
						}
					}
				}
				if seen < 20 {
					t.Fatal("fixture did not produce an actual outline")
				}
			})
		}
	}
}

func TestNativeFaceSeparatesAuthoredInkFromLitFill(t *testing.T) {
	n := fidelityNode(d2target.ShapeOval)
	n.Stroke, n.Metadata.Original.Stroke = "#e42683", "#e42683"
	n.StrokeWidth, n.Metadata.Original.StrokeWidth = 7, 7
	s := nativeFaceSource(n, "#9db6d3")
	b := &meshBuilder{ctx: context.Background(), scale: .01, faceMaxPixels: 128 << 10}
	fill, ink, area := b.nativeFaceLayers(s)
	if b.err != nil || fill == nil || ink == nil || fill.Bounds() != ink.Bounds() || area.width <= float64(s.Width) {
		t.Fatalf("native paint layers lost full stroke viewport: %v / %+v", b.err, area)
	}
	colored := 0
	for y := ink.Bounds().Min.Y; y < ink.Bounds().Max.Y; y++ {
		for x := ink.Bounds().Min.X; x < ink.Bounds().Max.X; x++ {
			c := color.NRGBAModel.Convert(ink.At(x, y)).(color.NRGBA)
			if c.A > 250 {
				if math.Abs(float64(c.R)-228) > 2 || math.Abs(float64(c.G)-38) > 2 || math.Abs(float64(c.B)-131) > 2 {
					t.Fatalf("outline contains fill color instead of authored ink: %+v", c)
				}
				colored++
			}
		}
	}
	if colored == 0 || ink.RGBAAt(ink.Bounds().Dx()/2, ink.Bounds().Dy()/2).A != 0 {
		t.Fatal("outline layer contains a filled substrate or has no authored stroke")
	}
	center := color.NRGBAModel.Convert(fill.At(fill.Bounds().Dx()/2, fill.Bounds().Dy()/2)).(color.NRGBA)
	if center != (color.NRGBA{157, 182, 211, 255}) {
		t.Fatalf("cap material lost its source fill: %+v", center)
	}
	if b.facePixels > 128<<10 {
		t.Fatal("splitting fill and ink exceeded the node texture share")
	}
}

func TestNativeCanonicalInkIsPrintedOverContinuousMatteSides(t *testing.T) {
	n := fidelityNode(d2target.ShapeDocument)
	n.StrokeDash, n.Metadata.Original.StrokeDash = 2, 2
	b := &meshBuilder{ctx: context.Background(), scale: .01}
	b.node(n, "#849ebc")
	if b.err != nil {
		t.Fatal(b.err)
	}
	lit, ink := 0, 0
	var side *Material
	for _, tri := range b.triangles {
		m := tri.Material
		if m.Texture != nil {
			if m.Unlit {
				ink++
			} else {
				lit++
			}
			continue
		}
		if m.Unlit {
			continue // The shared object outline is separate from the side material.
		}
		if side == nil {
			side = m
		} else if m != side {
			t.Fatal("canonical sidewall still has an unrelated decorative bottom stripe")
		}
		if m.Unlit || m.Roughness != .68 || m.Metalness != 0 {
			t.Fatal("canonical sidewall lost its matte physical material")
		}
	}
	if lit != 2 || ink != 2 || side == nil {
		t.Fatalf("missing independent cap fill, source ink, or continuous sidewall: %d / %d", lit, ink)
	}
}

func TestNativeSolidInkFitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &meshBuilder{ctx: ctx}
	b.solidInkFit(image.NewRGBA(image.Rect(0, 0, 32, 32)), false)
	if b.err != context.Canceled {
		t.Fatal("ink fitting ignored cancellation")
	}
}

func TestNativeSeparatedInkPreservesSourceGroupOpacity(t *testing.T) {
	n := fidelityNode(d2target.ShapeOval)
	n.StrokeWidth = 7
	s := nativeFaceSource(n, n.Fill)
	plain := &meshBuilder{ctx: context.Background(), scale: .01}
	fill, ink, _ := plain.nativeFaceLayers(s)
	const opacity = .35
	faded := &meshBuilder{ctx: context.Background(), scale: .01}
	lower, upper, _ := faded.nativeFaceLayers(s, opacity)
	if plain.err != nil || faded.err != nil {
		t.Fatalf("face paint failed: %v / %v", plain.err, faded.err)
	}
	checked := 0
	for y := fill.Bounds().Min.Y; y < fill.Bounds().Max.Y; y++ {
		for x := fill.Bounds().Min.X; x < fill.Bounds().Max.X; x++ {
			f, i := float64(fill.RGBAAt(x, y).A)/255, float64(ink.RGBAAt(x, y).A)/255
			if f == 0 || i == 0 {
				continue
			}
			lo, hi := float64(lower.RGBAAt(x, y).A)/255*opacity, float64(upper.RGBAAt(x, y).A)/255*opacity
			got, want := hi+lo*(1-hi), opacity*(i+f*(1-i))
			if math.Abs(got-want) > 1./255 {
				t.Fatalf("fill/ink overlap applied group opacity twice: alpha=%g want %g", got, want)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("fixture has no overlapping fill and source outline")
	}
}

func TestNativeRoundedSidewallsSmoothWithoutMovingContour(t *testing.T) {
	n := fidelityNode(d2target.ShapeRectangle)
	n.Metadata.Original.BorderRadius = 25
	profile, err := nativeShapeProfile(nativeFaceSource(n, n.Fill))
	if err != nil {
		t.Fatal(err)
	}
	for _, reversed := range []bool{false, true} {
		p := append([]Vec(nil), profile...)
		for i := range p {
			p[i].Y = 1
		}
		if reversed {
			for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
				p[i], p[j] = p[j], p[i]
			}
		}
		b := &meshBuilder{ctx: context.Background()}
		b.extrudedProfile(p, 0, nil, nativeMaterial(n.Fill, .55, .04, 1))
		if b.err != nil || len(b.triangles) != 2*len(p) {
			t.Fatalf("sidewall tessellation changed: %v", b.err)
		}
		allowed := map[Vec]bool{}
		for _, point := range p {
			allowed[point] = true
			point.Y = 0
			allowed[point] = true
		}
		normals := map[Vec]Vec{}
		curved := 0
		for _, tri := range b.triangles {
			geometric := ncross(nsub(tri.V[1].Position, tri.V[0].Position), nsub(tri.V[2].Position, tri.V[0].Position))
			for _, v := range tri.V {
				if !allowed[v.Position] || math.Abs(nlen(v.Normal)-1) > 1e-9 || ndot(geometric, v.Normal) <= 0 {
					t.Fatal("normal smoothing changed source contour or outward lighting")
				}
				if previous, ok := normals[v.Position]; ok && nlen(nsub(previous, v.Normal)) > 1e-9 {
					t.Fatal("rounded contour retains a visible facet boundary")
				}
				normals[v.Position] = v.Normal
				if math.Abs(v.Normal.X) > .05 && math.Abs(v.Normal.Z) > .05 {
					curved++
				}
			}
		}
		if curved < 16 {
			t.Fatal("fixture did not produce curved sidewall shading")
		}
	}
}

func TestNativeSidewallsKeepSharpAndConcaveCorners(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		profile []Vec
		corner  Vec
	}{
		{"square", []Vec{nv(0, 1, 0), nv(2, 1, 0), nv(2, 1, 2), nv(0, 1, 2)}, nv(2, 1, 2)},
		{"shallow-concave", []Vec{nv(0, 1, 0), nv(2, 1, 0), nv(2, 1, 2), nv(1, 1, 1.8), nv(0, 1, 2)}, nv(1, 1, 1.8)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			b := &meshBuilder{ctx: context.Background()}
			b.extrudedProfile(fixture.profile, 0, nil, nativeMaterial("#aabbcc", .55, .04, 1))
			normals := map[Vec]bool{}
			for _, tri := range b.triangles {
				for _, vertex := range tri.V {
					if vertex.Position == fixture.corner {
						normals[vertex.Normal] = true
					}
				}
			}
			if len(normals) != 2 {
				t.Fatal("real corner was incorrectly rounded by shared normals")
			}
		})
	}
}
