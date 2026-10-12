package d2isometricimg

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/d2lang/d2/d2target"
)

func TestSolidCapShadowKeepsPhysicalFillOpacity(t *testing.T) {
	for _, fixture := range []struct {
		name, fill string
		opacity    float64
		alpha      uint8
	}{
		{"opaque", "#aabbcc", 1, 255},
		{"node-opacity", "#aabbcc", .4, 102},
		{"fill-alpha", "rgba(170,187,204,0.5)", 1, 128},
		{"combined-opacity", "rgba(170,187,204,0.5)", .4, 51},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			n := solidTestNode(d2target.ShapeCylinder)
			n.Fill, n.Metadata.Original.Fill, n.Opacity = fixture.fill, fixture.fill, fixture.opacity
			b := &meshBuilder{ctx: context.Background(), scale: .01}
			b.node(n, "#777777")
			if b.err != nil {
				t.Fatal(b.err)
			}
			previous := append([]Triangle(nil), b.triangles...)
			caps := 0
			for i := range previous {
				if previous[i].ShadowFillAlpha != nil {
					caps++
					if *previous[i].ShadowFillAlpha != fixture.alpha {
						t.Fatalf("cap lost source alpha: %d, want %d", *previous[i].ShadowFillAlpha, fixture.alpha)
					}
					previous[i].ShadowFillAlpha = nil
				}
			}
			if caps == 0 {
				t.Fatal("filled cylinder cap did not own its physical shadow")
			}
			want := uint8(math.Round(float64(fixture.alpha) * .72))
			for _, fixed := range []bool{false, true} {
				triangles := previous
				if fixed {
					triangles = b.triangles
				}
				work := rasterWork{ctx: context.Background(), remaining: rasterMaxWork}
				shadow, err := rasterBuildShadow(&work, triangles)
				if err != nil {
					t.Fatal(err)
				}
				weaker, occupied := 0, 0
				for _, alpha := range shadow.opacity {
					if alpha == 0 {
						continue
					}
					occupied++
					if alpha < want {
						weaker++
					}
					if fixed && alpha != want {
						t.Fatalf("physical silhouette contains alpha %d, want uniform authored alpha %d", alpha, want)
					}
				}
				if occupied == 0 || !fixed && weaker < 10 {
					t.Fatal("fixture must expose the former pale contour inside the cast shadow")
				}
			}
		})
	}
}

func TestShadowTextureHolesAndPartialCoverageRemain(t *testing.T) {
	ctx := context.Background()
	texture := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	for i, alpha := range []uint8{255, 64, 0, 255} {
		texture.SetNRGBA(i, 0, color.NRGBA{R: 255, G: 255, B: 255, A: alpha})
	}
	material := &Material{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, Texture: texture}
	light := rasterUnit(rasterShadowDirection())
	right := rasterUnit(rasterCross(nv(0, 1, 0), light))
	up := rasterUnit(rasterCross(light, right))
	vertex := func(x, y, u, v float64) Vertex {
		return Vertex{Position: nadd(nmul(right, x), nmul(up, y)), Normal: light, U: u, V: v}
	}
	a, b, c, d := vertex(-1, 1, 0, 0), vertex(1, 1, 1, 0), vertex(1, -1, 1, 1), vertex(-1, -1, 0, 1)
	triangles := []Triangle{{V: [3]Vertex{a, b, c}, Material: material, CastShadow: true}, {V: [3]Vertex{a, c, d}, Material: material, CastShadow: true}}
	work := rasterWork{ctx: ctx, remaining: rasterMaxWork}
	shadow, err := rasterBuildShadow(&work, triangles)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []uint8{255, 64, 0, 255} {
		point := shadow.camera.project(nmul(right, -.75+float64(i)*.5))
		got := shadow.opacity[int(point.y)*shadow.camera.width+int(point.x)]
		if math.Abs(float64(got)-float64(want)) > 1 {
			t.Fatalf("texture shadow coverage changed at band %d: %d, want %d", i, got, want)
		}
	}
	n := solidTestNode(d2target.ShapeCylinder)
	n.Fill, n.Metadata.Original.Fill = "transparent", "transparent"
	builder := solidTestBuild(t, n)
	for _, triangle := range builder.triangles {
		if triangle.ShadowFillAlpha != nil {
			t.Fatal("transparent source fill became an opaque physical shadow")
		}
	}
}
