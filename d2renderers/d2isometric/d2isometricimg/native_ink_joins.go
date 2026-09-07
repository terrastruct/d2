package d2isometricimg

import (
	"fmt"
	"math"
	"sort"
)

// Paths split at structural branches so each dash keeps its own phase. Their
// butt ends need one shared exterior join where a silhouette turns a corner.
// Only ends of painted ribbon parts participate; a dash gap stays open.
func (b *meshBuilder) classicInkJunctions(segments, ends []classicInkSegment, radius float64, material *Material) {
	if b.err != nil || radius <= 0 {
		return
	}
	degree := make(map[classicInkKey]int)
	for _, s := range segments {
		degree[classicInkKeyOf(s.a)]++
		degree[classicInkKeyOf(s.c)]++
	}
	junctions := make(map[classicInkKey][]classicInkSegment)
	for _, s := range ends {
		key := classicInkKeyOf(s.a)
		if degree[key] > 2 {
			junctions[key] = append(junctions[key], s)
		}
	}
	keys := make([]classicInkKey, 0, len(junctions))
	for key := range junctions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return classicInkLess(keys[i], keys[j]) })
	camera := nativeCameraAxes()
	view := camera.direction
	type ray struct {
		direction Vec
		angle     float64
	}
	for _, key := range keys {
		if err := b.ctx.Err(); err != nil {
			b.err = err
			return
		}
		incident := junctions[key]
		if len(incident) < 2 {
			continue
		}
		var rays []ray
		var support []*classicInkFacet
		seen := make(map[*classicInkFacet]bool)
		for _, s := range incident {
			delta := nsub(s.c, s.a)
			delta = nsub(delta, nmul(view, ndot(delta, view)))
			if nlen(delta) < 1e-10 {
				continue
			}
			direction := nunit(delta)
			rays = append(rays, ray{direction, math.Atan2(ndot(direction, camera.up), ndot(direction, camera.right))})
			for _, face := range s.support {
				if !seen[face] {
					seen[face] = true
					support = append(support, face)
					if len(support) > maxClassicInkSupportFaces {
						b.err = fmt.Errorf("isometric outline join exceeds %d local supporting faces", maxClassicInkSupportFaces)
						return
					}
				}
			}
		}
		if len(rays) < 2 {
			continue
		}
		sort.Slice(rays, func(i, j int) bool { return rays[i].angle < rays[j].angle })
		at := incident[0].a
		vertex := func(offset Vec) Vertex {
			p := nadd(at, offset)
			if math.Abs(view.Y) > 1e-8 {
				p = nsub(p, nmul(view, offset.Y/view.Y))
			}
			return Vertex{Position: p, Normal: view}
		}
		for i, a := range rays {
			c := rays[(i+1)%len(rays)]
			angle := c.angle - a.angle
			if i == len(rays)-1 {
				angle += 2 * math.Pi
			}
			if angle <= math.Pi+1e-8 || angle >= 2*math.Pi-1e-8 {
				continue
			}
			// Smaller sectors are already covered by the adjacent strips.
			// Fill the outer wedge with a sharp miter, or a bounded bevel
			// at acute tips, without extending the authored stroke weight.
			u := ncross(view, a.direction)
			v := nmul(ncross(view, c.direction), -1)
			left, right := nmul(u, radius), nmul(v, radius)
			miter := nmul(nadd(u, v), radius/max(1e-12, 1+ndot(u, v)))
			if nlen(miter) <= 2*radius {
				b.classicInkFace([3]Vertex{vertex(Vec{}), vertex(left), vertex(miter)}, support, material, camera)
				b.classicInkFace([3]Vertex{vertex(Vec{}), vertex(miter), vertex(right)}, support, material, camera)
			} else {
				b.classicInkFace([3]Vertex{vertex(Vec{}), vertex(left), vertex(right)}, support, material, camera)
			}
		}
	}
}
