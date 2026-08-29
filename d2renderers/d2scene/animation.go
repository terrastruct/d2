package d2scene

import (
	"image/color"
	"time"
)

type AnimationProperty uint8

const (
	AnimateOpacity AnimationProperty = iota
	AnimateTransform
	AnimateStrokeDashOffset
	AnimateFillColor
	AnimateStrokeColor
	AnimateDropShadow
)

type AnimationValueKind uint8

const (
	NumberAnimationValue AnimationValueKind = iota
	TransformAnimationValue
	ColorAnimationValue
	ShadowAnimationValue
)

// AnimationValue is a small tagged union so renderers must handle every value
// kind explicitly. Unused fields are ignored.
type AnimationValue struct {
	Kind      AnimationValueKind
	Number    float64
	Transform Matrix
	Color     color.NRGBA
	Shadow    DropShadow
}

func NumberValue(value float64) AnimationValue {
	return AnimationValue{Kind: NumberAnimationValue, Number: value}
}

func TransformValue(value Matrix) AnimationValue {
	return AnimationValue{Kind: TransformAnimationValue, Transform: value}
}

func ColorValue(value color.NRGBA) AnimationValue {
	return AnimationValue{Kind: ColorAnimationValue, Color: value}
}

func ShadowValue(value DropShadow) AnimationValue {
	return AnimationValue{Kind: ShadowAnimationValue, Shadow: value}
}

type EasingKind uint8

const (
	EaseLinear EasingKind = iota
	EaseCubicBezier
	EaseStepStart
	EaseStepEnd
)

// Easing describes an outgoing keyframe easing. CubicBezier uses CSS-style
// control points (0,0), (X1,Y1), (X2,Y2), (1,1).
type Easing struct {
	Kind EasingKind
	X1   float64
	Y1   float64
	X2   float64
	Y2   float64
}

type Keyframe struct {
	Offset float64
	Value  AnimationValue
	Easing Easing
}

// Track describes one typed animation. Repeat loops forever. Renderers resolve
// tracks without changing the track or its node, so a scene can be rendered at
// multiple times concurrently.
type Track struct {
	Property AnimationProperty
	// TargetIndex selects an entry for indexed properties such as a drop
	// shadow in Node.Filters. It is zero for scalar node properties.
	TargetIndex int
	Delay       time.Duration
	Duration    time.Duration
	Repeat      bool
	Keyframes   []Keyframe
}
