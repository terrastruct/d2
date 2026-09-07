package d2isometricimg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

var nativeSolidTestKinds = []string{d2target.ShapeCylinder, d2target.ShapeQueue, d2target.ShapeCircle, d2target.ShapeOval, d2target.ShapeHexagon}

func solidTestNode(kind string) d2isometric.Node {
	n := fidelityNode(kind)
	n.Metadata.Original.Width, n.Metadata.Original.Height = 240, 160
	n.Size = nv(2.4, .85, 1.6)
	if kind == d2target.ShapeCylinder {
		n.Size.Y = 1.15
	}
	n.Position.Y = .07 + n.Size.Y/2
	return n
}

func solidTestBuild(t *testing.T, n d2isometric.Node) *meshBuilder {
	t.Helper()
	p, _ := newTextPainter(context.Background(), 2)
	p.configureOutputDensity(100)
	ic, err := newSurfaceIconPainter(context.Background(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ic.configureOutputDensity(100)
	b := &meshBuilder{ctx: context.Background(), scale: .01, outputDensity: 100, faceMaxPixels: 128 << 10, text: p, icons: ic}
	b.node(n, "#849ebc")
	if b.err != nil {
		t.Fatal(b.err)
	}
	return b
}

func solidTestBounds(ts []Triangle) (Vec, Vec) {
	lo, hi := nv(math.Inf(1), math.Inf(1), math.Inf(1)), nv(math.Inf(-1), math.Inf(-1), math.Inf(-1))
	for _, tri := range ts {
		for _, v := range tri.V {
			p := v.Position
			lo = nv(min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z))
			hi = nv(max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z))
		}
	}
	return lo, hi
}

func solidTestPhysical(ts []Triangle) []Triangle {
	var physical []Triangle
	for _, triangle := range ts {
		if triangle.Material != nil && !triangle.Material.Unlit {
			physical = append(physical, triangle)
		}
	}
	return physical
}

func TestNativeSolidsKeepGeometryAcrossPaintStyles(t *testing.T) {
	for _, kind := range nativeSolidTestKinds {
		t.Run(kind, func(t *testing.T) {
			n := solidTestNode(kind)
			plain := solidTestBuild(t, n)
			physical := solidTestPhysical(plain.triangles)
			changes := []func(*d2isometric.Node){
				func(n *d2isometric.Node) { n.Metadata.Original.FillPattern = "lines" },
				func(n *d2isometric.Node) {
					n.Stroke, n.StrokeWidth, n.StrokeDash = "#ee2981", 7, 4
					n.Metadata.Original.Stroke, n.Metadata.Original.StrokeWidth, n.Metadata.Original.StrokeDash = n.Stroke, n.StrokeWidth, n.StrokeDash
				},
				func(n *d2isometric.Node) { n.Metadata.Original.BorderRadius = 20 },
			}
			if kind == d2target.ShapeCircle || kind == d2target.ShapeOval {
				changes = append(changes, func(n *d2isometric.Node) { n.Metadata.Original.DoubleBorder = true })
			}
			for _, change := range changes {
				styled := n
				change(&styled)
				before, _ := json.Marshal(styled)
				got := solidTestBuild(t, styled)
				after, _ := json.Marshal(styled)
				if !bytes.Equal(before, after) {
					t.Fatal("solid paint mutated source metadata")
				}
				gotPhysical := solidTestPhysical(got.triangles)
				if nativeCanonicalNode(styled) || !nativeSolidNode(styled) || len(gotPhysical) != len(physical) {
					t.Fatal("surface style selected a different physical shape")
				}
				for i, tri := range gotPhysical {
					for j, v := range tri.V {
						want := physical[i].V[j]
						if v.Position != want.Position || v.Normal != want.Normal {
							t.Fatal("surface paint changed volume or curvature")
						}
					}
				}
			}
		})
	}
}

