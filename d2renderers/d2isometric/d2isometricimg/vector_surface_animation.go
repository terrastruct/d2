package d2isometricimg

import (
	"fmt"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// A static SVG uses the same time-zero snapshot as the native PNG renderer.
// The retained source tree remains immutable, including its animation tracks.
func nativeVectorInitialNode(node *d2scene.Node) (*d2scene.Node, error) {
	if len(node.Animations) == 0 {
		return node, nil
	}
	initial := *node
	initial.Filters = append([]d2scene.Filter(nil), node.Filters...)
	for _, track := range node.Animations {
		if len(track.Keyframes) == 0 {
			return nil, fmt.Errorf("native SVG animation has no keyframes")
		}
		value := track.Keyframes[0].Value
		switch track.Property {
		case d2scene.AnimateOpacity:
			initial.Opacity = value.Number
		case d2scene.AnimateTransform:
			initial.Transform = value.Transform
		case d2scene.AnimateDropShadow:
			if track.TargetIndex < 0 || track.TargetIndex >= len(initial.Filters) {
				return nil, fmt.Errorf("native SVG shadow animation target is invalid")
			}
			initial.Filters[track.TargetIndex] = value.Shadow
		default:
			primitive, err := nativeVectorInitialPaint(initial.Primitive, track.Property, value)
			if err != nil {
				return nil, err
			}
			initial.Primitive = primitive
		}
	}
	return &initial, nil
}

func nativeVectorInitialPaint(primitive d2scene.Primitive, property d2scene.AnimationProperty, value d2scene.AnimationValue) (d2scene.Primitive, error) {
	var fill *d2scene.Paint
	var stroke **d2scene.Stroke
	switch p := primitive.(type) {
	case *d2scene.Path:
		if p != nil {
			return nativeVectorInitialPaint(*p, property, value)
		}
	case *d2scene.Rect:
		if p != nil {
			return nativeVectorInitialPaint(*p, property, value)
		}
	case *d2scene.Ellipse:
		if p != nil {
			return nativeVectorInitialPaint(*p, property, value)
		}
	case *d2scene.TextRun:
		if p != nil {
			return nativeVectorInitialPaint(*p, property, value)
		}
	case d2scene.Path:
		fill, stroke = &p.Fill, &p.Stroke
		primitive = &p
	case d2scene.Rect:
		fill, stroke = &p.Fill, &p.Stroke
		primitive = &p
	case d2scene.Ellipse:
		fill, stroke = &p.Fill, &p.Stroke
		primitive = &p
	case d2scene.TextRun:
		fill, stroke = &p.Fill, &p.Stroke
		primitive = &p
	}
	if fill == nil {
		return nil, fmt.Errorf("native SVG animated paint has no painted primitive")
	}
	if property == d2scene.AnimateFillColor {
		*fill = d2scene.SolidPaint{Color: value.Color}
		return primitive, nil
	}
	if stroke == nil || *stroke == nil {
		return nil, fmt.Errorf("native SVG animated stroke is missing")
	}
	copy := **stroke
	switch property {
	case d2scene.AnimateStrokeColor:
		copy.Paint = d2scene.SolidPaint{Color: value.Color}
	case d2scene.AnimateStrokeDashOffset:
		copy.DashOffset = value.Number
	default:
		return nil, fmt.Errorf("native SVG unsupported animation property %d", property)
	}
	*stroke = &copy
	return primitive, nil
}
