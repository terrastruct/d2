package d2raster

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

type preparedPaintKind uint8

const (
	preparedSolidPaint preparedPaintKind = iota
	preparedLinearGradient
	preparedLinearXGradient
	preparedLinearYGradient
	preparedLinearXOnlyGradient
	preparedLinearYOnlyGradient
	preparedRadialGradient
	preparedPatternPaint
)

// preparedPaint is immutable render-time paint state. Solid colors retain the
// scanline rasterizer's optimized uniform-color path. Gradients carry the
// inverse of their complete coordinate mapping. Patterns carry one prepared,
// bounded tile that is rasterized once per frame before repeated sampling.
type preparedPaint struct {
	kind     preparedPaintKind
	solid    color.NRGBA
	gradient preparedGradient
	pattern  *preparedPattern
}

type preparedGradient struct {
	deviceToGradient d2scene.Matrix
	stops            []d2scene.GradientStop
	spread           d2scene.SpreadMethod
	// COLRv1 gradients use the OpenType-mandated premultiplied linear-light
	// interpolation. SVG gradients retain their existing sRGB behavior.
	colrv1Interpolation bool

	linearStart       d2scene.Point
	linearDelta       d2scene.Point
	linearDenominator float64

	radialFocal       d2scene.Point
	radialDelta       d2scene.Point
	radialFocalRadius float64
	radialDeltaRadius float64
	radialA           float64
	radialTangent     bool
	radialConcentric  bool
	radialAverage     color.NRGBA
}

type patternPaintPreparer func(d2scene.PatternPaint, d2scene.Box, d2scene.Matrix) (*preparedPaint, error)

func prepareAnimatedPaintWithPattern(paint d2scene.Paint, animatedColor *color.NRGBA, objectBounds d2scene.Box, objectToDevice d2scene.Matrix, preparePattern patternPaintPreparer) (*preparedPaint, error) {
	switch paint := paint.(type) {
	case nil:
		if animatedColor != nil {
			return nil, fmt.Errorf("color animation targets missing paint")
		}
		return nil, nil
	case d2scene.SolidPaint:
		value := paint.Color
		if animatedColor != nil {
			value = *animatedColor
		}
		return &preparedPaint{kind: preparedSolidPaint, solid: value}, nil
	case *d2scene.SolidPaint:
		if paint == nil {
			if animatedColor != nil {
				return nil, fmt.Errorf("color animation targets missing paint")
			}
			return nil, nil
		}
		return prepareAnimatedPaintWithPattern(*paint, animatedColor, objectBounds, objectToDevice, preparePattern)
	case d2scene.LinearGradient:
		if animatedColor != nil {
			return nil, fmt.Errorf("color animation targets non-solid paint")
		}
		return prepareLinearGradient(paint, objectBounds, objectToDevice)
	case *d2scene.LinearGradient:
		if paint == nil {
			return nil, fmt.Errorf("nil linear gradient")
		}
		return prepareAnimatedPaintWithPattern(*paint, animatedColor, objectBounds, objectToDevice, preparePattern)
	case d2scene.RadialGradient:
		if animatedColor != nil {
			return nil, fmt.Errorf("color animation targets non-solid paint")
		}
		return prepareRadialGradient(paint, objectBounds, objectToDevice)
	case *d2scene.RadialGradient:
		if paint == nil {
			return nil, fmt.Errorf("nil radial gradient")
		}
		return prepareAnimatedPaintWithPattern(*paint, animatedColor, objectBounds, objectToDevice, preparePattern)
	case d2scene.PatternPaint:
		if animatedColor != nil {
			return nil, fmt.Errorf("color animation targets pattern paint")
		}
		if preparePattern == nil {
			return nil, fmt.Errorf("unsupported pattern paint without render preflight")
		}
		return preparePattern(paint, objectBounds, objectToDevice)
	case *d2scene.PatternPaint:
		if paint == nil {
			return nil, fmt.Errorf("nil pattern paint")
		}
		return prepareAnimatedPaintWithPattern(*paint, animatedColor, objectBounds, objectToDevice, preparePattern)
	default:
		return nil, fmt.Errorf("unsupported paint %T", paint)
	}
}

func prepareLinearGradient(gradient d2scene.LinearGradient, objectBounds d2scene.Box, objectToDevice d2scene.Matrix) (*preparedPaint, error) {
	if !finitePoint(gradient.Start) || !finitePoint(gradient.End) {
		return nil, fmt.Errorf("linear gradient has non-finite geometry")
	}
	deviceToGradient, err := prepareGradientTransform(gradient.Units, gradient.Transform, objectBounds, objectToDevice)
	if err != nil {
		return nil, fmt.Errorf("linear gradient: %w", err)
	}
	stops, err := normalizeGradientStops(gradient.Stops)
	if err != nil {
		return nil, fmt.Errorf("linear gradient: %w", err)
	}
	if gradient.Spread > d2scene.SpreadRepeat {
		return nil, fmt.Errorf("linear gradient has invalid spread method %d", gradient.Spread)
	}
	delta := d2scene.Point{X: gradient.End.X - gradient.Start.X, Y: gradient.End.Y - gradient.Start.Y}
	denominator := delta.X*delta.X + delta.Y*delta.Y
	if !finite(denominator) {
		return nil, fmt.Errorf("linear gradient vector is outside the finite numeric domain")
	}
	// SVG 2 paints a zero-length linear gradient with its last stop color.
	if denominator == 0 || len(stops) == 1 {
		return &preparedPaint{kind: preparedSolidPaint, solid: stops[len(stops)-1].Color}, nil
	}
	kind := preparedLinearGradient
	if delta.Y == 0 && affineCoordinateFiniteForRaster(deviceToGradient.B, deviceToGradient.D, deviceToGradient.F, gradient.Start.Y) {
		if deviceToGradient.C == 0 {
			kind = preparedLinearXOnlyGradient
		} else {
			kind = preparedLinearXGradient
		}
	} else if delta.X == 0 && affineCoordinateFiniteForRaster(deviceToGradient.A, deviceToGradient.C, deviceToGradient.E, gradient.Start.X) {
		if deviceToGradient.B == 0 {
			kind = preparedLinearYOnlyGradient
		} else {
			kind = preparedLinearYGradient
		}
	}
	return &preparedPaint{
		kind: kind,
		gradient: preparedGradient{
			deviceToGradient:  deviceToGradient,
			stops:             stops,
			spread:            gradient.Spread,
			linearStart:       gradient.Start,
			linearDelta:       delta,
			linearDenominator: denominator,
		},
	}, nil
}

