package d2isometricimg

import (
	"fmt"
	"math"
	"sort"
)

// A texture's sampled linear RGB modulates diffuse light. Specular light and
// emission are added afterwards; multiplying by the shaded white material
// would incorrectly tint those contributions with the source texture.
func svgTextureLighting(camera rasterCamera, t Triangle) (slope, intercept [3]float64) {
	p := rasterMaterial(t.Material)
	slope = [3]float64{p.base.X, p.base.Y, p.base.Z}
	if p.unlit {
		return slope, intercept
	}
	normal := svgTriangleNormal(t, 0)
	normal = rasterUnit(normal)
	if rasterDot(normal, camera.direction) < 0 {
		normal = rasterMul(normal, -1)
	}
	ambient := .32 + .12*rasterClamp(normal.Y*.5+.5)
	illumination := Vec{X: ambient, Y: ambient, Z: ambient}
	spec := Vec{}
	for i, light := range rasterLightDirections {
		diffuse := math.Max(0, rasterDot(normal, light)) * rasterLightStrengths[i]
		illumination = rasterAdd(illumination, rasterMul(rasterLightColors[i], diffuse))
		half := rasterUnit(rasterAdd(light, camera.direction))
		power := math.Max(2, 2/(p.roughness*p.roughness)-2)
		s := math.Pow(math.Max(0, rasterDot(normal, half)), power) * diffuse * (.025 + p.metalness*.10)
		spec = rasterAdd(spec, rasterMul(rasterLightColors[i], s))
	}
	factor := 1 - p.metalness*.08
	slope[0], slope[1], slope[2] = slope[0]*illumination.X*factor, slope[1]*illumination.Y*factor, slope[2]*illumination.Z*factor
	intercept = [3]float64{spec.X + p.emission.X, spec.Y + p.emission.Y, spec.Z + p.emission.Z}
	return slope, intercept
}

func svgTriangleNormal(t Triangle, i int) Vec {
	normal := t.V[i].Normal
	if nlen(normal) < 1e-12 {
		normal = nunit(ncross(nsub(t.V[1].Position, t.V[0].Position), nsub(t.V[2].Position, t.V[0].Position)))
	}
	return normal
}

type svgLightingVertex struct {
	point    svgPoint
	position Vec
	normal   Vec
}

type svgGradientStop struct {
	offset float64
	color  Vec
}

type svgLightingGradient struct {
	start, end svgPoint
	stops      []svgGradientStop
	values     [3]float64
	valid      bool
}

type svgLightingPatch struct {
	vertices [3]svgLightingVertex
	gradient svgLightingGradient
}

func svgLightingColor(r *Raster, material rasterPaint, vertex svgLightingVertex) Vec {
	red, green, blue := r.shade(material, vertex.position, vertex.normal)
	return nv(red, green, blue)
}

func svgLightingMix(a, b svgLightingVertex, t float64) svgLightingVertex {
	return svgLightingVertex{
		point:    svgPoint{a.point.x + t*(b.point.x-a.point.x), a.point.y + t*(b.point.y-a.point.y), a.point.z + t*(b.point.z-a.point.z)},
		position: nlerp(a.position, b.position, t),
		normal:   nlerp(a.normal, b.normal, t),
	}
}

func svgColorError(a, b Vec) float64 {
	return math.Max(math.Abs(a.X-b.X), math.Max(math.Abs(a.Y-b.Y), math.Abs(a.Z-b.Z)))
}

func svgGradientColor(stops []svgGradientStop, t float64) Vec {
	if t <= stops[0].offset {
		return stops[0].color
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].offset {
			a, b := stops[i-1], stops[i]
			return nlerp(a.color, b.color, (t-a.offset)/(b.offset-a.offset))
		}
	}
	return stops[len(stops)-1].color
}