func TestNativeSolidsHaveBoundedFootprintsAndOutwardSmoothNormals(t *testing.T) {
	for _, kind := range nativeSolidTestKinds {
		for _, size := range [][2]int{{240, 160}, {240, 1}, {1, 1}} {
			t.Run(fmt.Sprintf("%s/%dx%d", kind, size[0], size[1]), func(t *testing.T) {
				n := solidTestNode(kind)
				n.Metadata.Original.Width, n.Metadata.Original.Height = size[0], size[1]
				n.Size.X, n.Size.Z = float64(size[0])*.01, float64(size[1])*.01
				b := solidTestBuild(t, n)
				physical := solidTestPhysical(b.triangles)
				lo, hi := solidTestBounds(physical)
				wantLo, wantHi := nv(n.Position.X-n.Size.X/2, .07, n.Position.Z-n.Size.Z/2), nv(n.Position.X+n.Size.X/2, .07+nativeSolidHeight(n), n.Position.Z+n.Size.Z/2)
				if nlen(nsub(lo, wantLo)) > 1e-9 || nlen(nsub(hi, wantHi)) > 1e-9 {
					t.Fatalf("source footprint/height changed: %v..%v, want %v..%v", lo, hi, wantLo, wantHi)
				}
				curved := false
				for i, tri := range physical {
					geometric := ncross(nsub(tri.V[1].Position, tri.V[0].Position), nsub(tri.V[2].Position, tri.V[0].Position))
					average := nadd(nadd(tri.V[0].Normal, tri.V[1].Normal), tri.V[2].Normal)
					if nlen(geometric) > 1e-15 && ndot(geometric, average) <= 0 {
						t.Fatalf("triangle %d winds inward relative to its lighting normals", i)
					}
					for _, v := range tri.V {
						if !captionFinite(v.Position.X, v.Position.Y, v.Position.Z, v.Normal.X, v.Normal.Y, v.Normal.Z, v.U, v.V) || math.Abs(nlen(v.Normal)-1) > 1e-9 {
							t.Fatal("invalid vertex or nonunit solid normal")
						}
						if kind == d2target.ShapeQueue {
							curved = curved || math.Abs(v.Normal.Y) > .1 && math.Abs(v.Normal.Z) > .1
						} else {
							curved = curved || math.Abs(v.Normal.X) > .1 && math.Abs(v.Normal.Z) > .1
						}
					}
				}
				if !curved && size[0] == 240 && size[1] == 160 {
					t.Fatal("solid lost curved/faceted side lighting")
				}
			})
		}
	}
}

func TestNativeSolidCrownRetainsAuthoredMaterialTone(t *testing.T) {
	n := solidTestNode(d2target.ShapeQueue)
	n.Fill, n.Metadata.Original.Fill, n.FillExplicit = "#a8c2d8", "#a8c2d8", true
	b := &meshBuilder{ctx: context.Background(), scale: .01, faceMaxPixels: 128 << 10}
	paint := b.solidPaint(n, d2target.ShapeOval, n.Fill)
	if b.err != nil {
		t.Fatal(b.err)
	}
	r := &Raster{camera: rasterCamera{direction: nunit(nativeViewDirection())}}
	shade := func(material *Material) Vec {
		p := rasterMaterial(material)
		if p.texture != nil {
			red, green, blue, _ := rasterTexture(p.texture, .5, .5)
			p.base = nv(p.base.X*rasterLinear(red), p.base.Y*rasterLinear(green), p.base.Z*rasterLinear(blue))
		}
		red, green, blue := r.shade(p, Vec{}, nv(0, 1, 0))
		return nv(red, green, blue)
	}
	// Equal source paint under equal illumination must produce equal tone.
	// This catches a baked dark multiplier on the barrel's upward-facing wall.
	if nlen(nsub(shade(paint.cap), shade(paint.wall))) > .006 {
		t.Fatal("queue crown darkens source paint before physical lighting")
	}
	n.Metadata.Original.FillPattern = "lines"
	pattern := b.solidPaint(n, d2target.ShapeOval, n.Fill)
	if b.err != nil || pattern.wall.Texture == nil {
		t.Fatal("patterned barrel lost its native surface paint", b.err)
	}
	neutral := *pattern.wall
	neutral.Color.R, neutral.Color.G, neutral.Color.B = 255, 255, 255
	if nlen(nsub(shade(pattern.wall), shade(&neutral))) > 1e-12 {
		t.Fatal("pattern artwork receives an extra baked wall tint")
	}
}

