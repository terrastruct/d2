package d2isometricimg

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

var reliefTestKinds = []string{d2target.ShapeCloud, d2target.ShapePerson, d2target.ShapeC4Person}

func reliefTestNode(kind string) d2isometric.Node {
	n := fidelityNode(kind)
	// Default source theme tokens previously selected the tall sculptures;
	// explicit paint took an entirely different canonical plaque path.
	n.Metadata.Original.Stroke = "N1"
	n.Size.Y, n.Position.Y = 1.35, .07+1.35/2
	return n
}

func reliefTestBuild(t *testing.T, n d2isometric.Node, hierarchy bool) *meshBuilder {
	t.Helper()
	p, err := newTextPainter(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	ic, err := newSurfaceIconPainter(context.Background(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	b := &meshBuilder{ctx: context.Background(), scale: .01, faceMaxPixels: 128 << 10, text: p, icons: ic}
	if hierarchy {
		b.hierarchyNode(n, "#849ebc")
	} else {
		b.node(n, "#849ebc")
	}
	if b.err != nil {
		t.Fatal(b.err)
	}
	return b
}

func reliefTestWalls(triangles []Triangle) []Triangle {
	var walls []Triangle
	for _, triangle := range triangles {
		if triangle.Material != nil && !triangle.Material.Unlit && triangle.Material.Texture == nil {
			walls = append(walls, triangle)
		}
	}
	return walls
}

func TestReliefSymbolsKeepSourceContourAcrossStyles(t *testing.T) {
	for _, kind := range reliefTestKinds {
		t.Run(kind, func(t *testing.T) {
			n := reliefTestNode(kind)
			plain := reliefTestWalls(reliefTestBuild(t, n, false).triangles)
			profiles, err := nativeShapeProfiles(n.Metadata.Original)
			if err != nil {
				t.Fatal(err)
			}
			vertices := 0
			for _, profile := range profiles {
				vertices += len(profile)
			}
			if len(plain) != vertices*2 {
				t.Fatalf("source contour acquired a baseplate or sculptural parts: %d wall triangles, want %d", len(plain), vertices*2)
			}
			for _, triangle := range plain {
				for _, vertex := range triangle.V {
					found := false
					for _, profile := range profiles {
						for _, point := range profile {
							x := n.Position.X - n.Size.X/2 + point.X*.01
							z := n.Position.Z - n.Size.Z/2 + point.Z*.01
							found = found || math.Hypot(vertex.Position.X-x, vertex.Position.Z-z) < 1e-9
						}
					}
					if !found {
						t.Fatal("wall vertex is not on the authored D2 contour")
					}
				}
			}
			for _, style := range []string{"default", "pattern", "dash", "multiple"} {
				styled := n
				switch style {
				case "pattern":
					styled.Metadata.Original.FillPattern = "lines"
				case "dash":
					styled.Stroke, styled.StrokeWidth, styled.StrokeDash = "#af2670", 5, 4
					styled.Metadata.Original.Stroke, styled.Metadata.Original.StrokeWidth, styled.Metadata.Original.StrokeDash = styled.Stroke, styled.StrokeWidth, styled.StrokeDash
				case "multiple":
					styled.Metadata.Original.Multiple = true
				}
				original := styled
				walls := reliefTestWalls(reliefTestBuild(t, styled, false).triangles)
				copies := 1
				if style == "multiple" {
					copies = 2
				}
				if !nativeCanonicalNode(styled) || nativeSolidNode(styled) || len(walls) != copies*len(plain) {
					t.Fatalf("%s chose a different source symbol geometry", style)
				}
				for i, triangle := range walls[len(walls)-len(plain):] {
					if triangle.V != plain[i].V {
						t.Fatalf("%s moved or reshaped the primary source contour", style)
					}
				}
				if copies == 2 {
					lo, hi := solidTestBounds(plain)
					offset := nv(d2target.MULTIPLE_OFFSET*.01, -min(.045, (hi.Y-lo.Y)*.25), -d2target.MULTIPLE_OFFSET*.01)
					for i, triangle := range walls[:len(plain)] {
						for j, vertex := range triangle.V {
							if nlen(nsub(vertex.Position, nadd(plain[i].V[j].Position, offset))) > 1e-9 {
								t.Fatal("multiple copy lost the authored D2 translation")
							}
						}
					}
				}
				if !reflect.DeepEqual(original, styled) {
					t.Fatal("rendering mutated the authored symbol")
				}
			}
		})
	}
}

func TestReliefSymbolDepthIgnoresLegacySculptureHeight(t *testing.T) {
	for _, kind := range reliefTestKinds {
		for _, dimensions := range [][2]int{{200, 120}, {80, 240}, {20, 20}} {
			t.Run(fmt.Sprintf("%s/%dx%d", kind, dimensions[0], dimensions[1]), func(t *testing.T) {
				var initial []Triangle
				for _, legacyHeight := range []float64{.01, 1.35, 8} {
					for _, threeD := range []bool{false, true} {
						n := reliefTestNode(kind)
						n.Metadata.Original.Width, n.Metadata.Original.Height = dimensions[0], dimensions[1]
						n.Metadata.Original.ThreeDee = threeD
						n.Size, n.Position.Y = nv(float64(dimensions[0])*.01, legacyHeight, float64(dimensions[1])*.01), .07+legacyHeight/2
						walls := reliefTestWalls(reliefTestBuild(t, n, false).triangles)
						lo, hi := solidTestBounds(walls)
						depth := .19 * min(n.Size.X, n.Size.Z)
						if kind == d2target.ShapeCloud {
							depth = .24 * min(n.Size.X, n.Size.Z)
						}
						if threeD {
							depth += d2target.THREE_DEE_OFFSET * .01
						}
						if depth <= 0 || math.Abs(lo.Y-.07) > 1e-9 || math.Abs(hi.Y-lo.Y-depth) > 1e-9 {
							t.Fatalf("relief adopted old sculpture height or lost its floor: %v..%v, want depth %g", lo, hi, depth)
						}
						profiles, err := nativeShapeProfiles(n.Metadata.Original)
						if err != nil {
							t.Fatal(err)
						}
						minX, minZ, maxX, maxZ := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
						for _, profile := range profiles {
							for _, point := range profile {
								x := n.Position.X - n.Size.X/2 + point.X*.01
								z := n.Position.Z - n.Size.Z/2 + point.Z*.01
								minX, minZ, maxX, maxZ = min(minX, x), min(minZ, z), max(maxX, x), max(maxZ, z)
							}
						}
						if math.Abs(lo.X-minX)+math.Abs(lo.Z-minZ)+math.Abs(hi.X-maxX)+math.Abs(hi.Z-maxZ) > 1e-9 {
							t.Fatal("relief enlarged or clipped the authored source path")
						}
						hierarchy := reliefTestWalls(reliefTestBuild(t, n, true).triangles)
						if len(hierarchy) != len(walls) {
							t.Fatal("hierarchy changed the symbol topology")
						}
						for i := range walls {
							for j := range walls[i].V {
								if nlen(nsub(hierarchy[i].V[j].Position, walls[i].V[j].Position)) > 1e-9 {
									t.Fatal("hierarchy compressed the chosen relief a second time")
								}
							}
						}
						header := hierarchyRenderNodes([]d2isometric.Node{n})[0]
						if math.Abs(header.Size.Y-depth) > 1e-9 || math.Abs(header.Position.Y-header.Size.Y/2-.07) > 1e-9 {
							t.Fatal("header obstacle retained the legacy sculpture height")
						}
						if !threeD {
							if initial == nil {
								initial = walls
								continue
							}
							for i := range walls {
								for j := range walls[i].V {
									if nlen(nsub(walls[i].V[j].Position, initial[i].V[j].Position)) > 1e-9 {
										t.Fatal("legacy Y estimate changed relief geometry")
									}
								}
							}
						}
					}
				}
			})
		}
	}
}

func TestReliefSymbolCaptionsAndIconsStayOnSourcePrintPlane(t *testing.T) {
	for _, kind := range reliefTestKinds {
		for _, position := range []string{"INSIDE_MIDDLE_CENTER", "OUTSIDE_TOP_CENTER", "BORDER_TOP_CENTER"} {
			n := reliefTestNode(kind)
			n.Label, n.Metadata.Original.Label = "Source", "Source"
			n.Metadata.Original.FontSize = 18
			n.Metadata.Original.LabelWidth, n.Metadata.Original.LabelHeight = 50, 24
			n.Metadata.Original.LabelPosition = position
			b := reliefTestBuild(t, n, true)
			caption := b.triangles[len(b.triangles)-2:]
			lo, hi := solidTestBounds(caption)
			if !caption[0].NoDepthWrite || caption[0].Material.Texture == nil || math.Abs(hi.X-lo.X-.5) > 1e-9 || math.Abs(hi.Z-lo.Z-.24) > 1e-9 || hi.Y != lo.Y {
				t.Fatalf("%s %s changed the source-sized flat caption", kind, position)
			}
			geometry := shape.NewShape(d2target.DSL_SHAPE_TO_SHAPE_TYPE[kind], geo.NewBox(geo.NewPoint(0, 0), 200, 120))
			box := geometry.GetInnerBox()
			p := label.FromString(position)
			if p.IsOutside() || p.IsBorder() {
				box = geometry.GetBox()
			}
			point := p.GetPointOnBox(box, label.PADDING, 50, 24)
			want := nv(n.Position.X-n.Size.X/2+point.X*.01, .07+nativeCanonicalHeight(n, .01)+.0015, n.Position.Z-n.Size.Z/2+point.Y*.01)
			if nlen(nsub(lo, want)) > 1e-9 {
				t.Fatalf("%s %s moved the authored label anchor: %v, want %v", kind, position, lo, want)
			}
			if position != "OUTSIDE_TOP_CENTER" {
				continue
			}
			n.Metadata.Original.Icon = iconData(t, "image/svg+xml", []byte(surfaceTestSVG))
			withIcon := reliefTestBuild(t, n, true)
			iconCaption := withIcon.triangles[len(withIcon.triangles)-2:]
			if !reflect.DeepEqual(caption[0].V, iconCaption[0].V) || !reflect.DeepEqual(caption[1].V, iconCaption[1].V) {
				t.Fatal("face icon moved the separately authored outside caption")
			}
			if len(withIcon.triangles) != len(b.triangles)+2 {
				t.Fatal("icon added physical support geometry instead of one printed quad")
			}
			icon := withIcon.triangles[len(withIcon.triangles)-4 : len(withIcon.triangles)-2]
			iconLo, iconHi := solidTestBounds(icon)
			if math.Abs(iconLo.Y-lo.Y) > 1e-9 || iconHi.Y != iconLo.Y || !icon[0].NoDepthWrite || icon[0].CastShadow {
				t.Fatal("icon ceased to be flat artwork on the original symbol cap")
			}
		}
	}
}

func TestReliefC4PersonKeepsBothContoursAndEmptyShoulders(t *testing.T) {
	n := reliefTestNode(d2target.ShapeC4Person)
	n.Metadata.Original.Height, n.Size.Z = 240, 2.4
	b := reliefTestBuild(t, n, false)
	profiles, err := nativeShapeProfiles(n.Metadata.Original)
	if err != nil || len(profiles) != 2 {
		t.Fatalf("C4 person lost the separate source head/body contours: %d, %v", len(profiles), err)
	}
	// Sample actual exported cap paint, not a synthetic bounding rectangle.
	// D2's head and torso overlap slightly; the regions beside the head are
	// empty, and must not turn into a rectangular baseplate or filled bridge.
	var cap []Triangle
	for _, triangle := range b.triangles {
		if triangle.Material != nil && triangle.Material.Texture != nil && !triangle.Material.Unlit {
			cap = append(cap, triangle)
		}
	}
	if len(cap) != 2 {
		t.Fatal("C4 person has no single source-painted cap")
	}
	lo, hi := solidTestBounds(cap)
	tex := cap[0].Material.Texture
	for _, sample := range []struct {
		x, z   float64
		filled bool
	}{{100, 44, true}, {100, 180, true}, {20, 40, false}, {180, 40, false}} {
		x := n.Position.X - n.Size.X/2 + sample.x*.01
		z := n.Position.Z - n.Size.Z/2 + sample.z*.01
		u, v := (x-lo.X)/(hi.X-lo.X), (z-lo.Z)/(hi.Z-lo.Z)
		_, _, _, alpha := tex.At(tex.Bounds().Min.X+int(u*float64(tex.Bounds().Dx())), tex.Bounds().Min.Y+int(v*float64(tex.Bounds().Dy()))).RGBA()
		if sample.filled && alpha < 65000 || !sample.filled && alpha != 0 {
			t.Fatalf("C4 source paint at (%g,%g): alpha=%d, want filled=%v", sample.x, sample.z, alpha, sample.filled)
		}
	}
}

func TestReliefTransparentSymbolsKeepSourceAlpha(t *testing.T) {
	for _, kind := range reliefTestKinds {
		n := reliefTestNode(kind)
		n.Fill, n.Metadata.Original.Fill = "transparent", "transparent"
		b := reliefTestBuild(t, n, false)
		if len(reliefTestWalls(b.triangles)) != 0 {
			t.Fatal("transparent source symbol acquired an opaque body")
		}
		if len(b.triangles) != 2 || !b.triangles[0].Material.Unlit || b.triangles[0].Material.Texture == nil {
			t.Fatal("transparent source symbol lost its authored contour ink")
		}
		n = reliefTestNode(kind)
		n.Opacity = .35
		b = reliefTestBuild(t, n, false)
		for _, triangle := range b.triangles {
			if triangle.Material.Color.A > 90 {
				t.Fatal("relief or ink bypassed authored group opacity")
			}
		}
	}
}

func TestReliefSymbolSourcePortsMeetWallsWithoutRouteExtensions(t *testing.T) {
	for _, kind := range reliefTestKinds {
		n := reliefTestNode(kind)
		profiles, err := nativeShapeProfiles(n.Metadata.Original)
		if err != nil {
			t.Fatal(err)
		}
		walls := reliefTestWalls(reliefTestBuild(t, n, true).triangles)
		for _, profile := range profiles {
			for i := 0; i < len(profile); i += max(1, len(profile)/4) {
				// Source routing contacts the contour at the common ground
				// elevation. A relief must meet that same port, including
				// the C4 head, without the old round-solid contact extension.
				p := nlerp(profile[i], profile[(i+1)%len(profile)], .5)
				port := nv(n.Position.X-n.Size.X/2+p.X*.01, .08, n.Position.Z-n.Size.Z/2+p.Z*.01)
				contact := false
				for _, triangle := range walls {
					contact = contact || contactTestOnTriangle(port, triangle)
				}
				if !contact {
					t.Fatalf("%s source port is detached from its relief wall: %v", kind, port)
				}
				paths := [][]Vec{{port, nadd(port, nv(1, 0, 0)), nadd(port, nv(1, 0, 2))}}
				edges := []d2isometric.Edge{{Source: n.ID, Opacity: 1}}
				got, err := nativeSolidContactRoutes(context.Background(), edges, []d2isometric.Node{n}, paths)
				if err != nil || !reflect.DeepEqual(got, paths) {
					t.Fatalf("relief changed authored flat routes: %v", err)
				}
			}
		}
	}
}
