package d2isometricimg

import "math"

// Subtract a flat caption area from its own already dashed wire and casing.
// Clipping the actual triangles keeps the original dash phase, joins,
// and full authored stroke width, including diagonal segments. Each half-plane
// emits an exterior piece and passes only the remaining interior to the next;
// the emitted pieces never overlap, including for translucent source ink.
func (b *meshBuilder) routeCaptionKnockout(first int, surface labelSurface) {
	if surface.width <= 0 || surface.depth <= 0 || first == len(b.triangles) {
		return
	}
	wire := append([]Triangle(nil), b.triangles[first:]...)
	b.triangles = b.triangles[:first]
	cos, sin := math.Cos(surface.angle), math.Sin(surface.angle)
	center := captionProjection(surface.center)
	bounds := [4]float64{-surface.width / 2, surface.width / 2, -surface.depth / 2, surface.depth / 2}
	for i, triangle := range wire {
		if b.err != nil {
			return
		}
		if i%1024 == 0 {
			if err := b.ctx.Err(); err != nil {
				b.err = err
				return
			}
		}
		var remaining [8]Vertex
		copy(remaining[:], triangle.V[:])
		count := 3
		for side, bound := range bounds {
			if count < 3 {
				break
			}
			distance := func(v Vertex) float64 {
				projected := captionProjection(v.Position)
				dx, dz := projected.x-center.x, projected.z-center.z
				x, z := dx*cos+dz*sin, -dx*sin+dz*cos
				switch side {
				case 0:
					return x - bound
				case 1:
					return bound - x
				case 2:
					return z - bound
				default:
					return bound - z
				}
			}
			var interior, exterior [8]Vertex
			ni, ne := 0, 0
			previous := remaining[count-1]
			previousDistance := distance(previous)
			for _, current := range remaining[:count] {
				currentDistance := distance(current)
				if (previousDistance >= 0) != (currentDistance >= 0) {
					t := previousDistance / (previousDistance - currentDistance)
					cut := Vertex{Position: nlerp(previous.Position, current.Position, t), Normal: nlerp(previous.Normal, current.Normal, t), U: previous.U + (current.U-previous.U)*t, V: previous.V + (current.V-previous.V)*t}
					if previousDistance != 0 && currentDistance != 0 {
						interior[ni], ni = cut, ni+1
					}
					exterior[ne], ne = cut, ne+1
				}
				if currentDistance >= 0 {
					interior[ni], ni = current, ni+1
				} else {
					exterior[ne], ne = current, ne+1
				}
				previous, previousDistance = current, currentDistance
			}
			for i := 1; i+1 < ne; i++ {
				if nlen(ncross(nsub(exterior[i].Position, exterior[0].Position), nsub(exterior[i+1].Position, exterior[0].Position))) > 1e-18 {
					first := len(b.triangles)
					b.triangle(exterior[0], exterior[i], exterior[i+1], triangle.Material, triangle.CastShadow)
					if len(b.triangles) > first {
						piece := triangle
						piece.V = b.triangles[first].V
						b.triangles[first] = piece
					}
				}
			}
			remaining, count = interior, ni
		}
	}
}
