package d2isometricimg

import (
	"context"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestSVGPaintCoverKeepsExactSilhouetteAndOneSourceOpacity(t *testing.T) {
	material := &Material{Color: color.NRGBA{R: 90, G: 130, B: 160, A: 128}}
	a := []svgPoint{{0, 0, 0}, {100, 0, 0}, {100, 100, 0}}
	b := []svgPoint{{0, 0, 0}, {100, 100, 0}, {0, 100, 0}}
	batches := []*svgPaintBatch{
		{triangle: Triangle{Material: material}, polygons: [][]svgPoint{a}, first: 10, color: "#7090a0"},
		{triangle: Triangle{Material: material}, polygons: [][]svgPoint{b}, first: 11, color: "#688898"},
	}
	covers := svgPreparePaintCovers(batches)
	cover := covers[batches[0]]
	if cover == nil || cover != covers[batches[1]] {
		t.Fatal("adjacent facets of one material did not share a paint cover")
	}
	w := &nativeSVGWriter{ctx: context.Background()}
	writeSVGPaintCover(w, cover, nativeCameraAxes())
	if w.err != nil {
		t.Fatal(w.err)
	}
	svg := w.buf.String()
	if strings.Count(svg, `fill-opacity=`) != 1 || !strings.Contains(svg, `fill-opacity="0.501961"`) {
		t.Fatalf("source alpha must be applied once after facet overdraw: %s", svg)
	}
	if strings.Count(svg, `stroke-width="0.8"`) != 2 {
		t.Fatal("missing internal facet coverage")
	}
	want := `<path d="` + svgPolygonPath([][]svgPoint{a, b}) + `" fill="url(#paint-cover-10)"`
	if !strings.Contains(svg, want) {
		t.Fatal("paint cover changed the exact source silhouette")
	}
	if strings.Contains(svg, `<image`) || strings.Contains(svg, `data:image`) {
		t.Fatal("paint coverage unexpectedly contains a bitmap")
	}
	writeSVGPaintCover(w, cover, nativeCameraAxes())
	if w.buf.String() != svg {
		t.Fatal("paint material was emitted again, compounding source alpha")
	}
}

func TestSVGPaintCoversKeepDifferentMaterialsAndDecalsSeparate(t *testing.T) {
	m1, m2 := &Material{}, &Material{}
	polygon := [][]svgPoint{{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}}
	batches := []*svgPaintBatch{
		{triangle: Triangle{Material: m1}, polygons: polygon, first: 1},
		{triangle: Triangle{Material: m1}, polygons: polygon, first: 2},
		{triangle: Triangle{Material: m2}, polygons: polygon, first: 3},
		{triangle: Triangle{Material: m2}, polygons: polygon, first: 4},
		{triangle: Triangle{Material: m1, NoDepthWrite: true}, polygons: polygon, first: 5},
		{triangle: Triangle{Material: m1}, polygons: polygon, texture: true, first: 6},
	}
	covers := svgPreparePaintCovers(batches)
	if covers[batches[0]] == nil || covers[batches[2]] == nil || covers[batches[0]] == covers[batches[2]] {
		t.Fatal("distinct source materials share a paint cover")
	}
	if covers[batches[4]] != nil || covers[batches[5]] != nil {
		t.Fatal("decal or textured face was collected into a physical paint cover")
	}
}

func TestSVGTexturedCoverPreservesOpacityAndTransparentExterior(t *testing.T) {
	box := d2scene.Box{Width: 1, Height: 1}
	surface := &nativeVectorSurface{document: &d2scene.Document{
		Root:    d2scene.NewNode(d2scene.Rect{Box: box, Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 90, G: 130, B: 160, A: 255}}}),
		ViewBox: box, LogicalWidth: 1, LogicalHeight: 1,
	}}
	material := &Material{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 128}, Vector: surface, svgSolidTexture: true, Unlit: true}
	a := []svgPoint{{10, 10, 0}, {90, 10, 0}, {90, 90, 0}}
	b := []svgPoint{{10, 10, 0}, {90, 90, 0}, {10, 90, 0}}
	batches := []*svgPaintBatch{
		{triangle: Triangle{Material: material}, polygons: [][]svgPoint{a}, first: 10, texture: true, affine: [6]float64{80, 0, 0, 80, 10, 10}},
		{triangle: Triangle{Material: material}, polygons: [][]svgPoint{b}, first: 11, texture: true, affine: [6]float64{80, 0, 0, 80, 10, 10}},
	}
	cover := svgPreparePaintCovers(batches)[batches[0]]
	if cover == nil {
		t.Fatal("filled textured facets did not share paint coverage")
	}
	w := &nativeSVGWriter{ctx: context.Background()}
	writeSVGPaintCover(w, cover, nativeCameraAxes())
	if w.err != nil {
		t.Fatal(w.err)
	}
	if material.Color.A != 128 || batches[0].cover != nil || svgPolygonPath(batches[0].polygons) != svgPolygonPath([][]svgPoint{a}) {
		t.Fatal("paint coverage mutated source material or geometry")
	}
	svg := w.buf.String()
	if strings.Count(svg, `fill-opacity="0.501961"`) != 1 || strings.Contains(svg, `opacity="0.501961"><g`) {
		t.Fatal("source alpha was applied to overlapping texture fragments")
	}
	if strings.Count(svg, `opacity="1"><g transform="matrix(80 0 0 80 10 10)"`) != 2 {
		t.Fatal("texture fields lost their affine mapping or gained overlapping alpha")
	}
	want := `</pattern></defs><path d="` + svgPolygonPath([][]svgPoint{a, b}) + `" fill="url(#paint-cover-10)" fill-opacity="0.501961"/>`
	if !strings.HasSuffix(svg, want) {
		t.Fatal("texture overdraw escapes its exact final silhouette")
	}
	if strings.Contains(svg, `<image`) || strings.Contains(svg, `data:image`) {
		t.Fatal("vector texture was flattened while removing seams")
	}
}

