package d2isometricimg

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

type nativeSceneOptions struct {
	fitContent    bool
	deferRaster   bool
	vector        bool          // retain solid cap coverage beneath vector face paint
	camera        *rasterCamera // final logical pixels, before supersampling
	outputDensity float64
	links         *d2scenebuild.LinkBudget
}

const nativeTrafficRadius = .045

// Projected extents include every final mesh vertex, including outside labels,
// routed captions and document decorations. All boards use the same axes.
type projectedExtent struct {
	minX, minY, minZ, maxX, maxY, maxZ float64
	valid                              bool
}

func (e *projectedExtent) add(p Vec) {
	c := nativeCameraAxes()
	x, y, z := rasterDot(p, c.right), rasterDot(p, c.up), rasterDot(p, c.direction)
	if !e.valid {
		*e = projectedExtent{x, y, z, x, y, z, true}
		return
	}
	e.minX, e.minY, e.minZ = min(e.minX, x), min(e.minY, y), min(e.minZ, z)
	e.maxX, e.maxY, e.maxZ = max(e.maxX, x), max(e.maxY, y), max(e.maxZ, z)
}

func (e *projectedExtent) mesh(ts []Triangle) {
	for _, t := range ts {
		for _, v := range t.V {
			e.add(v.Position)
		}
	}
}

func (e *projectedExtent) animatedMesh(ts []Triangle, nodes []nativeAnimatedNode, scale float64) {
	e.mesh(ts)
	for _, node := range nodes {
		for i := node.first; i < node.last; i++ {
			for _, v := range ts[i].V {
				v.Position.Z -= 4 * scale
				e.add(v.Position)
			}
		}
	}
	e.groundShadows(ts, nodes, scale)
}

// The ground receiver is implicit raster content, so its directional shadows
// have no mesh vertices for ordinary framing to see. Project caster vertices
// onto that same receiver, then reserve the shadow map's 3x3 sampling footprint.
// This changes framing only; the source geometry and output density stay intact.
func (e *projectedExtent) groundShadows(ts []Triangle, nodes []nativeAnimatedNode, scale float64, shadowCamera ...rasterCamera) {
	if len(ts) == 0 {
		return
	}
	ground := rasterShadowGround(ts)
	ts = rasterGroundTriangles(ts, ground, nativeViewDirection())
	e.preparedGroundShadows(ts, nodes, scale, ground, shadowCamera...)
}

func (e *projectedExtent) preparedGroundShadows(ts []Triangle, nodes []nativeAnimatedNode, scale, ground float64, shadowCamera ...rasterCamera) {
	receiver := ground + rasterShadowNormalOffset
	var light rasterCamera
	if len(shadowCamera) > 0 {
		light = shadowCamera[0]
	} else {
		light = rasterFit(ts, rasterShadowDirection(), 2048, 2048, 1.08)
	}
	var shadow projectedExtent
	cast := func(p Vec) {
		distance := (p.Y - receiver) / light.direction.Y
		p = nsub(p, nmul(light.direction, distance))
		p.Y = ground
		shadow.add(p)
	}
	triangle := func(t Triangle, shift float64, animated bool) {
		if t.Material != nil && t.Material.Color.A == 0 {
			return
		}
		casts := t.CastShadow && (t.ShadowOpacity == nil || *t.ShadowOpacity > 0)
		if animated && !t.CastShadow && (t.Material == nil || t.Material.Texture == nil || !t.NoDepthWrite) {
			casts = true // frameRaster enables an authored animation's shadow
		}
		if !casts {
			return
		}
		// Clip to the receiver's sampling plane. Geometry below it cannot cast
		// onto the ground; an intersecting wall contributes its intersection.
		for i, vertex := range t.V {
			a, b := vertex.Position, t.V[(i+1)%3].Position
			a.Z, b.Z = a.Z-shift, b.Z-shift
			if a.Y >= receiver {
				cast(a)
			}
			if (a.Y < receiver) != (b.Y < receiver) {
				cast(nlerp(a, b, (receiver-a.Y)/(b.Y-a.Y)))
			}
		}
	}
	for _, t := range ts {
		triangle(t, 0, false)
	}
	for _, node := range nodes {
		for _, t := range ts[node.first:node.last] {
			triangle(t, 0, true)
			triangle(t, 4*scale, true)
		}
	}
	if !shadow.valid {
		return
	}
	// A caster sample at a pixel center can influence receiver samples up to
	// 1.5 shadow texels away: floor-to-integer sampling plus the ±1 filter.
	// Move both light-camera basis vectors onto the ground to derive the
	// corresponding footprint in the final camera's coordinates.
	u := nsub(light.right, nmul(light.direction, light.right.Y/light.direction.Y))
	v := nsub(light.up, nmul(light.direction, light.up.Y/light.direction.Y))
	radius, camera := 1.5/light.scale, nativeCameraAxes()
	expand := func(axis Vec) float64 { return radius * (math.Abs(ndot(u, axis)) + math.Abs(ndot(v, axis))) }
	dx, dy, dz := expand(camera.right), expand(camera.up), expand(camera.direction)
	shadow.minX, shadow.maxX = shadow.minX-dx, shadow.maxX+dx
	shadow.minY, shadow.maxY = shadow.minY-dy, shadow.maxY+dy
	shadow.minZ, shadow.maxZ = shadow.minZ-dz, shadow.maxZ+dz
	if !e.valid {
		*e = shadow
		return
	}
	e.minX, e.minY, e.minZ = min(e.minX, shadow.minX), min(e.minY, shadow.minY), min(e.minZ, shadow.minZ)
	e.maxX, e.maxY, e.maxZ = max(e.maxX, shadow.maxX), max(e.maxY, shadow.maxY), max(e.maxZ, shadow.maxZ)
}

