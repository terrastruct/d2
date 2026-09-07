package d2isometricimg

import "math"

var rasterLightDirections = [3]Vec{rasterUnit(Vec{X: -1, Y: 1.8, Z: 1}), rasterUnit(Vec{X: 10, Y: 12, Z: -16}), rasterUnit(Vec{X: 12, Y: 5, Z: 12})}

// A broad key and restrained fill reveal the sidewalls without bleaching
// authored colors. Horizontal faces receive approximately unit illumination.
var rasterLightStrengths = [3]float64{.55, .16, .07}
var rasterLightColors = [3]Vec{{X: 1, Y: 1, Z: 1}, {X: .93, Y: .96, Z: 1}, {X: 1, Y: 1, Z: 1}}

func (r *Raster) shade(p rasterPaint, position, normal Vec) (float64, float64, float64) {
	if p.unlit {
		return rasterSRGB(p.base.X), rasterSRGB(p.base.Y), rasterSRGB(p.base.Z)
	}
	normal = rasterUnit(normal)
	if rasterDot(normal, r.camera.direction) < 0 {
		normal = rasterMul(normal, -1)
	}
	h := rasterClamp(normal.Y*.5 + .5)
	ambient := .32 + .12*h
	illumination := Vec{X: ambient, Y: ambient, Z: ambient}
	spec := Vec{}
	shadow := r.shadow.occlusion(position, normal)
	for i, l := range rasterLightDirections {
		diffuse := math.Max(0, rasterDot(normal, l)) * rasterLightStrengths[i]
		if i == 0 {
			diffuse *= 1 - shadow
		}
		illumination = rasterAdd(illumination, rasterMul(rasterLightColors[i], diffuse))
		half := rasterUnit(rasterAdd(l, r.camera.direction))
		power := math.Max(2, 2/(p.roughness*p.roughness)-2)
		s := math.Pow(math.Max(0, rasterDot(normal, half)), power) * diffuse * (.025 + p.metalness*.10)
		spec = rasterAdd(spec, rasterMul(rasterLightColors[i], s))
	}
	diffuseFactor := 1 - p.metalness*.08
	return rasterSRGB(p.base.X*illumination.X*diffuseFactor + spec.X + p.emission.X), rasterSRGB(p.base.Y*illumination.Y*diffuseFactor + spec.Y + p.emission.Y), rasterSRGB(p.base.Z*illumination.Z*diffuseFactor + spec.Z + p.emission.Z)
}