// Cylinder and queue facets interpolate between two normals, even though each
// quad is triangulated. That variation has one affine coordinate. Sampling the
// actual shader along it preserves curved highlights and all RGB channels,
// instead of approximating a nonlinear highlight by two luminance endpoints.
func svgFitLightingGradient(r *Raster, material rasterPaint, v [3]svgLightingVertex) (svgLightingGradient, float64) {
	lo, hi, distance := 0, 0, 0.0
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			delta := nsub(v[j].normal, v[i].normal)
			if d := ndot(delta, delta); d > distance {
				lo, hi, distance = i, j, d
			}
		}
	}
	gradient := svgLightingGradient{}
	if distance < 1e-20 {
		gradient.stops = []svgGradientStop{{0, svgLightingColor(r, material, v[0])}}
		return gradient, 0
	}
	direction := nsub(v[hi].normal, v[lo].normal)
	for i := range v {
		gradient.values[i] = ndot(nsub(v[i].normal, v[lo].normal), direction) / distance
	}
	a, b, c := v[0].point, v[1].point, v[2].point
	det := (b.x-a.x)*(c.y-a.y) - (c.x-a.x)*(b.y-a.y)
	if math.Abs(det) < 1e-12 {
		gradient.stops = []svgGradientStop{{0, svgLightingColor(r, material, v[0])}}
		return gradient, 0
	}
	values := gradient.values
	gx := ((values[1]-values[0])*(c.y-a.y) - (values[2]-values[0])*(b.y-a.y)) / det
	gy := ((b.x-a.x)*(values[2]-values[0]) - (c.x-a.x)*(values[1]-values[0])) / det
	norm := gx*gx + gy*gy
	if norm < 1e-24 {
		gradient.stops = []svgGradientStop{{0, svgLightingColor(r, material, v[0])}}
		return gradient, 0
	}
	gradient.start = v[lo].point
	gradient.end = svgPoint{x: gradient.start.x + gx/norm, y: gradient.start.y + gy/norm}
	gradient.valid = true
	first, last := svgLightingColor(r, material, v[lo]), svgLightingColor(r, material, v[hi])
	gradient.stops = append(gradient.stops, svgGradientStop{0, first})
	var sample func(float64, float64, Vec, Vec, int)
	sample = func(left, right float64, lc, rc Vec, depth int) {
		middle := (left + right) / 2
		actual := svgLightingColor(r, material, svgLightingMix(v[lo], v[hi], middle))
		if depth < 6 && svgColorError(actual, nlerp(lc, rc, .5)) > .6/255 {
			sample(left, middle, lc, actual, depth+1)
			sample(middle, right, actual, rc, depth+1)
		} else {
			gradient.stops = append(gradient.stops, svgGradientStop{right, rc})
		}
	}
	sample(0, 1, first, last, 0)
	err := 0.0
	for i := range v {
		err = math.Max(err, svgColorError(svgLightingColor(r, material, v[i]), svgGradientColor(gradient.stops, values[i])))
		j := (i + 1) % 3
		midpoint := svgLightingMix(v[i], v[j], .5)
		err = math.Max(err, svgColorError(svgLightingColor(r, material, midpoint), svgGradientColor(gradient.stops, (values[i]+values[j])/2)))
	}
	center := svgLightingMix(svgLightingMix(v[0], v[1], .5), v[2], 1.0/3)
	err = math.Max(err, svgColorError(svgLightingColor(r, material, center), svgGradientColor(gradient.stops, (values[0]+values[1]+values[2])/3)))
	return gradient, err
}

func svgLightingPatches(r *Raster, material rasterPaint, vertices [3]svgLightingVertex) []svgLightingPatch {
	var patches []svgLightingPatch
	var subdivide func([3]svgLightingVertex, int)
	subdivide = func(v [3]svgLightingVertex, depth int) {
		gradient, err := svgFitLightingGradient(r, material, v)
		// An occasional spherical facet needs a second spatial dimension. Keep
		// this bounded at 64 patches; normal native curve facets fit much earlier.
		if err <= 1.5/255 || depth == 3 {
			patches = append(patches, svgLightingPatch{v, gradient})
			return
		}
		a, b, c := svgLightingMix(v[0], v[1], .5), svgLightingMix(v[1], v[2], .5), svgLightingMix(v[2], v[0], .5)
		subdivide([3]svgLightingVertex{v[0], a, c}, depth+1)
		subdivide([3]svgLightingVertex{a, v[1], b}, depth+1)
		subdivide([3]svgLightingVertex{c, b, v[2]}, depth+1)
		subdivide([3]svgLightingVertex{a, b, c}, depth+1)
	}
	subdivide(vertices, 0)
	return patches
}

func writeSVGLightingGradient(w *nativeSVGWriter, gradient svgLightingGradient, id string) string {
	if !gradient.valid {
		c := gradient.stops[0].color
		return svgColor(c.X, c.Y, c.Z)
	}
	w.write(`<linearGradient id="%s" gradientUnits="userSpaceOnUse" color-interpolation="sRGB" x1="%s" y1="%s" x2="%s" y2="%s">`, id, svgNumber(gradient.start.x), svgNumber(gradient.start.y), svgNumber(gradient.end.x), svgNumber(gradient.end.y))
	for _, stop := range gradient.stops {
		w.write(`<stop offset="%s" stop-color="%s"/>`, svgNumber(stop.offset), svgColor(stop.color.X, stop.color.Y, stop.color.Z))
	}
	w.write(`</linearGradient>`)
	return "url(#" + id + ")"
}