// A packet interpolates linearly between routed vertices. Expanding their
// projected union by the physical sphere radius therefore contains every
// traffic phase, including packets at endpoints and on crossing bridges.
func (e *projectedExtent) trafficRoutes(packets []packetRoute) {
	var traffic projectedExtent
	for _, packet := range packets {
		for _, point := range packet.points {
			traffic.add(point)
		}
	}
	if !traffic.valid {
		return
	}
	traffic.minX -= nativeTrafficRadius
	traffic.minY -= nativeTrafficRadius
	traffic.minZ -= nativeTrafficRadius
	traffic.maxX += nativeTrafficRadius
	traffic.maxY += nativeTrafficRadius
	traffic.maxZ += nativeTrafficRadius
	if !e.valid {
		*e = traffic
		return
	}
	e.minX, e.minY, e.minZ = min(e.minX, traffic.minX), min(e.minY, traffic.minY), min(e.minZ, traffic.minZ)
	e.maxX, e.maxY, e.maxZ = max(e.maxX, traffic.maxX), max(e.maxY, traffic.maxY), max(e.maxZ, traffic.maxZ)
}

func nativeCameraAxes() rasterCamera {
	direction := rasterUnit(nativeViewDirection())
	right := rasterUnit(rasterCross(Vec{Y: 1}, direction))
	return rasterCamera{direction: direction, right: right, up: rasterUnit(rasterCross(direction, right))}
}

func (e projectedExtent) camera(width, height int, fit bool) rasterCamera {
	if !e.valid {
		e = projectedExtent{-1, -1, -1, 1, 1, 1, true}
	}
	w, h := max(.02, e.maxX-e.minX), max(.02, e.maxY-e.minY)
	if fit {
		// Preserve the maximum image budget and source geometry. The smaller
		// side contracts; very thin diagrams retain the 64-pixel admission floor.
		scale := min(float64(width)/w, float64(height)/h)
		width = max(64, min(width, int(math.Ceil(w*scale))))
		height = max(64, min(height, int(math.Ceil(h*scale))))
	}
	c := nativeCameraAxes()
	c.width, c.height = width, height
	c.centerX, c.centerY, c.centerDepth = (e.minX+e.maxX)/2, (e.minY+e.maxY)/2, (e.minZ+e.maxZ)/2
	c.scale = min(float64(width)/w, float64(height)/h) / 1.08
	return c
}

func cameraAtResolution(c rasterCamera, width, height int) rasterCamera {
	sx, sy := float64(width)/float64(c.width), float64(height)/float64(c.height)
	c.scale *= min(sx, sy)
	c.width, c.height = width, height
	return c
}

// Before textures exist, source geometry estimates their output sampling rate.
// Extra labels/decorations can only enlarge final bounds and lower that rate;
// texture budgets cap the conservative estimate. No layout dimensions change.
func sceneOutputDensity(s *d2isometric.Scene, width, height int, camera *rasterCamera) float64 {
	if camera != nil {
		return camera.scale
	}
	var e projectedExtent
	for _, board := range s.Boards {
		if board.Kind == "ungrouped" {
			continue
		}
		for _, x := range []float64{-.5, .5} {
			for _, z := range []float64{-.5, .5} {
				e.add(nadd(board.Position, nv(x*board.Size.X, 0, z*board.Size.Z)))
			}
		}
	}
	for _, n := range s.Nodes {
		if n.Opacity <= 0 {
			continue
		}
		floor := n.Position.Y - n.Size.Y/2
		h := max(.3, n.Size.Y)*1.3 + .2
		if nativeSolidNode(n) {
			h = nativeSolidHeight(n) + .002
		} else if nativeReliefSymbol(n) || nativeStructuredNode(n) {
			h = nativeCanonicalHeight(n, s.PixelScale) + .002
		}
		h *= hierarchyNodeRelief(n)
		for _, x := range []float64{-.5, .5} {
			for _, z := range []float64{-.5, .5} {
				for _, y := range []float64{floor, floor + h} {
					e.add(nv(n.Position.X+x*n.Size.X, y, n.Position.Z+z*n.Size.Z))
				}
			}
		}
	}
	for _, edge := range s.Edges {
		for _, p := range edge.Points {
			e.add(p)
		}
	}
	return e.camera(width, height, false).scale
}

// Measure one board at a time so animation framing uses the union of the exact
// final geometry without retaining every board's textures or raster buffers.
func timelineCamera(ctx context.Context, boards []*d2target.Diagram, o Options) (rasterCamera, error) {
	var bounds projectedExtent
	for _, board := range boards {
		if err := ctx.Err(); err != nil {
			return rasterCamera{}, err
		}
		s, err := d2isometric.BuildScene(board, &o.Render)
		if err != nil {
			return rasterCamera{}, err
		}
		n, err := newNativeSceneWithOptions(ctx, s, o.Width, o.Height, o.Assets, o.Fonts, nativeSceneOptions{deferRaster: true, outputDensity: sceneOutputDensity(s, o.Width, o.Height, nil)})
		if err != nil {
			return rasterCamera{}, err
		}
		bounds.animatedMesh(n.triangles, n.animatedNodes, n.pixelScale)
		bounds.trafficRoutes(n.packets)
	}
	return bounds.camera(o.Width, o.Height, o.FitContent), nil
}