// Axis-gradient sampling can omit the coordinate perpendicular to its axis,
// but the general expression remains unpainted if that coordinate overflows:
// infinity multiplied by the zero gradient delta produces NaN. Only select an
// axis fast path when every coordinate representable by an image.Rectangle is
// conservatively guaranteed to keep that otherwise-unused subtraction finite.
func affineCoordinateFiniteForRaster(a, b, offset, origin float64) bool {
	maximumCoordinate := float64(^uint(0) >> 1)
	// Keep the absolute bound far enough below MaxFloat64 that rounding in
	// the multiply-add sequence cannot turn an accepted bound into infinity.
	remaining := math.MaxFloat64 / 4
	for _, term := range [...]struct {
		factor float64
		scale  float64
	}{
		{factor: a, scale: maximumCoordinate},
		{factor: b, scale: maximumCoordinate},
		{factor: offset, scale: 1},
		{factor: origin, scale: 1},
	} {
		magnitude := math.Abs(term.factor)
		if magnitude > remaining/term.scale {
			return false
		}
		remaining -= magnitude * term.scale
	}
	return true
}

func prepareRadialGradient(gradient d2scene.RadialGradient, objectBounds d2scene.Box, objectToDevice d2scene.Matrix) (*preparedPaint, error) {
	if !finitePoint(gradient.Center) || !finitePoint(gradient.Focal) || !finite(gradient.Radius) || !finite(gradient.FocalRadius) {
		return nil, fmt.Errorf("radial gradient has non-finite geometry")
	}
	if gradient.Radius < 0 || gradient.FocalRadius < 0 {
		return nil, fmt.Errorf("radial gradient has negative radius")
	}
	deviceToGradient, err := prepareGradientTransform(gradient.Units, gradient.Transform, objectBounds, objectToDevice)
	if err != nil {
		return nil, fmt.Errorf("radial gradient: %w", err)
	}
	stops, err := normalizeGradientStops(gradient.Stops)
	if err != nil {
		return nil, fmt.Errorf("radial gradient: %w", err)
	}
	if gradient.Spread > d2scene.SpreadRepeat {
		return nil, fmt.Errorf("radial gradient has invalid spread method %d", gradient.Spread)
	}
	if len(stops) == 1 {
		return &preparedPaint{kind: preparedSolidPaint, solid: stops[0].Color}, nil
	}
	delta := d2scene.Point{X: gradient.Center.X - gradient.Focal.X, Y: gradient.Center.Y - gradient.Focal.Y}
	deltaRadius := gradient.Radius - gradient.FocalRadius
	a := delta.X*delta.X + delta.Y*delta.Y - deltaRadius*deltaRadius
	if !finite(a) || !finite(deltaRadius) {
		return nil, fmt.Errorf("radial gradient cone is outside the finite numeric domain")
	}
	// SVG 2 defines fully overlapping start and end circles as unpainted.
	if delta.X == 0 && delta.Y == 0 && deltaRadius == 0 {
		return &preparedPaint{kind: preparedSolidPaint, solid: color.NRGBA{}}, nil
	}
	aScale := delta.X*delta.X + delta.Y*delta.Y + deltaRadius*deltaRadius + 1
	return &preparedPaint{
		kind: preparedRadialGradient,
		gradient: preparedGradient{
			deviceToGradient:  deviceToGradient,
			stops:             stops,
			spread:            gradient.Spread,
			radialFocal:       gradient.Focal,
			radialDelta:       delta,
			radialFocalRadius: gradient.FocalRadius,
			radialDeltaRadius: deltaRadius,
			radialA:           a,
			radialTangent:     math.Abs(a) <= 1e-14*aScale,
			radialConcentric:  delta.X == 0 && delta.Y == 0 && gradient.FocalRadius == 0,
			radialAverage:     averageGradientColor(stops),
		},
	}, nil
}

func prepareGradientTransform(units d2scene.PaintUnits, gradientTransform d2scene.Matrix, objectBounds d2scene.Box, objectToDevice d2scene.Matrix) (d2scene.Matrix, error) {
	if units > d2scene.UserSpaceOnUse {
		return d2scene.Matrix{}, fmt.Errorf("invalid paint units %d", units)
	}
	if !gradientTransform.IsFinite() {
		return d2scene.Matrix{}, fmt.Errorf("non-finite gradient transform")
	}
	if !objectToDevice.IsFinite() {
		return d2scene.Matrix{}, fmt.Errorf("non-finite object transform")
	}
	unitsToObject := d2scene.Identity()
	if units == d2scene.ObjectBoundingBox {
		if err := validateBox(objectBounds); err != nil {
			return d2scene.Matrix{}, fmt.Errorf("invalid object bounding box: %w", err)
		}
		if objectBounds.Width == 0 || objectBounds.Height == 0 {
			// SVG ignores objectBoundingBox paint on zero-area geometry. The
			// scene contract rejects it instead so malformed scenes cannot
			// silently lose paint.
			return d2scene.Matrix{}, fmt.Errorf("object bounding box has zero width or height")
		}
		unitsToObject = d2scene.Translate(objectBounds.X, objectBounds.Y).Mul(d2scene.Scale(objectBounds.Width, objectBounds.Height))
	}
	// gradientTransform is post-multiplied to the units transform, matching
	// SVG: object transform * bbox transform * gradient transform.
	gradientToDevice := objectToDevice.Mul(unitsToObject).Mul(gradientTransform)
	if !gradientToDevice.IsFinite() {
		return d2scene.Matrix{}, fmt.Errorf("composed gradient transform is non-finite")
	}
	inverse, invertible, err := finiteAffineInverse(gradientToDevice)
	if err != nil || !invertible {
		return d2scene.Matrix{}, fmt.Errorf("singular gradient transform")
	}
	return inverse, nil
}

// normalizeGradientStops implements SVG's used-value rules in source order:
// offsets are clamped to [0,1], then any decreasing offset is raised to the
// previous used offset. Equal offsets are retained because the later stop owns
// the exact transition point.
func normalizeGradientStops(stops []d2scene.GradientStop) ([]d2scene.GradientStop, error) {
	if len(stops) == 0 {
		return nil, fmt.Errorf("gradient has no stops")
	}
	alreadyNormalized := true
	previous := 0.0
	for index, stop := range stops {
		if !finite(stop.Offset) {
			return nil, fmt.Errorf("gradient stop %d has non-finite offset", index)
		}
		usedOffset := math.Max(0, math.Min(1, stop.Offset))
		if index != 0 && usedOffset < previous {
			usedOffset = previous
		}
		alreadyNormalized = alreadyNormalized && usedOffset == stop.Offset
		previous = usedOffset
	}
	if alreadyNormalized {
		// Scene assets are immutable for the lifetime of a render, so a slice
		// that already contains the SVG used values needs no defensive copy.
		return stops, nil
	}

	normalized := make([]d2scene.GradientStop, len(stops))
	previous = 0
	for index, stop := range stops {
		stop.Offset = math.Max(0, math.Min(1, stop.Offset))
		if index != 0 && stop.Offset < previous {
			stop.Offset = previous
		}
		normalized[index] = stop
		previous = stop.Offset
	}
	return normalized, nil
}