func writeSVGFaceGradient(w *nativeSVGWriter, b *svgPaintBatch, id int, cameras ...rasterCamera) string {
	var vertices [3]svgLightingVertex
	for i, vertex := range b.triangle.V {
		vertices[i] = svgLightingVertex{b.points[i], vertex.Position, svgTriangleNormal(b.triangle, i)}
	}
	r := &Raster{camera: nativeCameraAxes()}
	if len(cameras) != 0 {
		r.camera = cameras[0]
	}
	// Lighting uses the fixed native view direction. Translation and image
	// fitting affect projection but never the orthographic lighting direction.
	patches := svgLightingPatches(r, rasterMaterial(b.triangle.Material), vertices)
	w.write(`<defs>`)
	fills := make([]string, len(patches))
	for i, patch := range patches {
		fills[i] = writeSVGLightingGradient(w, patch.gradient, fmt.Sprintf("shade-%d-%d", id, i))
	}
	if len(patches) == 1 {
		w.write(`</defs>`)
		return fills[0]
	}
	box := svgPolygonBox(b.points[:])
	// The opaque base prevents antialiasing gaps between the small shading
	// patches. Material/source opacity is applied once to the enclosing face.
	base, _ := svgFitLightingGradient(r, rasterMaterial(b.triangle.Material), vertices)
	baseFill := writeSVGLightingGradient(w, base, fmt.Sprintf("shade-%d-base", id))
	w.write(`<pattern id="shade-%d" patternUnits="userSpaceOnUse" x="%s" y="%s" width="%s" height="%s" viewBox="%s %s %s %s" preserveAspectRatio="none">`, id, svgNumber(box.minX), svgNumber(box.minY), svgNumber(box.maxX-box.minX), svgNumber(box.maxY-box.minY), svgNumber(box.minX), svgNumber(box.minY), svgNumber(box.maxX-box.minX), svgNumber(box.maxY-box.minY))
	w.write(`<path d="%s" fill="%s"/>`, svgPolygonPath([][]svgPoint{b.points[:]}), baseFill)
	for i, patch := range patches {
		p := []svgPoint{patch.vertices[0].point, patch.vertices[1].point, patch.vertices[2].point}
		w.write(`<path d="%s" fill="%s"/>`, svgPolygonPath([][]svgPoint{p}), fills[i])
	}
	w.write(`</pattern></defs>`)
	return fmt.Sprintf("url(#shade-%d)", id)
}

func svgCasterOpacity(t Triangle) uint8 {
	if !t.CastShadow || t.Material != nil && t.Material.Color.A == 0 {
		return 0
	}
	opacity := uint8(255)
	if t.Material != nil {
		opacity = t.Material.Color.A
	}
	if t.ShadowFillAlpha != nil {
		opacity = *t.ShadowFillAlpha
	}
	if t.ShadowOpacity != nil {
		opacity = uint8(math.Round(float64(opacity) * *t.ShadowOpacity))
	}
	if t.OpacityGroup != nil {
		opacity = uint8(math.Round(float64(opacity) * t.OpacityGroup.Opacity))
	}
	return opacity
}

func svgGroundCaster(t Triangle, ground, receiver float64, camera rasterCamera, light Vec) []svgPoint {
	var vertices []Vec
	for i, vertex := range t.V {
		a, b := vertex.Position, t.V[(i+1)%3].Position
		if a.Y >= receiver {
			vertices = append(vertices, a)
		}
		if (a.Y < receiver) != (b.Y < receiver) {
			vertices = append(vertices, nlerp(a, b, (receiver-a.Y)/(b.Y-a.Y)))
		}
	}
	var points []svgPoint
	for _, vertex := range vertices {
		p := nsub(vertex, nmul(light, (vertex.Y-(ground+rasterShadowNormalOffset))/light.Y))
		p.Y = ground
		q := camera.project(p)
		points = append(points, svgPoint{q.x, q.y, ndot(vertex, light)})
	}
	return points
}

