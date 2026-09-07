package d2isometricimg

import (
	"context"
	"image/color"
	"math"
	"strings"
	"testing"
)

func TestSVGTextureLightingMatchesNativeLinearShader(t *testing.T) {
	camera := nativeCameraAxes()
	for _, unlit := range []bool{false, true} {
		m := &Material{Color: color.NRGBA{180, 225, 95, 255}, Roughness: .12, Metalness: .8, Emissive: color.NRGBA{18, 12, 45, 220}, Unlit: unlit}
		target := Triangle{Material: m}
		for i := range target.V {
			target.V[i].Normal = nv(.3, .8, .6)
		}
		slope, intercept := svgTextureLighting(camera, target)
		for _, sample := range []Vec{nv(0, 0, 0), nv(1, 1, 1), nv(.15, .6, .9), nv(.5, .5, .5)} {
			paint := rasterMaterial(m)
			paint.base = nv(paint.base.X*rasterLinear(sample.X), paint.base.Y*rasterLinear(sample.Y), paint.base.Z*rasterLinear(sample.Z))
			r := &Raster{camera: camera}
			red, green, blue := r.shade(paint, Vec{}, target.V[0].Normal)
			want := nv(red, green, blue)
			got := nv(rasterSRGB(slope[0]*rasterLinear(sample.X)+intercept[0]), rasterSRGB(slope[1]*rasterLinear(sample.Y)+intercept[1]), rasterSRGB(slope[2]*rasterLinear(sample.Z)+intercept[2]))
			if err := svgColorError(got, want); err > 1e-12 {
				t.Fatalf("unlit=%v sample=%+v affine lighting error %v", unlit, sample, err)
			}
		}
	}
}

func TestSVGCurvedGradientSamplesTheNativeHighlight(t *testing.T) {
	r := &Raster{camera: nativeCameraAxes()}
	material := rasterMaterial(&Material{Color: color.NRGBA{145, 190, 225, 255}, Roughness: .2, Metalness: .7})
	v := [3]svgLightingVertex{
		{point: svgPoint{x: 0, y: 0}, normal: nunit(nv(-.6, .7, .1))},
		{point: svgPoint{x: 100, y: 0}, normal: nunit(nv(.8, .5, .6))},
		{point: svgPoint{x: 100, y: 100}, normal: nunit(nv(.8, .5, .6))},
	}
	gradient, err := svgFitLightingGradient(r, material, v)
	if !gradient.valid || len(gradient.stops) <= 2 || err > 1.5/255 {
		t.Fatalf("curve did not get an accurate sampled gradient: valid=%v stops=%d error=%v", gradient.valid, len(gradient.stops), err)
	}
	for i := 0; i <= 200; i++ {
		p := float64(i) / 200
		want := svgLightingColor(r, material, svgLightingMix(v[0], v[1], p))
		got := svgGradientColor(gradient.stops, p)
		if error := svgColorError(got, want); error > 1.5/255 {
			t.Fatalf("curve at %.3f misses native shading by %.4f", p, error)
		}
	}
}

func TestSVGCurvedGradientSubdividesOnlyWhenASecondAxisIsNeeded(t *testing.T) {
	r := &Raster{camera: nativeCameraAxes()}
	material := rasterMaterial(&Material{Color: color.NRGBA{125, 185, 220, 255}, Roughness: .3, Metalness: .4})
	v := [3]svgLightingVertex{
		{point: svgPoint{x: 0, y: 0}, normal: nunit(nv(-.4, 1, .2))},
		{point: svgPoint{x: 100, y: 0}, normal: nunit(nv(.7, .8, .3))},
		{point: svgPoint{x: 100, y: 100}, normal: nunit(nv(.1, .4, 1))},
	}
	patches := svgLightingPatches(r, material, v)
	if len(patches) <= 1 || len(patches) > 64 {
		t.Fatalf("expected bounded curved-facet subdivision, got %d patches", len(patches))
	}
	area := 0.0
	for _, patch := range patches {
		p := patch.vertices
		area += svgPolygonArea([]svgPoint{p[0].point, p[1].point, p[2].point})
		for _, bary := range [][3]float64{{.2, .3, .5}, {.8, .1, .1}, {.1, .7, .2}} {
			vertex := svgLightingMix(svgLightingMix(p[0], p[1], bary[1]/(bary[0]+bary[1])), p[2], bary[2])
			parameter := patch.gradient.values[0]*bary[0] + patch.gradient.values[1]*bary[1] + patch.gradient.values[2]*bary[2]
			want := svgLightingColor(r, material, vertex)
			got := svgGradientColor(patch.gradient.stops, parameter)
			if error := svgColorError(got, want); error > 3.0/255 {
				t.Fatalf("subdivided curved facet misses native shading by %.4f", error)
			}
		}
	}
	if math.Abs(area-5000) > 1e-6 {
		t.Fatalf("shading subdivision changed face area: %v", area)
	}
}

