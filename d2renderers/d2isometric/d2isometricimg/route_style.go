package d2isometricimg

import (
	"hash/fnv"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

// Explicit paints keep their authored meaning. Otherwise a stable muted hue
// helps follow an individual route through a bundle without depending on scene
// traversal order or introducing a new semantic grouping.
func nativeRouteTint(edge d2isometric.Edge) string {
	if edge.StrokeExplicit {
		return edge.Stroke
	}
	palette := [...]string{"#3d6d8f", "#36776b", "#786391", "#957245", "#586d86", "#8c606b"}
	h := fnv.New32a()
	_, _ = h.Write([]byte(edge.ID))
	return palette[int(h.Sum32())%len(palette)]
}

func nativeRouteRadius(edge d2isometric.Edge) float64 {
	return max(.012, min(.075, float64(edge.StrokeWidth)*.012))
}

// Diagram ink is a flat, unlit ribbon. Its authored path and any crossing
// bridge remain in three dimensions, while illumination cannot change the
// perceived color or weight of the stroke with its direction.
func (b *meshBuilder) routeInk(points []Vec, radius float64, mat *Material) {
	if len(points) < 2 || radius <= 0 {
		return
	}
	left, right := make([]Vertex, len(points)), make([]Vertex, len(points))
	for i, p := range points {
		a, c := points[max(0, i-1)], points[min(len(points)-1, i+1)]
		tangent := nunit(nsub(c, a))
		side := nunit(ncross(tangent, nv(0, 1, 0)))
		up := nunit(ncross(side, tangent))
		left[i] = Vertex{Position: nadd(p, nmul(side, radius)), Normal: up}
		right[i] = Vertex{Position: nsub(p, nmul(side, radius)), Normal: up}
	}
	for i := 1; i < len(points); i++ {
		b.triangle(left[i-1], right[i], right[i-1], mat, false)
		b.triangle(left[i-1], left[i], right[i], mat, false)
	}
}

// A lower ribbon provides a narrow paper-colored casing under the colored ink.
// It follows the same local bridge geometry, so its depth occludes only the
// route below a crossing.
func (b *meshBuilder) routeCasing(points []Vec, radius float64, opacity float64) {
	if len(points) < 2 {
		return
	}
	mat := nativeMaterial("#f5f7fb", 1, 0, opacity)
	mat.Unlit = true
	left, right := make([]Vertex, len(points)), make([]Vertex, len(points))
	for i, p := range points {
		a, c := points[max(0, i-1)], points[min(len(points)-1, i+1)]
		tangent := nunit(nsub(c, a))
		side := nunit(ncross(tangent, nv(0, 1, 0)))
		up := nunit(ncross(side, tangent))
		offset := radius * .8
		if b.routeCasingFloor > 0 && up.Y > 1e-9 {
			// The paper casing is a decorative backing below the same route.
			// Keep it above raised container caps without moving the ink path
			// or flattening local crossing bridges.
			offset = min(offset, max(0, (p.Y-b.routeCasingFloor)/up.Y))
		}
		center := nsub(p, nmul(up, offset))
		left[i] = Vertex{Position: nadd(center, nmul(side, radius*1.3)), Normal: up}
		right[i] = Vertex{Position: nsub(center, nmul(side, radius*1.3)), Normal: up}
	}
	for i := 1; i < len(points); i++ {
		b.triangle(left[i-1], right[i], right[i-1], mat, false)
		b.triangle(left[i-1], left[i], right[i], mat, false)
	}
}