func writeSVGGroundShadow(w *nativeSVGWriter, scene *nativeScene) {
	if len(scene.triangles) == 0 {
		return
	}
	ground, light := rasterShadowGround(scene.triangles), rasterShadowDirection()
	triangles := rasterGroundTriangles(scene.triangles, ground, scene.camera.direction)
	lightCamera := rasterFit(triangles, light, 2048, 2048, 1.08)
	bias := math.Max(.012, 2/lightCamera.scale)
	receiver := ground + rasterShadowNormalOffset + bias*light.Y
	var faces []svgVisibilityFace
	var sources []int
	var opacities []uint8
	for i, triangle := range triangles {
		if i%512 == 0 {
			if w.err = w.ctx.Err(); w.err != nil {
				return
			}
		}
		opacity := svgCasterOpacity(triangle)
		if opacity == 0 {
			continue
		}
		points := svgGroundCaster(triangle, ground, receiver, scene.camera, light)
		if svgValidFragment(points) == nil {
			continue
		}
		texture := triangle.ShadowFillAlpha == nil && triangle.Material != nil && triangle.Material.Texture != nil
		faces = append(faces, svgVisibilityFace{points: points, opaque: !texture, order: -i})
		sources, opacities = append(sources, i), append(opacities, opacity)
	}
	if len(faces) == 0 {
		return
	}
	visible, err := svgVisibleFaces(w.ctx, faces)
	if err != nil {
		w.err = fmt.Errorf("native SVG ground shadow: %w", err)
		return
	}
	// Native PCF has a three-sample footprint in both light-camera axes. A
	// single vector blur approximates its softness without triangle seams or
	// shadow bitmaps; its radius follows output density and light-camera fit.
	right := nsub(lightCamera.right, nmul(light, lightCamera.right.Y/light.Y))
	up := nsub(lightCamera.up, nmul(light, lightCamera.up.Y/light.Y))
	scale := scene.camera.scale / lightCamera.scale * math.Sqrt(2.0/3)
	sx := math.Hypot(ndot(right, scene.camera.right), ndot(up, scene.camera.right)) * scale
	sy := math.Hypot(ndot(right, scene.camera.up), ndot(up, scene.camera.up)) * scale
	w.write(`<defs><filter id="ground-softness" filterUnits="userSpaceOnUse" x="0" y="0" width="%d" height="%d" color-interpolation-filters="sRGB"><feGaussianBlur stdDeviation="%s %s"/></filter></defs><g filter="url(#ground-softness)">`, scene.width, scene.height, svgNumber(sx), svgNumber(sy))
	merged := make(map[uint8][][]svgPoint)
	textured := make(map[uint8][]int)
	for i, polygons := range visible {
		if len(polygons) == 0 {
			continue
		}
		t := triangles[sources[i]]
		if t.ShadowFillAlpha == nil && t.Material != nil && t.Material.Texture != nil {
			textured[opacities[i]] = append(textured[opacities[i]], i)
		} else {
			merged[opacities[i]] = append(merged[opacities[i]], polygons...)
		}
	}
	for alpha := 1; alpha <= 255; alpha++ {
		polygons, textures := merged[uint8(alpha)], textured[uint8(alpha)]
		if len(polygons) == 0 && len(textures) == 0 {
			continue
		}
		// Assemble one silhouette before applying its shadow strength. An
		// opaque printed cap overlaps its sidewalls in light space; fading
		// them separately produces a dark ring around a pale hollow center.
		// Retained vector alpha still cuts genuine holes in image artwork.
		w.write(`<g opacity="%s">`, svgNumber(.11*float64(alpha)/255))
		if len(polygons) != 0 {
			w.write(`<path d="%s" fill="#243754"/>`, svgPolygonPath(polygons))
		}
		sort.SliceStable(textures, func(a, b int) bool {
			depth := func(i int) float64 {
				sum := 0.0
				for _, p := range faces[i].points {
					sum += p.z
				}
				return sum / float64(len(faces[i].points))
			}
			return depth(textures[a]) < depth(textures[b])
		})
		for _, i := range textures {
			t := triangles[sources[i]]
			if t.Material.Vector == nil {
				w.err = fmt.Errorf("native SVG shadow surface %d has no retained vector source", sources[i])
				return
			}
			surfaceID := w.surfaceDefinition(t.Material.Vector)
			if w.err != nil {
				return
			}
			var p [3]svgPoint
			for j, v := range t.V {
				world := nsub(v.Position, nmul(light, (v.Position.Y-(ground+rasterShadowNormalOffset))/light.Y))
				world.Y = ground
				q := scene.camera.project(world)
				p[j] = svgPoint{q.x, q.y, q.z}
			}
			affine, err := svgTextureAffine(t, p)
			if err != nil {
				w.err = err
				return
			}
			w.write(`<defs><clipPath id="shadow-clip-%d"><path d="%s"/></clipPath><filter id="shadow-ink-%d" color-interpolation-filters="sRGB"><feColorMatrix values="0 0 0 0 0.14117647 0 0 0 0 0.21568627 0 0 0 0 0.32941176 0 0 0 1 0"/></filter></defs>`, i, svgPolygonPath(visible[i]), i)
			w.write(`<g clip-path="url(#shadow-clip-%d)"><g transform="matrix(%s %s %s %s %s %s)" filter="url(#shadow-ink-%d)"><use xlink:href="#%s"/></g></g>`, i, svgNumber(affine[0]), svgNumber(affine[1]), svgNumber(affine[2]), svgNumber(affine[3]), svgNumber(affine[4]), svgNumber(affine[5]), i, surfaceID)
		}
		w.write(`</g>`)
	}
	w.write(`</g>`)
}