func TestSVGShadowPreservesSourceOpacityAndFillAlpha(t *testing.T) {
	strength, fillAlpha := .5, uint8(200)
	target := Triangle{CastShadow: true, Material: &Material{Color: color.NRGBA{A: 128}}, ShadowFillAlpha: &fillAlpha, ShadowOpacity: &strength, OpacityGroup: &nativeOpacityGroup{Opacity: .4}}
	if got := svgCasterOpacity(target); got != 40 {
		t.Fatalf("caster opacity=%d, want native round(round(200*.5)*.4)=40", got)
	}
	target.ShadowFillAlpha = nil
	if got := svgCasterOpacity(target); got != 26 {
		t.Fatalf("material caster opacity=%d, want native round(round(128*.5)*.4)=26", got)
	}
	target.CastShadow = false
	if svgCasterOpacity(target) != 0 {
		t.Fatal("non-caster produced a shadow")
	}
}

func TestSVGShadowClipsTrianglesCrossingTheReceiver(t *testing.T) {
	camera := nativeCameraAxes()
	camera.scale, camera.width, camera.height = 100, 400, 400
	target := Triangle{V: [3]Vertex{{Position: nv(-2, .5, 0)}, {Position: nv(2, .5, 0)}, {Position: nv(0, -.5, 4)}}}
	points := svgGroundCaster(target, 0, .1, camera, rasterShadowDirection())
	if len(points) != 4 || math.Abs(svgPolygonArea(points)) < 1 {
		t.Fatalf("crossing triangle lost its clipped shadow: %+v", points)
	}
	for _, p := range points {
		if math.IsNaN(p.x) || math.IsNaN(p.y) || math.IsInf(p.z, 0) {
			t.Fatalf("invalid clipped shadow vertex: %+v", p)
		}
	}
}

func TestSVGShadowUnionsEqualOpacityAndKeepsNearestCaster(t *testing.T) {
	camera := nativeCameraAxes()
	camera.scale, camera.width, camera.height = 100, 400, 400
	light := rasterShadowDirection()
	material := &Material{Color: color.NRGBA{200, 210, 220, 255}}
	quad := func(offset float64, strength float64) []Triangle {
		point := func(x, z float64) Vertex {
			return Vertex{Position: nadd(nv(x, .8, z), nmul(light, offset)), Normal: nv(0, 1, 0)}
		}
		a, b, c, d := point(0, 0), point(1, 0), point(1, 1), point(0, 1)
		return []Triangle{{V: [3]Vertex{a, b, c}, CastShadow: true, Material: material, ShadowOpacity: &strength}, {V: [3]Vertex{a, c, d}, CastShadow: true, Material: material, ShadowOpacity: &strength}}
	}
	scene := &nativeScene{camera: camera, width: 400, height: 400, triangles: append(quad(0, 1), quad(.5, .5)...)}
	w := &nativeSVGWriter{ctx: context.Background()}
	writeSVGGroundShadow(w, scene)
	if w.err != nil {
		t.Fatal(w.err)
	}
	svg := w.buf.String()
	if strings.Count(svg, `<path `) != 1 || !strings.Contains(svg, `opacity="0.055216"`) {
		t.Fatalf("nearest half-strength caster should produce one union at .11*128/255: %s", svg)
	}
	if strings.Contains(svg, `<image`) || strings.Contains(svg, `data:image`) {
		t.Fatal("vector shadow unexpectedly contains a bitmap")
	}
}
