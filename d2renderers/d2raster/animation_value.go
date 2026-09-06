package d2raster

import (
	"fmt"
	"image/color"
	"math"
	"sort"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// animationValueAt validates and resolves a track with interruptible keyframe
// lookup. Retained vector definitions and each visible instance validate their
// own charged occurrence instead of caching mutable renderer state.
func (p *preflight) animationValueAt(track d2scene.Track) (d2scene.AnimationValue, error) {
	if track.TargetIndex < 0 {
		return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation target index is negative")
	}
	if track.Property > d2scene.AnimateDropShadow {
		return d2scene.AnimationValue{}, fmt.Errorf("d2scene: unknown animation property %d", track.Property)
	}
	if track.Delay < 0 {
		return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation delay is negative")
	}
	if track.Duration <= 0 {
		return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation duration must be positive")
	}
	if len(track.Keyframes) == 0 {
		return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation has no keyframes")
	}

	kind := track.Keyframes[0].Value.Kind
	expectedKind := animationKindForProperty(track.Property)
	if kind != expectedKind {
		return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation property %d requires value kind %d", track.Property, expectedKind)
	}
	previous := -1.0
	for index, keyframe := range track.Keyframes {
		if index&255 == 0 {
			if err := p.ctx.Err(); err != nil {
				return d2scene.AnimationValue{}, err
			}
		}
		if !finite(keyframe.Offset) || keyframe.Offset < 0 || keyframe.Offset > 1 || keyframe.Offset < previous {
			return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation keyframe %d has invalid offset", index)
		}
		if keyframe.Value.Kind != kind {
			return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation keyframe %d changes value kind", index)
		}
		if err := validateAnimationValue(keyframe.Value); err != nil {
			return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation keyframe %d: %w", index, err)
		}
		if err := validateAnimationEasing(keyframe.Easing); err != nil {
			return d2scene.AnimationValue{}, fmt.Errorf("d2scene: animation keyframe %d: %w", index, err)
		}
		previous = keyframe.Offset
	}
	if err := p.ctx.Err(); err != nil {
		return d2scene.AnimationValue{}, err
	}

	first := track.Keyframes[0]
	last := track.Keyframes[len(track.Keyframes)-1]
	if p.options.Time <= track.Delay {
		return first.Value, nil
	}
	local := p.options.Time - track.Delay
	if !track.Repeat && local >= track.Duration {
		return last.Value, nil
	}
	if track.Repeat {
		local %= track.Duration
	}
	progress := float64(local) / float64(track.Duration)
	if progress <= first.Offset {
		return first.Value, nil
	}
	rightIndex := sort.Search(len(track.Keyframes)-1, func(index int) bool {
		return track.Keyframes[index+1].Offset >= progress
	}) + 1
	if rightIndex >= len(track.Keyframes) {
		return last.Value, nil
	}
	right := track.Keyframes[rightIndex]
	left := track.Keyframes[rightIndex-1]
	span := right.Offset - left.Offset
	if span == 0 {
		return right.Value, nil
	}
	position := (progress - left.Offset) / span
	position = applyAnimationEasing(left.Easing, position)
	return interpolateRendererAnimationValue(left.Value, right.Value, position)
}

func animationKindForProperty(property d2scene.AnimationProperty) d2scene.AnimationValueKind {
	switch property {
	case d2scene.AnimateOpacity, d2scene.AnimateStrokeDashOffset:
		return d2scene.NumberAnimationValue
	case d2scene.AnimateTransform:
		return d2scene.TransformAnimationValue
	case d2scene.AnimateFillColor, d2scene.AnimateStrokeColor:
		return d2scene.ColorAnimationValue
	case d2scene.AnimateDropShadow:
		return d2scene.ShadowAnimationValue
	default:
		// The caller rejects unknown properties first, so the zero value is
		// unreachable for a valid track.
		return d2scene.NumberAnimationValue
	}
}

func validateAnimationValue(value d2scene.AnimationValue) error {
	switch value.Kind {
	case d2scene.NumberAnimationValue:
		if !finite(value.Number) {
			return fmt.Errorf("non-finite animation number")
		}
	case d2scene.TransformAnimationValue:
		if !value.Transform.IsFinite() {
			return fmt.Errorf("non-finite animation transform")
		}
	case d2scene.ColorAnimationValue:
		return nil
	case d2scene.ShadowAnimationValue:
		shadow := value.Shadow
		if shadow.SigmaX < 0 || shadow.SigmaY < 0 || !finite(shadow.SigmaX) || !finite(shadow.SigmaY) || !finite(shadow.OffsetX) || !finite(shadow.OffsetY) {
			return fmt.Errorf("invalid animation shadow")
		}
	default:
		return fmt.Errorf("unknown animation value kind %d", value.Kind)
	}
	return nil
}

func validateAnimationEasing(easing d2scene.Easing) error {
	switch easing.Kind {
	case d2scene.EaseLinear, d2scene.EaseStepStart, d2scene.EaseStepEnd:
		return nil
	case d2scene.EaseCubicBezier:
		if !finite(easing.X1) || !finite(easing.Y1) || !finite(easing.X2) || !finite(easing.Y2) || easing.X1 < 0 || easing.X1 > 1 || easing.X2 < 0 || easing.X2 > 1 {
			return fmt.Errorf("invalid cubic-bezier easing")
		}
		return nil
	default:
		return fmt.Errorf("unknown easing kind %d", easing.Kind)
	}
}

func applyAnimationEasing(easing d2scene.Easing, value float64) float64 {
	value = math.Max(0, math.Min(1, value))
	switch easing.Kind {
	case d2scene.EaseStepStart:
		if value == 0 {
			return 0
		}
		return 1
	case d2scene.EaseStepEnd:
		if value == 1 {
			return 1
		}
		return 0
	case d2scene.EaseCubicBezier:
		return rendererCubicBezierYForX(value, easing.X1, easing.Y1, easing.X2, easing.Y2)
	default:
		return value
	}
}

func rendererCubicBezierYForX(x, x1, y1, x2, y2 float64) float64 {
	low, high := 0.0, 1.0
	parameter := x
	for range 30 {
		parameter = (low + high) / 2
		estimate := rendererCubicScalar(0, x1, x2, 1, parameter)
		if estimate < x {
			low = parameter
		} else {
			high = parameter
		}
	}
	return rendererCubicScalar(0, y1, y2, 1, parameter)
}

func rendererCubicScalar(p0, p1, p2, p3, value float64) float64 {
	u := 1 - value
	return u*u*u*p0 + 3*u*u*value*p1 + 3*u*value*value*p2 + value*value*value*p3
}

func interpolateRendererAnimationValue(left, right d2scene.AnimationValue, value float64) (d2scene.AnimationValue, error) {
	if left.Kind != right.Kind {
		return d2scene.AnimationValue{}, fmt.Errorf("d2scene: cannot interpolate different animation value kinds")
	}
	lerp := func(a, b float64) float64 { return a + (b-a)*value }
	switch left.Kind {
	case d2scene.NumberAnimationValue:
		return d2scene.NumberValue(lerp(left.Number, right.Number)), nil
	case d2scene.TransformAnimationValue:
		return d2scene.TransformValue(d2scene.Matrix{
			A: lerp(left.Transform.A, right.Transform.A),
			B: lerp(left.Transform.B, right.Transform.B),
			C: lerp(left.Transform.C, right.Transform.C),
			D: lerp(left.Transform.D, right.Transform.D),
			E: lerp(left.Transform.E, right.Transform.E),
			F: lerp(left.Transform.F, right.Transform.F),
		}), nil
	case d2scene.ColorAnimationValue:
		return d2scene.ColorValue(interpolateRendererAnimationColor(left.Color, right.Color, value)), nil
	case d2scene.ShadowAnimationValue:
		return d2scene.ShadowValue(d2scene.DropShadow{
			OffsetX: lerp(left.Shadow.OffsetX, right.Shadow.OffsetX),
			OffsetY: lerp(left.Shadow.OffsetY, right.Shadow.OffsetY),
			SigmaX:  lerp(left.Shadow.SigmaX, right.Shadow.SigmaX),
			SigmaY:  lerp(left.Shadow.SigmaY, right.Shadow.SigmaY),
			Color:   interpolateRendererAnimationColor(left.Shadow.Color, right.Shadow.Color, value),
		}), nil
	default:
		return d2scene.AnimationValue{}, fmt.Errorf("d2scene: unknown animation value kind %d", left.Kind)
	}
}

func interpolateRendererAnimationColor(left, right color.NRGBA, value float64) color.NRGBA {
	channel := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a) + (float64(b)-float64(a))*value))
	}
	return color.NRGBA{
		R: channel(left.R, right.R),
		G: channel(left.G, right.G),
		B: channel(left.B, right.B),
		A: channel(left.A, right.A),
	}
}