func (paint *preparedPaint) colorAt(x, y float64) (color.NRGBA, bool) {
	if paint == nil {
		return color.NRGBA{}, false
	}
	if paint.kind == preparedSolidPaint {
		return paint.solid, paint.solid.A != 0
	}
	if paint.kind == preparedPatternPaint {
		return paint.pattern.colorAt(x, y)
	}
	gradient := &paint.gradient
	var parameter float64
	var ok bool
	switch paint.kind {
	case preparedLinearGradient:
		point := gradient.deviceToGradient.Point(d2scene.Point{X: x, Y: y})
		parameter = ((point.X-gradient.linearStart.X)*gradient.linearDelta.X + (point.Y-gradient.linearStart.Y)*gradient.linearDelta.Y) / gradient.linearDenominator
		ok = finite(parameter)
	case preparedLinearXGradient:
		pointX := gradient.deviceToGradient.A*x + gradient.deviceToGradient.C*y + gradient.deviceToGradient.E
		parameter = (pointX - gradient.linearStart.X) * gradient.linearDelta.X / gradient.linearDenominator
		ok = finite(parameter)
	case preparedLinearYGradient:
		pointY := gradient.deviceToGradient.B*x + gradient.deviceToGradient.D*y + gradient.deviceToGradient.F
		parameter = (pointY - gradient.linearStart.Y) * gradient.linearDelta.Y / gradient.linearDenominator
		ok = finite(parameter)
	case preparedLinearXOnlyGradient:
		pointX := gradient.deviceToGradient.A*x + gradient.deviceToGradient.E
		parameter = (pointX - gradient.linearStart.X) * gradient.linearDelta.X / gradient.linearDenominator
		ok = finite(parameter)
	case preparedLinearYOnlyGradient:
		pointY := gradient.deviceToGradient.D*y + gradient.deviceToGradient.F
		parameter = (pointY - gradient.linearStart.Y) * gradient.linearDelta.Y / gradient.linearDenominator
		ok = finite(parameter)
	case preparedRadialGradient:
		point := gradient.deviceToGradient.Point(d2scene.Point{X: x, Y: y})
		parameter, ok = gradient.radialParameter(point)
		if !ok && gradient.spread == d2scene.SpreadRepeat && gradient.radialIsTangent() {
			return gradient.radialAverage, gradient.radialAverage.A != 0
		}
	default:
		return color.NRGBA{}, false
	}
	if !ok {
		return color.NRGBA{}, false
	}
	parameter = spreadParameter(parameter, gradient.spread)
	if gradient.colrv1Interpolation {
		return interpolateCOLRv1GradientStops(gradient.stops, parameter), true
	}
	return interpolateGradientStops(gradient.stops, parameter), true
}

// radialParameter follows the SVG 2 / Canvas two-circle cone model. It solves
// |P - (F+t(C-F))| = fr+t(r-fr), choosing the first solution whose
// interpolated radius is non-negative, as Pixman's radial-gradient algorithm
// does. Points outside the cone return false (transparent black).
func (gradient *preparedGradient) radialParameter(point d2scene.Point) (float64, bool) {
	qx := point.X - gradient.radialFocal.X
	qy := point.Y - gradient.radialFocal.Y
	c := qx*qx + qy*qy - gradient.radialFocalRadius*gradient.radialFocalRadius
	a := gradient.radialA
	if gradient.radialConcentric {
		root := math.Sqrt(0 - a*c)
		for _, t := range [...]float64{root / a, -root / a} {
			if gradient.radialSolutionValid(t) {
				return t, true
			}
		}
		return 0, false
	}
	b := qx*gradient.radialDelta.X + qy*gradient.radialDelta.Y + gradient.radialFocalRadius*gradient.radialDeltaRadius
	if gradient.radialTangent {
		bScale := math.Abs(qx*gradient.radialDelta.X) + math.Abs(qy*gradient.radialDelta.Y) + math.Abs(gradient.radialFocalRadius*gradient.radialDeltaRadius) + 1
		if math.Abs(b) <= 1e-14*bScale {
			return 0, false
		}
		t := 0.5 * c / b
		if gradient.radialSolutionValid(t) {
			return t, true
		}
		return 0, false
	}
	discriminant := b*b - a*c
	if discriminant < 0 {
		discriminantScale := math.Abs(b*b) + math.Abs(a*c) + 1
		if discriminant >= -1e-14*discriminantScale {
			discriminant = 0
		} else {
			return 0, false
		}
	}
	root := math.Sqrt(discriminant)
	for _, t := range [...]float64{(b + root) / a, (b - root) / a} {
		if gradient.radialSolutionValid(t) {
			return t, true
		}
	}
	return 0, false
}

func (gradient *preparedGradient) radialSolutionValid(t float64) bool {
	if !finite(t) {
		return false
	}
	radius := gradient.radialFocalRadius + t*gradient.radialDeltaRadius
	return finite(radius) && radius >= -1e-12
}

func (gradient *preparedGradient) radialIsTangent() bool {
	return gradient.radialTangent
}

func spreadParameter(value float64, spread d2scene.SpreadMethod) float64 {
	switch spread {
	case d2scene.SpreadPad:
		return math.Max(0, math.Min(1, value))
	case d2scene.SpreadReflect:
		// math.Mod is comparatively expensive. Values in the nearest period
		// are already their own remainder, so preserve the exact arithmetic
		// that follows Mod without calling it.
		if value > -2 && value < 2 {
			if value < 0 {
				value += 2
			}
			if value > 1 {
				value = 2 - value
			}
			return value
		}
		value = math.Mod(value, 2)
		if value < 0 {
			value += 2
		}
		if value > 1 {
			value = 2 - value
		}
		return value
	case d2scene.SpreadRepeat:
		if value == 0 {
			return 0
		}
		if value > 0 && value < 1 {
			return value
		}
		return value - math.Floor(value)
	default:
		return value
	}
}

func interpolateGradientStops(stops []d2scene.GradientStop, value float64) color.NRGBA {
	if len(stops) == 1 || value < stops[0].Offset {
		return stops[0].Color
	}
	if value >= stops[len(stops)-1].Offset {
		return stops[len(stops)-1].Color
	}
	var left, right d2scene.GradientStop
	switch len(stops) {
	case 2:
		left, right = stops[0], stops[1]
	case 3:
		if stops[1].Offset <= value {
			left, right = stops[1], stops[2]
		} else {
			left, right = stops[0], stops[1]
		}
	default:
		// Upper-bound search makes the last stop at a repeated offset own
		// the exact transition point, while the first repeated stop owns the
		// approach from the left.
		low, high := 0, len(stops)
		for low < high {
			middle := low + (high-low)/2
			if stops[middle].Offset <= value {
				low = middle + 1
			} else {
				high = middle
			}
		}
		left, right = stops[low-1], stops[low]
	}
	amount := (value - left.Offset) / (right.Offset - left.Offset)
	return lerpNRGBA(left.Color, right.Color, amount)
}