func TestNativeSolidQueueLabelIsSupportedByContinuousCrown(t *testing.T) {
	n := solidTestNode(d2target.ShapeQueue)
	plain := solidTestPhysical(solidTestBuild(t, n).triangles)
	n.Label, n.Metadata.Original.Label = "Ring\nBuffer", "Ring\nBuffer"
	n.Metadata.Original.FontSize, n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 24, 150, 58
	n.Metadata.Original.LabelPosition = "INSIDE_MIDDLE_CENTER"
	before, _ := json.Marshal(n)
	b := solidTestBuild(t, n)
	after, _ := json.Marshal(n)
	physical := solidTestPhysical(b.triangles)
	if !bytes.Equal(before, after) || len(physical) != len(plain) {
		t.Fatal("queue label altered source metadata or added a separate support panel")
	}
	lo, hi := solidTestBounds(physical)
	wantLo, wantHi := solidTestBounds(plain)
	if nlen(nsub(lo, wantLo))+nlen(nsub(hi, wantHi)) > 1e-9 {
		t.Fatal("integrated crown changed the source footprint or body height")
	}
	surface := nativeNodeLabelSurface(n, nativeFaceSource(n, n.Fill), hi.Y)
	var crownMaterial *Material
	for _, x := range []float64{-.5, 0, .5} {
		for _, z := range []float64{-.5, 0, .5} {
			p := nv(surface.center.X+x*surface.width, hi.Y, surface.center.Z+z*surface.depth)
			found := false
			for _, triangle := range physical {
				if !contactTestOnTriangle(p, triangle) {
					continue
				}
				if triangle.V[0].Normal != nv(0, 1, 0) || triangle.V[1].Normal != nv(0, 1, 0) || triangle.V[2].Normal != nv(0, 1, 0) {
					continue
				}
				crownMaterial, found = triangle.Material, true
			}
			if !found {
				t.Fatalf("compiled label corner lacks a flat physical crown: %+v", p)
			}
		}
	}
	sharedWall := false
	for _, triangle := range physical {
		if triangle.Material == crownMaterial && math.Abs(triangle.V[0].Normal.Z) > .5 {
			sharedWall = true
		}
	}
	if !sharedWall {
		t.Fatal("printed crown is a separate material patch instead of the barrel wall")
	}
}

func TestNativeSolidPatternsDashesAndDoubleBordersRemainVisible(t *testing.T) {
	for _, kind := range nativeSolidTestKinds {
		t.Run(kind, func(t *testing.T) {
			var hashes [][32]byte
			styles := []string{"plain", "pattern", "dash"}
			if kind == d2target.ShapeCircle || kind == d2target.ShapeOval {
				styles = append(styles, "double")
			}
			for _, style := range styles {
				n := solidTestNode(kind)
				switch style {
				case "pattern":
					n.Metadata.Original.FillPattern = "lines"
				case "dash":
					n.StrokeDash, n.Metadata.Original.StrokeDash = 5, 5
				case "double":
					n.Metadata.Original.DoubleBorder = true
				}
				b := solidTestBuild(t, n)
				r, err := NewRaster(context.Background(), 360, 240, b.triangles, color.NRGBA{R: 245, G: 247, B: 251, A: 255})
				if err != nil {
					t.Fatal(err)
				}
				frame, err := r.Frame(context.Background(), nil)
				if err != nil {
					t.Fatal(err)
				}
				hash := sha256.Sum256(frame.Pix)
				for _, previous := range hashes {
					if hash == previous {
						t.Fatalf("%s source paint is not visible", style)
					}
				}
				hashes = append(hashes, hash)
			}
		})
	}
}

