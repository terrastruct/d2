package d2isometricimg

import (
	"math"
	"sort"
)

type routeCaptionPoint struct{ x, z float64 }

const maxRouteCaptionHullVertices = 64

// Project parallel to the final orthographic camera onto one common board
// plane. Comparing these coordinates is equivalent to comparing screen-space
// polygons, independent of output size, crop, or supersampling.
func captionProjection(p Vec) routeCaptionPoint {
	direction := nativeViewDirection()
	return routeCaptionPoint{p.X - p.Y*direction.X/direction.Y, p.Z - p.Y*direction.Z/direction.Y}
}

// AvoidMesh reserves a completed component's actual projected geometry. Its
// structural silhouette and printed surfaces are separate obstacles: joining an
// outside caption to the body's hull would unnecessarily block the gap between
// them. Call after relief, outside-caption clearance and other transforms.
func (p *routeCaptionPlacer) AvoidMesh(triangles []Triangle) {
	if len(p.rects) >= maxRouteCaptionRects {
		return
	}
	var body []routeCaptionPoint
	var surfaces [][]routeCaptionPoint
	indices := make(map[*Material]int)
	for _, t := range triangles {
		if t.Material == nil || t.Material.Color.A == 0 {
			continue
		}
		points := [3]routeCaptionPoint{}
		valid := true
		for i, v := range t.V {
			if !captionFinite(v.Position.X, v.Position.Y, v.Position.Z) {
				valid = false
				break
			}
			points[i] = captionProjection(v.Position)
		}
		if !valid {
			continue
		}
		if t.Material.Texture == nil {
			body = append(body, points[:]...)
			continue
		}
		index, found := indices[t.Material]
		if !found {
			index = len(surfaces)
			indices[t.Material] = index
			surfaces = append(surfaces, nil)
		}
		surfaces[index] = append(surfaces[index], points[:]...)
	}
	p.avoidProjectedHull(body)
	// Iteration follows mesh/material creation order, never map order.
	for _, surface := range surfaces {
		p.avoidProjectedHull(surface)
	}
}

func (p *routeCaptionPlacer) avoidProjectedHull(points []routeCaptionPoint) {
	if len(points) < 3 || len(p.rects) >= maxRouteCaptionRects {
		return
	}
	hull := captionConvexHull(points)
	if len(hull) < 3 {
		return
	}
	r := routeCaptionRect{left: math.Inf(1), front: math.Inf(1), right: math.Inf(-1), back: math.Inf(-1), hard: true, hull: hull}
	for _, q := range hull {
		r.left, r.right = min(r.left, q.x), max(r.right, q.x)
		r.front, r.back = min(r.front, q.z), max(r.back, q.z)
	}
	// Bound narrow-phase work for complex imported artwork. Falling back to
	// the actual projected bounds is conservative and never misses a collision.
	if len(hull) > maxRouteCaptionHullVertices {
		r.hull = []routeCaptionPoint{{r.left, r.front}, {r.right, r.front}, {r.right, r.back}, {r.left, r.back}}
	}
	r.left -= routeCaptionGap
	r.right += routeCaptionGap
	r.front -= routeCaptionGap
	r.back += routeCaptionGap
	p.reserve(r)
}

// Automatic captions also avoid crossing other wires. The reserved straight
// legs use finalized routed vertices, including the small crossing bridges.
// Keep room in the shared obstacle budget for the captions themselves.
func (p *routeCaptionPlacer) AvoidRoute(points []Vec, radius float64) {
	if radius <= 0 || !captionFinite(radius) {
		return
	}
	for i := 1; i < len(points) && len(p.rects) < maxRouteCaptionRects/2; i++ {
		a, b := captionProjection(points[i-1]), captionProjection(points[i])
		if !captionFinite(a.x, a.z, b.x, b.z) {
			continue
		}
		length := math.Hypot(b.x-a.x, b.z-a.z)
		if length <= 1e-9 {
			continue
		}
		// Coordinates are already projected, so Y remains zero here.
		p.reserve(captionRect(labelSurface{center: nv((a.x+b.x)/2, 0, (a.z+b.z)/2), width: length, depth: 2 * radius, angle: math.Atan2(b.z-a.z, b.x-a.x)}, true))
	}
}

func captionConvexHull(points []routeCaptionPoint) []routeCaptionPoint {
	sort.Slice(points, func(i, j int) bool {
		return points[i].x < points[j].x || points[i].x == points[j].x && points[i].z < points[j].z
	})
	turn := func(a, b, c routeCaptionPoint) float64 { return (b.x-a.x)*(c.z-a.z) - (b.z-a.z)*(c.x-a.x) }
	hull := make([]routeCaptionPoint, 0, min(len(points), 64))
	for _, q := range points {
		for len(hull) >= 2 && turn(hull[len(hull)-2], hull[len(hull)-1], q) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, q)
	}
	lower := len(hull)
	for i := len(points) - 2; i >= 0; i-- {
		q := points[i]
		for len(hull) > lower && turn(hull[len(hull)-2], hull[len(hull)-1], q) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, q)
	}
	return hull[:len(hull)-1]
}

func captionHullCorners(r routeCaptionRect) []routeCaptionPoint {
	if len(r.hull) > 0 {
		return r.hull
	}
	ux, uz, vx, vz := r.ux*r.hw, r.uz*r.hw, -r.uz*r.hd, r.ux*r.hd
	return []routeCaptionPoint{{r.x - ux - vx, r.z - uz - vz}, {r.x + ux - vx, r.z + uz - vz}, {r.x + ux + vx, r.z + uz + vz}, {r.x - ux + vx, r.z - uz + vz}}
}

func captionHullOverlap(a, b routeCaptionRect) bool {
	pa, pb := captionHullCorners(a), captionHullCorners(b)
	interval := func(points []routeCaptionPoint, ax, az float64) (float64, float64) {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, q := range points {
			distance := q.x*ax + q.z*az
			lo, hi = min(lo, distance), max(hi, distance)
		}
		return lo, hi
	}
	for _, points := range [][]routeCaptionPoint{pa, pb} {
		for i, q := range points {
			next := points[(i+1)%len(points)]
			ax, az := next.z-q.z, q.x-next.x
			length := math.Hypot(ax, az)
			if length <= 1e-12 {
				continue
			}
			alo, ahi := interval(pa, ax, az)
			blo, bhi := interval(pb, ax, az)
			// Rectangles already include the print gap; mesh hulls need it.
			gap := 0.
			if len(a.hull) > 0 {
				gap += routeCaptionGap * length
			}
			if len(b.hull) > 0 {
				gap += routeCaptionGap * length
			}
			if ahi+gap <= blo || bhi+gap <= alo {
				return false
			}
		}
	}
	return true
}