func lerpNRGBA(left, right color.NRGBA, amount float64) color.NRGBA {
	lerp := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a) + (float64(b)-float64(a))*amount))
	}
	return color.NRGBA{
		R: lerp(left.R, right.R),
		G: lerp(left.G, right.G),
		B: lerp(left.B, right.B),
		A: lerp(left.A, right.A),
	}
}

// interpolateCOLRv1GradientStops implements the color interpolation required
// by OpenType CPAL/COLR: decode sRGB to linear light, premultiply each color by
// alpha, interpolate, unpremultiply, then encode back to sRGB.
func interpolateCOLRv1GradientStops(stops []d2scene.GradientStop, value float64) color.NRGBA {
	if len(stops) == 1 || value < stops[0].Offset {
		return stops[0].Color
	}
	if value >= stops[len(stops)-1].Offset {
		return stops[len(stops)-1].Color
	}
	var left, right d2scene.GradientStop
	switch len(stops) {
	case 2:
		left, right = stops[0], stops[1]
	case 3:
		if stops[1].Offset <= value {
			left, right = stops[1], stops[2]
		} else {
			left, right = stops[0], stops[1]
		}
	default:
		low, high := 0, len(stops)
		for low < high {
			middle := low + (high-low)/2
			if stops[middle].Offset <= value {
				low = middle + 1
			} else {
				high = middle
			}
		}
		left, right = stops[low-1], stops[low]
	}
	amount := (value - left.Offset) / (right.Offset - left.Offset)
	return lerpCOLRv1Color(left.Color, right.Color, amount)
}

func lerpCOLRv1Color(left, right color.NRGBA, amount float64) color.NRGBA {
	leftAlpha := float64(left.A) / 255
	rightAlpha := float64(right.A) / 255
	alpha := leftAlpha + (rightAlpha-leftAlpha)*amount
	result := color.NRGBA{A: roundedByte(alpha * 255)}
	if alpha <= 0 {
		return result
	}
	interpolate := func(leftChannel, rightChannel uint8) uint8 {
		leftLinear := linearSRGBByte[leftChannel] * leftAlpha
		rightLinear := linearSRGBByte[rightChannel] * rightAlpha
		premultiplied := leftLinear + (rightLinear-leftLinear)*amount
		return roundedByte(linearToSRGB(premultiplied/alpha) * 255)
	}
	result.R = interpolate(left.R, right.R)
	result.G = interpolate(left.G, right.G)
	result.B = interpolate(left.B, right.B)
	return result
}

func averageCOLRv1GradientColor(stops []d2scene.GradientStop) color.NRGBA {
	if len(stops) == 0 {
		return color.NRGBA{}
	}
	totals := [4]float64{}
	components := func(value color.NRGBA) [4]float64 {
		alpha := float64(value.A) / 255
		return [4]float64{
			linearSRGBByte[value.R] * alpha,
			linearSRGBByte[value.G] * alpha,
			linearSRGBByte[value.B] * alpha,
			alpha,
		}
	}
	first := components(stops[0].Color)
	for channel := range totals {
		totals[channel] += first[channel] * stops[0].Offset
	}
	for index := 1; index < len(stops); index++ {
		width := stops[index].Offset - stops[index-1].Offset
		left, right := components(stops[index-1].Color), components(stops[index].Color)
		for channel := range totals {
			totals[channel] += width * (left[channel] + right[channel]) / 2
		}
	}
	last := components(stops[len(stops)-1].Color)
	for channel := range totals {
		totals[channel] += last[channel] * (1 - stops[len(stops)-1].Offset)
	}
	result := color.NRGBA{A: roundedByte(totals[3] * 255)}
	if totals[3] <= 0 {
		return result
	}
	result.R = roundedByte(linearToSRGB(totals[0]/totals[3]) * 255)
	result.G = roundedByte(linearToSRGB(totals[1]/totals[3]) * 255)
	result.B = roundedByte(linearToSRGB(totals[2]/totals[3]) * 255)
	return result
}

func averageGradientColor(stops []d2scene.GradientStop) color.NRGBA {
	if len(stops) == 0 {
		return color.NRGBA{}
	}
	var totals [4]float64
	channels := func(c color.NRGBA) [4]float64 {
		return [4]float64{float64(c.R), float64(c.G), float64(c.B), float64(c.A)}
	}
	first := channels(stops[0].Color)
	for channel := range totals {
		totals[channel] += first[channel] * stops[0].Offset
	}
	for index := 1; index < len(stops); index++ {
		width := stops[index].Offset - stops[index-1].Offset
		left, right := channels(stops[index-1].Color), channels(stops[index].Color)
		for channel := range totals {
			totals[channel] += width * (left[channel] + right[channel]) / 2
		}
	}
	last := channels(stops[len(stops)-1].Color)
	for channel := range totals {
		totals[channel] += last[channel] * (1 - stops[len(stops)-1].Offset)
	}
	return color.NRGBA{R: uint8(math.Round(totals[0])), G: uint8(math.Round(totals[1])), B: uint8(math.Round(totals[2])), A: uint8(math.Round(totals[3]))}
}