func TestNativeSolidLabelsAndIconsKeepCompiledPrintAreas(t *testing.T) {
	for _, kind := range nativeSolidTestKinds {
		t.Run(kind, func(t *testing.T) {
			n := solidTestNode(kind)
			n.Label, n.Metadata.Original.Label = "Ring\nBuffer", "Ring\nBuffer"
			n.Metadata.Original.FontSize, n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 24, 150, 58
			n.Metadata.Original.LabelPosition = "INSIDE_MIDDLE_CENTER"
			plain := solidTestBuild(t, n)
			labelTriangles := plain.triangles[len(plain.triangles)-2:]
			lo, hi := solidTestBounds(labelTriangles)
			if math.Abs(hi.X-lo.X-1.5) > 1e-9 || math.Abs(hi.Z-lo.Z-.58) > 1e-9 || plain.text.sourceBytes != len(n.Label) {
				t.Fatalf("compiled label dimensions/content changed: %v..%v", lo, hi)
			}
			u := iconData(t, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="#ff0080"/></svg>`))
			n.Icon, n.Metadata.Original.Icon = u.String(), u
			n.Metadata.Original.IconPosition = "INSIDE_MIDDLE_LEFT"
			withIcon := solidTestBuild(t, n)
			icons, labels := 0, 0
			for _, tri := range withIcon.triangles {
				if !tri.NoDepthWrite || tri.Material.Texture == nil {
					continue
				}
				tex := tri.Material.Texture
				c := color.NRGBAModel.Convert(tex.At(tex.Bounds().Dx()/2, tex.Bounds().Dy()/2)).(color.NRGBA)
				if c.R > 250 && c.G < 5 && c.B >= 127 && c.B <= 129 {
					icons++
				} else {
					labels++
				}
				for _, v := range tri.V {
					if v.Normal != nv(0, 1, 0) || math.Abs(v.Position.X-n.Position.X) > n.Size.X/2+1e-9 || math.Abs(v.Position.Z-n.Position.Z) > n.Size.Z/2+1e-9 {
						t.Fatal("printed content left its source footprint or horizontal surface")
					}
				}
			}
			if icons != 2 || labels != 2 || withIcon.text.sourceBytes != len(n.Label) {
				t.Fatalf("content missing: icon triangles=%d label triangles=%d", icons, labels)
			}
		})
	}
	// More than 256 characters still reaches the printer as complete source.
	n := solidTestNode(d2target.ShapeCylinder)
	n.Size.X, n.Metadata.Original.Width = 30, 3000
	n.Label = strings.Repeat("Complete source label. ", 15) + "END"
	n.Metadata.Original.Label, n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = n.Label, 2900, 30
	n.Metadata.Original.FontSize, n.Metadata.Original.LabelPosition = 16, "INSIDE_MIDDLE_CENTER"
	b := solidTestBuild(t, n)
	lo, hi := solidTestBounds(b.triangles[len(b.triangles)-2:])
	if b.text.sourceBytes != len(n.Label) || math.Abs(hi.X-lo.X-29) > 1e-9 || math.Abs(hi.Z-lo.Z-.3) > 1e-9 {
		t.Fatal("long solid caption lost source text or compiled print space")
	}
}

func TestNativeSolidOpacityCopiesAndTextureAdmission(t *testing.T) {
	for _, kind := range nativeSolidTestKinds {
		t.Run(kind, func(t *testing.T) {
			n := solidTestNode(kind)
			plain := solidTestBuild(t, n)
			plain.triangles = solidTestPhysical(plain.triangles)
			n.Opacity = .35
			faded := solidTestBuild(t, n)
			faded.triangles = solidTestPhysical(faded.triangles)
			if len(faded.triangles) != len(plain.triangles) {
				t.Fatal("opacity changed solid geometry")
			}
			for _, tri := range faded.triangles {
				if tri.Material.Color.A != 89 {
					t.Fatal("solid opacity lost or applied more than once")
				}
			}
			n.Opacity = 0
			if hidden := solidTestBuild(t, n); len(hidden.triangles) != 0 {
				t.Fatal("fully transparent solid is visible")
			}
			n.Opacity, n.Metadata.Original.Multiple = 1, true
			multiple := solidTestBuild(t, n)
			multiple.triangles = solidTestPhysical(multiple.triangles)
			if len(multiple.triangles) != 2*len(plain.triangles) {
				t.Fatal("multiple modifier did not duplicate the whole solid")
			}
			for i, tri := range plain.triangles {
				for j, v := range tri.V {
					got := multiple.triangles[len(plain.triangles)+i].V[j]
					if got.Position != v.Position || got.Normal != v.Normal {
						t.Fatal("multiple modifier changed the primary solid")
					}
					copy := multiple.triangles[i].V[j]
					want := nadd(v.Position, nv(d2target.MULTIPLE_OFFSET*.01, -min(.08, nativeSolidHeight(n)*.18), -d2target.MULTIPLE_OFFSET*.01))
					if nlen(nsub(copy.Position, want)) > 1e-9 {
						t.Fatal("multiple modifier changed the authored copy offset")
					}
				}
			}
		})
	}
	n := solidTestNode(d2target.ShapeCylinder)
	n.Metadata.Original.FillPattern = "lines"
	b := &meshBuilder{ctx: context.Background(), scale: .01, outputDensity: 1e9, faceMaxPixels: 4096}
	b.node(n, "#849ebc")
	if b.err != nil || b.facePixels > 4096 || b.faceMaxPixels != 4096 {
		t.Fatalf("patterned volume exceeded/replaced face budget: pixels=%d budget=%d error=%v", b.facePixels, b.faceMaxPixels, b.err)
	}
	b.facePixels = 24 << 20
	b.node(n, "#849ebc")
	if b.err == nil {
		t.Fatal("solid cap and wall bypassed aggregate texture budget")
	}
}

func TestNativeSolidIconPrintIsPhysicallySupported(t *testing.T) {
	for _, kind := range []string{d2target.ShapeCylinder, d2target.ShapeQueue} {
		for _, caption := range []string{"inside", "outside", "none"} {
			t.Run(kind+"/"+caption, func(t *testing.T) {
				n := solidTestNode(kind)
				n.Label, n.Metadata.Original.Label = "Ring\nBuffer", "Ring\nBuffer"
				n.Metadata.Original.FontSize, n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 24, 150, 58
				n.Metadata.Original.LabelPosition, n.Metadata.Original.IconPosition = "INSIDE_MIDDLE_CENTER", "INSIDE_MIDDLE_LEFT"
				if caption == "outside" {
					n.Metadata.Original.LabelPosition = "OUTSIDE_BOTTOM_CENTER"
				} else if caption == "none" {
					n.Label, n.Metadata.Original.Label = "", ""
					n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 0, 0
				}
				u := iconData(t, "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="#ff0080"/></svg>`))
				n.Icon, n.Metadata.Original.Icon = u.String(), u
				b := solidTestBuild(t, n)
				checked, iconTriangles := 0, 0
				for _, tri := range b.triangles {
					if !tri.NoDepthWrite || tri.Material.Texture == nil {
						continue
					}
					tex := tri.Material.Texture
					c := color.NRGBAModel.Convert(tex.At(tex.Bounds().Dx()/2, tex.Bounds().Dy()/2)).(color.NRGBA)
					if c.R < 250 || c.G > 5 || c.B < 127 || c.B > 129 {
						continue
					}
					iconTriangles++
					for i := 1; i < 19; i++ {
						for j := 1; j < 20-i; j++ {
							// Inspect actual ink, excluding transparent letterboxing
							// when an icon-only shape has a nonsquare print area.
							a, c, d := float64(i)/20, float64(j)/20, 1-float64(i+j)/20
							u := a*tri.V[0].U + c*tri.V[1].U + d*tri.V[2].U
							v := a*tri.V[0].V + c*tri.V[1].V + d*tri.V[2].V
							_, _, _, alpha := tex.At(min(tex.Bounds().Dx()-1, int(u*float64(tex.Bounds().Dx()))), min(tex.Bounds().Dy()-1, int(v*float64(tex.Bounds().Dy())))).RGBA()
							if alpha < 0xf000 {
								continue
							}
							point := nadd(nadd(nmul(tri.V[0].Position, a), nmul(tri.V[1].Position, c)), nmul(tri.V[2].Position, d))
							top := math.Inf(-1)
							for _, body := range b.triangles {
								if body.NoDepthWrite {
									continue
								}
								a, c, d := body.V[0].Position, body.V[1].Position, body.V[2].Position
								denominator := (c.Z-d.Z)*(a.X-d.X) + (d.X-c.X)*(a.Z-d.Z)
								if math.Abs(denominator) < 1e-15 {
									continue
								}
								u := ((c.Z-d.Z)*(point.X-d.X) + (d.X-c.X)*(point.Z-d.Z)) / denominator
								v := ((d.Z-a.Z)*(point.X-d.X) + (a.X-d.X)*(point.Z-d.Z)) / denominator
								w := 1 - u - v
								if u >= -1e-9 && v >= -1e-9 && w >= -1e-9 {
									y := u*a.Y + v*c.Y + w*d.Y
									if y <= point.Y+1e-6 {
										top = max(top, y)
									}
								}
							}
							if point.Y-top > .015 {
								t.Fatalf("opaque icon floats above/outside its support: point=%v gap=%g", point, point.Y-top)
							}
							checked++
						}
					}
				}
				if iconTriangles != 2 || checked < 6 {
					t.Fatal("icon print did not produce two complete surface triangles with visible artwork")
				}
			})
		}
	}
}