func TestSVGTexturedCoversExcludeUnfilledMarginsAndDecals(t *testing.T) {
	for _, kind := range []string{"unfilled", "decal", "multiply"} {
		t.Run(kind, func(t *testing.T) {
			m := &Material{svgSolidTexture: kind != "unfilled", Multiply: kind == "multiply"}
			polygon := [][]svgPoint{{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}}
			batches := []*svgPaintBatch{
				{triangle: Triangle{Material: m, NoDepthWrite: kind == "decal"}, polygons: polygon, first: 1, texture: true},
				{triangle: Triangle{Material: m, NoDepthWrite: kind == "decal"}, polygons: polygon, first: 2, texture: true},
			}
			if len(svgPreparePaintCovers(batches)) != 0 {
				t.Fatal("texture with unfilled margins or decal blending permits overdraw")
			}
		})
	}
}

func TestSVGCoverExpansionIsBoundedForBothWindingsAndAcuteCorners(t *testing.T) {
	for _, p := range [][]svgPoint{
		{{0, 0, 0}, {100, 0, 0}, {100, 100, 0}},
		{{100, 100, 0}, {100, 0, 0}, {0, 0, 0}},
		{{0, 0, 0}, {100, 0, 0}, {100, .00001, 0}},
	} {
		expanded := svgExpandCoverPolygon(p, .4)
		if len(expanded) < 3 || math.Abs(svgPolygonArea(expanded)) <= math.Abs(svgPolygonArea(p)) {
			t.Fatal("facet clip did not expand across its internal edge")
		}
		box := svgPolygonBox(p)
		for _, q := range expanded {
			if q.x < box.minX-.400001 || q.x > box.maxX+.400001 || q.y < box.minY-.400001 || q.y > box.maxY+.400001 {
				t.Fatal("acute facet expansion produced an unbounded miter")
			}
		}
	}
}