// drawRasterizedPaint streams analytic non-zero coverage directly into paint
// compositing. Unlike even-odd fills and clip/mask operations, ordinary paint
// does not need to retain coverage after a row has been consumed.
func drawRasterizedPaint(
	ctx context.Context,
	dst *image.RGBA,
	bounds image.Rectangle,
	paint *preparedPaint,
	scratch *rasterScratch,
	scanlinePurpose string,
	populate func(*scanline.Rasterizer) error,
) error {
	if bounds.Empty() {
		return nil
	}
	// Pattern tiles can themselves use the shared rasterizer, so render them
	// before populating the outer path.
	if err := paint.ensureRendered(ctx, scratch); err != nil {
		return err
	}
	rasterizer := scratch.reset(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	if err := populate(rasterizer); err != nil {
		return err
	}
	if err := drawPaintCoverage(ctx, dst, bounds, rasterizer, scratch.workBudget(), paint); err != nil {
		return fmt.Errorf("d2raster: %s: %w", scanlinePurpose, err)
	}
	return ctx.Err()
}

func drawPaintCoverage(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, rasterizer *scanline.Rasterizer, budget *scanline.WorkBudget, paint *preparedPaint) error {
	switch paint.kind {
	case preparedLinearGradient:
		return drawLinearGradientCoverage(ctx, dst, bounds, rasterizer, budget, &paint.gradient)
	case preparedLinearXGradient, preparedLinearYGradient, preparedLinearXOnlyGradient, preparedLinearYOnlyGradient:
		return drawAxisLinearGradientCoverage(ctx, dst, bounds, rasterizer, budget, &paint.gradient, paint.kind)
	case preparedRadialGradient:
		return drawRadialGradientCoverage(ctx, dst, bounds, rasterizer, budget, &paint.gradient)
	case preparedPatternPaint:
		if paint.pattern != nil && paint.pattern.directPixels {
			return drawDirectPatternCoverage(ctx, dst, bounds, rasterizer, budget, paint.pattern)
		}
	}
	return drawGeneralPaintCoverage(ctx, dst, bounds, rasterizer, budget, paint)
}

func drawGeneralPaintCoverage(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, rasterizer *scanline.Rasterizer, budget *scanline.WorkBudget, paint *preparedPaint) error {
	return rasterizer.WalkCoverage(ctx, budget, func(localY, minX int, partial, difference []float32) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		dstY := bounds.Min.Y + localY
		dstOffset := dst.PixOffset(bounds.Min.X+minX, dstY)
		var winding float32
		for index := range partial {
			if index != 0 && index&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			winding += difference[index]
			coverage := scanline.QuantizeCoverage(partial[index] + winding)
			if coverage == 0 {
				continue
			}
			dstX := bounds.Min.X + minX + index
			sample, ok := paint.colorAt(float64(dstX)+0.5, float64(dstY)+0.5)
			if !ok || sample.A == 0 {
				continue
			}
			pixelOffset := dstOffset + index*4
			compositeNRGBAOverRGBA(dst.Pix[pixelOffset:pixelOffset+4], sample, coverage)
		}
		return nil
	})
}

func drawDirectPatternCoverage(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, rasterizer *scanline.Rasterizer, budget *scanline.WorkBudget, pattern *preparedPattern) error {
	tile := pattern.tileResource
	if tile == nil || tile.image == nil {
		return drawGeneralPaintCoverage(ctx, dst, bounds, rasterizer, budget, &preparedPaint{kind: preparedPatternPaint, pattern: pattern})
	}
	wrapped := func(value, offset, period int) int {
		value %= period
		if value < 0 {
			value += period
		}
		if value >= period-offset {
			return value - (period - offset)
		}
		return value + offset
	}
	return rasterizer.WalkCoverage(ctx, budget, func(localY, minX int, partial, difference []float32) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		dstY := bounds.Min.Y + localY
		dstOffset := dst.PixOffset(bounds.Min.X+minX, dstY)
		pixelX := wrapped(bounds.Min.X+minX, pattern.pixelOffsetX, tile.width)
		pixelY := wrapped(dstY, pattern.pixelOffsetY, tile.height)
		var winding float32
		for index := range partial {
			if index != 0 && index&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			winding += difference[index]
			coverage := scanline.QuantizeCoverage(partial[index] + winding)
			if coverage != 0 {
				sourceOffset := tile.image.PixOffset(pixelX, pixelY)
				source := tile.image.Pix[sourceOffset : sourceOffset+4]
				alpha := source[3]
				if alpha != 0 {
					pixelOffset := dstOffset + index*4
					destination := dst.Pix[pixelOffset : pixelOffset+4]
					if alpha == 0xff && coverage == 0xff {
						destination[0], destination[1], destination[2], destination[3] = source[0], source[1], source[2], 0xff
					} else {
						var sample color.NRGBA
						if alpha == 0xff {
							sample = color.NRGBA{R: source[0], G: source[1], B: source[2], A: alpha}
						} else {
							sample = color.NRGBA{
								R: uint8((uint32(source[0]) * 0xffff / uint32(alpha)) >> 8),
								G: uint8((uint32(source[1]) * 0xffff / uint32(alpha)) >> 8),
								B: uint8((uint32(source[2]) * 0xffff / uint32(alpha)) >> 8),
								A: alpha,
							}
						}
						compositeNRGBAOverRGBA(destination, sample, coverage)
					}
				}
			}
			pixelX++
			if pixelX == tile.width {
				pixelX = 0
			}
		}
		return nil
	})
}

func drawAxisLinearGradientCoverage(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, rasterizer *scanline.Rasterizer, budget *scanline.WorkBudget, gradient *preparedGradient, kind preparedPaintKind) error {
	if kind == preparedLinearYOnlyGradient {
		return drawLinearYOnlyGradientCoverage(ctx, dst, bounds, rasterizer, budget, gradient)
	}
	var xCoefficient, yCoefficient, offset, start, delta float64
	switch kind {
	case preparedLinearXGradient:
		xCoefficient, yCoefficient, offset = gradient.deviceToGradient.A, gradient.deviceToGradient.C, gradient.deviceToGradient.E
		start, delta = gradient.linearStart.X, gradient.linearDelta.X
	case preparedLinearYGradient:
		xCoefficient, yCoefficient, offset = gradient.deviceToGradient.B, gradient.deviceToGradient.D, gradient.deviceToGradient.F
		start, delta = gradient.linearStart.Y, gradient.linearDelta.Y
	case preparedLinearXOnlyGradient:
		xCoefficient, offset = gradient.deviceToGradient.A, gradient.deviceToGradient.E
		start, delta = gradient.linearStart.X, gradient.linearDelta.X
	default:
		return drawGeneralPaintCoverage(ctx, dst, bounds, rasterizer, budget, &preparedPaint{kind: kind, gradient: *gradient})
	}
	return rasterizer.WalkCoverage(ctx, budget, func(localY, minX int, partial, difference []float32) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		dstY := bounds.Min.Y + localY
		dstOffset := dst.PixOffset(bounds.Min.X+minX, dstY)
		y := float64(dstY) + 0.5
		var winding float32
		for index := range partial {
			if index != 0 && index&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			winding += difference[index]
			coverage := scanline.QuantizeCoverage(partial[index] + winding)
			if coverage == 0 {
				continue
			}
			dstX := bounds.Min.X + minX + index
			x := float64(dstX) + 0.5
			coordinate := xCoefficient*x + yCoefficient*y + offset
			parameter := (coordinate - start) * delta / gradient.linearDenominator
			if !finite(parameter) {
				continue
			}
			parameter = spreadParameter(parameter, gradient.spread)
			var sample color.NRGBA
			if gradient.colrv1Interpolation {
				sample = interpolateCOLRv1GradientStops(gradient.stops, parameter)
			} else {
				sample = interpolateGradientStops(gradient.stops, parameter)
			}
			if sample.A == 0 {
				continue
			}
			pixelOffset := dstOffset + index*4
			compositeNRGBAOverRGBA(dst.Pix[pixelOffset:pixelOffset+4], sample, coverage)
		}
		return nil
	})
}

func drawLinearGradientCoverage(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, rasterizer *scanline.Rasterizer, budget *scanline.WorkBudget, gradient *preparedGradient) error {
	return rasterizer.WalkCoverage(ctx, budget, func(localY, minX int, partial, difference []float32) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		dstY := bounds.Min.Y + localY
		dstOffset := dst.PixOffset(bounds.Min.X+minX, dstY)
		y := float64(dstY) + 0.5
		var winding float32
		for index := range partial {
			if index != 0 && index&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			winding += difference[index]
			coverage := scanline.QuantizeCoverage(partial[index] + winding)
			if coverage == 0 {
				continue
			}
			dstX := bounds.Min.X + minX + index
			x := float64(dstX) + 0.5
			point := gradient.deviceToGradient.Point(d2scene.Point{X: x, Y: y})
			parameter := ((point.X-gradient.linearStart.X)*gradient.linearDelta.X + (point.Y-gradient.linearStart.Y)*gradient.linearDelta.Y) / gradient.linearDenominator
			if !finite(parameter) {
				continue
			}
			parameter = spreadParameter(parameter, gradient.spread)
			var sample color.NRGBA
			if gradient.colrv1Interpolation {
				sample = interpolateCOLRv1GradientStops(gradient.stops, parameter)
			} else {
				sample = interpolateGradientStops(gradient.stops, parameter)
			}
			if sample.A == 0 {
				continue
			}
			pixelOffset := dstOffset + index*4
			compositeNRGBAOverRGBA(dst.Pix[pixelOffset:pixelOffset+4], sample, coverage)
		}
		return nil
	})
}

func drawLinearYOnlyGradientCoverage(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, rasterizer *scanline.Rasterizer, budget *scanline.WorkBudget, gradient *preparedGradient) error {
	return rasterizer.WalkCoverage(ctx, budget, func(localY, minX int, partial, difference []float32) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		dstY := bounds.Min.Y + localY
		dstOffset := dst.PixOffset(bounds.Min.X+minX, dstY)
		y := float64(dstY) + 0.5
		coordinate := gradient.deviceToGradient.D*y + gradient.deviceToGradient.F
		parameter := (coordinate - gradient.linearStart.Y) * gradient.linearDelta.Y / gradient.linearDenominator
		validSample := finite(parameter)
		var sample color.NRGBA
		if validSample {
			parameter = spreadParameter(parameter, gradient.spread)
			if gradient.colrv1Interpolation {
				sample = interpolateCOLRv1GradientStops(gradient.stops, parameter)
			} else {
				sample = interpolateGradientStops(gradient.stops, parameter)
			}
			validSample = sample.A != 0
		}
		var winding float32
		for index := range partial {
			if index != 0 && index&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			winding += difference[index]
			coverage := scanline.QuantizeCoverage(partial[index] + winding)
			if coverage == 0 || !validSample {
				continue
			}
			pixelOffset := dstOffset + index*4
			compositeNRGBAOverRGBA(dst.Pix[pixelOffset:pixelOffset+4], sample, coverage)
		}
		return nil
	})
}

func drawRadialGradientCoverage(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, rasterizer *scanline.Rasterizer, budget *scanline.WorkBudget, gradient *preparedGradient) error {
	return rasterizer.WalkCoverage(ctx, budget, func(localY, minX int, partial, difference []float32) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		dstY := bounds.Min.Y + localY
		dstOffset := dst.PixOffset(bounds.Min.X+minX, dstY)
		y := float64(dstY) + 0.5
		var winding float32
		for index := range partial {
			if index != 0 && index&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			winding += difference[index]
			coverage := scanline.QuantizeCoverage(partial[index] + winding)
			if coverage == 0 {
				continue
			}
			dstX := bounds.Min.X + minX + index
			x := float64(dstX) + 0.5
			point := d2scene.Point{
				X: gradient.deviceToGradient.A*x + gradient.deviceToGradient.C*y + gradient.deviceToGradient.E,
				Y: gradient.deviceToGradient.B*x + gradient.deviceToGradient.D*y + gradient.deviceToGradient.F,
			}
			parameter, ok := gradient.radialParameter(point)
			var sample color.NRGBA
			useAverage := false
			if !ok && gradient.spread == d2scene.SpreadRepeat && gradient.radialIsTangent() {
				sample = gradient.radialAverage
				ok, useAverage = sample.A != 0, true
			}
			if !ok {
				continue
			}
			if !useAverage {
				parameter = spreadParameter(parameter, gradient.spread)
				if gradient.colrv1Interpolation {
					sample = interpolateCOLRv1GradientStops(gradient.stops, parameter)
				} else {
					sample = interpolateGradientStops(gradient.stops, parameter)
				}
			}
			if sample.A == 0 {
				continue
			}
			pixelOffset := dstOffset + index*4
			compositeNRGBAOverRGBA(dst.Pix[pixelOffset:pixelOffset+4], sample, coverage)
		}
		return nil
	})
}

func drawGradientMask(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, paint *preparedPaint, scratch *rasterScratch, populate func(*image.Alpha) error) error {
	return drawPaintMask(ctx, dst, bounds, paint, scratch, "gradient Alpha mask", populate)
}

func drawPaintMask(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, paint *preparedPaint, scratch *rasterScratch, purpose string, populate func(*image.Alpha) error) error {
	if bounds.Empty() {
		return nil
	}
	if err := paint.ensureRendered(ctx, scratch); err != nil {
		return err
	}
	maskBounds := image.Rect(0, 0, bounds.Dx(), bounds.Dy())
	mask, maskBytes, err := scratch.offscreen.newAlpha(maskBounds, purpose)
	if err != nil {
		return err
	}
	defer scratch.offscreen.recycleAlpha(mask, maskBytes)
	if err := populate(mask); err != nil {
		return err
	}
	// Paint kind is invariant across the mask. Specialized gradient loops keep
	// its dispatch out of the pixel loop; axis-aligned vertical gradients sample
	// only once per row.
	switch paint.kind {
	case preparedLinearGradient:
		return drawLinearGradientMaskPixels(ctx, dst, bounds, mask, &paint.gradient)
	case preparedLinearXGradient, preparedLinearYGradient, preparedLinearXOnlyGradient, preparedLinearYOnlyGradient:
		return drawAxisLinearGradientMaskPixels(ctx, dst, bounds, mask, &paint.gradient, paint.kind)
	case preparedRadialGradient:
		return drawRadialGradientMaskPixels(ctx, dst, bounds, mask, &paint.gradient)
	case preparedPatternPaint:
		if paint.pattern != nil && paint.pattern.directPixels {
			return drawDirectPatternMaskPixels(ctx, dst, bounds, mask, paint.pattern)
		}
	}
	return drawPaintMaskPixels(ctx, dst, bounds, mask, paint)
}

func drawDirectPatternMaskPixels(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha, pattern *preparedPattern) error {
	tile := pattern.tileResource
	if tile == nil || tile.image == nil {
		return drawPaintMaskPixels(ctx, dst, bounds, mask, &preparedPaint{kind: preparedPatternPaint, pattern: pattern})
	}
	wrapped := func(value, offset, period int) int {
		value %= period
		if value < 0 {
			value += period
		}
		// Both operands are in [0, period), so combine them without risking
		// overflow when period is close to the platform integer limit.
		if value >= period-offset {
			return value - (period - offset)
		}
		return value + offset
	}
	pixelY := wrapped(bounds.Min.Y, pattern.pixelOffsetY, tile.height)
	for localY := 0; localY < mask.Bounds().Dy(); localY++ {
		if localY&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		dstY := bounds.Min.Y + localY
		maskOffset := localY * mask.Stride
		dstOffset := dst.PixOffset(bounds.Min.X, dstY)
		pixelX := wrapped(bounds.Min.X, pattern.pixelOffsetX, tile.width)
		for localX := 0; localX < mask.Bounds().Dx(); localX++ {
			if localX&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			coverage := mask.Pix[maskOffset+localX]
			if coverage != 0 {
				sourceOffset := tile.image.PixOffset(pixelX, pixelY)
				source := tile.image.Pix[sourceOffset : sourceOffset+4]
				destination := dst.Pix[dstOffset+localX*4 : dstOffset+localX*4+4]
				alpha := source[3]
				if alpha == 0xff && coverage == 0xff {
					destination[0], destination[1], destination[2], destination[3] = source[0], source[1], source[2], 0xff
				} else if alpha != 0 {
					var sample color.NRGBA
					if alpha == 0xff {
						sample = color.NRGBA{R: source[0], G: source[1], B: source[2], A: alpha}
					} else {
						sample = color.NRGBA{
							R: uint8((uint32(source[0]) * 0xffff / uint32(alpha)) >> 8),
							G: uint8((uint32(source[1]) * 0xffff / uint32(alpha)) >> 8),
							B: uint8((uint32(source[2]) * 0xffff / uint32(alpha)) >> 8),
							A: alpha,
						}
					}
					compositeNRGBAOverRGBA(destination, sample, coverage)
				}
			}
			pixelX++
			if pixelX == tile.width {
				pixelX = 0
			}
		}
		pixelY++
		if pixelY == tile.height {
			pixelY = 0
		}
	}
	return ctx.Err()
}

func drawPaintMaskPixels(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha, paint *preparedPaint) error {
	for localY := 0; localY < mask.Bounds().Dy(); localY++ {
		if localY&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		dstY := bounds.Min.Y + localY
		maskOffset := localY * mask.Stride
		dstOffset := dst.PixOffset(bounds.Min.X, dstY)
		for localX := 0; localX < mask.Bounds().Dx(); localX++ {
			if localX&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			coverage := mask.Pix[maskOffset+localX]
			if coverage == 0 {
				continue
			}
			dstX := bounds.Min.X + localX
			sample, ok := paint.colorAt(float64(dstX)+0.5, float64(dstY)+0.5)
			if !ok || sample.A == 0 {
				continue
			}
			compositeNRGBAOverRGBA(dst.Pix[dstOffset+localX*4:dstOffset+localX*4+4], sample, coverage)
		}
	}
	return ctx.Err()
}

func drawAxisLinearGradientMaskPixels(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha, gradient *preparedGradient, kind preparedPaintKind) error {
	if kind == preparedLinearYOnlyGradient {
		return drawLinearYOnlyGradientMaskPixels(ctx, dst, bounds, mask, gradient)
	}
	var xCoefficient, yCoefficient, offset, start, delta float64
	switch kind {
	case preparedLinearXGradient:
		xCoefficient, yCoefficient, offset = gradient.deviceToGradient.A, gradient.deviceToGradient.C, gradient.deviceToGradient.E
		start, delta = gradient.linearStart.X, gradient.linearDelta.X
	case preparedLinearYGradient:
		xCoefficient, yCoefficient, offset = gradient.deviceToGradient.B, gradient.deviceToGradient.D, gradient.deviceToGradient.F
		start, delta = gradient.linearStart.Y, gradient.linearDelta.Y
	case preparedLinearXOnlyGradient:
		xCoefficient, offset = gradient.deviceToGradient.A, gradient.deviceToGradient.E
		start, delta = gradient.linearStart.X, gradient.linearDelta.X
	default:
		return drawPaintMaskPixels(ctx, dst, bounds, mask, &preparedPaint{kind: kind, gradient: *gradient})
	}
	for localY := 0; localY < mask.Bounds().Dy(); localY++ {
		if localY&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		dstY := bounds.Min.Y + localY
		maskOffset := localY * mask.Stride
		dstOffset := dst.PixOffset(bounds.Min.X, dstY)
		y := float64(dstY) + 0.5
		for localX := 0; localX < mask.Bounds().Dx(); localX++ {
			if localX&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			coverage := mask.Pix[maskOffset+localX]
			if coverage == 0 {
				continue
			}
			dstX := bounds.Min.X + localX
			x := float64(dstX) + 0.5
			coordinate := xCoefficient*x + yCoefficient*y + offset
			parameter := (coordinate - start) * delta / gradient.linearDenominator
			if !finite(parameter) {
				continue
			}
			parameter = spreadParameter(parameter, gradient.spread)
			var sample color.NRGBA
			if gradient.colrv1Interpolation {
				sample = interpolateCOLRv1GradientStops(gradient.stops, parameter)
			} else {
				sample = interpolateGradientStops(gradient.stops, parameter)
			}
			if sample.A == 0 {
				continue
			}
			compositeNRGBAOverRGBA(dst.Pix[dstOffset+localX*4:dstOffset+localX*4+4], sample, coverage)
		}
	}
	return ctx.Err()
}

func drawLinearGradientMaskPixels(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha, gradient *preparedGradient) error {
	for localY := 0; localY < mask.Bounds().Dy(); localY++ {
		if localY&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		dstY := bounds.Min.Y + localY
		maskOffset := localY * mask.Stride
		dstOffset := dst.PixOffset(bounds.Min.X, dstY)
		y := float64(dstY) + 0.5
		for localX := 0; localX < mask.Bounds().Dx(); localX++ {
			if localX&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			coverage := mask.Pix[maskOffset+localX]
			if coverage == 0 {
				continue
			}
			dstX := bounds.Min.X + localX
			x := float64(dstX) + 0.5
			point := gradient.deviceToGradient.Point(d2scene.Point{X: x, Y: y})
			parameter := ((point.X-gradient.linearStart.X)*gradient.linearDelta.X + (point.Y-gradient.linearStart.Y)*gradient.linearDelta.Y) / gradient.linearDenominator
			if !finite(parameter) {
				continue
			}
			parameter = spreadParameter(parameter, gradient.spread)
			var sample color.NRGBA
			if gradient.colrv1Interpolation {
				sample = interpolateCOLRv1GradientStops(gradient.stops, parameter)
			} else {
				sample = interpolateGradientStops(gradient.stops, parameter)
			}
			if sample.A == 0 {
				continue
			}
			compositeNRGBAOverRGBA(dst.Pix[dstOffset+localX*4:dstOffset+localX*4+4], sample, coverage)
		}
	}
	return ctx.Err()
}

func drawLinearYOnlyGradientMaskPixels(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha, gradient *preparedGradient) error {
	for localY := 0; localY < mask.Bounds().Dy(); localY++ {
		if localY&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		dstY := bounds.Min.Y + localY
		maskOffset := localY * mask.Stride
		dstOffset := dst.PixOffset(bounds.Min.X, dstY)
		y := float64(dstY) + 0.5
		coordinate := gradient.deviceToGradient.D*y + gradient.deviceToGradient.F
		parameter := (coordinate - gradient.linearStart.Y) * gradient.linearDelta.Y / gradient.linearDenominator
		validSample := finite(parameter)
		var sample color.NRGBA
		if validSample {
			parameter = spreadParameter(parameter, gradient.spread)
			if gradient.colrv1Interpolation {
				sample = interpolateCOLRv1GradientStops(gradient.stops, parameter)
			} else {
				sample = interpolateGradientStops(gradient.stops, parameter)
			}
			validSample = sample.A != 0
		}
		for localX := 0; localX < mask.Bounds().Dx(); localX++ {
			if localX&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			coverage := mask.Pix[maskOffset+localX]
			if coverage == 0 || !validSample {
				continue
			}
			compositeNRGBAOverRGBA(dst.Pix[dstOffset+localX*4:dstOffset+localX*4+4], sample, coverage)
		}
	}
	return ctx.Err()
}

func drawRadialGradientMaskPixels(ctx context.Context, dst *image.RGBA, bounds image.Rectangle, mask *image.Alpha, gradient *preparedGradient) error {
	for localY := 0; localY < mask.Bounds().Dy(); localY++ {
		if localY&31 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		dstY := bounds.Min.Y + localY
		maskOffset := localY * mask.Stride
		dstOffset := dst.PixOffset(bounds.Min.X, dstY)
		y := float64(dstY) + 0.5
		for localX := 0; localX < mask.Bounds().Dx(); localX++ {
			if localX&4095 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			coverage := mask.Pix[maskOffset+localX]
			if coverage == 0 {
				continue
			}
			dstX := bounds.Min.X + localX
			x := float64(dstX) + 0.5
			point := d2scene.Point{
				X: gradient.deviceToGradient.A*x + gradient.deviceToGradient.C*y + gradient.deviceToGradient.E,
				Y: gradient.deviceToGradient.B*x + gradient.deviceToGradient.D*y + gradient.deviceToGradient.F,
			}
			parameter, ok := gradient.radialParameter(point)
			var sample color.NRGBA
			useAverage := false
			if !ok && gradient.spread == d2scene.SpreadRepeat && gradient.radialIsTangent() {
				sample = gradient.radialAverage
				ok, useAverage = sample.A != 0, true
			}
			if !ok {
				continue
			}
			if !useAverage {
				parameter = spreadParameter(parameter, gradient.spread)
				if gradient.colrv1Interpolation {
					sample = interpolateCOLRv1GradientStops(gradient.stops, parameter)
				} else {
					sample = interpolateGradientStops(gradient.stops, parameter)
				}
			}
			if sample.A == 0 {
				continue
			}
			compositeNRGBAOverRGBA(dst.Pix[dstOffset+localX*4:dstOffset+localX*4+4], sample, coverage)
		}
	}
	return ctx.Err()
}

func compositeNRGBAOverRGBA(destination []byte, source color.NRGBA, coverage uint8) {
	if source.A == 0xff && coverage == 0xff {
		destination[0], destination[1], destination[2], destination[3] = source.R, source.G, source.B, 0xff
		return
	}
	mul255 := func(a, b uint32) uint32 { return (a*b + 127) / 255 }
	sourceAlpha := mul255(uint32(source.A), uint32(coverage))
	if destination[0]|destination[1]|destination[2]|destination[3] == 0 {
		destination[0] = uint8(mul255(uint32(source.R), sourceAlpha))
		destination[1] = uint8(mul255(uint32(source.G), sourceAlpha))
		destination[2] = uint8(mul255(uint32(source.B), sourceAlpha))
		destination[3] = uint8(sourceAlpha)
		return
	}
	inverseAlpha := 255 - sourceAlpha
	for channel, value := range [...]uint8{source.R, source.G, source.B} {
		premultiplied := mul255(uint32(value), sourceAlpha)
		result := premultiplied + mul255(uint32(destination[channel]), inverseAlpha)
		if result > 255 {
			result = 255
		}
		destination[channel] = uint8(result)
	}
	alpha := sourceAlpha + mul255(uint32(destination[3]), inverseAlpha)
	if alpha > 255 {
		alpha = 255
	}
	destination[3] = uint8(alpha)
}

func subpathPixelBounds(paths []subpath, transform d2scene.Matrix, expansion float64, canvas image.Rectangle) image.Rectangle {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, path := range paths {
		for _, local := range path.points {
			point := transform.Point(local)
			minX = math.Min(minX, point.X)
			minY = math.Min(minY, point.Y)
			maxX = math.Max(maxX, point.X)
			maxY = math.Max(maxY, point.Y)
		}
	}
	if math.IsInf(minX, 1) {
		return image.Rectangle{}
	}
	// One extra pixel covers vector antialiasing and flattening tolerance.
	expansion += 1
	minX, minY = minX-expansion, minY-expansion
	maxX, maxY = maxX+expansion, maxY+expansion
	if maxX < float64(canvas.Min.X) || maxY < float64(canvas.Min.Y) || minX > float64(canvas.Max.X) || minY > float64(canvas.Max.Y) {
		return image.Rectangle{}
	}
	minX = math.Max(minX, float64(canvas.Min.X))
	minY = math.Max(minY, float64(canvas.Min.Y))
	maxX = math.Min(maxX, float64(canvas.Max.X))
	maxY = math.Min(maxY, float64(canvas.Max.Y))
	return image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX)), int(math.Ceil(maxY))).Intersect(canvas)
}

func localObjectBounds(paths []subpath) d2scene.Box {
	var bounds d2scene.Bounds
	for _, path := range paths {
		for _, point := range path.points {
			bounds = bounds.Include(point)
		}
	}
	return bounds.Box()
}
